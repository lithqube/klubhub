package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/platform/config"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/rs/zerolog"
)

func hostRouter(cors string) http.Handler {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return apphttp.NewRouter(&config.Config{CORSOrigin: cors}, nil, nil, zerolog.Nop(), noop, noop, noop, noop, noop, noop, noop, noop)
}

// DNS rebinding: attacker domain resolves to a private address, so the
// browser sends Origin == Host == attacker. Host must be allowlisted.
func TestHostAllowlistUnsafeRequests(t *testing.T) {
	for _, tc := range []struct {
		name, cors, host, origin string
		want                     int
	}{
		{"rebinding Origin==Host evil", "https://dj.example", "evil.example", "http://evil.example", 403},
		{"rebinding with port", "https://dj.example", "evil.example:3000", "http://evil.example:3000", 403},
		{"evil host no Origin", "https://dj.example", "evil.example", "", 403},
		{"LAN IP host not allowlisted", "https://dj.example", "192.168.1.10:8080", "http://192.168.1.10:8080", 403},
		{"localhost with matching Origin", "https://dj.example", "localhost:8080", "http://localhost:8080", 204},
		{"127.0.0.1 with matching Origin", "https://dj.example", "127.0.0.1:8080", "http://127.0.0.1:8080", 204},
		{"[::1] with matching Origin", "https://dj.example", "[::1]:8080", "http://[::1]:8080", 204},
		{"[::1] no port", "https://dj.example", "[::1]", "http://[::1]", 204},
		{"uppercase LOCALHOST", "https://dj.example", "LOCALHOST:8080", "http://localhost:8080", 204},
		{"curl loopback without Origin", "https://dj.example", "127.0.0.1:8080", "", 204},
		{"configured host without port", "https://dj.example", "dj.example", "http://dj.example", 204},
		{"configured host any port", "https://dj.example", "dj.example:8443", "http://dj.example:8443", 204},
		{"configured with port, host with port", "http://dj.example:8080", "dj.example:8080", "http://dj.example:8080", 204},
		{"configured with port, host without port", "http://dj.example:8080", "dj.example", "http://dj.example", 204},
		{"suffix lookalike", "https://dj.example", "dj.example.evil.example", "http://dj.example.evil.example", 403},
		{"prefix lookalike", "https://dj.example", "evil-dj.example", "http://evil-dj.example", 403},
		{"localhost lookalike", "https://dj.example", "localhost.evil.example", "http://localhost.evil.example", 403},
		{"unparseable CORSOrigin still allows loopback", "https://dj.example/", "localhost:3000", "http://localhost:3000", 204},
		{"unparseable CORSOrigin gives no extra host", "https://dj.example/", "dj.example", "http://dj.example", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", "http://"+tc.host+"/api/v1/settings", strings.NewReader("{}"))
			r.Header.Set("Content-Type", "application/json")
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			hostRouter(tc.cors).ServeHTTP(rec, r)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestHostRejectionMessageNamesCORSOrigin(t *testing.T) {
	r := httptest.NewRequest("DELETE", "http://nas.lan:8080/api/v1/settings", nil)
	rec := httptest.NewRecorder()
	hostRouter("http://127.0.0.1:3000").ServeHTTP(rec, r)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "CORSOrigin") || !strings.Contains(rec.Body.String(), "CORS_ORIGIN") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostAllowlistLeavesSafeMethodsAndOtherPaths(t *testing.T) {
	for _, tc := range []struct{ method, path string }{{"GET", "/api/v1/settings"}, {"HEAD", "/api/v1/settings"}, {"OPTIONS", "/api/v1/settings"}} {
		r := httptest.NewRequest(tc.method, "http://evil.example"+tc.path, nil)
		rec := httptest.NewRecorder()
		hostRouter("https://dj.example").ServeHTTP(rec, r)
		if rec.Code == 403 {
			t.Fatalf("%s must not be gated by Host allowlist (unsafe methods only)", tc.method)
		}
	}
}

// Duplicate and empty Origin headers are rejected even when Host is allowed.
func TestOriginDuplicateAndEmptyHeaders(t *testing.T) {
	for name, values := range map[string][]string{
		"duplicate identical": {"http://dj.example", "http://dj.example"},
		"duplicate differing": {"http://dj.example", "https://evil.example"},
		"empty":               {""},
		"empty and valid":     {"", "http://dj.example"},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", "http://dj.example/api/v1/settings", strings.NewReader("{}"))
			r.Header.Set("Content-Type", "application/json")
			r.Header["Origin"] = values
			rec := httptest.NewRecorder()
			hostRouter("https://dj.example").ServeHTTP(rec, r)
			if rec.Code != 403 {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCORSOriginWarning(t *testing.T) {
	for raw, wantWarn := range map[string]bool{
		"":                                    false,
		"http://127.0.0.1:3000":               false,
		"https://dj.example":                  false,
		"https://dj.example:8443":             false,
		"https://dj.example/":                 true,
		"https://dj.example/app":              true,
		"dj.example":                          true,
		"http://%zz":                          true,
		"https://a.example https://b.example": true,
	} {
		got := apphttp.CORSOriginWarning(raw)
		if (got != "") != wantWarn {
			t.Errorf("%q: warning=%q wantWarn=%v", raw, got, wantWarn)
		}
	}
}
