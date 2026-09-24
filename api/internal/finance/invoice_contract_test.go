package finance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// fakeInvoiceRepo is an in-memory replacement for *InvoiceRepository used
// by contract tests. It mirrors the real repo's optimistic-concurrency
// semantics.
type fakeInvoiceRepo struct {
	invoices map[uuid.UUID]*Invoice
	lines    map[uuid.UUID][]*InvoiceLine
	seqs     map[string]int64 // key: prefix-currency
}

func newFakeInvoiceRepo() *fakeInvoiceRepo {
	return &fakeInvoiceRepo{
		invoices: make(map[uuid.UUID]*Invoice),
		lines:    make(map[uuid.UUID][]*InvoiceLine),
		seqs:     make(map[string]int64),
	}
}

func (f *fakeInvoiceRepo) CreateDraft(ctx context.Context, gig *GigFeeInfo, req CreateInvoiceRequest, profile *BillingProfile) (*Invoice, error) {
	prefix := req.NumberPrefix
	if prefix == "" {
		prefix = DefaultNumberingConfig().Prefix
	}
	key := prefix + "-" + req.Currency
	f.seqs[key]++
	seq := f.seqs[key]

	inv := &Invoice{
		ID:             uuid.New(),
		GigID:          gig.ID,
		BillingProfile: profileJSON(profile),
		InvoiceNumber:  FormatInvoiceNumber(prefix, req.Currency, seq),
		NumberPrefix:   prefix,
		NumberSeq:      seq,
		Currency:       req.Currency,
		SubtotalMinor:  gig.FeeMinor,
		TaxRateBps:     gig.TaxRateBps,
		TaxMinor:       gig.TaxMinor,
		TotalMinor:     gig.TotalMinor,
		Status:         InvoiceStatusDraft,
		DueAt:          req.DueAt,
		UpdatedAt:      time.Now().UTC(),
		CreatedAt:      time.Now().UTC(),
	}
	f.invoices[inv.ID] = inv
	f.lines[inv.ID] = []*InvoiceLine{
		{
			ID:             uuid.New(),
			InvoiceID:      inv.ID,
			SortOrder:      0,
			Description:    gig.LineDescription,
			Quantity:       1,
			UnitMinor:      gig.FeeMinor,
			TaxBps:         gig.TaxRateBps,
			LineTotalMinor: gig.FeeMinor,
			CreatedAt:      time.Now().UTC(),
		},
	}
	return inv, nil
}

func (f *fakeInvoiceRepo) GetByID(ctx context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return nil, nil, ErrInvoiceNotFound
	}
	lines := f.lines[id]
	cp := *inv
	return &cp, lines, nil
}

