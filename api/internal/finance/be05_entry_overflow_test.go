package finance

// BE-05: individual writes and exact PostgreSQL numeric aggregates must
// remain representable by JSON Number clients. Unsafe writes are rejected;
// unsafe reports fail deliberately, without truncated or partial totals.

import (
	"context"
	"errors"
	"testing"
)

// TestBE05_CreateEntryRejectsNearMaxInt64 verifies the per-row cap is
// enforced by entry validation before the row touches the database.
func TestBE05_CreateEntryRejectsNearMaxInt64(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	svc := NewEntryService(NewEntryRepository(testPool))

	for _, amt := range []int64{MaxEntryAmountMinor + 1, 1 << 62, 1<<62 + 12345, 9_000_000_000_000_000_000} {
		_, err := svc.Create(ctx, CreateEntryRequest{
			Kind: EntryKindIncome, AmountMinor: amt, Currency: "EUR",
			Category: "overflow", EntryDate: "2026-10-03",
			Description: "near-max", Notes: "",
		})
		if !errors.Is(err, ErrEntryValidation) {
			t.Fatalf("amount=%d: want ErrEntryValidation, got %v", amt, err)
		}
	}
}

// TestBE05_CreateEntryAcceptsMaxBoundary verifies the boundary itself
// (MaxEntryAmountMinor) is accepted: not so tight it makes the contract
// impractical.
func TestBE05_CreateEntryAcceptsMaxBoundary(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	svc := NewEntryService(NewEntryRepository(testPool))

	e, err := svc.Create(ctx, CreateEntryRequest{
		Kind: EntryKindIncome, AmountMinor: MaxEntryAmountMinor, Currency: "USD",
		Category: "ceiling", EntryDate: "2026-10-03",
		Description: "max allowed", Notes: "",
	})
	if err != nil {
		t.Fatalf("Create at max: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM finance_entries WHERE id=$1`, e.ID) })
	if e.AmountMinor != MaxEntryAmountMinor {
		t.Fatalf("stored amount = %d", e.AmountMinor)
	}
}

// Individually valid writes can collectively exceed the safe wire range.
// They remain valid ledger records; the report request is rejected.
func TestBE05_SummaryAggregateOverflow(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)
	svc := NewEntryService(repo)
	currency := "DKK"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM finance_entries WHERE category='agg-overflow' AND currency='DKK'`)
	})

	for i := 0; i < 12; i++ {
		_, err := svc.Create(ctx, CreateEntryRequest{
			Kind: EntryKindIncome, AmountMinor: MaxEntryAmountMinor, Currency: currency,
			Category: "agg-overflow", EntryDate: "2026-10-03",
			Description: "row", Notes: "",
		})
		if err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}

	_, err := repo.Summary(ctx, DateRange{From: "2026-10-01", To: "2026-11-01"}, nil)
	if !errors.Is(err, ErrEntryAggregateOverflow) {
		t.Fatalf("want ErrEntryAggregateOverflow, got %v", err)
	}
}

// TestBE05_SummaryWithinDocumentSafeRange: the happy path still succeeds
// well within the documented bounds. Uses an isolated currency and a
// narrow date range so other tests' overflow data does not bleed in.
func TestBE05_SummaryWithinDocumentSafeRange(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	svc := NewEntryService(NewEntryRepository(testPool))
	currency := "ZAR"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM finance_entries WHERE currency='ZAR' AND entry_date >= '2099-01-01' AND entry_date < '2099-02-01'`)
	})

	if _, err := svc.Create(ctx, CreateEntryRequest{
		Kind: EntryKindIncome, AmountMinor: 200_000_000_000, Currency: currency,
		Category: "normal", EntryDate: "2099-01-15",
		Description: "first", Notes: "",
	}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.Create(ctx, CreateEntryRequest{
		Kind: EntryKindExpense, AmountMinor: 100_000_000_000, Currency: currency,
		Category: "normal", EntryDate: "2099-01-16",
		Description: "second", Notes: "second",
	}); err != nil {
		t.Fatalf("second: %v", err)
	}
	sums, err := svc.Summary(ctx, DateRange{From: "2099-01-01", To: "2099-02-01"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	tot, ok := sums[currency]
	if !ok {
		t.Fatalf("missing %s summary: %+v", currency, sums)
	}
	if tot.IncomeMinor != 200_000_000_000 || tot.ExpenseMinor != 100_000_000_000 {
		t.Fatalf("%s summary = %+v", currency, tot)
	}
}
