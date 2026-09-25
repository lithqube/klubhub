package finance

import (
	"context"

	"github.com/google/uuid"
)

// RepositoryIface is the subset of Repository the service layer uses.
// Defined as an interface so handler tests can swap a fake.
type RepositoryIface interface {
	Get(ctx context.Context) (*BillingProfile, error)
	Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error)
}

// Service coordinates repository writes with validation.
type Service struct {
	repo RepositoryIface
}

// NewService returns a Service backed by repo.
func NewService(repo RepositoryIface) *Service { return &Service{repo: repo} }

// Get returns the singleton billing profile, creating an empty one on
// first call.
func (s *Service) Get(ctx context.Context) (*BillingProfile, error) {
	return s.repo.Get(ctx)
}

// Update validates the request and then writes it under the optimistic
// concurrency token. The returned profile carries the new UpdatedAt.
func (s *Service) Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error) {
	if errs := Validate(req); len(errs) > 0 {
		return nil, errs
	}
	return s.repo.Update(ctx, req)
}

// CanIssue reports whether the current profile has every field required
// to issue an invoice or an agreement. Callers should also enforce the
// service-level gate so a stale read never silently allows issuance.
func (s *Service) CanIssue(ctx context.Context) (bool, []string, error) {
	p, err := s.repo.Get(ctx)
	if err != nil {
		return false, nil, err
	}
	missing := MissingIssueFields(p)
	return len(missing) == 0, missing, nil
}

// Compile-time interface compliance for downstream packages.
var _ = uuid.Nil
