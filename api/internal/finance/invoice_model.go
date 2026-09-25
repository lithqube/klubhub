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

	"github.com/klubhub/dj/api/internal/finance/tax"
)

// InvoiceKind distinguishes invoices from credit notes. Amounts are stored
// positive on both; the kind carries the sign.
type InvoiceKind string

const (
	InvoiceKindInvoice    InvoiceKind = "invoice"
	InvoiceKindCreditNote InvoiceKind = "credit_note"
)

// IsValid reports whether k is a known kind.
func (k InvoiceKind) IsValid() bool {
	return k == InvoiceKindInvoice || k == InvoiceKindCreditNote
}

// CreditNotePrefix is the numbering series prefix of credit notes; invoices
// may not use it.
const CreditNotePrefix = "CN"

// InvoiceStatus enumerates the lifecycle states (docs/INVOICING.md §1):
// draft → issued → paid; draft → cancelled (no number consumed);
// issued|paid → credited (credit note) or → corrected (credit note + new
// draft). Issued documents are never edited or deleted.
type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusIssued    InvoiceStatus = "issued"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusCancelled InvoiceStatus = "cancelled"
	InvoiceStatusCredited  InvoiceStatus = "credited"
	InvoiceStatusCorrected InvoiceStatus = "corrected"
)

var validInvoiceStatuses = map[InvoiceStatus]struct{}{
	InvoiceStatusDraft:     {},
	InvoiceStatusIssued:    {},
	InvoiceStatusPaid:      {},
	InvoiceStatusCancelled: {},
	InvoiceStatusCredited:  {},
	InvoiceStatusCorrected: {},
}

// IsValid reports whether s is a known status.
func (s InvoiceStatus) IsValid() bool {
	_, ok := validInvoiceStatuses[s]
	return ok
}

// Party is a customer (or, later, supplier) identity snapshot as printed on
// an invoice. It is stored as JSONB on the invoice so later edits to the
// source contact never change an issued document.
type Party struct {
	ContactID    *uuid.UUID `json:"contact_id"`
	LegalName    string     `json:"legal_name"`
	Company      string     `json:"company"`
	Email        string     `json:"email"`
	AddressLine1 string     `json:"address_line1"`
	AddressLine2 string     `json:"address_line2"`
	City         string     `json:"city"`
	Region       string     `json:"region"`
	PostalCode   string     `json:"postal_code"`
	Country      string     `json:"country"` // ISO 3166-1 alpha-2, uppercase
	VATID        string     `json:"vat_id"`
	TaxID        string     `json:"tax_id"` // never an SSN
	IsBusiness   bool       `json:"is_business"`
}

// TaxCustomer is the tax-relevant view of the party.
func (p Party) TaxCustomer() tax.Customer {
	return tax.Customer{
		LegalName: p.LegalName, Company: p.Company, AddressLine1: p.AddressLine1,
		City: p.City, Country: p.Country, VATID: p.VATID, IsBusiness: p.IsBusiness,
	}
}

// Invoice is the core finance document. All monetary fields are minor
// units (cents etc.) stored as int64 to avoid floating-point drift. JSON
// names are the wire contract of docs/INVOICING.md §2.
type Invoice struct {
	ID                  uuid.UUID          `json:"id"`
	Kind                InvoiceKind        `json:"kind"`
	GigID               uuid.UUID          `json:"gig_id"`
	CreditsInvoiceID    *uuid.UUID         `json:"credits_invoice_id"`
	ReplacedByInvoiceID *uuid.UUID         `json:"replaced_by_invoice_id"`
	InvoiceNumber       *string            `json:"invoice_number"` // nil while draft
	NumberPrefix        string             `json:"number_prefix"`
	NumberSeq           *int64             `json:"number_seq"`
	Currency            string             `json:"currency"`
	Status              InvoiceStatus      `json:"status"`
	SupplyDate          *string            `json:"supply_date"` // YYYY-MM-DD
	IssuedAt            *time.Time         `json:"issued_at"`
	DueAt               *time.Time         `json:"due_at"`
	PaidAt              *time.Time         `json:"paid_at"`
	PaymentRef          string             `json:"payment_ref"`
	InternalNotes       string             `json:"internal_notes"`
	Customer            Party              `json:"customer"`
	BillingProfile      json.RawMessage    `json:"billing_profile"` // supplier snapshot, set at issue
	VATTreatment        tax.Treatment      `json:"vat_treatment"`
	TaxRateBps          int64              `json:"tax_rate_bps"`
	TaxNote             string             `json:"tax_note"`
	SubtotalMinor       int64              `json:"subtotal_minor"`
	TaxMinor            int64              `json:"tax_minor"`
	TotalMinor          int64              `json:"total_minor"`
	WithholdingRateBps  int64              `json:"withholding_rate_bps"`
	WithholdingMinor    int64              `json:"withholding_minor"`
	NetPayableMinor     int64              `json:"net_payable_minor"`
	TaxBreakdown        []tax.BreakdownRow `json:"tax_breakdown"`
	// Computed on read from payments; 0 for drafts and credit notes.
	ReceivedMinor    int64     `json:"received_minor"`
	PendingMinor     int64     `json:"pending_minor"`
	OutstandingMinor int64     `json:"outstanding_minor"`
	UpdatedAt        time.Time `json:"updated_at"` // optimistic-concurrency token
	CreatedAt        time.Time `json:"created_at"`
}