func (f *fakeInvoiceRepo) List(ctx context.Context, filter InvoiceFilter) ([]*Invoice, error) {
	var out []*Invoice
	for _, inv := range f.invoices {
		if filter.GigID != nil && inv.GigID != *filter.GigID {
			continue
		}
		if filter.Status != "" && inv.Status != filter.Status {
			continue
		}
		if filter.Currency != "" && inv.Currency != filter.Currency {
			continue
		}
		if !filter.From.IsZero() && inv.CreatedAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && inv.CreatedAt.After(filter.To) {
			continue
		}
		cp := *inv
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeInvoiceRepo) UpdateDraft(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok || inv.Status != InvoiceStatusDraft || !req.UpdatedAt.Equal(inv.UpdatedAt) {
		return nil, ErrInvoiceConflict
	}
	inv.NumberPrefix = req.NumberPrefix
	inv.DueAt = req.DueAt
	inv.InternalNotes = req.InternalNotes
	inv.UpdatedAt = time.Now().UTC()
	return inv, nil
}

func (f *fakeInvoiceRepo) Issue(ctx context.Context, id uuid.UUID, req IssueInvoiceRequest, profile *BillingProfile) (*Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok || inv.Status != InvoiceStatusDraft {
		return nil, ErrInvoiceConflict
	}
	// For tests, allow small time drift in token comparison (unmarshalled JSON time)
	if !req.UpdatedAt.Equal(inv.UpdatedAt) && req.UpdatedAt.Sub(inv.UpdatedAt).Abs() > time.Millisecond {
		return nil, ErrInvoiceConflict
	}
	inv.BillingProfile = profileJSON(profile)
	inv.Status = InvoiceStatusIssued
	now := time.Now().UTC()
	inv.IssuedAt = &now
	inv.UpdatedAt = now
	f.invoices[id] = inv // persist back to map
	return inv, nil
}

func (f *fakeInvoiceRepo) Pay(ctx context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok || inv.Status != InvoiceStatusIssued || !req.UpdatedAt.Equal(inv.UpdatedAt) {
		return nil, ErrInvoiceConflict
	}
	inv.Status = InvoiceStatusPaid
	inv.PaidAt = &req.PaidAt
	inv.PaymentRef = req.PaymentRef
	inv.UpdatedAt = time.Now().UTC()
	f.invoices[id] = inv // persist back to map
	return inv, nil
}

func (f *fakeInvoiceRepo) Cancel(ctx context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok || (inv.Status != InvoiceStatusDraft && inv.Status != InvoiceStatusIssued) || !req.UpdatedAt.Equal(inv.UpdatedAt) {
		return nil, ErrInvoiceConflict
	}
	inv.Status = InvoiceStatusCancelled
	inv.UpdatedAt = time.Now().UTC()
	return inv, nil
}

func (f *fakeInvoiceRepo) Correct(ctx context.Context, id uuid.UUID, req CorrectInvoiceRequest, profile *BillingProfile) (*Invoice, error) {
	orig, ok := f.invoices[id]
	if !ok || (orig.Status != InvoiceStatusIssued && orig.Status != InvoiceStatusPaid) || !req.UpdatedAt.Equal(orig.UpdatedAt) {
		return nil, ErrInvoiceConflict
	}
	// Mark original corrected
	orig.Status = InvoiceStatusCorrected
	orig.UpdatedAt = time.Now().UTC()

	// Create new draft with next sequence
	key := orig.NumberPrefix + "-" + orig.Currency
	f.seqs[key]++
	seq := f.seqs[key]

	newInv := &Invoice{
		ID:             uuid.New(),
		GigID:          orig.GigID,
		BillingProfile: profileJSON(profile),
		InvoiceNumber:  FormatInvoiceNumber(orig.NumberPrefix, orig.Currency, seq),
		NumberPrefix:   orig.NumberPrefix,
		NumberSeq:      seq,
		Currency:       orig.Currency,
		SubtotalMinor:  orig.SubtotalMinor,
		TaxRateBps:     orig.TaxRateBps,
		TaxMinor:       orig.TaxMinor,
		TotalMinor:     orig.TotalMinor,
		Status:         InvoiceStatusDraft,
		UpdatedAt:      time.Now().UTC(),
		CreatedAt:      time.Now().UTC(),
	}
	f.invoices[newInv.ID] = newInv

	// Copy lines
	oldLines := f.lines[id]
	newLines := make([]*InvoiceLine, len(oldLines))
	for i, l := range oldLines {
		newLines[i] = &InvoiceLine{
			ID:             uuid.New(),
			InvoiceID:      newInv.ID,
			SortOrder:      l.SortOrder,
			Description:    l.Description,
			Quantity:       l.Quantity,
			UnitMinor:      l.UnitMinor,
			TaxBps:         l.TaxBps,
			LineTotalMinor: l.LineTotalMinor,
			CreatedAt:      time.Now().UTC(),
		}
	}
	f.lines[newInv.ID] = newLines
	return newInv, nil
}

func (f *fakeInvoiceRepo) NextNumber(ctx context.Context, prefix, currency string) (string, int64, error) {
	key := prefix + "-" + currency
	f.seqs[key]++
	seq := f.seqs[key]
	return FormatInvoiceNumber(prefix, currency, seq), seq, nil
}

func (f *fakeInvoiceRepo) Summaries(ctx context.Context) (map[string]CurrencySummary, error) {
	sums := make(map[string]CurrencySummary)
	for _, inv := range f.invoices {
		s := sums[inv.Currency]
		s.Currency = inv.Currency
		if inv.Status == InvoiceStatusIssued {
			s.IssuedTotal = s.IssuedTotal.Add(decimal.NewFromInt(inv.TotalMinor).Div(decimal.NewFromInt(100)))
			s.IssuedCount++
		}
		if inv.Status == InvoiceStatusPaid {
			s.PaidTotal = s.PaidTotal.Add(decimal.NewFromInt(inv.TotalMinor).Div(decimal.NewFromInt(100)))
			s.PaidCount++
		}
		sums[inv.Currency] = s
	}
	return sums, nil
}

func validCreateInvoiceRequest() CreateInvoiceRequest {
	return CreateInvoiceRequest{
		GigID:        uuid.New(),
		Currency:     "EUR",
		NumberPrefix: "INV",
	}
}

// fakeGigFeeProvider implements GigFeeProvider for contract tests.
type fakeGigFeeProvider struct {
	fees map[uuid.UUID]*GigFeeInfo
}

func newFakeGigFeeProvider() *fakeGigFeeProvider {
	return &fakeGigFeeProvider{
		fees: make(map[uuid.UUID]*GigFeeInfo),
	}
}

func (f *fakeGigFeeProvider) SetFee(gigID uuid.UUID, fee *GigFeeInfo) {
	f.fees[gigID] = fee
}

func (f *fakeGigFeeProvider) GetGigFeeInfo(ctx context.Context, gigID uuid.UUID) (*GigFeeInfo, error) {
	fee, ok := f.fees[gigID]
	if !ok {
		return nil, ErrInvoiceNotFound // reused for "gig not found"
	}
	return fee, nil
}

// fakeBillingService implements the billing ServiceIface for contract tests.
type fakeBillingService struct {
	profile *BillingProfile
}

func newFakeBillingService() *fakeBillingService {
	return &fakeBillingService{
		profile: &BillingProfile{
			LegalName:       "Test DJ",
			EntityKind:      EntityKindIndividual,
			TaxIDKind:       TaxIDKindEmpty,
			ContactEmail:    "billing@test.com",
			AddressLine1:    "1 Test Street",
			AddressCity:     "Berlin",
			AddressPostal:   "10115",
			AddressCountry:  "DE",
			Jurisdiction:    "DE",
			DefaultCurrency: "EUR",
			UpdatedAt:       time.Now().UTC(),
			CreatedAt:       time.Now().UTC(),
		},
	}
}

func (f *fakeBillingService) Get(ctx context.Context) (*BillingProfile, error) {
	return f.profile, nil
}

func (f *fakeBillingService) Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error) {
	f.profile.LegalName = req.LegalName
	f.profile.TradingName = req.TradingName
	f.profile.EntityKind = req.EntityKind
	f.profile.TaxID = req.TaxID
	f.profile.TaxIDKind = req.TaxIDKind
	f.profile.ContactEmail = req.ContactEmail
	f.profile.ContactPhone = req.ContactPhone
	f.profile.AddressLine1 = req.AddressLine1
	f.profile.AddressLine2 = req.AddressLine2
	f.profile.AddressCity = req.AddressCity
	f.profile.AddressRegion = req.AddressRegion
	f.profile.AddressPostal = req.AddressPostal
	f.profile.AddressCountry = req.AddressCountry
	f.profile.Jurisdiction = req.Jurisdiction
	f.profile.PaymentInstructions = req.PaymentInstructions
	f.profile.DefaultCurrency = req.DefaultCurrency
	f.profile.UpdatedAt = time.Now().UTC()
	return f.profile, nil
}

