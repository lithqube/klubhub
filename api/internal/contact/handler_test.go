package contact

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

// fakeService validates like Service but stores in memory.
type fakeService struct {
	contacts map[uuid.UUID]*Contact
}

func (f *fakeService) CreateContact(_ context.Context, req *ContactCreate) (*Contact, error) {
	if err := NormalizeCreate(req); err != nil {
		return nil, err
	}
	c := &Contact{ID: uuid.New(), Name: req.Name, Company: req.Company, Email: req.Email, Phone: req.Phone,
		Type: req.Type, Notes: req.Notes, PartyFields: req.PartyFields, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.contacts[c.ID] = c
	return c, nil
}
func (f *fakeService) GetContact(_ context.Context, id uuid.UUID) (*Contact, error) {
	if c, ok := f.contacts[id]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}
func (f *fakeService) ListContacts(context.Context, *string, *ContactType) ([]*Contact, error) {
	return nil, nil
}
func (f *fakeService) UpdateContact(_ context.Context, id uuid.UUID, req *ContactUpdate) (*Contact, error) {
	if err := NormalizeUpdate(req); err != nil {
		return nil, err
	}
	c, ok := f.contacts[id]
	if !ok {
		return nil, ErrNotFound
	}
	if req.Country != nil {
		c.Country = *req.Country
	}
	if req.VATID != nil {
		c.VATID = *req.VATID
	}
	if req.IsBusiness != nil {
		c.IsBusiness = *req.IsBusiness
	}
	return c, nil
}
func (f *fakeService) DeleteContact(context.Context, uuid.UUID) error { return nil }
func (f *fakeService) Autocomplete(context.Context, string, int) ([]*Contact, error) {
	return nil, nil
}

func TestHandler_CreateWithPartyFields(t *testing.T) {
	h := NewHandler(&fakeService{contacts: map[uuid.UUID]*Contact{}}).Routes()
	body := `{"name":"Club GmbH","type":"promoter","address_line1":"Köpenicker Str. 70","city":"Berlin",
		"postal_code":"10179","country":" de ","vat_id":"de 123 456 789","is_business":true}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	for field, want := range map[string]any{
		"country": "DE", "vat_id": "DE123456789", "is_business": true, "city": "Berlin",
		"address_line2": "", "region": "", "tax_id": "",
	} {
		if got[field] != want {
			t.Errorf("%s = %v, want %v", field, got[field], want)
		}
	}
}

func TestHandler_RejectsInvalidPartyFields(t *testing.T) {
	svc := &fakeService{contacts: map[uuid.UUID]*Contact{}}
	h := NewHandler(svc).Routes()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"X","country":"Germany"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "country") {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	c, _ := svc.CreateContact(context.Background(), &ContactCreate{Name: "Y"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/"+c.ID.String(),
		strings.NewReader(`{"postal_code":"`+strings.Repeat("9", 21)+`"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "postal_code") {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
}

func TestNormalize(t *testing.T) {
	c := ContactCreate{Name: "  ", Type: "alien"}
	err := NormalizeCreate(&c)
	var ve *ValidationError
	if !errors.As(err, &ve) || !errors.Is(err, ErrValidation) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if ve.Fields["name"] == "" || ve.Fields["type"] == "" {
		t.Fatalf("fields = %v", ve.Fields)
	}

	ok := ContactCreate{Name: "Promoter"}
	if err := NormalizeCreate(&ok); err != nil || ok.Type != ContactTypeOther {
		t.Fatalf("defaults: type=%q err=%v", ok.Type, err)
	}

	country, vat := "fr", " fr.123 456 "
	u := ContactUpdate{Country: &country, VATID: &vat}
	if err := NormalizeUpdate(&u); err != nil || *u.Country != "FR" || *u.VATID != "FR123456" {
		t.Fatalf("update normalize: %v %q %q", err, *u.Country, *u.VATID)
	}
}
