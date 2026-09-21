package venue

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides DB access for venues.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetByID returns a venue by ID, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Venue, error) {
	var v Venue
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, city, country, capacity, website,
		       tech_contact_name, tech_contact_email, tech_contact_phone,
		       notes, created_at, updated_at, deleted_at
		FROM venues
		WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(
		&v.ID, &v.Name, &v.City, &v.Country, &v.Capacity, &v.Website,
		&v.TechContactName, &v.TechContactEmail, &v.TechContactPhone,
		&v.Notes, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// List returns all venues (optionally filtered by name/city), ordered by name.
func (r *Repository) List(ctx context.Context, name, city *string) ([]*Venue, error) {
	query := `
		SELECT id, name, city, country, capacity, website,
		       tech_contact_name, tech_contact_email, tech_contact_phone,
		       notes, created_at, updated_at, deleted_at
		FROM venues
		WHERE deleted_at IS NULL`
	args := []interface{}{}
	argID := 1

	if name != nil && *name != "" {
		query += " AND name ILIKE $" + strconv.Itoa(argID)
		args = append(args, "%"+*name+"%")
		argID++
	}
	if city != nil && *city != "" {
		query += " AND city ILIKE $" + strconv.Itoa(argID)
		args = append(args, "%"+*city+"%")
		argID++
	}

	query += " ORDER BY name ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var venues []*Venue
	for rows.Next() {
		var v Venue
		err := rows.Scan(
			&v.ID, &v.Name, &v.City, &v.Country, &v.Capacity, &v.Website,
			&v.TechContactName, &v.TechContactEmail, &v.TechContactPhone,
			&v.Notes, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		venues = append(venues, &v)
	}
	return venues, rows.Err()
}

// Create inserts a new venue and returns it.
func (r *Repository) Create(ctx context.Context, v *VenueCreate) (*Venue, error) {
	var out Venue
	err := r.pool.QueryRow(ctx, `
		INSERT INTO venues (
			id, name, city, country, capacity, website,
			tech_contact_name, tech_contact_email, tech_contact_phone,
			notes, created_at, updated_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now()
		) RETURNING
			id, name, city, country, capacity, website,
			tech_contact_name, tech_contact_email, tech_contact_phone,
			notes, created_at, updated_at, deleted_at`,
		v.Name, v.City, v.Country, v.Capacity, v.Website,
		v.TechContactName, v.TechContactEmail, v.TechContactPhone, v.Notes,
	).Scan(
		&out.ID, &out.Name, &out.City, &out.Country, &out.Capacity, &out.Website,
		&out.TechContactName, &out.TechContactEmail, &out.TechContactPhone,
		&out.Notes, &out.CreatedAt, &out.UpdatedAt, &out.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Update applies non-nil changes from u to the venue identified by id.
func (r *Repository) Update(ctx context.Context, id uuid.UUID, u *VenueUpdate) (*Venue, error) {
	setClauses := []string{}
	args := []interface{}{}
	argID := 1

	if u.Name != nil {
		setClauses = append(setClauses, "name = $"+strconv.Itoa(argID))
		args = append(args, *u.Name)
		argID++
	}
	if u.City != nil {
		setClauses = append(setClauses, "city = $"+strconv.Itoa(argID))
		args = append(args, *u.City)
		argID++
	}
	if u.Country != nil {
		setClauses = append(setClauses, "country = $"+strconv.Itoa(argID))
		args = append(args, *u.Country)
		argID++
	}
	if u.Capacity != nil {
		setClauses = append(setClauses, "capacity = $"+strconv.Itoa(argID))
		args = append(args, *u.Capacity)
		argID++
	}
	if u.Website != nil {
		setClauses = append(setClauses, "website = $"+strconv.Itoa(argID))
		args = append(args, *u.Website)
		argID++
	}
	if u.TechContactName != nil {
		setClauses = append(setClauses, "tech_contact_name = $"+strconv.Itoa(argID))
		args = append(args, *u.TechContactName)
		argID++
	}
	if u.TechContactEmail != nil {
		setClauses = append(setClauses, "tech_contact_email = $"+strconv.Itoa(argID))
		args = append(args, *u.TechContactEmail)
		argID++
	}
	if u.TechContactPhone != nil {
		setClauses = append(setClauses, "tech_contact_phone = $"+strconv.Itoa(argID))
		args = append(args, *u.TechContactPhone)
		argID++
	}
	if u.Notes != nil {
		setClauses = append(setClauses, "notes = $"+strconv.Itoa(argID))
		args = append(args, *u.Notes)
		argID++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := `
		UPDATE venues
		SET ` + strings.Join(setClauses, ", ") + `
		WHERE id = $` + strconv.Itoa(argID) + ` AND deleted_at IS NULL`
	args = append(args, id)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	return r.GetByID(ctx, id)
}

// SoftDelete sets deleted_at on a venue.
func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE venues SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
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

// SearchByName returns venues matching query (ILIKE), ordered by name, limit results.
func (r *Repository) SearchByName(ctx context.Context, query string, limit int) ([]*Venue, error) {
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, city, country, capacity, website,
		       tech_contact_name, tech_contact_email, tech_contact_phone,
		       notes, created_at, updated_at, deleted_at
		FROM venues
		WHERE deleted_at IS NULL AND LOWER(name) LIKE $1
		ORDER BY name ASC
		LIMIT $2`,
		pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var venues []*Venue
	for rows.Next() {
		var v Venue
		err := rows.Scan(
			&v.ID, &v.Name, &v.City, &v.Country, &v.Capacity, &v.Website,
			&v.TechContactName, &v.TechContactEmail, &v.TechContactPhone,
			&v.Notes, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		venues = append(venues, &v)
	}
	return venues, rows.Err()
}
