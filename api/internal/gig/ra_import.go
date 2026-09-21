package gig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/ra"
	"github.com/shopspring/decimal"

	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/venue"
)

// RAImportRequest represents an RA event import request
type RAImportRequest struct {
	ArtistSlug      string `json:"artist_slug"`      // RA artist slug to import events for
	VenueOverride   string `json:"venue_override"`     // Optional: force a specific venue name
	ContactOverride string `json:"contact_override"`   // Optional: force a specific contact name
	DryRun           bool   `json:"dry_run"`           // If true, don't create anything, just return what would be created
	// Optional: only import these RA event IDs (the user's selection from a
	// dry-run preview). Empty means every importable event.
	EventIDs []string `json:"event_ids"`
}

// RAImportResult represents the result of an RA import
type RAImportResult struct {
	Success        bool        `json:"success"`
	ArtistSlug     string      `json:"artist_slug"`
	EventsImported int         `json:"events_imported"`
	EventsSkipped  int         `json:"events_skipped"`
	GigsCreated    int         `json:"gigs_created"`
	GigsSkipped    int         `json:"gigs_skipped"`
	GigIDs         []uuid.UUID `json:"gig_ids"`
	SkippedReasons []string    `json:"skipped_reasons"`
	DryRun          bool        `json:"dry_run"`
	// Events that passed the import filters. A dry run returns them so the
	// UI can show a preview and let the user pick which ones to import.
	Events []ra.RAEVENT `json:"events"`
}

// RAImportHandler handles RA event imports
type RAImportHandler struct {
	gigSvc      ServiceIface
	venueSvc    venue.ServiceIface
	contactSvc   contact.ServiceIface
	raClient    RAArtistLoader
}

// RAArtistLoader defines the interface for loading RA artist data
type RAArtistLoader interface {
	GetArtist(ctx context.Context, slug string) (*ra.RAArtist, error)
	GetArtistEvents(ctx context.Context, artistID string, limit int) ([]ra.RAEVENT, error)
}

// NewRAImportHandler creates a new RA import handler
func NewRAImportHandler(gigSvc ServiceIface, venueSvc venue.ServiceIface, contactSvc contact.ServiceIface, raClient RAArtistLoader) *RAImportHandler {
	return &RAImportHandler{
		gigSvc:      gigSvc,
		venueSvc:    venueSvc,
		contactSvc:  contactSvc,
		raClient:    raClient,
	}
}

// writeJSON writes a JSON response with the given status code and data
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response with the given status code and message
func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

// Routes returns an http.Handler with RA import routes
func (h *RAImportHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/import-ra", h.HandleImportFromRA)
	r.Get("/info/{artistSlug}", h.GetArtistInfo)
	return r
}

