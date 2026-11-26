package marketing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// memQueue is an in-memory marketing_sends. The worker drives it from one
// goroutine, so it needs no lock.
type memQueue struct {
	rows    []*memRow
	tenants []uuid.UUID
	panics  bool
}

type memRow struct {
	send      Send
	status    string
	due       time.Time
	lastError string
}

func (q *memQueue) add(tenant uuid.UUID, n int, due time.Time) {
	q.tenants = append(q.tenants, tenant)
	for range n {
		q.rows = append(q.rows, &memRow{send: Send{TenantID: tenant, Message: Message{SendID: uuid.New()}}, status: "queued", due: due})
	}
}

func (q *memQueue) count(tenant uuid.UUID, status string) (n int) {
	for _, r := range q.rows {
		if r.send.TenantID == tenant && r.status == status {
			n++
		}
	}
	return n
}

func (q *memQueue) Tenants(context.Context) ([]uuid.UUID, error) {
	if q.panics {
		panic("queue exploded")
	}
	return q.tenants, nil
}

func (q *memQueue) Claim(_ context.Context, tenant uuid.UUID, now time.Time, limit int) (out []Send, _ error) {
	for _, r := range q.rows {
		if len(out) < limit && r.send.TenantID == tenant && r.status == "queued" && !r.due.After(now) {
			r.status = "sending"
			out = append(out, r.send)
		}
	}
	return out, nil
}

// failed records a failed attempt on the row and gives it the new status.
func (q *memQueue) failed(s Send, status, lastError string, due time.Time) error {
	for _, r := range q.rows {
		if r.send.Message.SendID == s.Message.SendID {
			r.status, r.lastError, r.due = status, lastError, due
			r.send.Attempts++
		}
	}
	return nil
}

func (q *memQueue) Sent(_ context.Context, s Send, _ string, _ time.Time) error {
	for _, r := range q.rows {
		if r.send.Message.SendID == s.Message.SendID {
			r.status = "sent"
		}
	}
	return nil
}

func (q *memQueue) Retry(_ context.Context, s Send, lastError string, at time.Time) error {
	return q.failed(s, "queued", lastError, at)
}

func (q *memQueue) Fail(_ context.Context, s Send, lastError string) error {
	return q.failed(s, "failed", lastError, time.Time{})
}

type fakeProvider struct{ send func(*Message) error }

func (p fakeProvider) SendMarketing(_ context.Context, m *Message) (string, error) {
	if p.send != nil {
		return "", p.send(m)
	}
	return "pm-1", nil
}

func (fakeProvider) HandleWebhook(*http.Request) ([]WebhookEvent, error) { return nil, nil }

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func setup(p MarketingProvider, perMinute int) (*Worker, *memQueue, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)}
	q := &memQueue{}
	w := &Worker{
		Queue:     q,
		Provider:  func(context.Context, uuid.UUID) (MarketingProvider, error) { return p, nil },
		Now:       clock.now,
		Interval:  time.Second,
		PerMinute: perMinute,
	}
	return w, q, clock
}

func tick(t *testing.T, w *Worker) {
	t.Helper()
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerDrainsAtPerTenantRate(t *testing.T) {
	w, q, clock := setup(fakeProvider{}, 120) // one send per 500 ms; a 1 s tick holds 2
	a, b := uuid.New(), uuid.New()
	q.add(a, 10, clock.t)
	q.add(b, 10, clock.t)
	tick(t, w)
	if got := q.count(a, "sent"); got != 2 {
		t.Fatalf("tenant a sent %d on the first tick, want 2", got)
	}
	tick(t, w) // same instant: budget spent
	if got := q.count(a, "sent"); got != 2 {
		t.Fatalf("tenant a sent %d with no time passed, want 2", got)
	}
	clock.advance(1500 * time.Millisecond) // 3 accrued, capped to one tick's worth
	tick(t, w)
	if got, gotB := q.count(a, "sent"), q.count(b, "sent"); got != 4 || gotB != 4 {
		t.Fatalf("after 1.5 s: a sent %d, b sent %d, want 4 each", got, gotB)
	}
	for range 3 {
		clock.advance(time.Second)
		tick(t, w)
	}
	if got := q.count(a, "sent"); got != 10 {
		t.Fatalf("tenant a sent %d, want all 10", got)
	}
}

func TestWorkerRetriesWithBackoffThenDeadLetters(t *testing.T) {
	var keys []uuid.UUID
	w, q, clock := setup(fakeProvider{send: func(m *Message) error {
		keys = append(keys, m.SendID)
		return errors.New("smtp 451 try later")
	}}, 60)
	w.MaxAttempts, w.Backoff = 3, time.Minute
	tenant := uuid.New()
	q.add(tenant, 1, clock.t.Add(time.Second))
	// Due at t0+1s; fails → +1m; fails → +2m; third failure dead-letters.
	for i, wait := range []time.Duration{time.Second, time.Minute, 2 * time.Minute} {
		clock.advance(wait - time.Millisecond)
		tick(t, w)
		if len(keys) != i {
			t.Fatalf("attempt %d ran before it was due", i+1)
		}
		clock.advance(time.Millisecond)
		tick(t, w)
		if len(keys) != i+1 {
			t.Fatalf("attempt %d did not run when due", i+1)
		}
	}
	r := q.rows[0]
	if r.status != "failed" || r.send.Attempts != 3 || !strings.Contains(r.lastError, "451") {
		t.Fatalf("dead letter = %s, attempts %d, last error %q", r.status, r.send.Attempts, r.lastError)
	}
	if keys[0] != keys[1] || keys[1] != keys[2] {
		t.Fatalf("retries used different idempotency keys: %v", keys)
	}
	clock.advance(time.Hour)
	tick(t, w)
	if len(keys) != 3 {
		t.Fatalf("dead-lettered send was attempted again: %d attempts", len(keys))
	}
}

func TestWorkerRecoversFromProviderPanic(t *testing.T) {
	calls := 0
	w, q, clock := setup(fakeProvider{send: func(*Message) error {
		if calls++; calls == 1 {
			panic("adapter bug")
		}
		return nil
	}}, 120)
	tenant := uuid.New()
	q.add(tenant, 2, clock.t)
	tick(t, w)
	if q.count(tenant, "sent") != 1 || q.count(tenant, "queued") != 1 {
		t.Fatalf("sent=%d queued=%d, want the panicking send queued for retry", q.count(tenant, "sent"), q.count(tenant, "queued"))
	}
	if r := q.rows[0]; !strings.Contains(r.lastError, "panic") {
		t.Fatalf("panic not recorded on the retried send: %q", r.lastError)
	}
}

func TestWorkerRunOnceRecoversFromQueuePanic(t *testing.T) {
	w := &Worker{Queue: &memQueue{panics: true}}
	if err := w.RunOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("RunOnce = %v, want the panic returned as an error", err)
	}
}
