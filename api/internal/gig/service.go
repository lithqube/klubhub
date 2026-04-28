package gig

import (
	"context"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/tracklist"
	"github.com/klubhub/dj/api/internal/venue"
)

// tracklistRepoIface is the subset of the tracklist repository needed by GigService.
type tracklistRepoIface interface {
	Get(ctx context.Context, id uuid.UUID) (*tracklist.Tracklist, error)
}

// venueRepoIface is the subset of the venue repository needed by GigService.
type venueRepoIface interface {
	GetByID(ctx context.Context, id uuid.UUID) (*venue.Venue, error)
}

// contactRepoIface is the subset of the contact repository needed by GigService.
type contactRepoIface interface {
	GetByID(ctx context.Context, id uuid.UUID) (*contact.Contact, error)
}

// Service implements business logic for gigs and satisfies the GigReader interface.
type Service struct {
	repo           *Repository
	venueRepo      venueRepoIface
	contactRepo    contactRepoIface
	tracklistRepo  tracklistRepoIface
}

// NewService creates a new Service.
func NewService(repo *Repository, venueRepo venueRepoIface, contactRepo contactRepoIface, tracklistRepo tracklistRepoIface) *Service {
	return &Service{
		repo:          repo,
		venueRepo:     venueRepo,
		contactRepo:   contactRepo,
		tracklistRepo: tracklistRepo,
	}
}

// ─── GigReader interface implementation ─────────────────────────────────────

// GetGig returns a gig by ID. Satisfies GigReader.
func (s *Service) GetGig(ctx context.Context, id uuid.UUID) (*Gig, error) {
	return s.repo.GetByID(ctx, id)
}

// ListGigs returns gigs matching the filter. Satisfies GigReader.
func (s *Service) ListGigs(ctx context.Context, f GigFilter) ([]*Gig, error) {
	return s.repo.List(ctx, f)
}

// Tracklists returns tracklists linked to a gig via tracklist_gigs join.
// Satisfies GigReader interface consumed by downstream modules.
func (s *Service) Tracklists(ctx context.Context, gigID uuid.UUID) ([]*tracklist.Tracklist, error) {
	// Verify gig exists first
	if _, err := s.repo.GetByID(ctx, gigID); err != nil {
		return nil, err
	}

	// Get linked tracklist IDs via the join table
	rows, err := s.repo.pool.Query(ctx, `
		SELECT t.id, t.title, t.source_format, t.raw_file_path, t.preset,
		       t.visible_fields, t.bg_mode, t.bg_value, t.max_tracks,
		       t.track_range_start, t.track_range_end,
		       t.created_at, t.updated_at, t.deleted_at
		FROM tracklists t
		JOIN tracklist_gigs tg ON tg.tracklist_id = t.id
		WHERE tg.gig_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.created_at DESC`,
		gigID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracklists []*tracklist.Tracklist
	for rows.Next() {
		var tl tracklist.Tracklist
		err := rows.Scan(
			&tl.ID, &tl.Title, &tl.SourceFormat, &tl.RawFilePath, &tl.Preset,
			&tl.VisibleFields, &tl.BgMode, &tl.BgValue, &tl.MaxTracks,
			&tl.TrackRangeStart, &tl.TrackRangeEnd,
			&tl.CreatedAt, &tl.UpdatedAt, &tl.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		tracklists = append(tracklists, &tl)
	}
	return tracklists, rows.Err()
}

// ─── Write operations ────────────────────────────────────────────────────────

// CreateGig creates a new gig with default status=inquiry.
func (s *Service) CreateGig(ctx context.Context, req *GigCreate) (*Gig, error) {
	return s.repo.Create(ctx, req)
}

// UpdateGig updates a gig with status transition validation.
// Returns ErrForbidden if trying to change status away from cancelled (cancelled is terminal).
func (s *Service) UpdateGig(ctx context.Context, id uuid.UUID, req *GigUpdate) (*Gig, error) {
	// Fetch current gig to check status transition
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// If current status is cancelled, reject any status change
	if req.Status != nil && current.Status == GigStatusCancelled && *req.Status != GigStatusCancelled {
		return nil, ErrForbidden
	}

	return s.repo.Update(ctx, id, req)
}

// DeleteGig soft-deletes a gig.
func (s *Service) DeleteGig(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDelete(ctx, id)
}

// LinkVenue links a gig to a venue record.
func (s *Service) LinkVenue(ctx context.Context, gigID, venueID uuid.UUID, isPrimary bool) error {
	// Verify venue exists
	if _, err := s.venueRepo.GetByID(ctx, venueID); err != nil {
		return err
	}
	return s.repo.LinkVenue(ctx, gigID, venueID, isPrimary)
}

// UnlinkVenue unlinks a gig from a venue.
func (s *Service) UnlinkVenue(ctx context.Context, gigID, venueID uuid.UUID) error {
	return s.repo.UnlinkVenue(ctx, gigID, venueID)
}

// LinkContact links a gig to a contact record.
func (s *Service) LinkContact(ctx context.Context, gigID, contactID uuid.UUID, role string) error {
	// Verify contact exists
	if _, err := s.contactRepo.GetByID(ctx, contactID); err != nil {
		return err
	}
	return s.repo.LinkContact(ctx, gigID, contactID, role)
}

// UnlinkContact unlinks a gig from a contact.
func (s *Service) UnlinkContact(ctx context.Context, gigID, contactID uuid.UUID) error {
	return s.repo.UnlinkContact(ctx, gigID, contactID)
}
