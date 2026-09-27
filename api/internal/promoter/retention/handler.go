package retention

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Handler exposes the retention settings, the privacy view and "erase now".
type Handler struct{ svc *Service }

// NewHandler builds the HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers the routes through the authz guard. The purge action
// (event.purge) needs a recent sign-in (step-up) in the policy.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	route := func(method, pattern, action, resource string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: resource}, fn, onDeny)
	}
	route(http.MethodGet, "/api/v1/org/retention", "org.read", "org", h.settings)
	route(http.MethodPut, "/api/v1/org/retention", "org.update", "org", h.update)
	route(http.MethodPost, "/api/v1/events/{eventID}/purge", "event.purge", "event", h.purge)
	route(http.MethodGet, "/api/v1/events/{eventID}/privacy", "event.read", "event", h.privacy)
}

const maxBody = 4 << 10

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return false
	}
	return true
}

// fail writes the HTTP form of a retention error. Other packages map
// ErrEventPurged the same way: 409 {"error":"event_purged"}.
func fail(w http.ResponseWriter, err error) {
	var inv *InvalidError
	switch {
	case errors.As(err, &inv):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": inv.Field, "problem": inv.Problem})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.Is(err, ErrEventPurged):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "event_purged"})
	case errors.Is(err, ErrNotEnded):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "event_not_ended"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func actor(r *http.Request) string {
	p, _ := authz.PrincipalFrom(r.Context())
	return p.Sub
}

func eventParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, ErrNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Settings(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RetentionDays int `json:"retention_days"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.UpdateRetention(r.Context(), in.RetentionDays, actor(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) purge(w http.ResponseWriter, r *http.Request) {
	id, ok := eventParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Purge(r.Context(), id, in.Confirm, actor(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) privacy(w http.ResponseWriter, r *http.Request) {
	id, ok := eventParam(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Privacy(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