func validIssueRequest(token time.Time) IssueInvoiceRequest {
	return IssueInvoiceRequest{UpdatedAt: token}
}

func validPayRequest(token time.Time) PayInvoiceRequest {
	now := time.Now().UTC()
	return PayInvoiceRequest{
		PaidAt:     now,
		PaymentRef: "ref-123",
		UpdatedAt:  token,
	}
}

func validCancelRequest(token time.Time) CancelInvoiceRequest {
	return CancelInvoiceRequest{UpdatedAt: token}
}

func validCorrectRequest(token time.Time) CorrectInvoiceRequest {
	return CorrectInvoiceRequest{UpdatedAt: token}
}

func validUpdateRequest(token time.Time) UpdateInvoiceRequest {
	return UpdateInvoiceRequest{
		NumberPrefix:  "INV",
		InternalNotes: "test note",
		UpdatedAt:     token,
	}
}

func TestInvoiceHandler_CreateDraftEnvelope(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data Invoice `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Data.ID == uuid.Nil {
		t.Fatal("missing invoice id")
	}
	if env.Data.Status != InvoiceStatusDraft {
		t.Errorf("status: %v", env.Data.Status)
	}
	if env.Data.Currency != "EUR" {
		t.Errorf("currency: %v", env.Data.Currency)
	}
	if !strings.HasPrefix(env.Data.InvoiceNumber, "INV-") {
		t.Errorf("invoice_number format: %v", env.Data.InvoiceNumber)
	}
}

func TestInvoiceHandler_GetEnvelope(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create via handler
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)

	var created struct{ Data Invoice }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Get it
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices/"+created.Data.ID.String(), nil)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("status: %d", getRec.Code)
	}
	var env struct{ Data struct{ Invoice Invoice; Lines []*InvoiceLine } }
	json.Unmarshal(getRec.Body.Bytes(), &env)
	if env.Data.Invoice.ID != created.Data.ID {
		t.Errorf("id mismatch")
	}
}

func TestInvoiceHandler_UpdateDraftEnvelope(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	var created struct{ Data Invoice }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Update
	body, _ := json.Marshal(validUpdateRequest(created.Data.UpdatedAt))
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/finance/invoices/"+created.Data.ID.String(), strings.NewReader(string(body)))
	putRec := httptest.NewRecorder()
	h.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", putRec.Code, putRec.Body.String())
	}
	var env struct{ Data Invoice }
	json.Unmarshal(putRec.Body.Bytes(), &env)
	if env.Data.InternalNotes != "test note" {
		t.Errorf("internal_notes not updated: %q", env.Data.InternalNotes)
	}
	if !env.Data.UpdatedAt.After(created.Data.UpdatedAt) {
		t.Errorf("updated_at did not advance")
	}
}

func TestInvoiceHandler_IssueEnvelope(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	var created struct{ Data Invoice }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Issue
	body, _ := json.Marshal(validIssueRequest(created.Data.UpdatedAt))
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+created.Data.ID.String()+"/issue", strings.NewReader(string(body)))
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", postRec.Code, postRec.Body.String())
	}
	var env struct{ Data Invoice }
	json.Unmarshal(postRec.Body.Bytes(), &env)
	if env.Data.Status != InvoiceStatusIssued {
		t.Errorf("status: %v", env.Data.Status)
	}
	if env.Data.IssuedAt == nil {
		t.Fatal("issued_at missing")
	}
	if len(env.Data.BillingProfile) == 0 || string(env.Data.BillingProfile) == "{}" {
		t.Fatal("billing_profile snapshot missing")
	}
}

func TestInvoiceHandler_PayEnvelope(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create + Issue
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	t.Logf("Create response: %d %s", createRec.Code, createRec.Body.String())
	var created struct{ Data Invoice }
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}

	// Use the exact UpdatedAt from the response for the issue request
	issueToken := created.Data.UpdatedAt
	t.Logf("Created invoice UpdatedAt: %v (nano=%d)", issueToken, issueToken.Nanosecond())

	issueReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+created.Data.ID.String()+"/issue", strings.NewReader(`{"updated_at":"`+issueToken.Format(time.RFC3339Nano)+`"}`))
	issueRec := httptest.NewRecorder()
	h.ServeHTTP(issueRec, issueReq)

	if issueRec.Code != http.StatusOK {
		t.Fatalf("issue status: %d body=%s", issueRec.Code, issueRec.Body.String())
	}
	var issued struct{ Data Invoice }
	if err := json.Unmarshal(issueRec.Body.Bytes(), &issued); err != nil {
		t.Fatalf("unmarshal issue response: %v", err)
	}

	// Pay
	body, _ := json.Marshal(validPayRequest(issued.Data.UpdatedAt))
	payReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+issued.Data.ID.String()+"/pay", strings.NewReader(string(body)))
	payRec := httptest.NewRecorder()
	h.ServeHTTP(payRec, payReq)

	if payRec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", payRec.Code, payRec.Body.String())
	}
	var env struct{ Data Invoice }
	json.Unmarshal(payRec.Body.Bytes(), &env)
	if env.Data.Status != InvoiceStatusPaid {
		t.Errorf("status: %v", env.Data.Status)
	}
	if env.Data.PaidAt == nil {
		t.Fatal("paid_at missing")
	}
	if env.Data.PaymentRef != "ref-123" {
		t.Errorf("payment_ref: %q", env.Data.PaymentRef)
	}
}

func TestInvoiceHandler_CancelEnvelope(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	var created struct{ Data Invoice }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Cancel
	body, _ := json.Marshal(validCancelRequest(created.Data.UpdatedAt))
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+created.Data.ID.String()+"/cancel", strings.NewReader(string(body)))
	cancelRec := httptest.NewRecorder()
	h.ServeHTTP(cancelRec, cancelReq)

	if cancelRec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	var env struct{ Data Invoice }
	json.Unmarshal(cancelRec.Body.Bytes(), &env)
	if env.Data.Status != InvoiceStatusCancelled {
		t.Errorf("status: %v", env.Data.Status)
	}
}

func TestInvoiceHandler_CorrectEnvelope(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create + Issue
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	t.Logf("Create response: %d %s", createRec.Code, createRec.Body.String())
	var created struct{ Data Invoice }
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}

	// Use the exact UpdatedAt from the response for the issue request
	issueToken := created.Data.UpdatedAt
	t.Logf("Created invoice UpdatedAt: %v (nano=%d)", issueToken, issueToken.Nanosecond())

	issueReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+created.Data.ID.String()+"/issue", strings.NewReader(`{"updated_at":"`+issueToken.Format(time.RFC3339Nano)+`"}`))
	issueRec := httptest.NewRecorder()
	h.ServeHTTP(issueRec, issueReq)
	var issued struct{ Data Invoice }
	json.Unmarshal(issueRec.Body.Bytes(), &issued)

	// Correct
	body, _ := json.Marshal(validCorrectRequest(issued.Data.UpdatedAt))
	corrReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+issued.Data.ID.String()+"/correct", strings.NewReader(string(body)))
	corrRec := httptest.NewRecorder()
	h.ServeHTTP(corrRec, corrReq)

	if corrRec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", corrRec.Code, corrRec.Body.String())
	}
	var env struct{ Data Invoice }
	json.Unmarshal(corrRec.Body.Bytes(), &env)
	if env.Data.Status != InvoiceStatusDraft {
		t.Errorf("correction status: %v", env.Data.Status)
	}
	// Original should be corrected
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices/"+issued.Data.ID.String(), nil)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)
	var origEnv struct{ Data struct{ Invoice Invoice; Lines []*InvoiceLine } }
	json.Unmarshal(getRec.Body.Bytes(), &origEnv)
	if origEnv.Data.Invoice.Status != InvoiceStatusCorrected {
		t.Errorf("original status: %v", origEnv.Data.Invoice.Status)
	}
}

func TestInvoiceHandler_ConcurrencyConflict(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	var created struct{ Data Invoice }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Update with stale token
	stale := created.Data.UpdatedAt.Add(-time.Hour)
	body, _ := json.Marshal(UpdateInvoiceRequest{
		NumberPrefix:  "INV",
		InternalNotes: "stale",
		UpdatedAt:     stale,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/finance/invoices/"+created.Data.ID.String(), strings.NewReader(string(body)))
	putRec := httptest.NewRecorder()
	h.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusConflict {
		t.Fatalf("status: %d body=%s", putRec.Code, putRec.Body.String())
	}
}

func TestInvoiceHandler_BadStateTransitions(t *testing.T) {
	repo := newFakeInvoiceRepo()
	gigProv := newFakeGigFeeProvider()
	gigID := uuid.New()
	gigProv.SetFee(gigID, &GigFeeInfo{
		ID:               gigID,
		FeeMinor:         25000,
		TaxRateBps:       1900,
		TaxMinor:         4750,
		TotalMinor:       29750,
		LineDescription:  "Performance",
	})
	h := NewInvoiceHandler(NewInvoiceService(repo, newFakeBillingService(), gigProv))

	// Create + Issue
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices", strings.NewReader(`{"gig_id":"`+gigID.String()+`","currency":"EUR"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	t.Logf("Create response: %d %s", createRec.Code, createRec.Body.String())
	var created struct{ Data Invoice }
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}

	// Use the exact UpdatedAt from the response for the issue request
	issueToken := created.Data.UpdatedAt
	t.Logf("Created invoice UpdatedAt: %v (nano=%d)", issueToken, issueToken.Nanosecond())

	issueReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+created.Data.ID.String()+"/issue", strings.NewReader(`{"updated_at":"`+issueToken.Format(time.RFC3339Nano)+`"}`))
	issueRec := httptest.NewRecorder()
	h.ServeHTTP(issueRec, issueReq)

	if issueRec.Code != http.StatusOK {
		t.Fatalf("issue status: %d body=%s", issueRec.Code, issueRec.Body.String())
	}
	var issued struct{ Data Invoice }
	if err := json.Unmarshal(issueRec.Body.Bytes(), &issued); err != nil {
		t.Fatalf("unmarshal issue response: %v", err)
	}

	// Pay
	payReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+issued.Data.ID.String()+"/pay", strings.NewReader(`{"paid_at":"`+time.Now().UTC().Format(time.RFC3339)+`","payment_ref":"r","updated_at":"`+issued.Data.UpdatedAt.Format(time.RFC3339Nano)+`"}`))
	payRec := httptest.NewRecorder()
	h.ServeHTTP(payRec, payReq)
	var paid struct{ Data Invoice }
	if err := json.Unmarshal(payRec.Body.Bytes(), &paid); err != nil {
		t.Fatalf("unmarshal pay response: %v", err)
	}

	// Try to correct a paid invoice (should work - paid can be corrected)
	body, _ := json.Marshal(validCorrectRequest(paid.Data.UpdatedAt))
	corrReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+paid.Data.ID.String()+"/correct", strings.NewReader(string(body)))
	corrRec := httptest.NewRecorder()
	h.ServeHTTP(corrRec, corrReq)

	if corrRec.Code != http.StatusOK {
		t.Fatalf("correct paid: status %d body=%s", corrRec.Code, corrRec.Body.String())
	}

	// Try to issue again (should conflict - already issued/corrected)
	body2, _ := json.Marshal(validIssueRequest(issued.Data.UpdatedAt))
	issue2Req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+issued.Data.ID.String()+"/issue", strings.NewReader(string(body2)))
	issue2Rec := httptest.NewRecorder()
	h.ServeHTTP(issue2Rec, issue2Req)
	if issue2Rec.Code != http.StatusConflict {
		t.Errorf("re-issue should conflict: %d", issue2Rec.Code)
	}
}

