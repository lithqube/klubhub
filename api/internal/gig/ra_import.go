package gig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	success := true
	gigIDs := []uuid.UUID{}
	skippedReasons := []string{}
	gigsCreated := 0
	gigsSkipped := 0

	// Fetch artist from RA
	artist, err := h.raClient.GetArtist(ctx, req.ArtistSlug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch artist from RA: "+err.Error())
		return
	}
	_ = artist // Artist fetched successfully, use for events retrieval

		// Get artist events from RA — use artist.ID since GetArtistEvents takes an artist ID
		events, err := h.raClient.GetArtistEvents(ctx, strconv.Itoa(artist.ID), 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch events from RA: "+err.Error())
			return
		}

	// Process each event
	for _, raEvent := range events {
		// Parse date from RA (ISO format YYYY-MM-DD)
		eventDate, err := time.Parse("2006-01-02", raEvent.Date)
		if err != nil {
			skippedReasons = append(skippedReasons, fmt.Sprintf("skipped event with invalid date '%s': %s", raEvent.Date, raEvent.Title))
			gigsSkipped++
			continue
		}

		// Skip past events (more than 7 days ago) for import
		if eventDate.Before(time.Now().AddDate(0, 0, -7)) {
			skippedReasons = append(skippedReasons, fmt.Sprintf("skipped past event: %s on %s",
				raEvent.Title, raEvent.Date))
			gigsSkipped++
			continue
		}

		// Skip events without a venue name
		if raEvent.VenueName == "" {
			skippedReasons = append(skippedReasons, fmt.Sprintf("skipped event without venue: %s", raEvent.Title))
			gigsSkipped++
			continue
		}

		// Find or create venue
		var venueID uuid.UUID
		var venueExists bool

		// Try to find existing venue by name (using override if provided)
		if req.VenueOverride != "" {
			venues, _ := h.venueSvc.Autocomplete(ctx, req.VenueOverride, 5)
			if len(venues) > 0 {
				// Check for exact match first
				for _, v := range venues {
					if strings.EqualFold(v.Name, req.VenueOverride) {
						venueID = v.ID
						venueExists = true
						break
					}
				}
				// If no exact match, use first close match
				if !venueExists && len(venues) > 0 {
					venueID = venues[0].ID
					venueExists = true
				}
			}
		}

		if !venueExists {
			// Search by venue name from RA
			venues, _ := h.venueSvc.Autocomplete(ctx, raEvent.VenueName, 5)
			if len(venues) > 0 {
				// Use first match
				venueID = venues[0].ID
				venueExists = true
			}
		}

		if !venueExists {
			// Create new venue from RA event
			venueID = uuid.New()
			newVenue := venue.VenueCreate{
				Name:      raEvent.VenueName,
				City:      "", // RA doesn't provide city in event listing
				Country:   "",
				Website:   &raEvent.VenueURL,
				Notes:     "",
				TechContactName:   "",
				TechContactEmail:  "",
				TechContactPhone:  "",
			}
			v, err := h.venueSvc.CreateVenue(ctx, &newVenue)
			if err != nil {
				skippedReasons = append(skippedReasons, fmt.Sprintf("failed to create venue '%s': %s",
					raEvent.VenueName, err.Error()))
				gigsSkipped++
				continue
			}
			venueID = v.ID
		}

		// Find or create contact (promoter)
		var contactID uuid.UUID
		var contactExists bool

		// Try to find existing contact by name (promoter)
		if req.ContactOverride != "" {
			contacts, _ := h.contactSvc.Autocomplete(ctx, req.ContactOverride, 5)
			if len(contacts) > 0 {
				for _, c := range contacts {
					if strings.EqualFold(c.Name, req.ContactOverride) {
						contactID = c.ID
						contactExists = true
						break
					}
				}
				if !contactExists && len(contacts) > 0 {
					contactID = contacts[0].ID
					contactExists = true
				}
			}
		}

		if !contactExists && raEvent.Hosts != nil && len(raEvent.Hosts) > 0 {
			// Use first host as promoter
			hostName := raEvent.Hosts[0].Name
			contacts, _ := h.contactSvc.Autocomplete(ctx, hostName, 5)
			if len(contacts) > 0 {
				contactID = contacts[0].ID
				contactExists = true
			}
		}

		if !contactExists {
			// Create new contact from event hosts or leave empty
			contactID = uuid.New()
			promoterName := ""
			if raEvent.Hosts != nil && len(raEvent.Hosts) > 0 {
				promoterName = raEvent.Hosts[0].Name
			}
			newContact := contact.ContactCreate{
				Name:    promoterName,
				Email:   "",
				Phone:   "",
				Type:    contact.ContactTypePromoter,
				Company: nil,
				Notes:   "",
			}
			c, err := h.contactSvc.CreateContact(ctx, &newContact)
			if err != nil {
				skippedReasons = append(skippedReasons, fmt.Sprintf("failed to create contact '%s': %s",
					promoterName, err.Error()))
				gigsSkipped++
				continue
			}
			contactID = c.ID
		}

		// Prepare gig data
		gigCreate := GigCreate{
			Date:           eventDate,
			Venue:           raEvent.VenueName,
			City:            "",
			Country:         "",
			EventName:       raEvent.Title,
			PromoterName:    "", // RA doesn't provide promoter name in events
			PromoterEmail:   "",
			PromoterPhone:   "",
			// Status defaults to inquiry per model
			FeeAmount:        decimal.NewFromFloat(0),
			FeeCurrency:      "",
			SetLengthMinutes: 0,
			Notes:            raEvent.Promo, // Use promo as notes
		}

		if req.DryRun {
			// Don't create, just count
			gigsCreated++
			gigIDs = append(gigIDs, uuid.New()) // placeholder ID for dry run
		} else {
			// Create the gig
			gig, err := h.gigSvc.CreateGig(ctx, &gigCreate)
			if err != nil {
				skippedReasons = append(skippedReasons, fmt.Sprintf("failed to create gig '%s': %s",
					raEvent.Title, err.Error()))
				gigsSkipped++
				continue
			}
			gigsCreated++
			gigIDs = append(gigIDs, gig.ID)

			// Update gig with RA venue/contact IDs
			gigUpdate := GigUpdate{
				GigReaderVenueID:   &venueID,
				GigReaderContactID: &contactID,
				UpdatedAt:          time.Now(),
			}
			_, err = h.gigSvc.UpdateGig(ctx, gig.ID, &gigUpdate)
			if err != nil {
				skippedReasons = append(skippedReasons, fmt.Sprintf("failed to update gig with venue/contact: %s", err.Error()))
			}

			// Link venue to gig
			if err := h.gigSvc.LinkVenue(ctx, gig.ID, venueID, true); err != nil {
				// Non-fatal: log but continue
				skippedReasons = append(skippedReasons, fmt.Sprintf("failed to link venue to gig: %s", err.Error()))
			}

			// Link contact to gig
			if err := h.gigSvc.LinkContact(ctx, gig.ID, contactID, "promoter"); err != nil {
				// Non-fatal: log but continue
				skippedReasons = append(skippedReasons, fmt.Sprintf("failed to link contact to gig: %s", err.Error()))
			}
		}
	}

	// Build response
	result := RAImportResult{
		Success:        success,
		ArtistSlug:     req.ArtistSlug,
		EventsImported: len(events),
		EventsSkipped:  gigsSkipped,
		GigsCreated:    gigsCreated,
		GigsSkipped:    gigsSkipped,
		GigIDs:         gigIDs,
		SkippedReasons: skippedReasons,
		DryRun:         req.DryRun,
	}

	writeJSON(w, http.StatusOK, result)
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
	if err != nil {
		writeError(w, http.StatusNotFound, "artist not found: "+artistSlug)
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
