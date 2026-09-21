package contact

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Contact type enum
type ContactType string

const (
	ContactTypePromoter ContactType = "promoter"
	ContactTypeAgent    ContactType = "agent"
	ContactTypeLabel    ContactType = "label"
	ContactTypeOther    ContactType = "other"
)

// Contact represents a reusable contact record (promoter, agent, label, etc.)
type Contact struct {
	ID        uuid.UUID   `json:"id" db:"id"`
	Name      string      `json:"name" db:"name"`
	Company   *string     `json:"company" db:"company"` // optional
	Email     string      `json:"email" db:"email"`
	Phone     string      `json:"phone" db:"phone"`
	Type      ContactType `json:"type" db:"type"`
	Notes     string      `json:"notes" db:"notes"`
	CreatedAt time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt time.Time   `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time  `json:"deleted_at" db:"deleted_at"` // soft delete
}

// ContactCreate contains fields required to create a new contact
type ContactCreate struct {
	Name    string      `json:"name" db:"name"`
	Company *string     `json:"company,omitempty" db:"company"`
	Email   string      `json:"email" db:"email"`
	Phone   string      `json:"phone" db:"phone"`
	Type    ContactType `json:"type" db:"type"`
	Notes   string      `json:"notes" db:"notes"`
}

// ContactUpdate contains optional fields for updating a contact
type ContactUpdate struct {
	Name      *string      `json:"name,omitempty" db:"name"`
	Company   *string      `json:"company,omitempty" db:"company"`
	Email     *string      `json:"email,omitempty" db:"email"`
	Phone     *string      `json:"phone,omitempty" db:"phone"`
	Type      *ContactType `json:"type,omitempty" db:"type"`
	Notes     *string      `json:"notes,omitempty" db:"notes"`
	UpdatedAt time.Time    `json:"updated_at" db:"updated_at"` // optimistic concurrency
}

// Sentinel errors
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)
