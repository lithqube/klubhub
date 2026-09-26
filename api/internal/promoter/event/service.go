package event

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// Service implements venues, events, stages, lineup and export data. Every
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

func (s *Service) emit(ctx context.Context, tx pgx.Tx, aggregate, verb string, refs map[string]uuid.UUID) error {
	subject, ev, err := events.New(tenantOf(ctx), aggregate, verb, refs, s.now())
	if err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, subject, ev)
}

// ---------------------------------------------------------------- venues ---

// Room is a stage template of a venue.
type Room struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Capacity *int      `json:"capacity"`
	Position int       `json:"position"`
}

// ProtectedFlags tells the UI which sealed fields hold a value (masked).
type ProtectedFlags struct {
	Address bool `json:"address"`
	Geo     bool `json:"geo"`
	Contact bool `json:"contact"`
}

// Venue as listed and edited (sealed fields masked).
type Venue struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	City        string         `json:"city"`
	Country     *string        `json:"country"`
	Timezone    string         `json:"timezone"`
	Capacity    *int           `json:"capacity"`
	CurfewLocal *string        `json:"curfew_local"`
	TechNotes   string         `json:"tech_notes"`
	Rooms       []Room         `json:"rooms"`
	Protected   ProtectedFlags `json:"protected"`
	ArchivedAt  *time.Time     `json:"archived_at"`
}

// VenueProtected holds the decrypted sealed fields (reveal only).
type VenueProtected struct {
	Address      string `json:"address"`
	Geo          string `json:"geo"`
	ContactName  string `json:"contact_name"`
	ContactEmail string `json:"contact_email"`
	ContactPhone string `json:"contact_phone"`
}

// RoomInput creates or keeps (ID set) a room.
type RoomInput struct {
	ID       *uuid.UUID `json:"id"`
	Name     string     `json:"name"`
	Capacity *int       `json:"capacity"`
}

// VenueInput creates or replaces a venue. Protected nil leaves sealed
// fields unchanged; an empty string clears one.
type VenueInput struct {
	Name        string          `json:"name"`
	City        string          `json:"city"`
	Country     *string         `json:"country"`
	Timezone    string          `json:"timezone"`
	Capacity    *int            `json:"capacity"`
	CurfewLocal *string         `json:"curfew_local"`
	TechNotes   string          `json:"tech_notes"`
	Rooms       []RoomInput     `json:"rooms"`
	Protected   *VenueProtected `json:"protected"`
}

var (
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
	hhmmRe    = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

func (in *VenueInput) validate() error {
	in.Name, in.City = strings.TrimSpace(in.Name), strings.TrimSpace(in.City)
	switch {
	case in.Name == "" || len(in.Name) > 200:
		return invalid("name", "required, at most 200 characters")
	case len(in.City) > 120:
		return invalid("city", "at most 120 characters")
	case in.Country != nil && !countryRe.MatchString(*in.Country):
		return invalid("country", "ISO 3166 alpha-2, e.g. DE")
	case in.Capacity != nil && *in.Capacity <= 0:
		return invalid("capacity", "must be positive")
	case in.CurfewLocal != nil && !hhmmRe.MatchString(*in.CurfewLocal):
		return invalid("curfew_local", "HH:MM")
	case len(in.TechNotes) > 5000:
		return invalid("tech_notes", "at most 5000 characters")
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return invalid("timezone", "unknown IANA timezone")
	}
	for i := range in.Rooms {
		in.Rooms[i].Name = strings.TrimSpace(in.Rooms[i].Name)
		if in.Rooms[i].Name == "" || len(in.Rooms[i].Name) > 80 {
			return invalid(fmt.Sprintf("rooms[%d].name", i), "required, at most 80 characters")
		}
	}
	return nil
}

const venueCols = `v.id, v.name, v.city, v.country, v.timezone, v.capacity, to_char(v.curfew_local, 'HH24:MI'), v.tech_notes,
	v.address_enc IS NOT NULL, v.geo_enc IS NOT NULL,
	(v.contact_name_enc IS NOT NULL OR v.contact_email_enc IS NOT NULL OR v.contact_phone_enc IS NOT NULL), v.archived_at`

func scanVenue(row pgx.Row) (Venue, error) {
	var v Venue
	var country *string
	err := row.Scan(&v.ID, &v.Name, &v.City, &country, &v.Timezone, &v.Capacity, &v.CurfewLocal, &v.TechNotes,
		&v.Protected.Address, &v.Protected.Geo, &v.Protected.Contact, &v.ArchivedAt)
	if country != nil {
		c := strings.TrimSpace(*country)
		v.Country = &c
	}
	return v, err
}

func (s *Service) loadRooms(ctx context.Context, tx pgx.Tx, venueID uuid.UUID) ([]Room, error) {
	rows, err := tx.Query(ctx, `SELECT id, name, capacity, position FROM venue_rooms WHERE venue_id = $1 ORDER BY position, name`, venueID)
	if err != nil {
		return nil, err
	}
	rooms, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Room, error) {
		var x Room
		var pos int16
		err := r.Scan(&x.ID, &x.Name, &x.Capacity, &pos)
		x.Position = int(pos)
		return x, err
	})
	if rooms == nil {
		rooms = []Room{}
	}
	return rooms, err
}

