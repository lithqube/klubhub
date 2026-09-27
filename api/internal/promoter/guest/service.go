package guest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/retention"
)

// Service implements lists, standing lists, allocations and guests. Every
// method runs in the tenant carried by ctx (set by the auth middleware).
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

// emit enqueues an outbox event. Refs are identifiers only: the bus never
// carries names, contacts or notes.
func (s *Service) emit(ctx context.Context, tx pgx.Tx, aggregate, verb string, refs map[string]uuid.UUID) error {
	subject, ev, err := events.New(tenantOf(ctx), aggregate, verb, refs, s.now())
	if err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, subject, ev)
}

func (s *Service) listUpdated(ctx context.Context, tx pgx.Tx, eventID, listID uuid.UUID) error {
	return s.emit(ctx, tx, "guestlist", "updated", map[string]uuid.UUID{"event_id": eventID, "list_id": listID})
}

// seal encrypts val for a guests/allocations column; empty stays NULL.
func seal(dek *envelope.DEK, tenant uuid.UUID, table, col string, row uuid.UUID, val string) ([]byte, error) {
	if val == "" {
		return nil, nil
	}
	return dek.Seal(tenant, envelope.Field{Table: table, Column: col, RowID: row}, []byte(val))
}

func (s *Service) open(ctx context.Context, tx pgx.Tx, table, col string, row uuid.UUID, sealed []byte) (string, error) {
	if sealed == nil {
		return "", nil
	}
	tenant := tenantOf(ctx)
	dek, err := s.keys.ForSealed(ctx, tx, tenant, sealed)
	if err != nil {
		return "", err
	}
	plain, err := dek.Open(tenant, envelope.Field{Table: table, Column: col, RowID: row}, sealed)
	return string(plain), err
}

type eventInfo struct {
	StartsAt time.Time
	EndsAt   time.Time
	Timezone string
	Slug     string
}

func eventOf(ctx context.Context, tx pgx.Tx, id uuid.UUID) (eventInfo, error) {
	var e eventInfo
	err := tx.QueryRow(ctx, `SELECT starts_at, ends_at, timezone, slug FROM events WHERE id = $1`, id).
		Scan(&e.StartsAt, &e.EndsAt, &e.Timezone, &e.Slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

// ---------------------------------------------------------------- lists ---

func termsJSON(t EntryTerms) ([]byte, error) {
	if t.Perks == nil {
		t.Perks = []string{}
	}
	return json.Marshal(t)
}

func checkCutoff(t EntryTerms, ev eventInfo) error {
	if t.CutoffAt != nil && (t.CutoffAt.After(ev.EndsAt) || t.CutoffAt.Before(ev.StartsAt.Add(-24*time.Hour))) {
		return invalid("entry_terms.cutoff_at", "must fall between the day before the start and the end of the event")
	}
	return nil
}

// ListLists returns an event's lists in order, with allocations and counts.
func (s *Service) ListLists(ctx context.Context, eventID uuid.UUID) ([]List, error) {
	out := []List{}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if _, err := eventOf(ctx, tx, eventID); err != nil {
			return err
		}
		var err error
		out, err = s.lists(ctx, tx, eventID, nil)
		return err
	})
	return out, err
}

func (s *Service) lists(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, only *uuid.UUID) ([]List, error) {
	rows, err := tx.Query(ctx, `SELECT l.id, l.event_id, l.name, l.type, l.entry_terms, l.collect_contact, l.standing_template_id, l.position,
	  (SELECT count(*) FROM guests g WHERE g.list_id = l.id AND g.status <> 'declined'),
	  (SELECT COALESCE(sum(1 + g.plus_n), 0) FROM guests g WHERE g.list_id = l.id AND g.status <> 'declined'),
	  (SELECT count(*) FROM guests g WHERE g.list_id = l.id AND g.status = 'pending'),
	  (SELECT COALESCE(sum(a.quota), 0) FROM guest_allocations a WHERE a.list_id = l.id AND a.revoked_at IS NULL)
	  FROM guest_lists l WHERE l.event_id = $1 AND ($2::uuid IS NULL OR l.id = $2) ORDER BY l.position, l.created_at`, eventID, only)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (List, error) {
		var l List
		var terms []byte
		var pos int16
		err := r.Scan(&l.ID, &l.EventID, &l.Name, &l.Type, &terms, &l.CollectContact, &l.StandingTemplateID, &pos,
			&l.Guests, &l.Heads, &l.Pending, &l.Quota)
		if err != nil {
			return l, err
		}
		l.Position = int(pos)
		err = json.Unmarshal(terms, &l.EntryTerms)
		if l.EntryTerms.Perks == nil {
			l.EntryTerms.Perks = []string{}
		}
		return l, err
	})
	if err != nil {
		return nil, err
	}
	allocs, err := s.allocations(ctx, tx, eventID, nil)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Allocations = []Allocation{}
		for _, a := range allocs {
			if a.ListID == out[i].ID {
				out[i].Allocations = append(out[i].Allocations, a)
			}
		}
	}
	if out == nil {
		out = []List{}
	}
	return out, nil
}

