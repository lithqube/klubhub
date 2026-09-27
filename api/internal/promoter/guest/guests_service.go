package guest

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/promoter/retention"
)

// Blind indexes are scoped to the guests table.
func emailIndex(dek *envelope.DEK, email string) []byte {
	if email == "" {
		return nil
	}
	return dek.BlindIndex("guests", "email_bidx", envelope.NormalizeEmail(email))
}

func nameIndex(dek *envelope.DEK, name string) []byte {
	return dek.BlindIndex("guests", "name_bidx", envelope.NormalizeName(name))
}

// NameIndex is the guests.name_bidx blind index of name. The door package
// uses it for on-the-spot adds, so exact lookups and dedupe keep working.
func NameIndex(dek *envelope.DEK, name string) []byte { return nameIndex(dek, name) }

type listInfo struct {
	CollectContact bool
	Name           string
}

func listOf(ctx context.Context, tx pgx.Tx, eventID, listID uuid.UUID) (listInfo, error) {
	var l listInfo
	err := tx.QueryRow(ctx, `SELECT collect_contact, name FROM guest_lists WHERE id = $1 AND event_id = $2`, listID, eventID).
		Scan(&l.CollectContact, &l.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

type allocInfo struct {
	ID               uuid.UUID
	ListID           uuid.UUID
	Label            string
	Quota            int
	PlusNMax         int
	Deadline         *time.Time
	RequiresApproval bool
	RevokedAt        *time.Time
	Used             int // heads holding quota
}

// allocFor loads an allocation with its used heads, excluding one guest
// (the guest being edited) from the count.
func allocFor(ctx context.Context, tx pgx.Tx, id uuid.UUID, exclude *uuid.UUID) (allocInfo, error) {
	var a allocInfo
	var plus int16
	err := tx.QueryRow(ctx, `SELECT a.id, a.list_id, a.label, a.quota, a.plus_n_max, a.deadline, a.requires_approval, a.revoked_at,
	  (SELECT COALESCE(sum(1 + g.plus_n), 0) FROM guests g WHERE g.allocation_id = a.id
	     AND g.status IN ('going', 'pending', 'invited') AND ($2::uuid IS NULL OR g.id <> $2))
	  FROM guest_allocations a WHERE a.id = $1 FOR UPDATE OF a`, id, exclude).
		Scan(&a.ID, &a.ListID, &a.Label, &a.Quota, &plus, &a.Deadline, &a.RequiresApproval, &a.RevokedAt, &a.Used)
	a.PlusNMax = int(plus)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (a allocInfo) checkQuota(requested int) error {
	if a.Used+requested > a.Quota {
		return &QuotaError{AllocationID: a.ID, Label: a.Label, Quota: a.Quota, Used: a.Used, Requested: requested}
	}
	return nil
}

// AddGuests adds guests to a list. Under an allocation the server enforces
// revoke, deadline, +N and quota; new guests default to pending when the
// allocation needs approval and to going otherwise. An explicit status
// (e.g. "add directly as Going") overrides the default. Inputs that repeat
// an email already in the event, or a name already on the list (name-only
// guests), are skipped and reported as duplicates.
func (s *Service) AddGuests(ctx context.Context, eventID uuid.UUID, in AddInput, actor string) (AddResult, error) {
	res := AddResult{Added: []Guest{}, Duplicates: []int{}}
	if in.Source == "" {
		in.Source = SourceManual
	}
	switch {
	case in.Source != SourceManual && in.Source != SourcePaste:
		return res, invalid("source", "manual or paste")
	case len(in.Guests) == 0 || len(in.Guests) > MaxBatch:
		return res, invalid("guests", "1 to 500 per request")
	}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if _, err := eventOf(ctx, tx, eventID); err != nil {
			return err
		}
		if err := retention.EnsureNotPurged(ctx, tx, eventID); err != nil {
			return err
		}
		list, err := listOf(ctx, tx, eventID, in.ListID)
		if err != nil {
			return err
		}
		for i := range in.Guests {
			if err := in.Guests[i].validate(fieldOf(i), list.CollectContact); err != nil {
				return err
			}
		}
		var alloc *allocInfo
		if in.AllocationID != nil {
			a, err := allocFor(ctx, tx, *in.AllocationID, nil)
			if err != nil || a.ListID != in.ListID {
				return invalid("allocation_id", "not an allocation of this list")
			}
			switch {
			case a.RevokedAt != nil:
				return ErrAllocationRevoked
			case a.Deadline != nil && s.now().After(*a.Deadline):
				return ErrAllocationClosed
			}
			for i, g := range in.Guests {
				if g.PlusN > a.PlusNMax {
					return invalid(fieldOf(i)+"plus_n", "more than this allocation allows")
				}
			}
			alloc = &a
		}

		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		type prepared struct {
			GuestInput
			idx   int
			email []byte
			name  []byte
		}
		var emails, names [][]byte
		batch := make([]prepared, len(in.Guests))
		for i, g := range in.Guests {
			batch[i] = prepared{GuestInput: g, idx: i, email: emailIndex(dek, g.Email), name: nameIndex(dek, g.Name)}
			if batch[i].email != nil {
				emails = append(emails, batch[i].email)
			} else {
				names = append(names, batch[i].name)
			}
		}
		seen := map[string]bool{}
		if err := collectKeys(ctx, tx, seen, "e", `SELECT email_bidx FROM guests WHERE event_id = $1 AND email_bidx = ANY($2)`, eventID, emails); err != nil {
			return err
		}
		if err := collectKeys(ctx, tx, seen, "n", `SELECT name_bidx FROM guests WHERE list_id = $1 AND email_bidx IS NULL AND name_bidx = ANY($2)`, in.ListID, names); err != nil {
			return err
		}
		fresh := make([]prepared, 0, len(batch))
		for _, p := range batch {
			key := "n" + string(p.name)
			if p.email != nil {
				key = "e" + string(p.email)
			}
			if seen[key] {
				res.Duplicates = append(res.Duplicates, p.idx)
				continue
			}
			seen[key] = true
			if p.Status == "" {
				p.Status = StatusGoing
				if alloc != nil && alloc.RequiresApproval {
					p.Status = StatusPending
				}
			}
			fresh = append(fresh, p)
		}
		if len(fresh) == 0 {
			return nil
		}

		var total int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM guests WHERE event_id = $1`, eventID).Scan(&total); err != nil {
			return err
		}
		if total+len(fresh) > MaxPerEvent {
			return invalid("guests", "an event holds at most 5000 guests")
		}
		if alloc != nil {
			heads := 0
			for _, p := range fresh {
				if consumes(p.Status) {
					heads += 1 + p.PlusN
				}
			}
			if err := alloc.checkQuota(heads); err != nil {
				return err
			}
		}

		now := s.now()
		for _, p := range fresh {
			id := uuid.Must(uuid.NewV7())
			sealed, err := sealGuest(dek, tenant, id, p.GuestInput)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO guests (id, tenant_id, event_id, list_id, allocation_id, name_enc, email_enc, phone_enc, note_enc,
			  name_bidx, email_bidx, plus_n, status, source, created_by, created_at, updated_at)
			  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)`,
				id, tenant, eventID, in.ListID, in.AllocationID, sealed[0], sealed[1], sealed[2], sealed[3],
				p.name, p.email, p.PlusN, p.Status, in.Source, actor, now); err != nil {
				return err
			}
			res.Added = append(res.Added, Guest{
				ID: id, ListID: in.ListID, AllocationID: in.AllocationID, Name: p.Name, Email: p.Email, Phone: p.Phone,
				Note: p.Note, PlusN: p.PlusN, Status: p.Status, Source: in.Source, CreatedAt: now, UpdatedAt: now,
			})
		}
		return s.listUpdated(ctx, tx, eventID, in.ListID)
	})
	if err != nil {
		return AddResult{}, err
	}
	return res, nil
}

