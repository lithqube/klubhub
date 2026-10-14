package finance

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type EntryRepositoryIface interface {
	Create(context.Context, CreateEntryRequest) (*Entry, error)
	CreateGenerated(context.Context, GeneratedEntryInput) (*Entry, error)
	Get(context.Context, uuid.UUID) (*Entry, error)
	GetByGeneratedSource(context.Context, EntrySourceKind, uuid.UUID, EntryKind) (*Entry, error)
	List(context.Context, EntryFilter) ([]*Entry, error)
	ListByGig(context.Context, uuid.UUID) ([]*Entry, error)
	Update(context.Context, uuid.UUID, UpdateEntryRequest) (*Entry, error)
	Delete(context.Context, uuid.UUID, time.Time) error
	Void(context.Context, uuid.UUID, time.Time) (*Entry, error)
	Summary(context.Context, DateRange, *uuid.UUID) (map[string]EntryTotals, error)
	GetPendingReconciliationByGig(context.Context, uuid.UUID) (*EntryReconciliation, error)
	GetPendingReconciliationByEntry(context.Context, uuid.UUID) (*EntryReconciliation, error)
	ResolveReconciliation(context.Context, uuid.UUID, ResolveReconciliationRequest) (*EntryReconciliation, error)
}

type EntryService struct{ repo EntryRepositoryIface }

func NewEntryService(repo EntryRepositoryIface) *EntryService { return &EntryService{repo: repo} }

func (s *EntryService) Create(ctx context.Context, req CreateEntryRequest) (*Entry, error) {
	if err := ValidateCreateEntry(req); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, req)
}

func (s *EntryService) CreateGenerated(ctx context.Context, req GeneratedEntryInput) (*Entry, error) {
	if err := ValidateGeneratedEntry(req); err != nil {
		return nil, err
	}
	return s.repo.CreateGenerated(ctx, req)
}

func (s *EntryService) Get(ctx context.Context, id uuid.UUID) (*Entry, error) {
	return s.repo.Get(ctx, id)
}
func (s *EntryService) GetByGeneratedSource(ctx context.Context, sk EntrySourceKind, sid uuid.UUID, kind EntryKind) (*Entry, error) {
	return s.repo.GetByGeneratedSource(ctx, sk, sid, kind)
}
func (s *EntryService) ListByGig(ctx context.Context, id uuid.UUID) ([]*Entry, error) {
	return s.repo.ListByGig(ctx, id)
}

func (s *EntryService) List(ctx context.Context, filter EntryFilter) ([]*Entry, error) {
	if filter.Kind != "" && !filter.Kind.IsValid() {
		return nil, EntryValidationErrors{{"kind", "must be income or expense"}}
	}
	if filter.Status != "" && !filter.Status.IsValid() {
		return nil, EntryValidationErrors{{"status", "must be active or voided"}}
	}
	if filter.Currency != "" && !isCurrencyCode(filter.Currency) {
		return nil, EntryValidationErrors{{"currency", "must be an uppercase 3-letter code"}}
	}
	if filter.GigID != nil && *filter.GigID == uuid.Nil {
		return nil, EntryValidationErrors{{"gig_id", "must not be nil UUID"}}
	}
	if !filter.Receipt.IsValid() {
		return nil, EntryValidationErrors{{"receipt", "must be missing or present"}}
	}
	if filter.From != "" || filter.To != "" {
		if err := (DateRange{From: filter.From, To: filter.To}).Validate(); err != nil {
			return nil, err
		}
	}
	return s.repo.List(ctx, filter)
}

func (s *EntryService) Update(ctx context.Context, id uuid.UUID, req UpdateEntryRequest) (*Entry, error) {
	if err := ValidateUpdateEntry(req); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, id, req)
}

func (s *EntryService) Delete(ctx context.Context, id uuid.UUID, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		return EntryValidationErrors{{"updated_at", "is required"}}
	}
	return s.repo.Delete(ctx, id, updatedAt)
}

func (s *EntryService) Void(ctx context.Context, id uuid.UUID, updatedAt time.Time) (*Entry, error) {
	if updatedAt.IsZero() {
		return nil, EntryValidationErrors{{"updated_at", "is required"}}
	}
	return s.repo.Void(ctx, id, updatedAt)
}

func (s *EntryService) Summary(ctx context.Context, dr DateRange) (map[string]EntryTotals, error) {
	if err := dr.Validate(); err != nil {
		return nil, err
	}
	return s.repo.Summary(ctx, dr, nil)
}

func (s *EntryService) ProfitLoss(ctx context.Context, f ProfitLossFilter) (map[string]ProfitLossTotals, error) {
	dr, err := f.DateRange()
	if err != nil {
		return nil, err
	}
	var gigID *uuid.UUID
	if f.Scope == ProfitLossScopeGig {
		gigID = f.GigID
	}
	totals, err := s.repo.Summary(ctx, dr, gigID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ProfitLossTotals, len(totals))
	for currency, v := range totals {
		out[currency] = ProfitLossTotals{Currency: currency, IncomeMinor: v.IncomeMinor, ExpenseMinor: v.ExpenseMinor, ProfitLossMinor: v.IncomeMinor - v.ExpenseMinor}
	}
	return out, nil
}

func (s *EntryService) GetPendingReconciliationByGig(ctx context.Context, gigID uuid.UUID) (*EntryReconciliation, error) {
	return s.repo.GetPendingReconciliationByGig(ctx, gigID)
}

func (s *EntryService) GetPendingReconciliationByEntry(ctx context.Context, entryID uuid.UUID) (*EntryReconciliation, error) {
	return s.repo.GetPendingReconciliationByEntry(ctx, entryID)
}

func (s *EntryService) ResolveReconciliation(ctx context.Context, id uuid.UUID, req ResolveReconciliationRequest) (*EntryReconciliation, error) {
	return s.repo.ResolveReconciliation(ctx, id, req)
}
