package finance

// BE-03 regression: payment completion must not consume a stale unlocked
// gig snapshot. The audit reproduced this with: an external writer
// holding the gig row lock while fee=250; the payment completion
// path reads fee=250, queues an UPDATE on gigs (blocked), the external
// writer commits fee=500; the payment commits with the stale snapshot
// and the generated income is wrong with no reconciliation raised.
//
//   - Lock the gig externally (FOR UPDATE).
//   - Concurrently, start payment completion and observe it blocked on
//     the UPDATE gigs SET payment_status lock.
//   - Commit fee=500 externally.
//   - After unblock, the generated income must reflect fee=500
//     (or a fee_changed reconciliation must be raised), not fee=250
//     silently.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBE03_PaymentCompletionReReadsLockedGigFee(t *testing.T) {
	requirePG(t)
	setBillingProfile(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	invSvc := newTestInvoiceService()
	r := NewPaymentRepositoryWithTransitionProcessor(testPool, NewGigPaymentTransitionProcessor(testPool))

	gigID := insertTestGig(t, "EUR")
	inv := issuableDraft(t, invSvc, gigID, "BE03", nil)
	inv, err := invSvc.Issue(ctx, inv.ID, IssueInvoiceRequest{UpdatedAt: inv.UpdatedAt})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	// Stage 1: external writer acquires the gig row lock.
	extTx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin external tx: %v", err)
	}
	defer extTx.Rollback(context.Background())
	var ignored uuid.UUID
	if err := extTx.QueryRow(ctx,
		`SELECT id FROM gigs WHERE id=$1 FOR UPDATE`, gigID).Scan(&ignored); err != nil {
		t.Fatalf("lock gig: %v", err)
	}

	// Stage 2: payment completion starts. Create a pending payment and
	// mark it completed in a separate transaction that will queue on
	// the gig lock via the transition processor.
	done := make(chan error, 1)
	var completedFee int64
	go func() {
		// We must run within a context that won't be cancelled mid-flight.
		gctx, gcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer gcancel()
		p, err := r.Create(gctx, inv.ID, CreatePaymentRequest{
			Currency: inv.Currency, AmountMinor: inv.NetPayableMinor, Kind: PaymentKindPayment,
		})
		if err != nil {
			done <- err
			return
		}
		// Update may block on gig lock; the gig is held externally until
		// we observe the block in the main goroutine.
		if _, err := r.Update(gctx, p.ID, UpdatePaymentRequest{
			Status: PaymentStatusCompleted, UpdatedAt: p.UpdatedAt,
		}); err != nil {
			done <- err
			return
		}
		// Read back the generated income to see what fee the processor used.
		var amount int64
		err = testPool.QueryRow(gctx, `
			SELECT amount_minor FROM finance_entries
			WHERE gig_id=$1 AND auto_generated=true AND source_kind='gig_payment'
			  AND status='active' AND deleted_at IS NULL`, gigID).Scan(&amount)
		if err != nil {
			done <- err
			return
		}
		completedFee = amount
		done <- nil
	}()

	// Stage 3: wait until the goroutine is blocked on UPDATE gigs.
	be03WaitForBlockedOnGig(t)

	// Stage 4: external writer commits fee=500.
	if _, err := extTx.Exec(ctx,
		`UPDATE gigs SET fee_amount=500.00, updated_at=clock_timestamp() WHERE id=$1`, gigID); err != nil {
		t.Fatalf("external fee update: %v", err)
	}
	if err := extTx.Commit(ctx); err != nil {
		t.Fatalf("external commit: %v", err)
	}

	// Stage 5: payment goroutine should unblock and complete.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("payment goroutine: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("payment did not complete: %v", ctx.Err())
	}

	// Stage 6: the stored fee is 500; the generated income must reflect
	// 50000 (or raise a fee_changed reconciliation). It must NOT be 25000
	// with no reconciliation, which is the audit's observed failure.
	var feeText string
	if err := testPool.QueryRow(ctx,
		`SELECT fee_amount::text FROM gigs WHERE id=$1`, gigID).Scan(&feeText); err != nil {
		t.Fatalf("read fee: %v", err)
	}
	if feeText != "500.00" {
		t.Fatalf("external fee update lost, fee=%s", feeText)
	}
	rec := getPendingReconciliationByGig(t, gigID)
	t.Logf("observed outcome: generated income=%d reconciliation=%+v", completedFee, rec)
	if completedFee != 50000 || rec != nil {
		t.Fatalf("generated income=%d rec=%+v; want income recomputed from locked fee=500 (50000) and no reconciliation",
			completedFee, rec)
	}
}

// be03WaitForBlockedOnGig polls pg_stat_activity for a backend that is
// waiting on a Lock wait_event with a query containing
// "gigs" — i.e. the payment completion path has reached the gig
// lock or the subsequent UPDATE gigs SET payment_status step. This
// detects both the SELECT FOR NO KEY UPDATE of g (taken by the
// payment-sync path) and the implicit FOR UPDATE via the UPDATE
// statement — either blocks the completion behind the external
// FOR UPDATE writer.
func be03WaitForBlockedOnGig(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := testPool.Query(context.Background(), `
			SELECT pid, wait_event, substring(query for 200), state
			FROM pg_stat_activity
			WHERE datname=current_database()
			  AND wait_event_type='Lock'
			  AND query ILIKE '%gigs%'
			  AND pid <> pg_backend_pid()`)
		if err == nil {
			var pid int64
			var waitEvent *string
			var q *string
			var state *string
			if rows.Next() {
				if err := rows.Scan(&pid, &waitEvent, &q, &state); err == nil {
					t.Logf("observed blocked waiter pid=%d wait_event=%v state=%v query=%q", pid, derefStr(waitEvent), derefStr(state), derefStr(q))
					rows.Close()
					return
				}
			}
			rows.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("payment completion never reached a gig lock during its update path")
}

func derefStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
