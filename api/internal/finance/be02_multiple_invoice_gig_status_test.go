package finance

// BE-02 regression: per-invoice balance must not overwrite a settled gig
// when a sibling invoice receives only a pending deposit.
//
//   - Issue invoice A and fully collect it (gig becomes paid).
//   - Issue invoice B for the same gig; create a pending deposit on B.
//   - The gig must remain paid: another invoice's pending deposit
//     cannot reverse settled revenue.
//   - Completing the deposit on B later must NOT raise a
//     payment_reversed reconciliation prompt against the already-paid
//     gig A revenue.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestBE02_PendingDepositOnSiblingInvoiceKeepsPaidGig(t *testing.T) {
	requirePG(t)
	setBillingProfile(t, nil)
	ctx := context.Background()

	invSvc := newTestInvoiceService()
	r := NewPaymentRepositoryWithTransitionProcessor(testPool, NewGigPaymentTransitionProcessor(testPool))

	gigID := insertTestGig(t, "EUR")
	first := issuableDraft(t, invSvc, gigID, "BE02A", nil)
	first, err := invSvc.Issue(ctx, first.ID, IssueInvoiceRequest{UpdatedAt: first.UpdatedAt})
	if err != nil {
		t.Fatalf("issue A: %v", err)
	}
	// Collect first invoice fully (completed payment >= net_payable).
	collected := auditCreateCompletedPayment(t, r, first)
	if collected == nil {
		t.Fatalf("first invoice not collected")
	}
	// Gig is now paid.
	if st := readGigPaymentStatus(t, gigID); st != "paid" {
		t.Fatalf("expected gig paid after first collection, got %q", st)
	}

	// Issue a second invoice for the same gig.
	second := issuableDraft(t, invSvc, gigID, "BE02B", nil)
	second, err = invSvc.Issue(ctx, second.ID, IssueInvoiceRequest{UpdatedAt: second.UpdatedAt})
	if err != nil {
		t.Fatalf("issue B: %v", err)
	}
	// Pending deposit on second invoice (does NOT complete).
	dep, err := r.Create(ctx, second.ID, CreatePaymentRequest{
		Currency:    "EUR",
		AmountMinor: 100,
		Kind:        PaymentKindDeposit,
	})
	if err != nil {
		t.Fatalf("pending deposit on B: %v", err)
	}
	if dep.Status != PaymentStatusPending {
		t.Fatalf("expected pending status, got %q", dep.Status)
	}

	// The gig must STILL be paid — pending money on a sibling invoice
	// cannot reverse the settled revenue on the first invoice.
	if st := readGigPaymentStatus(t, gigID); st != "paid" {
		t.Fatalf("gig flipped to %q after pending deposit on second invoice; expected 'paid'", st)
	}

	// No payment_reversed reconciliation should exist for this gig —
	// the audit observed this is what surfaces a misleading "delete/void"
	// prompt to the UI even though no money was reversed.
	if rec := getPendingReconciliationByGig(t, gigID); rec != nil {
		t.Fatalf("unexpected pending reconciliation %+v; pending deposit must not raise one", rec)
	}

	// Completing the deposit on B is a payment write that re-runs the gig
	// aggregation. A is still fully collected, so the gig stays paid and
	// no payment_reversed prompt is raised.
	if _, err := r.Update(ctx, dep.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: dep.UpdatedAt}); err != nil {
		t.Fatalf("complete deposit on B: %v", err)
	}
	if st := readGigPaymentStatus(t, gigID); st != "paid" {
		t.Fatalf("gig flipped to %q after completing deposit on B; expected 'paid'", st)
	}
	if rec := getPendingReconciliationByGig(t, gigID); rec != nil {
		t.Fatalf("completing deposit on B raised reconciliation %+v; expected none", rec)
	}
	var reversed int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM finance_entry_reconciliations WHERE gig_id=$1`, gigID).Scan(&reversed); err != nil {
		t.Fatalf("count reconciliations: %v", err)
	}
	if reversed != 0 {
		t.Fatalf("reconciliation rows for gig = %d, want 0", reversed)
	}
}

func TestBE02_AllInvoicesFullyPaidStillMarksGigPaid(t *testing.T) {
	requirePG(t)
	setBillingProfile(t, nil)
	ctx := context.Background()

	invSvc := newTestInvoiceService()
	r := NewPaymentRepositoryWithTransitionProcessor(testPool, NewGigPaymentTransitionProcessor(testPool))

	gigID := insertTestGig(t, "EUR")
	first := issuableDraft(t, invSvc, gigID, "BE02A", nil)
	first, err := invSvc.Issue(ctx, first.ID, IssueInvoiceRequest{UpdatedAt: first.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if auditCreateCompletedPayment(t, r, first) == nil {
		t.Fatal("first collection failed")
	}

	second := issuableDraft(t, invSvc, gigID, "BE02B", nil)
	second, err = invSvc.Issue(ctx, second.ID, IssueInvoiceRequest{UpdatedAt: second.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if auditCreateCompletedPayment(t, r, second) == nil {
		t.Fatal("second collection failed")
	}

	if st := readGigPaymentStatus(t, gigID); st != "paid" {
		t.Fatalf("expected gig paid after both invoices fully collected, got %q", st)
	}
}

func TestBE02_AllowListSettledCreditNoteOnCollectedInvoice(t *testing.T) {
	// Guardrail: credit notes must NEVER contribute to gig payment
	// status (they are reversed sign and live against the original
	// invoice). A credited original keeps the gig paid — only the
	// original's collected cash defines authority, and credit notes
	// stay out of the aggregation.
	requirePG(t)
	setBillingProfile(t, nil)
	ctx := context.Background()

	invSvc := newTestInvoiceService()
	r := NewPaymentRepositoryWithTransitionProcessor(testPool, NewGigPaymentTransitionProcessor(testPool))

	gigID := insertTestGig(t, "EUR")
	inv := issuableDraft(t, invSvc, gigID, "BE02C", nil)
	inv, err := invSvc.Issue(ctx, inv.ID, IssueInvoiceRequest{UpdatedAt: inv.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if auditCreateCompletedPayment(t, r, inv) == nil {
		t.Fatal("collect invoice")
	}
	res, err := invSvc.CreditNote(ctx, inv.ID, CreditNoteRequest{UpdatedAt: inv.UpdatedAt, Reason: "test"})
	if err != nil {
		t.Fatalf("credit note: %v", err)
	}
	if res.CreditNote == nil {
		t.Fatal("no credit note returned")
	}
	// Gig stays paid — credit note does not flip the gig.
	if st := readGigPaymentStatus(t, gigID); st != "paid" {
		t.Fatalf("credit note flipped gig to %q; expected 'paid'", st)
	}
}

func TestBE02_RefundDoesNotDropPaidGigBackToUnpaid(t *testing.T) {
	// Guardrail: a completed refund against invoice A's collected cash
	// may drive received_minor below net payable for that invoice, but
	// the gig's authority derives from any fully-collected invoice.
	// The gig stays paid; refunds are about money returned, not gig
	// authority reversal.
	requirePG(t)
	setBillingProfile(t, nil)
	ctx := context.Background()

	invSvc := newTestInvoiceService()
	r := NewPaymentRepositoryWithTransitionProcessor(testPool, NewGigPaymentTransitionProcessor(testPool))

	gigID := insertTestGig(t, "EUR")
	inv := issuableDraft(t, invSvc, gigID, "BE02D", nil)
	inv, err := invSvc.Issue(ctx, inv.ID, IssueInvoiceRequest{UpdatedAt: inv.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if auditCreateCompletedPayment(t, r, inv) == nil {
		t.Fatal("collect invoice")
	}
	// Refund 1 minor — invoice's received drops below net but gig was paid
	// by the earlier completed payment, so the gig stays paid.
	if _, err := r.Create(ctx, inv.ID, CreatePaymentRequest{
		Currency: "EUR", AmountMinor: 1, Kind: PaymentKindRefund,
	}); err != nil {
		t.Fatal(err)
	}
	if st := readGigPaymentStatus(t, gigID); st != "paid" {
		t.Fatalf("gig flipped to %q after tiny refund; expected 'paid'", st)
	}
}

// auditCreateCompletedPayment creates and completes a payment covering
// the invoice's full net payable.
func auditCreateCompletedPayment(t *testing.T, r *PaymentRepository, inv *Invoice) *Payment {
	t.Helper()
	p, err := r.Create(context.Background(), inv.ID, CreatePaymentRequest{
		Currency: inv.Currency, AmountMinor: inv.NetPayableMinor, Kind: PaymentKindPayment,
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	p, err = r.Update(context.Background(), p.ID, UpdatePaymentRequest{
		Status: PaymentStatusCompleted, UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("complete payment: %v", err)
	}
	return p
}

// readGigPaymentStatus reads the live gig.payment_status from the DB.
func readGigPaymentStatus(t *testing.T, gigID uuid.UUID) string {
	t.Helper()
	var s string
	if err := testPool.QueryRow(context.Background(),
		`SELECT payment_status FROM gigs WHERE id=$1`, gigID).Scan(&s); err != nil {
		t.Fatalf("read gig status: %v", err)
	}
	return s
}

// getPendingReconciliationByGig returns the pending reconciliation row
// for a gig, or nil if none.
func getPendingReconciliationByGig(t *testing.T, gigID uuid.UUID) *EntryReconciliation {
	t.Helper()
	rec, err := NewEntryRepository(testPool).GetPendingReconciliationByGig(context.Background(), gigID)
	if err != nil {
		if err.Error() == "finance entry reconciliation not found" {
			return nil
		}
		t.Fatalf("GetPendingReconciliationByGig: %v", err)
	}
	return rec
}
