package finance

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

// fakeRepo is an in-memory replacement for *Repository used by contract
// tests. It mirrors the optimistic-concurrency semantics of the real
// repository: Update returns ErrConflict when the caller's token does
// not match the stored UpdatedAt.
type fakeRepo struct {
	current *BillingProfile
}

func (f *fakeRepo) Get(ctx context.Context) (*BillingProfile, error) {
	if f.current == nil {
		return nil, errors.New("fakeRepo: not seeded")
	}
	cp := *f.current
	return &cp, nil
}

func (f *fakeRepo) Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error) {
	if !req.UpdatedAt.Equal(f.current.UpdatedAt) {
		return nil, ErrConflict
	}
	cp := *f.current
	cp.LegalName = req.LegalName
	cp.TradingName = req.TradingName
	cp.EntityKind = req.EntityKind
	cp.TaxID = req.TaxID
	cp.TaxIDKind = req.TaxIDKind
	cp.ContactEmail = req.ContactEmail
	cp.ContactPhone = req.ContactPhone
	cp.AddressLine1 = req.AddressLine1
	cp.AddressLine2 = req.AddressLine2
	cp.AddressCity = req.AddressCity
	cp.AddressRegion = req.AddressRegion
	cp.AddressPostal = req.AddressPostal
	cp.AddressCountry = req.AddressCountry
	cp.Jurisdiction = req.Jurisdiction
	cp.PaymentInstructions = req.PaymentInstructions
	cp.DefaultCurrency = req.DefaultCurrency
	cp.VATExemptSmallBusiness = req.VATExemptSmallBusiness
	cp.DefaultVATRateBps = req.DefaultVATRateBps
	// Advance from the token value passed in, so the test assertion against
	// repo.current.UpdatedAt passes.
	cp.UpdatedAt = req.UpdatedAt.Add(time.Millisecond)
	f.current = &cp
	return &cp, nil
}

func newSeededRepo() *fakeRepo {
	return &fakeRepo{
		current: &BillingProfile{
			ID:              uuid.New(),
			LegalName:       "",
			EntityKind:      EntityKindIndividual,
			TaxIDKind:       TaxIDKindEmpty,
			DefaultCurrency: "EUR",
			UpdatedAt:       time.Now().UTC(),
			CreatedAt:       time.Now().UTC(),
		},
	}
}

func validRequest(token time.Time) UpdateBillingProfileRequest {
	return UpdateBillingProfileRequest{
		LegalName:       "Lina Vasquez",
		EntityKind:      EntityKindSoleTrader,
		ContactEmail:    "billing@example.com",
		AddressLine1:    "1 Sample Street",
		AddressCity:     "Berlin",
		AddressPostal:   "10115",
		Jurisdiction:    "DE",
		DefaultCurrency: "EUR",
		UpdatedAt:       token,
	}
}

// TestHandler_GetReturnsEnvelope asserts the wire shape.
func TestHandler_GetReturnsEnvelope(t *testing.T) {
	repo := newSeededRepo()
	h := NewHandler(NewService(repo))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data BillingProfile `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Data.ID != repo.current.ID {
		t.Errorf("envelope data.id mismatch: got %v want %v", env.Data.ID, repo.current.ID)
	}
}

// TestHandler_PutEnvelopeSuccess asserts the PUT response also wraps
// the result, the response includes a fresh updated_at, and that the
// concurrency token advances.
func TestHandler_PutEnvelopeSuccess(t *testing.T) {
	repo := newSeededRepo()
	h := NewHandler(NewService(repo))

	baseline := repo.current.UpdatedAt
	body, _ := json.Marshal(validRequest(baseline))
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data BillingProfile `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Data.LegalName != "Lina Vasquez" {
		t.Errorf("legal_name: %q", env.Data.LegalName)
	}
	// The response's updated_at must advance from the token we sent.
	if !env.Data.UpdatedAt.After(baseline) {
		t.Errorf("updated_at did not advance: was %v now %v", baseline, env.Data.UpdatedAt)
	}
	if !env.Data.EntityKind.IsValid() {
		t.Errorf("entity_kind invalid: %q", env.Data.EntityKind)
	}
}

