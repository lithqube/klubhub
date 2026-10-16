package outreach

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
)

// fakeService implements ServiceIface with canned responses and error injection.
type fakeService struct {
	slots        []*Slot
	templates    []*Template
	matches      []*SlotMatch
	requests     []*Request
	suppressions []*Suppression

	err        error
	gotSlot    *SlotCreate
	gotTpl     *TemplateCreate
	gotTplUpd  *TemplateUpdate
	gotSend    *SendRequestInput
	gotSupp    *SuppressionCreate
	gotMatchID *uuid.UUID
}

func (f *fakeService) ListSlots(ctx context.Context) ([]*Slot, error) { return f.slots, f.err }

func (f *fakeService) CreateSlot(ctx context.Context, in *SlotCreate) (*Slot, error) {
	f.gotSlot = in
	if f.err != nil {
		return nil, f.err
	}
	return &Slot{ID: uuid.New(), Date: in.Date, City: in.City, CreatedAt: time.Now()}, nil
}

func (f *fakeService) DeleteSlot(ctx context.Context, id uuid.UUID) error { return f.err }

func (f *fakeService) ListTemplates(ctx context.Context) ([]*Template, error) {
	return f.templates, f.err
}

func (f *fakeService) CreateTemplate(ctx context.Context, in *TemplateCreate) (*Template, error) {
	f.gotTpl = in
	if f.err != nil {
		return nil, f.err
	}
	return &Template{ID: uuid.New(), Name: in.Name, Subject: in.Subject, Body: in.Body, Version: 1}, nil
}

func (f *fakeService) UpdateTemplate(ctx context.Context, id uuid.UUID, in *TemplateUpdate) (*Template, error) {
	f.gotTplUpd = in
	if f.err != nil {
		return nil, f.err
	}
	return &Template{ID: id, Name: "t", Subject: "s", Body: "b", Version: in.Version + 1}, nil
}

func (f *fakeService) DeleteTemplate(ctx context.Context, id uuid.UUID) error { return f.err }

func (f *fakeService) MatchSlots(ctx context.Context, slotID *uuid.UUID) ([]*SlotMatch, error) {
	f.gotMatchID = slotID
	return f.matches, f.err
}

func (f *fakeService) SendRequest(ctx context.Context, in *SendRequestInput) (*Request, error) {
	f.gotSend = in
	if f.err != nil {
		return nil, f.err
	}
	return &Request{ID: uuid.New(), ContactID: in.ContactID, ToEmail: "pat@club.example", Status: "sent", CreatedAt: time.Now()}, nil
}

func (f *fakeService) ListRequests(ctx context.Context) ([]*Request, error) { return f.requests, f.err }

func (f *fakeService) GetRequest(ctx context.Context, id uuid.UUID) (*Request, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &Request{ID: id, ContactID: uuid.New(), Status: "queued", CreatedAt: time.Now()}, nil
}

func (f *fakeService) ListSuppressions(ctx context.Context) ([]*Suppression, error) {
	return f.suppressions, f.err
}

func (f *fakeService) AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error) {
	f.gotSupp = in
	if f.err != nil {
		return nil, f.err
	}
	return &Suppression{ID: uuid.New(), Email: in.Email, Reason: in.Reason, CreatedAt: time.Now()}, nil
}

func (f *fakeService) DeleteSuppression(ctx context.Context, id uuid.UUID) error { return f.err }

// ─── Helpers ─────────────────────────────────────────────────────────────────

func doRequest(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
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

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, rec.Body.String())
	}
}

