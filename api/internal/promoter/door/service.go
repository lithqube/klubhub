package door

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/guest"
	"github.com/klubhub/dj/api/internal/promoter/identity"
	"github.com/klubhub/dj/api/internal/promoter/retention"
)

// ManagerPINs verifies the event's manager PIN and hands out its offline
// verifier. identity.Service implements it; the door tables it reads belong
// to identity.
type ManagerPINs interface {
	VerifyManagerPIN(ctx context.Context, tx pgx.Tx, tenant, event uuid.UUID, pin string) (bool, error)
	ManagerPINCheck(ctx context.Context, tx pgx.Tx, tenant, event uuid.UUID) (*identity.PINCheck, error)
}

// Service implements the door bundle, check-in sync and adds.
type Service struct {
	db   *tenantdb.DB
	keys *envelope.Keyring
	pins ManagerPINs // nil: no manager PIN (adds are always refused)
	now  func() time.Time
}

// NewService builds the service; now defaults to time.Now.
func NewService(db *tenantdb.DB, keys *envelope.Keyring, pins ManagerPINs, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, keys: keys, pins: pins, now: now}
}

// lockEvent serialises door writes and cursor reads of one event and
// returns the database clock read after the lock. Every row a sync writes
// is stamped with it, so stamps follow commit order per event and a cursor
// (a stamp) never skips a row committed later.
func lockEvent(ctx context.Context, tx pgx.Tx, s Session) (time.Time, error) {
	var stamp time.Time
	err := tx.QueryRow(ctx, `SELECT clock_timestamp() FROM (SELECT pg_advisory_xact_lock(hashtextextended($1, 0))) l`,
		"door:"+s.Tenant.String()+":"+s.Event.String()).Scan(&stamp)
	return stamp.UTC(), err
}

func (s *Service) open(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, table, col string, row uuid.UUID, sealed []byte) (string, error) {
	if sealed == nil {
		return "", nil
	}
	dek, err := s.keys.ForSealed(ctx, tx, tenant, sealed)
	if err != nil {
		return "", err
	}
	plain, err := dek.Open(tenant, envelope.Field{Table: table, Column: col, RowID: row}, sealed)
	return string(plain), err
}

// emit enqueues an identifiers-only outbox event; counts go to the audit log.
func (s *Service) emit(ctx context.Context, tx pgx.Tx, sess Session, verb string, refs map[string]uuid.UUID) error {
	subject, ev, err := events.New(sess.Tenant, "door", verb, refs, s.now())
	if err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, subject, ev)
}

