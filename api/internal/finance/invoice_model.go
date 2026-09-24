package finance

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// InvoiceStatus enumerates the lifecycle states. The only valid forward
// transitions are: draft → issued; issued → paid | cancelled | corrected.
// "corrected" means a subsequent correction invoice supersedes this one.
type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusIssued    InvoiceStatus = "issued"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusCancelled InvoiceStatus = "cancelled"
	InvoiceStatusCorrected InvoiceStatus = "corrected"
)

var validInvoiceStatuses = map[InvoiceStatus]struct{}{
	InvoiceStatusDraft:     {},
	InvoiceStatusIssued:    {},
	InvoiceStatusPaid:      {},
	InvoiceStatusCancelled: {},
	InvoiceStatusCorrected: {},
}

// IsValid reports whether s is a known status.
func (s InvoiceStatus) IsValid() bool {
	_, ok := validInvoiceStatuses[s]
	return ok
}

// Invoice is the core finance document. All monetary fields are minor
// units (cents/øre etc) stored as int64 to avoid floating-point drift.
// The JSON tags use the exact wire names the frontend expects
// (snake_case for DB, camelCase for JSON).
type Invoice struct {
	ID              uuid.UUID       `json:"id"                   db:"id"`
	GigID           uuid.UUID       `json:"gig_id"               db:"gig_id"`
	BillingProfile  json.RawMessage `json:"billing_profile"      db:"billing_profile"`
	InvoiceNumber   string          `json:"invoice_number"       db:"invoice_number"`
	NumberPrefix    string          `json:"number_prefix"        db:"number_prefix"`
	NumberSeq       int64           `json:"number_seq"           db:"number_seq"`
	Currency        string          `json:"currency"             db:"currency"`
	SubtotalMinor   int64           `json:"subtotal_minor"       db:"subtotal_minor"`
	TaxRateBps      int64           `json:"tax_rate_bps"         db:"tax_rate_bps"`
	TaxMinor        int64           `json:"tax_minor"            db:"tax_minor"`
	TotalMinor      int64           `json:"total_minor"          db:"total_minor"`
	Status          InvoiceStatus   `json:"status"               db:"status"`
	IssuedAt        *time.Time      `json:"issued_at"            db:"issued_at"`
	DueAt           *time.Time      `json:"due_at"               db:"due_at"`
	PaidAt          *time.Time      `json:"paid_at"              db:"paid_at"`
	PaymentRef      string          `json:"payment_ref"          db:"payment_ref"`
	InternalNotes   string          `json:"internal_notes"       db:"internal_notes"`
	UpdatedAt       time.Time       `json:"updated_at"           db:"updated_at"`
	CreatedAt       time.Time       `json:"created_at"           db:"created_at"`
}

// InvoiceLine is a single line on an invoice.
type InvoiceLine struct {
	ID               uuid.UUID `json:"id"                   db:"id"`
	InvoiceID        uuid.UUID `json:"invoice_id"           db:"invoice_id"`
	SortOrder        int       `json:"sort_order"           db:"sort_order"`
	Description      string    `json:"description"          db:"description"`
	Quantity         int       `json:"quantity"             db:"quantity"`
	UnitMinor        int64     `json:"unit_minor"           db:"unit_minor"`
	TaxBps           int64     `json:"tax_bps"              db:"tax_bps"`
	LineTotalMinor   int64     `json:"line_total_minor"     db:"line_total_minor"`
	CreatedAt        time.Time `json:"created_at"           db:"created_at"`
}

// Subtotal returns the sum of line totals as a decimal.Decimal for display.
func (l *InvoiceLine) Subtotal() decimal.Decimal {
	return decimal.NewFromInt(l.LineTotalMinor).Div(decimal.NewFromInt(100))
}

// UnitPrice returns the unit price as decimal.Decimal for display.
func (l *InvoiceLine) UnitPrice() decimal.Decimal {
	return decimal.NewFromInt(l.UnitMinor).Div(decimal.NewFromInt(100))
}

// InvoiceSentinel errors.
var (
	ErrInvoiceNotFound   = errors.New("invoice not found")
	ErrInvoiceConflict   = errors.New("invoice updated by another writer")
	ErrInvoiceBadState   = errors.New("invalid invoice state transition")
	ErrInvoiceValidation = errors.New("invalid invoice")
)

// InvoiceFieldError mirrors FieldError for invoice validation.
type InvoiceFieldError struct {
	Field   string
	Message string
}

func (e InvoiceFieldError) Error() string { return e.Field + ": " + e.Message }

// InvoiceValidationErrors is a multi-field validation failure.
type InvoiceValidationErrors []InvoiceFieldError

