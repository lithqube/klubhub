package rider

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the concrete Postgres-backed implementation of RepoIface.
// All read paths filter WHERE deleted_at IS NULL; soft-delete sets
// deleted_at to now() in the same UPDATE that touches the row.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ─── Templates ─────────────────────────────────────────────────────────────

// ListTemplates returns every non-deleted template sorted by name (case-
// sensitive ASCII order — locale-aware sort can come later if requested).
func (r *Repository) ListTemplates(ctx context.Context) ([]*RiderTemplate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, technical, hospitality, backline, other_notes,
		       created_at, updated_at, deleted_at
		FROM rider_templates
		WHERE deleted_at IS NULL
		ORDER BY name COLLATE "C" ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*RiderTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTemplate returns one non-deleted template by ID, or ErrNotFound.
func (r *Repository) GetTemplate(ctx context.Context, id uuid.UUID) (*RiderTemplate, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, technical, hospitality, backline, other_notes,
		       created_at, updated_at, deleted_at
		FROM rider_templates
		WHERE id = $1 AND deleted_at IS NULL`, id)
	t, err := scanTemplate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// CreateTemplate inserts a new template row and returns the populated
// record (server-assigned ID + timestamps). Name is required; the four
// section strings default to empty when absent.
func (r *Repository) CreateTemplate(ctx context.Context, in CreateTemplateInput) (*RiderTemplate, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO rider_templates (name, technical, hospitality, backline, other_notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, technical, hospitality, backline, other_notes,
		          created_at, updated_at, deleted_at`,
		in.Name, in.Technical, in.Hospitality, in.Backline, in.OtherNotes,
	)
	t, err := scanTemplate(row)
	if isNameTaken(err) {
		return nil, nameTakenError(in.Name)
	}
	return t, err
}

// isNameTaken reports whether err is the unique violation on the live
// template name index (rider_templates_name_unique).
func isNameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "rider_templates_name_unique"
}

// UpdateTemplate performs a partial update. Each *string pointer drives a
// COALESCE-style "update when non-nil, leave alone otherwise" — matches
// EPK's CASE WHEN pattern but is simpler to read. Name updates touch the
// column directly; every section is independently optional (nil → that
// column is left alone, pointer to "" → cleared).
func (r *Repository) UpdateTemplate(ctx context.Context, id uuid.UUID, in UpdateTemplateInput) (*RiderTemplate, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE rider_templates
		SET name        = COALESCE($2, name),
		    technical   = COALESCE($3, technical),
		    hospitality = COALESCE($4, hospitality),
		    backline    = COALESCE($5, backline),
		    other_notes = COALESCE($6, other_notes),
		    updated_at  = now()
		WHERE id = $1 AND deleted_at IS NULL AND updated_at = $7
		RETURNING id, name, technical, hospitality, backline, other_notes,
		          created_at, updated_at, deleted_at`,
		id, in.Name, in.Technical, in.Hospitality, in.Backline, in.OtherNotes, in.UpdatedAt,
	)
	t, err := scanTemplate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, r.missOrConflict(ctx, "rider_templates", id)
	}
	if isNameTaken(err) && in.Name != nil {
		return nil, nameTakenError(*in.Name)
	}
	return t, err
}

// missOrConflict explains why a compare-and-set update touched no row: the
// record is gone (ErrNotFound) or it exists but changed since the caller read
// it (ErrStaleUpdate). table is always a package constant, never user input.
func (r *Repository) missOrConflict(ctx context.Context, table string, id uuid.UUID) error {
	var one int
	err := r.pool.QueryRow(ctx,
		`SELECT 1 FROM `+table+` WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrStaleUpdate
}

