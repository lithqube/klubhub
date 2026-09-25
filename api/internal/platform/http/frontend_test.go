package http_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/klubhub/dj/api/internal/platform/config"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/rs/zerolog"
)

func frontendConfig(t *testing.T, enabled, target string) *config.Config {
	t.Helper()
	t.Setenv("SERVE_FRONTEND", enabled)
	if enabled == "" {
		os.Unsetenv("SERVE_FRONTEND")
	}
	t.Setenv("NUXT_INTERNAL_URL", target)
	t.Setenv("DATABASE_URL", "postgres://test/test")
	t.Setenv("S3_ENDPOINT", "localhost:3900")
	t.Setenv("S3_ACCESS_KEY", "test")
	t.Setenv("S3_SECRET_KEY", "test")
	t.Setenv("ICAL_SECRET", "test-only")
	cfg := &config.Config{}
	if err := envconfig.Process("", cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestFrontendRouting(t *testing.T) {
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frontend", "true")
		w.Write([]byte(r.URL.RequestURI()))
	}))
	defer frontend.Close()
	for _, enabled := range []string{"", "false", "true"} {
		t.Run("enabled="+enabled, func(t *testing.T) {
			cfg := frontendConfig(t, enabled, frontend.URL)
			api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
			router := apphttp.NewRouter(cfg, nil, nil, zerolog.Nop(), api, api, api, api, api, api, api, api)
			for _, path := range []string{"/", "/tracklists/123?preview=true", "/_nuxt/app.js", "/api/render"} {
				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
				want := 404
				if enabled == "true" {
					want = 200
				}
				if rr.Code != want {
					t.Errorf("%s: got %d, want %d", path, rr.Code, want)
				}
				if enabled == "true" && rr.Body.String() != path {
					t.Errorf("lost path/query: %q", rr.Body.String())
				}
			}
			for _, path := range []string{"/api/v1", "/api/v1/unknown", "/api/v1/settings"} {
				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
				want := 404
				if path == "/api/v1/settings" {
					want = 204
				}
				if rr.Code != want || rr.Header().Get("X-Frontend") != "" {
					t.Errorf("API escaped to frontend: %s: %d", path, rr.Code)
				}
			}
		})
	}
}
