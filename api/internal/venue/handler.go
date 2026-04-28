package venue

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for the venue package.
type Handler struct {
	svc ServiceIface
}

// ServiceIface defines the interface exposed by the venue service to the HTTP layer.
type ServiceIface interface {
	CreateVenue(ctx context.Context, req *VenueCreate) (*Venue, error)
	GetVenue(ctx context.Context, id uuid.UUID) (*Venue, error)
	ListVenues(ctx context.Context, name, city *string) ([]*Venue, error)
	UpdateVenue(ctx context.Context, id uuid.UUID, req *VenueUpdate) (*Venue, error)
	DeleteVenue(ctx context.Context, id uuid.UUID) error
	Autocomplete(ctx context.Context, query string, limit int) ([]*Venue, error)
}

// NewHandler creates a Handler backed by the given service.
func NewHandler(svc ServiceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns an http.Handler with all venue routes registered.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/", h.handleListVenues)
	r.Post("/", h.handleCreateVenue)
	r.Get("/{id}", h.handleGetVenue)
	r.Put("/{id}", h.handleUpdateVenue)
	r.Delete("/{id}", h.handleDeleteVenue)
	r.Get("/autocomplete", h.handleAutocomplete)

	return r
}

// handleListVenues returns venues with optional query filters.
func (h *Handler) handleListVenues(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	city := r.URL.Query().Get("city")

	var namePtr, cityPtr *string
	if name != "" {
		namePtr = &name
	}
	if city != "" {
		cityPtr = &city
	}

	venues, err := h.svc.ListVenues(r.Context(), namePtr, cityPtr)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, venues)
}

// handleCreateVenue creates a new venue.
func (h *Handler) handleCreateVenue(w http.ResponseWriter, r *http.Request) {
	var body VenueCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	venue, err := h.svc.CreateVenue(r.Context(), &body)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusCreated, venue)
}

// handleGetVenue returns a single venue by ID.
func (h *Handler) handleGetVenue(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid venue id")
		return
	}

	venue, err := h.svc.GetVenue(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "venue not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, venue)
}

// handleUpdateVenue updates a venue.
func (h *Handler) handleUpdateVenue(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid venue id")
		return
	}

	var body VenueUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	venue, err := h.svc.UpdateVenue(r.Context(), id, &body)
	if errors.Is(err, ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "venue not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, venue)
}

// handleDeleteVenue soft-deletes a venue.
func (h *Handler) handleDeleteVenue(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid venue id")
		return
	}

	if err := h.svc.DeleteVenue(r.Context(), id); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAutocomplete returns venues for autocomplete.
func (h *Handler) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		h.writeJSON(w, http.StatusOK, []*Venue{})
		return
	}

	venues, err := h.svc.Autocomplete(r.Context(), q, 5)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, venues)
}

// ─── Response helpers ─────────────────────────────────────────────────────────

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}