func wantErrorShape(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, status, rec.Body.String())
	}
	var body map[string]string
	decodeBody(t, rec, &body)
	if body["error"] == "" {
		t.Fatalf("error body missing 'error' key: %q", rec.Body.String())
	}
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestHandlerRoutes(t *testing.T) {
	svc := &fakeService{
		slots:        []*Slot{{ID: uuid.New(), City: "Berlin"}},
		templates:    []*Template{{ID: uuid.New(), Name: "t"}},
		matches:      []*SlotMatch{{Slot: Slot{ID: uuid.New(), City: "Berlin"}}},
		requests:     []*Request{{ID: uuid.New()}},
		suppressions: []*Suppression{{ID: uuid.New(), Email: "dj@club.example"}},
	}
	h := NewHandler(svc).Routes()

	t.Run("GET /availability/slots", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodGet, "/availability/slots", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var slots []*Slot
		decodeBody(t, rec, &slots)
		if len(slots) != 1 || slots[0].City != "Berlin" {
			t.Fatalf("slots = %+v", slots)
		}
	})

	t.Run("POST /availability/slots", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodPost, "/availability/slots", `{"date":"2026-11-01","city":"Berlin"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d (body %q)", rec.Code, rec.Body.String())
		}
		if svc.gotSlot == nil || svc.gotSlot.City != "Berlin" {
			t.Fatalf("gotSlot = %+v", svc.gotSlot)
		}
		var slot Slot
		decodeBody(t, rec, &slot)
		if slot.ID == uuid.Nil {
			t.Fatal("response slot has no id")
		}
	})

	t.Run("POST /availability/slots rejects unknown fields", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodPost, "/availability/slots", `{"date":"2026-11-01","city":"Berlin","bogus":1}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("POST /availability/slots rejects malformed JSON", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodPost, "/availability/slots", `{"date":`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("DELETE /availability/slots/{id}", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodDelete, "/availability/slots/"+uuid.NewString(), "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("DELETE /availability/slots/{id} rejects bad id", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodDelete, "/availability/slots/nope", "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("GET /availability/matches without slot_id", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodGet, "/availability/matches", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if svc.gotMatchID != nil {
			t.Fatalf("slotID = %v, want nil", svc.gotMatchID)
		}
		var matches []*SlotMatch
		decodeBody(t, rec, &matches)
		if len(matches) != 1 {
			t.Fatalf("matches = %d", len(matches))
		}
	})

	t.Run("GET /availability/matches?slot_id=", func(t *testing.T) {
		id := uuid.NewString()
		rec := doRequest(t, h, http.MethodGet, "/availability/matches?slot_id="+id, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if svc.gotMatchID == nil || svc.gotMatchID.String() != id {
			t.Fatalf("slotID = %v", svc.gotMatchID)
		}
	})

	t.Run("GET /availability/matches rejects bad slot_id", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodGet, "/availability/matches?slot_id=nope", "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("GET /templates", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodGet, "/templates", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("POST /templates", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodPost, "/templates", `{"name":"t","subject":"s","body":"b"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d", rec.Code)
		}
		if svc.gotTpl == nil || svc.gotTpl.Name != "t" {
			t.Fatalf("gotTpl = %+v", svc.gotTpl)
		}
	})

	t.Run("PUT /templates/{id}", func(t *testing.T) {
		id := uuid.NewString()
		rec := doRequest(t, h, http.MethodPut, "/templates/"+id, `{"version":2,"name":"new"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if svc.gotTplUpd == nil || svc.gotTplUpd.Version != 2 || svc.gotTplUpd.Name == nil {
			t.Fatalf("gotTplUpd = %+v", svc.gotTplUpd)
		}
	})

	t.Run("DELETE /templates/{id}", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodDelete, "/templates/"+uuid.NewString(), "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("POST /requests returns 201 with the row", func(t *testing.T) {
		contactID := uuid.NewString()
		rec := doRequest(t, h, http.MethodPost, "/requests", `{"contact_id":"`+contactID+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d (body %q)", rec.Code, rec.Body.String())
		}
		var row Request
		decodeBody(t, rec, &row)
		if row.ID == uuid.Nil || row.ContactID.String() != contactID {
			t.Fatalf("row = %+v", row)
		}
	})

	t.Run("GET /requests", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodGet, "/requests", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("GET /requests/{id}", func(t *testing.T) {
		id := uuid.NewString()
		rec := doRequest(t, h, http.MethodGet, "/requests/"+id, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var row Request
		decodeBody(t, rec, &row)
		if row.ID.String() != id {
			t.Fatalf("row id = %v", row.ID)
		}
	})

	t.Run("GET /suppressions", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodGet, "/suppressions", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("POST /suppressions", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodPost, "/suppressions", `{"email":"dj@club.example","reason":"asked"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d", rec.Code)
		}
		if svc.gotSupp == nil || svc.gotSupp.Email != "dj@club.example" {
			t.Fatalf("gotSupp = %+v", svc.gotSupp)
		}
	})

	t.Run("DELETE /suppressions/{id}", func(t *testing.T) {
		rec := doRequest(t, h, http.MethodDelete, "/suppressions/"+uuid.NewString(), "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

func TestHandlerErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"validation", ErrValidation, http.StatusUnprocessableEntity},
		{"no contact email", ErrNoContactEmail, http.StatusUnprocessableEntity},
		{"no reply-to", ErrNoReplyTo, http.StatusUnprocessableEntity},
		{"legacy epk", ErrEPKLegacyExport, http.StatusUnprocessableEntity},
		{"not found", ErrNotFound, http.StatusNotFound},
		{"suppressed", ErrSuppressed, http.StatusConflict},
		{"conflict", ErrConflict, http.StatusConflict},
		{"rate limited", ErrRateLimited, http.StatusTooManyRequests},
		{"email unavailable", ErrEmailUnavailable, http.StatusServiceUnavailable},
		{"wrapped validation", errors.Join(errors.New("ctx"), ErrValidation), http.StatusUnprocessableEntity},
		{"internal", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{err: tc.err}
			h := NewHandler(svc).Routes()

			// Exercise the mapping through several routes.
			rec := doRequest(t, h, http.MethodPost, "/requests", `{"contact_id":"`+uuid.NewString()+`"}`)
			wantErrorShape(t, rec, tc.status)

			rec = doRequest(t, h, http.MethodGet, "/requests/"+uuid.NewString(), "")
			wantErrorShape(t, rec, tc.status)

			rec = doRequest(t, h, http.MethodPost, "/availability/slots", `{"date":"2026-11-01","city":"Berlin"}`)
			wantErrorShape(t, rec, tc.status)

			rec = doRequest(t, h, http.MethodGet, "/availability/matches", "")
			wantErrorShape(t, rec, tc.status)

			rec = doRequest(t, h, http.MethodPost, "/suppressions", `{"email":"a@b.co"}`)
			wantErrorShape(t, rec, tc.status)
		})
	}
}

func TestHandlerListErrorsAre500(t *testing.T) {
	svc := &fakeService{err: errors.New("db down")}
	h := NewHandler(svc).Routes()
	for _, target := range []string{"/availability/slots", "/templates", "/requests", "/suppressions"} {
		rec := doRequest(t, h, http.MethodGet, target, "")
		wantErrorShape(t, rec, http.StatusInternalServerError)
	}
}
