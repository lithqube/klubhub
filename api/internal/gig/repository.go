package gig

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides DB access for gigs.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetByID returns a gig by ID, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Gig, error) {
	var g Gig
	err := r.pool.QueryRow(ctx, `
		SELECT id, date, venue, city, country, event_name,
		       promoter_name, promoter_email, promoter_phone,
		       fee_amount, fee_currency, set_length_minutes, notes,
		       status, payment_status, gig_reader_venue_id, gig_reader_contact_id,
		       created_at, updated_at, deleted_at
		FROM gigs
		WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(
		&g.ID, &g.Date, &g.Venue, &g.City, &g.Country, &g.EventName,
		&g.PromoterName, &g.PromoterEmail, &g.PromoterPhone,
		&g.FeeAmount, &g.FeeCurrency, &g.SetLengthMinutes, &g.Notes,
		&g.Status, &g.PaymentStatus, &g.GigReaderVenueID, &g.GigReaderContactID,
		&g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// List returns gigs matching the given filter, ordered by date DESC.
func (r *Repository) List(ctx context.Context, f GigFilter) ([]*Gig, error) {
	query := `
		SELECT id, date, venue, city, country, event_name,
		       promoter_name, promoter_email, promoter_phone,
		       fee_amount, fee_currency, set_length_minutes, notes,
		       status, payment_status, gig_reader_venue_id, gig_reader_contact_id,
		       created_at, updated_at, deleted_at
		FROM gigs
		WHERE deleted_at IS NULL`
	args := []interface{}{}
	argID := 1

	if f.Status != nil {
		query += " AND status = $" + string(rune('0'+argID))
		args = append(args, *f.Status)
		argID++
	}
	if f.VenueID != nil {
		query += " AND gig_reader_venue_id = $" + string(rune('0'+argID))
		args = append(args, *f.VenueID)
		argID++
	}
	if f.City != nil {
		query += " AND city ILIKE $" + string(rune('0'+argID))
		args = append(args, "%"+*f.City+"%")
		argID++
	}
	if f.FeeMin != nil {
		query += " AND fee_amount >= $" + string(rune('0'+argID))
		args = append(args, *f.FeeMin)
		argID++
	}
	if f.FeeMax != nil {
		query += " AND fee_amount <= $" + string(rune('0'+argID))
		args = append(args, *f.FeeMax)
		argID++
	}
	if f.From != nil {
		query += " AND date >= $" + string(rune('0'+argID))
		args = append(args, *f.From)
		argID++
	}
	if f.To != nil {
		query += " AND date <= $" + string(rune('0'+argID))
		args = append(args, *f.To)
		argID++
	}

	query += " ORDER BY date DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var gigs []*Gig
	for rows.Next() {
		var g Gig
		err := rows.Scan(
			&g.ID, &g.Date, &g.Venue, &g.City, &g.Country, &g.EventName,
			&g.PromoterName, &g.PromoterEmail, &g.PromoterPhone,
			&g.FeeAmount, &g.FeeCurrency, &g.SetLengthMinutes, &g.Notes,
			&g.Status, &g.PaymentStatus, &g.GigReaderVenueID, &g.GigReaderContactID,
			&g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		gigs = append(gigs, &g)
	}
	return gigs, rows.Err()
}

// Create inserts a new gig and returns it.
func (r *Repository) Create(ctx context.Context, g *GigCreate) (*Gig, error) {
	var out Gig
	err := r.pool.QueryRow(ctx, `
		INSERT INTO gigs (
			id, date, venue, city, country, event_name,
			promoter_name, promoter_email, promoter_phone,
			fee_amount, fee_currency, set_length_minutes, notes,
			status, payment_status, gig_reader_venue_id, gig_reader_contact_id,
			created_at, updated_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			'inquiry', 'unpaid', NULL, NULL,
			now(), now()
		) RETURNING
			id, date, venue, city, country, event_name,
			promoter_name, promoter_email, promoter_phone,
			fee_amount, fee_currency, set_length_minutes, notes,
			status, payment_status, gig_reader_venue_id, gig_reader_contact_id,
			created_at, updated_at, deleted_at`,
		g.Date, g.Venue, g.City, g.Country, g.EventName,
		g.PromoterName, g.PromoterEmail, g.PromoterPhone,
		g.FeeAmount, g.FeeCurrency, g.SetLengthMinutes, g.Notes,
	).Scan(
		&out.ID, &out.Date, &out.Venue, &out.City, &out.Country, &out.EventName,
		&out.PromoterName, &out.PromoterEmail, &out.PromoterPhone,
		&out.FeeAmount, &out.FeeCurrency, &out.SetLengthMinutes, &out.Notes,
		&out.Status, &out.PaymentStatus, &out.GigReaderVenueID, &out.GigReaderContactID,
		&out.CreatedAt, &out.UpdatedAt, &out.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Update applies non-nil changes from u to the gig identified by id.
// Returns ErrConflict if updated_at does not match (optimistic concurrency).
func (r *Repository) Update(ctx context.Context, id uuid.UUID, u *GigUpdate) (*Gig, error) {
	setClauses := []string{}
	args := []interface{}{}
	argID := 1

	if u.Date != nil {
		setClauses = append(setClauses, "date = $"+string(rune('0'+argID)))
		args = append(args, *u.Date)
		argID++
	}
	if u.Venue != nil {
		setClauses = append(setClauses, "venue = $"+string(rune('0'+argID)))
		args = append(args, *u.Venue)
		argID++
	}
	if u.City != nil {
		setClauses = append(setClauses, "city = $"+string(rune('0'+argID)))
		args = append(args, *u.City)
		argID++
	}
	if u.Country != nil {
		setClauses = append(setClauses, "country = $"+string(rune('0'+argID)))
		args = append(args, *u.Country)
		argID++
	}
	if u.EventName != nil {
		setClauses = append(setClauses, "event_name = $"+string(rune('0'+argID)))
		args = append(args, *u.EventName)
		argID++
	}
	if u.PromoterName != nil {
		setClauses = append(setClauses, "promoter_name = $"+string(rune('0'+argID)))
		args = append(args, *u.PromoterName)
		argID++
	}
	if u.PromoterEmail != nil {
		setClauses = append(setClauses, "promoter_email = $"+string(rune('0'+argID)))
		args = append(args, *u.PromoterEmail)
		argID++
	}
	if u.PromoterPhone != nil {
		setClauses = append(setClauses, "promoter_phone = $"+string(rune('0'+argID)))
		args = append(args, *u.PromoterPhone)
		argID++
	}
	if u.FeeAmount != nil {
		setClauses = append(setClauses, "fee_amount = $"+string(rune('0'+argID)))
		args = append(args, *u.FeeAmount)
		argID++
	}
	if u.FeeCurrency != nil {
		setClauses = append(setClauses, "fee_currency = $"+string(rune('0'+argID)))
		args = append(args, *u.FeeCurrency)
		argID++
	}
	if u.SetLengthMinutes != nil {
		setClauses = append(setClauses, "set_length_minutes = $"+string(rune('0'+argID)))
		args = append(args, *u.SetLengthMinutes)
		argID++
	}
	if u.Notes != nil {
		setClauses = append(setClauses, "notes = $"+string(rune('0'+argID)))
		args = append(args, *u.Notes)
		argID++
	}
	if u.Status != nil {
		setClauses = append(setClauses, "status = $"+string(rune('0'+argID)))
		args = append(args, *u.Status)
		argID++
	}
	if u.PaymentStatus != nil {
		setClauses = append(setClauses, "payment_status = $"+string(rune('0'+argID)))
		args = append(args, *u.PaymentStatus)
		argID++
	}
	if u.GigReaderVenueID != nil {
		setClauses = append(setClauses, "gig_reader_venue_id = $"+string(rune('0'+argID)))
		args = append(args, *u.GigReaderVenueID)
		argID++
	}
	if u.GigReaderContactID != nil {
		setClauses = append(setClauses, "gig_reader_contact_id = $"+string(rune('0'+argID)))
		args = append(args, *u.GigReaderContactID)
		argID++
	}

	if len(setClauses) == 0 {
		// Nothing to update; fetch and return current state
		return r.GetByID(ctx, id)
	}

	// Optimistic concurrency check
	setClauses = append(setClauses, "updated_at = NOW()")
	query := `
		UPDATE gigs
		SET ` + strings.Join(setClauses, ", ") + `
		WHERE id = $` + string(rune('0'+argID)) + ` AND deleted_at IS NULL AND updated_at = $` + string(rune('0'+argID+1))
	args = append(args, id, u.UpdatedAt)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrConflict
	}

	return r.GetByID(ctx, id)
}

// SoftDelete sets deleted_at on a gig.
func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE gigs SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LinkVenue creates a gig_venues association.
func (r *Repository) LinkVenue(ctx context.Context, gigID, venueID uuid.UUID, isPrimary bool) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO gig_venues (gig_id, venue_id, is_primary)
		VALUES ($1, $2, $3)
		ON CONFLICT (gig_id, venue_id) DO UPDATE SET is_primary = $3`,
		gigID, venueID, isPrimary,
	)
	return err
}

