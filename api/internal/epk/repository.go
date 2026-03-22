package epk

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// repoIface defines the DB operations for the EPK package.
type repoIface interface {
	GetContent(ctx context.Context) (*EPKContent, error)
	UpsertContent(ctx context.Context, req UpsertEPKContentRequest) (*EPKContent, error)
	InsertExport(ctx context.Context, minioPath string) (*EPKExport, error)
	ListExports(ctx context.Context) ([]EPKExport, error)
	DeleteExport(ctx context.Context, id uuid.UUID) error
}

// Repository provides DB access for the EPK package.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetContent returns the singleton epk_content row, or nil if none exists yet.
func (r *Repository) GetContent(ctx context.Context) (*EPKContent, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, bio_short, bio_long, tech_rider, stage_plot_path,
		       gig_highlights, press_quotes, photo_paths, section_visibility,
		       created_at, updated_at
		FROM epk_content
		LIMIT 1`)

	return scanContent(row)
}

// UpsertContent performs INSERT ... ON CONFLICT DO UPDATE on the singleton row.
// Only non-nil fields in req are applied; JSONB slices/maps replace existing values when provided.
func (r *Repository) UpsertContent(ctx context.Context, req UpsertEPKContentRequest) (*EPKContent, error) {
	// Encode JSONB fields.
	gigHighlightsJSON, err := jsonMarshal(req.GigHighlights)
	if err != nil {
		return nil, err
	}
	pressQuotesJSON, err := jsonMarshal(req.PressQuotes)
	if err != nil {
		return nil, err
	}
	photoPathsJSON, err := jsonMarshal(req.PhotoPaths)
	if err != nil {
		return nil, err
	}
	sectionVisJSON, err := jsonMarshal(req.SectionVisibility)
	if err != nil {
		return nil, err
	}

	// Resolve string fields — use empty string as default when nil.
	bioShort := ""
	if req.BioShort != nil {
		bioShort = *req.BioShort
	}
	bioLong := ""
	if req.BioLong != nil {
		bioLong = *req.BioLong
	}
	techRider := ""
	if req.TechRider != nil {
		techRider = *req.TechRider
	}
	stagePlotPath := ""
	if req.StagePlotPath != nil {
		stagePlotPath = *req.StagePlotPath
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO epk_content
			(bio_short, bio_long, tech_rider, stage_plot_path,
			 gig_highlights, press_quotes, photo_paths, section_visibility)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT ((true)) DO UPDATE
			SET bio_short          = CASE WHEN $1::text != '' THEN $1 ELSE epk_content.bio_short END,
			    bio_long           = CASE WHEN $2::text != '' THEN $2 ELSE epk_content.bio_long END,
			    tech_rider         = CASE WHEN $3::text != '' THEN $3 ELSE epk_content.tech_rider END,
			    stage_plot_path    = CASE WHEN $4::text != '' THEN $4 ELSE epk_content.stage_plot_path END,
			    gig_highlights     = EXCLUDED.gig_highlights,
			    press_quotes       = EXCLUDED.press_quotes,
			    photo_paths        = EXCLUDED.photo_paths,
			    section_visibility = EXCLUDED.section_visibility,
			    updated_at         = now()
		RETURNING id, bio_short, bio_long, tech_rider, stage_plot_path,
		          gig_highlights, press_quotes, photo_paths, section_visibility,
		          created_at, updated_at`,
		bioShort, bioLong, techRider, stagePlotPath,
		gigHighlightsJSON, pressQuotesJSON, photoPathsJSON, sectionVisJSON,
	)

	return scanContent(row)
}

// InsertExport inserts a new epk_exports row and returns the created record.
func (r *Repository) InsertExport(ctx context.Context, minioPath string) (*EPKExport, error) {
	var e EPKExport
	err := r.pool.QueryRow(ctx, `
		INSERT INTO epk_exports (id, minio_path, created_at)
		VALUES (gen_random_uuid(), $1, now())
		RETURNING id, minio_path, created_at`,
		minioPath,
	).Scan(&e.ID, &e.MinioPath, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListExports returns all export rows ordered by created_at DESC (newest first).
func (r *Repository) ListExports(ctx context.Context) ([]EPKExport, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, minio_path, created_at
		FROM epk_exports
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exports []EPKExport
	for rows.Next() {
		var e EPKExport
		if err := rows.Scan(&e.ID, &e.MinioPath, &e.CreatedAt); err != nil {
			return nil, err
		}
		exports = append(exports, e)
	}
	return exports, rows.Err()
}

// DeleteExport hard-deletes an export row by ID. Returns ErrNotFound if no row matched.
func (r *Repository) DeleteExport(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM epk_exports WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// scanContent reads a single epk_content row from a pgx.Row.
func scanContent(row pgx.Row) (*EPKContent, error) {
	var c EPKContent
	var gigHighlightsJSON, pressQuotesJSON, photoPathsJSON, sectionVisJSON []byte

	err := row.Scan(
		&c.ID, &c.BioShort, &c.BioLong, &c.TechRider, &c.StagePlotPath,
		&gigHighlightsJSON, &pressQuotesJSON, &photoPathsJSON, &sectionVisJSON,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(gigHighlightsJSON, &c.GigHighlights); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(pressQuotesJSON, &c.PressQuotes); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(photoPathsJSON, &c.PhotoPaths); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(sectionVisJSON, &c.SectionVisibility); err != nil {
		return nil, err
	}

	return &c, nil
}

// jsonMarshal encodes v to JSON bytes, returning "null" bytes for nil slices/maps.
func jsonMarshal(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}
