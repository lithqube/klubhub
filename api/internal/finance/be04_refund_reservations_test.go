package finance

import (
	"context"
	"errors"
	"testing"
)

func TestBE04_CancelledInvoiceRejectsPayments(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	draft := issuableDraft(t, svc, insertTestGig(t, "EUR"), "BE04CANCEL", nil)
	cancelled, err := svc.Cancel(ctx, draft.ID, CancelInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	pay := NewPaymentService(NewPaymentRepository(testPool))
	for _, kind := range []PaymentKind{PaymentKindDeposit, PaymentKindPayment, PaymentKindRefund} {
		if _, err := pay.Create(ctx, cancelled.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1, Kind: kind}); !errors.Is(err, ErrPaymentInvoiceState) {
			t.Errorf("cancelled %s: want invoice state rejection, got %v", kind, err)
		}
	}
}

func TestBE04_PendingRefundDoesNotFreeIncomingCapacity(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	draft := issuableDraft(t, svc, insertTestGig(t, "EUR"), "BE04CAP", nil)
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	pay := NewPaymentService(NewPaymentRepository(testPool))
	receipt, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: issued.NetPayableMinor, Kind: PaymentKindPayment})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pay.Update(ctx, receipt.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: receipt.UpdatedAt}); err != nil {
		t.Fatal(err)
	}
	refund, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1000, Kind: PaymentKindRefund})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1000, Kind: PaymentKindDeposit}); !errors.Is(err, ErrPaymentExceedsBalance) {
		t.Fatalf("pending refund must not free collection capacity: %v", err)
	}
	if _, err := pay.Update(ctx, refund.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: refund.UpdatedAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1000, Kind: PaymentKindDeposit}); err != nil {
		t.Fatalf("completed refund should free collection capacity: %v", err)
	}
}

func TestBE04_RefundReservationsCreditedAndCorrected(t *testing.T) {
	requirePG(t)
	for _, correct := range []bool{false, true} {
		name := "credited"
		prefix := "BE04CRED"
		if correct {
			name = "corrected"
			prefix = "BE04CORR"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			setBillingProfile(t, nil)
			svc := newTestInvoiceService()
			draft := issuableDraft(t, svc, insertTestGig(t, "EUR"), prefix, nil)
			issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: draft.UpdatedAt})
			if err != nil {
				t.Fatal(err)
			}
			pay := NewPaymentService(NewPaymentRepository(testPool))
			receipt, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 10000, Kind: PaymentKindPayment})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pay.Update(ctx, receipt.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: receipt.UpdatedAt}); err != nil {
				t.Fatal(err)
			}
			if correct {
				_, err = svc.Correct(ctx, issued.ID, CorrectInvoiceRequest{Reason: "correct", UpdatedAt: issued.UpdatedAt})
			} else {
				_, err = svc.CreditNote(ctx, issued.ID, CreditNoteRequest{Reason: "credit", UpdatedAt: issued.UpdatedAt})
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []PaymentKind{PaymentKindDeposit, PaymentKindPayment} {
				if _, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1, Kind: kind}); !errors.Is(err, ErrPaymentKindNotAllowed) {
					t.Fatalf("incoming %s: %v", kind, err)
				}
			}
			a, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 4000, Kind: PaymentKindRefund})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pay.Update(ctx, a.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: a.UpdatedAt}); err != nil {
				t.Fatal(err)
			}
			b, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 6000, Kind: PaymentKindRefund})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1, Kind: PaymentKindRefund}); !errors.Is(err, ErrPaymentExceedsBalance) {
				t.Fatalf("completed+pending refunds exceed receipts: %v", err)
			}
			if _, err := pay.Update(ctx, b.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: b.UpdatedAt}); err != nil {
				t.Fatal(err)
			}
			if _, err := pay.Create(ctx, issued.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1, Kind: PaymentKindRefund}); !errors.Is(err, ErrPaymentExceedsBalance) {
				t.Fatalf("completed refunds exceed receipts: %v", err)
			}
		})
	}
}