func (es InvoiceValidationErrors) Error() string {
	if len(es) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

func (es InvoiceValidationErrors) Is(target error) bool { return target == ErrInvoiceValidation }
func (es InvoiceValidationErrors) Unwrap() error        { return ErrInvoiceValidation }

// CreateInvoiceRequest is the body of POST /api/v1/finance/invoices.
// The service assembles the lines from the gig fee (and any extras) at
// creation time; the frontend only supplies the currency + optional due date
// + optional prefix override.
type CreateInvoiceRequest struct {
	GigID       uuid.UUID `json:"gig_id"`
	Currency    string    `json:"currency"`
	NumberPrefix string   `json:"number_prefix,omitempty"`
	DueAt       *time.Time `json:"due_at,omitempty"`
}

// UpdateInvoiceRequest is the body of PUT /api/v1/finance/invoices/{id}.
// Only draft invoices can be updated. UpdatedAt is the concurrency token.
type UpdateInvoiceRequest struct {
	NumberPrefix  string     `json:"number_prefix"`
	DueAt         *time.Time `json:"due_at"`
	InternalNotes string     `json:"internal_notes"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// IssueInvoiceRequest is the body of POST /api/v1/finance/invoices/{id}/issue.
// The billing profile snapshot is taken from the current finance profile
// at issuance time and stored in the invoice row. The request only carries
// the concurrency token.
type IssueInvoiceRequest struct {
	UpdatedAt time.Time `json:"updated_at"`
}

// PayInvoiceRequest is the body of POST /api/v1/finance/invoices/{id}/pay.
type PayInvoiceRequest struct {
	PaidAt     time.Time `json:"paid_at"`
	PaymentRef string    `json:"payment_ref"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CancelInvoiceRequest is the body of POST /api/v1/finance/invoices/{id}/cancel.
type CancelInvoiceRequest struct {
	UpdatedAt time.Time `json:"updated_at"`
}

// CorrectInvoiceRequest is the body of POST /api/v1/finance/invoices/{id}/correct.
// Creates a new draft invoice that mirrors the original but with updated
// fields; the original transitions to "corrected".
type CorrectInvoiceRequest struct {
	UpdatedAt time.Time `json:"updated_at"`
}

// NumberingConfig controls the invoice number format. Default prefix
// "INV" yields numbers like "INV-0001-EUR".
type NumberingConfig struct {
	Prefix string `json:"prefix"`
}

// DefaultNumberingConfig returns the default numbering config.
func DefaultNumberingConfig() NumberingConfig {
	return NumberingConfig{Prefix: "INV"}
}

// FormatInvoiceNumber formats the human-readable number. The sequence
// is zero-padded to 4 digits (configurable later if needed).
func FormatInvoiceNumber(prefix, currency string, seq int64) string {
	return fmt.Sprintf("%s-%04d-%s", strings.ToUpper(prefix), seq, strings.ToUpper(currency))
}

// ParseInvoiceNumber extracts prefix, seq, currency from a number.
// Returns (prefix, seq, currency, ok).
func ParseInvoiceNumber(num string) (string, int64, string, bool) {
	// Expected: PREFIX-0001-CUR
	parts := strings.Split(num, "-")
	if len(parts) != 3 {
		return "", 0, "", false
	}
	var seq int64
	fmt.Sscanf(parts[1], "%d", &seq)
	return parts[0], seq, parts[2], true
}

// Scan implements sql.Scanner for InvoiceStatus (db → Go).
func (s *InvoiceStatus) Scan(value interface{}) error {
	if value == nil {
		*s = InvoiceStatusDraft
		return nil
	}
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into InvoiceStatus", value)
	}
	*s = InvoiceStatus(str)
	return nil
}

// Value implements driver.Valuer for InvoiceStatus (Go → db).
func (s InvoiceStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// Money helpers for display.

// SubtotalEUR returns subtotal as decimal.Decimal with 2 decimal places.
func (i *Invoice) Subtotal() decimal.Decimal {
	return decimal.NewFromInt(i.SubtotalMinor).Div(decimal.NewFromInt(100))
}

// TaxAmount returns tax as decimal.Decimal.
func (i *Invoice) TaxAmount() decimal.Decimal {
	return decimal.NewFromInt(i.TaxMinor).Div(decimal.NewFromInt(100))
}

// Total returns total as decimal.Decimal.
func (i *Invoice) Total() decimal.Decimal {
	return decimal.NewFromInt(i.TotalMinor).Div(decimal.NewFromInt(100))
}

// SubtotalMajor returns subtotal in major units (e.g. EUR) as float64
// for display only — never use for math.
func (i *Invoice) SubtotalMajor() float64 {
	f, _ := i.Subtotal().Float64()
	return f
}

// TaxMajor returns tax in major units as float64 for display only.
func (i *Invoice) TaxMajor() float64 {
	f, _ := i.TaxAmount().Float64()
	return f
}

// TotalMajor returns total in major units as float64 for display only.
func (i *Invoice) TotalMajor() float64 {
	f, _ := i.Total().Float64()
	return f
}

// CurrencySummary holds per-currency dashboard aggregates.
type CurrencySummary struct {
	Currency     string          `json:"currency"`
	IssuedTotal  decimal.Decimal `json:"issued_total"`
	PaidTotal    decimal.Decimal `json:"paid_total"`
	IssuedCount  int64           `json:"issued_count"`
	PaidCount    int64           `json:"paid_count"`
}

// InvoiceFilter is the query filter for listing invoices.
type InvoiceFilter struct {
	GigID     *uuid.UUID
	Status    InvoiceStatus
	Currency  string
	From      time.Time
	To        time.Time
}