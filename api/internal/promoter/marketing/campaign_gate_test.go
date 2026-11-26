package marketing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// The gate is checked before anything touches the database, so a service
// with no db proves the refusal.
func TestSendCampaign_WithoutAttestationIs422(t *testing.T) {
	h := (&Handler{}).WithCampaigns(NewCampaignService(nil, nil, NewGate(false), nil))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("campaignID", uuid.NewString())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	h.sendCampaign(w, req)
	if w.Code != http.StatusUnprocessableEntity || w.Body.String() != `{"error":"marketing_not_permitted"}` {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}
