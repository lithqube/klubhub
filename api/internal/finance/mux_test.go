package finance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMux_RewritesPath tests the route dispatcher that mounts all finance
// handlers behind a single parent prefix stripper. PlunkSender is not used
// here; this is a HTTP routing contract only.
func TestMux_RewritesPath(t *testing.T) {
	type route struct {
		path          string
		wantSubstring string
	}

	cases := []route{
		{"/api/v1/finance/billing-profile", "billing"},
		{"/api/v1/finance/invoices", "invoices"},
		{"/api/v1/finance/invoices/abc-123", "invoices"},
		{"/api/v1/finance/invoices/summaries", "invoices"},
		{"/api/v1/finance/payments", "payments"},
		{"/api/v1/finance/documents", "documents"},
		{"/api/v1/finance/agreements/templates", "agreement"},
		{"/api/v1/finance/agreements/instances", "agreement"},
		{"/api/v1/finance/emails", "email"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var seenPath string
			mux := NewMux(
				echoHandler(&seenPath, "billing"),
				echoHandler(&seenPath, "invoices"),
				echoHandler(&seenPath, "payments"),
				echoHandler(&seenPath, "documents"),
				echoHandler(&seenPath, "agreement-template"),
				echoHandler(&seenPath, "agreement-instance"),
				echoHandler(&seenPath, "email"),
			)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(seenPath, tc.wantSubstring) {
				t.Fatalf("expected path rewritten under /api/v1/%s..., got %q", tc.wantSubstring, seenPath)
			}
		})
	}
}

// TestMux_DispatchesToOwningHandler checks each path reaches the right
// child with its full, unmodified path (children parse /api/v1/finance/...).
func TestMux_DispatchesToOwningHandler(t *testing.T) {
	cases := []struct{ path, wantLabel string }{
		{"/api/v1/finance/invoices/11111111-1111-1111-1111-111111111111", "invoices"},
		{"/api/v1/finance/invoices/11111111-1111-1111-1111-111111111111/issue", "invoices"},
		{"/api/v1/finance/invoices/11111111-1111-1111-1111-111111111111/payments", "payments"},
		{"/api/v1/finance/payments/22222222-2222-2222-2222-222222222222", "payments"},
		{"/api/v1/finance/agreements/instances/abc/sign", "agreement-instance"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var seen string
			mux := NewMux(
				echoHandler(&seen, "billing"),
				echoHandler(&seen, "invoices"),
				echoHandler(&seen, "payments"),
				echoHandler(&seen, "documents"),
				echoHandler(&seen, "agreement-template"),
				echoHandler(&seen, "agreement-instance"),
				echoHandler(&seen, "email"),
			)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if want := tc.wantLabel + ":" + tc.path; seen != want {
				t.Fatalf("seen %q, want %q", seen, want)
			}
		})
	}
}

// TestMux_NilHandlerReturns503 documents the partial-rollout behavior.
func TestMux_NilHandlerReturns503(t *testing.T) {
	mux := NewMux(nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "service_unavailable" {
		t.Fatalf("error=%v want service_unavailable", body["error"])
	}
}

// TestMux_UnknownResourceReturns404 covers the negative dispatch paths.
func TestMux_UnknownResourceReturns404(t *testing.T) {
	mux := NewMux(nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/finance/this-does-not-exist", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusNotFound)
	}
}

// echoHandler returns a handler that records the request path it sees
// after Mux dispatch.
func echoHandler(seen *string, label string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = label + ":" + r.URL.Path
		writeJSON(w, http.StatusOK, map[string]any{"path": r.URL.Path})
	})
}
