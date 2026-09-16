package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// minimalRequiredEnv sets the env vars config.Load() requires, returning a
// cleanup func that restores the previous state. Without these, config.Load
// returns an error and runHealthcheck exits 1 before any HTTP work happens.
func minimalRequiredEnv(t *testing.T) func() {
	t.Helper()
	prev := map[string]string{
		"DATABASE_URL":  os.Getenv("DATABASE_URL"),
		"S3_ENDPOINT":   os.Getenv("S3_ENDPOINT"),
		"S3_ACCESS_KEY": os.Getenv("S3_ACCESS_KEY"),
		"S3_SECRET_KEY": os.Getenv("S3_SECRET_KEY"),

		"BIND_ADDRESS":         os.Getenv("BIND_ADDRESS"),
		"PORT":                 os.Getenv("PORT"),
		"TOKEN_ENCRYPTION_KEY": os.Getenv("TOKEN_ENCRYPTION_KEY"),
		"ICAL_SECRET":          os.Getenv("ICAL_SECRET"),
	}
	required := map[string]string{
		"DATABASE_URL":  "postgres://localhost/test",
		"S3_ENDPOINT":   "localhost:39000",
		"S3_ACCESS_KEY": "test",
		"S3_SECRET_KEY": "test",

		"BIND_ADDRESS":         "127.0.0.1",
		"PORT":                 "0",
		"TOKEN_ENCRYPTION_KEY": "abcdefghijklmnopqrstuvwxyz123456",
		"ICAL_SECRET":          "this-is-a-32-byte-test-secret-1234",
	}
	for k, v := range required {
		os.Setenv(k, v)
	}
	return func() {
		for k, v := range prev {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}

// startFakeHealthServer spins up an httptest.Server that mimics the health
// handler's contract: returns 200 for ok, 503 for unhealthy.
func startFakeHealthServer(t *testing.T, ok bool) (*httptest.Server, func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if ok {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"status":"healthy"}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, `{"status":"unhealthy"}`)
	})
	srv := httptest.NewServer(mux)
	return srv, srv.Close
}

// freePort asks the kernel for a free TCP port.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not get free port: %v", err)
	}
	defer ln.Close()
	return fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
}

func TestRunHealthcheck_Healthy_Returns0(t *testing.T) {
	cleanup := minimalRequiredEnv(t)
	defer cleanup()

	srv, stop := startFakeHealthServer(t, true)
	defer stop()

	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	os.Setenv("BIND_ADDRESS", host)
	os.Setenv("PORT", port)

	if code := runHealthcheck(); code != 0 {
		t.Fatalf("expected exit 0 on healthy /health, got %d", code)
	}
}

func TestRunHealthcheck_Unhealthy_Returns1(t *testing.T) {
	cleanup := minimalRequiredEnv(t)
	defer cleanup()

	srv, stop := startFakeHealthServer(t, false)
	defer stop()

	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	os.Setenv("BIND_ADDRESS", host)
	os.Setenv("PORT", port)

	if code := runHealthcheck(); code != 1 {
		t.Fatalf("expected exit 1 on 503 /health, got %d", code)
	}
}

func TestRunHealthcheck_Unreachable_Returns1(t *testing.T) {
	cleanup := minimalRequiredEnv(t)
	defer cleanup()

	os.Setenv("BIND_ADDRESS", "127.0.0.1")
	os.Setenv("PORT", freePort(t))

	if code := runHealthcheck(); code != 1 {
		t.Fatalf("expected exit 1 when server unreachable, got %d", code)
	}
}

