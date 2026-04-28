package contact

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides DB access for contacts.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetByID returns a contact by ID, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Contact, error) {
	var c Contact
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, company, email, phone, type, notes,
		       created_at, updated_at, deleted_at
		FROM contacts
		WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(
		&c.ID, &c.Name, &c.Company, &c.Email, &c.Phone, &c.Type, &c.Notes,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List returns all contacts (optionally filtered by name/type), ordered by name.
func (r *Repository) List(ctx context.Context, name *string, contactType *ContactType) ([]*Contact, error) {
	query := `
		SELECT id, name, company, email, phone, type, notes,
		       created_at, updated_at, deleted_at
		FROM contacts
		WHERE deleted_at IS NULL`
	args := []interface{}{}
	argID := 1

	if name != nil && *name != "" {
		query += " AND name ILIKE $" + string(rune('0'+argID))
		args = append(args, "%"+*name+"%")
		argID++
	}
	if contactType != nil {
		query += " AND type = $" + string(rune('0'+argID))
		args = append(args, *contactType)
		argID++
	}

	query += " ORDER BY name ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []*Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(
			&c.ID, &c.Name, &c.Company, &c.Email, &c.Phone, &c.Type, &c.Notes,
			&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, &c)
	}
	return contacts, rows.Err()
}

// Create inserts a new contact and returns it.
func (r *Repository) Create(ctx context.Context, c *ContactCreate) (*Contact, error) {
	var out Contact
	err := r.pool.QueryRow(ctx, `
		INSERT INTO contacts (
			id, name, company, email, phone, type, notes,
			created_at, updated_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, now(), now()
		) RETURNING
			id, name, company, email, phone, type, notes,
			created_at, updated_at, deleted_at`,
		c.Name, c.Company, c.Email, c.Phone, c.Type, c.Notes,
	).Scan(
		&out.ID, &out.Name, &out.Company, &out.Email, &out.Phone, &out.Type, &out.Notes,
		&out.CreatedAt, &out.UpdatedAt, &out.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Update applies non-nil changes from u to the contact identified by id.
func (r *Repository) Update(ctx context.Context, id uuid.UUID, u *ContactUpdate) (*Contact, error) {
	setClauses := []string{}
	args := []interface{}{}
	argID := 1

	if u.Name != nil {
		setClauses = append(setClauses, "name = $"+string(rune('0'+argID)))
		args = append(args, *u.Name)
		argID++
	}
	if u.Company != nil {
		setClauses = append(setClauses, "company = $"+string(rune('0'+argID)))
		args = append(args, *u.Company)
		argID++
	}
	if u.Email != nil {
		setClauses = append(setClauses, "email = $"+string(rune('0'+argID)))
		args = append(args, *u.Email)
		argID++
	}
	if u.Phone != nil {
		setClauses = append(setClauses, "phone = $"+string(rune('0'+argID)))
		args = append(args, *u.Phone)
		argID++
	}
	if u.Type != nil {
		setClauses = append(setClauses, "type = $"+string(rune('0'+argID)))
		args = append(args, *u.Type)
		argID++
	}
	if u.Notes != nil {
		setClauses = append(setClauses, "notes = $"+string(rune('0'+argID)))
		args = append(args, *u.Notes)
		argID++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := `
		UPDATE contacts
		SET ` + strings.Join(setClauses, ", ") + `
		WHERE id = $` + string(rune('0'+argID)) + ` AND deleted_at IS NULL`
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

// SoftDelete sets deleted_at on a contact.
func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE contacts SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
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

// SearchByName returns contacts matching query (ILIKE), ordered by name, limit results.
func (r *Repository) SearchByName(ctx context.Context, query string, limit int) ([]*Contact, error) {
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, company, email, phone, type, notes,
		       created_at, updated_at, deleted_at
		FROM contacts
		WHERE deleted_at IS NULL AND LOWER(name) LIKE $1
		ORDER BY name ASC
		LIMIT $2`,
		pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []*Contact
	for rows.Next() {
		var c Contact
		err := rows.Scan(
			&c.ID, &c.Name, &c.Company, &c.Email, &c.Phone, &c.Type, &c.Notes,
			&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, &c)
	}
	return contacts, rows.Err()
}