func touchDevice(ctx context.Context, tx pgx.Tx, sess Session, now time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE door_devices SET last_seen_at = $2 WHERE id = $1`, sess.Device, now)
	return err
}

// ---------------------------------------------------------------- bundle ---

// BundleEvent is the event as the door shows it.
type BundleEvent struct {
	ID       uuid.UUID  `json:"id"`
	Title    string     `json:"title"`
	StartsAt time.Time  `json:"starts_at"`
	EndsAt   time.Time  `json:"ends_at"`
	DoorsAt  *time.Time `json:"doors_at"`
	Timezone string     `json:"timezone"`
	Capacity *int       `json:"capacity"`
}

// EntryTerms of a list, as the door card shows them.
type EntryTerms struct {
	PriceMode        string     `json:"price_mode"`
	ReducedPriceText string     `json:"reduced_price_text"`
	CutoffAt         *time.Time `json:"cutoff_at"`
	Perks            []string   `json:"perks"`
}

// BundleList is one guest list of the event.
type BundleList struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	EntryTerms EntryTerms `json:"entry_terms"`
}

// BundleGuest is a guest, name-only: no email or phone reaches the door.
type BundleGuest struct {
	ID     uuid.UUID `json:"id"`
	ListID uuid.UUID `json:"list_id"`
	Name   string    `json:"name"`
	PlusN  int       `json:"plus_n"`
	Status string    `json:"status"`
	Note   string    `json:"note"`
}

// BundleTicket is an imported ticket with its secret (the QR payload), so
// scans match offline. Secret is "" when the export had none.
type BundleTicket struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	TicketType string    `json:"ticket_type"`
	OrderRef   string    `json:"order_ref"`
	Source     string    `json:"source"`
	Status     string    `json:"status"`
	Secret     string    `json:"secret"`
}

// Bundle is everything a door device needs to work offline for one event.
type Bundle struct {
	GeneratedAt      time.Time          `json:"generated_at"`
	DeviceID         uuid.UUID          `json:"device_id"`
	SessionExpiresAt time.Time          `json:"session_expires_at"`
	Event            BundleEvent        `json:"event"`
	Lists            []BundleList       `json:"lists"`
	Guests           []BundleGuest      `json:"guests"`
	Tickets          []BundleTicket     `json:"tickets"`
	Checkins         []Checkin          `json:"checkins"`
	Counters         Counters           `json:"counters"`
	Cursor           string             `json:"cursor"`
	ManagerPIN       *identity.PINCheck `json:"manager_pin"`
}

// Bundle builds the session's event bundle and audits the download.
func (s *Service) Bundle(ctx context.Context, sess Session) (Bundle, error) {
	b := Bundle{DeviceID: sess.Device, Lists: []BundleList{}, Guests: []BundleGuest{}, Tickets: []BundleTicket{}}
	err := s.db.WithTenant(ctx, sess.Tenant, func(tx pgx.Tx) error {
		stamp, err := lockEvent(ctx, tx, sess)
		if err != nil {
			return err
		}
		b.GeneratedAt, b.Cursor = stamp, encodeCursor(stamp)
		e := &b.Event
		err = tx.QueryRow(ctx, `SELECT id, title, starts_at, ends_at, doors_at, timezone, capacity FROM events WHERE id = $1`, sess.Event).
			Scan(&e.ID, &e.Title, &e.StartsAt, &e.EndsAt, &e.DoorsAt, &e.Timezone, &e.Capacity)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// An erased event has no names to hand out (P2.5).
		if err := retention.RefuseIfPurged(ctx, tx, sess.Event); err != nil {
			return err
		}
		// The principal does not carry its session, so this is the latest
		// expiry among the device's live sessions for the event (door
		// sessions end with the staff PIN window they were opened in).
		var expires *time.Time
		if err := tx.QueryRow(ctx, `SELECT max(expires_at) FROM sessions
		  WHERE device_id = $1 AND event_scope = $2 AND revoked_at IS NULL AND expires_at > $3`,
			sess.Device, sess.Event, s.now()).Scan(&expires); err != nil {
			return err
		}
		b.SessionExpiresAt = stamp
		if expires != nil {
			b.SessionExpiresAt = expires.UTC()
		}
		if b.Lists, err = bundleLists(ctx, tx, sess.Event); err != nil {
			return err
		}
		if b.Guests, err = s.bundleGuests(ctx, tx, sess); err != nil {
			return err
		}
		if b.Tickets, err = s.bundleTickets(ctx, tx, sess); err != nil {
			return err
		}
		if b.Checkins, err = checkinsSince(ctx, tx, sess.Event, nil); err != nil {
			return err
		}
		if b.Counters, err = counters(ctx, tx, sess.Event); err != nil {
			return err
		}
		if s.pins != nil {
			if b.ManagerPIN, err = s.pins.ManagerPINCheck(ctx, tx, sess.Tenant, sess.Event); err != nil {
				return err
			}
		}
		if err := touchDevice(ctx, tx, sess, s.now()); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, sess.Tenant, audit.Entry{
			ActorID: sess.Sub, Action: "door.bundle_downloaded", Resource: "event:" + sess.Event.String(), Allowed: true,
			Reason: fmt.Sprintf("%d lists, %d guests, %d tickets, %d check-ins", len(b.Lists), len(b.Guests), len(b.Tickets), len(b.Checkins)),
		}); err != nil {
			return err
		}
		return s.emit(ctx, tx, sess, "bundle_downloaded", map[string]uuid.UUID{"event_id": sess.Event, "device_id": sess.Device})
	})
	if err != nil {
		return Bundle{}, err
	}
	return b, nil
}

func bundleLists(ctx context.Context, tx pgx.Tx, event uuid.UUID) ([]BundleList, error) {
	rows, err := tx.Query(ctx, `SELECT id, name, type, entry_terms FROM guest_lists WHERE event_id = $1 ORDER BY position, created_at`, event)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (BundleList, error) {
		var l BundleList
		var terms []byte
		if err := r.Scan(&l.ID, &l.Name, &l.Type, &terms); err != nil {
			return l, err
		}
		err := json.Unmarshal(terms, &l.EntryTerms)
		if l.EntryTerms.PriceMode == "" {
			l.EntryTerms.PriceMode = guest.PriceFree
		}
		if l.EntryTerms.Perks == nil {
			l.EntryTerms.Perks = []string{}
		}
		return l, err
	})
	if out == nil {
		out = []BundleList{}
	}
	return out, err
}

func (s *Service) bundleGuests(ctx context.Context, tx pgx.Tx, sess Session) ([]BundleGuest, error) {
	rows, err := tx.Query(ctx, `SELECT id, list_id, name_enc, note_enc, plus_n, status FROM guests
	  WHERE event_id = $1 AND status <> 'declined' ORDER BY created_at, id`, sess.Event)
	if err != nil {
		return nil, err
	}
	type raw struct {
		BundleGuest
		name, note []byte
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (raw, error) {
		var x raw
		var plus int16
		err := r.Scan(&x.ID, &x.ListID, &x.name, &x.note, &plus, &x.Status)
		x.PlusN = int(plus)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out := make([]BundleGuest, len(list))
	for i, x := range list {
		out[i] = x.BundleGuest
		if out[i].Name, err = s.open(ctx, tx, sess.Tenant, "guests", "name_enc", x.ID, x.name); err != nil {
			return nil, err
		}
		if out[i].Note, err = s.open(ctx, tx, sess.Tenant, "guests", "note_enc", x.ID, x.note); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) bundleTickets(ctx context.Context, tx pgx.Tx, sess Session) ([]BundleTicket, error) {
	rows, err := tx.Query(ctx, `SELECT p.id, p.attendee_name_enc, p.secret_enc, t.name, o.external_ref, o.source, p.status
	  FROM order_positions p JOIN orders o ON o.id = p.order_id JOIN ticket_types t ON t.id = p.ticket_type_id
	  WHERE p.event_id = $1 ORDER BY o.external_ref, p.external_ref`, sess.Event)
	if err != nil {
		return nil, err
	}
	type raw struct {
		BundleTicket
		name, secret []byte
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (raw, error) {
		var x raw
		err := r.Scan(&x.ID, &x.name, &x.secret, &x.TicketType, &x.OrderRef, &x.Source, &x.Status)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out := make([]BundleTicket, len(list))
	for i, x := range list {
		out[i] = x.BundleTicket
		if out[i].Name, err = s.open(ctx, tx, sess.Tenant, "order_positions", "attendee_name_enc", x.ID, x.name); err != nil {
			return nil, err
		}
		if out[i].Secret, err = s.open(ctx, tx, sess.Tenant, "order_positions", "secret_enc", x.ID, x.secret); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// checkinsSince returns the event's check-ins received or undone after
// since (all of them when since is nil), in arrival order.
func checkinsSince(ctx context.Context, tx pgx.Tx, event uuid.UUID, since *time.Time) ([]Checkin, error) {
	rows, err := tx.Query(ctx, `SELECT client_nonce, guest_id, position_id, count, direction, at, device_id, undone_at IS NOT NULL, conflict_of IS NOT NULL
	  FROM checkins WHERE event_id = $1 AND ($2::timestamptz IS NULL OR received_at > $2 OR undone_at > $2)
	  ORDER BY received_at, id`, event, since)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Checkin, error) {
		var c Checkin
		var guestID, positionID *uuid.UUID
		var count int16
		err := r.Scan(&c.Nonce, &guestID, &positionID, &count, &c.Direction, &c.At, &c.DeviceID, &c.Undone, &c.Conflict)
		c.Count = int(count)
		c.At = c.At.UTC()
		if guestID != nil {
			c.Subject = Subject{Kind: KindGuest, ID: guestID.String()}
		} else if positionID != nil {
			c.Subject = Subject{Kind: KindTicket, ID: positionID.String()}
		}
		return c, err
	})
	if out == nil {
		out = []Checkin{}
	}
	return out, err
}

func counters(ctx context.Context, tx pgx.Tx, event uuid.UUID) (Counters, error) {
	var c Counters
	err := tx.QueryRow(ctx, `SELECT
	    COALESCE(sum(delta) FILTER (WHERE kind = 'walkup'), 0),
	    COALESCE(sum(delta) FILTER (WHERE kind = 'in'), 0),
	    COALESCE(sum(delta) FILTER (WHERE kind = 'out'), 0)
	  FROM door_counters WHERE event_id = $1 AND undone_at IS NULL`, event).Scan(&c.Walkups, &c.ManualIn, &c.ManualOut)
	return c, err
}

// ------------------------------------------------------------------ sync ---

type subjectInfo struct {
	found     bool
	allowance int
}

// Sync applies ops in order in one transaction and returns their results
// with everything received since the cursor. A nonce seen before (in this
// or an earlier batch) is a duplicate and reports the original outcome.
func (s *Service) Sync(ctx context.Context, sess Session, in SyncInput) (SyncResult, error) {
	if len(in.Ops) > MaxOps {
		return SyncResult{}, invalid("ops", fmt.Sprintf("at most %d per request", MaxOps))
	}
	since, err := decodeCursor(in.Since)
	if err != nil {
		return SyncResult{}, err
	}
	res := SyncResult{Results: make([]Result, 0, len(in.Ops))}
	var applied, duplicates, rejected, conflicted int
	err = s.db.WithTenant(ctx, sess.Tenant, func(tx pgx.Tx) error {
		stamp, err := lockEvent(ctx, tx, sess)
		if err != nil {
			return err
		}
		seen, err := knownNonces(ctx, tx, in.Ops)
		if err != nil {
			return err
		}
		subjects := map[uuid.UUID]subjectInfo{}
		for _, op := range in.Ops {
			r := Result{Nonce: op.Nonce}
			p, problem := op.parse()
			switch {
			case problem != "":
				r.Status, r.Error = StatusRejected, ErrCodeInvalidOp+": "+problem
			case seen[op.Nonce] != nil:
				r.Status, r.Conflict = StatusDuplicate, *seen[op.Nonce]
			default:
				r, err = s.apply(ctx, tx, sess, p, stamp, subjects)
				if err != nil {
					return err
				}
				if r.Status == StatusApplied {
					c := r.Conflict
					seen[op.Nonce] = &c
				}
			}
			switch r.Status {
			case StatusApplied:
				applied++
			case StatusDuplicate:
				duplicates++
			default:
				rejected++
			}
			if r.Conflict && r.Status == StatusApplied {
				conflicted++
			}
			res.Results = append(res.Results, r)
		}
		if res.Checkins, err = checkinsSince(ctx, tx, sess.Event, since); err != nil {
			return err
		}
		if res.Counters, err = counters(ctx, tx, sess.Event); err != nil {
			return err
		}
		res.Cursor = encodeCursor(stamp)
		if err := touchDevice(ctx, tx, sess, s.now()); err != nil {
			return err
		}
		if len(in.Ops) == 0 {
			return nil // a pull-only sync is not audited
		}
		if err := audit.Record(ctx, tx, sess.Tenant, audit.Entry{
			ActorID: sess.Sub, Action: "door.checkins_synced", Resource: "event:" + sess.Event.String(), Allowed: true,
			Reason: fmt.Sprintf("%d ops: %d applied, %d duplicate, %d rejected, %d conflicts", len(in.Ops), applied, duplicates, rejected, conflicted),
		}); err != nil {
			return err
		}
		return s.emit(ctx, tx, sess, "checkins_synced", map[string]uuid.UUID{"event_id": sess.Event, "device_id": sess.Device})
	})
	if err != nil {
		return SyncResult{}, err
	}
	return res, nil
}

// knownNonces maps each nonce of ops that is already stored (as a check-in,
// counter or undo, anywhere in the tenant) to its conflict flag.
func knownNonces(ctx context.Context, tx pgx.Tx, ops []Op) (map[string]*bool, error) {
	seen := map[string]*bool{}
	nonces := make([]string, 0, len(ops))
	for _, op := range ops {
		if validNonce(op.Nonce) {
			nonces = append(nonces, op.Nonce)
		}
	}
	if len(nonces) == 0 {
		return seen, nil
	}
	rows, err := tx.Query(ctx, `SELECT client_nonce, conflict_of IS NOT NULL FROM checkins WHERE client_nonce = ANY($1)
	  UNION ALL SELECT undo_nonce, false FROM checkins WHERE undo_nonce = ANY($1)
	  UNION ALL SELECT client_nonce, false FROM door_counters WHERE client_nonce = ANY($1)
	  UNION ALL SELECT undo_nonce, false FROM door_counters WHERE undo_nonce = ANY($1)`, nonces)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		var c bool
		if err := rows.Scan(&n, &c); err != nil {
			return nil, err
		}
		seen[n] = &c
	}
	return seen, rows.Err()
}

func (s *Service) apply(ctx context.Context, tx pgx.Tx, sess Session, p parsedOp, stamp time.Time, subjects map[uuid.UUID]subjectInfo) (Result, error) {
	r := Result{Nonce: p.Nonce, Status: StatusApplied}
	switch p.Type {
	case OpCheckin:
		info, ok := subjects[p.subjectID]
		if !ok {
			var err error
			if info, err = subject(ctx, tx, sess.Event, p.Subject.Kind, p.subjectID); err != nil {
				return r, err
			}
			subjects[p.subjectID] = info
		}
		if !info.found {
			r.Status, r.Error = StatusRejected, ErrCodeUnknownSubject
			return r, nil
		}
		var conflictOf *uuid.UUID
		if p.Direction == DirIn {
			t, earliest, err := tallyOf(ctx, tx, sess.Device, p.subjectID)
			if err != nil {
				return r, err
			}
			if conflicts(info.allowance, p.Count, t) {
				conflictOf, r.Conflict = earliest, true
			}
		}
		var guestID, positionID *uuid.UUID
		if p.Subject.Kind == KindGuest {
			guestID = &p.subjectID
		} else {
			positionID = &p.subjectID
		}
		_, err := tx.Exec(ctx, `INSERT INTO checkins (id, tenant_id, event_id, guest_id, position_id, count, direction, client_nonce,
		  device_id, at, received_at, conflict_of) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			uuid.Must(uuid.NewV7()), sess.Tenant, sess.Event, guestID, positionID, p.Count, p.Direction, p.Nonce,
			sess.Device, p.at, stamp, conflictOf)
		return r, err
	case OpUndo:
		// The target must be a check-in or counter of this event. Undoing
		// something already undone changes nothing and still succeeds.
		for _, table := range []string{"checkins", "door_counters"} {
			var undone bool
			err := tx.QueryRow(ctx, `SELECT undone_at IS NOT NULL FROM `+table+` WHERE client_nonce = $1 AND event_id = $2 FOR UPDATE`,
				p.Target, sess.Event).Scan(&undone)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil || undone {
				return r, err
			}
			_, err = tx.Exec(ctx, `UPDATE `+table+` SET undone_at = $3, undo_nonce = $4 WHERE client_nonce = $1 AND event_id = $2`,
				p.Target, sess.Event, stamp, p.Nonce)
			return r, err
		}
		r.Status, r.Error = StatusRejected, ErrCodeUnknownTarget
		return r, nil
	default: // OpCounter
		_, err := tx.Exec(ctx, `INSERT INTO door_counters (id, tenant_id, event_id, device_id, kind, delta, client_nonce, at, received_at)
		  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			uuid.Must(uuid.NewV7()), sess.Tenant, sess.Event, sess.Device, p.Kind, p.Delta, p.Nonce, p.at, stamp)
		return r, err
	}
}

// subject checks that a guest or ticket belongs to the event and returns
// its allowance in heads. Any status is accepted: if the door let someone
// in, the check-in is a fact worth keeping.
func subject(ctx context.Context, tx pgx.Tx, event uuid.UUID, kind string, id uuid.UUID) (subjectInfo, error) {
	var info subjectInfo
	var err error
	if kind == KindGuest {
		var plus int16
		err = tx.QueryRow(ctx, `SELECT plus_n FROM guests WHERE id = $1 AND event_id = $2`, id, event).Scan(&plus)
		info.allowance = 1 + int(plus)
	} else {
		err = tx.QueryRow(ctx, `SELECT 1 FROM order_positions WHERE id = $1 AND event_id = $2`, id, event).Scan(&info.allowance)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return subjectInfo{}, nil
	}
	info.found = err == nil
	return info, err
}

// tallyOf sums the subject's live check-ins and finds the earliest live
// "in" from another device (the row a conflict points at).
func tallyOf(ctx context.Context, tx pgx.Tx, device, subjectID uuid.UUID) (tally, *uuid.UUID, error) {
	var t tally
	var earliest *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT
	    COALESCE(sum(count) FILTER (WHERE direction = 'in' AND device_id <> $2), 0),
	    COALESCE(sum(count) FILTER (WHERE direction = 'in' AND device_id = $2), 0),
	    COALESCE(sum(count) FILTER (WHERE direction = 'out'), 0),
	    (SELECT c.id FROM checkins c WHERE (c.guest_id = $1 OR c.position_id = $1) AND c.undone_at IS NULL
	       AND c.direction = 'in' AND c.device_id <> $2 ORDER BY c.at, c.received_at, c.id LIMIT 1)
	  FROM checkins WHERE (guest_id = $1 OR position_id = $1) AND undone_at IS NULL`, subjectID, device).
		Scan(&t.OtherIn, &t.OwnIn, &t.Out, &earliest)
	return t, earliest, err
}