// Number returns the invoice number, or "" while unnumbered.
func (i *Invoice) Number() string {
	if i.InvoiceNumber == nil {
		return ""
	}
	return *i.InvoiceNumber
}

// finalizeBalances derives the computed payment fields after a read.
// Outstanding is net payable − received − pending, floored at 0, and only
// for issued invoices: drafts and credit notes take no payments, and
// paid / credited / corrected / cancelled documents owe nothing.
func (i *Invoice) finalizeBalances() {
	if i.TaxBreakdown == nil {
		i.TaxBreakdown = []tax.BreakdownRow{}
	}
	if i.Kind == InvoiceKindCreditNote || i.Status == InvoiceStatusDraft {
		i.ReceivedMinor, i.PendingMinor, i.OutstandingMinor = 0, 0, 0
		return
	}
	i.OutstandingMinor = 0
	if i.Status == InvoiceStatusIssued {
		if o := i.NetPayableMinor - i.ReceivedMinor - i.PendingMinor; o > 0 {
			i.OutstandingMinor = o
		}
	}
}

// InvoiceLine is a single line on an invoice.
type InvoiceLine struct {
	ID             uuid.UUID `json:"id"                   db:"id"`
	InvoiceID      uuid.UUID `json:"invoice_id"           db:"invoice_id"`
	SortOrder      int       `json:"sort_order"           db:"sort_order"`
	Description    string    `json:"description"          db:"description"`
	Quantity       int       `json:"quantity"             db:"quantity"`
	UnitMinor      int64     `json:"unit_minor"           db:"unit_minor"`
	TaxBps         int64     `json:"tax_bps"              db:"tax_bps"`
	LineTotalMinor int64     `json:"line_total_minor"     db:"line_total_minor"`
	CreatedAt      time.Time `json:"created_at"           db:"created_at"`
}

// Subtotal returns the line total as a decimal.Decimal for display.
func (l *InvoiceLine) Subtotal() decimal.Decimal {
	return decimal.NewFromInt(l.LineTotalMinor).Div(decimal.NewFromInt(100))
}

// UnitPrice returns the unit price as decimal.Decimal for display.
func (l *InvoiceLine) UnitPrice() decimal.Decimal {
	return decimal.NewFromInt(l.UnitMinor).Div(decimal.NewFromInt(100))
}

// Invoice sentinel errors. Conflict means a stale updated_at token;
// BadState means the action is not allowed in the invoice's status/kind.
var (
	ErrInvoiceNotFound    = errors.New("invoice not found")
	ErrInvoiceConflict    = errors.New("invoice updated by another writer")
	ErrInvoiceBadState    = errors.New("invalid invoice state transition")
	ErrInvoiceValidation  = errors.New("invalid invoice")
	ErrInvoiceNotIssuable = errors.New("invoice is not ready to be issued")
)

// NotIssuableError carries the issue-check problems that blocked an issue.
type NotIssuableError struct {
	Problems []tax.Problem
}

func (e *NotIssuableError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Field + ": " + p.Message
	}
	return "invoice is not ready to be issued: " + strings.Join(parts, "; ")
}

// Is makes errors.Is(err, ErrInvoiceNotIssuable) match.
func (e *NotIssuableError) Is(target error) bool { return target == ErrInvoiceNotIssuable }

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

// FlexTime accepts either an RFC 3339 timestamp or a YYYY-MM-DD date
// (midnight UTC) in request bodies.
type FlexTime struct{ time.Time }

// UnmarshalJSON implements json.Unmarshaler.
func (t *FlexTime) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed
			return nil
		}
	}
	return fmt.Errorf("invalid time %q: want RFC 3339 or YYYY-MM-DD", s)
}

// MarshalJSON implements json.Marshaler.
func (t FlexTime) MarshalJSON() ([]byte, error) { return t.Time.MarshalJSON() }

// ptr converts an optional FlexTime into an optional time.Time.
func (t *FlexTime) ptr() *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	v := t.Time
	return &v
}

