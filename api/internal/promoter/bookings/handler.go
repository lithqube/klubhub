package bookings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// ServiceIface defines the interface exposed by the bookings service to the
// HTTP layer. Fakes in tests substitute it.
type ServiceIface interface {
	ListTalentContacts(ctx context.Context, filter TalentFilter) ([]*TalentContact, error)
	CreateTalentContact(ctx context.Context, in TalentContactInput) (*TalentContact, error)
	UpdateTalentContact(ctx context.Context, id uuid.UUID, in TalentContactInput) (*TalentContact, error)
	DeleteTalentContact(ctx context.Context, id uuid.UUID) error
	SuppressTalentContact(ctx context.Context, id uuid.UUID) (*Suppression, error)

	ListOpenSlots(ctx context.Context, eventID uuid.UUID) ([]*OpenSlot, error)
	CreateOpenSlot(ctx context.Context, eventID uuid.UUID, in OpenSlotInput) (*OpenSlot, error)
	DeleteOpenSlot(ctx context.Context, slotID uuid.UUID) error
	MatchSlot(ctx context.Context, slotID uuid.UUID) ([]*TalentMatch, error)

	ListTemplates(ctx context.Context) ([]*Template, error)
	CreateTemplate(ctx context.Context, in TemplateCreate) (*Template, error)
	UpdateTemplate(ctx context.Context, id uuid.UUID, in TemplateUpdate) (*Template, error)
	DeleteTemplate(ctx context.Context, id uuid.UUID) error

	SendOffer(ctx context.Context, p authz.Principal, in *SendOfferInput) (*OfferRequest, error)
	ListOffers(ctx context.Context) ([]*OfferRequest, error)
	GetOffer(ctx context.Context, id uuid.UUID) (*OfferRequest, error)

	ListSuppressions(ctx context.Context) ([]*Suppression, error)
	AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error)
	DeleteSuppression(ctx context.Context, id uuid.UUID) error
}

// Handler exposes the bookings module under /api/v1/bookings.
type Handler struct {
	svc ServiceIface
}

// NewHandler builds the HTTP adapter.
func NewHandler(svc ServiceIface) *Handler { return &Handler{svc: svc} }

// Mount registers every route through the authz guard (SPEC §6.2 v2.1 action
// split): talent.* guards the talent PII surfaces (contacts, matches, the
// sent log, suppressions), booking.* guards the booking work (slots,
// templates). POST /offers declares talent.write at the route; the service
// additionally re-checks the slot's event is the caller's (booking.write
// semantics).
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	route := func(method, pattern, action, resource string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: resource}, fn, onDeny)
	}

	route(http.MethodGet, "/api/v1/bookings/talent", "talent.read", "talent_contact", h.listTalent)
	route(http.MethodPost, "/api/v1/bookings/talent", "talent.write", "talent_contact", h.createTalent)
	route(http.MethodPut, "/api/v1/bookings/talent/{contactID}", "talent.write", "talent_contact", h.updateTalent)
	route(http.MethodDelete, "/api/v1/bookings/talent/{contactID}", "talent.write", "talent_contact", h.deleteTalent)
	route(http.MethodPost, "/api/v1/bookings/talent/{contactID}/suppress", "talent.write", "talent_contact", h.suppressTalent)

	route(http.MethodGet, "/api/v1/bookings/events/{eventID}/slots", "booking.read", "booking", h.listSlots)
	route(http.MethodPost, "/api/v1/bookings/events/{eventID}/slots", "booking.write", "booking", h.createSlot)
	route(http.MethodDelete, "/api/v1/bookings/slots/{slotID}", "booking.write", "booking", h.deleteSlot)
	route(http.MethodGet, "/api/v1/bookings/slots/{slotID}/matches", "talent.read", "talent_contact", h.matchSlot)

	route(http.MethodGet, "/api/v1/bookings/templates", "booking.read", "booking", h.listTemplates)
	route(http.MethodPost, "/api/v1/bookings/templates", "booking.write", "booking", h.createTemplate)
	route(http.MethodPut, "/api/v1/bookings/templates/{templateID}", "booking.write", "booking", h.updateTemplate)
	route(http.MethodDelete, "/api/v1/bookings/templates/{templateID}", "booking.write", "booking", h.deleteTemplate)

	route(http.MethodPost, "/api/v1/bookings/offers", "talent.write", "talent_contact", h.sendOffer)
	route(http.MethodGet, "/api/v1/bookings/offers", "talent.read", "talent_contact", h.listOffers)
	route(http.MethodGet, "/api/v1/bookings/offers/{offerID}", "talent.read", "talent_contact", h.getOffer)

	route(http.MethodGet, "/api/v1/bookings/suppressions", "talent.read", "talent_contact", h.listSuppressions)
	route(http.MethodPost, "/api/v1/bookings/suppressions", "talent.write", "talent_contact", h.addSuppression)
	route(http.MethodDelete, "/api/v1/bookings/suppressions/{suppressionID}", "talent.write", "talent_contact", h.deleteSuppression)
}

