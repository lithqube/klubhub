package audience

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Handler exposes audience contacts and segments under /api/v1.
type Handler struct{ svc *Service }

// NewHandler builds the HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers every route through the authz guard. Actions
// (audience.read, audience.write, audience.export) are already declared in
// api/internal/platform/authz/policies/authz.rego from P0 — this phase
// only had to use them, not add new ones.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	route := func(method, pattern, action, resource string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: resource}, fn, onDeny)
	}
	route(http.MethodGet, "/api/v1/audience/contacts", "audience.read", "audience_contact", h.listContacts)
	route(http.MethodPost, "/api/v1/audience/contacts", "audience.write", "audience_contact", h.createContact)
	route(http.MethodPut, "/api/v1/audience/contacts/{contactID}", "audience.write", "audience_contact", h.updateContact)
	route(http.MethodPost, "/api/v1/audience/contacts/{contactID}/status", "audience.write", "audience_contact", h.setStatus)
	route(http.MethodDelete, "/api/v1/audience/contacts/{contactID}", "audience.write", "audience_contact", h.deleteContact)
	route(http.MethodPost, "/api/v1/audience/contacts/import", "audience.write", "audience_contact", h.importCSV)
	route(http.MethodGet, "/api/v1/audience/contacts/export.csv", "audience.export", "audience_contact", h.exportCSV)

	route(http.MethodGet, "/api/v1/audience/segments", "audience.read", "audience_segment", h.listSegments)
	route(http.MethodPost, "/api/v1/audience/segments", "audience.write", "audience_segment", h.createSegment)
	route(http.MethodPut, "/api/v1/audience/segments/{segmentID}", "audience.write", "audience_segment", h.updateSegment)
	route(http.MethodDelete, "/api/v1/audience/segments/{segmentID}", "audience.write", "audience_segment", h.deleteSegment)
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

func fail(w http.ResponseWriter, err error) {
	var inv *InvalidError
	switch {
	case errors.As(err, &inv):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": inv.Field, "problem": inv.Problem})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.Is(err, ErrDuplicate):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "duplicate"})
	case errors.Is(err, ErrSegmentExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "segment_exists"})
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

func actor(r *http.Request) string {
	p, _ := authz.PrincipalFrom(r.Context())
	return p.Sub
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

// -------------------------------------------------------------- contacts --

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.ListContacts(r.Context(), ContactFilter{Status: q.Get("status"), Source: q.Get("source"), Q: q.Get("q")})
	respond(w, http.StatusOK, page, err)
}

// clientConsent fills the request-observed parts of a ConsentInput
// (recorded_at defaults to now, ip defaults to the request's remote
// address) so a caller only has to supply basis, form_text and, for a
// returning contact, double_opt_in_confirmed_at.
func clientConsent(r *http.Request, in *ConsentInput) {
	if in.RecordedAt.IsZero() {
		in.RecordedAt = time.Now()
	}
	if in.IP == "" {
		in.IP = clientIP(r)
	}
}

// clientIP returns the connection's own remote address, never a
// caller-supplied header: an authenticated audience.write caller could
// otherwise put an arbitrary X-Forwarded-For value into a consent record.
// There is no trusted-proxy layer in front of this service today; add one
// here (and only trust it for known proxy hops) if that changes.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

func (h *Handler) createContact(w http.ResponseWriter, r *http.Request) {
	var in ContactInput
	if !decode(w, r, &in) {
		return
	}
	clientConsent(r, &in.Consent)
	c, err := h.svc.CreateContact(r.Context(), in)
	respond(w, http.StatusCreated, c, err)
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "contactID")
	if !ok {
		return
	}
	var in ContactInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.UpdateContact(r.Context(), id, in)
	respond(w, http.StatusOK, c, err)
}

func (h *Handler) setStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "contactID")
	if !ok {
		return
	}
	var in StatusInput
	if !decode(w, r, &in) {
		return
	}
	err := h.svc.SetStatus(r.Context(), id, in)
	respond(w, http.StatusOK, nil, err)
}

func (h *Handler) deleteContact(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "contactID")
	if !ok {
		return
	}
	err := h.svc.DeleteContact(r.Context(), id, actor(r))
	respond(w, http.StatusOK, nil, err)
}

type importRequest struct {
	Rows    []ImportRow  `json:"rows"`
	Consent ConsentInput `json:"consent"`
}

func (h *Handler) importCSV(w http.ResponseWriter, r *http.Request) {
	var in importRequest
	if !decode(w, r, &in) {
		return
	}
	clientConsent(r, &in.Consent)
	res, err := h.svc.ImportCSV(r.Context(), in.Rows, in.Consent)
	respond(w, http.StatusOK, res, err)
}

// trackingWriter notices whether anything has reached the client yet, so
// exportCSV can still answer with a proper error status if the export
// fails before the first CSV byte is written (after that, the 200 status
// is already committed and there is nothing more to do on a write error).
type trackingWriter struct {
	w     io.Writer
	wrote bool
}

func (t *trackingWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		t.wrote = true
	}
	return t.w.Write(p)
}

func (h *Handler) exportCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audience.csv"`)
	tw := &trackingWriter{w: w}
	filter := ContactFilter{Status: q.Get("status"), Source: q.Get("source")}
	if err := h.svc.ExportContacts(r.Context(), filter, tw); err != nil && !tw.wrote {
		fail(w, err)
	}
}

// -------------------------------------------------------------- segments --

func (h *Handler) listSegments(w http.ResponseWriter, r *http.Request) {
	segs, err := h.svc.ListSegments(r.Context())
	respond(w, http.StatusOK, segs, err)
}

func (h *Handler) createSegment(w http.ResponseWriter, r *http.Request) {
	var in SegmentInput
	if !decode(w, r, &in) {
		return
	}
	seg, err := h.svc.CreateSegment(r.Context(), in)
	respond(w, http.StatusCreated, seg, err)
}

func (h *Handler) updateSegment(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "segmentID")
	if !ok {
		return
	}
	var in SegmentInput
	if !decode(w, r, &in) {
		return
	}
	seg, err := h.svc.UpdateSegment(r.Context(), id, in)
	respond(w, http.StatusOK, seg, err)
}

func (h *Handler) deleteSegment(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "segmentID")
	if !ok {
		return
	}
	err := h.svc.DeleteSegment(r.Context(), id)
	respond(w, http.StatusOK, nil, err)
}
