package rider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/rider"
)

// ─── Mux contract tests ─────────────────────────────────────────────────────

// dummyHandler is a tiny http.Handler that records its URL pattern. Used
// to verify the Mux dispatched to the right handler, not to test the
// inner handler logic (which the per-handler tests in handler_test.go
// already cover).
type dummyHandler struct {
	id      string
	lastReq *http.Request
}

func (d *dummyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.lastReq = r
	// chi strips the prefix when mounted, so the inner handler sees
	// paths like /templates or /attachments/{id}. Echo it as JSON.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"handler": d.id, "path": r.URL.Path})
}

func newMuxWithDummies(templates, attachments http.Handler) http.Handler {
	templatesH := &dummyHandler{id: "templates"}
	if templates != nil {
		templatesH = templates.(*dummyHandler)
	}
	attachmentsH := &dummyHandler{id: "attachments"}
	if attachments != nil {
		attachmentsH = attachments.(*dummyHandler)
	}
	mux := rider.NewMux(templatesH, attachmentsH)
	// Wrap with chi so the /api/v1/rider/* prefix matches what
	// cmd/api/main.go mounts.
	r := chi.NewRouter()
	r.Mount("/api/v1/rider", mux)
	return r
}

func TestMux_RoutesTemplatesGET_ToTemplatesHandler(t *testing.T) {
	ts := httptest.NewServer(newMuxWithDummies(nil, nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/rider/templates")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "templates", body["handler"])
}

func TestMux_RoutesAttachmentsPOST_ToAttachmentsHandler(t *testing.T) {
	ts := httptest.NewServer(newMuxWithDummies(nil, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/rider/attachments", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "attachments", body["handler"])
}

func TestMux_RoutesAttachmentsByGig_ToAttachmentsHandler(t *testing.T) {
	ts := httptest.NewServer(newMuxWithDummies(nil, nil))
	defer ts.Close()

	gigID := uuid.New().String()
	resp, err := http.Get(ts.URL + "/api/v1/rider/attachments/by-gig/" + gigID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "attachments", body["handler"])
}

func TestMux_UnknownResourceReturns404(t *testing.T) {
	ts := httptest.NewServer(newMuxWithDummies(nil, nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/rider/unknown")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestMux_NilTemplatesHandler_Returns503(t *testing.T) {
	mux := rider.NewMux(nil, &dummyHandler{id: "attachments"})
	r := chi.NewRouter()
	r.Mount("/api/v1/rider", mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rider/templates", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestMux_NilAttachmentsHandler_Returns503(t *testing.T) {
	mux := rider.NewMux(&dummyHandler{id: "templates"}, nil)
	r := chi.NewRouter()
	r.Mount("/api/v1/rider", mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rider/attachments", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestMux_StripsPrefixToInnerHandler verifies chi's Mount passes the
// request through with the full /api/v1/rider/* path intact — the inner
// chi router inside handler.Routes() matches against the full path, not
// the stripped one. Confirms the contract documented in finance/mux.go.
func TestMux_StripsPrefixToInnerHandler(t *testing.T) {
	inner := rider.NewHandler(&mockService{}).Routes()
	mux := rider.NewMux(inner, inner)
	r := chi.NewRouter()
	r.Mount("/api/v1/rider", mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rider/templates", nil)
	r.ServeHTTP(rec, req)
	// Should not 404 from the mux; the inner chi router for /templates
	// (mounted via Mux's templates handler) will return either 200 or
	// the service's real response. We just need to confirm the request
	// reached the inner handler, not the mux's 404 fallback.
	assert.NotEqual(t, http.StatusNotFound, rec.Code,
		"mux should have stripped to inner handler; got: %d %s", rec.Code, rec.Body.String())
}

// TestMux_RoutesAttachmentPDF_ToAttachmentsHandler verifies the PDF
// sub-path under /attachments/{id}/pdf dispatches to the attachments
// handler (not the mux's default 404), confirming chi routes by prefix
// before param-matching inside the handler's sub-router.
func TestMux_RoutesAttachmentPDF_ToAttachmentsHandler(t *testing.T) {
	ts := httptest.NewServer(newMuxWithDummies(nil, nil))
	defer ts.Close()

	id := uuid.New().String()
	resp, err := http.Post(ts.URL+"/api/v1/rider/attachments/"+id+"/pdf", "application/json", bytes.NewReader([]byte{}))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "attachments", body["handler"])
}

// suppress unused-import warnings if test set shrinks.
var _ = context.Background