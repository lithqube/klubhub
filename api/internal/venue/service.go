package venue

import (
	"context"

	"github.com/google/uuid"
)

// Service implements business logic for venues.
type Service struct {
	repo *Repository
}

// NewService creates a new Service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CreateVenue creates a new venue.
func (s *Service) CreateVenue(ctx context.Context, req *VenueCreate) (*Venue, error) {
	return s.repo.Create(ctx, req)
}

// GetVenue returns a venue by ID.
func (s *Service) GetVenue(ctx context.Context, id uuid.UUID) (*Venue, error) {
	return s.repo.GetByID(ctx, id)
}

// ListVenues returns venues with optional filters.
func (s *Service) ListVenues(ctx context.Context, name, city *string) ([]*Venue, error) {
	return s.repo.List(ctx, name, city)
}

// UpdateVenue updates a venue.
func (s *Service) UpdateVenue(ctx context.Context, id uuid.UUID, req *VenueUpdate) (*Venue, error) {
	return s.repo.Update(ctx, id, req)
}

// DeleteVenue soft-deletes a venue.
func (s *Service) DeleteVenue(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDelete(ctx, id)
}

// Autocomplete returns venues matching query for autocomplete UI.
func (s *Service) Autocomplete(ctx context.Context, query string, limit int) ([]*Venue, error) {
	if len(query) < 2 {
		return []*Venue{}, nil
	}
	return s.repo.SearchByName(ctx, query, limit)
}
