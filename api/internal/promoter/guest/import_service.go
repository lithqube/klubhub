package guest

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/envelope"
)

// ImportInput is a parsed export plus the preset (and, for generic, the
// explicit field → header mapping).
type ImportInput struct {
	Preset  string
	Mapping map[string]string
	Table   *Table
}

// ImportCounts summarise an import (or what a dry run would do).
type ImportCounts struct {
	Rows               int `json:"rows"`
	OrdersNew          int `json:"orders_new"`
	OrdersUpdated      int `json:"orders_updated"`
	PositionsNew       int `json:"positions_new"`
	PositionsUpdated   int `json:"positions_updated"`
	PositionsUnchanged int `json:"positions_unchanged"`
	Rejected           int `json:"rejected"`
	// NotInFile counts tickets imported earlier from the same platform that
	// this file does not mention; they are left as they are.
	NotInFile int `json:"not_in_file"`
	// OnGuestList counts rows whose email is also on a guest list.
	OnGuestList int `json:"on_guest_list"`
}

// ImportTicketType is a ticket type the file uses, with its row counts.
type ImportTicketType struct {
	Name    string `json:"name"`
	New     bool   `json:"new"`
	Tickets int    `json:"tickets"`
	Valid   int    `json:"valid"`
}

// RejectedRow is a row that cannot be imported, by line.
type RejectedRow struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// PreviewRow is a mapped row with its personal data masked.
type PreviewRow struct {
	Line       int    `json:"line"`
	OrderRef   string `json:"order_ref"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	TicketType string `json:"ticket_type"`
	Status     string `json:"status"`
	// Action is new, update or unchanged.
	Action string `json:"action"`
}

// ImportResult is the dry-run report or the applied import.
type ImportResult struct {
	DryRun      bool               `json:"dry_run"`
	ImportID    *uuid.UUID         `json:"import_id"`
	Preset      string             `json:"preset"`
	Encoding    string             `json:"encoding"`
	Mapping     map[string]string  `json:"mapping"`
	Counts      ImportCounts       `json:"counts"`
	TicketTypes []ImportTicketType `json:"ticket_types"`
	// Rejected lists at most 200 rows; Counts.Rejected has the total.
	Rejected []RejectedRow `json:"rejected"`
	Preview  []PreviewRow  `json:"preview"`
}

// Blind indexes for the attendee tables. The secret is matched exactly
// (trimmed only): the door's QR lookup (P2.3) must use SecretIndex too.
func SecretIndex(dek *envelope.DEK, secret string) []byte {
	return dek.BlindIndex("order_positions", "secret_bidx", secret)
}

func attendeeEmailIndex(dek *envelope.DEK, email string) []byte {
	if email == "" {
		return nil
	}
	return dek.BlindIndex("order_positions", "attendee_email_bidx", envelope.NormalizeEmail(email))
}

func attendeeNameIndex(dek *envelope.DEK, name string) []byte {
	return dek.BlindIndex("order_positions", "attendee_name_bidx", envelope.NormalizeName(name))
}

func buyerEmailIndex(dek *envelope.DEK, email string) []byte {
	if email == "" {
		return nil
	}
	return dek.BlindIndex("orders", "buyer_email_bidx", envelope.NormalizeEmail(email))
}

type ticketType struct {
	id      uuid.UUID
	name    string
	key     string
	ref     *string
	isNew   bool
	used    bool
	tickets int
	valid   int
}

type existingPos struct {
	id, typeID uuid.UUID
	ref        string
	nameEnc    []byte
	emailIdx   []byte
	secretIdx  []byte
	status     string
	matched    bool
}

type existingOrder struct {
	id           uuid.UUID
	buyerNameEnc []byte
	buyerIdx     []byte
	positions    []*existingPos
}

type plannedPos struct {
	id        uuid.UUID
	ref       string
	row       ImportRow
	typ       *ticketType
	existing  *existingPos
	changed   bool
	emailIdx  []byte
	secretIdx []byte
}

type plannedOrder struct {
	id         uuid.UUID
	ref        string
	existing   *existingOrder
	changed    bool
	buyerName  string
	buyerEmail string
	rows       []int // indexes into the accepted rows, file order
	positions  []*plannedPos
}

type importPlan struct {
	types  []*ticketType // used by this file, first-seen order
	orders []*plannedOrder
}

// ImportAttendees imports (or, with dryRun, only checks) an export of
// tickets bought elsewhere. Orders are keyed by (event, preset, order
// ref) and tickets by (order, ticket ref or barcode, else n-th ticket of
// the order), so a re-import updates names, types and statuses instead of
// duplicating. The real run is one transaction; it is audited and emits
// attendees.imported with identifiers only.
func (s *Service) ImportAttendees(ctx context.Context, eventID uuid.UUID, in ImportInput, dryRun bool, actor string) (ImportResult, error) {
	res := ImportResult{DryRun: dryRun, Preset: in.Preset, TicketTypes: []ImportTicketType{}, Rejected: []RejectedRow{}, Preview: []PreviewRow{}}
	if in.Table == nil {
		return res, invalid("file", "no file")
	}
	m, err := ResolveMapping(in.Preset, in.Table.Headers, in.Mapping)
	if err != nil {
		return res, err
	}
	res.Encoding, res.Mapping = in.Table.Encoding, m.Header
	res.Counts.Rows = len(in.Table.Rows)
	var rows []ImportRow
	for _, tr := range in.Table.Rows {
		r, reason := m.MapRow(tr)
		if reason != "" {
			res.reject(tr.Line, reason)
			continue
		}
		rows = append(rows, r)
	}
	err = s.db.Run(ctx, func(tx pgx.Tx) error {
		lock := ""
		if !dryRun {
			lock = " FOR UPDATE" // one import per event at a time
		}
		if err := tx.QueryRow(ctx, `SELECT 1 FROM events WHERE id = $1`+lock, eventID).Scan(new(int)); err != nil {
			if err == pgx.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		plan, err := s.planImport(ctx, tx, dek, eventID, in.Preset, rows, &res)
		if err != nil || dryRun {
			return err
		}
		return s.applyImport(ctx, tx, dek, eventID, in.Preset, plan, &res, actor)
	})
	if err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

func (r *ImportResult) reject(line int, reason string) {
	r.Counts.Rejected++
	if len(r.Rejected) < maxRejectedListed {
		r.Rejected = append(r.Rejected, RejectedRow{Line: line, Reason: reason})
	}
}

func (s *Service) loadTicketTypes(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) ([]*ticketType, error) {
	rows, err := tx.Query(ctx, `SELECT id, name, name_key, external_ref FROM ticket_types WHERE event_id = $1 ORDER BY position, created_at`, eventID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (*ticketType, error) {
		t := &ticketType{}
		return t, r.Scan(&t.id, &t.name, &t.key, &t.ref)
	})
}

func (s *Service) loadOrders(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, source string) (map[string]*existingOrder, int, error) {
	orders := map[string]*existingOrder{}
	byID := map[uuid.UUID]*existingOrder{}
	rows, err := tx.Query(ctx, `SELECT id, external_ref, buyer_name_enc, buyer_email_bidx FROM orders WHERE event_id = $1 AND source = $2`, eventID, source)
	if err != nil {
		return nil, 0, err
	}
	for rows.Next() {
		o := &existingOrder{}
		var ref string
		if err := rows.Scan(&o.id, &ref, &o.buyerNameEnc, &o.buyerIdx); err != nil {
			rows.Close()
			return nil, 0, err
		}
		orders[ref], byID[o.id] = o, o
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows, err = tx.Query(ctx, `SELECT p.id, p.order_id, p.ticket_type_id, p.external_ref, p.attendee_name_enc, p.attendee_email_bidx, p.secret_bidx, p.status
	  FROM order_positions p JOIN orders o ON o.id = p.order_id WHERE p.event_id = $1 AND o.source = $2 ORDER BY p.created_at, p.id`, eventID, source)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		p := &existingPos{}
		var orderID uuid.UUID
		if err := rows.Scan(&p.id, &orderID, &p.typeID, &p.ref, &p.nameEnc, &p.emailIdx, &p.secretIdx, &p.status); err != nil {
			return nil, 0, err
		}
		if o := byID[orderID]; o != nil {
			o.positions = append(o.positions, p)
			n++
		}
	}
	return orders, n, rows.Err()
}

// planImport matches rows to existing orders, tickets and ticket types and
// fills res with counts, per-type totals, rejections and the preview. It
// reads only.
func (s *Service) planImport(ctx context.Context, tx pgx.Tx, dek *envelope.DEK, eventID uuid.UUID, source string, rows []ImportRow, res *ImportResult) (*importPlan, error) {
	types, err := s.loadTicketTypes(ctx, tx, eventID)
	if err != nil {
		return nil, err
	}
	byRef, byKey := map[string]*ticketType{}, map[string]*ticketType{}
	for _, t := range types {
		byKey[t.key] = t
		if t.ref != nil {
			byRef[*t.ref] = t
		}
	}
	existing, existingCount, err := s.loadOrders(ctx, tx, eventID, source)
	if err != nil {
		return nil, err
	}
	// Every barcode already in the event, whatever its source.
	secretOwner := map[string]uuid.UUID{}
	var eventTickets int
	srows, err := tx.Query(ctx, `SELECT id, secret_bidx FROM order_positions WHERE event_id = $1`, eventID)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var id uuid.UUID
		var idx []byte
		if err := srows.Scan(&id, &idx); err != nil {
			srows.Close()
			return nil, err
		}
		eventTickets++
		if idx != nil {
			secretOwner[string(idx)] = id
		}
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, err
	}

	// Group accepted rows by order, in file order.
	plan := &importPlan{}
	orders := map[string]*plannedOrder{}
	secrets := make([][]byte, len(rows))
	for i, r := range rows {
		if r.Secret != "" {
			secrets[i] = SecretIndex(dek, r.Secret)
		}
		ref := r.OrderRef
		switch {
		case ref != "":
		case r.TicketRef != "":
			ref = r.TicketRef
		default:
			ref = "~" + hex.EncodeToString(secrets[i][:16])
		}
		o := orders[ref]
		if o == nil {
			o = &plannedOrder{ref: ref, existing: existing[ref]}
			orders[ref] = o
			plan.orders = append(plan.orders, o)
		}
		o.rows = append(o.rows, i)
	}

	accepted := make([]*plannedPos, len(rows))
	rejected := map[int]string{}
	seenSecret := map[string]int{}
	for _, o := range plan.orders {
		seenRef := map[string]int{}
		var byRefPos, bySecretPos map[string]*existingPos
		if o.existing != nil {
			byRefPos, bySecretPos = map[string]*existingPos{}, map[string]*existingPos{}
			for _, p := range o.existing.positions {
				byRefPos[p.ref] = p
				if p.secretIdx != nil {
					bySecretPos[string(p.secretIdx)] = p
				}
			}
		}
		// Pass 1: ticket ids and barcodes.
		for _, i := range o.rows {
			r := rows[i]
			if secrets[i] != nil {
				if line, dup := seenSecret[string(secrets[i])]; dup {
					rejected[i] = fmt.Sprintf("same barcode as line %d", line)
					continue
				}
				seenSecret[string(secrets[i])] = r.Line
			}
			p := &plannedPos{row: r, secretIdx: secrets[i]}
			if r.TicketRef != "" {
				if line, dup := seenRef[r.TicketRef]; dup {
					rejected[i] = fmt.Sprintf("same ticket as line %d", line)
					continue
				}
				seenRef[r.TicketRef] = r.Line
				p.ref = r.TicketRef
				if ex := byRefPos[r.TicketRef]; ex != nil {
					p.existing = ex
				}
			} else if ex := bySecretPos[string(secrets[i])]; secrets[i] != nil && ex != nil && !ex.matched {
				p.existing, p.ref = ex, ex.ref
			}
			if p.existing != nil {
				p.existing.matched = true
			}
			accepted[i] = p
		}
		// Pass 2: rows without a ticket id or known barcode take the n-th
		// earlier ticket of the order that has no conflicting barcode.
		n := 0
		for _, i := range o.rows {
			p := accepted[i]
			if p == nil || p.row.TicketRef != "" {
				continue
			}
			n++
			if p.existing != nil {
				continue
			}
			key := ticketRefOrdinalMark + strconv.Itoa(n)
			if ex := byRefPos[key]; ex != nil && !ex.matched && (ex.secretIdx == nil || p.secretIdx == nil) {
				p.existing, p.ref = ex, key
				ex.matched = true
				continue
			}
			for k := n; ; k++ { // a fresh "#k" that no ticket of the order uses
				key = ticketRefOrdinalMark + strconv.Itoa(k)
				if byRefPos[key] == nil && seenRef[key] == 0 {
					break
				}
			}
			seenRef[key] = p.row.Line
			p.ref = key
		}
		// Barcodes that already belong to another ticket of the event.
		for _, i := range o.rows {
			p := accepted[i]
			if p == nil || p.secretIdx == nil {
				continue
			}
			if owner, ok := secretOwner[string(p.secretIdx)]; ok && (p.existing == nil || p.existing.id != owner) {
				rejected[i] = "barcode already belongs to another ticket of this event"
				if p.existing != nil {
					p.existing.matched = false
				}
				accepted[i] = nil
			}
		}
	}

	// Ticket types, changes and counts, in file order.
	onList := map[int][]byte{}
	for i, p := range accepted {
		r := rows[i]
		if p == nil {
			res.reject(r.Line, rejected[i])
			continue
		}
		key := envelope.NormalizeName(r.TicketType)
		var t *ticketType
		if r.TicketTypeRef != "" {
			t = byRef[r.TicketTypeRef]
		}
		if t == nil {
			t = byKey[key]
		}
		if t == nil {
			t = &ticketType{id: uuid.Must(uuid.NewV7()), name: r.TicketType, key: key, isNew: true}
			if r.TicketTypeRef != "" {
				ref := r.TicketTypeRef
				t.ref = &ref
				byRef[ref] = t
			}
			byKey[key] = t
		}
		if !t.used {
			t.used = true
			plan.types = append(plan.types, t)
		}
		t.tickets++
		if r.Status == TicketValid {
			t.valid++
		}
		p.typ = t
		p.emailIdx = attendeeEmailIndex(dek, r.Email)
		if p.existing == nil {
			p.id = uuid.Must(uuid.NewV7())
		} else {
			ex := p.existing
			p.id = ex.id
			name, err := s.open(ctx, tx, "order_positions", "attendee_name_enc", ex.id, ex.nameEnc)
			if err != nil {
				return nil, err
			}
			p.changed = ex.typeID != t.id || ex.status != r.Status || name != r.Name || !bytes.Equal(ex.emailIdx, p.emailIdx) ||
				(p.secretIdx != nil && !bytes.Equal(ex.secretIdx, p.secretIdx))
		}
		if r.Email != "" {
			onList[i] = dek.BlindIndex("guests", "email_bidx", envelope.NormalizeEmail(r.Email))
		}
	}

	for _, o := range plan.orders {
		for _, i := range o.rows {
			if accepted[i] != nil {
				o.positions = append(o.positions, accepted[i])
			}
		}
		if len(o.positions) == 0 {
			continue
		}
		first := o.positions[0].row
		o.buyerName, o.buyerEmail = first.BuyerName, first.BuyerEmail
		if o.existing == nil {
			o.id = uuid.Must(uuid.NewV7())
			res.Counts.OrdersNew++
		} else {
			o.id = o.existing.id
			name, err := s.open(ctx, tx, "orders", "buyer_name_enc", o.id, o.existing.buyerNameEnc)
			if err != nil {
				return nil, err
			}
			o.changed = name != o.buyerName || !bytes.Equal(o.existing.buyerIdx, buyerEmailIndex(dek, o.buyerEmail))
			if o.changed {
				res.Counts.OrdersUpdated++
			}
		}
		for _, p := range o.positions {
			switch {
			case p.existing == nil:
				res.Counts.PositionsNew++
			case p.changed:
				res.Counts.PositionsUpdated++
			default:
				res.Counts.PositionsUnchanged++
			}
		}
	}
	matched := 0
	for _, o := range existing {
		for _, p := range o.positions {
			if p.matched {
				matched++
			}
		}
	}
	res.Counts.NotInFile = existingCount - matched
	if eventTickets+res.Counts.PositionsNew > MaxTicketsPerEvent {
		return nil, invalid("file", fmt.Sprintf("an event holds at most %d tickets", MaxTicketsPerEvent))
	}

	if len(onList) > 0 {
		idx := make([][]byte, 0, len(onList))
		for _, v := range onList {
			idx = append(idx, v)
		}
		found := map[string]bool{}
		if err := collectKeys(ctx, tx, found, "", `SELECT email_bidx FROM guests WHERE event_id = $1 AND email_bidx = ANY($2)`, eventID, idx); err != nil {
			return nil, err
		}
		for _, v := range onList {
			if found[string(v)] {
				res.Counts.OnGuestList++
			}
		}
	}

	sort.SliceStable(res.Rejected, func(a, b int) bool { return res.Rejected[a].Line < res.Rejected[b].Line })
	for _, t := range plan.types {
		res.TicketTypes = append(res.TicketTypes, ImportTicketType{Name: t.name, New: t.isNew, Tickets: t.tickets, Valid: t.valid})
	}
	for i, p := range accepted {
		if len(res.Preview) == previewRows {
			break
		}
		if p == nil {
			continue
		}
		action := "new"
		if p.existing != nil {
			action = "unchanged"
			if p.changed {
				action = "update"
			}
		}
		r := rows[i]
		orderRef := r.OrderRef
		if orderRef == "" {
			orderRef = r.TicketRef
		}
		res.Preview = append(res.Preview, PreviewRow{
			Line: r.Line, OrderRef: orderRef, Name: MaskName(r.Name), Email: MaskEmail(r.Email),
			TicketType: p.typ.name, Status: r.Status, Action: action,
		})
	}
	return plan, nil
}

// applyImport writes the plan: the import record, new ticket types,
// orders and tickets, then the outbox event and the audit entry.
func (s *Service) applyImport(ctx context.Context, tx pgx.Tx, dek *envelope.DEK, eventID uuid.UUID, source string, plan *importPlan, res *ImportResult, actor string) error {
	tenant := tenantOf(ctx)
	importID := uuid.Must(uuid.NewV7())
	c := res.Counts
	if _, err := tx.Exec(ctx, `INSERT INTO attendee_imports (id, tenant_id, event_id, source, rows_total, orders_new, orders_updated,
	  positions_new, positions_updated, positions_unchanged, rejected, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		importID, tenant, eventID, source, c.Rows, c.OrdersNew, c.OrdersUpdated, c.PositionsNew, c.PositionsUpdated, c.PositionsUnchanged,
		c.Rejected, actor); err != nil {
		return err
	}
	now := s.now()
	b := &pgx.Batch{}
	var pos int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(position) + 1, 0) FROM ticket_types WHERE event_id = $1`, eventID).Scan(&pos); err != nil {
		return err
	}
	for _, t := range plan.types {
		if !t.isNew {
			continue
		}
		b.Queue(`INSERT INTO ticket_types (id, tenant_id, event_id, name, name_key, external_ref, position) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			t.id, tenant, eventID, t.name, t.key, t.ref, pos)
		pos++
	}
	for _, o := range plan.orders {
		if len(o.positions) == 0 {
			continue
		}
		if o.existing == nil || o.changed {
			name, err := seal(dek, tenant, "orders", "buyer_name_enc", o.id, o.buyerName)
			if err != nil {
				return err
			}
			email, err := seal(dek, tenant, "orders", "buyer_email_enc", o.id, o.buyerEmail)
			if err != nil {
				return err
			}
			if o.existing == nil {
				b.Queue(`INSERT INTO orders (id, tenant_id, event_id, source, external_ref, buyer_name_enc, buyer_email_enc, buyer_email_bidx,
				  import_id, imported_at, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10,$10)`,
					o.id, tenant, eventID, source, o.ref, name, email, buyerEmailIndex(dek, o.buyerEmail), importID, now)
			} else {
				b.Queue(`UPDATE orders SET buyer_name_enc = $2, buyer_email_enc = $3, buyer_email_bidx = $4, import_id = $5, imported_at = $6,
				  updated_at = $6 WHERE id = $1`, o.id, name, email, buyerEmailIndex(dek, o.buyerEmail), importID, now)
			}
		}
		for _, p := range o.positions {
			if p.existing != nil && !p.changed {
				continue
			}
			r := p.row
			name, err := seal(dek, tenant, "order_positions", "attendee_name_enc", p.id, r.Name)
			if err != nil {
				return err
			}
			email, err := seal(dek, tenant, "order_positions", "attendee_email_enc", p.id, r.Email)
			if err != nil {
				return err
			}
			secret, err := seal(dek, tenant, "order_positions", "secret_enc", p.id, r.Secret)
			if err != nil {
				return err
			}
			if p.existing == nil {
				b.Queue(`INSERT INTO order_positions (id, tenant_id, event_id, order_id, ticket_type_id, external_ref, attendee_name_enc,
				  attendee_email_enc, secret_enc, attendee_name_bidx, attendee_email_bidx, secret_bidx, status, imported_at, created_at, updated_at)
				  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14,$14)`,
					p.id, tenant, eventID, o.id, p.typ.id, p.ref, name, email, secret, attendeeNameIndex(dek, r.Name), p.emailIdx, p.secretIdx, r.Status, now)
				continue
			}
			// A row without a barcode keeps the stored one.
			b.Queue(`UPDATE order_positions SET ticket_type_id = $2, attendee_name_enc = $3, attendee_email_enc = $4, attendee_name_bidx = $5,
			  attendee_email_bidx = $6, secret_enc = COALESCE($7, secret_enc), secret_bidx = COALESCE($8, secret_bidx), status = $9,
			  imported_at = $10, updated_at = $10 WHERE id = $1`,
				p.id, p.typ.id, name, email, attendeeNameIndex(dek, r.Name), p.emailIdx, secret, p.secretIdx, r.Status, now)
		}
	}
	if b.Len() > 0 {
		if err := tx.SendBatch(ctx, b).Close(); err != nil {
			return err
		}
	}
	if err := s.emit(ctx, tx, "attendees", "imported", map[string]uuid.UUID{"event_id": eventID, "import_id": importID}); err != nil {
		return err
	}
	res.ImportID = &importID
	return audit.Record(ctx, tx, tenant, audit.Entry{
		ActorID: actor, Action: "attendees.import", Resource: "event:" + eventID.String(), Allowed: true,
		Reason: fmt.Sprintf("import %s from %s: %d rows, %d new and %d updated tickets, %d rejected",
			importID, source, c.Rows, c.PositionsNew, c.PositionsUpdated, c.Rejected),
	})
}
