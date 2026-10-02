package finance

// BE-08 follow-ups (credited/corrected/cancelled originals):
//
//  1. Gig aggregation must keep counting settled receipts held by a
//     credited or corrected original. A later payment write on a sibling
//     invoice must not flip a settled gig down.
//  2. PaymentRepository.Update must not complete an incoming payment
//     (deposit/payment) once its invoice is credited, corrected or
//     cancelled; failing/releasing a pending payment stays allowed.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type be08Fixture struct {
	gigID uuid.UUID
	svc   *InvoiceService
	repo  *PaymentRepository
}

func be08Setup(t *testing.T) be08Fixture {
	t.Helper()
	requirePG(t)
	setBillingProfile(t, nil)
	return be08Fixture{
		gigID: insertTestGig(t, "EUR"),
		svc:   newTestInvoiceService(),
		repo:  NewPaymentRepositoryWithTransitionProcessor(testPool, NewGigPaymentTransitionProcessor(testPool)),
	}
}

func (f be08Fixture) issue(t *testing.T, prefix string) *Invoice {
	t.Helper()
	d := issuableDraft(t, f.svc, f.gigID, prefix, nil)
	inv, err := f.svc.Issue(context.Background(), d.ID, IssueInvoiceRequest{UpdatedAt: d.UpdatedAt})
	if err != nil {
		t.Fatalf("issue %s: %v", prefix, err)
	}
	return inv
}

// retire credits or corrects inv and returns the retired original.
func (f be08Fixture) retire(t *testing.T, inv *Invoice, correct bool) *Invoice {
	t.Helper()
	ctx := context.Background()
	var (
		res *CreditNoteResult
		err error
	)
	if correct {
		res, err = f.svc.Correct(ctx, inv.ID, CorrectInvoiceRequest{Reason: "be08", UpdatedAt: inv.UpdatedAt})
	} else {
		res, err = f.svc.CreditNote(ctx, inv.ID, CreditNoteRequest{Reason: "be08", UpdatedAt: inv.UpdatedAt})
	}
	if err != nil {
		t.Fatalf("retire (correct=%v): %v", correct, err)
	}
	return res.Original
}

func TestBE08_RetiredOriginalReceiptsKeepGigPaidOnSiblingWrite(t *testing.T) {
	for _, correct := range []bool{false, true} {
		name := "credited"
		if correct {
			name = "corrected"
		}
		t.Run(name, func(t *testing.T) {
			f := be08Setup(t)
			ctx := context.Background()
			a := f.issue(t, "BE08A")
			auditCreateCompletedPayment(t, f.repo, a)
			if st := readGigPaymentStatus(t, f.gigID); st != "paid" {
				t.Fatalf("gig after collecting A = %q, want paid", st)
			}
			orig := f.retire(t, a, correct)
			wantStatus := InvoiceStatusCredited
			if correct {
				wantStatus = InvoiceStatusCorrected
			}
			if orig.Status != wantStatus {
				t.Fatalf("original status = %s, want %s", orig.Status, wantStatus)
			}

			// Sibling invoice B receives and completes a small deposit:
			// this triggers the gig aggregation again.
			b := f.issue(t, "BE08B")
			dep, err := f.repo.Create(ctx, b.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 100, Kind: PaymentKindDeposit})
			if err != nil {
				t.Fatalf("deposit on B: %v", err)
			}
			if _, err := f.repo.Update(ctx, dep.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: dep.UpdatedAt}); err != nil {
				t.Fatalf("complete deposit on B: %v", err)
			}
			if st := readGigPaymentStatus(t, f.gigID); st != "paid" {
				t.Fatalf("settled receipts on %s original dropped out of gig aggregation: gig = %q, want paid", name, st)
			}
		})
	}
}

// Guardrail: once the cash is fully returned, the retired original no
// longer supports a paid gig.
func TestBE08_FullRefundOnCreditedOriginalReleasesGig(t *testing.T) {
	f := be08Setup(t)
	ctx := context.Background()
	a := f.issue(t, "BE08R")
	auditCreateCompletedPayment(t, f.repo, a)
	orig := f.retire(t, a, false)
	ref, err := f.repo.Create(ctx, orig.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: a.NetPayableMinor, Kind: PaymentKindRefund})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if _, err := f.repo.Update(ctx, ref.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: ref.UpdatedAt}); err != nil {
		t.Fatalf("complete refund: %v", err)
	}
	if st := readGigPaymentStatus(t, f.gigID); st != "unpaid" {
		t.Fatalf("gig after full refund = %q, want unpaid", st)
	}
}

func TestBE08_PendingIncomingCannotCompleteAfterInvoiceRetired(t *testing.T) {
	for _, tc := range []struct {
		name    string
		correct bool
	}{{"credited", false}, {"corrected", true}} {
		for _, kind := range []PaymentKind{PaymentKindPayment, PaymentKindDeposit} {
			t.Run(tc.name+"/"+string(kind), func(t *testing.T) {
				f := be08Setup(t)
				ctx := context.Background()
				inv := f.issue(t, "BE08C")
				p, err := f.repo.Create(ctx, inv.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1000, Kind: kind})
				if err != nil {
					t.Fatalf("pending %s: %v", kind, err)
				}
				f.retire(t, inv, tc.correct)

				_, err = f.repo.Update(ctx, p.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: p.UpdatedAt})
				if !errors.Is(err, ErrPaymentKindNotAllowed) {
					t.Fatalf("complete incoming on %s invoice: err = %v, want ErrPaymentKindNotAllowed", tc.name, err)
				}
				got, err := f.repo.GetByID(ctx, p.ID)
				if err != nil || got.Status != PaymentStatusPending {
					t.Fatalf("payment after rejected completion = %+v err=%v, want pending", got, err)
				}
				if st := readGigPaymentStatus(t, f.gigID); st != "unpaid" {
					t.Fatalf("gig = %q, want unpaid (no cash minted)", st)
				}
				// Releasing the pending payment stays possible.
				if _, err := f.repo.Update(ctx, p.ID, UpdatePaymentRequest{Status: PaymentStatusFailed, UpdatedAt: p.UpdatedAt}); err != nil {
					t.Fatalf("failing pending incoming on %s invoice must stay allowed: %v", tc.name, err)
				}
			})
		}
	}
}

func TestBE08_PendingIncomingCannotCompleteOnCancelledInvoice(t *testing.T) {
	f := be08Setup(t)
	ctx := context.Background()
	inv := f.issue(t, "BE08X")
	p, err := f.repo.Create(ctx, inv.ID, CreatePaymentRequest{Currency: "EUR", AmountMinor: 1000, Kind: PaymentKindPayment})
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	// Cancel is draft-only in the service; force the state directly to
	// prove the repository itself is closed to it.
	if _, err := testPool.Exec(ctx, `UPDATE invoices SET status='cancelled' WHERE id=$1`, inv.ID); err != nil {
		t.Fatalf("force cancelled: %v", err)
	}
	_, err = f.repo.Update(ctx, p.ID, UpdatePaymentRequest{Status: PaymentStatusCompleted, UpdatedAt: p.UpdatedAt})
	if !errors.Is(err, ErrPaymentInvoiceState) {
		t.Fatalf("complete on cancelled invoice: err = %v, want ErrPaymentInvoiceState", err)
	}
	got, _ := f.repo.GetByID(ctx, p.ID)
	if got == nil || got.Status != PaymentStatusPending {
		t.Fatalf("payment after rejection = %+v, want pending", got)
	}
}
