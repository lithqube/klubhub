package finance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/gig"
)

// Hold an uncommitted unique-index winner, prove the contender is blocked
// by that transaction in PostgreSQL, then commit the winner. No timing-only
// sleep trigger or mocked database can substitute for this contention.
func TestBE07_GeneratedIncomeRejectsDifferentMetadata(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	id := insertTestGig(t, "EUR")
	snap := gig.PaymentSnapshot{GigID: id, Date: time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC), EventName: "BE07 metadata"}
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	e, err := createGeneratedGigIncome(ctx, tx, snap, 25000, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE finance_entries SET notes='different payload' WHERE id=$1`, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := createGeneratedGigIncome(ctx, tx, snap, 25000, "EUR"); !errors.Is(err, ErrGeneratedEntryConflict) {
		t.Fatalf("different metadata: want conflict, got %v", err)
	}
}

func TestBE07_GeneratedIncomeConcurrentInsert(t *testing.T) {
	requirePG(t)
	for _, unequal := range []bool{false, true} {
		name := "equal"
		if unequal {
			name = "unequal"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			id := insertTestGig(t, "EUR")
			snap := gig.PaymentSnapshot{GigID: id, Date: time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC), EventName: "BE07"}
			winner, err := testPool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer winner.Rollback(context.Background())
			first, err := createGeneratedGigIncome(ctx, winner, snap, 25000, "EUR")
			if err != nil {
				t.Fatal(err)
			}
			contender, err := testPool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer contender.Rollback(context.Background())
			var pid int
			if err := contender.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			amount := int64(25000)
			if unequal {
				amount++
			}
			type result struct {
				entry *Entry
				err   error
			}
			done := make(chan result, 1)
			go func() {
				e, err := createGeneratedGigIncome(ctx, contender, snap, amount, "EUR")
				done <- result{e, err}
			}()
			deadline := time.Now().Add(5 * time.Second)
			blocked := false
			for time.Now().Before(deadline) {
				if err := testPool.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1)) > 0`, pid).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !blocked {
				t.Fatal("contender never blocked on uncommitted unique-index winner")
			}
			if err := winner.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			r := <-done
			if unequal {
				if !errors.Is(r.err, ErrGeneratedEntryConflict) {
					t.Fatalf("unequal payload: want conflict, got %v", r.err)
				}
			} else if r.err != nil || r.entry == nil || r.entry.ID != first.ID {
				t.Fatalf("equal payload: want winner %s, got %+v err=%v", first.ID, r.entry, r.err)
			}
			// Both outcomes leave the caller transaction usable, not SQLSTATE 25P02.
			var one int
			if err := contender.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
				t.Fatalf("transaction aborted: %v", err)
			}
			if err := contender.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := testPool.QueryRow(ctx, `SELECT count(*) FROM finance_entries WHERE source_id=$1 AND status='active' AND deleted_at IS NULL`, id).Scan(&count); err != nil || count != 1 {
				t.Fatalf("count=%d err=%v", count, err)
			}
			persisted, err := NewEntryRepository(testPool).GetByGeneratedSource(ctx, EntrySourceGigPayment, id, EntryKindIncome)
			if err != nil || persisted.ID != first.ID || persisted.AmountMinor != 25000 {
				t.Fatalf("winner changed: %+v err=%v", persisted, err)
			}
		})
	}
}