// CreateInvoiceRequest is the body of POST /api/v1/finance/invoices. Omitted
// fields are pre-filled: customer from the gig contact, supply date from
// the gig date, treatment/rate/note from tax.Suggest. Currency is the gig
// fee currency; if sent it must match.
type CreateInvoiceRequest struct {
	GigID              uuid.UUID      `json:"gig_id"`
	Currency           string         `json:"currency,omitempty"`
	Customer           *Party         `json:"customer,omitempty"`
	VATTreatment       *tax.Treatment `json:"vat_treatment,omitempty"`
	TaxRateBps         *int64         `json:"tax_rate_bps,omitempty"`
	WithholdingRateBps *int64         `json:"withholding_rate_bps,omitempty"`
	SupplyDate         *string        `json:"supply_date,omitempty"`
	DueAt              *FlexTime      `json:"due_at,omitempty"`
	NumberPrefix       string         `json:"number_prefix,omitempty"`
}

// UpdateInvoiceRequest is the body of PUT /api/v1/finance/invoices/{id}: a
// full replacement of the editable draft fields. Totals are recomputed.
type UpdateInvoiceRequest struct {
	Customer           Party         `json:"customer"`
	VATTreatment       tax.Treatment `json:"vat_treatment"`
	TaxRateBps         int64         `json:"tax_rate_bps"`
	TaxNote            string        `json:"tax_note"`
	WithholdingRateBps int64         `json:"withholding_rate_bps"`
	SupplyDate         *string       `json:"supply_date"`
	DueAt              *FlexTime     `json:"due_at"`
	NumberPrefix       string        `json:"number_prefix"`
	InternalNotes      string        `json:"internal_notes"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

// IssueInvoiceRequest is the body of POST /invoices/{id}/issue.
type IssueInvoiceRequest struct {
	UpdatedAt time.Time `json:"updated_at"`
}

// PayInvoiceRequest is the body of POST /invoices/{id}/pay.
type PayInvoiceRequest struct {
	PaidAt     time.Time `json:"paid_at"`
	PaymentRef string    `json:"payment_ref"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CancelInvoiceRequest is the body of POST /invoices/{id}/cancel.
type CancelInvoiceRequest struct {
	UpdatedAt time.Time `json:"updated_at"`
}

// CreditNoteRequest is the body of POST /invoices/{id}/credit-note and
// /correct. Reason is kept on the credit note's internal notes.
type CreditNoteRequest struct {
	Reason    string    `json:"reason"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CorrectInvoiceRequest is the body of POST /invoices/{id}/correct.
type CorrectInvoiceRequest = CreditNoteRequest

// CreditNoteResult is returned by the credit-note and correct actions.
type CreditNoteResult struct {
	CreditNote  *Invoice `json:"credit_note"`
	Original    *Invoice `json:"original"`
	Replacement *Invoice `json:"replacement,omitempty"`
}

// IssueCheck is the result of GET /invoices/{id}/issue-check.
type IssueCheck struct {
	Ready    bool          `json:"ready"`
	Problems []tax.Problem `json:"problems"`
}

// DraftLine is a line to insert on a new draft.
type DraftLine struct {
	Description string
	Quantity    int
	UnitMinor   int64
}

// DraftInput is a fully resolved, validated draft handed to the repository.
type DraftInput struct {
	GigID              uuid.UUID
	Currency           string
	NumberPrefix       string
	Customer           Party
	VATTreatment       tax.Treatment
	TaxRateBps         int64
	TaxNote            string
	WithholdingRateBps int64
	SupplyDate         *string
	DueAt              *time.Time
	Lines              []DraftLine
	Totals             tax.Totals
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
// is zero-padded to 4 digits.
func FormatInvoiceNumber(prefix, currency string, seq int64) string {
	return fmt.Sprintf("%s-%04d-%s", strings.ToUpper(prefix), seq, strings.ToUpper(currency))
}

// ParseInvoiceNumber extracts prefix, seq, currency from a number.
// Returns (prefix, seq, currency, ok).
func ParseInvoiceNumber(num string) (string, int64, string, bool) {
	parts := strings.Split(num, "-")
	if len(parts) != 3 {
		return "", 0, "", false
	}
	var seq int64
	if _, err := fmt.Sscanf(parts[1], "%d", &seq); err != nil {
		return "", 0, "", false
	}
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

// CurrencySummary holds per-currency dashboard aggregates over invoices
// (credit notes excluded), all money in minor units.
//   - outstanding_minor: Σ outstanding of issued invoices
//   - paid_minor: Σ net payable of paid invoices + Σ received on issued ones
type CurrencySummary struct {
	Currency         string `json:"currency"`
	DraftCount       int64  `json:"draft_count"`
	IssuedCount      int64  `json:"issued_count"`
	PaidCount        int64  `json:"paid_count"`
	OutstandingMinor int64  `json:"outstanding_minor"`
	PaidMinor        int64  `json:"paid_minor"`
}

// InvoiceFilter is the query filter for listing invoices.
type InvoiceFilter struct {
	GigID    *uuid.UUID
	Status   InvoiceStatus
	Kind     InvoiceKind
	Currency string
	From     time.Time
	To       time.Time
}
