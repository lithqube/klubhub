package finance

// BE-06 regression: GetByGeneratedSource uses the
// (source_kind, source_id, kind) WHERE clause only — it does not require
// status='active' AND deleted_at IS NULL. Once a generated row is voided
// or soft-deleted, a replacement INSERT can leave the slot visible in
// this lookup pointing at the old inactive row. Callers that consult
// this method to short-circuit a retry (CreateGenerated) end up comparing
// the wrong row.
//
// The production gig-payment transition processor uses its own active
// lookup and is unaffected, but the generic entry-level helper is
// reachable from admin tooling and any future code path that mirrors
// the processor. The fix is an explicit predicate plus most-recent-first
// ordering so the caller deterministically sees the active row.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestBE06_GetByGeneratedSourceReturnsActiveReplacement is the canonical
// reproduction: void the original, insert a fresh active row for the same
// source directly in the DB (bypassing CreateGenerated's precheck that
// guards against this very situation), and confirm the helper returns the
// active row, not the voided one.
func TestBE06_GetByGeneratedSourceReturnsActiveReplacement(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)

	gigID := insertTestGig(t, "EUR")
	sourceID := uuid.New()

	// Seed an original active generated row.
	first, err := repo.CreateGenerated(ctx, GeneratedEntryInput{
		Kind: EntryKindIncome, AmountMinor: 25000, Currency: "EUR",
		Category: "gig_fee", EntryDate: "2026-10-03",
		Description: "DJ performance – Night Shift @ Tresor",
		GigID:       &gigID,
		SourceKind:  EntrySourceGigPayment, SourceID: sourceID,
		SourceAmountMinor: intPtr(25000), SourceCurrency: strPtr("EUR"),
		SourceDescription: "gig:" + gigID.String(),
	})
	if err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`UPDATE finance_entries SET status='voided', updated_at=clock_timestamp() WHERE id=$1`,
		first.ID); err != nil {
		t.Fatalf("void: %v", err)
	}

	// Insert a fresh active row directly (bypassing CreateGenerated's
	// precheck, which is the path the bug analysis flagged). The
	// partial unique index permits this because it excludes voided rows.
	var secondID uuid.UUID
	if err := testPool.QueryRow(ctx, `
		INSERT INTO finance_entries
		(kind, amount_minor, currency, category, entry_date, description, notes, gig_id,
		 source_amount_minor, source_currency, source_description,
		 auto_generated, source_kind, source_id)
		VALUES ('income', 25000, $3, 'gig_fee', '2026-10-03'::date,
		        'DJ performance – Night Shift @ Tresor', '', $1::uuid,
		        25000, 'EUR', 'gig:' || $1::text,
		        true, 'gig_payment', $2::uuid)
		RETURNING id`, gigID, sourceID, "EUR").Scan(&secondID); err != nil {
		t.Fatalf("insert replacement: %v", err)
	}

	found, err := repo.GetByGeneratedSource(ctx, EntrySourceGigPayment, sourceID, EntryKindIncome)
	if err != nil {
		t.Fatalf("GetByGeneratedSource: %v", err)
	}
	if found.ID != secondID {
		t.Fatalf("lookup id = %v, want %v (active replacement)", found.ID, secondID)
	}
	if found.Status != EntryStatusActive || found.DeletedAt != nil {
		t.Fatalf("lookup returned inactive row: status=%s deleted_at=%v", found.Status, found.DeletedAt)
	}
}

// TestBE06_GetByGeneratedSourceIgnoresDeleted also checks the
// soft-deleted branch: a soft-deleted row must not surface.
func TestBE06_GetByGeneratedSourceIgnoresDeleted(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)

	gigID := insertTestGig(t, "USD")
	sourceID := uuid.New()

	created, err := repo.CreateGenerated(ctx, GeneratedEntryInput{
		Kind: EntryKindIncome, AmountMinor: 10000, Currency: "USD",
		Category: "gig_fee", EntryDate: "2026-10-03",
		Description: "DJ performance", GigID: &gigID,
		SourceKind: EntrySourceGigPayment, SourceID: sourceID,
		SourceAmountMinor: intPtr(10000), SourceCurrency: strPtr("USD"),
		SourceDescription: "gig:" + gigID.String(),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Soft delete: this is the manual delete path on auto-generated
	// rows, which is forbidden by mutationError normally — but the
	// underlying UPDATE statement accepts it for the test.
	if _, err := testPool.Exec(ctx,
		`UPDATE finance_entries SET deleted_at=clock_timestamp() WHERE id=$1`,
		created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = repo.GetByGeneratedSource(ctx, EntrySourceGigPayment, sourceID, EntryKindIncome)
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound after delete, got %v", err)
	}
}

// TestBE06_GetByGeneratedSourceNoActiveReturnsNotFound: when no active
// row exists (only a voided prior one), the helper must return
// ErrEntryNotFound rather than a silently-wrong voided row.
func TestBE06_GetByGeneratedSourceNoActiveReturnsNotFound(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)

	gigID := insertTestGig(t, "SEK")
	sourceID := uuid.New()

	first, err := repo.CreateGenerated(ctx, GeneratedEntryInput{
		Kind: EntryKindIncome, AmountMinor: 25000, Currency: "SEK",
		Category: "gig_fee", EntryDate: "2026-10-03",
		Description: "DJ performance", GigID: &gigID,
		SourceKind: EntrySourceGigPayment, SourceID: sourceID,
		SourceAmountMinor: intPtr(25000), SourceCurrency: strPtr("SEK"),
		SourceDescription: "gig:" + gigID.String(),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`UPDATE finance_entries SET status='voided' WHERE id=$1`, first.ID); err != nil {
		t.Fatalf("void: %v", err)
	}

	_, err = repo.GetByGeneratedSource(ctx, EntrySourceGigPayment, sourceID, EntryKindIncome)
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound when only voided row exists, got %v", err)
	}
}

// TestBE06_CreateGeneratedRetryAfterDeleteHistory: a generic retry against a
// soft-deleted generated source must be rejected as a conflict (no new
// revenue row) and leave the deleted row untouched.
func TestBE06_CreateGeneratedRetryAfterDeleteHistory(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)

	gigID := insertTestGig(t, "EUR")
	in := GeneratedEntryInput{
		Kind: EntryKindIncome, AmountMinor: 25000, Currency: "EUR",
		Category: "gig_fee", EntryDate: "2026-10-03",
		Description: "DJ performance", GigID: &gigID,
		SourceKind: EntrySourceGigPayment, SourceID: uuid.New(),
		SourceAmountMinor: intPtr(25000), SourceCurrency: strPtr("EUR"),
		SourceDescription: "gig:" + gigID.String(),
	}
	created, err := repo.CreateGenerated(ctx, in)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`UPDATE finance_entries SET deleted_at=clock_timestamp() WHERE id=$1`, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := repo.CreateGenerated(ctx, in); !errors.Is(err, ErrGeneratedEntryConflict) {
		t.Fatalf("retry after delete: err = %v, want ErrGeneratedEntryConflict", err)
	}
	var rows, active int
	if err := testPool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE deleted_at IS NULL AND status='active')
		FROM finance_entries WHERE source_kind=$1 AND source_id=$2`, in.SourceKind, in.SourceID).Scan(&rows, &active); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 || active != 0 {
		t.Fatalf("rows=%d active=%d after rejected retry, want 1 and 0", rows, active)
	}
}

func intPtr(v int64) *int64   { return &v }
func strPtr(s string) *string { return &s }
