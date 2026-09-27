package retention

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// SystemActor is the audit actor of scheduled purges.
const SystemActor = "system:retention"

// Service implements the retention settings, the privacy view, manual and
// scheduled purges. Request methods run in the tenant carried by ctx; the
// job methods visit every tenant through tenantdb.
type Service struct {
	db  *tenantdb.DB
	now func() time.Time
}

// NewService builds the service; now defaults to time.Now.
func NewService(db *tenantdb.DB, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

func (s *Service) clock() time.Time { return s.now().UTC() }

func tenantOf(ctx context.Context) (uuid.UUID, error) {
	id, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return uuid.Nil, tenantdb.ErrNoTenant
	}
	return id, nil
}

// retentionDays reads the tenant's retention period (default when unset).
func retentionDays(ctx context.Context, tx pgx.Tx) (int, error) {
	var days int16
	err := tx.QueryRow(ctx, `SELECT retention_days FROM org_privacy`).Scan(&days)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultRetentionDays, nil
	}
	return int(days), err
}

type eventRow struct {
	ID       uuid.UUID
	Title    string
	StartsAt time.Time
	EndsAt   *time.Time
}

func (e eventRow) end() time.Time { return EndOf(e.StartsAt, e.EndsAt) }

// unpurgedEvents returns the tenant's events that have not been purged.
func unpurgedEvents(ctx context.Context, tx pgx.Tx) ([]eventRow, error) {
	rows, err := tx.Query(ctx, `SELECT e.id, e.title, e.starts_at, e.ends_at FROM events e
	  WHERE NOT EXISTS (SELECT 1 FROM event_purges p WHERE p.event_id = e.id AND p.purged_at IS NOT NULL)
	  ORDER BY e.ends_at, e.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (eventRow, error) {
		var e eventRow
		err := r.Scan(&e.ID, &e.Title, &e.StartsAt, &e.EndsAt)
		return e, err
	})
}

// schedule (re)computes purge_after for every unpurged event of the tenant,
// so a changed retention period or event end is reflected in
// event_purges. Rows of purged events never change.
func schedule(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, days int) ([]eventRow, error) {
	evs, err := unpurgedEvents(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		if _, err := tx.Exec(ctx, `INSERT INTO event_purges (tenant_id, event_id, purge_after) VALUES ($1, $2, $3)
		  ON CONFLICT (event_id) DO UPDATE SET purge_after = EXCLUDED.purge_after
		  WHERE event_purges.purged_at IS NULL AND event_purges.purge_after IS DISTINCT FROM EXCLUDED.purge_after`,
			tenant, e.ID, PurgeAfter(e.StartsAt, e.EndsAt, days)); err != nil {
			return nil, err
		}
	}
	return evs, nil
}

func dueOf(tenant uuid.UUID, evs []eventRow, days int, now time.Time) []DueEvent {
	var out []DueEvent
	for _, e := range evs {
		if at := PurgeAfter(e.StartsAt, e.EndsAt, days); Due(at, now) {
			out = append(out, DueEvent{TenantID: tenant, EventID: e.ID, Title: e.Title, EndsAt: e.end(), PurgeAfter: at})
		}
	}
	return out
}

// ---------------------------------------------------------- settings ---

// Settings returns the tenant's retention period, the next purges (events
// that have started, by purge_after) and the latest purges.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	var out Settings
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		out, err = s.settings(ctx, tx)
		return err
	})
	return out, err
}

func (s *Service) settings(ctx context.Context, tx pgx.Tx) (Settings, error) {
	out := Settings{Upcoming: []Upcoming{}, Recent: []Recent{}}
	days, err := retentionDays(ctx, tx)
	if err != nil {
		return out, err
	}
	out.RetentionDays = days
	// purge_after is the end plus a constant, so ordering by the end is
	// ordering by purge_after.
	rows, err := tx.Query(ctx, `SELECT e.id, e.title, e.starts_at, e.ends_at FROM events e
	  WHERE e.starts_at <= $1
	    AND NOT EXISTS (SELECT 1 FROM event_purges p WHERE p.event_id = e.id AND p.purged_at IS NOT NULL)
	  ORDER BY COALESCE(e.ends_at, e.starts_at + interval '24 hours'), e.id LIMIT $2`, s.clock(), listLimit)
	if err != nil {
		return out, err
	}
	evs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (eventRow, error) {
		var e eventRow
		err := r.Scan(&e.ID, &e.Title, &e.StartsAt, &e.EndsAt)
		return e, err
	})
	if err != nil {
		return out, err
	}
	for _, e := range evs {
		out.Upcoming = append(out.Upcoming, Upcoming{EventID: e.ID, Title: e.Title, EndsAt: e.end(), PurgeAfter: PurgeAfter(e.StartsAt, e.EndsAt, days)})
	}
	rows, err = tx.Query(ctx, `SELECT p.event_id, e.title, p.purged_at, p.trigger, p.counts FROM event_purges p
	  JOIN events e ON e.id = p.event_id
	  WHERE p.purged_at IS NOT NULL ORDER BY p.purged_at DESC, p.event_id LIMIT $1`, listLimit)
	if err != nil {
		return out, err
	}
	recent, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Recent, error) {
		var x Recent
		var counts []byte
		if err := r.Scan(&x.EventID, &x.Title, &x.PurgedAt, &x.Trigger, &counts); err != nil {
			return x, err
		}
		x.PurgedAt = x.PurgedAt.UTC()
		return x, json.Unmarshal(counts, &x.Counts)
	})
	if err != nil {
		return out, err
	}
	out.Recent = append(out.Recent, recent...)
	return out, nil
}

// UpdateRetention sets the tenant's retention period (1..365 days), audits
// the change and recomputes purge_after of every unpurged event.
func (s *Service) UpdateRetention(ctx context.Context, days int, actor string) (Settings, error) {
	var out Settings
	if err := ValidateDays(days); err != nil {
		return out, err
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		before, err := retentionDays(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_privacy (tenant_id, retention_days, updated_at, updated_by) VALUES ($1, $2, $3, $4)
		  ON CONFLICT (tenant_id) DO UPDATE SET retention_days = EXCLUDED.retention_days, updated_at = EXCLUDED.updated_at,
		    updated_by = EXCLUDED.updated_by`, tenant, days, s.clock(), actor); err != nil {
			return err
		}
		if _, err := schedule(ctx, tx, tenant, days); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, tenant, audit.Entry{
			ActorID: actor, Action: "org.retention_updated", Resource: "org:" + tenant.String(), Allowed: true,
			Reason: fmt.Sprintf("retention %d → %d days", before, days),
		}); err != nil {
			return err
		}
		out, err = s.settings(ctx, tx)
		return err
	})
	return out, err
}

