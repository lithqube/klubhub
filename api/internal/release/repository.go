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

// InsertDeadline stores a deadline and fills in its ID and timestamps.
func (r *Repository) InsertDeadline(ctx context.Context, d *Deadline) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO release_deadlines (release_id, name, due_date, completed)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		d.ReleaseID, d.Name, d.DueDate, d.Completed,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
}

// UpdateDeadline writes a deadline and bumps updated_at.
func (r *Repository) UpdateDeadline(ctx context.Context, d *Deadline) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE release_deadlines
		SET name = $2, due_date = $3, completed = $4, updated_at = now()
		WHERE id = $1`,
		d.ID, d.Name, d.DueDate, d.Completed,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return r.pool.QueryRow(ctx, `SELECT created_at, updated_at FROM release_deadlines WHERE id = $1`, d.ID).
		Scan(&d.CreatedAt, &d.UpdatedAt)
}

// DeleteDeadline removes a deadline.
func (r *Repository) DeleteDeadline(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM release_deadlines WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetDeadline returns a deadline by ID, or ErrNotFound.
func (r *Repository) GetDeadline(ctx context.Context, id uuid.UUID) (*Deadline, error) {
	var d Deadline
	err := r.pool.QueryRow(ctx, `
		SELECT id, release_id, name, due_date, completed, created_at, updated_at
		FROM release_deadlines
		WHERE id = $1`, id,
	).Scan(&d.ID, &d.ReleaseID, &d.Name, &d.DueDate, &d.Completed, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDeadlines returns the deadlines of a release, soonest first.
func (r *Repository) ListDeadlines(ctx context.Context, releaseID uuid.UUID) ([]*Deadline, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, release_id, name, due_date, completed, created_at, updated_at
		FROM release_deadlines
		WHERE release_id = $1
		ORDER BY due_date, name`, releaseID)
	if err != nil {
		return nil, err
	}
	out := make([]*Deadline, 0, 4)
	for rows.Next() {
		var d Deadline
		if err := rows.Scan(&d.ID, &d.ReleaseID, &d.Name, &d.DueDate, &d.Completed, &d.CreatedAt, &d.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}

// DeleteDeadlinesForRelease removes every deadline of a release. A release is
// soft-deleted, so its deadlines are removed explicitly; the FK CASCADE
// covers hard deletes of the release itself.
func (r *Repository) DeleteDeadlinesForRelease(ctx context.Context, releaseID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM release_deadlines WHERE release_id = $1`, releaseID)
	return err
}

// UpsertPlatformLink stores the link of one platform, replacing the previous
// link of that platform.
func (r *Repository) UpsertPlatformLink(ctx context.Context, link *PlatformLink) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO release_platform_links (release_id, platform, url)
		VALUES ($1, $2, $3)
		ON CONFLICT (release_id, platform)
		DO UPDATE SET url = EXCLUDED.url, updated_at = now()
		RETURNING id, created_at, updated_at`,
		link.ReleaseID, link.Platform, link.URL,
	).Scan(&link.ID, &link.CreatedAt, &link.UpdatedAt)
}

// DeletePlatformLink removes the link of one platform of a release.
func (r *Repository) DeletePlatformLink(ctx context.Context, releaseID uuid.UUID, platform Platform) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM release_platform_links WHERE release_id = $1 AND platform = $2`, releaseID, platform)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPlatformLinks returns the links of a release.
func (r *Repository) ListPlatformLinks(ctx context.Context, releaseID uuid.UUID) ([]*PlatformLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, release_id, platform, url, created_at, updated_at
		FROM release_platform_links
		WHERE release_id = $1
		ORDER BY platform`, releaseID)
	if err != nil {
		return nil, err
	}
	out := make([]*PlatformLink, 0, 4)
	for rows.Next() {
		var l PlatformLink
		if err := rows.Scan(&l.ID, &l.ReleaseID, &l.Platform, &l.URL, &l.CreatedAt, &l.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, &l)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
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