func fieldOf(i int) string { return "guests[" + strconv.Itoa(i) + "]." }

func collectKeys(ctx context.Context, tx pgx.Tx, into map[string]bool, prefix, sql string, scope uuid.UUID, keys [][]byte) error {
	if len(keys) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, sql, scope, keys)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k []byte
		if err := rows.Scan(&k); err != nil {
			return err
		}
		into[prefix+string(k)] = true
	}
	return rows.Err()
}

// sealGuest seals name, email, phone and note (in that order).
func sealGuest(dek *envelope.DEK, tenant, id uuid.UUID, g GuestInput) ([4][]byte, error) {
	var out [4][]byte
	for i, f := range []struct{ col, val string }{{"name_enc", g.Name}, {"email_enc", g.Email}, {"phone_enc", g.Phone}, {"note_enc", g.Note}} {
		v, err := seal(dek, tenant, "guests", f.col, id, f.val)
		if err != nil {
			return out, err
		}
		out[i] = v
	}
	return out, nil
}

type guestRow struct {
	Guest
	sealed [4][]byte
}

const guestCols = `g.id, g.list_id, g.allocation_id, g.name_enc, g.email_enc, g.phone_enc, g.note_enc, g.plus_n, g.status, g.source, g.created_at, g.updated_at,
  g.purged_at IS NOT NULL`