// ListVenues returns active venues, alphabetically.
func (s *Service) ListVenues(ctx context.Context) ([]Venue, error) {
	out := []Venue{}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+venueCols+` FROM venues v WHERE v.archived_at IS NULL ORDER BY lower(v.name)`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Venue, error) { return scanVenue(r) })
		if err != nil {
			return err
		}
		for i := range out {
			if out[i].Rooms, err = s.loadRooms(ctx, tx, out[i].ID); err != nil {
				return err
			}
		}
		return nil
	})
	if out == nil {
		out = []Venue{}
	}
	return out, err
}

// GetVenue returns one venue (masked).
func (s *Service) GetVenue(ctx context.Context, id uuid.UUID) (Venue, error) {
	var v Venue
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		var err error
		v, err = s.getVenue(ctx, tx, id)
		return err
	})
	return v, err
}

func (s *Service) getVenue(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Venue, error) {
	v, err := scanVenue(tx.QueryRow(ctx, `SELECT `+venueCols+` FROM venues v WHERE v.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Venue{}, ErrNotFound
	}
	if err != nil {
		return Venue{}, err
	}
	v.Rooms, err = s.loadRooms(ctx, tx, id)
	return v, err
}

// CreateVenue stores a venue, sealing protected fields.
func (s *Service) CreateVenue(ctx context.Context, in VenueInput) (Venue, error) {
	if err := in.validate(); err != nil {
		return Venue{}, err
	}
	id := uuid.Must(uuid.NewV7())
	var v Venue
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO venues (id, tenant_id, name, city, country, timezone, capacity, curfew_local, tech_notes)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $8::time, $9)`,
			id, tenantOf(ctx), in.Name, in.City, in.Country, in.Timezone, in.Capacity, in.CurfewLocal, in.TechNotes); err != nil {
			return err
		}
		if err := s.writeProtected(ctx, tx, id, in.Protected); err != nil {
			return err
		}
		if err := s.replaceRooms(ctx, tx, id, in.Rooms); err != nil {
			return err
		}
		var err error
		v, err = s.getVenue(ctx, tx, id)
		return err
	})
	return v, err
}

// UpdateVenue replaces a venue's fields and rooms.
func (s *Service) UpdateVenue(ctx context.Context, id uuid.UUID, in VenueInput) (Venue, error) {
	if err := in.validate(); err != nil {
		return Venue{}, err
	}
	var v Venue
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE venues SET name = $2, city = $3, country = $4, timezone = $5, capacity = $6,
		  curfew_local = $7::time, tech_notes = $8, updated_at = now() WHERE id = $1 AND archived_at IS NULL`,
			id, in.Name, in.City, in.Country, in.Timezone, in.Capacity, in.CurfewLocal, in.TechNotes)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := s.writeProtected(ctx, tx, id, in.Protected); err != nil {
			return err
		}
		if err := s.replaceRooms(ctx, tx, id, in.Rooms); err != nil {
			return err
		}
		v, err = s.getVenue(ctx, tx, id)
		return err
	})
	return v, err
}

// ArchiveVenue hides a venue unless upcoming events use it.
func (s *Service) ArchiveVenue(ctx context.Context, id uuid.UUID) error {
	return s.db.Run(ctx, func(tx pgx.Tx) error {
		var inUse bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM events WHERE venue_id = $1 AND ends_at > $2 AND status <> 'cancelled')`,
			id, s.now()).Scan(&inUse); err != nil {
			return err
		}
		if inUse {
			return ErrVenueInUse
		}
		tag, err := tx.Exec(ctx, `UPDATE venues SET archived_at = $2 WHERE id = $1 AND archived_at IS NULL`, id, s.now())
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
}

