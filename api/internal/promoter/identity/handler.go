package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
)

const maxBody = 16 << 10

// Handler exposes the identity endpoints under /api/v1.
type Handler struct{ svc *Service }

// NewHandler builds the HTTP adapter for svc.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers every identity route through the authz guard.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	pub := func(method, pattern string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Public: true}, fn, onDeny)
	}
	guarded := func(method, pattern, action, resource string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: resource}, fn, onDeny)
	}
	pub(http.MethodPost, "/api/v1/auth/login", h.login)
	pub(http.MethodPost, "/api/v1/auth/logout", h.logout)
	pub(http.MethodGet, "/api/v1/auth/me", h.me)
	pub(http.MethodPost, "/api/v1/auth/setup", h.setup)
	pub(http.MethodPost, "/api/v1/auth/invites/accept", h.acceptInvite)
	pub(http.MethodPost, "/api/v1/door/login", h.doorLogin)

	guarded(http.MethodGet, "/api/v1/org", "org.read", "org", h.org)
	guarded(http.MethodPut, "/api/v1/org/profile", "org.update", "org", h.updateProfile)
	guarded(http.MethodPost, "/api/v1/auth/totp/enroll", "account.self", "account", h.totpEnroll)
	guarded(http.MethodPost, "/api/v1/auth/totp/confirm", "account.self", "account", h.totpConfirm)
	guarded(http.MethodPost, "/api/v1/members/invites", "member.manage", "member", h.invite)
	guarded(http.MethodPost, "/api/v1/door/devices", "door.device.manage", "door_device", h.registerDevice)
	guarded(http.MethodDelete, "/api/v1/door/devices/{deviceID}", "door.device.manage", "door_device", h.revokeDevice)
	guarded(http.MethodPost, "/api/v1/door/events/{eventID}/pin", "door.device.manage", "door_pin", h.setPIN)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	var perr *ProfileError
	switch {
	case errors.As(err, &perr):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": perr.Field, "problem": perr.Problem})
	case errors.Is(err, ErrTOTPRequired):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "totp_required"})
	case errors.Is(err, ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
	case errors.Is(err, ErrInvalidLink):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_link"})
	case errors.Is(err, ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "weak_password"})
	case errors.Is(err, ErrInvalidRole), errors.Is(err, ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_input"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.Login(r.Context(), LoginInput{Email: in.Email, Password: in.Password, TOTP: in.TOTP})
	if err != nil {
		fail(w, err)
		return
	}
	auth.SetSessionCookie(w, res.Cookie, res.Expires)
	writeJSON(w, http.StatusOK, map[string]bool{"totp_enrolled": res.TOTPEnrolled})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = h.svc.Logout(r.Context(), c.Value)
	}
	auth.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, ok := authz.PrincipalFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	mfa := slices.ContainsFunc(p.AMR, func(m string) bool { return m == "otp" || m == "mfa" || m == "totp" })
	writeJSON(w, http.StatusOK, map[string]any{
		"sub": p.Sub, "org_id": p.OrgID, "roles": p.Roles, "mfa": mfa,
		"auth_time": p.AuthTime.UTC().Format(time.RFC3339), "event_scope": p.EventScope,
	})
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.CompleteSetup(r.Context(), in.Token, in.Password); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token       string `json:"token"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.AcceptInvite(r.Context(), in.Token, in.DisplayName, in.Password); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) org(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	o, err := h.svc.Org(r.Context(), p)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	var in Profile
	if !decode(w, r, &in) {
		return
	}
	o, err := h.svc.UpdateProfile(r.Context(), p, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) totpEnroll(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	_, uri, err := h.svc.EnrollTOTP(r.Context(), p)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"otpauth_uri": uri})
}

func (h *Handler) totpConfirm(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := r.Cookie(auth.SessionCookie)
	if err != nil {
		fail(w, ErrInvalidInput)
		return
	}
	if err := h.svc.ConfirmTOTP(r.Context(), p, c.Value, in.Code); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	tok, err := h.svc.Invite(r.Context(), p, in.Email, in.Role)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"invite_token": tok})
}

func (h *Handler) registerDevice(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	var in struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.RegisterDoorDevice(r.Context(), p, in.Label)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) revokeDevice(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "deviceID"))
	if err != nil {
		fail(w, ErrInvalidInput)
		return
	}
	if err := h.svc.RevokeDoorDevice(r.Context(), p, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setPIN(w http.ResponseWriter, r *http.Request) {
	p, _ := authz.PrincipalFrom(r.Context())
	event, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, ErrInvalidInput)
		return
	}
	var in struct {
		ValidUntil time.Time `json:"valid_until"`
	}
	if !decode(w, r, &in) {
		return
	}
	pin, err := h.svc.SetDoorPIN(r.Context(), p, event, in.ValidUntil)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"pin": pin, "valid_until": in.ValidUntil})
}

func (h *Handler) doorLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeviceToken string    `json:"device_token"`
		EventID     uuid.UUID `json:"event_id"`
		PIN         string    `json:"pin"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.DoorLogin(r.Context(), in.DeviceToken, in.EventID, in.PIN)
	if err != nil {
		fail(w, err)
		return
	}
	auth.SetSessionCookie(w, res.Cookie, res.Expires)
	w.WriteHeader(http.StatusNoContent)
}
