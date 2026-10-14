package venue_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/venue"
)

// stubService implements venue.ServiceIface with canned responses and call
// recording so handler tests stay pure unit tests (no database).
type stubService struct {
	createFn       func(ctx context.Context, req *venue.VenueCreate) (*venue.Venue, error)
	getFn          func(ctx context.Context, id uuid.UUID) (*venue.Venue, error)
	listFn         func(ctx context.Context, name, city *string) ([]*venue.Venue, error)
	updateFn       func(ctx context.Context, id uuid.UUID, req *venue.VenueUpdate) (*venue.Venue, error)
	deleteFn       func(ctx context.Context, id uuid.UUID) error
	autocompleteFn func(ctx context.Context, query string, limit int) ([]*venue.Venue, error)

	gotName, gotCity  *string
	gotQuery          string
	gotLimit          int
	autocompleteCalls int
}

func (s *stubService) CreateVenue(ctx context.Context, req *venue.VenueCreate) (*venue.Venue, error) {
	return s.createFn(ctx, req)
}
func (s *stubService) GetVenue(ctx context.Context, id uuid.UUID) (*venue.Venue, error) {
	return s.getFn(ctx, id)
}
func (s *stubService) ListVenues(ctx context.Context, name, city *string) ([]*venue.Venue, error) {
	s.gotName, s.gotCity = name, city
	return s.listFn(ctx, name, city)
}
func (s *stubService) UpdateVenue(ctx context.Context, id uuid.UUID, req *venue.VenueUpdate) (*venue.Venue, error) {
	return s.updateFn(ctx, id, req)
}
func (s *stubService) DeleteVenue(ctx context.Context, id uuid.UUID) error {
	return s.deleteFn(ctx, id)
}
func (s *stubService) Autocomplete(ctx context.Context, query string, limit int) ([]*venue.Venue, error) {
	s.autocompleteCalls++
	s.gotQuery, s.gotLimit = query, limit
	return s.autocompleteFn(ctx, query, limit)
}