// scanGuest scans guestCols; with arrivals it also scans the heads_in and
// first_in_at columns that follow them.
func scanGuest(r pgx.Row, arrivals ...bool) (guestRow, error) {
	var x guestRow
	var plus int16
	dst := []any{&x.ID, &x.ListID, &x.AllocationID, &x.sealed[0], &x.sealed[1], &x.sealed[2], &x.sealed[3], &plus,
		&x.Status, &x.Source, &x.CreatedAt, &x.UpdatedAt, &x.Purged}
	if len(arrivals) > 0 && arrivals[0] {
		dst = append(dst, &x.HeadsIn, &x.FirstInAt)
	}
	err := r.Scan(dst...)
	x.PlusN = int(plus)
	return x, err
}

// guestArrivals aggregates an event's live check-ins ($1) per guest:
// heads_in = Σ in − Σ out (floored at 0) and the earliest `in`. Undone rows
// never count.
const guestArrivals = `SELECT guest_id,
    GREATEST(COALESCE(sum(count) FILTER (WHERE direction = 'in'), 0) - COALESCE(sum(count) FILTER (WHERE direction = 'out'), 0), 0)::int AS heads_in,
    min(at) FILTER (WHERE direction = 'in') AS first_in_at
  FROM checkins WHERE event_id = $1 AND guest_id IS NOT NULL AND undone_at IS NULL GROUP BY guest_id`

// arrivalOf is guestArrivals for one guest.
func arrivalOf(ctx context.Context, tx pgx.Tx, guestID uuid.UUID) (int, *time.Time, error) {
	var heads int
	var first *time.Time
	err := tx.QueryRow(ctx, `SELECT
	    GREATEST(COALESCE(sum(count) FILTER (WHERE direction = 'in'), 0) - COALESCE(sum(count) FILTER (WHERE direction = 'out'), 0), 0)::int,
	    min(at) FILTER (WHERE direction = 'in')
	  FROM checkins WHERE guest_id = $1 AND undone_at IS NULL`, guestID).Scan(&heads, &first)
	return heads, first, err
}

func (s *Service) decrypt(ctx context.Context, tx pgx.Tx, x guestRow) (Guest, error) {
	g := x.Guest
	dst := []*string{&g.Name, &g.Email, &g.Phone, &g.Note}
	for i, col := range []string{"name_enc", "email_enc", "phone_enc", "note_enc"} {
		v, err := s.open(ctx, tx, "guests", col, g.ID, x.sealed[i])
		if err != nil {
			return g, err
		}
		*dst[i] = v
	}
	return g, nil
}

// validate checks the filter; checkedIn allows the derived StatusCheckedIn
// (the guest table does, the CSV export does not).
func (f *GuestFilter) validate(checkedIn bool) error {
	if f.Status != "" && !slices.Contains(Statuses, f.Status) && (!checkedIn || f.Status != StatusCheckedIn) {
		allowed := Statuses
		if checkedIn {
			allowed = append(slices.Clone(Statuses), StatusCheckedIn)
		}
		return invalid("status", strings.Join(allowed, ", "))
	}
	f.Q = strings.TrimSpace(f.Q)
	if len(f.Q) > maxEmailLen {
		return invalid("q", "too long")
	}
	return nil
}

// ListGuests returns the guest table for an event and the tab counts. The
// counts respect the list filter but not the status filter, so every tab
// shows its own number. Guests and tickets carry their live check-in state;
// status=checked_in lists guests with heads in (any status).
func (s *Service) ListGuests(ctx context.Context, eventID uuid.UUID, f GuestFilter) (GuestPage, error) {
	page := GuestPage{Guests: []Guest{}, Tickets: []Ticket{}}
	if err := f.validate(true); err != nil {
		return page, err
	}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if _, err := eventOf(ctx, tx, eventID); err != nil {
			return err
		}
		rows, err := s.query(ctx, tx, eventID, f)
		if err != nil {
			return err
		}
		for _, x := range rows {
			g, err := s.decrypt(ctx, tx, x)
			if err != nil {
				return err
			}
			page.Guests = append(page.Guests, g)
		}
		page.Counts, err = counts(ctx, tx, eventID, f.ListID)
		if err != nil || f.Status != "" || f.ListID != nil {
			return err
		}
		page.Tickets, err = s.tickets(ctx, tx, eventID, f.Q)
		return err
	})
	return page, err
}