// ------------------------------------------------------------------ adds ---

// Adds creates on-the-spot guests (source door, status going) with the ids
// the device generated. Each add carries the manager PIN, verified against
// the Argon2id hash; wrong PINs count towards the manager PIN's lockout.
// A batch verifies each distinct PIN once, so a device replaying a queue
// with one mistyped PIN spends one attempt, not one per add.
func (s *Service) Adds(ctx context.Context, sess Session, in AddsInput) (AddsResult, error) {
	if len(in.Adds) == 0 || len(in.Adds) > MaxAdds {
		return AddsResult{}, invalid("adds", fmt.Sprintf("1 to %d per request", MaxAdds))
	}
	res := AddsResult{Results: make([]Result, 0, len(in.Adds))}
	err := s.db.WithTenant(ctx, sess.Tenant, func(tx pgx.Tx) error {
		// Adds are personal data: refused for an erased event (P2.5).
		if err := retention.EnsureNotPurged(ctx, tx, sess.Event); err != nil {
			return err
		}
		dek, err := s.keys.Current(ctx, tx, sess.Tenant)
		if err != nil {
			return err
		}
		var total int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM guests WHERE event_id = $1`, sess.Event).Scan(&total); err != nil {
			return err
		}
		lists := map[uuid.UUID]bool{}
		pins := map[string]bool{}
		touched := map[uuid.UUID]bool{}
		added := 0
		now := s.now()
		for _, a := range in.Adds {
			p, problem := a.parse(guest.MaxNameLen, guest.MaxPlusN)
			r := Result{Nonce: a.Nonce, ID: a.ID}
			reject := func(code string) { r.Status, r.Error = StatusRejected, code }
			if problem != "" {
				reject(ErrCodeInvalidAdd + ": " + problem)
				res.Results = append(res.Results, r)
				continue
			}
			var existing uuid.UUID
			err := tx.QueryRow(ctx, `SELECT event_id FROM guests WHERE id = $1`, p.id).Scan(&existing)
			switch {
			case err == nil && existing == sess.Event:
				r.Status = StatusDuplicate
			case err == nil:
				reject(ErrCodeIDConflict)
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			default:
				r, err = s.addOne(ctx, tx, sess, dek, p, r, lists, pins, total+added, now)
				if err != nil {
					return err
				}
				if r.Status == StatusApplied {
					added++
					touched[p.listID] = true
				}
			}
			res.Results = append(res.Results, r)
		}
		if added == 0 {
			return nil
		}
		for listID := range touched {
			subject, ev, err := events.New(sess.Tenant, "guestlist", "updated", map[string]uuid.UUID{"event_id": sess.Event, "list_id": listID}, now)
			if err != nil {
				return err
			}
			if err := events.Enqueue(ctx, tx, subject, ev); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, sess.Tenant, audit.Entry{
			ActorID: sess.Sub, Action: "door.guests_added", Resource: "event:" + sess.Event.String(), Allowed: true,
			Reason: fmt.Sprintf("%d added at the door", added),
		})
	})
	if err != nil {
		return AddsResult{}, err
	}
	return res, nil
}

func (s *Service) addOne(ctx context.Context, tx pgx.Tx, sess Session, dek *envelope.DEK, p parsedAdd, r Result,
	lists map[uuid.UUID]bool, pins map[string]bool, guests int, now time.Time) (Result, error) {
	ok, known := lists[p.listID]
	if !known {
		err := tx.QueryRow(ctx, `SELECT true FROM guest_lists WHERE id = $1 AND event_id = $2`, p.listID, sess.Event).Scan(&ok)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return r, err
		}
		lists[p.listID] = ok
	}
	if !ok {
		r.Status, r.Error = StatusRejected, ErrCodeUnknownList
		return r, nil
	}
	verified, known := pins[p.ManagerPIN]
	if !known {
		if s.pins != nil {
			var err error
			if verified, err = s.pins.VerifyManagerPIN(ctx, tx, sess.Tenant, sess.Event, p.ManagerPIN); err != nil {
				return r, err
			}
		}
		pins[p.ManagerPIN] = verified
	}
	if !verified {
		r.Status, r.Error = StatusRejected, ErrCodeManagerPINInvalid
		return r, nil
	}
	if guests >= guest.MaxPerEvent {
		r.Status, r.Error = StatusRejected, ErrCodeEventFull
		return r, nil
	}
	nameEnc, err := dek.Seal(sess.Tenant, envelope.Field{Table: "guests", Column: "name_enc", RowID: p.id}, []byte(p.Name))
	if err != nil {
		return r, err
	}
	// ON CONFLICT covers an id taken by a row this tenant cannot see (row
	// level security hides other tenants): nothing is written or revealed.
	tag, err := tx.Exec(ctx, `INSERT INTO guests (id, tenant_id, event_id, list_id, name_enc, name_bidx, plus_n, status, source, created_by, created_at, updated_at)
	  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11) ON CONFLICT (id) DO NOTHING`,
		p.id, sess.Tenant, sess.Event, p.listID, nameEnc, guest.NameIndex(dek, p.Name), p.PlusN, guest.StatusGoing, guest.SourceDoor, sess.Sub, now)
	if err != nil {
		return r, err
	}
	if tag.RowsAffected() == 0 {
		r.Status, r.Error = StatusRejected, ErrCodeIDConflict
		return r, nil
	}
	r.Status = StatusApplied
	return r, nil
}