// Privacy returns an event's retention state. Before the purge,
// purge_after is computed from the current event end and retention period.
func (s *Service) Privacy(ctx context.Context, eventID uuid.UUID) (Privacy, error) {
	var out Privacy
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var e eventRow
		err := tx.QueryRow(ctx, `SELECT id, title, starts_at, ends_at FROM events WHERE id = $1`, eventID).
			Scan(&e.ID, &e.Title, &e.StartsAt, &e.EndsAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if out.RetentionDays, err = retentionDays(ctx, tx); err != nil {
			return err
		}
		out.PurgeAfter = PurgeAfter(e.StartsAt, e.EndsAt, out.RetentionDays)
		var stored time.Time
		var purgedAt *time.Time
		err = tx.QueryRow(ctx, `SELECT purge_after, purged_at FROM event_purges WHERE event_id = $1`, eventID).Scan(&stored, &purgedAt)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		case purgedAt != nil:
			out.PurgeAfter, out.PurgedAt = stored.UTC(), ptr(purgedAt.UTC())
		}
		return tx.QueryRow(ctx, `SELECT
		    (SELECT count(*) FROM guests WHERE event_id = $1 AND purged_at IS NULL)
		  + (SELECT count(*) FROM order_positions WHERE event_id = $1 AND purged_at IS NULL)`, eventID).Scan(&out.PersonalRows)
	})
	return out, err
}

func ptr[T any](v T) *T { return &v }