func (s *Service) query(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, f GuestFilter) ([]guestRow, error) {
	var emailBidx, nameBidx []byte
	if f.Q != "" {
		dek, err := s.keys.Current(ctx, tx, tenantOf(ctx))
		if err != nil {
			return nil, err
		}
		if strings.Contains(f.Q, "@") {
			emailBidx = emailIndex(dek, f.Q)
		} else {
			nameBidx = nameIndex(dek, f.Q)
		}
	}
	var status *string
	arrived := f.Status == StatusCheckedIn
	if f.Status != "" && !arrived {
		status = &f.Status
	}
	rows, err := tx.Query(ctx, `WITH a AS (`+guestArrivals+`)
	  SELECT `+guestCols+`, COALESCE(a.heads_in, 0), a.first_in_at FROM guests g LEFT JOIN a ON a.guest_id = g.id
	  WHERE g.event_id = $1 AND ($2::text IS NULL OR g.status = $2) AND ($3::uuid IS NULL OR g.list_id = $3)
	    AND ($4::bytea IS NULL OR g.email_bidx = $4) AND ($5::bytea IS NULL OR g.name_bidx = $5)
	    AND (NOT $7::bool OR COALESCE(a.heads_in, 0) >= 1)
	  ORDER BY g.created_at, g.id LIMIT $6`, eventID, status, f.ListID, emailBidx, nameBidx, MaxPerEvent, arrived)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (guestRow, error) { return scanGuest(r, true) })
}

