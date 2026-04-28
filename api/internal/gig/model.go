package gig

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/tracklist"
	"github.com/shopspring/decimal"
)

// Gig status enum: inquiry → confirmed → advanced → played → cancelled
// cancelled is terminal; all other transitions allowed in both directions
type GigStatus string

const (
	GigStatusInquiry   GigStatus = "inquiry"
	GigStatusConfirmed GigStatus = "confirmed"
	GigStatusAdvanced  GigStatus = "advanced"
	GigStatusPlayed    GigStatus = "played"
	GigStatusCancelled GigStatus = "cancelled"
)

// Payment status tracked independently from gig status
type PaymentStatus string

const (
	PaymentStatusUnpaid       PaymentStatus = "unpaid"
	PaymentStatusDepositPaid  PaymentStatus = "deposit_paid"
	PaymentStatusPaid         PaymentStatus = "paid"
	PaymentStatusOverdue      PaymentStatus = "overdue"
	PaymentStatusWaived       PaymentStatus = "waived"
)

// Gig represents a DJ gig/booking
type Gig struct {
	ID                   uuid.UUID    `db:"id"`
	Date                 time.Time    `db:"date"`
	Venue                string       `db:"venue"`
	City                 string       `db:"city"`
	Country              string       `db:"country"`
	EventName            string       `db:"event_name"`
	PromoterName         string       `db:"promoter_name"`
	PromoterEmail        string       `db:"promoter_email"`
	PromoterPhone        string       `db:"promoter_phone"`
	FeeAmount            decimal.Decimal `db:"fee_amount"`
	FeeCurrency          string       `db:"fee_currency"` // ISO 4217
	SetLengthMinutes     int          `db:"set_length_minutes"`
	Notes                string       `db:"notes"`
	Status               GigStatus    `db:"status"`
	PaymentStatus        PaymentStatus `db:"payment_status"`
	GigReaderVenueID     *uuid.UUID   `db:"gig_reader_venue_id"`   // FK to reusable venue record
	GigReaderContactID   *uuid.UUID   `db:"gig_reader_contact_id"` // FK to reusable contact record
	CreatedAt            time.Time    `db:"created_at"`
	UpdatedAt            time.Time    `db:"updated_at"`
	DeletedAt            *time.Time   `db:"deleted_at"` // soft delete
}

// GigCreate contains fields required to create a new gig
type GigCreate struct {
	Date               time.Time    `json:"date" db:"date"`
	Venue              string       `json:"venue" db:"venue"`
	City               string       `json:"city" db:"city"`
	Country            string       `json:"country" db:"country"`
	EventName          string       `json:"event_name" db:"event_name"`
	PromoterName       string       `json:"promoter_name" db:"promoter_name"`
	PromoterEmail      string       `json:"promoter_email" db:"promoter_email"`
	PromoterPhone      string       `json:"promoter_phone" db:"promoter_phone"`
	FeeAmount          decimal.Decimal `json:"fee_amount" db:"fee_amount"`
	FeeCurrency        string       `json:"fee_currency" db:"fee_currency"`
	SetLengthMinutes   int          `json:"set_length_minutes" db:"set_length_minutes"`
	Notes              string       `json:"notes" db:"notes"`
	// Status defaults to inquiry; PaymentStatus defaults to unpaid
}

// GigUpdate contains optional fields for updating a gig (all pointers for partial update)
type GigUpdate struct {
	Date               *time.Time    `json:"date,omitempty" db:"date"`
	Venue              *string       `json:"venue,omitempty" db:"venue"`
	City               *string       `json:"city,omitempty" db:"city"`
	Country            *string       `json:"country,omitempty" db:"country"`
	EventName          *string       `json:"event_name,omitempty" db:"event_name"`
	PromoterName       *string       `json:"promoter_name,omitempty" db:"promoter_name"`
	PromoterEmail      *string       `json:"promoter_email,omitempty" db:"promoter_email"`
	PromoterPhone      *string       `json:"promoter_phone,omitempty" db:"promoter_phone"`
	FeeAmount          *decimal.Decimal `json:"fee_amount,omitempty" db:"fee_amount"`
	FeeCurrency        *string       `json:"fee_currency,omitempty" db:"fee_currency"`
	SetLengthMinutes   *int          `json:"set_length_minutes,omitempty" db:"set_length_minutes"`
	Notes              *string       `json:"notes,omitempty" db:"notes"`
	Status             *GigStatus    `json:"status,omitempty" db:"status"`
	PaymentStatus      *PaymentStatus `json:"payment_status,omitempty" db:"payment_status"`
	GigReaderVenueID   *uuid.UUID    `json:"gig_reader_venue_id,omitempty" db:"gig_reader_venue_id"`
	GigReaderContactID *uuid.UUID    `json:"gig_reader_contact_id,omitempty" db:"gig_reader_contact_id"`
	UpdatedAt          time.Time     `json:"updated_at" db:"updated_at"` // optimistic concurrency
}

// GigFilter contains optional filters for listing gigs
type GigFilter struct {
	Status       *GigStatus    // filter by gig status
	VenueID      *uuid.UUID    // filter by linked venue
	City         *string       // ILIKE search
	FeeMin       *decimal.Decimal // minimum fee
	FeeMax       *decimal.Decimal // maximum fee
	From         *time.Time    // date range start
	To           *time.Time    // date range end
}

// GigReader is the stable interface consumed by downstream modules:
// - Finance Tracker (Phase 5): auto-creates income when payment_status → paid
// - Tour Manager (Phase 7): groups gigs into tours
// - EPK-10 Import: "Import from Gigs" button
type GigReader interface {
	GetGig(ctx context.Context, id uuid.UUID) (*Gig, error)
	ListGigs(ctx context.Context, f GigFilter) ([]*Gig, error)
	Tracklists(ctx context.Context, gigID uuid.UUID) ([]*tracklist.Tracklist, error)
}

// Sentinel errors
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrForbidden  = errors.New("forbidden")
)
