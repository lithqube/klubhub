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
	Get(ctx context.Context, id uuid.UUID) (*tracklist.Tracklist, []tracklist.Track, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
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
	storage        PDFStorageClientIface
}

// NewService creates a new Service.
func NewService(repo *Repository, venueRepo venueRepoIface, contactRepo contactRepoIface, tracklistRepo tracklistRepoIface, storage PDFStorageClientIface) *Service {
	return &Service{
		repo:          repo,
		venueRepo:     venueRepo,
		contactRepo:   contactRepo,
		tracklistRepo: tracklistRepo,
		storage:       storage,
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


// GetGigDetail returns a gig by ID with linked venues, contacts, and tracklists.
func (s *Service) GetGigDetail(ctx context.Context, id uuid.UUID) (*GigDetailResponse, error) {
	gig, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get linked venues from gig_venues join table
	var venues []LinkedVenueResponse
	venueRows, err := s.repo.pool.Query(ctx, `
		SELECT v.id, v.name, gv.is_primary
		FROM venues v
		JOIN gig_venues gv ON gv.venue_id = v.id
		WHERE gv.gig_id = $1 AND v.deleted_at IS NULL
		ORDER BY gv.is_primary DESC, v.name`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer venueRows.Close()
	for venueRows.Next() {
		var v venue.Venue
		var isPrimary bool
		if err := venueRows.Scan(&v.ID, &v.Name, &isPrimary); err != nil {
			return nil, err
		}
		venues = append(venues, LinkedVenueResponse{
			Venue: VenueResponse{
				ID:   v.ID,
				Name: v.Name,
			},
			IsPrimary: isPrimary,
		})
	}

	// Get linked contacts from gig_contacts join table
	var contacts []LinkedContactResponse
	contactRows, err := s.repo.pool.Query(ctx, `
		SELECT c.id, c.name, gc.role
		FROM contacts c
		JOIN gig_contacts gc ON gc.contact_id = c.id
		WHERE gc.gig_id = $1 AND c.deleted_at IS NULL
		ORDER BY gc.role`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer contactRows.Close()
	for contactRows.Next() {
		var c contact.Contact
		var role string
		if err := contactRows.Scan(&c.ID, &c.Name, &role); err != nil {
			return nil, err
		}
		contacts = append(contacts, LinkedContactResponse{
			Contact: ContactResponse{
				ID:   c.ID,
				Name: c.Name,
			},
			Role: role,
		})
	}

	// Get tracklists
	tracklists, err := s.Tracklists(ctx, id)
	if err != nil {
		return nil, err
	}

	// Initialize with capacity for empty array serialization
	tracklistResponses := make([]TracklistResponse, 0, len(tracklists))
	for _, tl := range tracklists {
		tracklistResponses = append(tracklistResponses, TracklistResponse{
			ID:    tl.ID,
			Title: tl.Title,
		})
	}

	return &GigDetailResponse{
		Gig:        *gig,
		Venues:     venues,
		Contacts:   contacts,
		Tracklists: tracklistResponses,
	}, nil
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

// LinkTracklist links a gig to a tracklist record.
func (s *Service) LinkTracklist(ctx context.Context, gigID, tracklistID uuid.UUID) error {
	if _, _, err := s.tracklistRepo.Get(ctx, tracklistID); err != nil {
		return err
	}
	return s.repo.LinkTracklist(ctx, gigID, tracklistID)
}

// UnlinkTracklist unlinks a gig from a tracklist.
func (s *Service) UnlinkTracklist(ctx context.Context, gigID, tracklistID uuid.UUID) error {
	return s.repo.UnlinkTracklist(ctx, gigID, tracklistID)
}
func (s *Service) GenerateCalendar(ctx context.Context, config CalendarConfig) (string, error) {
	return GenerateCalendar(ctx, s, config)
}

// GenerateBookingPDF generates a booking confirmation PDF for a confirmed gig.
// Returns PDF bytes and storage path.
func (s *Service) GenerateBookingPDF(ctx context.Context, gigID uuid.UUID, djName string) ([]byte, string, error) {
	gig, err := s.repo.GetByID(ctx, gigID)
	if err != nil {
		return nil, "", err
	}

	// PDF only available for confirmed gigs
	if gig.Status != GigStatusConfirmed {
		return nil, "", ErrForbidden
	}

	pdfData, err := GenerateBookingPDF(gig, djName)
	if err != nil {
		return nil, "", err
	}

	storagePath, err := StoreBookingPDF(ctx, gig, pdfData, s.storage)
	if err != nil {
		return nil, "", err
	}

	return pdfData, storagePath, nil
}
