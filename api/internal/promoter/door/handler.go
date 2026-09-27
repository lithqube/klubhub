package door

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Handler exposes the door device routes under /api/v1/door.
type Handler struct{ svc *Service }

// NewHandler builds the HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers the door routes. The event always comes from the door
// session (EventFromScope), never from the URL.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	route := func(method, pattern, action string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: "door", EventFromScope: true}, fn, onDeny)
	}
	route(http.MethodGet, "/api/v1/door/bundle", "door.read", h.bundle)
	route(http.MethodPost, "/api/v1/door/checkins", "door.checkin", h.checkins)
	route(http.MethodPost, "/api/v1/door/adds", "door.checkin", h.adds)
}

// 500 ops of a few hundred bytes each fit comfortably.
const maxBody = 512 << 10

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

func fail(w http.ResponseWriter, err error) {
	var inv *InvalidError
	switch {
	case errors.Is(err, ErrNoSession):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "door_session_required"})
	case errors.As(err, &inv):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": inv.Field, "problem": inv.Problem})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

// session turns the verified principal into a door session. Staff and door
// principals without an event scope are refused: door routes act on the
// session's event only.
func session(r *http.Request) (Session, error) {
	p, ok := authz.PrincipalFrom(r.Context())
	if !ok || p.EventScope == "" {
		return Session{}, ErrNoSession
	}
	tenant, err1 := uuid.Parse(p.OrgID)
	event, err2 := uuid.Parse(p.EventScope)
	device, err3 := uuid.Parse(p.DeviceID)
	if err1 != nil || err2 != nil || err3 != nil {
		return Session{}, ErrNoSession
	}
	return Session{Tenant: tenant, Event: event, Device: device, Sub: p.Sub}, nil
}

func (h *Handler) bundle(w http.ResponseWriter, r *http.Request) {
	sess, err := session(r)
	if err != nil {
		fail(w, err)
		return
	}
	b, err := h.svc.Bundle(r.Context(), sess)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) checkins(w http.ResponseWriter, r *http.Request) {
	sess, err := session(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in SyncInput
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.Sync(r.Context(), sess, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) adds(w http.ResponseWriter, r *http.Request) {
	sess, err := session(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in AddsInput
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.Adds(r.Context(), sess, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