// TestHandler_PutValidationFailure asserts a 400 + validation envelope on
// a malformed PUT.
func TestHandler_PutValidationFailure(t *testing.T) {
	repo := newSeededRepo()
	h := NewHandler(NewService(repo))

	body := `{"legal_name":"","contact_email":"not-an-email","address_line1":"","address_city":"","address_postal":"","jurisdiction":"","default_currency":"EU","entity_kind":"alien","updated_at":"2026-01-01T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "validation_failed") {
		t.Errorf("expected validation_failed error code, got %s", rec.Body.String())
	}
	for _, field := range []string{"legal_name", "contact_email", "address_line1", "address_city", "address_postal", "jurisdiction", "default_currency", "entity_kind"} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("validation message missing field %q: %s", field, rec.Body.String())
		}
	}
}

// TestHandler_PutConcurrencyConflict asserts that a PUT with a stale
// token answers 409 and the stored profile is unchanged.
func TestHandler_PutConcurrencyConflict(t *testing.T) {
	repo := newSeededRepo()
	h := NewHandler(NewService(repo))

	staleToken := repo.current.UpdatedAt.Add(-time.Hour)
	body, _ := json.Marshal(validRequest(staleToken))
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.current.LegalName != "" {
		t.Errorf("stale PUT mutated legal_name to %q", repo.current.LegalName)
	}
}

// TestHandler_PutMissingUpdatedAt returns 400.
func TestHandler_PutMissingUpdatedAt(t *testing.T) {
	repo := newSeededRepo()
	h := NewHandler(NewService(repo))

	body := `{"legal_name":"X","contact_email":"x@y.com","address_line1":"a","address_city":"b","address_postal":"1","jurisdiction":"DE","default_currency":"EUR","entity_kind":"individual"}`
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "updated_at") {
		t.Errorf("expected updated_at in error: %s", rec.Body.String())
	}
}

// TestService_CanIssueAfterFill confirms the readiness gate flips once
// every RequiredFieldForIssue field is set.
func TestService_CanIssueAfterFill(t *testing.T) {
	repo := newSeededRepo()
	svc := NewService(repo)

	ok, missing, err := svc.CanIssue(context.Background())
	if err != nil {
		t.Fatalf("CanIssue: %v", err)
	}
	if ok {
		t.Fatalf("expected CanIssue=false on empty profile")
	}
	if len(missing) == 0 {
		t.Fatalf("expected missing fields on empty profile")
	}

	body := validRequest(repo.current.UpdatedAt)
	if _, err := svc.Update(context.Background(), &body); err != nil {
		t.Fatalf("Update: %v", err)
	}

	ok, missing, err = svc.CanIssue(context.Background())
	if err != nil {
		t.Fatalf("CanIssue after fill: %v", err)
	}
	if !ok || len(missing) != 0 {
		t.Fatalf("expected ready, got ok=%v missing=%v", ok, missing)
	}
}

// TestValidation_RejectLowercaseCurrency ensures ISO 4217 codes are
// uppercase; lowercase is rejected so we never silently sum currencies.
func TestValidation_RejectLowercaseCurrency(t *testing.T) {
	r := validRequest(time.Now())
	r.DefaultCurrency = "eur"
	if errs := Validate(&r); len(errs) == 0 {
		t.Fatalf("expected validation error for lowercase currency")
	}
}

// TestValidation_RejectBadEntityKind ensures the closed set is enforced.
func TestValidation_RejectBadEntityKind(t *testing.T) {
	r := validRequest(time.Now())
	r.EntityKind = EntityKind("martian")
	if errs := Validate(&r); len(errs) == 0 {
		t.Fatalf("expected validation error for unknown entity kind")
	}
}