func (s *Service) oneList(ctx context.Context, tx pgx.Tx, eventID, listID uuid.UUID) (List, error) {
	ls, err := s.lists(ctx, tx, eventID, &listID)
	if err != nil {
		return List{}, err
	}
	if len(ls) == 0 {
		return List{}, ErrNotFound
	}
	return ls[0], nil
}

// CreateList adds a list at the end of the event's lists.
func (s *Service) CreateList(ctx context.Context, eventID uuid.UUID, in ListInput) (List, error) {
	if err := in.validate(false); err != nil {
		return List{}, err
	}
	id := uuid.Must(uuid.NewV7())
	var l List
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		ev, err := eventOf(ctx, tx, eventID)
		if err != nil {
			return err
		}
		if err := checkCutoff(in.EntryTerms, ev); err != nil {
			return err
		}
		terms, err := termsJSON(in.EntryTerms)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO guest_lists (id, tenant_id, event_id, name, type, entry_terms, collect_contact, position)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, (SELECT COALESCE(max(position) + 1, 0) FROM guest_lists WHERE event_id = $3))`,
			id, tenantOf(ctx), eventID, in.Name, in.Type, terms, in.CollectContact); err != nil {
			return err
		}
		if err := s.listUpdated(ctx, tx, eventID, id); err != nil {
			return err
		}
		l, err = s.oneList(ctx, tx, eventID, id)
		return err
	})
	return l, err
}

// UpdateList replaces a list's settings. Turning contact collection off
// erases the emails and phone numbers already stored on the list.
func (s *Service) UpdateList(ctx context.Context, eventID, listID uuid.UUID, in ListInput) (List, error) {
	if err := in.validate(false); err != nil {
		return List{}, err
	}
	var l List
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		ev, err := eventOf(ctx, tx, eventID)
		if err != nil {
			return err
		}
		if err := checkCutoff(in.EntryTerms, ev); err != nil {
			return err
		}
		terms, err := termsJSON(in.EntryTerms)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE guest_lists SET name = $3, type = $4, entry_terms = $5, collect_contact = $6, updated_at = now()
		  WHERE id = $1 AND event_id = $2`, listID, eventID, in.Name, in.Type, terms, in.CollectContact)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if !in.CollectContact {
			if _, err := tx.Exec(ctx, `UPDATE guests SET email_enc = NULL, phone_enc = NULL, email_bidx = NULL, updated_at = now()
			  WHERE list_id = $1 AND (email_enc IS NOT NULL OR phone_enc IS NOT NULL)`, listID); err != nil {
				return err
			}
		}
		if err := s.listUpdated(ctx, tx, eventID, listID); err != nil {
			return err
		}
		l, err = s.oneList(ctx, tx, eventID, listID)
		return err
	})
	return l, err
}

// DeleteList removes a list. A list with guests needs force, which deletes
// its guests too.
func (s *Service) DeleteList(ctx context.Context, eventID, listID uuid.UUID, force bool) error {
	return s.db.Run(ctx, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM guests WHERE list_id = $1 AND event_id = $2`, listID, eventID).Scan(&n); err != nil {
			return err
		}
		if n > 0 && !force {
			return &NotEmptyError{Guests: n}
		}
		tag, err := tx.Exec(ctx, `DELETE FROM guest_lists WHERE id = $1 AND event_id = $2`, listID, eventID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return s.listUpdated(ctx, tx, eventID, listID)
	})
}

// ------------------------------------------------------- standing lists ---

func scanStanding(r pgx.Row) (StandingList, error) {
	var l StandingList
	var terms []byte
	var pos int16
	if err := r.Scan(&l.ID, &l.Name, &l.Type, &terms, &l.CollectContact, &pos); err != nil {
		return l, err
	}
	l.Position = int(pos)
	err := json.Unmarshal(terms, &l.EntryTerms)
	if l.EntryTerms.Perks == nil {
		l.EntryTerms.Perks = []string{}
	}
	return l, err
}

const standingCols = `id, name, type, entry_terms, collect_contact, position`

// ListStanding returns the organisation's standing lists in order.
func (s *Service) ListStanding(ctx context.Context) ([]StandingList, error) {
	out := []StandingList{}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+standingCols+` FROM standing_lists ORDER BY position, created_at`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (StandingList, error) { return scanStanding(r) })
		return err
	})
	if out == nil {
		out = []StandingList{}
	}
	return out, err
}

// CreateStanding adds a template; it applies to events created afterwards.
func (s *Service) CreateStanding(ctx context.Context, in ListInput) (StandingList, error) {
	if err := in.validate(true); err != nil {
		return StandingList{}, err
	}
	id := uuid.Must(uuid.NewV7())
	var out StandingList
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		terms, err := termsJSON(in.EntryTerms)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO standing_lists (id, tenant_id, name, type, entry_terms, collect_contact, position)
		  VALUES ($1, $2, $3, $4, $5, $6, (SELECT COALESCE(max(position) + 1, 0) FROM standing_lists))`,
			id, tenantOf(ctx), in.Name, in.Type, terms, in.CollectContact); err != nil {
			return err
		}
		out, err = scanStanding(tx.QueryRow(ctx, `SELECT `+standingCols+` FROM standing_lists WHERE id = $1`, id))
		return err
	})
	return out, err
}