// ------------------------------------------------------------- purges ---

// Purge erases one event now ("erase now"). The event must have ended and
// confirm must be its exact title (surrounding spaces ignored).
func (s *Service) Purge(ctx context.Context, eventID uuid.UUID, confirm, actor string) (PurgeResult, error) {
	var out PurgeResult
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		e, purged, err := lockEvent(ctx, tx, eventID)
		if err != nil {
			return err
		}
		now := s.clock()
		switch {
		case purged:
			return ErrEventPurged
		case !Ended(e.StartsAt, e.EndsAt, now):
			return ErrNotEnded
		case strings.TrimSpace(confirm) != e.Title:
			return &InvalidError{Field: "confirm", Problem: "type the event title exactly"}
		}
		days, err := retentionDays(ctx, tx)
		if err != nil {
			return err
		}
		out, err = s.purgeLocked(ctx, tx, tenant, e, days, TriggerManual, actor, now)
		return err
	})
	return out, err
}

// lockEvent locks the event row for the purge (writers take a share lock on
// it, see EnsureNotPurged) and reports whether it is already purged.
func lockEvent(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) (eventRow, bool, error) {
	var e eventRow
	err := tx.QueryRow(ctx, `SELECT id, title, starts_at, ends_at FROM events WHERE id = $1 FOR UPDATE`, eventID).
		Scan(&e.ID, &e.Title, &e.StartsAt, &e.EndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, false, ErrNotFound
	}
	if err != nil {
		return e, false, err
	}
	err = refuseIfPurged(ctx, tx, eventID)
	if errors.Is(err, ErrEventPurged) {
		return e, true, nil
	}
	return e, false, err
}

// purgeLocked anonymises the locked event: personal columns and blind
// indexes become NULL (rows stay, with purged_at), allocation contacts are
// erased, door PINs deleted and door sessions revoked. It records the purge
// with counts, emits retention.purged (identifiers only) and audits it.
func (s *Service) purgeLocked(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, e eventRow, days int, trigger, actor string, now time.Time) (PurgeResult, error) {
	res := PurgeResult{TenantID: tenant, EventID: e.ID, PurgedAt: now, Trigger: trigger}
	steps := []struct {
		n    *int
		sql  string
		args []any
	}{
		{&res.Counts.Guests, `UPDATE guests SET name_enc = NULL, email_enc = NULL, phone_enc = NULL, note_enc = NULL,
		  name_bidx = NULL, email_bidx = NULL, purged_at = $2 WHERE event_id = $1 AND purged_at IS NULL`, []any{e.ID, now}},
		{&res.Counts.Orders, `UPDATE orders SET buyer_name_enc = NULL, buyer_email_enc = NULL, buyer_email_bidx = NULL, purged_at = $2
		  WHERE event_id = $1 AND purged_at IS NULL`, []any{e.ID, now}},
		{&res.Counts.OrderPositions, `UPDATE order_positions SET attendee_name_enc = NULL, attendee_email_enc = NULL, secret_enc = NULL,
		  attendee_name_bidx = NULL, attendee_email_bidx = NULL, secret_bidx = NULL, purged_at = $2
		  WHERE event_id = $1 AND purged_at IS NULL`, []any{e.ID, now}},
		{&res.Counts.GuestAllocations, `UPDATE guest_allocations SET submitter_contact_enc = NULL
		  WHERE event_id = $1 AND submitter_contact_enc IS NOT NULL`, []any{e.ID}},
		{&res.Counts.DoorPins, `DELETE FROM door_pins WHERE event_id = $1`, []any{e.ID}},
	}
	for _, st := range steps {
		tag, err := tx.Exec(ctx, st.sql, st.args...)
		if err != nil {
			return res, err
		}
		*st.n = int(tag.RowsAffected())
	}
	// Door sessions of the event end with it: the device cache is useless
	// without names, and a bundle would be refused anyway.
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE event_scope = $1 AND revoked_at IS NULL`, e.ID, now); err != nil {
		return res, err
	}
	counts, err := json.Marshal(res.Counts)
	if err != nil {
		return res, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO event_purges (tenant_id, event_id, purge_after, purged_at, counts, trigger)
	  VALUES ($1, $2, $3, $4, $5, $6)
	  ON CONFLICT (event_id) DO UPDATE SET purged_at = EXCLUDED.purged_at, counts = EXCLUDED.counts, trigger = EXCLUDED.trigger,
	    purge_after = CASE WHEN EXCLUDED.trigger = 'schedule' THEN EXCLUDED.purge_after ELSE event_purges.purge_after END`,
		tenant, e.ID, PurgeAfter(e.StartsAt, e.EndsAt, days), now, counts, trigger); err != nil {
		return res, err
	}
	subject, ev, err := events.New(tenant, "retention", "purged", map[string]uuid.UUID{"event_id": e.ID}, now)
	if err != nil {
		return res, err
	}
	if err := events.Enqueue(ctx, tx, subject, ev); err != nil {
		return res, err
	}
	c := res.Counts
	err = audit.Record(ctx, tx, tenant, audit.Entry{
		ActorID: actor, Action: "retention.purged", Resource: "event:" + e.ID.String(), Allowed: true,
		Reason: fmt.Sprintf("%s: %d guests, %d orders, %d order positions, %d allocation contacts, %d door pins",
			trigger, c.Guests, c.Orders, c.OrderPositions, c.GuestAllocations, c.DoorPins),
	})
	return res, err
}

