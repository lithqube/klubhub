package outreach

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Date is a calendar date (YYYY-MM-DD) for availability slots. It wraps
// time.Time so pgx can scan the DATE column directly, but serialises as a
// plain calendar date in JSON.
type Date time.Time

// DateFormat is the wire/JSON representation of a slot date.
const DateFormat = "2006-01-02"

// MarshalJSON renders the date as YYYY-MM-DD.
func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(d).UTC().Format(DateFormat))
}

// UnmarshalJSON parses a YYYY-MM-DD calendar date.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(DateFormat, s)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

// Slot is one availability slot: a date + city the DJ can be booked in.
type Slot struct {
	ID        uuid.UUID `json:"id"`
	Date      Date      `json:"date"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	Genres    []string  `json:"genres"`
	FeeMin    *float64  `json:"fee_min,omitempty"`
	FeeMax    *float64  `json:"fee_max,omitempty"`
	Currency  *string   `json:"currency,omitempty"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

// SlotCreate is the body of POST /availability/slots.
type SlotCreate struct {
	Date     Date     `json:"date"`
	City     string   `json:"city"`
	Country  string   `json:"country"`
	Genres   []string `json:"genres"`
	FeeMin   *float64 `json:"fee_min"`
	FeeMax   *float64 `json:"fee_max"`
	Currency *string  `json:"currency"`
	Notes    string   `json:"notes"`
}

// Template is a customizable outreach email template. The rendered copy is
// stored on each request row at send time; the template is never mutated.
type Template struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Direction string    `json:"direction"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	IsDefault bool      `json:"is_default"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateCreate is the body of POST /templates.
type TemplateCreate struct {
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	IsDefault bool   `json:"is_default"`
}

// TemplateUpdate is the body of PUT /templates/{id}. Nil pointer = no
// update intent. Version is the optimistic-concurrency token.
type TemplateUpdate struct {
	Name      *string `json:"name"`
	Subject   *string `json:"subject"`
	Body      *string `json:"body"`
	IsDefault *bool   `json:"is_default"`
	Version   int     `json:"version"`
}

// Request is one row of the sent log: a booking-request email, linked to
// the email outbox row for delivery status.
type Request struct {
	ID                 uuid.UUID  `json:"id"`
	ContactID          uuid.UUID  `json:"contact_id"`
	AvailabilitySlotID *uuid.UUID `json:"availability_slot_id,omitempty"`
	TemplateID         *uuid.UUID `json:"template_id,omitempty"`
	EPKExportID        *uuid.UUID `json:"epk_export_id,omitempty"`
	EmailMessageID     *uuid.UUID `json:"email_message_id,omitempty"`
	ToEmail            string     `json:"to_email"`
	Subject            string     `json:"subject"`
	Body               string     `json:"body"`
	Status             string     `json:"status"`
	SentAt             *time.Time `json:"sent_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	// EmailStatus/EmailError come from the joined email_messages row.
	EmailStatus string `json:"email_status,omitempty"`
	EmailError  string `json:"email_error,omitempty"`
}

// SendRequestInput is the body of POST /requests. Null subject/body renders
// from the template; a non-nil value overrides the template copy for this
// one send (the template itself is never mutated).
type SendRequestInput struct {
	ContactID          uuid.UUID  `json:"contact_id"`
	AvailabilitySlotID *uuid.UUID `json:"availability_slot_id"`
	TemplateID         *uuid.UUID `json:"template_id"`
	EPKExportID        *uuid.UUID `json:"epk_export_id"`
	Subject            *string    `json:"subject"`
	Body               *string    `json:"body"`
}

// Suppression is a do-not-contact entry. The DJ side stores the normalized
// (lower, trimmed) email in plaintext — single-user OSS, consistent with
// contacts.email.
type Suppression struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// SuppressionCreate is the body of POST /suppressions.
type SuppressionCreate struct {
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// MatchedContact is a promoter contact matched to a slot. Contacts without
// an email are still returned, with HasEmail=false — they are matchable but
// not sendable (OUTR-05 v2.1).
type MatchedContact struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Company       string    `json:"company,omitempty"`
	Email         string    `json:"email"`
	HasEmail      bool      `json:"has_email"`
	City          string    `json:"city"`
	PastGigInCity bool      `json:"past_gig_in_city"`
}

// MatchedVenue is a venue in the slot's city, surfaced as context.
type MatchedVenue struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	City string    `json:"city"`
}

// SlotMatch is the match result for one availability slot.
type SlotMatch struct {
	Slot     Slot              `json:"slot"`
	Contacts []*MatchedContact `json:"contacts"`
	Venues   []*MatchedVenue   `json:"venues"`
}

// EPKExportRef is the subset of epk_exports the attach path needs.
type EPKExportRef struct {
	ID              uuid.UUID
	GarageObjectKey string
}

// ContactInfo is the DJ's own contact block (user_settings.contact_info
// JSON {name, email, phone}) — the source of {{dj_*}} variables and the
// ReplyTo of every outreach email (OUTR-06 v2.1).
type ContactInfo struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// Sentinel errors. The handler maps them to HTTP statuses:
// invalid→422, no contact email / no DJ reply-to / legacy EPK export→422,
// not found→404, suppressed / template version conflict→409,
// rate limited→429, email unavailable→503.
var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict")
	ErrValidation       = errors.New("invalid outreach request")
	ErrNoContactEmail   = errors.New("contact has no email address")
	ErrNoReplyTo        = errors.New("no DJ contact email configured")
	ErrSuppressed       = errors.New("address is suppressed")
	ErrRateLimited      = errors.New("daily outreach limit reached")
	ErrEmailUnavailable = errors.New("email is not available")
	ErrEPKLegacyExport  = errors.New("epk export has no garage object key")
)
