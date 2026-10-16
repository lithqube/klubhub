package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/outreach"
	"github.com/klubhub/dj/api/internal/platform/config"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/rs/zerolog"
)

// originOutreachService is a minimal outreach.ServiceIface: every method
// answers with a canned value so the route table is what is under test.
type originOutreachService struct{}

func (originOutreachService) ListSlots(ctx context.Context) ([]*outreach.Slot, error) {
	return []*outreach.Slot{}, nil
}
func (originOutreachService) CreateSlot(ctx context.Context, in *outreach.SlotCreate) (*outreach.Slot, error) {
	return &outreach.Slot{}, nil
}
func (originOutreachService) DeleteSlot(ctx context.Context, id uuid.UUID) error { return nil }
func (originOutreachService) ListTemplates(ctx context.Context) ([]*outreach.Template, error) {
	return []*outreach.Template{}, nil
}
func (originOutreachService) CreateTemplate(ctx context.Context, in *outreach.TemplateCreate) (*outreach.Template, error) {
	return &outreach.Template{}, nil
}
func (originOutreachService) UpdateTemplate(ctx context.Context, id uuid.UUID, in *outreach.TemplateUpdate) (*outreach.Template, error) {
	return &outreach.Template{}, nil
}
func (originOutreachService) DeleteTemplate(ctx context.Context, id uuid.UUID) error { return nil }
func (originOutreachService) MatchSlots(ctx context.Context, slotID *uuid.UUID) ([]*outreach.SlotMatch, error) {
	return []*outreach.SlotMatch{}, nil
}
func (originOutreachService) SendRequest(ctx context.Context, in *outreach.SendRequestInput) (*outreach.Request, error) {
	return &outreach.Request{}, nil
}
func (originOutreachService) ListRequests(ctx context.Context) ([]*outreach.Request, error) {
	return []*outreach.Request{}, nil
}
func (originOutreachService) GetRequest(ctx context.Context, id uuid.UUID) (*outreach.Request, error) {
	return &outreach.Request{}, nil
}
func (originOutreachService) ListSuppressions(ctx context.Context) ([]*outreach.Suppression, error) {
	return []*outreach.Suppression{}, nil
}
func (originOutreachService) AddSuppression(ctx context.Context, in *outreach.SuppressionCreate) (*outreach.Suppression, error) {
	return &outreach.Suppression{}, nil
}
func (originOutreachService) DeleteSuppression(ctx context.Context, id uuid.UUID) error { return nil }

// TestDJOutreachRegisteredRoutesOriginContract proves the outreach handler is
// mounted at /api/v1/outreach with the prefix stripped: every route answers
// from the real handler (200/201/204 on the canned service), not the router's
// 404.
func TestDJOutreachRegisteredRoutesOriginContract(t *testing.T) {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	handler := outreach.NewHandler(originOutreachService{}).Routes()
	router := apphttp.NewRouter(&config.Config{CORSOrigin: "https://dj.example"}, nil, nil, zerolog.Nop(), noop, noop, noop, noop, noop, noop, noop, noop, noop, handler)
	const id = "11111111-1111-1111-1111-111111111111"
	for _, route := range []struct {
		method, path string
		body         string
		want         int
	}{
		{"GET", "availability/slots", "", 200},
		{"POST", "availability/slots", `{"date":"2026-11-01","city":"Berlin"}`, 201},
		{"DELETE", "availability/slots/" + id, "", 204},
		{"GET", "availability/matches", "", 200},
		{"GET", "availability/matches?slot_id=" + id, "", 200},
		{"GET", "templates", "", 200},
		{"POST", "templates", `{"name":"t","subject":"s","body":"b"}`, 201},
		{"PUT", "templates/" + id, `{"version":1}`, 200},
		{"DELETE", "templates/" + id, "", 204},
		{"POST", "requests", `{"contact_id":"` + id + `"}`, 201},
		{"GET", "requests", "", 200},
		{"GET", "requests/" + id, "", 200},
		{"GET", "suppressions", "", 200},
		{"POST", "suppressions", `{"email":"a@b.co"}`, 201},
		{"DELETE", "suppressions/" + id, "", 204},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, "http://dj.example/api/v1/outreach/"+route.path, strings.NewReader(route.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != route.want {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, route.want, rec.Body.String())
			}
		})
	}
}
