package finance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GetPendingReconciliationByGig returns the active pending decision for
// gigID, or ErrReconciliationNotFound when there is nothing to surface.
// There is at most one pending row per gig (the partial unique index on
// (gig_id) WHERE status='pending' guarantees it).
func (r *EntryRepository) GetPendingReconciliationByGig(ctx context.Context, gigID uuid.UUID) (*EntryReconciliation, error) {
	var rec EntryReconciliation
	err := r.pool.QueryRow(ctx, `SELECT `+reconciliationColumns+` FROM finance_entry_reconciliations
		WHERE gig_id=$1 AND status='pending'`, gigID).Scan(scanReconciliation(&rec)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReconciliationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get pending reconciliation by gig: %w", err)
	}
	return &rec, nil
}

// GetPendingReconciliationByEntry is the entry-scoped lookup used by the
// gig update response path so the UI can keep showing the latest prompt
// after a redirect.
func (r *EntryRepository) GetPendingReconciliationByEntry(ctx context.Context, entryID uuid.UUID) (*EntryReconciliation, error) {
	var rec EntryReconciliation
	err := r.pool.QueryRow(ctx, `SELECT `+reconciliationColumns+` FROM finance_entry_reconciliations
		WHERE entry_id=$1 AND status='pending'`, entryID).Scan(scanReconciliation(&rec)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReconciliationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get pending reconciliation by entry: %w", err)
	}
	return &rec, nil
}

// ResolveReconciliation applies the user's action to the pending decision
// with optimistic concurrency on the reconciliation row, then mutates the
// linked finance entry per the action.
//
// Actions allowed by the migration's allowed_actions CHECK constraint:
//   - update -> adjusts the linked entry to current gig fee/currency and
//     acknowledges the snapshot.
//   - delete -> soft-deletes the linked entry.
//   - void   -> marks the linked entry voided, preserving the amount.
//   - keep   -> leaves the entry unchanged and acknowledges the snapshot
//     so the same prompt does not recur.
func (r *EntryRepository) ResolveReconciliation(ctx context.Context, id uuid.UUID, req ResolveReconciliationRequest) (*EntryReconciliation, error) {
	if req.UpdatedAt.IsZero() {
		return nil, EntryValidationErrors{{Field: "updated_at", Message: "is required"}}
	}
	if !isKnownReconciliationAction(req.Action) {
		return nil, EntryValidationErrors{{Field: "action", Message: "must be update, delete, void, or keep"}}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin resolve reconciliation tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var rec EntryReconciliation
	err = tx.QueryRow(ctx, `SELECT `+reconciliationColumns+` FROM finance_entry_reconciliations
		WHERE id=$1 AND status='pending' FOR UPDATE`, id).Scan(scanReconciliation(&rec)...)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either it never existed, was already resolved, or was
		// superseded by an acknowledged-keep row.
		return nil, ErrReconciliationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load reconciliation: %w", err)
	}
	if !rec.UpdatedAt.Equal(req.UpdatedAt) {
		return nil, ErrReconciliationConflict
	}
	if !actionAllowedFor(rec.Reason, req.Action) {
		return nil, ErrReconciliationAction
	}

	// Pull the entry so we can mutate it consistently.
	var entry Entry
	err = tx.QueryRow(ctx, `SELECT `+financeEntryColumns+` FROM finance_entries
		WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, rec.EntryID).Scan(scanFinanceEntry(&entry)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEntryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load entry: %w", err)
	}

	switch req.Action {
	case ReconciliationActionUpdate:
		newAmount := rec.GigAmountMinor
		newCurrency := rec.GigCurrency
		_, err = tx.Exec(ctx, `UPDATE finance_entries SET amount_minor=$2, currency=$3, source_amount_minor=$4, source_currency=$5, updated_at=clock_timestamp()
			WHERE id=$1 AND auto_generated=true AND status='active' AND deleted_at IS NULL`,
			entry.ID, newAmount, newCurrency, &newAmount, &newCurrency)
		if err != nil {
			return nil, fmt.Errorf("apply update to entry: %w", err)
		}
	case ReconciliationActionDelete:
		_, err = tx.Exec(ctx, `UPDATE finance_entries SET deleted_at=clock_timestamp(), updated_at=clock_timestamp()
			WHERE id=$1 AND deleted_at IS NULL`, entry.ID)
		if err != nil {
			return nil, fmt.Errorf("delete entry: %w", err)
		}
	case ReconciliationActionVoid:
		_, err = tx.Exec(ctx, `UPDATE finance_entries SET status='voided', updated_at=clock_timestamp()
			WHERE id=$1 AND status='active' AND deleted_at IS NULL`, entry.ID)
		if err != nil {
			return nil, fmt.Errorf("void entry: %w", err)
		}
	case ReconciliationActionKeep:
		// Acknowledge the snapshot: bump source_amount_minor and
		// source_currency on the entry so a later transition through
		// the same values does not re-raise the same prompt. The
		// amount_minor / currency the user sees stays untouched.
		newAmount := rec.GigAmountMinor
		newCurrency := rec.GigCurrency
		_, err = tx.Exec(ctx, `UPDATE finance_entries SET source_amount_minor=$2, source_currency=$3, updated_at=clock_timestamp()
			WHERE id=$1 AND auto_generated=true AND status='active' AND deleted_at IS NULL`,
			entry.ID, &newAmount, &newCurrency)
		if err != nil {
			return nil, fmt.Errorf("acknowledge snapshot on entry: %w", err)
		}
	}

	now := time.Now().UTC()
	var resolved EntryReconciliation
	err = tx.QueryRow(ctx, `UPDATE finance_entry_reconciliations
		SET status='resolved', resolution=$2, resolved_at=$3, updated_at=$3
		WHERE id=$1 AND status='pending' AND updated_at=$4
		RETURNING `+reconciliationColumns,
		id, req.Action, now, req.UpdatedAt).Scan(scanReconciliation(&resolved)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReconciliationConflict
	}
	if err != nil {
		return nil, fmt.Errorf("resolve reconciliation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit resolve reconciliation: %w", err)
	}
	return &resolved, nil
}

func isKnownReconciliationAction(a ReconciliationAction) bool {
	switch a {
	case ReconciliationActionUpdate, ReconciliationActionDelete, ReconciliationActionVoid, ReconciliationActionKeep:
		return true
	}
	return false
}

func actionAllowedFor(reason ReconciliationReason, a ReconciliationAction) bool {
	for _, x := range allowedActionsFor(reason) {
		if x == a {
			return true
		}
	}
	return false
}
