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
	ID      uuid.UUID   `json:"id" db:"id"`
	Name    string      `json:"name" db:"name"`
	Company *string     `json:"company" db:"company"` // optional
	Email   string      `json:"email" db:"email"`
	Phone   string      `json:"phone" db:"phone"`
	Type    ContactType `json:"type" db:"type"`
	Notes   string      `json:"notes" db:"notes"`
	PartyFields
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" db:"deleted_at"` // soft delete
}

// ContactCreate contains fields required to create a new contact
type ContactCreate struct {
	Name    string      `json:"name" db:"name"`
	Company *string     `json:"company,omitempty" db:"company"`
	Email   string      `json:"email" db:"email"`
	Phone   string      `json:"phone" db:"phone"`
	Type    ContactType `json:"type" db:"type"`
	Notes   string      `json:"notes" db:"notes"`
	PartyFields
}

// ContactUpdate contains optional fields for updating a contact
type ContactUpdate struct {
	Name    *string      `json:"name,omitempty" db:"name"`
	Company *string      `json:"company,omitempty" db:"company"`
	Email   *string      `json:"email,omitempty" db:"email"`
	Phone   *string      `json:"phone,omitempty" db:"phone"`
	Type    *ContactType `json:"type,omitempty" db:"type"`
	Notes   *string      `json:"notes,omitempty" db:"notes"`

	AddressLine1 *string `json:"address_line1,omitempty" db:"address_line1"`
	AddressLine2 *string `json:"address_line2,omitempty" db:"address_line2"`
	City         *string `json:"city,omitempty" db:"city"`
	Region       *string `json:"region,omitempty" db:"region"`
	PostalCode   *string `json:"postal_code,omitempty" db:"postal_code"`
	Country      *string `json:"country,omitempty" db:"country"`
	VATID        *string `json:"vat_id,omitempty" db:"vat_id"`
	TaxID        *string `json:"tax_id,omitempty" db:"tax_id"`
	IsBusiness   *bool   `json:"is_business,omitempty" db:"is_business"`

	UpdatedAt time.Time `json:"updated_at" db:"updated_at"` // optimistic concurrency
}

// PartyFields are the address / tax fields a contact shares with an invoice
// Party (docs/INVOICING.md §2) so invoice customers can be pre-filled.
type PartyFields struct {
	AddressLine1 string `json:"address_line1" db:"address_line1"`
	AddressLine2 string `json:"address_line2" db:"address_line2"`
	City         string `json:"city" db:"city"`
	Region       string `json:"region" db:"region"` // state / province
	PostalCode   string `json:"postal_code" db:"postal_code"`
	Country      string `json:"country" db:"country"` // ISO 3166-1 alpha-2, uppercase
	VATID        string `json:"vat_id" db:"vat_id"`   // EU VAT ID incl. country prefix
	TaxID        string `json:"tax_id" db:"tax_id"`   // other tax id; never an SSN
	IsBusiness   bool   `json:"is_business" db:"is_business"`
}

// Sentinel errors
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("invalid contact")
)
