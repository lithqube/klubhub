package settings_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/settings"
)

// Handler contract tests drive the real handler -> service -> repository
// chain against the disposable Postgres from TestMain (repository_test.go).

func newHandler() *settings.Handler {
	return settings.NewHandler(settings.NewService(settings.NewRepository(testPool)))
}

func doReq(h http.Handler, method, body string) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != "" {
		rdr = bytes.NewReader([]byte(body))
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/api/v1/settings", rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// getToken seeds the singleton (if needed) and returns its updated_at
// optimistic-concurrency token.
func getToken(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := doReq(h, http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET returned %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET body is not JSON: %v", err)
	}
	token, ok := body["updated_at"].(string)
	if !ok || token == "" {
		t.Fatalf("GET response missing updated_at token: %v", body)
	}
	return token
}

func TestHandlerGet_CreatesSingleton(t *testing.T) {
	clearSettings(t)
	h := newHandler()

	rec := doReq(h, http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET returned %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET body is not JSON: %v", err)
	}
	if id, _ := body["id"].(string); id == "" {
		t.Error("GET response missing id")
	}
	if _, ok := body["updated_at"].(string); !ok {
		t.Error("GET response missing updated_at")
	}
}

func TestHandlerPut_RoundTrip(t *testing.T) {
	clearSettings(t)
	h := newHandler()
	token := getToken(t, h)

	payload, err := json.Marshal(map[string]any{
		"updated_at": token,
		"dj_name":    "Coverage DJ",
		"bio_short":  "short bio",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := doReq(h, http.MethodPut, string(payload))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT returned %d: %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("PUT body is not JSON: %v", err)
	}
	if updated["dj_name"] != "Coverage DJ" {
		t.Errorf("PUT response dj_name = %v, want Coverage DJ", updated["dj_name"])
	}

	// The write is persisted, not just echoed.
	get := doReq(h, http.MethodGet, "")
	var stored map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &stored); err != nil {
		t.Fatalf("GET body is not JSON: %v", err)
	}
	if stored["dj_name"] != "Coverage DJ" || stored["bio_short"] != "short bio" {
		t.Errorf("persisted settings = %v, want dj_name/bio_short updated", stored)
	}
}

func TestHandlerPut_StaleTokenConflict(t *testing.T) {
	clearSettings(t)
	h := newHandler()
	token := getToken(t, h)

	put := func(tok string) *httptest.ResponseRecorder {
		payload, err := json.Marshal(map[string]any{"updated_at": tok, "dj_name": "X"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return doReq(h, http.MethodPut, string(payload))
	}

	if rec := put(token); rec.Code != http.StatusOK {
		t.Fatalf("first PUT returned %d: %s", rec.Code, rec.Body.String())
	}
	// Replaying the same (now stale) token must conflict, not silently
	// overwrite — that is the optimistic-concurrency contract.
	rec := put(token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale-token PUT returned %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("409 body is not JSON: %v", err)
	}
	if body["error"] != "conflict" {
		t.Errorf("error code = %q, want conflict", body["error"])
	}
}

func TestHandlerPut_InvalidJSON(t *testing.T) {
	clearSettings(t)
	h := newHandler()

	rec := doReq(h, http.MethodPut, `{"dj_name":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT invalid JSON returned %d, want 400", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("400 body is not JSON: %v", err)
	}
	if body["error"] != "bad_request" {
		t.Errorf("error code = %q, want bad_request", body["error"])
	}
}

// The route-specific 1 MiB cap must surface as 413 (Plan B.6), not a generic
// 400, so the middleware contract is observable end to end.
func TestHandlerPut_OversizedBodyIs413(t *testing.T) {
	clearSettings(t)
	h := newHandler()

	big := `{"dj_name":"` + strings.Repeat("a", 1<<20) + `"}`
	rec := doReq(h, http.MethodPut, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized PUT returned %d, want 413: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("413 body is not JSON: %v", err)
	}
	if body["error"] != "request_too_large" {
		t.Errorf("error code = %q, want request_too_large", body["error"])
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	clearSettings(t)
	h := newHandler()

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		rec := doReq(h, method, "")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s returned %d, want 405", method, rec.Code)
		}
	}
}
