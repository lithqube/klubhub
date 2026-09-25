package finance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// InvoiceRepositoryIface is the subset of InvoiceRepository the service uses.
type InvoiceRepositoryIface interface {
	CreateDraft(ctx context.Context, gig *GigFeeInfo, req CreateInvoiceRequest, profile *BillingProfile) (*Invoice, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error)
	List(ctx context.Context, filter InvoiceFilter) ([]*Invoice, error)
	UpdateDraft(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error)
	Issue(ctx context.Context, id uuid.UUID, req IssueInvoiceRequest, profile *BillingProfile) (*Invoice, error)
	Pay(ctx context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error)
	Cancel(ctx context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error)
	Correct(ctx context.Context, id uuid.UUID, req CorrectInvoiceRequest, profile *BillingProfile) (*Invoice, error)
	NextNumber(ctx context.Context, prefix, currency string) (string, int64, error)
	Summaries(ctx context.Context) (map[string]CurrencySummary, error)
}

// GigFeeProvider supplies the gig fee info needed to create an invoice.
type GigFeeProvider interface {
	GetGigFeeInfo(ctx context.Context, gigID uuid.UUID) (*GigFeeInfo, error)
}

// BillingServiceIface is the subset of billing.Service the invoice service uses.
type BillingServiceIface interface {
	Get(ctx context.Context) (*BillingProfile, error)
	Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error)
}

// InvoiceService coordinates repository writes with validation.
type InvoiceService struct {
	repo        InvoiceRepositoryIface
	billingSvc  BillingServiceIface
	gigProvider GigFeeProvider
}

// NewInvoiceService returns an InvoiceService.
func NewInvoiceService(repo InvoiceRepositoryIface, billingSvc BillingServiceIface, gigProvider GigFeeProvider) *InvoiceService {
	return &InvoiceService{repo: repo, billingSvc: billingSvc, gigProvider: gigProvider}
}

// CreateDraft creates a draft invoice for a gig.
func (s *InvoiceService) CreateDraft(ctx context.Context, gigID uuid.UUID, req CreateInvoiceRequest) (*Invoice, error) {
	if s.gigProvider == nil {
		return nil, errors.New("gig fee provider not configured")
	}
	gig, err := s.gigProvider.GetGigFeeInfo(ctx, gigID)
	if err != nil {
		return nil, err
	}
	// The line amounts are denominated in the gig's currency; invoicing
	// them under another code would silently mix currencies.
	if gig.Currency != "" && req.Currency != gig.Currency {
		return nil, fmt.Errorf("%w: currency %s does not match gig fee currency %s",
			ErrInvoiceValidation, req.Currency, gig.Currency)
	}
	var profile *BillingProfile
	if s.billingSvc != nil {
		p, err := s.billingSvc.Get(ctx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		profile = p
	}
	return s.repo.CreateDraft(ctx, gig, req, profile)
}

// GetByID returns an invoice with its lines.
func (s *InvoiceService) GetByID(ctx context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns invoices matching the filter.
func (s *InvoiceService) List(ctx context.Context, filter InvoiceFilter) ([]*Invoice, error) {
	return s.repo.List(ctx, filter)
}

// UpdateDraft updates a draft invoice.
func (s *InvoiceService) UpdateDraft(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error) {
	return s.repo.UpdateDraft(ctx, id, req)
}

// Issue transitions draft → issued, snapshotting the billing profile.
func (s *InvoiceService) Issue(ctx context.Context, id uuid.UUID, req IssueInvoiceRequest) (*Invoice, error) {
	var profile *BillingProfile
	if s.billingSvc != nil {
		p, err := s.billingSvc.Get(ctx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		profile = p
	}
	return s.repo.Issue(ctx, id, req, profile)
}

// Pay transitions issued → paid.
func (s *InvoiceService) Pay(ctx context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error) {
	return s.repo.Pay(ctx, id, req)
}

// Cancel transitions draft/issued → cancelled.
func (s *InvoiceService) Cancel(ctx context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error) {
	return s.repo.Cancel(ctx, id, req)
}

// Correct creates a correction draft and marks original corrected.
func (s *InvoiceService) Correct(ctx context.Context, id uuid.UUID, req CorrectInvoiceRequest) (*Invoice, error) {
	var profile *BillingProfile
	if s.billingSvc != nil {
		p, err := s.billingSvc.Get(ctx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		profile = p
	}
	return s.repo.Correct(ctx, id, req, profile)
}

// NextNumber previews the next number.
func (s *InvoiceService) NextNumber(ctx context.Context, prefix, currency string) (string, int64, error) {
	return s.repo.NextNumber(ctx, prefix, currency)
}

// Summaries returns per-currency dashboard totals.
func (s *InvoiceService) Summaries(ctx context.Context) (map[string]CurrencySummary, error) {
	return s.repo.Summaries(ctx)
}

// Compile-time interface compliance.
var _ = uuid.Nil
