package finance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/finance/tax"
)

// InvoiceRepositoryIface is the subset of InvoiceRepository the service uses.
type InvoiceRepositoryIface interface {
	CreateDraft(ctx context.Context, in DraftInput) (*Invoice, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error)
	List(ctx context.Context, filter InvoiceFilter) ([]*Invoice, error)
	UpdateDraft(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error)
	Issue(ctx context.Context, id uuid.UUID, req IssueInvoiceRequest, profile *BillingProfile, check IssueCheckFunc) (*Invoice, error)
	Pay(ctx context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error)
	Cancel(ctx context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error)
	CreditNote(ctx context.Context, id uuid.UUID, req CreditNoteRequest, profile *BillingProfile, correct bool) (*CreditNoteResult, error)
	NextNumber(ctx context.Context, prefix, currency string) (string, int64, error)
	Summaries(ctx context.Context) (map[string]CurrencySummary, error)
}

// GigFeeProvider supplies the gig data needed to create an invoice.
type GigFeeProvider interface {
	GetGigFeeInfo(ctx context.Context, gigID uuid.UUID) (*GigFeeInfo, error)
}

// BillingServiceIface is the subset of billing.Service the invoice service uses.
type BillingServiceIface interface {
	Get(ctx context.Context) (*BillingProfile, error)
	Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error)
}

// InvoiceService coordinates repository writes with validation and the
// tax rules of package tax.
type InvoiceService struct {
	repo        InvoiceRepositoryIface
	billingSvc  BillingServiceIface
	gigProvider GigFeeProvider
}

// NewInvoiceService returns an InvoiceService.
func NewInvoiceService(repo InvoiceRepositoryIface, billingSvc BillingServiceIface, gigProvider GigFeeProvider) *InvoiceService {
	return &InvoiceService{repo: repo, billingSvc: billingSvc, gigProvider: gigProvider}
}

// profile returns the billing profile, or nil when billing isn't wired or
// no profile exists yet.
func (s *InvoiceService) profile(ctx context.Context) (*BillingProfile, error) {
	if s.billingSvc == nil {
		return nil, nil
	}
	p, err := s.billingSvc.Get(ctx)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return p, nil
}

// CreateDraft creates an unnumbered draft for a gig. Omitted fields are
// pre-filled: customer from the gig contact, supply date from the gig
// date, treatment / rate / note from tax.Suggest.
func (s *InvoiceService) CreateDraft(ctx context.Context, gigID uuid.UUID, req CreateInvoiceRequest) (*Invoice, error) {
	if s.gigProvider == nil {
		return nil, errors.New("gig fee provider not configured")
	}
	gig, err := s.gigProvider.GetGigFeeInfo(ctx, gigID)
	if err != nil {
		return nil, err
	}
	var errs InvoiceValidationErrors

	// The line amounts are denominated in the gig's currency; invoicing
	// them under another code would silently mix currencies.
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	switch {
	case gig.Currency != "" && currency != "" && currency != gig.Currency:
		errs = append(errs, InvoiceFieldError{"currency",
			fmt.Sprintf("currency %s does not match gig fee currency %s", currency, gig.Currency)})
	case gig.Currency != "":
		currency = gig.Currency
	}
	if !isUpperAlpha(currency, 3) {
		errs = append(errs, InvoiceFieldError{"currency", "must be a 3-letter ISO 4217 code"})
	}

	profile, err := s.profile(ctx)
	if err != nil {
		return nil, err
	}
	supplier := supplierFromProfile(profile)

	customer := Party{}
	switch {
	case req.Customer != nil:
		customer = *req.Customer
	case gig.Customer != nil:
		customer = *gig.Customer
	}
	normalizeParty(&customer)

	sugg := tax.Suggest(supplier, customer.TaxCustomer())
	treatment, rate, note := sugg.VATTreatment, sugg.TaxRateBps, sugg.TaxNote
	if req.VATTreatment != nil && *req.VATTreatment != sugg.VATTreatment {
		treatment = *req.VATTreatment
		rate = tax.DefaultRate(treatment, supplier)
		note = tax.Notes.Note(supplier.Country, treatment)
	}
	if req.TaxRateBps != nil {
		rate = *req.TaxRateBps
	}
	var withholding int64
	if req.WithholdingRateBps != nil {
		withholding = *req.WithholdingRateBps
	}
	supplyDate := normalizeDate(req.SupplyDate)
	if supplyDate == nil && gig.Date != "" {
		d := gig.Date
		supplyDate = &d
	}
	prefix := normalizePrefix(req.NumberPrefix)

	errs = append(errs, validateParty("customer.", customer)...)
	errs = append(errs, validateTaxFields(treatment, rate, withholding, note)...)
	errs = append(errs, validatePrefix(prefix)...)
	errs = append(errs, validateDate("supply_date", supplyDate)...)
	if len(errs) > 0 {
		return nil, errs
	}

	lines := []DraftLine{{Description: gig.LineDescription, Quantity: 1, UnitMinor: gig.FeeMinor}}
	taxLines := make([]tax.Line, len(lines))
	for i, l := range lines {
		taxLines[i] = tax.Line{NetMinor: int64(l.Quantity) * l.UnitMinor}
	}
	return s.repo.CreateDraft(ctx, DraftInput{
		GigID:              gig.ID,
		Currency:           currency,
		NumberPrefix:       prefix,
		Customer:           customer,
		VATTreatment:       treatment,
		TaxRateBps:         rate,
		TaxNote:            note,
		WithholdingRateBps: withholding,
		SupplyDate:         supplyDate,
		DueAt:              req.DueAt.ptr(),
		Lines:              lines,
		Totals:             tax.ComputeTotals(taxLines, rate, withholding),
	})
}

