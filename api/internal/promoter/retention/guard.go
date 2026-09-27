package retention

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EnsureNotPurged refuses (ErrEventPurged) a write that would add personal
// data to an erased event. Guests, attendee import, door adds and door PIN
// generation call it inside their own tenant transaction, before touching
// other rows of the event.
//
// It takes a share lock on the event row first. A purge holds the row
// FOR UPDATE for its whole transaction, so a write either commits before
// the purge anonymises the event (and is anonymised with it) or runs after
// it and sees purged_at. The purge check is a separate statement so it reads
// a snapshot taken after the lock was granted. A missing event is left to
// the caller's own not-found handling.
func EnsureNotPurged(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) error {
	var one int
	err := tx.QueryRow(ctx, `SELECT 1 FROM events WHERE id = $1 FOR SHARE`, eventID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return refuseIfPurged(ctx, tx, eventID)
}

// RefuseIfPurged is the read-side check (exports, the door bundle): it
// returns ErrEventPurged for an erased event, without locking.
func RefuseIfPurged(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) error {
	return refuseIfPurged(ctx, tx, eventID)
}

func refuseIfPurged(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) error {
	var purged bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM event_purges WHERE event_id = $1 AND purged_at IS NOT NULL)`,
		eventID).Scan(&purged); err != nil {
		return err
	}
	if purged {
		return ErrEventPurged
	}
	return nil
}
