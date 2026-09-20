package gig

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/ra"
	"github.com/klubhub/dj/api/internal/venue"
)

// mockVenueService implements venue.ServiceIface for testing
type mockVenueService struct {
	venues   map[uuid.UUID]*venue.Venue
	creates  []*venue.Venue
	autocompleteResults []*venue.Venue
}

func newMockVenueService() *mockVenueService {
	return &mockVenueService{
		venues:  make(map[uuid.UUID]*venue.Venue),
		creates: make([]*venue.Venue, 0),
	}
}

func (m *mockVenueService) CreateVenue(ctx context.Context, req *venue.VenueCreate) (*venue.Venue, error) {
	id := uuid.New()
	v := &venue.Venue{
		ID:        id,
		Name:      req.Name,
		City:      req.City,
		Country:   req.Country,
		Website:   req.Website,
		Notes:     req.Notes,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.venues[id] = v
	m.creates = append(m.creates, v)
	return v, nil
}

func (m *mockVenueService) Autocomplete(ctx context.Context, query string, limit int) ([]*venue.Venue, error) {
	if len(m.autocompleteResults) > 0 {
		if limit > len(m.autocompleteResults) {
			limit = len(m.autocompleteResults)
		}
		result := make([]*venue.Venue, limit)
		copy(result, m.autocompleteResults[:limit])
		return result, nil
	}
	return []*venue.Venue{}, nil
}

func (m *mockVenueService) GetByID(ctx context.Context, id uuid.UUID) (*venue.Venue, error) {
	if v, ok := m.venues[id]; ok {
		return v, nil
	}
	return nil, venue.ErrNotFound
}

func (m *mockVenueService) GetVenue(ctx context.Context, id uuid.UUID) (*venue.Venue, error) {
	return m.GetByID(ctx, id)
}

func (m *mockVenueService) ListVenues(ctx context.Context, name, city *string) ([]*venue.Venue, error) {
	return nil, nil
}

func (m *mockVenueService) UpdateVenue(ctx context.Context, id uuid.UUID, req *venue.VenueUpdate) (*venue.Venue, error) {
	return nil, nil
}

func (m *mockVenueService) DeleteVenue(ctx context.Context, id uuid.UUID) error {
	return nil
}

// mockContactService implements contact.ServiceIface for testing
type mockContactService struct {
	contacts   map[uuid.UUID]*contact.Contact
	creates    []*contact.Contact
	autocompleteResults []*contact.Contact
}

func newMockContactService() *mockContactService {
	return &mockContactService{
		contacts:  make(map[uuid.UUID]*contact.Contact),
		creates:   make([]*contact.Contact, 0),
	}
}

func (m *mockContactService) CreateContact(ctx context.Context, req *contact.ContactCreate) (*contact.Contact, error) {
	id := uuid.New()
	c := &contact.Contact{
		ID:        id,
		Name:      req.Name,
		Email:     req.Email,
		Phone:     req.Phone,
		Type:      req.Type,
		Notes:     req.Notes,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.contacts[id] = c
	m.creates = append(m.creates, c)
	return c, nil
}

func (m *mockContactService) Autocomplete(ctx context.Context, query string, limit int) ([]*contact.Contact, error) {
	if len(m.autocompleteResults) > 0 {
		if limit > len(m.autocompleteResults) {
			limit = len(m.autocompleteResults)
		}
		result := make([]*contact.Contact, limit)
		copy(result, m.autocompleteResults[:limit])
		return result, nil
	}
	return []*contact.Contact{}, nil
}

func (m *mockContactService) GetByID(ctx context.Context, id uuid.UUID) (*contact.Contact, error) {
	if c, ok := m.contacts[id]; ok {
		return c, nil
	}
	return nil, contact.ErrNotFound
}

func (m *mockContactService) GetContact(ctx context.Context, id uuid.UUID) (*contact.Contact, error) {
	return m.GetByID(ctx, id)
}

func (m *mockContactService) ListContacts(ctx context.Context, name *string, contactType *contact.ContactType) ([]*contact.Contact, error) {
	return nil, nil
}

func (m *mockContactService) UpdateContact(ctx context.Context, id uuid.UUID, req *contact.ContactUpdate) (*contact.Contact, error) {
	return nil, nil
}

func (m *mockContactService) DeleteContact(ctx context.Context, id uuid.UUID) error {
	return nil
}

// TestRAImportHandler_HandleImportFromRA_Success tests successful RA import
func TestRAImportHandler_HandleImportFromRA_Success(t *testing.T) {
	// Create mock services with proper interfaces
	mockGigSvc := &mockGigServiceForTest{}
	mockVenueSvc := newMockVenueService()
	mockContactSvc := newMockContactService()
	mockRAClient := &mockRAClientForTest{
		artist: &ra.RAArtist{
			ID:   "123",
			Name: "Test Artist",
			Slug: "test-artist",
		},
		events: []ra.RAEVENT{
			{
				ID:        "evt1",
				Title:     "Test Event 1",
				Date:      "2027-06-15",
				VenueName: "Test Venue",
				VenueURL:  "https://ra.co/venue/test",
				Hosts:     []ra.RAArtistRef{{Name: "DJ Host"}},
				Promo:     "An amazing night",
			},
		},
	}

	// Create handler - need to use actual Service type
	handler := &RAImportHandler{
		gigSvc:      mockGigSvc,
		venueSvc:    mockVenueSvc,
		contactSvc:  mockContactSvc,
		raClient:    mockRAClient,
	}

	// Create test request
	reqBody := RAImportRequest{
		ArtistSlug: "test-artist",
		DryRun:     false,
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/import-ra/import-ra", strings.NewReader(string(reqJSON)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// Call handler
	handler.HandleImportFromRA(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RAImportResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected success, got failure")
	}
	if result.GigsCreated != 1 {
		t.Errorf("expected 1 gig created, got %d", result.GigsCreated)
	}
	if len(result.GigIDs) != 1 {
		t.Errorf("expected 1 gig ID, got %d", len(result.GigIDs))
	}
	if len(mockVenueSvc.creates) != 1 {
		t.Errorf("expected 1 venue create, got %d", len(mockVenueSvc.creates))
	}
	if len(mockContactSvc.creates) != 1 {
		t.Errorf("expected 1 contact create, got %d", len(mockContactSvc.creates))
	}
}

// TestRAImportHandler_HandleImportFromRA_DryRun tests dry run mode
func TestRAImportHandler_HandleImportFromRA_DryRun(t *testing.T) {
	mockGigSvc := &mockGigServiceForTest{}
	mockVenueSvc := newMockVenueService()
	mockContactSvc := newMockContactService()
	mockRAClient := &mockRAClientForTest{
		artist: &ra.RAArtist{
			ID:   "123",
			Name: "Test Artist",
			Slug: "test-artist",
		},
		events: []ra.RAEVENT{
			{
				ID:        "evt1",
				Title:     "Test Event",
				Date:      "2027-06-15",
				VenueName: "Test Venue",
				VenueURL:  "https://ra.co/venue/test",
			},
		},
	}

	handler := &RAImportHandler{
		gigSvc:      mockGigSvc,
		venueSvc:    mockVenueSvc,
		contactSvc:  mockContactSvc,
		raClient:    mockRAClient,
	}

	reqBody := RAImportRequest{
		ArtistSlug: "test-artist",
		DryRun:     true,
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/import-ra/import-ra", strings.NewReader(string(reqJSON)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleImportFromRA(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var result RAImportResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.DryRun != true {
		t.Error("expected dry_run to be true")
	}
	if result.GigsCreated != 1 {
		t.Errorf("expected 1 gig created (dry run), got %d", result.GigsCreated)
	}
}

// TestRAImportHandler_HandleImportFromRA_SkipsPastEvents tests that past events are skipped
func TestRAImportHandler_HandleImportFromRA_SkipsPastEvents(t *testing.T) {
	mockGigSvc := &mockGigServiceForTest{}
	mockVenueSvc := newMockVenueService()
	mockContactSvc := newMockContactService()
	mockRAClient := &mockRAClientForTest{
		artist: &ra.RAArtist{
			ID:   "123",
			Name: "Test Artist",
			Slug: "test-artist",
		},
		events: []ra.RAEVENT{
			{
				ID:        "evt1",
				Title:     "Future Event",
				Date:      "2027-06-15",
				VenueName: "Test Venue",
				VenueURL:  "https://ra.co/venue/test",
			},
			{
				ID:        "evt2",
				Title:     "Past Event",
				Date:      "2020-01-01",
				VenueName: "Old Venue",
				VenueURL:  "https://ra.co/venue/old",
			},
		},
	}

	handler := &RAImportHandler{
		gigSvc:      mockGigSvc,
		venueSvc:    mockVenueSvc,
		contactSvc:  mockContactSvc,
		raClient:    mockRAClient,
	}

	reqBody := RAImportRequest{
		ArtistSlug: "test-artist",
		DryRun:     false,
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/import-ra/import-ra", strings.NewReader(string(reqJSON)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleImportFromRA(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var result RAImportResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.GigsCreated != 1 {
		t.Errorf("expected 1 future event imported, got %d", result.GigsCreated)
	}
	if result.GigsSkipped != 1 {
		t.Errorf("expected 1 past event skipped, got %d", result.GigsSkipped)
	}
	if len(result.SkippedReasons) != 1 {
		t.Errorf("expected 1 skip reason, got %d", len(result.SkippedReasons))
	}
	if !strings.Contains(result.SkippedReasons[0], "past event") {
		t.Errorf("expected skip reason to mention 'past event', got: %s", result.SkippedReasons[0])
	}
}

// TestRAImportHandler_HandleImportFromRA_EmptyArtistSlug tests validation
func TestRAImportHandler_HandleImportFromRA_EmptyArtistSlug(t *testing.T) {
	mockGigSvc := &mockGigServiceForTest{}
	mockVenueSvc := newMockVenueService()
	mockContactSvc := newMockContactService()
	mockRAClient := &mockRAClientForTest{}

	handler := &RAImportHandler{
		gigSvc:      mockGigSvc,
		venueSvc:    mockVenueSvc,
		contactSvc:  mockContactSvc,
		raClient:    mockRAClient,
	}

	req := httptest.NewRequest("POST", "/import-ra/import-ra", strings.NewReader(`{"artist_slug": ""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleImportFromRA(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var errResp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["error"] == "" {
		t.Error("expected error message")
	}
}

// mockGigServiceForTest implements gig.ServiceIface for testing
type mockGigServiceForTest struct{}

func (m *mockGigServiceForTest) CreateGig(ctx context.Context, req *GigCreate) (*Gig, error) {
	id := uuid.New()
	return &Gig{
		ID:               id,
		Date:             req.Date,
		Venue:            req.Venue,
		City:             req.City,
		Country:          req.Country,
		EventName:        req.EventName,
		PromoterName:     req.PromoterName,
		PromoterEmail:    req.PromoterEmail,
		PromoterPhone:    req.PromoterPhone,
		FeeAmount:        req.FeeAmount,
		FeeCurrency:      req.FeeCurrency,
		SetLengthMinutes: req.SetLengthMinutes,
		Notes:            req.Notes,
		Status:           GigStatusInquiry,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}, nil
}

func (m *mockGigServiceForTest) UpdateGig(ctx context.Context, id uuid.UUID, req *GigUpdate) (*Gig, error) {
	return &Gig{ID: id}, nil
}

func (m *mockGigServiceForTest) LinkVenue(ctx context.Context, gigID, venueID uuid.UUID, isPrimary bool) error {
	return nil
}

func (m *mockGigServiceForTest) LinkContact(ctx context.Context, gigID, contactID uuid.UUID, role string) error {
	return nil
}

func (m *mockGigServiceForTest) GetGig(ctx context.Context, id uuid.UUID) (*Gig, error) {
	return nil, nil
}

func (m *mockGigServiceForTest) ListGigs(ctx context.Context, f GigFilter) ([]*Gig, error) {
	return nil, nil
}

func (m *mockGigServiceForTest) DeleteGig(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockGigServiceForTest) GetGigDetail(ctx context.Context, id uuid.UUID) (*GigDetailResponse, error) {
	return nil, nil
}

func (m *mockGigServiceForTest) GenerateCalendar(ctx context.Context, config CalendarConfig) (string, error) {
	return "", nil
}

func (m *mockGigServiceForTest) GenerateBookingPDF(ctx context.Context, gigID uuid.UUID, djName string) ([]byte, string, error) {
	return nil, "", nil
}
func (m *mockGigServiceForTest) LinkTracklist(ctx context.Context, gigID, tracklistID uuid.UUID) error {
	return nil
}
func (m *mockGigServiceForTest) UnlinkTracklist(ctx context.Context, gigID, tracklistID uuid.UUID) error {
	return nil
}

// mockRAClientForTest implements a minimal RA client interface for testing
type mockRAClientForTest struct {
	artist *ra.RAArtist
	events []ra.RAEVENT
	err    error
}

func (m *mockRAClientForTest) GetArtist(ctx context.Context, slug string) (*ra.RAArtist, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.artist, nil
}

func (m *mockRAClientForTest) GetArtistEvents(ctx context.Context, artistID string, limit int) ([]ra.RAEVENT, error) {
	if m.err != nil {
		return nil, m.err
	}
	if limit > 0 && limit < len(m.events) {
		return m.events[:limit], nil
	}
	return m.events, nil
}

func (m *mockRAClientForTest) GetEvent(ctx context.Context, eventID string) (*ra.RAEVENT, error) {
	return nil, nil
}

func (m *mockRAClientForTest) GetVenue(ctx context.Context, venueID string) (*ra.RAVenue, error) {
	return nil, nil
}

func (m *mockRAClientForTest) SearchAreas(ctx context.Context, query string) ([]ra.RAAreaResult, error) {
	return nil, nil
}

func (m *mockRAClientForTest) GetAreas(ctx context.Context, country string) ([]ra.RAAreaResult, error) {
	return nil, nil
}
