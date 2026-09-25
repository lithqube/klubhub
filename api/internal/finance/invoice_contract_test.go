package finance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/finance/tax"
)

// fakeInvoiceRepo is an in-memory replacement for *InvoiceRepository used
// by contract tests. It mirrors the real repo's semantics: numbers are
// allocated at issue, stale tokens are conflicts, wrong states are
// bad_state, credit notes are issued in the CN series.
type fakeInvoiceRepo struct {
	mu       sync.Mutex
	invoices map[uuid.UUID]*Invoice
	lines    map[uuid.UUID][]*InvoiceLine
	clock    time.Time
	// gigCurrency stands in for the gigs table read inside Issue.
	gigCurrency map[uuid.UUID]string
}

func newFakeInvoiceRepo() *fakeInvoiceRepo {
	return &fakeInvoiceRepo{
		invoices:    make(map[uuid.UUID]*Invoice),
		lines:       make(map[uuid.UUID][]*InvoiceLine),
		clock:       time.Now().UTC().Truncate(time.Microsecond),
		gigCurrency: make(map[uuid.UUID]string),
	}
}

// tick returns a strictly increasing timestamp so tokens always change.
func (f *fakeInvoiceRepo) tick() time.Time {
	f.clock = f.clock.Add(time.Millisecond)
	return f.clock
}

func (f *fakeInvoiceRepo) snapshot(inv *Invoice) *Invoice {
	cp := *inv
	cp.finalizeBalances()
	return &cp
}

func (f *fakeInvoiceRepo) CreateDraft(_ context.Context, in DraftInput) (*Invoice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.tick()
	t := in.Totals
	inv := &Invoice{
		ID: uuid.New(), Kind: InvoiceKindInvoice, GigID: in.GigID, NumberPrefix: in.NumberPrefix,
		Currency: in.Currency, Status: InvoiceStatusDraft, SupplyDate: in.SupplyDate, DueAt: in.DueAt,
		Customer: in.Customer, BillingProfile: json.RawMessage(`{}`), VATTreatment: in.VATTreatment,
		TaxRateBps: in.TaxRateBps, TaxNote: in.TaxNote, SubtotalMinor: t.SubtotalMinor, TaxMinor: t.TaxMinor,
		TotalMinor: t.TotalMinor, WithholdingRateBps: in.WithholdingRateBps, WithholdingMinor: t.WithholdingMinor,
		NetPayableMinor: t.NetPayableMinor, TaxBreakdown: t.Breakdown, UpdatedAt: now, CreatedAt: now,
	}
	f.invoices[inv.ID] = inv
	for i, l := range in.Lines {
		f.lines[inv.ID] = append(f.lines[inv.ID], &InvoiceLine{
			ID: uuid.New(), InvoiceID: inv.ID, SortOrder: i, Description: l.Description, Quantity: l.Quantity,
			UnitMinor: l.UnitMinor, TaxBps: t.LineTaxBps[i], LineTotalMinor: int64(l.Quantity) * l.UnitMinor, CreatedAt: now,
		})
	}
	return f.snapshot(inv), nil
}

func (f *fakeInvoiceRepo) GetByID(_ context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, ok := f.invoices[id]
	if !ok {
		return nil, nil, ErrInvoiceNotFound
	}
	return f.snapshot(inv), append([]*InvoiceLine{}, f.lines[id]...), nil
}

func (f *fakeInvoiceRepo) List(_ context.Context, filter InvoiceFilter) ([]*Invoice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*Invoice{}
	for _, inv := range f.invoices {
		if filter.GigID != nil && inv.GigID != *filter.GigID ||
			filter.Status != "" && inv.Status != filter.Status ||
			filter.Kind != "" && inv.Kind != filter.Kind ||
			filter.Currency != "" && inv.Currency != filter.Currency {
			continue
		}
		out = append(out, f.snapshot(inv))
	}
	return out, nil
}

// lock mirrors lockForTransition: not found, then stale token.
func (f *fakeInvoiceRepo) lock(id uuid.UUID, token time.Time) (*Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return nil, ErrInvoiceNotFound
	}
	if !inv.UpdatedAt.Equal(token) {
		return nil, ErrInvoiceConflict
	}
	return inv, nil
}

func isDraft(inv *Invoice) bool {
	return inv.Kind == InvoiceKindInvoice && inv.Status == InvoiceStatusDraft
}

