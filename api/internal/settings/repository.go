package settings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data access for the user_settings singleton.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given pgxpool.Pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetOrCreate returns the active (non-deleted) user_settings row.
// If no row exists, it inserts one with defaults and returns it.
// All queries include WHERE deleted_at IS NULL.
func (r *Repository) GetOrCreate(ctx context.Context) (*UserSettings, error) {
	us, err := r.Get(ctx)
	if err == nil {
		return us, nil
	}
	if err != ErrNotFound {
		return nil, err
	}

	// Insert with defaults; DB defaults handle everything we need.
	const insertSQL = `
		INSERT INTO user_settings DEFAULT VALUES
		RETURNING id, dj_name, logo_path, default_colors, default_template,
		          visible_fields, social_links, bio_short, bio_long,
		          contact_info, invoice_prefix, created_at, updated_at, deleted_at`

	rows, err := r.pool.Query(ctx, insertSQL)
	if err != nil {
		return nil, err
	}
	return scanOne(rows)
}

// Get returns the active user_settings row or ErrNotFound if none exists.
// All queries include WHERE deleted_at IS NULL.
func (r *Repository) Get(ctx context.Context) (*UserSettings, error) {
	const selectSQL = `
		SELECT id, dj_name, logo_path, default_colors, default_template,
		       visible_fields, social_links, bio_short, bio_long,
		       contact_info, invoice_prefix, created_at, updated_at, deleted_at
		FROM user_settings
		WHERE deleted_at IS NULL
		LIMIT 1`

	rows, err := r.pool.Query(ctx, selectSQL)
	if err != nil {
		return nil, err
	}
	us, err := scanOne(rows)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return us, err
}

// Update modifies the user_settings row.
// req.UpdatedAt must exactly match the current updated_at in the database
// (optimistic concurrency check). If not, ErrConflict is returned.
// All queries include WHERE deleted_at IS NULL.
func (r *Repository) Update(ctx context.Context, req UpdateSettingsRequest) (*UserSettings, error) {
	// Marshal JSONB fields
	defaultColors, err := marshalJSON(req.DefaultColors)
	if err != nil {
		return nil, err
	}
	visibleFields, err := marshalJSON(req.VisibleFields)
	if err != nil {
		return nil, err
	}
	socialLinks, err := marshalJSON(req.SocialLinks)
	if err != nil {
		return nil, err
	}

	const updateSQL = `
		UPDATE user_settings
		SET dj_name          = $1,
		    logo_path         = $2,
		    default_colors    = $3,
		    default_template  = $4,
		    visible_fields    = $5,
		    social_links      = $6,
		    bio_short         = $7,
		    bio_long          = $8,
		    contact_info      = $9,
		    invoice_prefix    = $10,
		    updated_at        = NOW()
		WHERE deleted_at IS NULL
		  AND updated_at = $11
		RETURNING id, dj_name, logo_path, default_colors, default_template,
		          visible_fields, social_links, bio_short, bio_long,
		          contact_info, invoice_prefix, created_at, updated_at, deleted_at`

	rows, err := r.pool.Query(ctx, updateSQL,
		req.DJName,
		req.LogoPath,
		defaultColors,
		req.DefaultTemplate,
		visibleFields,
		socialLinks,
		req.BioShort,
		req.BioLong,
		req.ContactInfo,
		req.InvoicePrefix,
		req.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	us, err := scanOne(rows)
	if err == pgx.ErrNoRows {
		return nil, ErrConflict
	}
	return us, err
}

// scanOne collects the first row from rows into a UserSettings struct.
// Returns pgx.ErrNoRows if the result set is empty.
func scanOne(rows pgx.Rows) (*UserSettings, error) {
	defer rows.Close()

	type rawRow struct {
		ID              [16]byte
		DJName          string
		LogoPath        string
		DefaultColors   []byte
		DefaultTemplate string
		VisibleFields   []byte
		SocialLinks     []byte
		BioShort        string
		BioLong         string
		ContactInfo     string
		InvoicePrefix   string
		CreatedAt       interface{}
		UpdatedAt       interface{}
		DeletedAt       interface{}
	}

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}

	us := &UserSettings{}
	var defaultColorsRaw, visibleFieldsRaw, socialLinksRaw []byte

	err := rows.Scan(
		&us.ID,
		&us.DJName,
		&us.LogoPath,
		&defaultColorsRaw,
		&us.DefaultTemplate,
		&visibleFieldsRaw,
		&socialLinksRaw,
		&us.BioShort,
		&us.BioLong,
		&us.ContactInfo,
		&us.InvoicePrefix,
		&us.CreatedAt,
		&us.UpdatedAt,
		&us.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSONB fields
	if len(defaultColorsRaw) > 0 {
		if err := json.Unmarshal(defaultColorsRaw, &us.DefaultColors); err != nil {
			return nil, err
		}
	}
	if us.DefaultColors == nil {
		us.DefaultColors = map[string]any{}
	}

	if len(visibleFieldsRaw) > 0 {
		if err := json.Unmarshal(visibleFieldsRaw, &us.VisibleFields); err != nil {
			return nil, err
		}
	}
	if us.VisibleFields == nil {
		us.VisibleFields = map[string]any{}
	}

	if len(socialLinksRaw) > 0 {
		if err := json.Unmarshal(socialLinksRaw, &us.SocialLinks); err != nil {
			return nil, err
		}
	}
	if us.SocialLinks == nil {
		us.SocialLinks = map[string]any{}
	}

	return us, nil
}

// marshalJSON encodes a map to JSON bytes for JSONB parameters.
// Returns "{}" for nil maps.
func marshalJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}
