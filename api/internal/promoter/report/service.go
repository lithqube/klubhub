package report

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/retention"
)

// Service computes reports. Every method runs in the tenant carried by ctx
// (set by the auth middleware); RLS keeps other tenants' rows invisible.
type Service struct {
	db   *tenantdb.DB
	keys *envelope.Keyring
	now  func() time.Time
}

// NewService builds the service; now defaults to time.Now.
func NewService(db *tenantdb.DB, keys *envelope.Keyring, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, keys: keys, now: now}
}

func tenantOf(ctx context.Context) uuid.UUID {
	id, _ := tenantdb.TenantFromContext(ctx)
	return id
}

type eventRow struct {
	Event
	Slug string
	loc  *time.Location
}

func loadEvent(ctx context.Context, tx pgx.Tx, id uuid.UUID) (eventRow, error) {
	var e eventRow
	err := tx.QueryRow(ctx, `SELECT id, title, slug, starts_at, ends_at, timezone, capacity FROM events WHERE id = $1`, id).
		Scan(&e.ID, &e.Title, &e.Slug, &e.StartsAt, &e.EndsAt, &e.Timezone, &e.Capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	e.StartsAt, e.EndsAt = e.StartsAt.UTC(), e.EndsAt.UTC()
	if e.loc, err = time.LoadLocation(e.Timezone); err != nil {
		e.loc = time.UTC // validated on write; never fail a report over it
	}
	return e, nil
}

// Report builds the post-event report. It works during the event too.
func (s *Service) Report(ctx context.Context, eventID uuid.UUID) (Report, error) {
	var rep Report
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		ev, err := loadEvent(ctx, tx, eventID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		rep = Report{Event: ev.Event, GeneratedAt: now, Live: now.Before(ev.EndsAt)}

		groups, err := guestGroups(ctx, tx, eventID)
		if err != nil {
			return err
		}
		lists, err := listRows(ctx, tx, eventID)
		if err != nil {
			return err
		}
		subs, err := submitterRows(ctx, tx, eventID)
		if err != nil {
			return err
		}
		var guests guestTally
		guests, rep.ByList, rep.BySubmitter = rollup(groups, lists, subs)

		var ticketHeads int
		if rep.TicketsByType, ticketHeads, err = ticketRows(ctx, tx, eventID); err != nil {
			return err
		}
		t := &rep.Totals
		t.GuestsGoing, t.GuestsArrived = guests.Going, guests.Arrived
		t.NoShowRate = noShowRate(guests.Going, guests.GoingArrived)
		t.PlusOnesAllowed, t.PlusOnesUsed = guests.PlusAllowed, guests.PlusUsed
		for _, tt := range rep.TicketsByType {
			t.TicketsValid += tt.Valid
			t.TicketsScanned += tt.Scanned
		}
		t.HeadsExpected = guests.HeadsExpected + t.TicketsValid
		t.HeadsAdmitted = guests.HeadsAdmitted + ticketHeads

		if err := tx.QueryRow(ctx, `SELECT
		    (SELECT count(*) FROM checkins WHERE event_id = $1 AND undone_at IS NULL AND conflict_of IS NOT NULL),
		    (SELECT COALESCE(sum(delta), 0) FROM door_counters WHERE event_id = $1 AND undone_at IS NULL AND kind = 'walkup')`,
			eventID).Scan(&t.Conflicts, &t.Walkups); err != nil {
			return err
		}

		ps, err := activity(ctx, tx, eventID)
		if err != nil {
			return err
		}
		ps = clampPoints(ps, ev.StartsAt.Add(-curveMargin), ev.EndsAt.Add(curveMargin))
		rep.Curve = buildCurve(ps, ev.loc)
		t.PeakOccupancy, t.PeakAt = peak(rep.Curve)
		return nil
	})
	return rep, err
}

// groupKey identifies the guests of one list and allocation (nil for guests
// added straight to the list).
type groupKey struct {
	List       uuid.UUID
	Allocation uuid.UUID // uuid.Nil: no allocation
}

