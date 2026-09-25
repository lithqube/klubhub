// Package finance owns the DJ's billing identity and the invoice lifecycle
// (drafts, atomic issuance, payments, deposits). Agreements live in
// internal/gig; documents and email outbox live in internal/document and
// internal/mailer respectively.
//
// Wave 1 of Phase 5: structured billing identity with strict validation,
// stored separately from user_settings (public artist biography).
package finance

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EntityKind enumerates the legal-entity classes the profile accepts. New
// kinds are an additive migration; existing rows keep their current value.
type EntityKind string

const (
	EntityKindIndividual  EntityKind = "individual"
	EntityKindSoleTrader  EntityKind = "sole_trader"
	EntityKindPartnership EntityKind = "partnership"
	EntityKindLLC         EntityKind = "llc"
	EntityKindCorp        EntityKind = "corp"
	EntityKindOther       EntityKind = "other"
)

// ValidEntityKinds is the closed set; reject anything else in Validate.
var ValidEntityKinds = map[EntityKind]struct{}{
	EntityKindIndividual:  {},
	EntityKindSoleTrader:  {},
	EntityKindPartnership: {},
	EntityKindLLC:         {},
	EntityKindCorp:        {},
	EntityKindOther:       {},
}

// TaxIDKind enumerates the tax-ID schemes the profile accepts. Empty means
// the DJ does not have / did not supply one (the UI must still surface a
// confirmation before issuing).
type TaxIDKind string

const (
	TaxIDKindEmpty TaxIDKind = ""
	TaxIDKindVAT   TaxIDKind = "vat"
	TaxIDKindEIN   TaxIDKind = "ein"
	TaxIDKindGST   TaxIDKind = "gst"
	TaxIDKindABN   TaxIDKind = "abn"
	TaxIDKindOther TaxIDKind = "other"
)

// ValidTaxIDKinds is the closed set; reject anything else in Validate.
var ValidTaxIDKinds = map[TaxIDKind]struct{}{
	TaxIDKindEmpty: {},
	TaxIDKindVAT:   {},
	TaxIDKindEIN:   {},
	TaxIDKindGST:   {},
	TaxIDKindABN:   {},
	TaxIDKindOther: {},
}

// IsValid reports whether k is in ValidEntityKinds.
func (k EntityKind) IsValid() bool {
	_, ok := ValidEntityKinds[k]
	return ok
}

// IsValid reports whether k is in ValidTaxIDKinds.
func (k TaxIDKind) IsValid() bool {
	_, ok := ValidTaxIDKinds[k]
	return ok
}

// BillingProfile is the DJ's structured billing identity. It is distinct
// from user_settings (artist biography exposed in the EPK) so that
// private fields here cannot leak into the public EPK export.
type BillingProfile struct {
	ID                  uuid.UUID  `json:"id"                   db:"id"`
	LegalName           string     `json:"legal_name"           db:"legal_name"`
	TradingName         string     `json:"trading_name"         db:"trading_name"`
	EntityKind          EntityKind `json:"entity_kind"          db:"entity_kind"`
	TaxID               string     `json:"tax_id"               db:"tax_id"`
	TaxIDKind           TaxIDKind  `json:"tax_id_kind"          db:"tax_id_kind"`
	ContactEmail        string     `json:"contact_email"        db:"contact_email"`
	ContactPhone        string     `json:"contact_phone"        db:"contact_phone"`
	AddressLine1        string     `json:"address_line1"        db:"address_line1"`
	AddressLine2        string     `json:"address_line2"        db:"address_line2"`
	AddressCity         string     `json:"address_city"         db:"address_city"`
	AddressRegion       string     `json:"address_region"       db:"address_region"`
	AddressPostal       string     `json:"address_postal"      db:"address_postal"`
	AddressCountry      string     `json:"address_country"      db:"address_country"`
	Jurisdiction        string     `json:"jurisdiction"         db:"jurisdiction"`
	PaymentInstructions string     `json:"payment_instructions" db:"payment_instructions"`
	DefaultCurrency     string     `json:"default_currency"     db:"default_currency"`
	UpdatedAt           time.Time  `json:"updated_at"           db:"updated_at"`
	CreatedAt           time.Time  `json:"created_at"           db:"created_at"`
}

// UpdateBillingProfileRequest is the body of PUT /api/v1/finance/billing-profile.
// UpdatedAt is the optimistic concurrency token, identical pattern to
// settings.UpdateSettingsRequest. Missing UpdatedAt yields ErrConflict
// from the service layer.
type UpdateBillingProfileRequest struct {
	LegalName           string     `json:"legal_name"`
	TradingName         string     `json:"trading_name"`
	EntityKind          EntityKind `json:"entity_kind"`
	TaxID               string     `json:"tax_id"`
	TaxIDKind           TaxIDKind  `json:"tax_id_kind"`
	ContactEmail        string     `json:"contact_email"`
	ContactPhone        string     `json:"contact_phone"`
	AddressLine1        string     `json:"address_line1"`
	AddressLine2        string     `json:"address_line2"`
	AddressCity         string     `json:"address_city"`
	AddressRegion       string     `json:"address_region"`
	AddressPostal       string     `json:"address_postal"`
	AddressCountry      string     `json:"address_country"`
	Jurisdiction        string     `json:"jurisdiction"`
	PaymentInstructions string     `json:"payment_instructions"`
	DefaultCurrency     string     `json:"default_currency"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// Sentinel errors for service / handler layer.
var (
	ErrNotFound    = errors.New("billing profile not found")
	ErrConflict    = errors.New("billing profile updated by another writer")
	ErrValidation  = errors.New("invalid billing profile")
	ErrUnissuedDoc = errors.New("documents cannot be issued until the profile is complete")
)

// FieldError reports a single field violation. Multiple FieldErrors may be
// returned wrapped in ErrValidation.
type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }

// ValidationErrors is a multi-field validation failure that satisfies
// errors.Is(err, ErrValidation).
type ValidationErrors []FieldError

func (es ValidationErrors) Error() string {
	if len(es) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Is reports whether the chain contains ErrValidation.
func (es ValidationErrors) Is(target error) bool { return target == ErrValidation }

// Unwrap returns the sentinel so errors.Is finds it.
func (es ValidationErrors) Unwrap() error { return ErrValidation }
