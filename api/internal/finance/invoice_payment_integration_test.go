package finance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/finance/tax"
)

// These tests run the real repositories against the testcontainers
// Postgres from TestMain. The contract tests use in-memory fakes; these
// prove the SQL (numbering, locks, backfilled columns, lateral payment
// aggregates, gig status sync) actually behaves.

func requirePG(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
}

// insertTestGig creates a gig with a 250.00 fee in currency, dated 2026-10-03.
func insertTestGig(t *testing.T, currency string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO gigs (date, venue, city, event_name, fee_amount, fee_currency)
		VALUES ('2026-10-03T21:00:00Z', 'Tresor', 'Berlin', 'Night Shift', 250.00, $1)
		RETURNING id`, currency).Scan(&id)
	if err != nil {
		t.Fatalf("insert gig: %v", err)
	}
	return id
}

func newTestInvoiceService() *InvoiceService {
	return NewInvoiceService(NewInvoiceRepository(testPool), NewService(NewRepository(testPool)), NewPGGigFeeProvider(testPool))
}

// setBillingProfile writes a complete German VAT-registered supplier
// profile (19%), then applies mutate for test-specific variations.
func setBillingProfile(t *testing.T, mutate func(*UpdateBillingProfileRequest)) {
	t.Helper()
	svc := NewService(NewRepository(testPool))
	cur, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	req := UpdateBillingProfileRequest{
		LegalName: "Lina Vasquez", EntityKind: EntityKindSoleTrader, TaxID: "DE123456789", TaxIDKind: TaxIDKindVAT,
		ContactEmail: "billing@example.com", AddressLine1: "Köpenicker Str. 70", AddressCity: "Berlin",
		AddressPostal: "10179", AddressCountry: "DE", Jurisdiction: "DE", DefaultCurrency: "EUR",
		DefaultVATRateBps: 1900, UpdatedAt: cur.UpdatedAt,
	}
	if mutate != nil {
		mutate(&req)
	}
	if _, err := svc.Update(context.Background(), &req); err != nil {
		t.Fatalf("update profile: %v", err)
	}
}

// hamburgClub is a complete domestic (DE) business customer.
func hamburgClub() *Party {
	return &Party{LegalName: "Club Hamburg GmbH", AddressLine1: "Reeperbahn 1", City: "Hamburg",
		PostalCode: "20359", Country: "DE", IsBusiness: true}
}

// issuableDraft creates a draft for gigID that passes the issue check.
func issuableDraft(t *testing.T, svc *InvoiceService, gigID uuid.UUID, prefix string, mutate func(*CreateInvoiceRequest)) *Invoice {
	t.Helper()
	req := CreateInvoiceRequest{GigID: gigID, NumberPrefix: prefix, Customer: hamburgClub()}
	if mutate != nil {
		mutate(&req)
	}
	inv, err := svc.CreateDraft(context.Background(), gigID, req)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	return inv
}

func problemFields(err error) []string {
	var ni *NotIssuableError
	if !errors.As(err, &ni) {
		return nil
	}
	out := make([]string, len(ni.Problems))
	for i, p := range ni.Problems {
		out[i] = p.Field
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestIntegration_DraftUnnumberedWithCustomerPrefill(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	contacts := contact.NewRepository(testPool)

	company := "Promo SARL"
	reader, err := contacts.Create(ctx, &contact.ContactCreate{Name: "Amélie Durand", Company: &company,
		Email: "amelie@promo.fr", Type: contact.ContactTypePromoter, PartyFields: contact.PartyFields{
			AddressLine1: "12 Rue Oberkampf", City: "Paris", PostalCode: "75011", Country: "FR",
			VATID: "FR12345678901", IsBusiness: true}})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	// 1. gig_reader_contact_id wins.
	gigID := insertTestGig(t, "EUR")
	if _, err := testPool.Exec(ctx, `UPDATE gigs SET gig_reader_contact_id = $2, promoter_name = 'Ignored' WHERE id = $1`, gigID, reader.ID); err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if draft.InvoiceNumber != nil || draft.NumberSeq != nil || draft.Status != InvoiceStatusDraft || draft.Currency != "EUR" {
		t.Fatalf("draft must be unnumbered: %+v", draft)
	}
	c := draft.Customer
	if c.ContactID == nil || *c.ContactID != reader.ID || c.LegalName != "Amélie Durand" || c.Company != "Promo SARL" ||
		c.City != "Paris" || c.Country != "FR" || c.VATID != "FR12345678901" || !c.IsBusiness {
		t.Fatalf("customer = %+v", c)
	}
	if draft.SupplyDate == nil || *draft.SupplyDate != "2026-10-03" {
		t.Fatalf("supply_date = %v", draft.SupplyDate)
	}
	// DE supplier, FR business with VAT ID → reverse charge.
	if draft.VATTreatment != tax.ReverseCharge || draft.TaxRateBps != 0 || !strings.Contains(draft.TaxNote, "Reverse charge") ||
		draft.TotalMinor != 25000 || draft.NetPayableMinor != 25000 {
		t.Fatalf("tax = %+v", draft)
	}
	var storedContact *uuid.UUID
	if err := testPool.QueryRow(ctx, `SELECT customer_contact_id FROM invoices WHERE id = $1`, draft.ID).Scan(&storedContact); err != nil ||
		storedContact == nil || *storedContact != reader.ID {
		t.Fatalf("customer_contact_id = %v err = %v", storedContact, err)
	}
	_, lines, err := svc.GetByID(ctx, draft.ID)
	if err != nil || len(lines) != 1 || lines[0].TaxBps != 0 || lines[0].LineTotalMinor != 25000 {
		t.Fatalf("lines = %+v err = %v", lines, err)
	}

	// 2. No reader contact: a promoter linked through gig_contacts.
	promoter, err := contacts.Create(ctx, &contact.ContactCreate{Name: "Hamburg Promotions", Type: contact.ContactTypePromoter,
		PartyFields: contact.PartyFields{AddressLine1: "Reeperbahn 1", City: "Hamburg", Country: "DE", IsBusiness: true}})
	if err != nil {
		t.Fatal(err)
	}
	gig2 := insertTestGig(t, "EUR")
	if _, err := testPool.Exec(ctx, `INSERT INTO gig_contacts (gig_id, contact_id, role) VALUES ($1, $2, 'promoter')`, gig2, promoter.ID); err != nil {
		t.Fatal(err)
	}
	d2, err := svc.CreateDraft(ctx, gig2, CreateInvoiceRequest{GigID: gig2})
	if err != nil || d2.Customer.ContactID == nil || *d2.Customer.ContactID != promoter.ID {
		t.Fatalf("promoter prefill: %+v err = %v", d2, err)
	}
	if d2.VATTreatment != tax.Domestic || d2.TaxRateBps != 1900 || d2.TaxMinor != 4750 || d2.TotalMinor != 29750 {
		t.Fatalf("domestic suggestion: %+v", d2)
	}

	// 3. Only free-text promoter fields; a soft-deleted reader contact is ignored.
	deleted, err := contacts.Create(ctx, &contact.ContactCreate{Name: "Gone", Type: contact.ContactTypeOther})
	if err != nil {
		t.Fatal(err)
	}
	if err := contacts.SoftDelete(ctx, deleted.ID); err != nil {
		t.Fatal(err)
	}
	gig3 := insertTestGig(t, "EUR")
	if _, err := testPool.Exec(ctx, `UPDATE gigs SET gig_reader_contact_id = $2, promoter_name = 'Night Owls', promoter_email = 'owls@example.com' WHERE id = $1`, gig3, deleted.ID); err != nil {
		t.Fatal(err)
	}
	d3, err := svc.CreateDraft(ctx, gig3, CreateInvoiceRequest{GigID: gig3})
	if err != nil || d3.Customer.ContactID != nil || d3.Customer.LegalName != "Night Owls" || d3.Customer.Email != "owls@example.com" {
		t.Fatalf("promoter-name prefill: %+v err = %v", d3.Customer, err)
	}

	// Explicit customer wins over the gig contact.
	d4, err := svc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID, Customer: hamburgClub()})
	if err != nil || d4.Customer.ContactID != nil || d4.Customer.City != "Hamburg" {
		t.Fatalf("explicit customer: %+v err = %v", d4.Customer, err)
	}
	// Currency, when sent, must match the gig.
	if _, err := svc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID, Currency: "USD"}); !errors.Is(err, ErrInvoiceValidation) {
		t.Fatalf("currency mismatch: %v", err)
	}
}

func TestIntegration_IssueCheckThenIssue(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, func(r *UpdateBillingProfileRequest) { r.AddressCountry = "" })
	svc := newTestInvoiceService()
	gigID := insertTestGig(t, "EUR")
	if _, err := testPool.Exec(ctx, `UPDATE gigs SET promoter_name = 'Night Owls' WHERE id = $1`, gigID); err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID, NumberPrefix: "CHK"})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	check, err := svc.IssueCheck(ctx, draft.ID)
	if err != nil || check.Ready {
		t.Fatalf("check = %+v err = %v", check, err)
	}
	got := map[string]bool{}
	for _, p := range check.Problems {
		got[p.Field] = true
	}
	for _, f := range []string{"supplier.country", "customer.address_line1", "customer.city", "customer.country"} {
		if !got[f] {
			t.Errorf("missing problem %q in %+v", f, check.Problems)
		}
	}

	// Issue enforces the same check and consumes no number.
	_, err = svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if !errors.Is(err, ErrInvoiceNotIssuable) || !contains(problemFields(err), "customer.city") {
		t.Fatalf("issue unready: %v", err)
	}
	var num *string
	if err := testPool.QueryRow(ctx, `SELECT invoice_number FROM invoices WHERE id = $1`, draft.ID).Scan(&num); err != nil || num != nil {
		t.Fatalf("number after refused issue = %v err = %v", num, err)
	}

	// Fix supplier and customer, then issue.
	setBillingProfile(t, nil)
	fixed, err := svc.UpdateDraft(ctx, draft.ID, UpdateInvoiceRequest{
		Customer: *hamburgClub(), VATTreatment: tax.Domestic, TaxRateBps: 1900, NumberPrefix: "CHK",
		SupplyDate: draft.SupplyDate, InternalNotes: "fixed", UpdatedAt: draft.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	if fixed.TaxMinor != 4750 || fixed.TotalMinor != 29750 || fixed.Customer.City != "Hamburg" {
		t.Fatalf("recomputed = %+v", fixed)
	}
	if check, err = svc.IssueCheck(ctx, draft.ID); err != nil || !check.Ready || len(check.Problems) != 0 {
		t.Fatalf("check after fix = %+v err = %v", check, err)
	}
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: fixed.UpdatedAt})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if issued.Number() != "CHK-0001-EUR" || *issued.NumberSeq != 1 || issued.Status != InvoiceStatusIssued || issued.IssuedAt == nil {
		t.Fatalf("issued = %+v", issued)
	}
	var snap BillingProfileSnapshot
	if err := snap.FromJSON(issued.BillingProfile); err != nil || snap.LegalName != "Lina Vasquez" || snap.TaxID != "DE123456789" {
		t.Fatalf("snapshot = %+v err = %v", snap, err)
	}
	if issued.OutstandingMinor != 29750 || issued.ReceivedMinor != 0 {
		t.Fatalf("balances = %+v", issued)
	}
	// Issued documents are immutable.
	if _, err := svc.UpdateDraft(ctx, draft.ID, UpdateInvoiceRequest{VATTreatment: tax.None, UpdatedAt: issued.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("edit issued: %v", err)
	}
	if _, err := svc.Cancel(ctx, draft.ID, CancelInvoiceRequest{UpdatedAt: issued.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("cancel issued: %v", err)
	}
	if _, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: fixed.UpdatedAt}); !errors.Is(err, ErrInvoiceConflict) {
		t.Fatalf("stale re-issue: %v", err)
	}
	if _, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: issued.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("re-issue: %v", err)
	}
	if _, err := svc.Issue(ctx, uuid.New(), IssueInvoiceRequest{UpdatedAt: issued.UpdatedAt}); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestIntegration_GapFreeNumbering(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	gigID := insertTestGig(t, "SEK")
	// Domestic 19% is fine for numbering; currency matches the gig.
	mk := func() *Invoice { return issuableDraft(t, svc, gigID, "GAP", nil) }

	a, b, c := mk(), mk(), mk()
	if _, err := svc.Cancel(ctx, b.ID, CancelInvoiceRequest{UpdatedAt: b.UpdatedAt}); err != nil {
		t.Fatalf("cancel draft: %v", err)
	}
	for i, d := range []*Invoice{a, c} {
		issued, err := svc.Issue(ctx, d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		if want := FormatInvoiceNumber("GAP", "SEK", int64(i+1)); issued.Number() != want {
			t.Fatalf("number = %s, want %s (cancelled drafts must not consume numbers)", issued.Number(), want)
		}
	}
	// A refused issue consumes nothing either.
	bad := issuableDraft(t, svc, gigID, "GAP", func(r *CreateInvoiceRequest) { r.Customer = &Party{LegalName: "No Address"} })
	if _, err := svc.Issue(ctx, bad.ID, IssueInvoiceRequest{UpdatedAt: bad.UpdatedAt}); !errors.Is(err, ErrInvoiceNotIssuable) {
		t.Fatalf("issue incomplete: %v", err)
	}

	// Concurrent issues get distinct, contiguous numbers.
	const n = 10
	drafts := make([]*Invoice, n)
	for i := range drafts {
		drafts[i] = mk()
	}
	var wg sync.WaitGroup
	seqs := make(chan int64, n)
	for _, d := range drafts {
		wg.Add(1)
		go func(d *Invoice) {
			defer wg.Done()
			issued, err := svc.Issue(ctx, d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt})
			if err != nil {
				t.Errorf("concurrent issue: %v", err)
				return
			}
			seqs <- *issued.NumberSeq
		}(d)
	}
	wg.Wait()
	close(seqs)
	var got []int64
	for s := range seqs {
		got = append(got, s)
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if len(got) != n {
		t.Fatalf("issued %d of %d", len(got), n)
	}
	for i, s := range got {
		if s != int64(i+3) {
			t.Fatalf("sequences = %v, want 3..%d without gaps or duplicates", got, n+2)
		}
	}
}

func TestIntegration_ReverseChargeValidation(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	gigID := insertTestGig(t, "EUR")

	paris := Party{LegalName: "Paris Club SAS", AddressLine1: "1 Rue", City: "Paris", Country: "FR", IsBusiness: true}
	draft := issuableDraft(t, svc, gigID, "RC", func(r *CreateInvoiceRequest) {
		rc := tax.ReverseCharge
		r.Customer, r.VATTreatment = &paris, &rc
	})
	if draft.TaxRateBps != 0 || !strings.Contains(draft.TaxNote, "Art. 196") {
		t.Fatalf("explicit reverse charge: %+v", draft)
	}
	_, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if fs := problemFields(err); !contains(fs, "customer.vat_id") {
		t.Fatalf("missing customer VAT ID: %v (%v)", fs, err)
	}

	// Same country is not reverse charge.
	update := func(d *Invoice, customer Party, rate int64) (*Invoice, error) {
		return svc.UpdateDraft(ctx, d.ID, UpdateInvoiceRequest{Customer: customer, VATTreatment: tax.ReverseCharge,
			TaxRateBps: rate, TaxNote: draft.TaxNote, NumberPrefix: "RC", SupplyDate: d.SupplyDate, UpdatedAt: d.UpdatedAt})
	}
	berlin := *hamburgClub()
	berlin.VATID = "DE999999999"
	d, err := update(draft, berlin, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Issue(ctx, d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt}); !contains(problemFields(err), "customer.country") {
		t.Fatalf("same-country reverse charge: %v", err)
	}
	// Rate must be 0.
	paris.VATID = "FR12345678901"
	if d, err = update(d, paris, 1900); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Issue(ctx, d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt}); !contains(problemFields(err), "tax_rate_bps") {
		t.Fatalf("reverse charge with VAT: %v", err)
	}
	// Supplier without a VAT registration can't reverse-charge.
	setBillingProfile(t, func(r *UpdateBillingProfileRequest) { r.TaxIDKind = TaxIDKindOther })
	if d, err = update(d, paris, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Issue(ctx, d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt}); !contains(problemFields(err), "supplier.vat_id") {
		t.Fatalf("supplier VAT ID: %v", err)
	}
	setBillingProfile(t, nil)
	issued, err := svc.Issue(ctx, d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt})
	if err != nil {
		t.Fatalf("valid reverse charge: %v", err)
	}
	if issued.TaxMinor != 0 || issued.VATTreatment != tax.ReverseCharge || len(issued.TaxBreakdown) != 1 || issued.TaxBreakdown[0].RateBps != 0 {
		t.Fatalf("issued = %+v", issued)
	}
}

func TestIntegration_WithholdingPaymentsAndGigStatus(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	paySvc := NewPaymentService(NewPaymentRepository(testPool))
	gigID := insertTestGig(t, "CHF")
	gigStatus := func() string {
		var s string
		if err := testPool.QueryRow(ctx, `SELECT payment_status::text FROM gigs WHERE id = $1`, gigID).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	wh := int64(1500)
	draft := issuableDraft(t, svc, gigID, "WH", func(r *CreateInvoiceRequest) { r.WithholdingRateBps = &wh })
	// 250.00 + 19% = 297.50; withholding 15% of 250.00 = 37.50; net 260.00.
	if draft.TotalMinor != 29750 || draft.WithholdingMinor != 3750 || draft.NetPayableMinor != 26000 {
		t.Fatalf("withholding totals = %+v", draft)
	}
	pay := func(amount int64, kind PaymentKind) (*Payment, error) {
		return paySvc.Create(ctx, draft.ID, CreatePaymentRequest{Currency: "CHF", AmountMinor: amount, Kind: kind})
	}
	complete := func(p *Payment) {
		t.Helper()
		if _, err := paySvc.Update(ctx, p.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: p.UpdatedAt}); err != nil {
			t.Fatalf("complete payment: %v", err)
		}
	}
	if _, err := pay(1000, PaymentKindDeposit); !errors.Is(err, ErrPaymentInvoiceState) {
		t.Fatalf("payment on draft: %v", err)
	}
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// The limit is net payable (260.00), not the total (297.50).
	if _, err := pay(26001, PaymentKindPayment); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("over net payable: %v", err)
	}
	deposit, err := pay(10000, PaymentKindDeposit)
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	if s := gigStatus(); s != "unpaid" {
		t.Fatalf("pending deposit: gig status %s", s)
	}
	complete(deposit)
	if s := gigStatus(); s != "deposit_paid" {
		t.Fatalf("completed deposit: gig status %s", s)
	}
	rest, err := pay(16000, PaymentKindPayment)
	if err != nil {
		t.Fatalf("remainder: %v", err)
	}
	// List computes balances in SQL: received 100.00, pending 160.00, outstanding 0.
	list, err := svc.List(ctx, InvoiceFilter{GigID: &gigID})
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v err = %v", list, err)
	}
	if l := list[0]; l.ReceivedMinor != 10000 || l.PendingMinor != 16000 || l.OutstandingMinor != 0 {
		t.Fatalf("balances = received %d pending %d outstanding %d", l.ReceivedMinor, l.PendingMinor, l.OutstandingMinor)
	}
	if _, err := pay(1, PaymentKindPayment); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("pending counts toward the limit: %v", err)
	}
	complete(rest)
	if s := gigStatus(); s != "paid" {
		t.Fatalf("fully paid: gig status %s", s)
	}
	got, _, _ := svc.GetByID(ctx, issued.ID)
	if got.ReceivedMinor != 26000 || got.OutstandingMinor != 0 {
		t.Fatalf("after payment = %+v", got)
	}
	refund, err := pay(1000, PaymentKindRefund)
	if err != nil {
		t.Fatal(err)
	}
	complete(refund)
	if s := gigStatus(); s != "deposit_paid" {
		t.Fatalf("after refund: gig status %s", s)
	}
	got, _, _ = svc.GetByID(ctx, issued.ID)
	if got.ReceivedMinor != 25000 || got.OutstandingMinor != 1000 {
		t.Fatalf("after refund = %+v", got)
	}

	sums, err := svc.Summaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s := sums["CHF"]; s.IssuedCount != 1 || s.OutstandingMinor != 1000 || s.PaidMinor != 25000 {
		t.Fatalf("CHF summary = %+v", s)
	}

	// Credit notes are never payable, and the credited original stops taking money.
	res, err := svc.CreditNote(ctx, issued.ID, CreditNoteRequest{Reason: "refund in full", UpdatedAt: got.UpdatedAt})
	if err != nil {
		t.Fatalf("CreditNote: %v", err)
	}
	if _, err := paySvc.Create(ctx, res.CreditNote.ID, CreatePaymentRequest{Currency: "CHF", AmountMinor: 100, Kind: PaymentKindPayment}); !errors.Is(err, ErrPaymentInvoiceState) {
		t.Fatalf("payment on credit note: %v", err)
	}
	if _, err := pay(100, PaymentKindPayment); !errors.Is(err, ErrPaymentInvoiceState) {
		t.Fatalf("payment on credited invoice: %v", err)
	}
}

func TestIntegration_CreditNoteAndCorrect(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	gigID := insertTestGig(t, "NOK")
	issue := func() *Invoice {
		d := issuableDraft(t, svc, gigID, "CRN", nil)
		inv, err := svc.Issue(ctx, d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt})
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		return inv
	}

	first := issue()
	if _, err := svc.CreditNote(ctx, first.ID, CreditNoteRequest{UpdatedAt: first.UpdatedAt.Add(-time.Second)}); !errors.Is(err, ErrInvoiceConflict) {
		t.Fatalf("stale credit note: %v", err)
	}
	res, err := svc.CreditNote(ctx, first.ID, CreditNoteRequest{Reason: "gig cancelled", UpdatedAt: first.UpdatedAt})
	if err != nil {
		t.Fatalf("CreditNote: %v", err)
	}
	cn := res.CreditNote
	if cn.Kind != InvoiceKindCreditNote || cn.Number() != "CN-0001-NOK" || cn.Status != InvoiceStatusIssued || cn.IssuedAt == nil ||
		cn.CreditsInvoiceID == nil || *cn.CreditsInvoiceID != first.ID {
		t.Fatalf("credit note = %+v", cn)
	}
	if cn.TotalMinor != first.TotalMinor || cn.TaxMinor != first.TaxMinor || cn.Customer != first.Customer ||
		cn.VATTreatment != first.VATTreatment || cn.InternalNotes != "gig cancelled" || cn.OutstandingMinor != 0 {
		t.Fatalf("credit note copy = %+v", cn)
	}
	if res.Original.Status != InvoiceStatusCredited || res.Replacement != nil {
		t.Fatalf("original = %+v", res.Original)
	}
	_, cnLines, err := svc.GetByID(ctx, cn.ID)
	if err != nil || len(cnLines) != 1 || cnLines[0].UnitMinor != 25000 {
		t.Fatalf("credit note lines = %+v err = %v", cnLines, err)
	}
	if _, err := svc.CreditNote(ctx, first.ID, CreditNoteRequest{UpdatedAt: res.Original.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("double credit: %v", err)
	}
	if _, err := svc.CreditNote(ctx, cn.ID, CreditNoteRequest{UpdatedAt: cn.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("credit a credit note: %v", err)
	}
	if _, err := svc.Pay(ctx, cn.ID, PayInvoiceRequest{PaidAt: time.Now(), UpdatedAt: cn.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("pay credit note: %v", err)
	}

	// Correct a paid invoice: credit note + unnumbered replacement draft.
	second := issue()
	paid, err := svc.Pay(ctx, second.ID, PayInvoiceRequest{PaidAt: time.Now(), PaymentRef: "bank", UpdatedAt: second.UpdatedAt})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	corr, err := svc.Correct(ctx, paid.ID, CorrectInvoiceRequest{Reason: "wrong address", UpdatedAt: paid.UpdatedAt})
	if err != nil {
		t.Fatalf("Correct: %v", err)
	}
	repl := corr.Replacement
	if corr.CreditNote.Number() != "CN-0002-NOK" || corr.Original.Status != InvoiceStatusCorrected ||
		corr.Original.ReplacedByInvoiceID == nil || *corr.Original.ReplacedByInvoiceID != repl.ID {
		t.Fatalf("correct result = %+v / %+v", corr.CreditNote, corr.Original)
	}
	if repl.Status != InvoiceStatusDraft || repl.InvoiceNumber != nil || repl.Kind != InvoiceKindInvoice ||
		repl.TotalMinor != second.TotalMinor || repl.Customer != second.Customer || repl.NumberPrefix != "CRN" ||
		repl.InternalNotes != "Replaces "+second.Number() {
		t.Fatalf("replacement = %+v", repl)
	}
	if string(repl.BillingProfile) != "{}" {
		t.Fatalf("replacement supplier snapshot must be set at issue, got %s", repl.BillingProfile)
	}
	reissued, err := svc.Issue(ctx, repl.ID, IssueInvoiceRequest{UpdatedAt: repl.UpdatedAt})
	if err != nil || reissued.Number() != "CRN-0003-NOK" {
		t.Fatalf("issue replacement = %v err = %v", reissued, err)
	}
	if _, err := svc.Correct(ctx, repl.ID, CorrectInvoiceRequest{UpdatedAt: repl.UpdatedAt}); !errors.Is(err, ErrInvoiceConflict) {
		t.Fatalf("stale correct: %v", err)
	}
	d := issuableDraft(t, svc, gigID, "CRN", nil)
	if _, err := svc.Correct(ctx, d.ID, CorrectInvoiceRequest{UpdatedAt: d.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("correct draft: %v", err)
	}

	cns, err := svc.List(ctx, InvoiceFilter{GigID: &gigID, Kind: InvoiceKindCreditNote})
	if err != nil || len(cns) != 2 {
		t.Fatalf("credit notes listed = %d err = %v", len(cns), err)
	}
	sums, _ := svc.Summaries(ctx)
	// Credit notes are excluded; the credited/corrected originals count nowhere.
	if s := sums["NOK"]; s.DraftCount != 1 || s.IssuedCount != 1 || s.PaidCount != 0 || s.OutstandingMinor != reissued.NetPayableMinor {
		t.Fatalf("NOK summary = %+v", s)
	}
}

// TestIntegration_HTTPThroughChiMount drives the real handlers through
// finance.Mux mounted the way apphttp.NewRouter mounts it, so path
// dispatch (including /invoices/{id}/payments → payments) is covered.
func TestIntegration_HTTPThroughChiMount(t *testing.T) {
	requirePG(t)
	setBillingProfile(t, nil)
	gigID := insertTestGig(t, "EUR")
	mux := NewMux(
		NewHandler(NewService(NewRepository(testPool))),
		NewInvoiceHandler(newTestInvoiceService()),
		NewPaymentHandler(NewPaymentService(NewPaymentRepository(testPool))),
		nil, nil, nil, nil,
	)
	router := chi.NewRouter()
	router.Mount("/api/v1/finance", mux)

	do := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var raw string
		switch b := body.(type) {
		case nil:
		case string:
			raw = b
		default:
			bs, _ := json.Marshal(b)
			raw = string(bs)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/finance"+path, strings.NewReader(raw)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	field := func(m map[string]any, keys ...string) any {
		var cur any = m
		for _, k := range keys {
			cur = cur.(map[string]any)[k]
		}
		return cur
	}

	// Draft without customer details → issue-check lists problems.
	code, out := do(http.MethodPost, "/invoices", `{"gig_id":"`+gigID.String()+`","number_prefix":"HTTP"}`)
	if code != http.StatusCreated || field(out, "data", "invoice_number") != nil {
		t.Fatalf("create draft: %d %v", code, out)
	}
	id := field(out, "data", "id").(string)
	updatedAt := field(out, "data", "updated_at").(string)

	code, out = do(http.MethodGet, "/invoices/"+id+"/issue-check", nil)
	if code != http.StatusOK || field(out, "data", "ready") != false || len(field(out, "data", "problems").([]any)) == 0 {
		t.Fatalf("issue-check: %d %v", code, out)
	}
	code, out = do(http.MethodPost, "/invoices/"+id+"/issue", `{"updated_at":"`+updatedAt+`"}`)
	if code != http.StatusUnprocessableEntity || out["error"] != "not_issuable" || len(out["problems"].([]any)) == 0 {
		t.Fatalf("issue unready: %d %v", code, out)
	}

	// PUT a complete customer; totals recomputed.
	code, out = do(http.MethodPut, "/invoices/"+id, map[string]any{
		"customer": hamburgClub(), "vat_treatment": "domestic", "tax_rate_bps": 700, "tax_note": "",
		"withholding_rate_bps": 0, "supply_date": "2026-10-03", "due_at": "2026-11-01", "number_prefix": "HTTP",
		"internal_notes": "", "updated_at": updatedAt,
	})
	if code != http.StatusOK || field(out, "data", "tax_minor") != float64(1750) || field(out, "data", "due_at") == nil {
		t.Fatalf("update: %d %v", code, out)
	}
	updatedAt = field(out, "data", "updated_at").(string)

	code, out = do(http.MethodGet, "/invoices/"+id, nil)
	if code != http.StatusOK || field(out, "data", "id") != id || len(out["lines"].([]any)) != 1 {
		t.Fatalf("get: %d %v", code, out)
	}
	if line := out["lines"].([]any)[0].(map[string]any); line["tax_bps"] != float64(700) {
		t.Fatalf("line tax_bps = %v", line["tax_bps"])
	}

	code, out = do(http.MethodPost, "/invoices/"+id+"/issue", `{"updated_at":"`+updatedAt+`"}`)
	if code != http.StatusOK || field(out, "data", "invoice_number") != "HTTP-0001-EUR" {
		t.Fatalf("issue: %d %v", code, out)
	}
	updatedAt = field(out, "data", "updated_at").(string)
	if code, out := do(http.MethodPost, "/invoices/"+id+"/issue", `{"updated_at":"`+updatedAt+`"}`); code != http.StatusConflict || out["error"] != "bad_state" {
		t.Fatalf("re-issue: %d %v", code, out)
	}

	// Payments through the mux (net payable = 267.50).
	if code, out := do(http.MethodPost, "/invoices/"+id+"/payments", `{"currency":"EUR","amount_minor":99999,"kind":"payment"}`); code != http.StatusUnprocessableEntity || out["error"] != "exceeds_balance" {
		t.Fatalf("overpayment: %d %v", code, out)
	}
	if code, out := do(http.MethodPost, "/invoices/"+id+"/payments", `{"currency":"EUR","amount_minor":10000,"kind":"deposit"}`); code != http.StatusCreated {
		t.Fatalf("deposit: %d %v", code, out)
	}
	code, out = do(http.MethodGet, "/invoices?gig_id="+gigID.String(), nil)
	if code != http.StatusOK {
		t.Fatalf("list: %d %v", code, out)
	}
	row := out["data"].([]any)[0].(map[string]any)
	if row["pending_minor"] != float64(10000) || row["outstanding_minor"] != float64(16750) || row["received_minor"] != float64(0) {
		t.Fatalf("list balances = %v", row)
	}
	updatedAt = row["updated_at"].(string)

	code, out = do(http.MethodGet, "/invoices/tax-suggestion?customer_country=AT&customer_vat_id=ATU12345678&customer_is_business=true", nil)
	if code != http.StatusOK || field(out, "data", "vat_treatment") != "reverse_charge" {
		t.Fatalf("tax-suggestion: %d %v", code, out)
	}

	// Correct → credit note + replacement draft.
	code, out = do(http.MethodPost, "/invoices/"+id+"/correct", map[string]any{"reason": "typo", "updated_at": updatedAt})
	if code != http.StatusCreated {
		t.Fatalf("correct: %d %v", code, out)
	}
	cnID := field(out, "data", "credit_note", "id").(string)
	if field(out, "data", "original", "status") != "corrected" || field(out, "data", "replacement", "status") != "draft" ||
		field(out, "data", "credit_note", "kind") != "credit_note" {
		t.Fatalf("correct result: %v", out)
	}
	if code, out := do(http.MethodPost, "/invoices/"+cnID+"/payments", `{"currency":"EUR","amount_minor":100,"kind":"payment"}`); code != http.StatusUnprocessableEntity || out["error"] != "invoice_not_payable" {
		t.Fatalf("pay credit note: %d %v", code, out)
	}
	if code, out := do(http.MethodPost, "/invoices/"+id+"/credit-note", `{"updated_at":"`+updatedAt+`"}`); code != http.StatusConflict || out["error"] != "conflict" {
		t.Fatalf("stale credit-note: %d %v", code, out)
	}

	code, out = do(http.MethodGet, "/invoices/summaries", nil)
	if code != http.StatusOK || field(out, "data", "EUR", "currency") != "EUR" {
		t.Fatalf("summaries: %d %v", code, out)
	}
	if code, _ := do(http.MethodGet, "/emails", nil); code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured email: %d, want 503", code)
	}
}