// SoftDeleteTemplate sets deleted_at on a live template. Idempotent:
// calling this on an already-deleted template returns ErrNotFound because
// the WHERE deleted_at IS NULL clause matches zero rows.
func (r *Repository) SoftDeleteTemplate(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	tag, err := tx.Exec(ctx, `
		UPDATE rider_templates
		SET deleted_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	// Per-gig copies keep their content but lose the pointer to a template
	// that no longer exists (the FK's SET NULL only fires on a hard delete).
	// updated_at is deliberately left alone: template_id is not editable
	// content, and bumping it would invalidate every open editor's token.
	if _, err := tx.Exec(ctx,
		`UPDATE rider_attachments SET template_id = NULL WHERE template_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ─── Attachments ────────────────────────────────────────────────────────────

// GetAttachmentByGig returns the live attachment for a gig, or ErrNotFound
// when none exists (including when one existed and was soft-deleted).
func (r *Repository) GetAttachmentByGig(ctx context.Context, gigID uuid.UUID) (*RiderAttachment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, gig_id, template_id, technical, hospitality, backline,
		       other_notes, created_at, updated_at, deleted_at
		FROM rider_attachments
		WHERE gig_id = $1 AND deleted_at IS NULL
		LIMIT 1`, gigID)
	a, err := scanAttachment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// GetAttachment returns one non-deleted attachment by ID, or ErrNotFound.
func (r *Repository) GetAttachment(ctx context.Context, id uuid.UUID) (*RiderAttachment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, gig_id, template_id, technical, hospitality, backline,
		       other_notes, created_at, updated_at, deleted_at
		FROM rider_attachments
		WHERE id = $1 AND deleted_at IS NULL`, id)
	a, err := scanAttachment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// CreateAttachment inserts a new attachment row and returns the populated
// record. Two Postgres error codes are translated:
//   - 23503 (foreign_key_violation) on gig_id    → ErrNotFound (the gig is
//     missing; the FK exists because rider_attachments.gig_id REFERENCES
//     gigs.id).
//   - 23505 (unique_violation) on the partial index
//     rider_attachments_one_per_gig                 → ErrConflict (a live
//     attachment for this gig already exists; concurrent insert race).
func (r *Repository) CreateAttachment(ctx context.Context, in CreateAttachmentInput) (*RiderAttachment, error) {
	// INSERT ... SELECT from gigs so the liveness check and the insert are
	// one statement: gigs are soft-deleted (the FK never fires for them), so
	// a plain INSERT would happily attach a rider to a deleted gig where
	// nothing could ever reach it. No live gig -> no row -> ErrNotFound.
	row := r.pool.QueryRow(ctx, `
		INSERT INTO rider_attachments
			(gig_id, template_id, technical, hospitality, backline, other_notes)
		SELECT g.id, $2::uuid, $3::text, $4::text, $5::text, $6::text
		FROM gigs g
		WHERE g.id = $1 AND g.deleted_at IS NULL
		RETURNING id, gig_id, template_id, technical, hospitality, backline,
		          other_notes, created_at, updated_at, deleted_at`,
		in.GigID, in.TemplateID, in.Technical, in.Hospitality, in.Backline, in.OtherNotes,
	)
	a, err := scanAttachment(row)
	if err == nil {
		return a, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	// Translate FK / unique-violation errors. The pgxpool wraps the
	// underlying pgconn.PgError; SQLSTATE codes are documented by Postgres
	// and stable across versions.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return nil, ErrNotFound
		case "23505":
			return nil, ErrConflict
		}
	}
	return nil, err
}

// UpdateAttachment performs a partial update of the four section columns.
// TemplateID is intentionally not updatable here: a per-gig copy's source
// is fixed at insert time so the user's "where did this come from"
// question is stable. Callers that want to "re-source" an attachment
// should delete and recreate it.
func (r *Repository) UpdateAttachment(ctx context.Context, id uuid.UUID, in UpdateAttachmentInput) (*RiderAttachment, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE rider_attachments
		SET technical   = COALESCE($2, technical),
		    hospitality = COALESCE($3, hospitality),
		    backline    = COALESCE($4, backline),
		    other_notes = COALESCE($5, other_notes),
		    updated_at  = now()
		WHERE id = $1 AND deleted_at IS NULL AND updated_at = $6
		RETURNING id, gig_id, template_id, technical, hospitality, backline,
		          other_notes, created_at, updated_at, deleted_at`,
		id, in.Technical, in.Hospitality, in.Backline, in.OtherNotes, in.UpdatedAt,
	)
	a, err := scanAttachment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, r.missOrConflict(ctx, "rider_attachments", id)
	}
	return a, err
}

// SoftDeleteAttachment sets deleted_at on a live attachment. Idempotent:
// a second call on the same ID returns ErrNotFound because the WHERE
// deleted_at IS NULL predicate matches zero rows after the first call.
func (r *Repository) SoftDeleteAttachment(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE rider_attachments
		SET deleted_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Row scanners ───────────────────────────────────────────────────────────

// scanTemplate reads a single rider_templates row from a pgx.Row. pgx.ErrNoRows
// is propagated so callers can translate it to ErrNotFound where appropriate
// (CreateTemplate/UpdateTemplate paths handle that themselves; ListTemplates
// uses the rows iterator and never sees ErrNoRows).
func scanTemplate(row pgx.Row) (*RiderTemplate, error) {
	var t RiderTemplate
	err := row.Scan(
		&t.ID, &t.Name, &t.Technical, &t.Hospitality, &t.Backline,
		&t.OtherNotes, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// scanAttachment reads a single rider_attachments row from a pgx.Row. Same
// pgx.ErrNoRows propagation rule as scanTemplate.
func scanAttachment(row pgx.Row) (*RiderAttachment, error) {
	var a RiderAttachment
	err := row.Scan(
		&a.ID, &a.GigID, &a.TemplateID, &a.Technical, &a.Hospitality,
		&a.Backline, &a.OtherNotes, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
