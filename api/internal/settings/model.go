package settings

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors used by repository and service layers.
var (
	ErrConflict = errors.New("conflict")
	ErrNotFound = errors.New("not found")
)

// UserSettings represents the singleton row in the user_settings table.
type UserSettings struct {
	ID              uuid.UUID      `json:"id"                db:"id"`
	DJName          string         `json:"dj_name"           db:"dj_name"`
	LogoPath        string         `json:"logo_path"         db:"logo_path"`
	DefaultColors   map[string]any `json:"default_colors"    db:"default_colors"`
	DefaultTemplate string         `json:"default_template"  db:"default_template"`
	VisibleFields   map[string]any `json:"visible_fields"    db:"visible_fields"`
	SocialLinks     map[string]any `json:"social_links"      db:"social_links"`
	BioShort        string         `json:"bio_short"         db:"bio_short"`
	BioLong         string         `json:"bio_long"          db:"bio_long"`
	ContactInfo     string         `json:"contact_info"      db:"contact_info"`
	InvoicePrefix   string         `json:"invoice_prefix"    db:"invoice_prefix"`
	CreatedAt       time.Time      `json:"created_at"        db:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"        db:"updated_at"`
	DeletedAt       *time.Time     `json:"deleted_at,omitempty" db:"deleted_at"`
}

// UpdateSettingsRequest contains fields that may be changed via PUT /api/v1/settings.
// UpdatedAt is the optimistic concurrency token — it must match the current updated_at in DB.
type UpdateSettingsRequest struct {
	DJName          string         `json:"dj_name"`
	LogoPath        string         `json:"logo_path"`
	DefaultColors   map[string]any `json:"default_colors"`
	DefaultTemplate string         `json:"default_template"`
	VisibleFields   map[string]any `json:"visible_fields"`
	SocialLinks     map[string]any `json:"social_links"`
	BioShort        string         `json:"bio_short"`
	BioLong         string         `json:"bio_long"`
	ContactInfo     string         `json:"contact_info"`
	InvoicePrefix   string         `json:"invoice_prefix"`
	UpdatedAt       time.Time      `json:"updated_at"` // optimistic concurrency token
}