func counts(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, listID *uuid.UUID) (Counts, error) {
	var c Counts
	rows, err := tx.Query(ctx, `SELECT status, count(*), COALESCE(sum(1 + plus_n), 0) FROM guests
	  WHERE event_id = $1 AND ($2::uuid IS NULL OR list_id = $2) GROUP BY status`, eventID, listID)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n, heads int
		if err := rows.Scan(&st, &n, &heads); err != nil {
			return c, err
		}
		c.add(st, n, heads)
	}
	if err := rows.Err(); err != nil {
		return c, err
	}
	if err := tx.QueryRow(ctx, `WITH a AS (`+guestArrivals+`)
	  SELECT count(*) FROM guests g JOIN a ON a.guest_id = g.id
	  WHERE g.event_id = $1 AND ($2::uuid IS NULL OR g.list_id = $2) AND a.heads_in >= 1`, eventID, listID).Scan(&c.CheckedIn); err != nil || listID != nil {
		return c, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM order_positions WHERE event_id = $1 AND status = 'valid'`, eventID).Scan(&c.Tickets)
	return c, err
}

// UpdateGuest replaces a guest's fields. Quota and +N are re-checked when
// the guest would take more of their allocation than before.
func (s *Service) UpdateGuest(ctx context.Context, eventID, guestID uuid.UUID, in GuestInput) (Guest, error) {
	var out Guest
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		// Before the guest row lock: purges lock the event first.
		if err := retention.EnsureNotPurged(ctx, tx, eventID); err != nil {
			return err
		}
		cur, err := scanGuest(tx.QueryRow(ctx, `SELECT `+guestCols+` FROM guests g WHERE g.id = $1 AND g.event_id = $2 FOR UPDATE`, guestID, eventID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		list, err := listOf(ctx, tx, eventID, cur.ListID)
		if err != nil {
			return err
		}
		if in.Status == "" {
			in.Status = cur.Status
		}
		if err := in.validate("", list.CollectContact); err != nil {
			return err
		}
		if cur.AllocationID != nil {
			a, err := allocFor(ctx, tx, *cur.AllocationID, &guestID)
			if err != nil {
				return err
			}
			if in.PlusN > a.PlusNMax && in.PlusN > cur.PlusN {
				return invalid("plus_n", "more than this allocation allows")
			}
			before, after := 0, 0
			if consumes(cur.Status) {
				before = 1 + cur.PlusN
			}
			if consumes(in.Status) {
				after = 1 + in.PlusN
			}
			if after > before {
				if err := a.checkQuota(after); err != nil {
					return err
				}
			}
		}
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		sealed, err := sealGuest(dek, tenant, guestID, in)
		if err != nil {
			return err
		}
		x, err := scanGuest(tx.QueryRow(ctx, `UPDATE guests g SET name_enc = $2, email_enc = $3, phone_enc = $4, note_enc = $5,
		  name_bidx = $6, email_bidx = $7, plus_n = $8, status = $9, updated_at = now() WHERE g.id = $1 RETURNING `+guestCols,
			guestID, sealed[0], sealed[1], sealed[2], sealed[3], nameIndex(dek, in.Name), emailIndex(dek, in.Email), in.PlusN, in.Status))
		if err != nil {
			return err
		}
		if cur.Status != in.Status {
			if err := s.emit(ctx, tx, "guest", "status_changed", map[string]uuid.UUID{"event_id": eventID, "list_id": cur.ListID, "guest_id": guestID}); err != nil {
				return err
			}
		}
		if err := s.listUpdated(ctx, tx, eventID, cur.ListID); err != nil {
			return err
		}
		if x.HeadsIn, x.FirstInAt, err = arrivalOf(ctx, tx, guestID); err != nil {
			return err
		}
		out, err = s.decrypt(ctx, tx, x)
		return err
	})
	return out, err
}

// DeleteGuest removes a guest and their personal data.
func (s *Service) DeleteGuest(ctx context.Context, eventID, guestID uuid.UUID) error {
	return s.db.Run(ctx, func(tx pgx.Tx) error {
		var listID uuid.UUID
		err := tx.QueryRow(ctx, `DELETE FROM guests WHERE id = $1 AND event_id = $2 RETURNING list_id`, guestID, eventID).Scan(&listID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return s.listUpdated(ctx, tx, eventID, listID)
	})
}

// BulkStatus sets one status on guests picked by id and/or by pasted
// emails (matched through the email blind index). Moving guests into a
// status that holds quota is checked per allocation, all or nothing.
func (s *Service) BulkStatus(ctx context.Context, eventID uuid.UUID, in BulkStatusInput) (BulkResult, error) {
	res := BulkResult{Unmatched: []string{}}
	switch {
	case !slices.Contains(Statuses, in.Status):
		return res, invalid("status", strings.Join(Statuses, ", "))
	case len(in.GuestIDs) == 0 && len(in.Emails) == 0:
		return res, invalid("emails", "paste at least one email or pick guests")
	case len(in.GuestIDs) > MaxPerEvent || len(in.Emails) > MaxBatch:
		return res, invalid("emails", "at most 500 emails per request")
	}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if _, err := eventOf(ctx, tx, eventID); err != nil {
			return err
		}
		dek, err := s.keys.Current(ctx, tx, tenantOf(ctx))
		if err != nil {
			return err
		}
		byIndex := map[string]string{}
		var order []string
		indexes := [][]byte{}
		for _, e := range in.Emails {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}
			bi := emailIndex(dek, e)
			if _, dup := byIndex[string(bi)]; dup {
				continue
			}
			byIndex[string(bi)] = e
			order = append(order, string(bi))
			indexes = append(indexes, bi)
		}
		ids := in.GuestIDs
		if ids == nil {
			ids = []uuid.UUID{}
		}
		rows, err := tx.Query(ctx, `SELECT id, list_id, allocation_id, status, plus_n, email_bidx FROM guests
		  WHERE event_id = $1 AND (id = ANY($2) OR email_bidx = ANY($3)) FOR UPDATE`, eventID, ids, indexes)
		if err != nil {
			return err
		}
		type target struct {
			id, list uuid.UUID
			alloc    *uuid.UUID
			status   string
			plus     int16
		}
		var targets []target
		matchedEmail := map[string]bool{}
		for rows.Next() {
			var t target
			var bi []byte
			if err := rows.Scan(&t.id, &t.list, &t.alloc, &t.status, &t.plus, &bi); err != nil {
				rows.Close()
				return err
			}
			matchedEmail[string(bi)] = true
			targets = append(targets, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, k := range order {
			if !matchedEmail[k] {
				res.Unmatched = append(res.Unmatched, byIndex[k])
			}
		}
		res.Matched = len(targets)

		delta := map[uuid.UUID]int{}
		var changed []target
		for _, t := range targets {
			if t.status == in.Status {
				continue
			}
			changed = append(changed, t)
			if t.alloc != nil && consumes(in.Status) && !consumes(t.status) {
				delta[*t.alloc] += 1 + int(t.plus)
			}
		}
		for allocID, heads := range delta {
			a, err := allocFor(ctx, tx, allocID, nil)
			if err != nil {
				return err
			}
			if err := a.checkQuota(heads); err != nil {
				return err
			}
		}
		lists := map[uuid.UUID]bool{}
		for _, t := range changed {
			if _, err := tx.Exec(ctx, `UPDATE guests SET status = $2, updated_at = now() WHERE id = $1`, t.id, in.Status); err != nil {
				return err
			}
			if err := s.emit(ctx, tx, "guest", "status_changed", map[string]uuid.UUID{"event_id": eventID, "list_id": t.list, "guest_id": t.id}); err != nil {
				return err
			}
			lists[t.list] = true
		}
		res.Updated = len(changed)
		for l := range lists {
			if err := s.listUpdated(ctx, tx, eventID, l); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return BulkResult{}, err
	}
	return res, nil
}

// Export returns the CSV rows for an event (honouring status and list
// filters) and records the export in the audit log: it hands personal data
// to a file outside the system.
func (s *Service) Export(ctx context.Context, eventID uuid.UUID, f GuestFilter, actor string) (string, []ExportRow, error) {
	if err := f.validate(false); err != nil {
		return "", nil, err
	}
	var slug string
	out := []ExportRow{}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		ev, err := eventOf(ctx, tx, eventID)
		if err != nil {
			return err
		}
		if err := retention.RefuseIfPurged(ctx, tx, eventID); err != nil {
			return err
		}
		slug = ev.Slug
		listNames := map[uuid.UUID]string{}
		allocLabels := map[uuid.UUID]string{}
		ls, err := s.lists(ctx, tx, eventID, nil)
		if err != nil {
			return err
		}
		for _, l := range ls {
			listNames[l.ID] = l.Name
			for _, a := range l.Allocations {
				allocLabels[a.ID] = a.Label
			}
		}
		rows, err := s.query(ctx, tx, eventID, f)
		if err != nil {
			return err
		}
		for _, x := range rows {
			g, err := s.decrypt(ctx, tx, x)
			if err != nil {
				return err
			}
			r := ExportRow{Name: g.Name, PlusN: g.PlusN, Status: g.Status, List: listNames[g.ListID], Email: g.Email, Phone: g.Phone, Note: g.Note}
			if g.AllocationID != nil {
				r.Allocation = allocLabels[*g.AllocationID]
			}
			out = append(out, r)
		}
		return audit.Record(ctx, tx, tenantOf(ctx), audit.Entry{
			ActorID: actor, Action: "guestlist.export", Resource: "event:" + eventID.String(), Allowed: true,
		})
	})
	return slug, out, err
}

// Overview lists upcoming events with list fill and pending approvals for
// the cross-event guests page.
func (s *Service) Overview(ctx context.Context) ([]OverviewRow, error) {
	out := []OverviewRow{}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT e.id, e.title, e.status, e.starts_at, e.timezone, e.capacity,
		  (SELECT count(*) FROM guest_lists l WHERE l.event_id = e.id),
		  (SELECT count(*) FROM guests g WHERE g.event_id = e.id AND g.status <> 'declined'),
		  (SELECT COALESCE(sum(1 + g.plus_n), 0) FROM guests g WHERE g.event_id = e.id AND g.status = 'going'),
		  (SELECT count(*) FROM guests g WHERE g.event_id = e.id AND g.status = 'pending'),
		  (SELECT COALESCE(sum(1 + g.plus_n), 0) FROM guests g JOIN guest_allocations a ON a.id = g.allocation_id
		     WHERE g.event_id = e.id AND a.revoked_at IS NULL AND g.status IN ('going', 'pending', 'invited')),
		  (SELECT COALESCE(sum(a.quota), 0) FROM guest_allocations a WHERE a.event_id = e.id AND a.revoked_at IS NULL),
		  (SELECT count(*) FROM order_positions p WHERE p.event_id = e.id AND p.status = 'valid')
		  FROM events e WHERE e.ends_at > $1 AND e.status <> 'cancelled' ORDER BY e.starts_at LIMIT 50`, s.now())
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (OverviewRow, error) {
			var x OverviewRow
			err := r.Scan(&x.EventID, &x.Title, &x.Status, &x.StartsAt, &x.Timezone, &x.Capacity, &x.Lists, &x.Guests,
				&x.GoingHeads, &x.Pending, &x.Used, &x.Quota, &x.Tickets)
			return x, err
		})
		return err
	})
	if out == nil {
		out = []OverviewRow{}
	}
	return out, err
}
