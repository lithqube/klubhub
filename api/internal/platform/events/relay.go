package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Publisher sends one message with a de-duplication id.
type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte, msgID string) error
}

// Relay moves committed outbox rows to the bus. Its pool must connect as
// klubhub_relay (the only role that can read the outbox across tenants).
type Relay struct {
	Pool      *pgxpool.Pool
	Publisher Publisher
	Log       zerolog.Logger
	Batch     int
	Poll      time.Duration
	Now       func() time.Time
}

// Run listens for outbox notifications (with a polling fallback) until ctx
// ends.
func (r *Relay) Run(ctx context.Context) error {
	if r.Poll == 0 {
		r.Poll = 5 * time.Second
	}
	conn, err := r.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN promoter_outbox"); err != nil {
		return err
	}
	for {
		for {
			n, err := r.Drain(ctx)
			if err != nil {
				r.Log.Error().Err(err).Msg("outbox relay drain failed")
				break
			}
			if n < r.batch() {
				break
			}
		}
		wait, cancel := context.WithTimeout(ctx, r.Poll)
		_, err := conn.Conn().WaitForNotification(wait)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}
}

// Drain publishes one batch of pending rows and returns how many it
// attempted. Rows are locked with SKIP LOCKED so several relays can run.
func (r *Relay) Drain(ctx context.Context) (int, error) {
	batch := r.batch()
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	n := 0
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, subject, payload FROM outbox
		  WHERE published_at IS NULL ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED`, batch)
		if err != nil {
			return err
		}
		type row struct {
			id      uuid.UUID
			subject string
			payload []byte
		}
		var pending []row
		for rows.Next() {
			var x row
			if err := rows.Scan(&x.id, &x.subject, &x.payload); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, x)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, x := range pending {
			n++
			if err := r.Publisher.Publish(ctx, x.subject, x.payload, x.id.String()); err != nil {
				msg := err.Error()
				if len(msg) > 500 {
					msg = msg[:500]
				}
				if _, uerr := tx.Exec(ctx, `UPDATE outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1`, x.id, msg); uerr != nil {
					return uerr
				}
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = $2, last_error = NULL WHERE id = $1`, x.id, now()); err != nil {
				return fmt.Errorf("mark published: %w", err)
			}
		}
		return nil
	})
	return n, err
}

func (r *Relay) batch() int {
	if r.Batch <= 0 {
		return 100
	}
	return r.Batch
}
