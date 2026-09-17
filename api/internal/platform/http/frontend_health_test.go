package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/rs/zerolog"
)

func TestFrontendHealth(t *testing.T) {
	for _, status := range []int{200, 302, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/" {
					t.Errorf("probe must not recurse through /api/v1: %s", r.URL.Path)
				}
				w.Header().Set("Location", "/api/v1/health")
				w.WriteHeader(status)
			}))
			defer frontend.Close()
			cfg := frontendConfig(t, "true", frontend.URL)
			handler := apphttp.NewHealthHandler(&fakePool{}, &fakeStore{}, cfg)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/health", nil))
			want := 503
			if status == 200 {
				want = 200
			}
			if rr.Code != want || calls != 1 {
				t.Fatalf("status=%d calls=%d; want %d and 1", rr.Code, calls, want)
			}
		})
	}
}

func TestFrontendHealthDisabledAndUnreachable(t *testing.T) {
	frontend := httptest.NewServer(http.NotFoundHandler())
	frontend.Close()
	for _, enabled := range []string{"false", "true"} {
		cfg := frontendConfig(t, enabled, frontend.URL)
		rr := httptest.NewRecorder()
		apphttp.NewHealthHandler(&fakePool{}, &fakeStore{}, cfg).ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/health", nil))
		want := 200
		if enabled == "true" {
			want = 503
		}
		if rr.Code != want {
			t.Fatalf("enabled=%s: status %d, want %d", enabled, rr.Code, want)
		}
	}
}

func TestFrontendHealthSelfTargetDoesNotLoop(t *testing.T) {
	cfg := frontendConfig(t, "true", "http://127.0.0.1:1")
	api := http.NotFoundHandler()
	router := apphttp.NewRouter(cfg, nil, nil, zerolog.Nop(), api, api, api, api, api, api, api)
	server := httptest.NewServer(router)
	defer server.Close()
	cfg.NuxtInternalURL = server.URL
	rr := httptest.NewRecorder()
	start := time.Now()
	apphttp.NewHealthHandler(&fakePool{}, &fakeStore{}, cfg).ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/health", nil))
	if rr.Code != 503 || time.Since(start) > time.Second {
		t.Fatalf("self-target must fail promptly, got %d", rr.Code)
	}
}