// guestGroups aggregates guests per (list, allocation) in SQL. Heads per
// guest are its live `in` counts; the per-guest definitions of the
// contract (arrived, +1s used) are applied before grouping.
func guestGroups(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) (map[groupKey]guestTally, error) {
	rows, err := tx.Query(ctx, `WITH heads AS (
	    SELECT guest_id, sum(count)::int AS n FROM checkins
	    WHERE event_id = $1 AND guest_id IS NOT NULL AND direction = 'in' AND undone_at IS NULL
	    GROUP BY guest_id
	  ), g AS (
	    SELECT g.list_id, g.allocation_id, g.status = 'going' AS going, g.plus_n::int AS plus_n, COALESCE(h.n, 0) AS heads
	    FROM guests g LEFT JOIN heads h ON h.guest_id = g.id WHERE g.event_id = $1
	  )
	  SELECT list_id, allocation_id,
	    count(*) FILTER (WHERE going),
	    count(*) FILTER (WHERE heads >= 1),
	    count(*) FILTER (WHERE going AND heads >= 1),
	    COALESCE(sum(1 + plus_n) FILTER (WHERE going), 0),
	    COALESCE(sum(heads), 0),
	    COALESCE(sum(plus_n) FILTER (WHERE going), 0),
	    COALESCE(sum(LEAST(plus_n, heads - 1)) FILTER (WHERE heads >= 1), 0)
	  FROM g GROUP BY list_id, allocation_id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[groupKey]guestTally{}
	for rows.Next() {
		var k groupKey
		var alloc *uuid.UUID
		var t guestTally
		if err := rows.Scan(&k.List, &alloc, &t.Going, &t.Arrived, &t.GoingArrived, &t.HeadsExpected, &t.HeadsAdmitted,
			&t.PlusAllowed, &t.PlusUsed); err != nil {
			return nil, err
		}
		if alloc != nil {
			k.Allocation = *alloc
		}
		out[k] = t
	}
	return out, rows.Err()
}

func listRows(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) ([]ListRow, error) {
	rows, err := tx.Query(ctx, `SELECT id, name, type FROM guest_lists WHERE event_id = $1 ORDER BY position, created_at, id`, eventID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ListRow, error) {
		var l ListRow
		err := r.Scan(&l.ListID, &l.Name, &l.Type)
		return l, err
	})
}

// submitterRows lists every allocation of the event, revoked ones too
// (their guests stay), artist lists first.
func submitterRows(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) ([]SubmitterRow, error) {
	rows, err := tx.Query(ctx, `SELECT a.id, a.list_id, l.name, l.type, a.label, a.quota, a.revoked_at IS NOT NULL
	  FROM guest_allocations a JOIN guest_lists l ON l.id = a.list_id
	  WHERE a.event_id = $1 ORDER BY (l.type = 'artist') DESC, l.position, l.created_at, a.created_at, a.id`, eventID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (SubmitterRow, error) {
		var s SubmitterRow
		err := r.Scan(&s.AllocationID, &s.ListID, &s.ListName, &s.ListType, &s.Submitter, &s.Quota, &s.Revoked)
		return s, err
	})
}

// rollup fills the list and submitter rows from the grouped tallies and
// returns the event-wide guest tally. Slices are never nil.
func rollup(groups map[groupKey]guestTally, lists []ListRow, subs []SubmitterRow) (guestTally, []ListRow, []SubmitterRow) {
	var total guestTally
	perList := map[uuid.UUID]guestTally{}
	perAlloc := map[uuid.UUID]guestTally{}
	for k, t := range groups {
		total.add(t)
		lt := perList[k.List]
		lt.add(t)
		perList[k.List] = lt
		if k.Allocation != uuid.Nil {
			at := perAlloc[k.Allocation]
			at.add(t)
			perAlloc[k.Allocation] = at
		}
	}
	outLists := make([]ListRow, 0, len(lists))
	for _, l := range lists {
		t := perList[l.ListID]
		l.Going, l.Arrived, l.NoShowRate = t.Going, t.Arrived, noShowRate(t.Going, t.GoingArrived)
		l.HeadsExpected, l.HeadsAdmitted = t.HeadsExpected, t.HeadsAdmitted
		l.PlusOnesAllowed, l.PlusOnesUsed = t.PlusAllowed, t.PlusUsed
		outLists = append(outLists, l)
	}
	outSubs := make([]SubmitterRow, 0, len(subs))
	for _, s := range subs {
		t := perAlloc[s.AllocationID]
		s.Going, s.Arrived, s.NoShowRate, s.HeadsAdmitted = t.Going, t.Arrived, noShowRate(t.Going, t.GoingArrived), t.HeadsAdmitted
		outSubs = append(outSubs, s)
	}
	return total, outLists, outSubs
}

// ticketRows returns tickets per tier and the heads admitted on tickets.
// Valid counts tickets in status valid; scanned counts tickets of any
// status with a live `in` (what happened at the door).
func ticketRows(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) ([]TicketTypeRow, int, error) {
	rows, err := tx.Query(ctx, `WITH heads AS (
	    SELECT position_id, sum(count)::int AS n FROM checkins
	    WHERE event_id = $1 AND position_id IS NOT NULL AND direction = 'in' AND undone_at IS NULL
	    GROUP BY position_id
	  )
	  SELECT t.id, t.name, count(p.id) FILTER (WHERE p.status = 'valid'), count(h.position_id), COALESCE(sum(h.n), 0)
	  FROM ticket_types t
	  LEFT JOIN order_positions p ON p.ticket_type_id = t.id
	  LEFT JOIN heads h ON h.position_id = p.id
	  WHERE t.event_id = $1
	  GROUP BY t.id, t.name, t.position, t.created_at ORDER BY t.position, t.created_at, t.id`, eventID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []TicketTypeRow{}
	heads := 0
	for rows.Next() {
		var t TicketTypeRow
		var n int
		if err := rows.Scan(&t.TicketTypeID, &t.Name, &t.Valid, &t.Scanned, &n); err != nil {
			return nil, 0, err
		}
		heads += n
		out = append(out, t)
	}
	return out, heads, rows.Err()
}

// activity sums live check-ins and counters per minute of the device
// clock (`at`), in SQL; bucketing into local quarter hours happens in Go.
func activity(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) ([]point, error) {
	rows, err := tx.Query(ctx, `SELECT to_timestamp(floor(extract(epoch FROM at) / 60) * 60) AS m,
	    COALESCE(sum(i), 0)::int, COALESCE(sum(o), 0)::int, COALESCE(sum(w), 0)::int
	  FROM (
	    SELECT at, CASE WHEN direction = 'in' THEN count ELSE 0 END AS i, CASE WHEN direction = 'out' THEN count ELSE 0 END AS o, 0 AS w
	    FROM checkins WHERE event_id = $1 AND undone_at IS NULL
	    UNION ALL
	    SELECT at, CASE WHEN kind = 'in' THEN delta ELSE 0 END, CASE WHEN kind = 'out' THEN delta ELSE 0 END, CASE WHEN kind = 'walkup' THEN delta ELSE 0 END
	    FROM door_counters WHERE event_id = $1 AND undone_at IS NULL
	  ) x GROUP BY m ORDER BY m`, eventID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (point, error) {
		var p point
		err := r.Scan(&p.At, &p.In, &p.Out, &p.Walkups)
		p.At = p.At.UTC()
		return p, err
	})
}

// ----------------------------------------------------------- list back ---

// ListBack is one allocation's guests with their door outcome, for sending
// back to the submitter.
type ListBack struct {
	Filename string
	Rows     []ListBackRow
	loc      *time.Location
}

// ListBack returns an allocation's guests (every status) with arrival
// data. The allocation must belong to the event. The export is audited as
// guestlist.export with counts only.
func (s *Service) ListBack(ctx context.Context, eventID, allocationID uuid.UUID, actor string) (ListBack, error) {
	var lb ListBack
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		ev, err := loadEvent(ctx, tx, eventID)
		if err != nil {
			return err
		}
		// Names of an erased event are gone; the report JSON still works.
		if err := retention.RefuseIfPurged(ctx, tx, eventID); err != nil {
			return err
		}
		var label string
		err = tx.QueryRow(ctx, `SELECT label FROM guest_allocations WHERE id = $1 AND event_id = $2`, allocationID, eventID).Scan(&label)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		lb.loc = ev.loc
		lb.Filename = filename(ev.Slug, label, allocationID)

		rows, err := tx.Query(ctx, `SELECT g.id, g.name_enc, g.plus_n, g.status,
		    COALESCE(sum(c.count), 0)::int, min(c.at)
		  FROM guests g
		  LEFT JOIN checkins c ON c.guest_id = g.id AND c.direction = 'in' AND c.undone_at IS NULL
		  WHERE g.event_id = $1 AND g.allocation_id = $2
		  GROUP BY g.id ORDER BY g.created_at, g.id`, eventID, allocationID)
		if err != nil {
			return err
		}
		type sealedRow struct {
			id     uuid.UUID
			sealed []byte
			row    ListBackRow
		}
		raw, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (sealedRow, error) {
			var x sealedRow
			var plusN int16
			err := r.Scan(&x.id, &x.sealed, &plusN, &x.row.Status, &x.row.HeadsAdmitted, &x.row.FirstIn)
			x.row.PlusN = int(plusN)
			return x, err
		})
		if err != nil {
			return err
		}
		tenant := tenantOf(ctx)
		lb.Rows = make([]ListBackRow, 0, len(raw))
		arrived := 0
		for _, x := range raw {
			dek, err := s.keys.ForSealed(ctx, tx, tenant, x.sealed)
			if err != nil {
				return err
			}
			name, err := dek.Open(tenant, envelope.Field{Table: "guests", Column: "name_enc", RowID: x.id}, x.sealed)
			if err != nil {
				return err
			}
			x.row.Name = string(name)
			if x.row.HeadsAdmitted >= 1 {
				arrived++
			}
			lb.Rows = append(lb.Rows, x.row)
		}
		return audit.Record(ctx, tx, tenant, audit.Entry{
			ActorID: actor, Action: "guestlist.export", Resource: "event:" + eventID.String(), Allowed: true,
			Reason: fmt.Sprintf("list-back allocation:%s, %d guests, %d arrived", allocationID, len(lb.Rows), arrived),
		})
	})
	return lb, err
}
