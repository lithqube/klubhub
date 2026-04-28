package venue

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Venue represents a reusable venue record
type Venue struct {
	ID                uuid.UUID  `db:"id"`
	Name              string     `db:"name"`
	City              string     `db:"city"`
	Country           string     `db:"country"`
	Capacity          *int       `db:"capacity"`           // optional
	Website           *string    `db:"website"`            // optional
	TechContactName   string     `db:"tech_contact_name"`
	TechContactEmail  string     `db:"tech_contact_email"`
	TechContactPhone  string     `db:"tech_contact_phone"`
	Notes             string     `db:"notes"`
	CreatedAt         time.Time  `db:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"`
	DeletedAt         *time.Time `db:"deleted_at"` // soft delete
}

// VenueCreate contains fields required to create a new venue
type VenueCreate struct {
	Name             string  `json:"name" db:"name"`
	City             string  `json:"city" db:"city"`
	Country          string  `json:"country" db:"country"`
	Capacity         *int    `json:"capacity,omitempty" db:"capacity"`
	Website          *string `json:"website,omitempty" db:"website"`
	TechContactName  string  `json:"tech_contact_name" db:"tech_contact_name"`
	TechContactEmail string  `json:"tech_contact_email" db:"tech_contact_email"`
	TechContactPhone string  `json:"tech_contact_phone" db:"tech_contact_phone"`
	Notes            string  `json:"notes" db:"notes"`
}

// VenueUpdate contains optional fields for updating a venue
type VenueUpdate struct {
	Name             *string    `json:"name,omitempty" db:"name"`
	City             *string    `json:"city,omitempty" db:"city"`
	Country          *string    `json:"country,omitempty" db:"country"`
	Capacity         *int       `json:"capacity,omitempty" db:"capacity"`
	Website          *string    `json:"website,omitempty" db:"website"`
	TechContactName  *string    `json:"tech_contact_name,omitempty" db:"tech_contact_name"`
	TechContactEmail *string    `json:"tech_contact_email,omitempty" db:"tech_contact_email"`
	TechContactPhone *string    `json:"tech_contact_phone,omitempty" db:"tech_contact_phone"`
	Notes            *string    `json:"notes,omitempty" db:"notes"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"` // optimistic concurrency
}

// Sentinel errors
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)
