package gig

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/ra"
)

// listingGigService is mockGigServiceForTest with a controllable ListGigs,
// used to exercise duplicate detection.
type listingGigService struct {
	mockGigServiceForTest
	existing []*Gig
	created  int
}

func (m *listingGigService) ListGigs(ctx context.Context, f GigFilter) ([]*Gig, error) {
	return m.existing, nil
}

func (m *listingGigService) CreateGig(ctx context.Context, req *GigCreate) (*Gig, error) {
	m.created++
	return m.mockGigServiceForTest.CreateGig(ctx, req)
}

func runRAImport(t *testing.T, h *RAImportHandler, body RAImportRequest) (int, RAImportResult) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/import-ra", strings.NewReader(string(raw)))
	rec := httptest.NewRecorder()
	h.HandleImportFromRA(rec, req)

	var result RAImportResult
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	return rec.Code, result
}

func futureEvents() []ra.RAEVENT {
	return []ra.RAEVENT{
		{ID: "evt1", Title: "Warehouse Night", Date: "2099-06-15", VenueName: "Venue One",
			Hosts: []ra.RAArtistRef{{ArtistID: "p1", Name: "Night Promotions"}}},
		{ID: "evt2", Title: "Open Air", Date: "2099-07-20", VenueName: "Venue Two"},
	}
}

func newBehaviourHandler(gigSvc ServiceIface, events []ra.RAEVENT) (*RAImportHandler, *mockVenueService, *mockContactService) {
	venueSvc := newMockVenueService()
	contactSvc := newMockContactService()
	return &RAImportHandler{
		gigSvc:     gigSvc,
		venueSvc:   venueSvc,
		contactSvc: contactSvc,
		raClient:   &mockRAClientForTest{artist: &ra.RAArtist{ID: "123", Slug: "test-artist"}, events: events},
	}, venueSvc, contactSvc
}

// A dry run is what the FETCH button sends. It must preview, not persist:
// venue and contact creation used to run before the dry-run check.
func TestRAImport_DryRunWritesNothingAndReturnsPreview(t *testing.T) {
	gigSvc := &listingGigService{}
	h, venueSvc, contactSvc := newBehaviourHandler(gigSvc, futureEvents())

	code, result := runRAImport(t, h, RAImportRequest{ArtistSlug: "test-artist", DryRun: true})

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if gigSvc.created != 0 || len(venueSvc.creates) != 0 || len(contactSvc.creates) != 0 {
		t.Fatalf("dry run wrote data: gigs=%d venues=%d contacts=%d",
			gigSvc.created, len(venueSvc.creates), len(contactSvc.creates))
	}
	if len(result.Events) != 2 || result.Events[0].Title != "Warehouse Night" {
		t.Fatalf("expected both events in the preview, got %+v", result.Events)
	}
	if result.GigsCreated != 2 || len(result.GigIDs) != 0 {
		t.Fatalf("expected would-create count 2 and no gig IDs, got %d / %v", result.GigsCreated, result.GigIDs)
	}
}

func TestRAImport_OnlyImportsSelectedEvents(t *testing.T) {
	gigSvc := &listingGigService{}
	h, _, _ := newBehaviourHandler(gigSvc, futureEvents())

	_, result := runRAImport(t, h, RAImportRequest{ArtistSlug: "test-artist", EventIDs: []string{"evt2"}})

	if gigSvc.created != 1 || len(result.Events) != 1 || result.Events[0].ID != "evt2" {
		t.Fatalf("expected only evt2 imported, created=%d events=%+v", gigSvc.created, result.Events)
	}
	if result.GigsSkipped != 0 {
		t.Fatalf("unselected events must not count as skipped, got %d", result.GigsSkipped)
	}
}

func TestRAImport_SkipsGigsAlreadyTracked(t *testing.T) {
	date, _ := time.Parse("2006-01-02", "2099-06-15")
	gigSvc := &listingGigService{existing: []*Gig{{Date: date, EventName: "warehouse night "}}}
	h, _, _ := newBehaviourHandler(gigSvc, futureEvents())

	_, result := runRAImport(t, h, RAImportRequest{ArtistSlug: "test-artist"})

	if gigSvc.created != 1 {
		t.Fatalf("expected only the new event to be created, got %d", gigSvc.created)
	}
	if len(result.SkippedReasons) != 1 || !strings.Contains(result.SkippedReasons[0], "already in your gigs") {
		t.Fatalf("expected a duplicate skip reason, got %v", result.SkippedReasons)
	}
}

// RA names a promoter for evt1 only. evt2 must not produce a blank contact.
func TestRAImport_NoBlankContacts(t *testing.T) {
	gigSvc := &listingGigService{}
	h, _, contactSvc := newBehaviourHandler(gigSvc, futureEvents())

	runRAImport(t, h, RAImportRequest{ArtistSlug: "test-artist"})

	if len(contactSvc.creates) != 1 || contactSvc.creates[0].Name != "Night Promotions" {
		names := []string{}
		for _, c := range contactSvc.creates {
			names = append(names, c.Name)
		}
		t.Fatalf("expected exactly one contact 'Night Promotions', got %q", names)
	}
}

func TestRAImport_UpstreamErrorsAreNotReportedAsNotFound(t *testing.T) {
	h, _, _ := newBehaviourHandler(&listingGigService{}, nil)

	h.raClient = &mockRAClientForTest{err: ra.ErrArtistNotFound}
	if code, _ := runRAImport(t, h, RAImportRequest{ArtistSlug: "nobody"}); code != http.StatusNotFound {
		t.Fatalf("missing artist: expected 404, got %d", code)
	}

	h.raClient = &mockRAClientForTest{err: context.DeadlineExceeded}
	if code, _ := runRAImport(t, h, RAImportRequest{ArtistSlug: "anyone"}); code != http.StatusBadGateway {
		t.Fatalf("RA unreachable: expected 502, got %d", code)
	}
}
