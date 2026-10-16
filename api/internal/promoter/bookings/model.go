// Package bookings is the P4 promoter→DJ outreach module (phase 04.10,
// SPEC §6): talent contacts (sealed PII — the DJs a promoter knows), open
// slots on events, city+genre matching, offer templates, and sending a
// booking-offer email through the shared platform/email service. The sent
// log seals the recipient and the rendered copy; suppressions store only the
// email blind index.
//
// Everything runs inside tenantdb.WithTenant (RLS FORCE isolates tenants);
// talent PII is sealed with the tenant DEK (envelope pattern, audience
// service). Routes sit behind the talent.* / booking.* authz actions
// (SPEC §6.2 v2.1).
package bookings

import (
	"time"

	"github.com/google/uuid"
)

// Talent contact sources: how the promoter knows the DJ (SPEC §9 provenance).
const (
	SourcePastGig  = "past_gig"
	SourceReferral = "referral"
	SourceManual   = "manual"
)

// Sources in display order.
var Sources = []string{SourcePastGig, SourceReferral, SourceManual}

// TalentContact is one row of the talent book, unsealed for display. Email
// is empty for name-only contacts; HasEmail tells the UI not to offer a send
// (OFFER-10).
type TalentContact struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	HasEmail  bool      `json:"has_email"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	Genres    []string  `json:"genres"`
	Source    string    `json:"source"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TalentContactInput is the body of POST /talent and PUT /talent/{id}
// (full replace of the editable fields, audience UpdateContact semantics).
type TalentContactInput struct {
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	Phone   string   `json:"phone"`
	City    string   `json:"city"`
	Country string   `json:"country"`
	Genres  []string `json:"genres"`
	Source  string   `json:"source"`
	Notes   string   `json:"notes"`
}

// TalentFilter narrows the talent list: city (case-insensitive), genre
// overlap, and Q (exact blind-index match on name or email).
type TalentFilter struct {
	City  string
	Genre string
	Q     string
}

// OpenSlot is one unfilled position on an event.
type OpenSlot struct {
	ID        uuid.UUID `json:"id"`
	EventID   uuid.UUID `json:"event_id"`
	Role      string    `json:"role"`
	Genres    []string  `json:"genres"`
	FeeMin    *float64  `json:"fee_min,omitempty"`
	FeeMax    *float64  `json:"fee_max,omitempty"`
	Currency  *string   `json:"currency,omitempty"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

// OpenSlotInput is the body of POST /events/{eventID}/slots.
type OpenSlotInput struct {
	Role     string   `json:"role"`
	Genres   []string `json:"genres"`
	FeeMin   *float64 `json:"fee_min"`
	FeeMax   *float64 `json:"fee_max"`
	Currency *string  `json:"currency"`
	Notes    string   `json:"notes"`
}

// Template is a customizable offer email template. The rendered copy is
// stored (sealed) on each request row at send time; the template is never
// mutated.
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

// OfferRequest is one row of the sent log, unsealed for display (the service
// unseals when rendering the log — SPEC §6.1 v2.1).
type OfferRequest struct {
	ID              uuid.UUID  `json:"id"`
	EventID         *uuid.UUID `json:"event_id,omitempty"`
	OpenSlotID      *uuid.UUID `json:"open_slot_id,omitempty"`
	TalentContactID *uuid.UUID `json:"talent_contact_id,omitempty"`
	TemplateID      *uuid.UUID `json:"template_id,omitempty"`
	EmailMessageID  *uuid.UUID `json:"email_message_id,omitempty"`
	ToEmail         string     `json:"to_email"`
	Subject         string     `json:"subject"`
	Body            string     `json:"body"`
	Status          string     `json:"status"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	// EmailStatus/EmailError come from the joined email_messages row.
	EmailStatus string `json:"email_status,omitempty"`
	EmailError  string `json:"email_error,omitempty"`
}

// SendOfferInput is the body of POST /offers (SPEC §6.2). Null subject/body
// renders from the template; a non-nil value overrides the template copy for
// this one send (the template itself is never mutated).
type SendOfferInput struct {
	TalentContactID uuid.UUID  `json:"talent_contact_id"`
	OpenSlotID      *uuid.UUID `json:"open_slot_id"`
	TemplateID      *uuid.UUID `json:"template_id"`
	Subject         *string    `json:"subject"`
	Body            *string    `json:"body"`
}

// Suppression is a do-not-contact entry. The email itself is NEVER stored —
// only its blind index (SPEC §9); the list endpoint therefore cannot show
// the address, by design.
type Suppression struct {
	ID        uuid.UUID `json:"id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// SuppressionCreate is the body of POST /suppressions.
type SuppressionCreate struct {
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// TalentMatch is one matched talent contact for an open slot, ranked
// (SPEC §8): genre-overlap count desc, then most recently added.
type TalentMatch struct {
	Contact       TalentContact `json:"contact"`
	OverlapGenres []string      `json:"overlap_genres"`
}
