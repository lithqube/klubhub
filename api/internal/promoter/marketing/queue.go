package marketing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Worker defaults.
const (
	DefaultInterval    = time.Second
	DefaultPerMinute   = 60
	DefaultMaxAttempts = 5
	DefaultBackoff     = time.Minute
	maxClaim           = 50 // rows per tenant per tick, whatever the rate
)

// Send is one claimed marketing_sends row, unsealed; Attempts are prior ones.
type Send struct {
	TenantID uuid.UUID
	Attempts int
	Message  Message
}

// Queue is the marketing_sends persistence the Worker drives. Every method
// runs under the row's tenant (tenantdb.WithTenant).
type Queue interface {
	Tenants(ctx context.Context) ([]uuid.UUID, error)
	// Claim atomically moves up to limit of tenant's 'queued' rows with
	// scheduled_at <= now to 'sending' and returns them. Rows left in
	// 'sending' by a crashed worker must become claimable again with the same
	// id, so the provider's idempotency key (Message.SendID) stops a double send.
	Claim(ctx context.Context, tenant uuid.UUID, now time.Time, limit int) ([]Send, error)
	Sent(ctx context.Context, s Send, providerMessageID string, at time.Time) error
	// Retry: status 'queued', attempts+1, last_error, scheduled_at = at.
	Retry(ctx context.Context, s Send, lastError string, at time.Time) error
	// Fail dead-letters: status 'failed', attempts+1, last_error.
	Fail(ctx context.Context, s Send, lastError string) error
}

// Worker drains the queue: each tick, per tenant, it claims what the tenant's
// rate allows and delivers through the tenant's provider; failures back off
// exponentially and dead-letter after MaxAttempts. Not safe for concurrent use.
type Worker struct {
	Queue Queue
	// Provider builds the tenant's MarketingProvider (its provider config row).
	Provider func(ctx context.Context, tenant uuid.UUID) (MarketingProvider, error)
	Now      func() time.Time
	Interval time.Duration
	// PerMinute caps each tenant's sends per minute.
	PerMinute   int
	MaxAttempts int
	// Backoff is the first retry delay; it doubles on each further failure.
	Backoff time.Duration
	OnError func(error)
	// cursor[t] is the virtual time tenant t's send budget is spent up to.
	cursor map[uuid.UUID]time.Time
}

// Run ticks every Interval, starting immediately, until ctx ends. Errors
// (including recovered panics) go to OnError and the next tick retries.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := w.RunOnce(ctx); err != nil && ctx.Err() == nil && w.OnError != nil {
			w.OnError(err)
		}
		t.Reset(w.interval())
	}
}

// RunOnce runs one pass over every tenant. A panic in the queue is returned
// as an error so it cannot kill the process (chi's Recoverer does not cover
// this goroutine).
func (w *Worker) RunOnce(ctx context.Context) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("marketing worker panic: %v", rec)
		}
	}()
	tenants, err := w.Queue.Tenants(ctx)
	if err != nil {
		return err
	}
	var errs error
	for _, tenant := range tenants {
		if ctx.Err() != nil {
			break
		}
		errs = errors.Join(errs, w.drain(ctx, tenant))
	}
	return errs
}

func (w *Worker) drain(ctx context.Context, tenant uuid.UUID) error {
	now := w.now()
	n := min(w.budget(tenant, now), maxClaim)
	if n <= 0 {
		return nil
	}
	sends, err := w.Queue.Claim(ctx, tenant, now, n)
	if err != nil || len(sends) == 0 {
		return err
	}
	p, perr := w.Provider(ctx, tenant)
	var errs error
	for _, s := range sends {
		w.cursor[tenant] = w.cursor[tenant].Add(w.gap())
		sendErr := perr
		var id string
		if sendErr == nil {
			id, sendErr = deliver(ctx, p, s.Message)
		}
		errs = errors.Join(errs, w.finish(ctx, s, id, sendErr))
	}
	return errs
}

// deliver sends one message; a provider panic becomes that send's error.
func deliver(ctx context.Context, p MarketingProvider, msg Message) (id string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("provider panic: %v", rec)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second) // a hung transport must not stall the queue
	defer cancel()
	return p.SendMarketing(ctx, &msg)
}

func (w *Worker) finish(ctx context.Context, s Send, providerID string, sendErr error) error {
	// Record the outcome even when shutdown cancelled the delivery.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := w.now()
	if sendErr == nil {
		return w.Queue.Sent(ctx, s, providerID, now)
	}
	attempts := s.Attempts + 1
	if attempts >= w.maxAttempts() {
		return w.Queue.Fail(ctx, s, sendErr.Error())
	}
	return w.Queue.Retry(ctx, s, sendErr.Error(), now.Add(w.backoff()<<min(attempts-1, 16)))
}

// budget is a per-tenant token bucket: one send per gap, holding at most one
// tick's worth so a slow tick cannot release a burst; sends move the cursor.
func (w *Worker) budget(tenant uuid.UUID, now time.Time) int {
	if w.cursor == nil {
		w.cursor = map[uuid.UUID]time.Time{}
	}
	floor := now.Add(-max(w.interval(), w.gap()))
	if w.cursor[tenant].Before(floor) {
		w.cursor[tenant] = floor
	}
	return int(now.Sub(w.cursor[tenant]) / w.gap())
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Worker) interval() time.Duration { return orDefault(w.Interval, DefaultInterval) }
func (w *Worker) gap() time.Duration {
	return time.Minute / time.Duration(orDefault(w.PerMinute, DefaultPerMinute))
}
func (w *Worker) maxAttempts() int       { return orDefault(w.MaxAttempts, DefaultMaxAttempts) }
func (w *Worker) backoff() time.Duration { return orDefault(w.Backoff, DefaultBackoff) }

func orDefault[T int | time.Duration](v, d T) T {
	if v <= 0 {
		return d
	}
	return v
}