// ---------------------------------------------------------------- job ---

// DryRun lists, across tenants, the events the job would purge now.
// Nothing is written.
func (s *Service) DryRun(ctx context.Context) ([]DueEvent, error) {
	tenants, err := s.db.Tenants(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	var out []DueEvent
	for _, tenant := range tenants {
		err := s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			days, err := retentionDays(ctx, tx)
			if err != nil {
				return err
			}
			evs, err := unpurgedEvents(ctx, tx)
			if err != nil {
				return err
			}
			out = append(out, dueOf(tenant, evs, days, now)...)
			return nil
		})
		if err != nil {
			return out, fmt.Errorf("tenant %s: %w", tenant, err)
		}
	}
	return out, nil
}

// RunDue is one run of the retention job: for each tenant it refreshes the
// schedule, then purges each due event in its own transaction (re-checked
// under the event lock, so an end moved later in the meantime is honoured
// and a concurrent run purges nothing twice). A failing tenant does not
// stop the others; its error is returned joined with the rest.
func (s *Service) RunDue(ctx context.Context) ([]PurgeResult, error) {
	tenants, err := s.db.Tenants(ctx)
	if err != nil {
		return nil, err
	}
	var out []PurgeResult
	var errs []error
	for _, tenant := range tenants {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		res, err := s.runTenant(ctx, tenant)
		out = append(out, res...)
		if err != nil {
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenant, err))
		}
	}
	return out, errors.Join(errs...)
}

func (s *Service) runTenant(ctx context.Context, tenant uuid.UUID) ([]PurgeResult, error) {
	var due []DueEvent
	err := s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		days, err := retentionDays(ctx, tx)
		if err != nil {
			return err
		}
		evs, err := schedule(ctx, tx, tenant, days)
		if err != nil {
			return err
		}
		due = dueOf(tenant, evs, days, s.clock())
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []PurgeResult
	for _, d := range due {
		var res *PurgeResult
		err := s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			e, purged, err := lockEvent(ctx, tx, d.EventID)
			if errors.Is(err, ErrNotFound) || purged {
				return nil // deleted or purged meanwhile
			}
			if err != nil {
				return err
			}
			days, err := retentionDays(ctx, tx)
			if err != nil {
				return err
			}
			now := s.clock()
			if !Due(PurgeAfter(e.StartsAt, e.EndsAt, days), now) {
				return nil
			}
			r, err := s.purgeLocked(ctx, tx, tenant, e, days, TriggerSchedule, SystemActor, now)
			res = &r
			return err
		})
		if err != nil {
			return out, fmt.Errorf("event %s: %w", d.EventID, err)
		}
		if res != nil {
			out = append(out, *res)
		}
	}
	return out, nil
}
