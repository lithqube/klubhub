package gig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Handler handles HTTP requests for the gig package.
type Handler struct {
	svc ServiceIface
}

// ServiceIface defines the interface exposed by the gig service to the HTTP layer.
type ServiceIface interface {
	CreateGig(ctx context.Context, req *GigCreate) (*Gig, error)
	GetGig(ctx context.Context, id uuid.UUID) (*Gig, error)
	ListGigs(ctx context.Context, f GigFilter) ([]*Gig, error)
	UpdateGig(ctx context.Context, id uuid.UUID, req *GigUpdate) (*Gig, error)
	DeleteGig(ctx context.Context, id uuid.UUID) error
	LinkVenue(ctx context.Context, gigID, venueID uuid.UUID, isPrimary bool) error
	LinkContact(ctx context.Context, gigID, contactID uuid.UUID, role string) error
	GenerateCalendar(ctx context.Context, config CalendarConfig) (string, error)
	GenerateBookingPDF(ctx context.Context, gigID uuid.UUID, djName string) ([]byte, string, error)
}

// NewHandler creates a Handler backed by the given service.
func NewHandler(svc ServiceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns an http.Handler with all gig routes registered.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/", h.handleListGigs)
	r.Post("/", h.handleCreateGig)
	r.Get("/autocomplete", h.handleAutocomplete)
	r.Get("/calendar.ics", h.handleCalendarICS)
	r.Get("/{id}", h.handleGetGig)
	r.Put("/{id}", h.handleUpdateGig)
	r.Delete("/{id}", h.handleDeleteGig)
	r.Post("/{id}/link-venue", h.handleLinkVenue)
	r.Post("/{id}/link-contact", h.handleLinkContact)
	r.Get("/{id}/pdf", h.handleGetBookingPDF)

	return r
}

// handleListGigs returns gigs with optional query filters.
func (h *Handler) handleListGigs(w http.ResponseWriter, r *http.Request) {
	f := GigFilter{}

	if status := r.URL.Query().Get("status"); status != "" {
		s := GigStatus(status)
		f.Status = &s
	}
	if venueID := r.URL.Query().Get("venue_id"); venueID != "" {
		if id, err := uuid.Parse(venueID); err == nil {
			f.VenueID = &id
		}
	}
	if city := r.URL.Query().Get("city"); city != "" {
		f.City = &city
	}
	if feeMin := r.URL.Query().Get("fee_min"); feeMin != "" {
		if amt, err := decimal.NewFromString(feeMin); err == nil {
			f.FeeMin = &amt
		}
	}
	if feeMax := r.URL.Query().Get("fee_max"); feeMax != "" {
		if amt, err := decimal.NewFromString(feeMax); err == nil {
			f.FeeMax = &amt
		}
	}
	if from := r.URL.Query().Get("from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			f.From = &t
		}
	}
	if to := r.URL.Query().Get("to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			f.To = &t
		}
	}

	gigs, err := h.svc.ListGigs(r.Context(), f)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, gigs)
}

// handleCreateGig creates a new gig.
func (h *Handler) handleCreateGig(w http.ResponseWriter, r *http.Request) {
	var body GigCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	gig, err := h.svc.CreateGig(r.Context(), &body)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusCreated, gig)
}

// handleGetGig returns a single gig by ID.
func (h *Handler) handleGetGig(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid gig id")
		return
	}

	gig, err := h.svc.GetGig(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "gig not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, gig)
}

// handleUpdateGig updates a gig.
func (h *Handler) handleUpdateGig(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid gig id")
		return
	}

	var body GigUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	gig, err := h.svc.UpdateGig(r.Context(), id, &body)
	if errors.Is(err, ErrConflict) {
		h.writeError(w, http.StatusConflict, "conflict: gig was modified by another request")
		return
	}
	if errors.Is(err, ErrForbidden) {
		h.writeError(w, http.StatusForbidden, "cannot change status of cancelled gig")
		return
	}
	if errors.Is(err, ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "gig not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, gig)
}

// handleDeleteGig soft-deletes a gig.
func (h *Handler) handleDeleteGig(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid gig id")
		return
	}

	if err := h.svc.DeleteGig(r.Context(), id); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleLinkVenue links a gig to a venue.
func (h *Handler) handleLinkVenue(w http.ResponseWriter, r *http.Request) {
	gigID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid gig id")
		return
	}

	var body struct {
		VenueID   string `json:"venue_id"`
		IsPrimary bool   `json:"is_primary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	venueID, err := uuid.Parse(body.VenueID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid venue_id")
		return
	}

	if err := h.svc.LinkVenue(r.Context(), gigID, venueID, body.IsPrimary); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleLinkContact links a gig to a contact.
func (h *Handler) handleLinkContact(w http.ResponseWriter, r *http.Request) {
	gigID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid gig id")
		return
	}

	var body struct {
		ContactID string `json:"contact_id"`
		Role      string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	contactID, err := uuid.Parse(body.ContactID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid contact_id")
		return
	}

	if err := h.svc.LinkContact(r.Context(), gigID, contactID, body.Role); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAutocomplete returns gigs for "Copy from Previous Gig" dropdown.
func (h *Handler) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		h.writeJSON(w, http.StatusOK, []*Gig{})
		return
	}

	// Filter by query - gigs matching venue or event name (case-insensitive)
	f := GigFilter{}
	nameFilter := "%" + q + "%"
	f.City = &nameFilter

	gigs, err := h.svc.ListGigs(r.Context(), f)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, gigs)
}

// handleCalendarICS returns an RFC 5545 compliant iCal feed.
func (h *Handler) handleCalendarICS(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	if !ValidateSecret(secret, "ICAL_SECRET") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	calendar, err := h.svc.GenerateCalendar(r.Context(), CalendarConfig{Secret: secret})
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=klubhub-gigs.ics")
	_, _ = w.Write([]byte(calendar))
}

// handleGetBookingPDF returns a booking confirmation PDF for a confirmed gig.
func (h *Handler) handleGetBookingPDF(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	if !ValidateSecret(secret, "ICAL_SECRET") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid gig id")
		return
	}

	pdfData, storagePath, err := h.svc.GenerateBookingPDF(r.Context(), id, "DJ Name")
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=booking-confirmation.pdf")
	_, _ = w.Write(pdfData)
	_ = storagePath // TODO: return storage path or presigned URL
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