var protectedCols = []struct {
	col string
	get func(*VenueProtected) string
	set func(*VenueProtected, string)
}{
	{"address_enc", func(p *VenueProtected) string { return p.Address }, func(p *VenueProtected, v string) { p.Address = v }},
	{"geo_enc", func(p *VenueProtected) string { return p.Geo }, func(p *VenueProtected, v string) { p.Geo = v }},
	{"contact_name_enc", func(p *VenueProtected) string { return p.ContactName }, func(p *VenueProtected, v string) { p.ContactName = v }},
	{"contact_email_enc", func(p *VenueProtected) string { return p.ContactEmail }, func(p *VenueProtected, v string) { p.ContactEmail = v }},
	{"contact_phone_enc", func(p *VenueProtected) string { return p.ContactPhone }, func(p *VenueProtected, v string) { p.ContactPhone = v }},
}

func (s *Service) writeProtected(ctx context.Context, tx pgx.Tx, venueID uuid.UUID, p *VenueProtected) error {
	if p == nil {
		return nil
	}
	tenant := tenantOf(ctx)
	dek, err := s.keys.Current(ctx, tx, tenant)
	if err != nil {
		return err
	}
	for _, c := range protectedCols {
		val := strings.TrimSpace(c.get(p))
		var sealed []byte
		if val != "" {
			if sealed, err = dek.Seal(tenant, envelope.Field{Table: "venues", Column: c.col, RowID: venueID}, []byte(val)); err != nil {
				return err
			}
		}
		// Column names come from the fixed list above, never from input.
		if _, err := tx.Exec(ctx, `UPDATE venues SET `+c.col+` = $2 WHERE id = $1`, venueID, sealed); err != nil {
			return err
		}
	}
	return nil
}

// RevealVenue decrypts the protected fields for an authorised caller and
// records the reveal in the audit log (UX decision 4).
func (s *Service) RevealVenue(ctx context.Context, id uuid.UUID, actor string) (VenueProtected, error) {
	var out VenueProtected
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		var cols [5][]byte
		err := tx.QueryRow(ctx, `SELECT address_enc, geo_enc, contact_name_enc, contact_email_enc, contact_phone_enc FROM venues WHERE id = $1`, id).
			Scan(&cols[0], &cols[1], &cols[2], &cols[3], &cols[4])
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		tenant := tenantOf(ctx)
		for i, c := range protectedCols {
			if cols[i] == nil {
				continue
			}
			dek, err := s.keys.ForSealed(ctx, tx, tenant, cols[i])
			if err != nil {
				return err
			}
			plain, err := dek.Open(tenant, envelope.Field{Table: "venues", Column: c.col, RowID: id}, cols[i])
			if err != nil {
				return err
			}
			c.set(&out, string(plain))
		}
		return audit.Record(ctx, tx, tenant, audit.Entry{ActorID: actor, Action: "venue.reveal", Resource: "venue:" + id.String(), Allowed: true})
	})
	return out, err
}

func (s *Service) replaceRooms(ctx context.Context, tx pgx.Tx, venueID uuid.UUID, rooms []RoomInput) error {
	keep := []uuid.UUID{}
	for i, r := range rooms {
		id := uuid.Must(uuid.NewV7())
		if r.ID != nil {
			id = *r.ID
		}
		keep = append(keep, id)
		if _, err := tx.Exec(ctx, `INSERT INTO venue_rooms (id, tenant_id, venue_id, name, capacity, position) VALUES ($1, $2, $3, $4, $5, $6)
		  ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, capacity = EXCLUDED.capacity, position = EXCLUDED.position
		  WHERE venue_rooms.venue_id = EXCLUDED.venue_id`,
			id, tenantOf(ctx), venueID, r.Name, r.Capacity, i); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `DELETE FROM venue_rooms WHERE venue_id = $1 AND NOT (id = ANY($2))`, venueID, keep)
	return err
}