func (f *fakeInvoiceRepo) UpdateDraft(_ context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, err := f.lock(id, req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if !isDraft(inv) {
		return nil, ErrInvoiceBadState
	}
	lines := f.lines[id]
	taxLines := make([]tax.Line, len(lines))
	for i, l := range lines {
		taxLines[i] = tax.Line{NetMinor: l.LineTotalMinor}
	}
	t := tax.ComputeTotals(taxLines, req.TaxRateBps, req.WithholdingRateBps)
	for i, l := range lines {
		l.TaxBps = t.LineTaxBps[i]
	}
	inv.Customer, inv.VATTreatment, inv.TaxRateBps, inv.TaxNote = req.Customer, req.VATTreatment, req.TaxRateBps, req.TaxNote
	inv.WithholdingRateBps, inv.SupplyDate, inv.DueAt = req.WithholdingRateBps, req.SupplyDate, req.DueAt.ptr()
	inv.NumberPrefix, inv.InternalNotes = req.NumberPrefix, req.InternalNotes
	inv.SubtotalMinor, inv.TaxMinor, inv.TotalMinor = t.SubtotalMinor, t.TaxMinor, t.TotalMinor
	inv.WithholdingMinor, inv.NetPayableMinor, inv.TaxBreakdown = t.WithholdingMinor, t.NetPayableMinor, t.Breakdown
	inv.UpdatedAt = f.tick()
	return f.snapshot(inv), nil
}

func (f *fakeInvoiceRepo) nextSeq(prefix, currency string) int64 {
	var seq int64
	for _, inv := range f.invoices {
		if inv.NumberPrefix == prefix && inv.Currency == currency && inv.NumberSeq != nil && *inv.NumberSeq > seq {
			seq = *inv.NumberSeq
		}
	}
	return seq + 1
}

func (f *fakeInvoiceRepo) number(inv *Invoice, prefix string) {
	seq := f.nextSeq(prefix, inv.Currency)
	num := FormatInvoiceNumber(prefix, inv.Currency, seq)
	inv.NumberPrefix, inv.NumberSeq, inv.InvoiceNumber = prefix, &seq, &num
}

func (f *fakeInvoiceRepo) Issue(_ context.Context, id uuid.UUID, req IssueInvoiceRequest, profile *BillingProfile, check IssueCheckFunc) (*Invoice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, err := f.lock(id, req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if !isDraft(inv) {
		return nil, ErrInvoiceBadState
	}
	if check != nil {
		if problems := check(f.snapshot(inv), f.gigCurrency[inv.GigID]); len(problems) > 0 {
			return nil, &NotIssuableError{Problems: problems}
		}
	}
	f.number(inv, inv.NumberPrefix)
	now := f.tick()
	inv.BillingProfile, inv.Status, inv.IssuedAt, inv.UpdatedAt = profileJSON(profile), InvoiceStatusIssued, &now, now
	return f.snapshot(inv), nil
}

func (f *fakeInvoiceRepo) Pay(_ context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, err := f.lock(id, req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if inv.Kind != InvoiceKindInvoice || inv.Status != InvoiceStatusIssued {
		return nil, ErrInvoiceBadState
	}
	paidAt := req.PaidAt
	inv.Status, inv.PaidAt, inv.PaymentRef, inv.UpdatedAt = InvoiceStatusPaid, &paidAt, req.PaymentRef, f.tick()
	return f.snapshot(inv), nil
}

func (f *fakeInvoiceRepo) Cancel(_ context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, err := f.lock(id, req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if !isDraft(inv) {
		return nil, ErrInvoiceBadState
	}
	inv.Status, inv.UpdatedAt = InvoiceStatusCancelled, f.tick()
	return f.snapshot(inv), nil
}

func (f *fakeInvoiceRepo) copyLines(from, to uuid.UUID) {
	for _, l := range f.lines[from] {
		cp := *l
		cp.ID, cp.InvoiceID = uuid.New(), to
		f.lines[to] = append(f.lines[to], &cp)
	}
}

func (f *fakeInvoiceRepo) CreditNote(_ context.Context, id uuid.UUID, req CreditNoteRequest, profile *BillingProfile, correct bool) (*CreditNoteResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	orig, err := f.lock(id, req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if orig.Kind != InvoiceKindInvoice || (orig.Status != InvoiceStatusIssued && orig.Status != InvoiceStatusPaid) {
		return nil, ErrInvoiceBadState
	}
	now := f.tick()
	cn := *orig
	cn.ID, cn.Kind, cn.CreditsInvoiceID, cn.Status = uuid.New(), InvoiceKindCreditNote, &orig.ID, InvoiceStatusIssued
	cn.IssuedAt, cn.DueAt, cn.PaidAt, cn.PaymentRef, cn.InternalNotes = &now, nil, nil, "", req.Reason
	cn.UpdatedAt, cn.CreatedAt = now, now
	if profile != nil {
		cn.BillingProfile = profileJSON(profile)
	}
	f.number(&cn, CreditNotePrefix)
	f.invoices[cn.ID] = &cn
	f.copyLines(orig.ID, cn.ID)
	res := &CreditNoteResult{}
	if correct {
		repl := *orig
		repl.ID, repl.Status, repl.InvoiceNumber, repl.NumberSeq = uuid.New(), InvoiceStatusDraft, nil, nil
		repl.IssuedAt, repl.DueAt, repl.PaidAt, repl.PaymentRef = nil, nil, nil, ""
		repl.BillingProfile, repl.InternalNotes = json.RawMessage(`{}`), "Replaces "+orig.Number()
		repl.UpdatedAt, repl.CreatedAt = now, now
		f.invoices[repl.ID] = &repl
		f.copyLines(orig.ID, repl.ID)
		orig.Status, orig.ReplacedByInvoiceID = InvoiceStatusCorrected, &repl.ID
		res.Replacement = f.snapshot(&repl)
	} else {
		orig.Status = InvoiceStatusCredited
	}
	orig.UpdatedAt = now
	res.CreditNote, res.Original = f.snapshot(&cn), f.snapshot(orig)
	return res, nil
}

func (f *fakeInvoiceRepo) NextNumber(_ context.Context, prefix, currency string) (string, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seq := f.nextSeq(prefix, currency)
	return FormatInvoiceNumber(prefix, currency, seq), seq, nil
}

func (f *fakeInvoiceRepo) Summaries(_ context.Context) (map[string]CurrencySummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sums := make(map[string]CurrencySummary)
	for _, inv := range f.invoices {
		if inv.Kind != InvoiceKindInvoice {
			continue
		}
		s := sums[inv.Currency]
		s.Currency = inv.Currency
		b := f.snapshot(inv)
		switch inv.Status {
		case InvoiceStatusDraft:
			s.DraftCount++
		case InvoiceStatusIssued:
			s.IssuedCount++
			s.OutstandingMinor += b.OutstandingMinor
			s.PaidMinor += b.ReceivedMinor
		case InvoiceStatusPaid:
			s.PaidCount++
			s.PaidMinor += inv.NetPayableMinor
		}
		sums[inv.Currency] = s
	}
	return sums, nil
}

// fakeGigFeeProvider implements GigFeeProvider for contract tests.
type fakeGigFeeProvider struct {
	fees map[uuid.UUID]*GigFeeInfo
}

func newFakeGigFeeProvider() *fakeGigFeeProvider {
	return &fakeGigFeeProvider{fees: make(map[uuid.UUID]*GigFeeInfo)}
}

func (f *fakeGigFeeProvider) SetFee(gigID uuid.UUID, fee *GigFeeInfo) { f.fees[gigID] = fee }

func (f *fakeGigFeeProvider) GetGigFeeInfo(_ context.Context, gigID uuid.UUID) (*GigFeeInfo, error) {
	fee, ok := f.fees[gigID]
	if !ok {
		return nil, ErrInvoiceNotFound // reused for "gig not found"
	}
	return fee, nil
}

// fakeBillingService implements the billing ServiceIface for contract tests:
// a German VAT-registered supplier charging 19%.
type fakeBillingService struct {
	profile *BillingProfile
}

func newFakeBillingService() *fakeBillingService {
	return &fakeBillingService{
		profile: &BillingProfile{
			LegalName:         "Test DJ",
			EntityKind:        EntityKindIndividual,
			TaxID:             "DE123456789",
			TaxIDKind:         TaxIDKindVAT,
			ContactEmail:      "billing@test.com",
			AddressLine1:      "1 Test Street",
			AddressCity:       "Berlin",
			AddressPostal:     "10115",
			AddressCountry:    "DE",
			Jurisdiction:      "DE",
			DefaultCurrency:   "EUR",
			DefaultVATRateBps: 1900,
			UpdatedAt:         time.Now().UTC(),
			CreatedAt:         time.Now().UTC(),
		},
	}
}

func (f *fakeBillingService) Get(ctx context.Context) (*BillingProfile, error) {
	cp := *f.profile
	return &cp, nil
}

func (f *fakeBillingService) Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error) {
	f.profile.LegalName = req.LegalName
	f.profile.AddressLine1 = req.AddressLine1
	f.profile.AddressCity = req.AddressCity
	f.profile.AddressPostal = req.AddressPostal
	f.profile.AddressCountry = req.AddressCountry
	f.profile.TaxID = req.TaxID
	f.profile.TaxIDKind = req.TaxIDKind
	f.profile.VATExemptSmallBusiness = req.VATExemptSmallBusiness
	f.profile.DefaultVATRateBps = req.DefaultVATRateBps
	f.profile.UpdatedAt = time.Now().UTC()
	return f.profile, nil
}

// --- harness ---------------------------------------------------------------

type invoiceHarness struct {
	t       *testing.T
	repo    *fakeInvoiceRepo
	gigs    *fakeGigFeeProvider
	billing *fakeBillingService
	h       http.Handler
	gigID   uuid.UUID
}

// completeCustomer is a Hamburg club: issuable as domestic German VAT.
func completeCustomer() *Party {
	return &Party{LegalName: "Club Hamburg GmbH", AddressLine1: "Reeperbahn 1", City: "Hamburg",
		PostalCode: "20359", Country: "DE", IsBusiness: true}
}

func newInvoiceHarness(t *testing.T) *invoiceHarness {
	t.Helper()
	hs := &invoiceHarness{t: t, repo: newFakeInvoiceRepo(), gigs: newFakeGigFeeProvider(), billing: newFakeBillingService(), gigID: uuid.New()}
	hs.gigs.SetFee(hs.gigID, &GigFeeInfo{
		ID: hs.gigID, Currency: "EUR", FeeMinor: 25000, LineDescription: "Performance",
		Date: "2026-10-03", Customer: completeCustomer(),
	})
	hs.repo.gigCurrency[hs.gigID] = "EUR"
	hs.h = NewInvoiceHandler(NewInvoiceService(hs.repo, hs.billing, hs.gigs))
	return hs
}

// do sends a request and decodes the JSON response into a generic map.
func (hs *invoiceHarness) do(method, path string, body any) (int, map[string]any) {
	hs.t.Helper()
	var rd *strings.Reader
	switch b := body.(type) {
	case nil:
		rd = strings.NewReader("")
	case string:
		rd = strings.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = strings.NewReader(string(raw))
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/finance/invoices"+path, rd))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// decode re-decodes a map (e.g. out["data"]) into v.
func decode(t *testing.T, m any, v any) {
	t.Helper()
	raw, _ := json.Marshal(m)
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
}

func (hs *invoiceHarness) createDraft(body string) Invoice {
	hs.t.Helper()
	if body == "" {
		body = `{"gig_id":"` + hs.gigID.String() + `"}`
	}
	code, out := hs.do(http.MethodPost, "", body)
	if code != http.StatusCreated {
		hs.t.Fatalf("create draft: %d %v", code, out)
	}
	var inv Invoice
	decode(hs.t, out["data"], &inv)
	return inv
}

func (hs *invoiceHarness) issue(inv Invoice) Invoice {
	hs.t.Helper()
	code, out := hs.do(http.MethodPost, "/"+inv.ID.String()+"/issue", IssueInvoiceRequest{UpdatedAt: inv.UpdatedAt})
	if code != http.StatusOK {
		hs.t.Fatalf("issue: %d %v", code, out)
	}
	var issued Invoice
	decode(hs.t, out["data"], &issued)
	return issued
}

func token(inv Invoice) map[string]any { return map[string]any{"updated_at": inv.UpdatedAt} }

// --- tests -----------------------------------------------------------------

func TestInvoiceHandler_CreateDraftPrefillsAndIsUnnumbered(t *testing.T) {
	hs := newInvoiceHarness(t)
	code, out := hs.do(http.MethodPost, "", `{"gig_id":"`+hs.gigID.String()+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("status %d: %v", code, out)
	}
	data := out["data"].(map[string]any)
	// Wire contract: unnumbered drafts serialise as null.
	for _, f := range []string{"invoice_number", "number_seq", "issued_at", "credits_invoice_id", "replaced_by_invoice_id"} {
		if v, present := data[f]; !present || v != nil {
			t.Errorf("%s = %v (present=%v), want null", f, v, present)
		}
	}
	for _, f := range []string{"kind", "customer", "supply_date", "vat_treatment", "tax_note", "withholding_rate_bps",
		"withholding_minor", "net_payable_minor", "tax_breakdown", "received_minor", "pending_minor", "outstanding_minor"} {
		if _, ok := data[f]; !ok {
			t.Errorf("missing field %q", f)
		}
	}
	var inv Invoice
	decode(t, data, &inv)
	if inv.Kind != InvoiceKindInvoice || inv.Status != InvoiceStatusDraft || inv.Currency != "EUR" || inv.NumberPrefix != "INV" {
		t.Fatalf("draft = %+v", inv)
	}
	if inv.Customer.LegalName != "Club Hamburg GmbH" || inv.SupplyDate == nil || *inv.SupplyDate != "2026-10-03" {
		t.Fatalf("prefill: customer=%+v supply=%v", inv.Customer, inv.SupplyDate)
	}
	// DE supplier + DE customer → domestic 19%.
	if inv.VATTreatment != tax.Domestic || inv.TaxRateBps != 1900 || inv.TaxMinor != 4750 || inv.TotalMinor != 29750 || inv.NetPayableMinor != 29750 {
		t.Fatalf("tax: %+v", inv)
	}
	if len(inv.TaxBreakdown) != 1 || inv.TaxBreakdown[0] != (tax.BreakdownRow{RateBps: 1900, TaxableMinor: 25000, TaxMinor: 4750}) {
		t.Fatalf("breakdown = %+v", inv.TaxBreakdown)
	}
}

func TestInvoiceHandler_CreateDraftOverridesAndValidation(t *testing.T) {
	hs := newInvoiceHarness(t)
	inv := hs.createDraft(`{"gig_id":"` + hs.gigID.String() + `","vat_treatment":"exempt","withholding_rate_bps":1500,
		"supply_date":"2026-10-04","due_at":"2026-11-01","number_prefix":"gig",
		"customer":{"legal_name":"Promo SARL","country":"fr","vat_id":"fr 123"}}`)
	if inv.VATTreatment != tax.Exempt || inv.TaxRateBps != 0 || !strings.Contains(inv.TaxNote, "exempt") {
		t.Fatalf("explicit treatment: %+v", inv)
	}
	if inv.WithholdingMinor != 3750 || inv.NetPayableMinor != 21250 || inv.NumberPrefix != "GIG" {
		t.Fatalf("withholding/prefix: %+v", inv)
	}
	if inv.Customer.Country != "FR" || inv.Customer.VATID != "FR123" || *inv.SupplyDate != "2026-10-04" || inv.DueAt == nil {
		t.Fatalf("normalised overrides: %+v", inv)
	}

	cases := map[string]string{
		"currency mismatch": `{"gig_id":"` + hs.gigID.String() + `","currency":"USD"}`,
		"reserved prefix":   `{"gig_id":"` + hs.gigID.String() + `","number_prefix":"CN"}`,
		"bad treatment":     `{"gig_id":"` + hs.gigID.String() + `","vat_treatment":"zero"}`,
		"bad supply date":   `{"gig_id":"` + hs.gigID.String() + `","supply_date":"03/10/2026"}`,
		"bad country":       `{"gig_id":"` + hs.gigID.String() + `","customer":{"country":"Germany"}}`,
		"rate out of range": `{"gig_id":"` + hs.gigID.String() + `","tax_rate_bps":10001}`,
		"missing gig":       `{}`,
	}
	for name, body := range cases {
		code, out := hs.do(http.MethodPost, "", body)
		if code != http.StatusBadRequest || out["error"] != "validation_failed" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if code, out := hs.do(http.MethodPost, "", `{"gig_id":"`+uuid.NewString()+`"}`); code != http.StatusNotFound {
		t.Errorf("unknown gig: %d %v", code, out)
	}
}

func TestInvoiceHandler_GetEnvelope(t *testing.T) {
	hs := newInvoiceHarness(t)
	inv := hs.createDraft("")
	code, out := hs.do(http.MethodGet, "/"+inv.ID.String(), nil)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var got Invoice
	decode(t, out["data"], &got)
	var lines []InvoiceLine
	decode(t, out["lines"], &lines)
	if got.ID != inv.ID || len(lines) != 1 || lines[0].TaxBps != 1900 {
		t.Fatalf("get: %+v lines=%+v", got, lines)
	}
	if code, out := hs.do(http.MethodGet, "/"+uuid.NewString(), nil); code != http.StatusNotFound || out["error"] != "not_found" {
		t.Fatalf("unknown invoice: %d %v", code, out)
	}
	if code, _ := hs.do(http.MethodGet, "/not-a-uuid", nil); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}
}

func TestInvoiceHandler_UpdateDraftRecomputesTotals(t *testing.T) {
	hs := newInvoiceHarness(t)
	inv := hs.createDraft("")
	body := UpdateInvoiceRequest{
		Customer:     Party{LegalName: "Paris Club SAS", AddressLine1: "1 Rue", City: "Paris", Country: "FR", VATID: "FR12345678901", IsBusiness: true},
		VATTreatment: tax.ReverseCharge, TaxRateBps: 0, TaxNote: tax.Notes.Note("DE", tax.ReverseCharge),
		WithholdingRateBps: 1000, NumberPrefix: "INV", InternalNotes: "note", UpdatedAt: inv.UpdatedAt,
	}
	code, out := hs.do(http.MethodPut, "/"+inv.ID.String(), body)
	if code != http.StatusOK {
		t.Fatalf("update: %d %v", code, out)
	}
	var upd Invoice
	decode(t, out["data"], &upd)
	if upd.TaxMinor != 0 || upd.TotalMinor != 25000 || upd.WithholdingMinor != 2500 || upd.NetPayableMinor != 22500 {
		t.Fatalf("totals not recomputed: %+v", upd)
	}
	if !upd.UpdatedAt.After(inv.UpdatedAt) || upd.InternalNotes != "note" || upd.Customer.Country != "FR" {
		t.Fatalf("update: %+v", upd)
	}
	_, lines, _ := hs.repo.GetByID(context.Background(), inv.ID)
	if lines[0].TaxBps != 0 {
		t.Fatalf("line tax_bps not rewritten: %d", lines[0].TaxBps)
	}

	// Stale token → conflict; invalid body → validation_failed; missing token → 400.
	body.UpdatedAt = inv.UpdatedAt
	if code, out := hs.do(http.MethodPut, "/"+inv.ID.String(), body); code != http.StatusConflict || out["error"] != "conflict" {
		t.Fatalf("stale: %d %v", code, out)
	}
	body.UpdatedAt, body.VATTreatment = upd.UpdatedAt, ""
	if code, out := hs.do(http.MethodPut, "/"+inv.ID.String(), body); code != http.StatusBadRequest || out["error"] != "validation_failed" {
		t.Fatalf("invalid: %d %v", code, out)
	}
	if code, _ := hs.do(http.MethodPut, "/"+inv.ID.String(), `{"vat_treatment":"none"}`); code != http.StatusBadRequest {
		t.Fatalf("missing token: %d", code)
	}
}

func TestInvoiceHandler_IssueCheckThenIssue(t *testing.T) {
	hs := newInvoiceHarness(t)
	hs.gigs.fees[hs.gigID].Customer = &Party{LegalName: "Promoter"} // no address
	inv := hs.createDraft("")

	code, out := hs.do(http.MethodGet, "/"+inv.ID.String()+"/issue-check", nil)
	if code != http.StatusOK {
		t.Fatalf("issue-check: %d %v", code, out)
	}
	var check IssueCheck
	decode(t, out["data"], &check)
	if check.Ready || len(check.Problems) == 0 {
		t.Fatalf("check = %+v", check)
	}
	fields := map[string]bool{}
	for _, p := range check.Problems {
		fields[p.Field] = true
	}
	for _, f := range []string{"customer.address_line1", "customer.city", "customer.country"} {
		if !fields[f] {
			t.Errorf("missing problem %s in %+v", f, check.Problems)
		}
	}

	// Issue is refused with the same problems and consumes no number.
	code, out = hs.do(http.MethodPost, "/"+inv.ID.String()+"/issue", token(inv))
	if code != http.StatusUnprocessableEntity || out["error"] != "not_issuable" {
		t.Fatalf("issue unready: %d %v", code, out)
	}
	if ps, ok := out["problems"].([]any); !ok || len(ps) != len(check.Problems) {
		t.Fatalf("problems = %v", out["problems"])
	}

	body := UpdateInvoiceRequest{Customer: *completeCustomer(), VATTreatment: tax.Domestic, TaxRateBps: 1900,
		NumberPrefix: "INV", SupplyDate: inv.SupplyDate, UpdatedAt: inv.UpdatedAt}
	_, out = hs.do(http.MethodPut, "/"+inv.ID.String(), body)
	var fixed Invoice
	decode(t, out["data"], &fixed)

	code, out = hs.do(http.MethodGet, "/"+inv.ID.String()+"/issue-check", nil)
	decode(t, out["data"], &check)
	if code != http.StatusOK || !check.Ready || len(check.Problems) != 0 {
		t.Fatalf("after fix: %d %+v", code, check)
	}
	issued := hs.issue(fixed)
	if issued.Number() != "INV-0001-EUR" || issued.Status != InvoiceStatusIssued || issued.IssuedAt == nil {
		t.Fatalf("issued = %+v", issued)
	}
	if !strings.Contains(string(issued.BillingProfile), "Test DJ") {
		t.Fatalf("billing profile not snapshotted: %s", issued.BillingProfile)
	}
	if issued.OutstandingMinor != issued.NetPayableMinor {
		t.Fatalf("outstanding = %d", issued.OutstandingMinor)
	}

	// Issued invoices are not drafts any more.
	code, out = hs.do(http.MethodGet, "/"+inv.ID.String()+"/issue-check", nil)
	decode(t, out["data"], &check)
	if check.Ready || check.Problems[0].Field != "status" {
		t.Fatalf("check on issued: %+v", check)
	}
}

func TestInvoiceHandler_ConflictVersusBadState(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")
	issued := hs.issue(draft)

	// Stale token → 409 conflict.
	if code, out := hs.do(http.MethodPost, "/"+draft.ID.String()+"/issue", token(draft)); code != http.StatusConflict || out["error"] != "conflict" {
		t.Fatalf("stale re-issue: %d %v", code, out)
	}
	// Fresh token, wrong state → 409 bad_state.
	if code, out := hs.do(http.MethodPost, "/"+draft.ID.String()+"/issue", token(issued)); code != http.StatusConflict || out["error"] != "bad_state" {
		t.Fatalf("re-issue: %d %v", code, out)
	}
	if code, out := hs.do(http.MethodPost, "/"+draft.ID.String()+"/cancel", token(issued)); code != http.StatusConflict || out["error"] != "bad_state" {
		t.Fatalf("cancel issued: %d %v", code, out)
	}
	if code, out := hs.do(http.MethodPut, "/"+draft.ID.String(), UpdateInvoiceRequest{VATTreatment: tax.None, UpdatedAt: issued.UpdatedAt}); code != http.StatusConflict || out["error"] != "bad_state" {
		t.Fatalf("edit issued: %d %v", code, out)
	}
	if code, out := hs.do(http.MethodPost, "/"+uuid.NewString()+"/pay", token(issued)); code != http.StatusNotFound {
		t.Fatalf("pay unknown: %d %v", code, out)
	}
	if code, _ := hs.do(http.MethodPost, "/"+draft.ID.String()+"/issue", `{}`); code != http.StatusBadRequest {
		t.Fatalf("missing token: %d", code)
	}
	if code, _ := hs.do(http.MethodGet, "/"+draft.ID.String()+"/issue", nil); code != http.StatusMethodNotAllowed {
		t.Fatalf("GET issue: %d", code)
	}
	if code, _ := hs.do(http.MethodPost, "/"+draft.ID.String()+"/explode", `{}`); code != http.StatusNotFound {
		t.Fatalf("unknown action: %d", code)
	}
}

func TestInvoiceHandler_PayAndCancel(t *testing.T) {
	hs := newInvoiceHarness(t)
	issued := hs.issue(hs.createDraft(""))
	code, out := hs.do(http.MethodPost, "/"+issued.ID.String()+"/pay", PayInvoiceRequest{PaidAt: time.Now().UTC(), PaymentRef: "ref-123", UpdatedAt: issued.UpdatedAt})
	if code != http.StatusOK {
		t.Fatalf("pay: %d %v", code, out)
	}
	var paid Invoice
	decode(t, out["data"], &paid)
	if paid.Status != InvoiceStatusPaid || paid.PaidAt == nil || paid.PaymentRef != "ref-123" || paid.OutstandingMinor != 0 {
		t.Fatalf("paid = %+v", paid)
	}

	draft := hs.createDraft("")
	code, out = hs.do(http.MethodPost, "/"+draft.ID.String()+"/cancel", token(draft))
	var cancelled Invoice
	decode(t, out["data"], &cancelled)
	if code != http.StatusOK || cancelled.Status != InvoiceStatusCancelled || cancelled.InvoiceNumber != nil {
		t.Fatalf("cancel: %d %+v", code, cancelled)
	}
	// The cancelled draft consumed nothing: the next issue is INV-0002.
	if next := hs.issue(hs.createDraft("")); next.Number() != "INV-0002-EUR" {
		t.Fatalf("after cancel: %s", next.Number())
	}
}

func TestInvoiceHandler_CreditNote(t *testing.T) {
	hs := newInvoiceHarness(t)
	issued := hs.issue(hs.createDraft(""))

	code, out := hs.do(http.MethodPost, "/"+issued.ID.String()+"/credit-note", map[string]any{"reason": "gig cancelled", "updated_at": issued.UpdatedAt})
	if code != http.StatusCreated {
		t.Fatalf("credit note: %d %v", code, out)
	}
	var res CreditNoteResult
	decode(t, out["data"], &res)
	cn, orig := res.CreditNote, res.Original
	if cn == nil || orig == nil || res.Replacement != nil {
		t.Fatalf("result = %+v", res)
	}
	if cn.Kind != InvoiceKindCreditNote || cn.Number() != "CN-0001-EUR" || cn.Status != InvoiceStatusIssued ||
		cn.CreditsInvoiceID == nil || *cn.CreditsInvoiceID != issued.ID || cn.TotalMinor != issued.TotalMinor {
		t.Fatalf("credit note = %+v", cn)
	}
	if cn.OutstandingMinor != 0 || cn.InternalNotes != "gig cancelled" {
		t.Fatalf("credit note balances/reason = %+v", cn)
	}
	if orig.Status != InvoiceStatusCredited || orig.OutstandingMinor != 0 {
		t.Fatalf("original = %+v", orig)
	}
	// Credit notes can't be paid, credited or cancelled.
	for _, action := range []string{"pay", "credit-note", "cancel", "correct"} {
		if code, out := hs.do(http.MethodPost, "/"+cn.ID.String()+"/"+action, token(*cn)); code != http.StatusConflict || out["error"] != "bad_state" {
			t.Errorf("%s on credit note: %d %v", action, code, out)
		}
	}
	// A credited invoice can't be credited again.
	if code, out := hs.do(http.MethodPost, "/"+orig.ID.String()+"/credit-note", token(*orig)); code != http.StatusConflict || out["error"] != "bad_state" {
		t.Fatalf("double credit: %d %v", code, out)
	}
	// A draft can't be credited.
	d := hs.createDraft("")
	if code, out := hs.do(http.MethodPost, "/"+d.ID.String()+"/credit-note", token(d)); code != http.StatusConflict || out["error"] != "bad_state" {
		t.Fatalf("credit draft: %d %v", code, out)
	}
	// kind filter
	code, out = hs.do(http.MethodGet, "?kind=credit_note", nil)
	if list, _ := out["data"].([]any); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("kind filter: %d %v", code, out)
	}
	if code, _ := hs.do(http.MethodGet, "?kind=bogus", nil); code != http.StatusBadRequest {
		t.Fatalf("bad kind filter: %d", code)
	}
}

func TestInvoiceHandler_Correct(t *testing.T) {
	hs := newInvoiceHarness(t)
	issued := hs.issue(hs.createDraft(""))
	code, out := hs.do(http.MethodPost, "/"+issued.ID.String()+"/correct", map[string]any{"reason": "wrong VAT", "updated_at": issued.UpdatedAt})
	if code != http.StatusCreated {
		t.Fatalf("correct: %d %v", code, out)
	}
	var res CreditNoteResult
	decode(t, out["data"], &res)
	if res.CreditNote == nil || res.Replacement == nil || res.Original == nil {
		t.Fatalf("result = %+v", res)
	}
	repl := res.Replacement
	if repl.Status != InvoiceStatusDraft || repl.InvoiceNumber != nil || repl.TotalMinor != issued.TotalMinor || repl.Customer != issued.Customer {
		t.Fatalf("replacement = %+v", repl)
	}
	if res.Original.Status != InvoiceStatusCorrected || res.Original.ReplacedByInvoiceID == nil || *res.Original.ReplacedByInvoiceID != repl.ID {
		t.Fatalf("original = %+v", res.Original)
	}
	// The replacement issues as the next number in the invoice series.
	if next := hs.issue(*repl); next.Number() != "INV-0002-EUR" {
		t.Fatalf("replacement number = %s", next.Number())
	}
}

func TestInvoiceHandler_TaxSuggestion(t *testing.T) {
	hs := newInvoiceHarness(t)
	cases := []struct {
		query string
		want  tax.Treatment
		rate  float64
	}{
		{"customer_country=DE", tax.Domestic, 1900},
		{"customer_country=fr&customer_vat_id=FR123&customer_is_business=true", tax.ReverseCharge, 0},
		{"customer_country=FR", tax.Domestic, 1900},
		{"customer_country=US", tax.OutsideScope, 0},
	}
	for _, tc := range cases {
		code, out := hs.do(http.MethodGet, "/tax-suggestion?"+tc.query, nil)
		data, _ := out["data"].(map[string]any)
		if code != http.StatusOK || data["vat_treatment"] != string(tc.want) || data["tax_rate_bps"] != tc.rate {
			t.Errorf("%s: %d %v", tc.query, code, out)
		}
		for _, f := range []string{"tax_note", "reason"} {
			if _, ok := data[f]; !ok {
				t.Errorf("%s: missing %s", tc.query, f)
			}
		}
	}
	if code, _ := hs.do(http.MethodPost, "/tax-suggestion", `{}`); code != http.StatusMethodNotAllowed {
		t.Fatalf("POST tax-suggestion: %d", code)
	}
}

func TestInvoiceHandler_ListAndSummaries(t *testing.T) {
	hs := newInvoiceHarness(t)
	hs.createDraft("")
	paidSrc := hs.issue(hs.createDraft(""))
	hs.do(http.MethodPost, "/"+paidSrc.ID.String()+"/pay", PayInvoiceRequest{PaidAt: time.Now(), UpdatedAt: paidSrc.UpdatedAt})
	hs.issue(hs.createDraft(""))

	code, out := hs.do(http.MethodGet, "?status=issued", nil)
	if list, _ := out["data"].([]any); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list issued: %d %v", code, out)
	}
	if code, _ := hs.do(http.MethodGet, "?status=bogus", nil); code != http.StatusBadRequest {
		t.Fatalf("bad status filter: %d", code)
	}

	code, out = hs.do(http.MethodGet, "/summaries", nil)
	if code != http.StatusOK {
		t.Fatalf("summaries: %d %v", code, out)
	}
	var sums map[string]CurrencySummary
	decode(t, out["data"], &sums)
	eur := sums["EUR"]
	want := CurrencySummary{Currency: "EUR", DraftCount: 1, IssuedCount: 1, PaidCount: 1, OutstandingMinor: 29750, PaidMinor: 29750}
	if eur != want {
		t.Fatalf("summary = %+v, want %+v", eur, want)
	}
}

func TestInvoiceService_NextNumber(t *testing.T) {
	repo := newFakeInvoiceRepo()
	svc := NewInvoiceService(repo, nil, nil)
	num, seq, err := svc.NextNumber(context.Background(), "INV", "EUR")
	if err != nil || num != "INV-0001-EUR" || seq != 1 {
		t.Fatalf("NextNumber = %q %d %v", num, seq, err)
	}
}

func TestInvoiceService_IssueWithoutProfileIsNotIssuable(t *testing.T) {
	hs := newInvoiceHarness(t)
	svc := NewInvoiceService(hs.repo, nil, hs.gigs)
	inv, err := svc.CreateDraft(context.Background(), hs.gigID, CreateInvoiceRequest{GigID: hs.gigID})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	// No billing profile: no supplier country → treatment "none", supplier problems.
	_, err = svc.Issue(context.Background(), inv.ID, IssueInvoiceRequest{UpdatedAt: inv.UpdatedAt})
	var ni *NotIssuableError
	if !errors.As(err, &ni) || !errors.Is(err, ErrInvoiceNotIssuable) {
		t.Fatalf("want NotIssuableError, got %v", err)
	}
}

func TestFlexTime(t *testing.T) {
	var v struct{ At *FlexTime }
	for _, in := range []string{`{"At":"2026-11-01"}`, `{"At":"2026-11-01T00:00:00Z"}`} {
		if err := json.Unmarshal([]byte(in), &v); err != nil || v.At.ptr() == nil || v.At.ptr().Format("2006-01-02") != "2026-11-01" {
			t.Errorf("%s: %v %v", in, err, v.At)
		}
	}
	if err := json.Unmarshal([]byte(`{"At":"01/11/2026"}`), &v); err == nil {
		t.Error("expected error for bad date")
	}
	v.At = nil
	if err := json.Unmarshal([]byte(`{"At":null}`), &v); err != nil || v.At.ptr() != nil {
		t.Errorf("null: %v", err)
	}
}