// GetByID returns an invoice with its lines.
func (s *InvoiceService) GetByID(ctx context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns invoices matching the filter.
func (s *InvoiceService) List(ctx context.Context, filter InvoiceFilter) ([]*Invoice, error) {
	return s.repo.List(ctx, filter)
}

// UpdateDraft validates and applies a full draft update; the repository
// recomputes totals from the lines.
func (s *InvoiceService) UpdateDraft(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error) {
	if err := normalizeUpdate(&req); err != nil {
		return nil, err
	}
	return s.repo.UpdateDraft(ctx, id, req)
}

// TaxSuggestion suggests a treatment for a customer, using the billing
// profile as supplier.
func (s *InvoiceService) TaxSuggestion(ctx context.Context, customer Party) (tax.Suggestion, error) {
	profile, err := s.profile(ctx)
	if err != nil {
		return tax.Suggestion{}, err
	}
	normalizeParty(&customer)
	return tax.Suggest(supplierFromProfile(profile), customer.TaxCustomer()), nil
}

// problemsFor runs tax.ValidateForIssue for an invoice. It is pure so it
// can run inside the issue transaction.
func problemsFor(inv *Invoice, profile *BillingProfile, gigCurrency string) []tax.Problem {
	return tax.ValidateForIssue(tax.IssueInput{
		Supplier:           supplierFromProfile(profile),
		Customer:           inv.Customer.TaxCustomer(),
		Treatment:          inv.VATTreatment,
		TaxRateBps:         inv.TaxRateBps,
		TaxNote:            inv.TaxNote,
		WithholdingRateBps: inv.WithholdingRateBps,
		SupplyDateSet:      inv.SupplyDate != nil && *inv.SupplyDate != "",
		TotalMinor:         inv.TotalMinor,
		Currency:           inv.Currency,
		GigCurrency:        gigCurrency,
	})
}

// IssueCheck reports whether a draft can be issued and, if not, why.
func (s *InvoiceService) IssueCheck(ctx context.Context, id uuid.UUID) (*IssueCheck, error) {
	inv, _, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if inv.Kind != InvoiceKindInvoice || inv.Status != InvoiceStatusDraft {
		return &IssueCheck{Ready: false, Problems: []tax.Problem{{Field: "status", Message: "only draft invoices can be issued"}}}, nil
	}
	profile, err := s.profile(ctx)
	if err != nil {
		return nil, err
	}
	gigCurrency := ""
	if s.gigProvider != nil {
		gig, err := s.gigProvider.GetGigFeeInfo(ctx, inv.GigID)
		switch {
		case err == nil:
			gigCurrency = gig.Currency
		case errors.Is(err, ErrInvoiceNotFound):
			// Gig deleted since the draft was made: nothing to match against.
		default:
			return nil, err
		}
	}
	problems := problemsFor(inv, profile, gigCurrency)
	return &IssueCheck{Ready: len(problems) == 0, Problems: problems}, nil
}

// Issue validates the draft (422 not_issuable with problems), allocates
// its number and snapshots the billing profile, all in one transaction.
func (s *InvoiceService) Issue(ctx context.Context, id uuid.UUID, req IssueInvoiceRequest) (*Invoice, error) {
	profile, err := s.profile(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.Issue(ctx, id, req, profile, func(inv *Invoice, gigCurrency string) []tax.Problem {
		return problemsFor(inv, profile, gigCurrency)
	})
}

// Pay transitions issued → paid.
func (s *InvoiceService) Pay(ctx context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error) {
	return s.repo.Pay(ctx, id, req)
}

// Cancel transitions draft → cancelled.
func (s *InvoiceService) Cancel(ctx context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error) {
	return s.repo.Cancel(ctx, id, req)
}

func validateReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 1000 {
		return "", InvoiceValidationErrors{{"reason", "exceeds 1000 characters"}}
	}
	return reason, nil
}

// CreditNote issues a credit note reversing an issued/paid invoice.
func (s *InvoiceService) CreditNote(ctx context.Context, id uuid.UUID, req CreditNoteRequest) (*CreditNoteResult, error) {
	return s.creditNote(ctx, id, req, false)
}

// Correct issues a credit note and creates a replacement draft.
func (s *InvoiceService) Correct(ctx context.Context, id uuid.UUID, req CorrectInvoiceRequest) (*CreditNoteResult, error) {
	return s.creditNote(ctx, id, req, true)
}

func (s *InvoiceService) creditNote(ctx context.Context, id uuid.UUID, req CreditNoteRequest, correct bool) (*CreditNoteResult, error) {
	reason, err := validateReason(req.Reason)
	if err != nil {
		return nil, err
	}
	req.Reason = reason
	profile, err := s.profile(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.CreditNote(ctx, id, req, profile, correct)
}

// NextNumber previews the next number.
func (s *InvoiceService) NextNumber(ctx context.Context, prefix, currency string) (string, int64, error) {
	return s.repo.NextNumber(ctx, prefix, currency)
}

// Summaries returns per-currency dashboard totals.
func (s *InvoiceService) Summaries(ctx context.Context) (map[string]CurrencySummary, error) {
	return s.repo.Summaries(ctx)
}