// UpdateStanding replaces a template. Lists already copied into events
// keep their own settings.
func (s *Service) UpdateStanding(ctx context.Context, id uuid.UUID, in ListInput) (StandingList, error) {
	if err := in.validate(true); err != nil {
		return StandingList{}, err
	}
	var out StandingList
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		terms, err := termsJSON(in.EntryTerms)
		if err != nil {
			return err
		}
		out, err = scanStanding(tx.QueryRow(ctx, `UPDATE standing_lists SET name = $2, type = $3, entry_terms = $4, collect_contact = $5,
		  updated_at = now() WHERE id = $1 RETURNING `+standingCols, id, in.Name, in.Type, terms, in.CollectContact))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	return out, err
}

// DeleteStanding removes a template; copies in events stay.
func (s *Service) DeleteStanding(ctx context.Context, id uuid.UUID) error {
	return s.db.Run(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM standing_lists WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
}

// CopyStandingLists is the event.CreateHook: it copies every standing list
// into a new event inside the event-creation transaction, turning local
// cutoff times into instants for that night.
func (s *Service) CopyStandingLists(ctx context.Context, tx pgx.Tx, ev event.Event) error {
	rows, err := tx.Query(ctx, `SELECT `+standingCols+` FROM standing_lists ORDER BY position, created_at`)
	if err != nil {
		return err
	}
	templates, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (StandingList, error) { return scanStanding(r) })
	if err != nil {
		return err
	}
	for i, t := range templates {
		terms := t.EntryTerms
		if terms.CutoffLocal != nil {
			c, err := CutoffFor(ev.StartsAt, ev.Timezone, *terms.CutoffLocal)
			if err != nil {
				return err
			}
			terms.CutoffAt, terms.CutoffLocal = &c, nil
		}
		raw, err := termsJSON(terms)
		if err != nil {
			return err
		}
		id := uuid.Must(uuid.NewV7())
		if _, err := tx.Exec(ctx, `INSERT INTO guest_lists (id, tenant_id, event_id, name, type, entry_terms, collect_contact, standing_template_id, position)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id, tenantOf(ctx), ev.ID, t.Name, t.Type, raw, t.CollectContact, t.ID, i); err != nil {
			return err
		}
		if err := s.listUpdated(ctx, tx, ev.ID, id); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------- allocations ---

const allocCols = `a.id, a.list_id, a.label, a.submitter_contact_enc, a.quota, a.plus_n_max, a.deadline, a.requires_approval, a.revoked_at,
	(SELECT COALESCE(sum(1 + g.plus_n), 0) FROM guests g WHERE g.allocation_id = a.id AND g.status IN ('going', 'pending', 'invited')),
	(SELECT count(*) FROM guests g WHERE g.allocation_id = a.id AND g.status <> 'declined'),
	(SELECT count(*) FROM guests g WHERE g.allocation_id = a.id AND g.status = 'pending')`

func (s *Service) allocations(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, only *uuid.UUID) ([]Allocation, error) {
	rows, err := tx.Query(ctx, `SELECT `+allocCols+` FROM guest_allocations a
	  WHERE a.event_id = $1 AND ($2::uuid IS NULL OR a.list_id = $2) ORDER BY a.revoked_at NULLS FIRST, a.created_at`, eventID, only)
	if err != nil {
		return nil, err
	}
	type raw struct {
		Allocation
		contact []byte
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (raw, error) {
		var x raw
		var plus int16
		err := r.Scan(&x.ID, &x.ListID, &x.Label, &x.contact, &x.Quota, &plus, &x.Deadline, &x.RequiresApproval, &x.RevokedAt,
			&x.Used, &x.Guests, &x.Pending)
		x.PlusNMax = int(plus)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out := make([]Allocation, len(list))
	for i, x := range list {
		out[i] = x.Allocation
		if out[i].SubmitterContact, err = s.open(ctx, tx, "guest_allocations", "submitter_contact_enc", x.ID, x.contact); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) oneAllocation(ctx context.Context, tx pgx.Tx, eventID, listID, id uuid.UUID) (Allocation, error) {
	all, err := s.allocations(ctx, tx, eventID, &listID)
	if err != nil {
		return Allocation{}, err
	}
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
	}
	return Allocation{}, ErrNotFound
}

func listExists(ctx context.Context, tx pgx.Tx, eventID, listID uuid.UUID) error {
	err := tx.QueryRow(ctx, `SELECT 1 FROM guest_lists WHERE id = $1 AND event_id = $2`, listID, eventID).Scan(new(int))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ListAllocations returns a list's allocations (active first).
func (s *Service) ListAllocations(ctx context.Context, eventID, listID uuid.UUID) ([]Allocation, error) {
	out := []Allocation{}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if err := listExists(ctx, tx, eventID, listID); err != nil {
			return err
		}
		var err error
		out, err = s.allocations(ctx, tx, eventID, &listID)
		return err
	})
	return out, err
}

// contactAllowed refuses a submitter contact (personal data) for an erased
// event. Called before any allocation row is locked: purges lock the event
// first.
func contactAllowed(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, contact *string) error {
	if contact == nil || strings.TrimSpace(*contact) == "" {
		return nil
	}
	return retention.EnsureNotPurged(ctx, tx, eventID)
}

func (s *Service) writeContact(ctx context.Context, tx pgx.Tx, id uuid.UUID, contact *string) error {
	if contact == nil {
		return nil
	}
	tenant := tenantOf(ctx)
	dek, err := s.keys.Current(ctx, tx, tenant)
	if err != nil {
		return err
	}
	sealed, err := seal(dek, tenant, "guest_allocations", "submitter_contact_enc", id, strings.TrimSpace(*contact))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE guest_allocations SET submitter_contact_enc = $2 WHERE id = $1`, id, sealed)
	return err
}

// CreateAllocation gives a submitter a share of a list.
func (s *Service) CreateAllocation(ctx context.Context, eventID, listID uuid.UUID, in AllocationInput) (Allocation, error) {
	if err := in.validate(); err != nil {
		return Allocation{}, err
	}
	id := uuid.Must(uuid.NewV7())
	var a Allocation
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if err := contactAllowed(ctx, tx, eventID, in.SubmitterContact); err != nil {
			return err
		}
		if err := listExists(ctx, tx, eventID, listID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO guest_allocations (id, tenant_id, event_id, list_id, label, quota, plus_n_max, deadline, requires_approval)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id, tenantOf(ctx), eventID, listID, in.Label, in.Quota, in.PlusNMax, in.Deadline, in.RequiresApproval); err != nil {
			return err
		}
		if err := s.writeContact(ctx, tx, id, in.SubmitterContact); err != nil {
			return err
		}
		if err := s.listUpdated(ctx, tx, eventID, listID); err != nil {
			return err
		}
		var err error
		a, err = s.oneAllocation(ctx, tx, eventID, listID, id)
		return err
	})
	return a, err
}