const maxBody = 512 << 10

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// fail maps service errors to HTTP statuses (SPEC §12): invalid / no contact
// email / no member reply-to → 422, not found → 404, forbidden (slot outside
// the caller's tenant) → 403, duplicate / template version conflict /
// suppressed → 409, rate limited → 429, email unavailable → 503.
func fail(w http.ResponseWriter, err error) {
	var inv *InvalidError
	switch {
	case errors.As(err, &inv):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": inv.Field, "problem": inv.Problem})
	case errors.Is(err, ErrNoContactEmail):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no_contact_email"})
	case errors.Is(err, ErrNoReplyTo):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no_reply_to"})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.Is(err, ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, ErrDuplicate):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "duplicate"})
	case errors.Is(err, ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "conflict"})
	case errors.Is(err, ErrSuppressed):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "suppressed"})
	case errors.Is(err, ErrRateLimited):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
	case errors.Is(err, ErrEmailUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "email_unavailable"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func idParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return uuid.Nil, false
	}
	return id, true
}

func respond(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	if v == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, status, v)
}

// -------------------------------------------------------------- talent --

func (h *Handler) listTalent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	contacts, err := h.svc.ListTalentContacts(r.Context(), TalentFilter{
		City:  q.Get("city"),
		Genre: q.Get("genre"),
		Q:     q.Get("q"),
	})
	respond(w, http.StatusOK, contacts, err)
}

func (h *Handler) createTalent(w http.ResponseWriter, r *http.Request) {
	var in TalentContactInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.CreateTalentContact(r.Context(), in)
	respond(w, http.StatusCreated, c, err)
}

func (h *Handler) updateTalent(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "contactID")
	if !ok {
		return
	}
	var in TalentContactInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.UpdateTalentContact(r.Context(), id, in)
	respond(w, http.StatusOK, c, err)
}

func (h *Handler) deleteTalent(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "contactID")
	if !ok {
		return
	}
	err := h.svc.DeleteTalentContact(r.Context(), id)
	respond(w, http.StatusNoContent, nil, err)
}

func (h *Handler) suppressTalent(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "contactID")
	if !ok {
		return
	}
	suppression, err := h.svc.SuppressTalentContact(r.Context(), id)
	respond(w, http.StatusCreated, suppression, err)
}

// -------------------------------------------------------------- slots --

func (h *Handler) listSlots(w http.ResponseWriter, r *http.Request) {
	eventID, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	slots, err := h.svc.ListOpenSlots(r.Context(), eventID)
	respond(w, http.StatusOK, slots, err)
}

func (h *Handler) createSlot(w http.ResponseWriter, r *http.Request) {
	eventID, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	var in OpenSlotInput
	if !decode(w, r, &in) {
		return
	}
	slot, err := h.svc.CreateOpenSlot(r.Context(), eventID, in)
	respond(w, http.StatusCreated, slot, err)
}

func (h *Handler) deleteSlot(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "slotID")
	if !ok {
		return
	}
	err := h.svc.DeleteOpenSlot(r.Context(), id)
	respond(w, http.StatusNoContent, nil, err)
}

func (h *Handler) matchSlot(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "slotID")
	if !ok {
		return
	}
	matches, err := h.svc.MatchSlot(r.Context(), id)
	respond(w, http.StatusOK, matches, err)
}

// -------------------------------------------------------------- templates --

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.svc.ListTemplates(r.Context())
	respond(w, http.StatusOK, templates, err)
}

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var in TemplateCreate
	if !decode(w, r, &in) {
		return
	}
	template, err := h.svc.CreateTemplate(r.Context(), in)
	respond(w, http.StatusCreated, template, err)
}

func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "templateID")
	if !ok {
		return
	}
	var in TemplateUpdate
	if !decode(w, r, &in) {
		return
	}
	template, err := h.svc.UpdateTemplate(r.Context(), id, in)
	respond(w, http.StatusOK, template, err)
}

func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "templateID")
	if !ok {
		return
	}
	err := h.svc.DeleteTemplate(r.Context(), id)
	respond(w, http.StatusNoContent, nil, err)
}

// -------------------------------------------------------------- offers --

func (h *Handler) sendOffer(w http.ResponseWriter, r *http.Request) {
	var in SendOfferInput
	if !decode(w, r, &in) {
		return
	}
	p, _ := authz.PrincipalFrom(r.Context())
	offer, err := h.svc.SendOffer(r.Context(), p, &in)
	respond(w, http.StatusCreated, offer, err)
}

func (h *Handler) listOffers(w http.ResponseWriter, r *http.Request) {
	offers, err := h.svc.ListOffers(r.Context())
	respond(w, http.StatusOK, offers, err)
}

func (h *Handler) getOffer(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "offerID")
	if !ok {
		return
	}
	offer, err := h.svc.GetOffer(r.Context(), id)
	respond(w, http.StatusOK, offer, err)
}

// -------------------------------------------------------------- suppressions --

func (h *Handler) listSuppressions(w http.ResponseWriter, r *http.Request) {
	suppressions, err := h.svc.ListSuppressions(r.Context())
	respond(w, http.StatusOK, suppressions, err)
}

func (h *Handler) addSuppression(w http.ResponseWriter, r *http.Request) {
	var in SuppressionCreate
	if !decode(w, r, &in) {
		return
	}
	suppression, err := h.svc.AddSuppression(r.Context(), &in)
	respond(w, http.StatusCreated, suppression, err)
}

func (h *Handler) deleteSuppression(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "suppressionID")
	if !ok {
		return
	}
	err := h.svc.DeleteSuppression(r.Context(), id)
	respond(w, http.StatusNoContent, nil, err)
}
