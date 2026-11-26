package release

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides DB access for releases.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a release and fills in its ID and timestamps.
func (r *Repository) Create(ctx context.Context, rel *Release) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO releases (title, artists, label, distributor, release_date, format, notes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		rel.Title, rel.Artists, rel.Label, rel.Distributor, rel.ReleaseDate, rel.Format, rel.Notes, rel.Status,
	).Scan(&rel.ID, &rel.CreatedAt, &rel.UpdatedAt)
}

// Update writes the editable fields and bumps updated_at.
func (r *Repository) Update(ctx context.Context, rel *Release) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET title = $2, artists = $3, label = $4, distributor = $5,
		    release_date = $6, format = $7, notes = $8, status = $9, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`,
		rel.ID, rel.Title, rel.Artists, rel.Label, rel.Distributor, rel.ReleaseDate, rel.Format, rel.Notes, rel.Status,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return r.pool.QueryRow(ctx, `SELECT created_at, updated_at FROM releases WHERE id = $1`, rel.ID).
		Scan(&rel.CreatedAt, &rel.UpdatedAt)
}

// SoftDelete marks a release deleted; it disappears from reads but the row
// stays for audit.
func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE releases SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByID returns a live release by ID, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Release, error) {
	var rel Release
	err := r.pool.QueryRow(ctx, `
		SELECT id, title, artists, label, distributor, release_date, format, notes, status,
		       created_at, updated_at, deleted_at
		FROM releases
		WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(
		&rel.ID, &rel.Title, &rel.Artists, &rel.Label, &rel.Distributor, &rel.ReleaseDate,
		&rel.Format, &rel.Notes, &rel.Status, &rel.CreatedAt, &rel.UpdatedAt, &rel.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rel, nil
}

// List returns every live release ordered by release date, then title.
func (r *Repository) List(ctx context.Context) ([]*Release, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, artists, label, distributor, release_date, format, notes, status,
		       created_at, updated_at, deleted_at
		FROM releases
		WHERE deleted_at IS NULL
		ORDER BY release_date, title`)
	if err != nil {
		return nil, err
	}
	out := make([]*Release, 0, 8)
	for rows.Next() {
		var rel Release
		if err := rows.Scan(
			&rel.ID, &rel.Title, &rel.Artists, &rel.Label, &rel.Distributor, &rel.ReleaseDate,
			&rel.Format, &rel.Notes, &rel.Status, &rel.CreatedAt, &rel.UpdatedAt, &rel.DeletedAt,
		); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, &rel)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}