// HandleImportFromRA handles POST /api/v1/gigs/import-ra
func (h *RAImportHandler) HandleImportFromRA(w http.ResponseWriter, r *http.Request) {
	var req RAImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.ArtistSlug == "" {
		writeError(w, http.StatusBadRequest, "artist_slug is required")
		return
	}

	ctx := r.Context()
	gigIDs := []uuid.UUID{}
	skippedReasons := []string{}
	importable := []ra.RAEVENT{}
	gigsCreated := 0
	gigsSkipped := 0

	// Fetch artist from RA
	artist, err := h.raClient.GetArtist(ctx, req.ArtistSlug)
	if errors.Is(err, ra.ErrArtistNotFound) {
		writeError(w, http.StatusNotFound, "artist not found: "+req.ArtistSlug)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not reach Resident Advisor: "+err.Error())
		return
	}

	events, err := h.raClient.GetArtistEvents(ctx, artist.ID, 50)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not fetch events from Resident Advisor: "+err.Error())
		return
	}

	selected := map[string]bool{}
	for _, id := range req.EventIDs {
		selected[id] = true
	}

	// Gigs already tracked, keyed by date + event name, so importing the
	// same RA calendar twice does not duplicate every gig.
	existing := map[string]bool{}
	if gigs, err := h.gigSvc.ListGigs(ctx, GigFilter{}); err == nil {
		for _, g := range gigs {
			existing[gigKey(g.Date, g.EventName)] = true
		}
	}

	for _, raEvent := range events {
		if len(selected) > 0 && !selected[raEvent.ID] {
			continue // not part of the user's selection; not a "skip"
		}

		eventDate, err := time.Parse("2006-01-02", raEvent.Date)
		if err != nil {
			skippedReasons = append(skippedReasons, fmt.Sprintf("skipped event with invalid date '%s': %s", raEvent.Date, raEvent.Title))
			gigsSkipped++
			continue
		}

		// Skip past events (more than 7 days ago) for import
		if eventDate.Before(time.Now().AddDate(0, 0, -7)) {
			skippedReasons = append(skippedReasons, fmt.Sprintf("skipped past event: %s on %s", raEvent.Title, raEvent.Date))
			gigsSkipped++
			continue
		}

		if raEvent.VenueName == "" {
			skippedReasons = append(skippedReasons, fmt.Sprintf("skipped event without venue: %s", raEvent.Title))
			gigsSkipped++
			continue
		}

		if existing[gigKey(eventDate, raEvent.Title)] {
			skippedReasons = append(skippedReasons, fmt.Sprintf("already in your gigs: %s on %s", raEvent.Title, raEvent.Date))
			gigsSkipped++
			continue
		}

		importable = append(importable, raEvent)

		// A dry run must not write anything. Venue and contact creation used
		// to run before this check, so every preview left rows behind.
		if req.DryRun {
			gigsCreated++
			continue
		}

		venueID, err := h.findOrCreateVenue(ctx, &req, &raEvent)
		if err != nil {
			skippedReasons = append(skippedReasons, fmt.Sprintf("failed to create venue '%s': %s", raEvent.VenueName, err.Error()))
			gigsSkipped++
			continue
		}

		promoterName := ""
		if len(raEvent.Hosts) > 0 {
			promoterName = raEvent.Hosts[0].Name
		}

		gig, err := h.gigSvc.CreateGig(ctx, &GigCreate{
			Date:         eventDate,
			Venue:        raEvent.VenueName,
			EventName:    raEvent.Title,
			PromoterName: promoterName,
			// Status defaults to inquiry per model
			FeeAmount: decimal.NewFromFloat(0),
			Notes:     raEvent.Promo,
		})
		if err != nil {
			skippedReasons = append(skippedReasons, fmt.Sprintf("failed to create gig '%s': %s", raEvent.Title, err.Error()))
			gigsSkipped++
			continue
		}
		gigsCreated++
		gigIDs = append(gigIDs, gig.ID)
		existing[gigKey(eventDate, raEvent.Title)] = true

		// Only link a contact when RA names a promoter (or one was forced);
		// otherwise every event produced a blank-named contact.
		contactID, hasContact, err := h.findOrCreateContact(ctx, &req, promoterName)
		if err != nil {
			skippedReasons = append(skippedReasons, fmt.Sprintf("failed to create contact '%s': %s", promoterName, err.Error()))
		}

		gigUpdate := GigUpdate{
			GigReaderVenueID: &venueID,
			// Optimistic concurrency matches on the stored updated_at;
			// time.Now() never matched, so these IDs were never saved.
			UpdatedAt: gig.UpdatedAt,
		}
		if hasContact {
			gigUpdate.GigReaderContactID = &contactID
		}
		if _, err = h.gigSvc.UpdateGig(ctx, gig.ID, &gigUpdate); err != nil {
			skippedReasons = append(skippedReasons, fmt.Sprintf("failed to update gig with venue/contact: %s", err.Error()))
		}

		if err := h.gigSvc.LinkVenue(ctx, gig.ID, venueID, true); err != nil {
			skippedReasons = append(skippedReasons, fmt.Sprintf("failed to link venue to gig: %s", err.Error()))
		}
		if hasContact {
			if err := h.gigSvc.LinkContact(ctx, gig.ID, contactID, "promoter"); err != nil {
				skippedReasons = append(skippedReasons, fmt.Sprintf("failed to link contact to gig: %s", err.Error()))
			}
		}
	}

	result := RAImportResult{
		Success:        true,
		ArtistSlug:     req.ArtistSlug,
		EventsImported: len(events),
		EventsSkipped:  gigsSkipped,
		GigsCreated:    gigsCreated,
		GigsSkipped:    gigsSkipped,
		GigIDs:         gigIDs,
		SkippedReasons: skippedReasons,
		DryRun:         req.DryRun,
		Events:         importable,
	}

	writeJSON(w, http.StatusOK, result)
}

