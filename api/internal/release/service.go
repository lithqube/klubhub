package release

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store is the storage the service needs. The concrete implementation is
// Repository; tests substitute an in-memory one.
type Store interface {
	Create(ctx context.Context, rel *Release) error
	Update(ctx context.Context, rel *Release) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*Release, error)
	List(ctx context.Context) ([]*Release, error)
}

// Input is what a caller supplies to create or edit a release (REL-01).
type Input struct {
	Title       string
	Artists     string
	Label       string
	Distributor string
	ReleaseDate time.Time
	Format      ReleaseFormat
	Notes       string
}

// Service applies the release rules on top of storage.
type Service struct {
	repo Store
	now  func() time.Time
}

// NewService creates a Service over the given store.
func NewService(repo Store) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Create stores a new release in its first workflow state.
func (s *Service) Create(ctx context.Context, in Input) (*Release, error) {
	rel, err := s.build(in)
	if err != nil {
		return nil, err
	}
	rel.Status = StatusDemoSent
	if err := s.repo.Create(ctx, rel); err != nil {
		return nil, err
	}
	return rel, nil
}

// Update edits the release fields and leaves the status untouched.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (*Release, error) {
	rel, err := s.build(in)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rel.ID = existing.ID
	rel.Status = existing.Status
	rel.CreatedAt = existing.CreatedAt
	if err := s.repo.Update(ctx, rel); err != nil {
		return nil, err
	}
	return rel, nil
}

// Delete soft-deletes a release.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDelete(ctx, id)
}

// Transition moves a release to the next workflow state (REL-02). Moving to
// the state it is already in is a no-op; any other illegal move is refused.
func (s *Service) Transition(ctx context.Context, id uuid.UUID, to ReleaseStatus) (*Release, error) {
	rel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rel.Status == to {
		return rel, nil
	}
	if !CanTransition(rel.Status, to) {
		return nil, ErrInvalidTransition
	}
	rel.Status = to
	if err := s.repo.Update(ctx, rel); err != nil {
		return nil, err
	}
	return rel, nil
}

// Get returns one release.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Release, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns every live release, ordered by release date.
func (s *Service) List(ctx context.Context) ([]*Release, error) {
	return s.repo.List(ctx)
}

// build validates the input and returns the release it describes.
func (s *Service) build(in Input) (*Release, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, errors.Join(ErrInvalid, errors.New("title is required"))
	}
	if in.ReleaseDate.IsZero() {
		return nil, errors.Join(ErrInvalid, errors.New("release_date is required"))
	}
	format := in.Format
	if format == "" {
		format = FormatDigital
	}
	if !ValidFormat(format) {
		return nil, errors.Join(ErrInvalid, errors.New("format must be digital, vinyl or both"))
	}
	return &Release{
		Title:       title,
		Artists:     strings.TrimSpace(in.Artists),
		Label:       strings.TrimSpace(in.Label),
		Distributor: strings.TrimSpace(in.Distributor),
		ReleaseDate: time.Date(in.ReleaseDate.Year(), in.ReleaseDate.Month(), in.ReleaseDate.Day(), 0, 0, 0, 0, time.UTC),
		Format:      format,
		Notes:       in.Notes,
	}, nil
}
