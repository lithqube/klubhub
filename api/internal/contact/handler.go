package contact

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for the contact package.
type Handler struct {
	svc ServiceIface
}

// ServiceIface defines the interface exposed by the contact service to the HTTP layer.
type ServiceIface interface {
	CreateContact(ctx context.Context, req *ContactCreate) (*Contact, error)
	GetContact(ctx context.Context, id uuid.UUID) (*Contact, error)
	ListContacts(ctx context.Context, name *string, contactType *ContactType) ([]*Contact, error)
	UpdateContact(ctx context.Context, id uuid.UUID, req *ContactUpdate) (*Contact, error)
	DeleteContact(ctx context.Context, id uuid.UUID) error
	Autocomplete(ctx context.Context, query string, limit int) ([]*Contact, error)
}

// NewHandler creates a Handler backed by the given service.
func NewHandler(svc ServiceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns an http.Handler with all contact routes registered.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/", h.handleListContacts)
	r.Post("/", h.handleCreateContact)
	r.Get("/{id}", h.handleGetContact)
	r.Put("/{id}", h.handleUpdateContact)
	r.Delete("/{id}", h.handleDeleteContact)
	r.Get("/autocomplete", h.handleAutocomplete)

	return r
}

// handleListContacts returns contacts with optional query filters.
func (h *Handler) handleListContacts(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	typeParam := r.URL.Query().Get("type")

	var namePtr *string
	if name != "" {
		namePtr = &name
	}

	var typePtr *ContactType
	if typeParam != "" {
		t := ContactType(typeParam)
		typePtr = &t
	}

	contacts, err := h.svc.ListContacts(r.Context(), namePtr, typePtr)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, contacts)
}

// handleCreateContact creates a new contact.
func (h *Handler) handleCreateContact(w http.ResponseWriter, r *http.Request) {
	var body ContactCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	contact, err := h.svc.CreateContact(r.Context(), &body)
	if errors.Is(err, ErrValidation) {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusCreated, contact)
}

// handleGetContact returns a single contact by ID.
func (h *Handler) handleGetContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid contact id")
		return
	}

	contact, err := h.svc.GetContact(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "contact not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, contact)
}

// handleUpdateContact updates a contact.
func (h *Handler) handleUpdateContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid contact id")
		return
	}

	var body ContactUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	contact, err := h.svc.UpdateContact(r.Context(), id, &body)
	if errors.Is(err, ErrValidation) {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, ErrNotFound) {
		h.writeError(w, http.StatusNotFound, "contact not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, contact)
}

// handleDeleteContact soft-deletes a contact.
func (h *Handler) handleDeleteContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid contact id")
		return
	}

	if err := h.svc.DeleteContact(r.Context(), id); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAutocomplete returns contacts for autocomplete.
func (h *Handler) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		h.writeJSON(w, http.StatusOK, []*Contact{})
		return
	}

	contacts, err := h.svc.Autocomplete(r.Context(), q, 5)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, contacts)
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