// UpdateAllocation replaces an allocation. A quota below the heads already
// on it is refused; lowering +N does not touch existing guests.
func (s *Service) UpdateAllocation(ctx context.Context, eventID, listID, id uuid.UUID, in AllocationInput) (Allocation, error) {
	if err := in.validate(); err != nil {
		return Allocation{}, err
	}
	var a Allocation
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if err := contactAllowed(ctx, tx, eventID, in.SubmitterContact); err != nil {
			return err
		}
		cur, err := s.oneAllocation(ctx, tx, eventID, listID, id)
		if err != nil {
			return err
		}
		if in.Quota < cur.Used {
			return invalid("quota", "lower than the guests already on it; decline or move some first")
		}
		if _, err := tx.Exec(ctx, `UPDATE guest_allocations SET label = $2, quota = $3, plus_n_max = $4, deadline = $5,
		  requires_approval = $6, updated_at = now() WHERE id = $1`,
			id, in.Label, in.Quota, in.PlusNMax, in.Deadline, in.RequiresApproval); err != nil {
			return err
		}
		if err := s.writeContact(ctx, tx, id, in.SubmitterContact); err != nil {
			return err
		}
		if err := s.listUpdated(ctx, tx, eventID, listID); err != nil {
			return err
		}
		a, err = s.oneAllocation(ctx, tx, eventID, listID, id)
		return err
	})
	return a, err
}

// RevokeAllocation closes an allocation: no new guests. Guests already on
// it stay, so the door is not surprised; staff decline them if needed.
func (s *Service) RevokeAllocation(ctx context.Context, eventID, listID, id uuid.UUID) error {
	return s.db.Run(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE guest_allocations SET revoked_at = COALESCE(revoked_at, $4), updated_at = now()
		  WHERE id = $1 AND list_id = $2 AND event_id = $3`, id, listID, eventID, s.now())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return s.listUpdated(ctx, tx, eventID, listID)
	})
}
