package contact

import (
	"context"

	"github.com/google/uuid"
)

// Service implements business logic for contacts.
type Service struct {
	repo *Repository
}

// NewService creates a new Service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CreateContact creates a new contact.
func (s *Service) CreateContact(ctx context.Context, req *ContactCreate) (*Contact, error) {
	return s.repo.Create(ctx, req)
}

// GetContact returns a contact by ID.
func (s *Service) GetContact(ctx context.Context, id uuid.UUID) (*Contact, error) {
	return s.repo.GetByID(ctx, id)
}

// ListContacts returns contacts with optional filters.
func (s *Service) ListContacts(ctx context.Context, name *string, contactType *ContactType) ([]*Contact, error) {
	return s.repo.List(ctx, name, contactType)
}

// UpdateContact updates a contact.
func (s *Service) UpdateContact(ctx context.Context, id uuid.UUID, req *ContactUpdate) (*Contact, error) {
	return s.repo.Update(ctx, id, req)
}

// DeleteContact soft-deletes a contact.
func (s *Service) DeleteContact(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDelete(ctx, id)
}

// Autocomplete returns contacts matching query for autocomplete UI.
func (s *Service) Autocomplete(ctx context.Context, query string, limit int) ([]*Contact, error) {
	if len(query) < 2 {
		return []*Contact{}, nil
	}
	return s.repo.SearchByName(ctx, query, limit)
}
