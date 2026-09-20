package epk

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/klubhub/dj/api/internal/ra"
)

// RAImportHandler wraps the EPK handler with RA import capability
type RAImportHandler struct {
	epkHandler *Handler
	raClient   *ra.RAClient
}

// NewRAImportHandler creates a new RA import handler
func NewRAImportHandler(epkHandler *Handler, raClient *ra.RAClient) *RAImportHandler {
	return &RAImportHandler{
		epkHandler: epkHandler,
		raClient:   raClient,
	}
}

// Routes returns an http.Handler with the RA import route
func (h *RAImportHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/import-ra", h.handleImportFromRA)
	return r
}

// RAImportResult represents the result of an RA import
type RAImportResult struct {
	Success        bool   `json:"success"`
	ArtistName     string `json:"artistName"`
	Biography      string `json:"biography"`
	Instagram      string `json:"instagram,omitempty"`
	Soundcloud     string `json:"soundcloud,omitempty"`
	Bandcamp       string `json:"bandcamp,omitempty"`
	Discogs        string `json:"discogs,omitempty"`
	Website        string `json:"website,omitempty"`
	Facebook       string `json:"facebook,omitempty"`
	Twitter        string `json:"twitter,omitempty"`
	Followers      int    `json:"followers"`
	HeaderImage    string `json:"headerImage,omitempty"`
	ProfileImage   string `json:"profileImage,omitempty"`
	NeedsUpdate    bool   `json:"needsUpdate"` // true if EPK content needs to be updated with this data
}

// handleImportFromRA handles POST /api/v1/epk/import-ra
func (h *RAImportHandler) handleImportFromRA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ArtistSlug string `json:"artist_slug"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.epkHandler.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if body.ArtistSlug == "" {
		h.epkHandler.writeError(w, http.StatusBadRequest, "artist_slug is required")
		return
	}

	// Fetch artist from RA
	artist, err := h.raClient.GetArtist(r.Context(), body.ArtistSlug)
	if err != nil {
		h.epkHandler.writeError(w, http.StatusBadRequest, "failed to fetch artist from RA: "+err.Error())
		return
	}

	// Build result
	result := RAImportResult{
		Success:      true,
		ArtistName:   artist.Name,
		Biography:    artist.Biography.Blurb,
		Instagram:    artist.Instagram,
		Soundcloud:   artist.Soundcloud,
		Bandcamp:     artist.Bandcamp,
		Discogs:      artist.Discogs,
		Website:      artist.Website,
		Facebook:     artist.Facebook,
		Twitter:      artist.Twitter,
		Followers:    artist.Followers,
		HeaderImage:  artist.HeaderImage,
		ProfileImage: artist.ProfileImage,
		NeedsUpdate:  true,
	}

	h.epkHandler.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": result,
	})
}