func TestRunHealthcheck_ConfigLoadFailure_Returns1(t *testing.T) {
	keys := []string{"DATABASE_URL", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY"}
	prev := map[string]string{}
	for _, k := range keys {
		prev[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range prev {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	if code := runHealthcheck(); code != 1 {
		t.Fatalf("expected exit 1 on config.Load failure, got %d", code)
	}
}

// TestHealthcheckFlag_Parsing exercises the flag-parsing branch in main()
// via a mirror flag set. We can't invoke main() from a test because
// os.Exit would kill the runner, so this verifies the parsing contract
// only. The end-to-end wiring is asserted by the runHealthcheck tests
// above.
func TestHealthcheckFlag_Parsing(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no flag", []string{}, false},
		{"-healthcheck", []string{"-healthcheck"}, true},
		{"-healthcheck=true", []string{"-healthcheck=true"}, true},
		{"-healthcheck=false", []string{"-healthcheck=false"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("api-test", flag.ContinueOnError)
			hc := fs.Bool("healthcheck", false, "")
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parse %v: %v", tc.args, err)
			}
			if *hc != tc.want {
				t.Fatalf("for args %v: want %v, got %v", tc.args, tc.want, *hc)
			}
		})
	}
}

// silence the unused-error import guard when the test file is read in
// isolation; remove if/when an error path test is added that uses errors.
var _ = errors.New

// ----------------------------------------------------------------------------
// B.3 graceful shutdown tests
//
// These tests verify the fix for the audit finding at api/cmd/api/main.go
// lines 163-170 (the pre-B.3 signal handler that cancelled ctx but never
// called http.Server.Shutdown). The contract under test:
//
//   - In-flight HTTP requests started before Shutdown() MUST complete
//     successfully (not be cut off mid-response).
//   - After Shutdown returns, no new connections are accepted.
//   - The publish worker goroutine MUST have fully exited before
//     gracefulShutdown returns nil.
//   - If the bounded timeout elapses, gracefulShutdown returns an error
//     but does NOT block indefinitely.
// ----------------------------------------------------------------------------

// silentLogger returns a zerolog.Logger that drops all output. Tests use
// it instead of stdout-spamming the runner.
func silentLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).Level(zerolog.Disabled)
}

// newSlowHandler returns an http.Handler whose /slow endpoint blocks for
// `block` before returning 200. Used to assert gracefulShutdown lets
// in-flight requests finish.
func newSlowHandler(block time.Duration) (http.Handler, *atomic.Int32, *atomic.Int32) {
	var started, completed atomic.Int32
	h := http.NewServeMux()
	h.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		started.Add(1)
		time.Sleep(block)
		completed.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "done")
	})
	h.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return h, &started, &completed
}

func TestGracefulShutdown_InflightRequestCompletes(t *testing.T) {
	// Server with a slow handler: 750ms per request. Shutdown timeout
	// 5s should easily let it finish.
	slow := 750 * time.Millisecond
	handler, started, completed := newSlowHandler(slow)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}

	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		defer serveWG.Done()
		_ = srv.Serve(ln)
	}()

	// Wait a moment for the server to be ready, then fire a slow request.
	addr := ln.Addr().String()
	ready := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				c.Close()
				close(ready)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	<-ready

	var clientErr error
	var clientWG sync.WaitGroup
	clientWG.Add(1)
	var clientStatus int
	go func() {
		defer clientWG.Done()
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			clientErr = err
			return
		}
		defer resp.Body.Close()
		clientStatus = resp.StatusCode
	}()

	// Give the request time to reach the handler.
	deadline := time.Now().Add(500 * time.Millisecond)
	for started.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if started.Load() == 0 {
		t.Fatal("slow request did not reach the handler within 500ms")
	}

	// Now trigger graceful shutdown with a generous 5s budget.
	workerDone := make(chan struct{})
	close(workerDone) // no worker; pretend it already finished.
	if err := gracefulShutdown(srv, workerDone, 5*time.Second, silentLogger()); err != nil {
		t.Fatalf("gracefulShutdown returned error: %v", err)
	}

	// Client must have received a 200 with full body.
	clientWG.Wait()
	if clientErr != nil {
		t.Fatalf("client got error during graceful shutdown: %v", clientErr)
	}
	if clientStatus != http.StatusOK {
		t.Fatalf("client status = %d; want 200", clientStatus)
	}
	if completed.Load() != 1 {
		t.Fatalf("in-flight request did not complete (completed=%d; want 1)", completed.Load())
	}
	serveWG.Wait()
}