func TestInvoiceService_NextNumber(t *testing.T) {
	repo := newFakeInvoiceRepo()
	svc := NewInvoiceService(repo, nil, nil)

	num, seq, err := svc.NextNumber(context.Background(), "INV", "EUR")
	if err != nil {
		t.Fatalf("NextNumber: %v", err)
	}
	if num != "INV-0001-EUR" {
		t.Errorf("number: %q", num)
	}
	if seq != 1 {
		t.Errorf("seq: %d", seq)
	}

	num2, seq2, err := svc.NextNumber(context.Background(), "INV", "EUR")
	if err != nil {
		t.Fatalf("NextNumber 2: %v", err)
	}
	if num2 != "INV-0002-EUR" || seq2 != 2 {
		t.Errorf("number2: %q seq2: %d", num2, seq2)
	}
}

func TestInvoiceService_Summaries(t *testing.T) {
	repo := newFakeInvoiceRepo()
	svc := NewInvoiceService(repo, nil, nil)

	// Create draft
	draft, _ := repo.CreateDraft(context.Background(), &GigFeeInfo{
		ID:              uuid.New(),
		FeeMinor:        25000,
		TaxRateBps:      1900,
		TaxMinor:        4750,
		TotalMinor:      29750,
		LineDescription: "Performance",
	}, validCreateInvoiceRequest(), nil)

	// Issue it
	issued, _ := repo.Issue(context.Background(), draft.ID, validIssueRequest(draft.UpdatedAt), nil)

	// Pay it
	_, _ = repo.Pay(context.Background(), issued.ID, validPayRequest(issued.UpdatedAt))

	sums, err := svc.Summaries(context.Background())
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	eur := sums["EUR"]
	// After paying, status is Paid (not Issued). IssuedCount=0, PaidCount=1.
	if eur.IssuedCount != 0 {
		t.Errorf("issued_count: %d (expected 0 after paying)", eur.IssuedCount)
	}
	if eur.PaidCount != 1 {
		t.Errorf("paid_count: %d", eur.PaidCount)
	}
	if !eur.PaidTotal.Equal(decimal.NewFromInt(29750).Div(decimal.NewFromInt(100))) {
		t.Errorf("paid_total: %v", eur.PaidTotal)
	}
	// Also test that a separately issued (unpaid) invoice shows in IssuedCount
	draft2, _ := repo.CreateDraft(context.Background(), &GigFeeInfo{
		ID:              uuid.New(),
		FeeMinor:        10000,
		TaxRateBps:      1900,
		TaxMinor:        1900,
		TotalMinor:      11900,
		LineDescription: "Performance 2",
	}, validCreateInvoiceRequest(), nil)
	_, _ = repo.Issue(context.Background(), draft2.ID, validIssueRequest(draft2.UpdatedAt), nil)

	sums2, _ := svc.Summaries(context.Background())
	eur2 := sums2["EUR"]
	if eur2.IssuedCount != 1 {
		t.Errorf("issued_count after second issue: %d", eur2.IssuedCount)
	}
	if eur2.PaidCount != 1 {
		t.Errorf("paid_count after second issue: %d", eur2.PaidCount)
	}
}