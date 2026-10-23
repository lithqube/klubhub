package outreach

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for the outreach package.
type Handler struct {
	svc ServiceIface
}

// ServiceIface defines the interface exposed by the outreach service to the
// HTTP layer.
type ServiceIface interface {
	ListSlots(ctx context.Context) ([]*Slot, error)
	CreateSlot(ctx context.Context, in *SlotCreate) (*Slot, error)
	DeleteSlot(ctx context.Context, id uuid.UUID) error
	ListTemplates(ctx context.Context) ([]*Template, error)
	CreateTemplate(ctx context.Context, in *TemplateCreate) (*Template, error)
	UpdateTemplate(ctx context.Context, id uuid.UUID, in *TemplateUpdate) (*Template, error)
	DeleteTemplate(ctx context.Context, id uuid.UUID) error
	MatchSlots(ctx context.Context, slotID *uuid.UUID) ([]*SlotMatch, error)
	SendRequest(ctx context.Context, in *SendRequestInput) (*Request, error)
	ListRequests(ctx context.Context) ([]*Request, error)
	GetRequest(ctx context.Context, id uuid.UUID) (*Request, error)
	ListSuppressions(ctx context.Context) ([]*Suppression, error)
	AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error)
	DeleteSuppression(ctx context.Context, id uuid.UUID) error
}

// NewHandler creates a Handler backed by the given service.
func NewHandler(svc ServiceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns an http.Handler with all outreach routes registered.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/availability/slots", h.handleListSlots)
	r.Post("/availability/slots", h.handleCreateSlot)
	r.Delete("/availability/slots/{id}", h.handleDeleteSlot)
	r.Get("/availability/matches", h.handleMatchSlots)

	r.Get("/templates", h.handleListTemplates)
	r.Post("/templates", h.handleCreateTemplate)
	r.Put("/templates/{id}", h.handleUpdateTemplate)
	r.Delete("/templates/{id}", h.handleDeleteTemplate)

	r.Post("/requests", h.handleSendRequest)
	r.Get("/requests", h.handleListRequests)
	r.Get("/requests/{id}", h.handleGetRequest)

	r.Get("/suppressions", h.handleListSuppressions)
	r.Post("/suppressions", h.handleAddSuppression)
	r.Delete("/suppressions/{id}", h.handleDeleteSuppression)

	return r
}

// ─── Availability slots ──────────────────────────────────────────────────────

func (h *Handler) handleListSlots(w http.ResponseWriter, r *http.Request) {
	slots, err := h.svc.ListSlots(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, slots)
}

func (h *Handler) handleCreateSlot(w http.ResponseWriter, r *http.Request) {
	var body SlotCreate
	if err := h.decodeBody(r, &body); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	slot, err := h.svc.CreateSlot(r.Context(), &body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, slot)
}

func (h *Handler) handleDeleteSlot(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid slot id")
		return
	}
	if err := h.svc.DeleteSlot(r.Context(), id); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleMatchSlots(w http.ResponseWriter, r *http.Request) {
	var slotID *uuid.UUID
	if raw := r.URL.Query().Get("slot_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid slot_id")
			return
		}
		slotID = &id
	}
	matches, err := h.svc.MatchSlots(r.Context(), slotID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, matches)
}

// ─── Templates ───────────────────────────────────────────────────────────────

func (h *Handler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.svc.ListTemplates(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, templates)
}

func (h *Handler) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var body TemplateCreate
	if err := h.decodeBody(r, &body); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	template, err := h.svc.CreateTemplate(r.Context(), &body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, template)
}

func (h *Handler) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid template id")
		return
	}
	var body TemplateUpdate
	if err := h.decodeBody(r, &body); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	template, err := h.svc.UpdateTemplate(r.Context(), id, &body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, template)
}

func (h *Handler) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid template id")
		return
	}
	if err := h.svc.DeleteTemplate(r.Context(), id); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Requests (sent log) ─────────────────────────────────────────────────────

func (h *Handler) handleSendRequest(w http.ResponseWriter, r *http.Request) {
	var body SendRequestInput
	if err := h.decodeBody(r, &body); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, err := h.svc.SendRequest(r.Context(), &body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, req)
}

func (h *Handler) handleListRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := h.svc.ListRequests(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, requests)
}

func (h *Handler) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request id")
		return
	}
	req, err := h.svc.GetRequest(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, req)
}

// ─── Suppressions ────────────────────────────────────────────────────────────

func (h *Handler) handleListSuppressions(w http.ResponseWriter, r *http.Request) {
	suppressions, err := h.svc.ListSuppressions(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, suppressions)
}

func (h *Handler) handleAddSuppression(w http.ResponseWriter, r *http.Request) {
	var body SuppressionCreate
	if err := h.decodeBody(r, &body); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	suppression, err := h.svc.AddSuppression(r.Context(), &body)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, suppression)
}

func (h *Handler) handleDeleteSuppression(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid suppression id")
		return
	}
	if err := h.svc.DeleteSuppression(r.Context(), id); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// decodeBody decodes a JSON body, rejecting unknown fields.
func (h *Handler) decodeBody(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid JSON body")
	}
	return nil
}

// writeServiceError maps service sentinel errors to HTTP statuses
// (SPEC §12): invalid→422, not found→404, suppressed/conflict→409,
// rate limited→429, email unavailable→503.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrValidation),
		errors.Is(err, ErrNoContactEmail),
		errors.Is(err, ErrNoReplyTo),
		errors.Is(err, ErrEPKLegacyExport):
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrNotFound):
		h.writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSuppressed), errors.Is(err, ErrConflict):
		h.writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrRateLimited):
		h.writeError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, ErrOutreachNotPermitted):
		// 422, not 503: a provider-terms decision is not a transient outage.
		// Retrying cannot help until the operator attests a provider.
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrEmailUnavailable):
		// SPEC §5.5 / §6.3: unconfigured sender stays 503 (transient).
		h.writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		h.writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}