func TestGracefulShutdown_TimeoutForceCloseReturnsError(t *testing.T) {
	// Slow handler blocks for 2s; shutdown budget is 100ms. Graceful
	// shutdown must give up, force-close, and return ctx.Err().
	handler, started, _ := newSlowHandler(2 * time.Second)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}

	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		defer serveWG.Done()
		_ = srv.Serve(ln)
	}()
	addr := ln.Addr().String()

	// Wait until ready.
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Fire a slow request, wait for it to be in-flight, then shut down.
	go func() {
		_, _ = http.Get("http://" + addr + "/slow")
	}()
	deadline := time.Now().Add(500 * time.Millisecond)
	for started.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if started.Load() == 0 {
		t.Fatal("slow request did not reach the handler within 500ms")
	}

	workerDone := make(chan struct{})
	start := time.Now()
	err = gracefulShutdown(srv, workerDone, 100*time.Millisecond, silentLogger())
	elapsed := time.Since(start)
	serveWG.Wait()

	if err == nil {
		t.Fatal("gracefulShutdown returned nil; want timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("gracefulShutdown err = %v; want context.DeadlineExceeded", err)
	}
	// Must NOT have blocked waiting for the full 2s handler.
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("gracefulShutdown took %v; expected to give up around 100ms", elapsed)
	}
}

func TestGracefulShutdown_WaitsForWorkerDone(t *testing.T) {
	// No slow handler; just assert gracefulShutdown returns nil ONLY
	// after the workerDone channel is closed.
	handler := http.NewServeMux()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		defer serveWG.Done()
		_ = srv.Serve(ln)
	}()
	addr := ln.Addr().String()
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	workerDone := make(chan struct{})
	closed := make(chan struct{})

	go func() {
		// After 200ms close workerDone; gracefulShutdown should return
		// around then, not earlier.
		time.Sleep(200 * time.Millisecond)
		close(workerDone)
		close(closed)
	}()

	start := time.Now()
	if err := gracefulShutdown(srv, workerDone, 5*time.Second, silentLogger()); err != nil {
		t.Fatalf("gracefulShutdown err: %v", err)
	}
	elapsed := time.Since(start)
	serveWG.Wait()

	if elapsed < 150*time.Millisecond {
		t.Fatalf("gracefulShutdown returned after only %v; should wait for workerDone", elapsed)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("gracefulShutdown took %v; expected ~200ms", elapsed)
	}
}

func TestGracefulShutdown_WorkerStuckReturnsDeadlineError(t *testing.T) {
	// Worker never finishes; shutdown budget is 100ms. Graceful
	// shutdown must return deadline error.
	handler := http.NewServeMux()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		defer serveWG.Done()
		_ = srv.Serve(ln)
	}()
	addr := ln.Addr().String()
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	workerDone := make(chan struct{}) // intentionally never closed
	start := time.Now()
	err = gracefulShutdown(srv, workerDone, 100*time.Millisecond, silentLogger())
	elapsed := time.Since(start)
	serveWG.Wait()

	if err == nil {
		t.Fatal("gracefulShutdown returned nil; want deadline error for stuck worker")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("gracefulShutdown err = %v; want context.DeadlineExceeded", err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("gracefulShutdown blocked for %v; expected ~100ms", elapsed)
	}
}

func TestGracefulShutdown_ZeroTimeoutUsesDefault(t *testing.T) {
	// timeout <= 0 should fall back to a sane default rather than
	// immediately timing out.
	handler := http.NewServeMux()
	handler.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		defer serveWG.Done()
		_ = srv.Serve(ln)
	}()
	addr := ln.Addr().String()
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	workerDone := make(chan struct{})
	close(workerDone)

	start := time.Now()
	if err := gracefulShutdown(srv, workerDone, 0, silentLogger()); err != nil {
		// With default 30s, this should not error in any reasonable test
		// runtime.
		t.Fatalf("gracefulShutdown with default timeout returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("gracefulShutdown took %v; expected well under 5s", elapsed)
	}
	serveWG.Wait()
}