// gigKey identifies a gig for duplicate detection during import.
func gigKey(date time.Time, eventName string) string {
	return date.Format("2006-01-02") + "|" + strings.ToLower(strings.TrimSpace(eventName))
}

// findOrCreateVenue resolves the venue for an RA event: the override if one
// was given, else an existing venue matching the RA name, else a new venue.
func (h *RAImportHandler) findOrCreateVenue(ctx context.Context, req *RAImportRequest, raEvent *ra.RAEVENT) (uuid.UUID, error) {
	if req.VenueOverride != "" {
		venues, _ := h.venueSvc.Autocomplete(ctx, req.VenueOverride, 5)
		for _, v := range venues {
			if strings.EqualFold(v.Name, req.VenueOverride) {
				return v.ID, nil
			}
		}
		if len(venues) > 0 {
			return venues[0].ID, nil
		}
	}

	if venues, _ := h.venueSvc.Autocomplete(ctx, raEvent.VenueName, 5); len(venues) > 0 {
		return venues[0].ID, nil
	}

	v, err := h.venueSvc.CreateVenue(ctx, &venue.VenueCreate{
		Name:    raEvent.VenueName,
		Website: &raEvent.VenueURL,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return v.ID, nil
}

// findOrCreateContact resolves the promoter contact. It reports hasContact
// false when there is no name to go on, so no blank contact is created.
func (h *RAImportHandler) findOrCreateContact(ctx context.Context, req *RAImportRequest, promoterName string) (id uuid.UUID, hasContact bool, err error) {
	if req.ContactOverride != "" {
		contacts, _ := h.contactSvc.Autocomplete(ctx, req.ContactOverride, 5)
		for _, c := range contacts {
			if strings.EqualFold(c.Name, req.ContactOverride) {
				return c.ID, true, nil
			}
		}
		if len(contacts) > 0 {
			return contacts[0].ID, true, nil
		}
	}

	if promoterName == "" {
		return uuid.Nil, false, nil
	}

	if contacts, _ := h.contactSvc.Autocomplete(ctx, promoterName, 5); len(contacts) > 0 {
		return contacts[0].ID, true, nil
	}

	c, err := h.contactSvc.CreateContact(ctx, &contact.ContactCreate{
		Name: promoterName,
		Type: contact.ContactTypePromoter,
	})
	if err != nil {
		return uuid.Nil, false, err
	}
	return c.ID, true, nil
}

// GetArtistInfo returns basic artist info from RA
func (h *RAImportHandler) GetArtistInfo(w http.ResponseWriter, r *http.Request) {
	artistSlug := chi.URLParam(r, "artistSlug")
	if artistSlug == "" {
		writeError(w, http.StatusBadRequest, "artist slug is required")
		return
	}

	ctx := r.Context()
	artist, err := h.raClient.GetArtist(ctx, artistSlug)
	if errors.Is(err, ra.ErrArtistNotFound) {
		writeError(w, http.StatusNotFound, "artist not found: "+artistSlug)
		return
	}
	if err != nil {
		// Reporting every failure as "not found" hid an RA block and a
		// stale query behind a message that blamed the user's slug.
		writeError(w, http.StatusBadGateway, "could not reach Resident Advisor: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"id":            artist.ID,
			"name":          artist.Name,
			"slug":          artist.Slug,
			"url":           artist.URL,
			"followers":     artist.Followers,
			"biography":     artist.Biography.Blurb,
			"instagram":     artist.Instagram,
			"soundcloud":    artist.Soundcloud,
			"bandcamp":      artist.Bandcamp,
			"discogs":       artist.Discogs,
			"website":       artist.Website,
			"facebook":      artist.Facebook,
			"twitter":       artist.Twitter,
			"headerImage":   artist.HeaderImage,
			"profileImage":  artist.ProfileImage,
		},
	})
}
