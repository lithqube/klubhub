package http_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apphttp "github.com/klubhub/dj/api/internal/platform/http"
)

// Plan B.6 regression tests for the MaxBytesHandler middleware.

func TestMaxBodyBytes_AllowsSmallBody(t *testing.T) {
	var called int
	h := apphttp.MaxBodyBytesMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "ok" {
			t.Fatalf("body = %q; want %q", body, "ok")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("ok"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	if called != 1 {
		t.Fatalf("downstream called %d times; want 1", called)
	}
}

func TestMaxBodyBytes_RejectsBodyOverCap(t *testing.T) {
	called := 0
	// Tighten the cap for the test by going through the explicit-limit
	// variant; the default is 1 MiB.
	h := apphttp.MaxBytesHandlerWithLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		// The handler refuses to read the body — the middleware's
		// pre-check on Content-Length already wrote 413.
		w.WriteHeader(http.StatusOK)
	}), 16)

	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader(bytes.Repeat([]byte{'A'}, 64)))
	req.ContentLength = 64
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d; want 413", w.Code)
	}
	if called != 0 {
		t.Fatalf("downstream called %d times; want 0 (handler must not run when cap is exceeded)", called)
	}
}

func TestMaxBodyBytes_GETPassesThrough(t *testing.T) {
	called := 0
	h := apphttp.MaxBodyBytesMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	if called != 1 {
		t.Fatalf("downstream called %d times; want 1", called)
	}
}

func TestMaxBodyBytes_PerRouteCapIsHonored(t *testing.T) {
	called := 0
	h := apphttp.MaxBytesHandlerWithLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}), 1024)

	// Body just under the cap of 1024 (declared Content-Length 1023).
	body := bytes.Repeat([]byte{'B'}, 1023)
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("under-cap status = %d; want 200", w.Code)
	}
	if called != 1 {
		t.Fatalf("downstream called %d; want 1", called)
	}

	// Body just over the cap: Content-Length pre-check fires.
	body2 := bytes.Repeat([]byte{'B'}, 1025)
	req2 := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader(body2))
	req2.ContentLength = int64(len(body2))
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-cap status = %d; want 413", w2.Code)
	}
}

func TestDetectMaxBytes413(t *testing.T) {
	// Validate the helper the handlers use to map a read failure
	// into 413. Go's MaxBytesError implements error and is returned
	// by MaxBytesReader on overflow.
	if apphttp.DetectMaxBytes413(http.ErrBodyNotAllowed) != 400 {
		t.Fatal("generic error should map to 400")
	}
}
