package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/platform/config"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
)

// mockPool implements a minimal interface for testing health by wrapping pgxpool.Pool.
// We use a real pool but point at a bad URL to simulate DB failure.
// For the "DB connected" tests we use a no-op pool shim.

// fakeStore implements the storage health check interface used by HealthHandler.
type fakeStore struct {
	err error
}

func (f *fakeStore) HealthCheck(ctx context.Context) error {
	return f.err
}

// fakePool simulates the DB ping behaviour.
type fakePool struct {
	pingErr error
}

func (f *fakePool) Ping(ctx context.Context) error {
	return f.pingErr
}

func testConfig(spotifyID string) *config.Config {
	return &config.Config{
		DatabaseURL:       "postgres://test:test@localhost/test",
		S3Endpoint:        "localhost:39000",
		S3AccessKey:       "test",
		S3SecretKey:       "test",
		S3PublicEndpoint:  "http://localhost:39000",
		S3Bucket:          "test",
		SpotifyClientID:   spotifyID,
		DiscogsAPIKey:     "",
		InstagramClientID: "",
	}
}

func TestHealthHandler_AllHealthy(t *testing.T) {
	cfg := testConfig("spotify-id-set")
	handler := apphttp.NewHealthHandler(&fakePool{pingErr: nil}, &fakeStore{err: nil}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp apphttp.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status=healthy, got %q", resp.Status)
	}

	if resp.Integrations["database"].Status != "ok" {
		t.Errorf("expected database=ok, got %q", resp.Integrations["database"].Status)
	}

	if resp.Integrations["storage"].Status != "ok" {
		t.Errorf("expected storage=ok, got %q", resp.Integrations["storage"].Status)
	}
}

func TestHealthHandler_DBFails_Degraded(t *testing.T) {
	cfg := testConfig("")
	handler := apphttp.NewHealthHandler(
		&fakePool{pingErr: context.DeadlineExceeded},
		&fakeStore{err: nil},
		cfg,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// B.2: a failed DB subsystem must surface as HTTP 503 so Docker's
	// healthcheck (and external monitors) see a real failure rather than
	// an always-200 probe.
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on DB failure, got %d", rr.Code)
	}

	var resp apphttp.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("expected status=unhealthy when DB fails, got %q", resp.Status)
	}

	if resp.Integrations["database"].Status != "error" {
		t.Errorf("expected database=error, got %q", resp.Integrations["database"].Status)
	}
}

// TestHealthHandler_StorageFails_Degraded asserts that a degraded storage
// subsystem surfaces as HTTP 503 (mirrors the DB failure case).
func TestHealthHandler_StorageFails_Degraded(t *testing.T) {
	cfg := testConfig("")
	handler := apphttp.NewHealthHandler(
		&fakePool{pingErr: nil},
		&fakeStore{err: errors.New("bucket does not exist")},
		cfg,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on storage failure, got %d", rr.Code)
	}

	var resp apphttp.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("expected status=unhealthy when storage fails, got %q", resp.Status)
	}

	if resp.Integrations["storage"].Status != "error" {
		t.Errorf("expected storage=error, got %q", resp.Integrations["storage"].Status)
	}
}

// TestHealthHandler_BothFail_Degraded asserts 503 when both DB and storage
// fail simultaneously.
func TestHealthHandler_BothFail_Degraded(t *testing.T) {
	cfg := testConfig("")
	handler := apphttp.NewHealthHandler(
		&fakePool{pingErr: context.DeadlineExceeded},
		&fakeStore{err: errors.New("storage down")},
		cfg,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when both subsystems fail, got %d", rr.Code)
	}
}

func TestHealthHandler_SpotifyUnconfigured(t *testing.T) {
	cfg := testConfig("") // empty SpotifyClientID
	handler := apphttp.NewHealthHandler(&fakePool{pingErr: nil}, &fakeStore{err: nil}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp apphttp.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Integrations["spotify"].Status != "unconfigured" {
		t.Errorf("expected spotify=unconfigured when SpotifyClientID empty, got %q", resp.Integrations["spotify"].Status)
	}
}

func TestHealthHandler_SpotifyConfigured(t *testing.T) {
	cfg := testConfig("my-spotify-client-id")
	handler := apphttp.NewHealthHandler(&fakePool{pingErr: nil}, &fakeStore{err: nil}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp apphttp.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Integrations["spotify"].Status != "ok" {
		t.Errorf("expected spotify=ok when SpotifyClientID set, got %q", resp.Integrations["spotify"].Status)
	}
}

func TestHealthHandler_AllSixIntegrationKeys(t *testing.T) {
	cfg := testConfig("")
	handler := apphttp.NewHealthHandler(&fakePool{pingErr: nil}, &fakeStore{err: nil}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp apphttp.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	required := []string{"database", "storage", "spotify", "discogs", "musicbrainz", "instagram"}
	for _, key := range required {
		if _, ok := resp.Integrations[key]; !ok {
			t.Errorf("missing integration key: %q", key)
		}
	}
}

func TestHealthHandler_RespondsUnder500ms(t *testing.T) {
	cfg := testConfig("spotify-id")
	handler := apphttp.NewHealthHandler(&fakePool{pingErr: nil}, &fakeStore{err: nil}, cfg)

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("health handler took %v, expected < 500ms", elapsed)
	}
}

// compile-time check: ensure pgxpool.Pool satisfies DBPinger (won't fail at test time, only compile time)
var _ apphttp.DBPinger = (*pgxpool.Pool)(nil)

func TestHealthHandler_ReportsEditionFeatures(t *testing.T) {
	for _, on := range []bool{false, true} {
		cfg := testConfig("")
		cfg.Features.RAImport = on
		handler := apphttp.NewHealthHandler(&fakePool{}, &fakeStore{}, cfg)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

		var resp apphttp.HealthResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		got, ok := resp.Features["ra_import"]
		if !ok || got != on {
			t.Errorf("RAImport=%v: expected features.ra_import=%v, got %v (present=%v)", on, on, got, ok)
		}
	}
}
