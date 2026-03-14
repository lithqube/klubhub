package settings

import "context"

// Service provides business logic for user_settings.
// It wraps Repository and maps errors as needed.
type Service struct {
	repo *Repository
}

// NewService creates a new Service backed by the given Repository.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetOrCreate returns the active user_settings singleton, creating it if absent.
func (s *Service) GetOrCreate(ctx context.Context) (*UserSettings, error) {
	return s.repo.GetOrCreate(ctx)
}

// Update modifies the user_settings row with optimistic concurrency.
// Returns ErrConflict if req.UpdatedAt is stale.
func (s *Service) Update(ctx context.Context, req UpdateSettingsRequest) (*UserSettings, error) {
	return s.repo.Update(ctx, req)
}
