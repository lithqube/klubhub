package sealed

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Handler exposes the key and ban-list routes. Mount keeps the policy
// engine: replacing one's member key is authorised again as
// security.manage (owner recovery with the kit).
type Handler struct {
	svc    *Service
	engine *authz.Engine
	onDeny authz.Denied
}

// NewHandler builds the HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers the routes through the authz guard.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	h.engine, h.onDeny = e, onDeny
	route := func(method, pattern, action, resource string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: resource}, fn, onDeny)
	}
	route(http.MethodGet, "/api/v1/keys/me", "account.self", "member_key", h.myKey)
	route(http.MethodPut, "/api/v1/keys/me", "account.self", "member_key", h.putMyKey)
	route(http.MethodGet, "/api/v1/keys/org", "account.self", "org_key", h.orgStatus)
	route(http.MethodGet, "/api/v1/keys/org/recipients", "security.manage", "org_key", h.recipients)
	route(http.MethodPost, "/api/v1/keys/org/setup", "security.manage", "org_key", h.setup)
	route(http.MethodPost, "/api/v1/keys/org/wraps", "security.manage", "org_key", h.addWraps)
	route(http.MethodPost, "/api/v1/keys/org/rotate", "security.manage", "org_key", h.rotate)
	route(http.MethodGet, "/api/v1/keys/org/recovery", "security.manage", "org_key", h.recovery)
	route(http.MethodPut, "/api/v1/keys/devices/{deviceID}/wrap", "door.device.manage", "door_device", h.deviceWrap)
	route(http.MethodGet, "/api/v1/ban-list", "guestlist.read", "ban_list", h.banList)
	route(http.MethodPost, "/api/v1/ban-list", "guestlist.write", "ban_list", h.addBan)
	route(http.MethodPut, "/api/v1/ban-list/{entryID}", "guestlist.write", "ban_list", h.updateBan)
	route(http.MethodDelete, "/api/v1/ban-list/{entryID}", "guestlist.write", "ban_list", h.deleteBan)
}

// Body caps: a rotation carries every ban entry (MaxBanEntries of at most
// MaxEntrySealed bytes, base64url) plus MaxWraps wraps.
const (
	maxBody       = 64 << 10
	maxRotateBody = 16 << 20
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "body_too_large"})
			return false
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return false
	}
	return true
}

func fail(w http.ResponseWriter, err error) {
	var inv *InvalidError
	var c *ConflictError
	var denied *DeniedError
	switch {
	case errors.As(err, &inv):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": inv.Field, "problem": inv.Problem})
	case errors.As(err, &c):
		writeJSON(w, http.StatusConflict, map[string]string{"error": c.Code})
	case errors.As(err, &denied):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": denied.Reason})
	case errors.Is(err, ErrNoMemberKey):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no_member_key"})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.Is(err, ErrLocalIdentity):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "local_identity_required"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func principal(r *http.Request) authz.Principal {
	p, _ := authz.PrincipalFrom(r.Context())
	return p
}

func idParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		fail(w, ErrNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) myKey(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.MyKey(r.Context(), principal(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) putMyKey(w http.ResponseWriter, r *http.Request) {
	var in MemberKeyInput
	if !decode(w, r, maxBody, &in) {
		return
	}
	v, created, err := h.svc.PutMyKey(r.Context(), principal(r), in, func() error { return h.authorize(r, "security.manage") })
	if err != nil {
		fail(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, v)
}

// authorize asks the policy again for action (with the caller's real
// principal), auditing a refusal like the route guard does.
func (h *Handler) authorize(r *http.Request, action string) error {
	p, ok := authz.PrincipalFrom(r.Context())
	if !ok || h.engine == nil {
		return &DeniedError{Reason: authz.ReasonPolicyError}
	}
	d := h.engine.Authorize(r.Context(), authz.Request{
		Principal: p, Action: action, Resource: authz.Resource{Type: "member_key", OrgID: p.OrgID},
		Route: "/api/v1/keys/me", Method: r.Method,
	})
	if d.Allow {
		return nil
	}
	if h.onDeny != nil {
		h.onDeny(r, p, action, d)
	}
	return &DeniedError{Reason: d.Reason}
}

func (h *Handler) orgStatus(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.OrgStatus(r.Context(), principal(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) recipients(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Recipients(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	var in SetupInput
	if !decode(w, r, maxBody*8, &in) {
		return
	}
	v, err := h.svc.Setup(r.Context(), principal(r), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) addWraps(w http.ResponseWriter, r *http.Request) {
	var in WrapsInput
	if !decode(w, r, maxBody*8, &in) {
		return
	}
	v, err := h.svc.AddWraps(r.Context(), principal(r), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) rotate(w http.ResponseWriter, r *http.Request) {
	var in RotateInput
	if !decode(w, r, maxRotateBody, &in) {
		return
	}
	v, err := h.svc.Rotate(r.Context(), principal(r), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) recovery(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Recovery(r.Context(), principal(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) deviceWrap(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "deviceID")
	if !ok {
		return
	}
	var in DeviceWrapInput
	if !decode(w, r, maxBody, &in) {
		return
	}
	v, err := h.svc.PutDeviceWrap(r.Context(), principal(r), id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) banList(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.BanList(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) addBan(w http.ResponseWriter, r *http.Request) {
	var in BanInput
	if !decode(w, r, maxBody, &in) {
		return
	}
	v, err := h.svc.AddBan(r.Context(), principal(r), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) updateBan(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "entryID")
	if !ok {
		return
	}
	var in BanInput
	if !decode(w, r, maxBody, &in) {
		return
	}
	v, err := h.svc.UpdateBan(r.Context(), principal(r), id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) deleteBan(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "entryID")
	if !ok {
		return
	}
	if err := h.svc.DeleteBan(r.Context(), principal(r), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
