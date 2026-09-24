package finance

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PaymentKind enumerates the types of money movement.
type PaymentKind string

const (
	PaymentKindDeposit  PaymentKind = "deposit"
	PaymentKindPayment  PaymentKind = "payment"
	PaymentKindRefund   PaymentKind = "refund"
)

var validPaymentKinds = map[PaymentKind]struct{}{
	PaymentKindDeposit: {},
	PaymentKindPayment: {},
	PaymentKindRefund:  {},
}

func (k PaymentKind) IsValid() bool {
	_, ok := validPaymentKinds[k]
	return ok
}

// PaymentStatus tracks the lifecycle of a payment.
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

var validPaymentStatuses = map[PaymentStatus]struct{}{
	PaymentStatusPending:   {},
	PaymentStatusCompleted: {},
	PaymentStatusFailed:    {},
	PaymentStatusRefunded:  {},
}

func (s PaymentStatus) IsValid() bool {
	_, ok := validPaymentStatuses[s]
	return ok
}

// Payment is a recorded money movement against an invoice.
type Payment struct {
	ID           uuid.UUID     `json:"id"               db:"id"`
	InvoiceID    uuid.UUID     `json:"invoice_id"       db:"invoice_id"`
	Currency     string        `json:"currency"         db:"currency"`
	AmountMinor  int64         `json:"amount_minor"     db:"amount_minor"`
	Kind         PaymentKind   `json:"kind"             db:"kind"`
	Status       PaymentStatus `json:"status"           db:"status"`
	Method       string        `json:"method"           db:"method"`
	Reference    string        `json:"reference"        db:"reference"`
	ReceivedAt   *time.Time    `json:"received_at"      db:"received_at"`
	CreatedAt    time.Time     `json:"created_at"       db:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"       db:"updated_at"`
}

var (
	ErrPaymentNotFound = errors.New("payment not found")
	ErrPaymentConflict = errors.New("payment updated by another writer")
	ErrPaymentValidation = errors.New("invalid payment")
)

type PaymentFieldError struct {
	Field   string
	Message string
}

func (e PaymentFieldError) Error() string { return e.Field + ": " + e.Message }

type PaymentValidationErrors []PaymentFieldError

func (es PaymentValidationErrors) Error() string {
	if len(es) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

func (es PaymentValidationErrors) Is(target error) bool { return target == ErrPaymentValidation }
func (es PaymentValidationErrors) Unwrap() error        { return ErrPaymentValidation }

// CreatePaymentRequest is the body of POST /api/v1/finance/invoices/{id}/payments.
type CreatePaymentRequest struct {
	Currency    string      `json:"currency"`
	AmountMinor int64       `json:"amount_minor"`
	Kind        PaymentKind `json:"kind"`
	Method      string      `json:"method,omitempty"`
	Reference   string      `json:"reference,omitempty"`
	ReceivedAt  *time.Time  `json:"received_at,omitempty"`
}

// UpdatePaymentRequest is the body of PUT /api/v1/finance/payments/{id}.
type UpdatePaymentRequest struct {
	Status       PaymentStatus `json:"status"`
	Method       string        `json:"method"`
	Reference    string        `json:"reference"`
	ReceivedAt   *time.Time    `json:"received_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// PaymentRepositoryIface is the subset the service uses.
type PaymentRepositoryIface interface {
	Create(ctx context.Context, invoiceID uuid.UUID, req CreatePaymentRequest) (*Payment, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Payment, error)
	ListByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]*Payment, error)
	Update(ctx context.Context, id uuid.UUID, req UpdatePaymentRequest) (*Payment, error)
	SumByInvoice(ctx context.Context, invoiceID uuid.UUID) (int64, error) // sum of completed payments
}

// PaymentService coordinates payment writes.
type PaymentService struct {
	repo PaymentRepositoryIface
}

// NewPaymentService returns a PaymentService.
func NewPaymentService(repo PaymentRepositoryIface) *PaymentService {
	return &PaymentService{repo: repo}
}

// Create records a new payment (defaults to pending status).
func (s *PaymentService) Create(ctx context.Context, invoiceID uuid.UUID, req CreatePaymentRequest) (*Payment, error) {
	if errs := ValidatePaymentRequest(&req); len(errs) > 0 {
		return nil, errs
	}
	return s.repo.Create(ctx, invoiceID, req)
}

// GetByID returns a payment by ID.
func (s *PaymentService) GetByID(ctx context.Context, id uuid.UUID) (*Payment, error) {
	return s.repo.GetByID(ctx, id)
}

// ListByInvoice returns all payments for an invoice.
func (s *PaymentService) ListByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]*Payment, error) {
	return s.repo.ListByInvoice(ctx, invoiceID)
}

// Update updates a payment's status/method/reference/received_at.
func (s *PaymentService) Update(ctx context.Context, id uuid.UUID, req UpdatePaymentRequest) (*Payment, error) {
	return s.repo.Update(ctx, id, req)
}

// SumCompleted returns the sum of completed payments for an invoice.
func (s *PaymentService) SumCompleted(ctx context.Context, invoiceID uuid.UUID) (int64, error) {
	return s.repo.SumByInvoice(ctx, invoiceID)
}

// ValidatePaymentRequest validates a CreatePaymentRequest.
func ValidatePaymentRequest(req *CreatePaymentRequest) PaymentValidationErrors {
	var errs PaymentValidationErrors

	if len(req.Currency) != 3 {
		errs = append(errs, PaymentFieldError{"currency", "must be a 3-letter ISO 4217 code"})
	} else {
		for _, ch := range req.Currency {
			if ch < 'A' || ch > 'Z' {
				errs = append(errs, PaymentFieldError{"currency", "must be uppercase A-Z"})
				break
			}
		}
	}

	if req.AmountMinor <= 0 {
		errs = append(errs, PaymentFieldError{"amount_minor", "must be positive"})
	}

	if !req.Kind.IsValid() {
		errs = append(errs, PaymentFieldError{"kind", "must be one of deposit, payment, refund"})
	}

	if req.Method != "" && len(req.Method) > 50 {
		errs = append(errs, PaymentFieldError{"method", "exceeds 50 characters"})
	}
	if req.Reference != "" && len(req.Reference) > 200 {
		errs = append(errs, PaymentFieldError{"reference", "exceeds 200 characters"})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Amount returns the amount as decimal.Decimal for display.
func (p *Payment) Amount() decimal.Decimal {
	return decimal.NewFromInt(p.AmountMinor).Div(decimal.NewFromInt(100))
}

// AmountMajor returns the amount in major units as float64 for display only.
func (p *Payment) AmountMajor() float64 {
	f, _ := p.Amount().Float64()
	return f
}

// Scan implements sql.Scanner for PaymentStatus.
func (s *PaymentStatus) Scan(value interface{}) error {
	if value == nil {
		*s = PaymentStatusPending
		return nil
	}
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into PaymentStatus", value)
	}
	*s = PaymentStatus(str)
	return nil
}

// Value implements driver.Valuer for PaymentStatus.
func (s PaymentStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements sql.Scanner for PaymentKind.
func (k *PaymentKind) Scan(value interface{}) error {
	if value == nil {
		*k = PaymentKindPayment
		return nil
	}
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into PaymentKind", value)
	}
	*k = PaymentKind(str)
	return nil
}

// Value implements driver.Valuer for PaymentKind.
func (k PaymentKind) Value() (driver.Value, error) {
	return string(k), nil
}