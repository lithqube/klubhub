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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// These tests run the real repositories against the testcontainers
// Postgres from TestMain. The contract tests use in-memory fakes, which is
// how the nil-pointer Scan and the missing format_invoice_number() SQL
// function slipped through before.

func requirePG(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
}

// insertTestGig creates a gig with a 250.00 fee in currency.
func insertTestGig(t *testing.T, currency string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO gigs (date, venue, city, event_name, fee_amount, fee_currency)
		VALUES (now(), 'Tresor', 'Berlin', 'Night Shift', 250.00, $1)
		RETURNING id`, currency).Scan(&id)
	if err != nil {
		t.Fatalf("insert gig: %v", err)
	}
	return id
}

func newTestInvoiceService() *InvoiceService {
	return NewInvoiceService(NewInvoiceRepository(testPool), NewService(NewRepository(testPool)), NewPGGigFeeProvider(testPool))
}

func TestIntegration_InvoiceLifecycleAndPayments(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	gigID := insertTestGig(t, "EUR")
	invSvc := newTestInvoiceService()
	paySvc := NewPaymentService(NewPaymentRepository(testPool))

	// Currency must match the gig fee currency.
	if _, err := invSvc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID, Currency: "USD", NumberPrefix: "LIFE"}); !errors.Is(err, ErrInvoiceValidation) {
		t.Fatalf("mismatched currency: want ErrInvoiceValidation, got %v", err)
	}

	draft, err := invSvc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID, Currency: "EUR", NumberPrefix: "life"})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if draft.InvoiceNumber != "LIFE-0001-EUR" || draft.NumberPrefix != "LIFE" {
		t.Fatalf("number = %q prefix = %q", draft.InvoiceNumber, draft.NumberPrefix)
	}
	if draft.TotalMinor != 25000 {
		t.Fatalf("total = %d, want 25000", draft.TotalMinor)
	}
	_, lines, err := invSvc.GetByID(ctx, draft.ID)
	if err != nil || len(lines) != 1 {
		t.Fatalf("lines = %d err = %v", len(lines), err)
	}

	pay := func(amount int64, kind PaymentKind) (*Payment, error) {
		return paySvc.Create(ctx, draft.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: amount, Kind: kind})
	}

	// Drafts don't take payments.
	if _, err := pay(1000, PaymentKindDeposit); !errors.Is(err, ErrPaymentInvoiceState) {
		t.Fatalf("payment on draft: want ErrPaymentInvoiceState, got %v", err)
	}

	if _, err := invSvc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt}); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Wrong currency.
	if _, err := paySvc.Create(ctx, draft.ID, CreatePaymentRequest{Currency: "USD", AmountMinor: 100, Kind: PaymentKindPayment}); !errors.Is(err, ErrPaymentValidation) {
		t.Fatalf("wrong currency: want ErrPaymentValidation, got %v", err)
	}
	// Unknown invoice.
	if _, err := paySvc.Create(ctx, uuid.New(), CreatePaymentRequest{Currency: "EUR", AmountMinor: 100, Kind: PaymentKindPayment}); !errors.Is(err, ErrPaymentInvoiceNotFound) {
		t.Fatalf("unknown invoice: want ErrPaymentInvoiceNotFound, got %v", err)
	}

	deposit, err := pay(20000, PaymentKindDeposit)
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	// Pending deposit already counts toward the balance: 20000 + 6000 > 25000.
	if _, err := pay(6000, PaymentKindPayment); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("overpayment: want ErrPaymentExceedsBalance, got %v", err)
	}
	if _, err := pay(5000, PaymentKindPayment); err != nil {
		t.Fatalf("exact remainder: %v", err)
	}

	// A refund can't complete before money has been received…
	refund, err := pay(1000, PaymentKindRefund)
	if err != nil {
		t.Fatalf("create refund: %v", err)
	}
	if _, err := paySvc.Update(ctx, refund.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: refund.UpdatedAt}); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("refund before receipt: want ErrPaymentExceedsBalance, got %v", err)
	}
	// …but can once the deposit is completed.
	if _, err := paySvc.Update(ctx, deposit.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: deposit.UpdatedAt}); err != nil {
		t.Fatalf("complete deposit: %v", err)
	}
	if _, err := paySvc.Update(ctx, refund.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: refund.UpdatedAt}); err != nil {
		t.Fatalf("complete refund: %v", err)
	}
	// Invalid status is a validation error, not a 500 from the CHECK constraint.
	if _, err := paySvc.Update(ctx, deposit.ID, UpdatePaymentRequest{Status: "bogus", UpdatedAt: deposit.UpdatedAt}); !errors.Is(err, ErrPaymentValidation) {
		t.Fatalf("bogus status: want ErrPaymentValidation, got %v", err)
	}
	sum, err := paySvc.SumCompleted(ctx, draft.ID)
	if err != nil || sum != 19000 { // 20000 deposit − 1000 refund
		t.Fatalf("SumCompleted = %d err = %v", sum, err)
	}
}

func TestIntegration_ConcurrentDraftsGetDistinctNumbers(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	gigID := insertTestGig(t, "GBP")
	svc := newTestInvoiceService()

	const n = 12
	var wg sync.WaitGroup
	numbers := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inv, err := svc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID, Currency: "GBP", NumberPrefix: "RACE"})
			if err != nil {
				errs <- err
				return
			}
			numbers <- inv.InvoiceNumber
		}()
	}
	wg.Wait()
	close(numbers)
	close(errs)
	for err := range errs {
		t.Errorf("CreateDraft: %v", err)
	}
	seen := map[string]bool{}
	for num := range numbers {
		if seen[num] {
			t.Errorf("duplicate invoice number %s", num)
		}
		seen[num] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d distinct numbers, want %d", len(seen), n)
	}
}

func TestIntegration_CorrectIssuedInvoice(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	gigID := insertTestGig(t, "EUR")
	svc := newTestInvoiceService()

	draft, err := svc.CreateDraft(ctx, gigID, CreateInvoiceRequest{GigID: gigID, Currency: "EUR", NumberPrefix: "CORR"})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	// Drafts can't be corrected.
	if _, err := svc.Correct(ctx, draft.ID, CorrectInvoiceRequest{UpdatedAt: draft.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("correct draft: want ErrInvoiceBadState, got %v", err)
	}
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	correction, err := svc.Correct(ctx, issued.ID, CorrectInvoiceRequest{UpdatedAt: issued.UpdatedAt})
	if err != nil {
		t.Fatalf("Correct: %v", err)
	}
	if correction.InvoiceNumber != "CORR-0002-EUR" || correction.Status != InvoiceStatusDraft || correction.TotalMinor != issued.TotalMinor {
		t.Fatalf("correction = %+v", correction)
	}
	original, _, err := svc.GetByID(ctx, issued.ID)
	if err != nil || original.Status != InvoiceStatusCorrected {
		t.Fatalf("original status = %v err = %v", original.Status, err)
	}
	// Replaying the same correction is rejected.
	if _, err := svc.Correct(ctx, issued.ID, CorrectInvoiceRequest{UpdatedAt: issued.UpdatedAt}); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("replayed correction: want ErrInvoiceBadState, got %v", err)
	}
	if _, err := svc.Correct(ctx, uuid.New(), CorrectInvoiceRequest{}); !errors.Is(err, ErrInvoiceNotFound) {
		t.Fatalf("unknown invoice: want ErrInvoiceNotFound, got %v", err)
	}
}

// TestIntegration_HTTPThroughChiMount drives the real handlers through
// finance.Mux mounted the way apphttp.NewRouter mounts it, so path
// dispatch (including /invoices/{id}/payments → payments) is covered.
func TestIntegration_HTTPThroughChiMount(t *testing.T) {
	requirePG(t)
	gigID := insertTestGig(t, "EUR")
	invSvc := newTestInvoiceService()
	mux := NewMux(
		NewHandler(NewService(NewRepository(testPool))),
		NewInvoiceHandler(invSvc),
		NewPaymentHandler(NewPaymentService(NewPaymentRepository(testPool))),
		nil, nil, nil, nil,
	)
	router := chi.NewRouter()
	router.Mount("/api/v1/finance", mux)

	do := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	code, out := do(http.MethodPost, "/api/v1/finance/invoices",
		`{"gig_id":"`+gigID.String()+`","currency":"EUR","number_prefix":"HTTP"}`)
	if code != http.StatusCreated {
		t.Fatalf("create draft: %d %v", code, out)
	}
	inv := out["data"].(map[string]any)
	id, updatedAt := inv["id"].(string), inv["updated_at"].(string)

	if code, out := do(http.MethodPost, "/api/v1/finance/invoices/"+id+"/issue", `{"updated_at":"`+updatedAt+`"}`); code != http.StatusOK {
		t.Fatalf("issue: %d %v", code, out)
	}
	if code, out := do(http.MethodPost, "/api/v1/finance/invoices/"+id+"/payments",
		`{"currency":"EUR","amount_minor":99999,"kind":"payment"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("overpayment: %d %v", code, out)
	}
	if code, out := do(http.MethodPost, "/api/v1/finance/invoices/"+id+"/payments",
		`{"currency":"EUR","amount_minor":10000,"kind":"deposit"}`); code != http.StatusCreated {
		t.Fatalf("deposit: %d %v", code, out)
	}
	if code, out := do(http.MethodGet, "/api/v1/finance/invoices/"+id+"/payments", ""); code != http.StatusOK || len(out["data"].([]any)) != 1 {
		t.Fatalf("list payments: %d %v", code, out)
	}
	if code, _ := do(http.MethodGet, "/api/v1/finance/emails", ""); code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured email: %d, want 503", code)
	}
}