// UnlinkVenue removes a gig_venues association.
func (r *Repository) UnlinkVenue(ctx context.Context, gigID, venueID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM gig_venues WHERE gig_id = $1 AND venue_id = $2`,
		gigID, venueID,
	)
	return err
}

// LinkContact creates a gig_contacts association.
func (r *Repository) LinkContact(ctx context.Context, gigID, contactID uuid.UUID, role string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO gig_contacts (gig_id, contact_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (gig_id, contact_id) DO UPDATE SET role = $3`,
		gigID, contactID, role,
	)
	return err
}

// UnlinkContact removes a gig_contacts association.
func (r *Repository) UnlinkContact(ctx context.Context, gigID, contactID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM gig_contacts WHERE gig_id = $1 AND contact_id = $2`,
		gigID, contactID,
	)
	return err
}

// GetLinkedVenueID returns the primary venue ID for a gig, or nil.
func (r *Repository) GetLinkedVenueID(ctx context.Context, gigID uuid.UUID) (*uuid.UUID, error) {
	var venueID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT venue_id FROM gig_venues WHERE gig_id = $1 AND is_primary = true LIMIT 1`,
		gigID,
	).Scan(&venueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &venueID, nil
}

// GetLinkedContactID returns the contact ID for a gig with given role, or nil.
func (r *Repository) GetLinkedContactID(ctx context.Context, gigID uuid.UUID, role string) (*uuid.UUID, error) {
	var contactID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT contact_id FROM gig_contacts WHERE gig_id = $1 AND role = $2 LIMIT 1`,
		gigID, role,
	).Scan(&contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &contactID, nil
}

// Autocomplete returns gigs matching q (ILIKE), ordered by date DESC, limit results.
func (r *Repository) Autocomplete(ctx context.Context, q string, limit int) ([]*Gig, error) {
	pattern := "%" + strings.ToLower(q) + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, date, venue, city, country, event_name,
		       promoter_name, promoter_email, promoter_phone,
		       fee_amount, fee_currency, set_length_minutes, notes,
		       status, payment_status, gig_reader_venue_id, gig_reader_contact_id,
		       created_at, updated_at, deleted_at
		FROM gigs
		WHERE deleted_at IS NULL
		  AND (LOWER(venue) LIKE $1 OR LOWER(event_name) LIKE $1)
		ORDER BY date DESC
		LIMIT $2`,
		pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var gigs []*Gig
	for rows.Next() {
		var g Gig
		err := rows.Scan(
			&g.ID, &g.Date, &g.Venue, &g.City, &g.Country, &g.EventName,
			&g.PromoterName, &g.PromoterEmail, &g.PromoterPhone,
			&g.FeeAmount, &g.FeeCurrency, &g.SetLengthMinutes, &g.Notes,
			&g.Status, &g.PaymentStatus, &g.GigReaderVenueID, &g.GigReaderContactID,
			&g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		gigs = append(gigs, &g)
	}
	return gigs, rows.Err()
}
