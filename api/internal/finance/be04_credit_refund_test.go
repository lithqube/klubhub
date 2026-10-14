package finance

// BE-04 regression: PaymentRepository.Create must accept a bounded refund
// on a credited or corrected invoice (returning previously-collected cash)
// while still rejecting new incoming payments (deposit/payment) on the
// same documents. Credit notes themselves never accept any payment.

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBE04_RefundAllowedOnCreditedInvoice(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	payRepo := NewPaymentRepository(testPool)
	paySvc := NewPaymentService(payRepo)

	gigID := insertTestGig(t, "EUR")
	draft := issuableDraft(t, svc, gigID, "BE0401", nil)
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	// Collect 250.00 in full, then mark the invoice paid so we can credit it.
	dep, err := paySvc.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 25000, Kind: PaymentKindPayment})
	if err != nil {
		t.Fatalf("collect payment: %v", err)
	}
	if _, err := paySvc.Update(ctx, dep.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: dep.UpdatedAt}); err != nil {
		t.Fatalf("complete payment: %v", err)
	}
	got, err := svc.Pay(ctx, issued.ID, PayInvoiceRequest{PaidAt: time.Now(), UpdatedAt: issued.UpdatedAt})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if got.Status != InvoiceStatusPaid {
		t.Fatalf("expected paid, got %s", got.Status)
	}
	res, err := svc.CreditNote(ctx, got.ID, CreditNoteRequest{Reason: "refund in full", UpdatedAt: got.UpdatedAt})
	if err != nil {
		t.Fatalf("CreditNote: %v", err)
	}

	// Credit note itself accepts no payment.
	if _, err := paySvc.Create(ctx, res.CreditNote.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 100, Kind: PaymentKindRefund}); !errors.Is(err, ErrPaymentInvoiceState) {
		t.Fatalf("payment on credit note: %v", err)
	}
	// Credited invoice rejects new incoming payments but accepts bounded refunds.
	if _, err := paySvc.Create(ctx, res.Original.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 100, Kind: PaymentKindPayment}); !errors.Is(err, ErrPaymentKindNotAllowed) {
		t.Fatalf("deposit on credited invoice: want ErrPaymentKindNotAllowed, got %v", err)
	}
	if _, err := paySvc.Create(ctx, res.Original.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 100, Kind: PaymentKindDeposit}); !errors.Is(err, ErrPaymentKindNotAllowed) {
		t.Fatalf("payment-kind on credited invoice: want ErrPaymentKindNotAllowed, got %v", err)
	}
	refund, err := paySvc.Create(ctx, res.Original.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 5000, Kind: PaymentKindRefund})
	if err != nil {
		t.Fatalf("refund on credited invoice: %v", err)
	}
	if refund.Kind != PaymentKindRefund || refund.InvoiceID != res.Original.ID {
		t.Fatalf("refund record = %+v", refund)
	}
	if _, err := paySvc.Update(ctx, refund.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: refund.UpdatedAt}); err != nil {
		t.Fatalf("complete refund: %v", err)
	}
}

func TestBE04_RefundAllowedOnCorrectedInvoice(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	paySvc := NewPaymentService(NewPaymentRepository(testPool))

	gigID := insertTestGig(t, "EUR")
	draft := issuableDraft(t, svc, gigID, "BE0402", nil)
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	// Collect a partial amount via a real payment record so refunds can be
	// bounded against actual receipts (the manual svc.Pay path marks the
	// invoice paid without writing payments rows; the bounded-refund check
	// reads from the payments table by design).
	dep, err := paySvc.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 5000, Kind: PaymentKindPayment})
	if err != nil {
		t.Fatalf("partial collect: %v", err)
	}
	if _, err := paySvc.Update(ctx, dep.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: dep.UpdatedAt}); err != nil {
		t.Fatalf("complete payment: %v", err)
	}
	paid, err := svc.Pay(ctx, issued.ID, PayInvoiceRequest{PaidAt: time.Now(), UpdatedAt: issued.UpdatedAt})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	corr, err := svc.Correct(ctx, paid.ID, CorrectInvoiceRequest{Reason: "wrong address", UpdatedAt: paid.UpdatedAt})
	if err != nil {
		t.Fatalf("Correct: %v", err)
	}

	if _, err := paySvc.Create(ctx, corr.Original.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 100, Kind: PaymentKindPayment}); !errors.Is(err, ErrPaymentKindNotAllowed) {
		t.Fatalf("payment-kind on corrected invoice: want ErrPaymentKindNotAllowed, got %v", err)
	}
	// Refund up to the receipts is allowed; over the receipts is not.
	refund, err := paySvc.Create(ctx, corr.Original.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1000, Kind: PaymentKindRefund})
	if err != nil {
		t.Fatalf("refund on corrected invoice: %v", err)
	}
	if refund.AmountMinor != 1000 {
		t.Fatalf("refund amount = %d", refund.AmountMinor)
	}
	if _, err := paySvc.Create(ctx, corr.Original.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 10000, Kind: PaymentKindRefund}); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("over-refund: want ErrPaymentExceedsBalance, got %v", err)
	}
}

func TestBE04_RefundOnCreditedInvoiceStillBound(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	paySvc := NewPaymentService(NewPaymentRepository(testPool))

	gigID := insertTestGig(t, "EUR")
	draft := issuableDraft(t, svc, gigID, "BE0403", nil)
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	// Collect only 100.00 of 250.00.
	dep, err := paySvc.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 10000, Kind: PaymentKindPayment})
	if err != nil {
		t.Fatalf("partial collect: %v", err)
	}
	if _, err := paySvc.Update(ctx, dep.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: dep.UpdatedAt}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := svc.Pay(ctx, issued.ID, PayInvoiceRequest{PaidAt: time.Now(), UpdatedAt: issued.UpdatedAt})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	res, err := svc.CreditNote(ctx, got.ID, CreditNoteRequest{Reason: "underpayment", UpdatedAt: got.UpdatedAt})
	if err != nil {
		t.Fatalf("CreditNote: %v", err)
	}
	// Trying to refund more than was collected must still be rejected.
	if _, err := paySvc.Create(ctx, res.Original.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 10001, Kind: PaymentKindRefund}); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("over-refund: want ErrPaymentExceedsBalance, got %v", err)
	}
}

// TestBE04_CreditNoteNeverAcceptsAnyPayment keeps the invariant that
// credit notes are immutable money documents — they are issued only to
// reverse an existing invoice, never to record an external movement.
func TestBE04_CreditNoteNeverAcceptsAnyPayment(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	paySvc := NewPaymentService(NewPaymentRepository(testPool))

	gigID := insertTestGig(t, "EUR")
	draft := issuableDraft(t, svc, gigID, "BE0404", nil)
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	res, err := svc.CreditNote(ctx, issued.ID, CreditNoteRequest{Reason: "no sale", UpdatedAt: issued.UpdatedAt})
	if err != nil {
		t.Fatalf("CreditNote: %v", err)
	}
	for _, k := range []PaymentKind{PaymentKindDeposit, PaymentKindPayment, PaymentKindRefund} {
		if _, err := paySvc.Create(ctx, res.CreditNote.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 100, Kind: k}); !errors.Is(err, ErrPaymentInvoiceState) {
			t.Fatalf("credit note kind=%s: want ErrPaymentInvoiceState, got %v", k, err)
		}
	}
}
