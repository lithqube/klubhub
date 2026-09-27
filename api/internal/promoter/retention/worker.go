package retention

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// DefaultInterval is how often the worker runs the job.
const DefaultInterval = time.Hour

// Worker runs the retention job once at start and then every Interval
// until ctx ends (same lifecycle as the outbox relay in `promoter serve`).
type Worker struct {
	Service  *Service
	Log      zerolog.Logger
	Interval time.Duration
}

// Run blocks until ctx is cancelled. Errors are logged and retried on the
// next tick; a purge that failed half-way rolled back with its transaction.
func (w *Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		w.RunOnce(ctx)
		t.Reset(interval)
	}
}

// RunOnce runs the job (due event purges, then expired ban entries) and
// logs the outcome (identifiers and counts only).
func (w *Worker) RunOnce(ctx context.Context) {
	res, err := w.Service.RunDue(ctx)
	for _, r := range res {
		w.Log.Info().Str("event", "retention.purged").Str("org", r.TenantID.String()).Str("event_id", r.EventID.String()).
			Int("guests", r.Counts.Guests).Int("order_positions", r.Counts.OrderPositions).Msg("event personal data purged")
	}
	if err != nil && ctx.Err() == nil {
		w.Log.Error().Err(err).Msg("retention job failed")
	}
	bans, err := w.Service.PurgeExpiredBans(ctx)
	for _, b := range bans {
		w.Log.Info().Str("event", "banlist.expired_purged").Str("org", b.TenantID.String()).Int("deleted", b.Deleted).Msg("expired ban entries deleted")
	}
	if err != nil && ctx.Err() == nil {
		w.Log.Error().Err(err).Msg("ban list expiry job failed")
	}
}