func newTestVenue() *venue.Venue {
	return &venue.Venue{
		ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Name:      "Funkhaus",
		City:      "Berlin",
		Country:   "DE",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func serve(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandleListVenues_NoFilters(t *testing.T) {
	svc := &stubService{
		listFn: func(ctx context.Context, name, city *string) ([]*venue.Venue, error) {
			return []*venue.Venue{newTestVenue()}, nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if svc.gotName != nil || svc.gotCity != nil {
		t.Errorf("expected nil filters, got name=%v city=%v", svc.gotName, svc.gotCity)
	}
	var venues []*venue.Venue
	if err := json.NewDecoder(rec.Body).Decode(&venues); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(venues) != 1 || venues[0].Name != "Funkhaus" {
		t.Errorf("unexpected venues: %+v", venues)
	}
}

func TestHandleListVenues_WithFilters(t *testing.T) {
	svc := &stubService{
		listFn: func(ctx context.Context, name, city *string) ([]*venue.Venue, error) {
			return []*venue.Venue{}, nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/?name=funk&city=berlin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotName == nil || *svc.gotName != "funk" {
		t.Errorf("name filter = %v, want funk", svc.gotName)
	}
	if svc.gotCity == nil || *svc.gotCity != "berlin" {
		t.Errorf("city filter = %v, want berlin", svc.gotCity)
	}
}

func TestHandleListVenues_ServiceError(t *testing.T) {
	svc := &stubService{
		listFn: func(ctx context.Context, name, city *string) ([]*venue.Venue, error) {
			return nil, errors.New("db down")
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected error message in body")
	}
}

func TestHandleCreateVenue_Success(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, req *venue.VenueCreate) (*venue.Venue, error) {
			if req.Name != "Funkhaus" {
				t.Errorf("req.Name = %q, want Funkhaus", req.Name)
			}
			return newTestVenue(), nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodPost, "/", `{"name":"Funkhaus","city":"Berlin","country":"DE"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var v venue.Venue
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.ID != newTestVenue().ID {
		t.Errorf("id = %v, want %v", v.ID, newTestVenue().ID)
	}
}

func TestHandleCreateVenue_InvalidJSON(t *testing.T) {
	svc := &stubService{
		createFn: func(ctx context.Context, req *venue.VenueCreate) (*venue.Venue, error) {
			t.Error("service must not be called on invalid JSON")
			return nil, nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodPost, "/", `{"name":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleGetVenue_Success(t *testing.T) {
	id := newTestVenue().ID
	svc := &stubService{
		getFn: func(ctx context.Context, got uuid.UUID) (*venue.Venue, error) {
			if got != id {
				t.Errorf("id = %v, want %v", got, id)
			}
			return newTestVenue(), nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/"+id.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestHandleGetVenue_InvalidID(t *testing.T) {
	h := venue.NewHandler(&stubService{})
	rec := serve(h.Routes(), http.MethodGet, "/not-a-uuid", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleGetVenue_NotFound(t *testing.T) {
	svc := &stubService{
		getFn: func(ctx context.Context, id uuid.UUID) (*venue.Venue, error) {
			return nil, venue.ErrNotFound
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/"+uuid.NewString(), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleGetVenue_ServiceError(t *testing.T) {
	svc := &stubService{
		getFn: func(ctx context.Context, id uuid.UUID) (*venue.Venue, error) {
			return nil, errors.New("db down")
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/"+uuid.NewString(), "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestHandleUpdateVenue_Success(t *testing.T) {
	id := newTestVenue().ID
	svc := &stubService{
		updateFn: func(ctx context.Context, got uuid.UUID, req *venue.VenueUpdate) (*venue.Venue, error) {
			if got != id {
				t.Errorf("id = %v, want %v", got, id)
			}
			if req.Name == nil || *req.Name != "Funkhaus 2" {
				t.Errorf("req.Name = %v, want Funkhaus 2", req.Name)
			}
			return newTestVenue(), nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodPut, "/"+id.String(), `{"name":"Funkhaus 2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateVenue_InvalidID(t *testing.T) {
	h := venue.NewHandler(&stubService{})
	rec := serve(h.Routes(), http.MethodPut, "/nope", `{"name":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleUpdateVenue_InvalidJSON(t *testing.T) {
	h := venue.NewHandler(&stubService{})
	rec := serve(h.Routes(), http.MethodPut, "/"+uuid.NewString(), `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleUpdateVenue_NotFound(t *testing.T) {
	svc := &stubService{
		updateFn: func(ctx context.Context, id uuid.UUID, req *venue.VenueUpdate) (*venue.Venue, error) {
			return nil, venue.ErrNotFound
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodPut, "/"+uuid.NewString(), `{"name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleDeleteVenue_Success(t *testing.T) {
	id := newTestVenue().ID
	svc := &stubService{
		deleteFn: func(ctx context.Context, got uuid.UUID) error {
			if got != id {
				t.Errorf("id = %v, want %v", got, id)
			}
			return nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodDelete, "/"+id.String(), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestHandleDeleteVenue_InvalidID(t *testing.T) {
	h := venue.NewHandler(&stubService{})
	rec := serve(h.Routes(), http.MethodDelete, "/nope", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleDeleteVenue_ServiceError(t *testing.T) {
	svc := &stubService{
		deleteFn: func(ctx context.Context, id uuid.UUID) error {
			return errors.New("db down")
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodDelete, "/"+uuid.NewString(), "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// The static /autocomplete route must win over the /{id} wildcard: with a
// short query the handler short-circuits without touching the service, so a
// 200 with an empty array proves the static route matched (the wildcard would
// 400 on uuid parsing "autocomplete").
func TestHandleAutocomplete_ShortQueryShortCircuits(t *testing.T) {
	svc := &stubService{
		autocompleteFn: func(ctx context.Context, query string, limit int) ([]*venue.Venue, error) {
			t.Error("service must not be called for queries shorter than 2 chars")
			return nil, nil
		},
	}
	h := venue.NewHandler(svc)

	for _, target := range []string{"/autocomplete", "/autocomplete?q=", "/autocomplete?q=a"} {
		rec := serve(h.Routes(), http.MethodGet, target, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", target, rec.Code)
		}
		var venues []*venue.Venue
		if err := json.NewDecoder(rec.Body).Decode(&venues); err != nil {
			t.Fatalf("%s: decode: %v", target, err)
		}
		if len(venues) != 0 {
			t.Errorf("%s: expected empty array, got %+v", target, venues)
		}
	}
	if svc.autocompleteCalls != 0 {
		t.Errorf("autocomplete called %d times, want 0", svc.autocompleteCalls)
	}
}

func TestHandleAutocomplete_DelegatesWithLimit5(t *testing.T) {
	svc := &stubService{
		autocompleteFn: func(ctx context.Context, query string, limit int) ([]*venue.Venue, error) {
			return []*venue.Venue{newTestVenue()}, nil
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/autocomplete?q=funk", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gotQuery != "funk" {
		t.Errorf("query = %q, want funk", svc.gotQuery)
	}
	if svc.gotLimit != 5 {
		t.Errorf("limit = %d, want 5 (handler-fixed)", svc.gotLimit)
	}
}

func TestHandleAutocomplete_ServiceError(t *testing.T) {
	svc := &stubService{
		autocompleteFn: func(ctx context.Context, query string, limit int) ([]*venue.Venue, error) {
			return nil, errors.New("db down")
		},
	}
	h := venue.NewHandler(svc)

	rec := serve(h.Routes(), http.MethodGet, "/autocomplete?q=funk", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
