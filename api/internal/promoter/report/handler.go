package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Handler exposes the post-event report under /api/v1/events/{eventID}.
type Handler struct{ svc *Service }

// NewHandler builds the HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers the report routes through the authz guard. Both need
// guestlist.read, which the door role does not have.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	const ev = "/api/v1/events/{eventID}"
	e.Handle(r, reg, authz.Route{Method: http.MethodGet, Pattern: ev + "/report", Action: "guestlist.read", ResourceType: "guestlist"},
		h.report, onDeny)
	e.Handle(r, reg, authz.Route{Method: http.MethodGet, Pattern: ev + "/report/list-back.csv", Action: "guestlist.read", ResourceType: "guest"},
		h.listBack, onDeny)
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
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func eventParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "eventID"))
	if err != nil {
		fail(w, ErrNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	eventID, ok := eventParam(w, r)
	if !ok {
		return
	}
	rep, err := h.svc.Report(r.Context(), eventID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) listBack(w http.ResponseWriter, r *http.Request) {
	eventID, ok := eventParam(w, r)
	if !ok {
		return
	}
	allocationID, err := uuid.Parse(r.URL.Query().Get("allocation_id"))
	if err != nil {
		fail(w, &InvalidError{Field: "allocation_id", Problem: "an allocation id is required"})
		return
	}
	p, _ := authz.PrincipalFrom(r.Context())
	lb, err := h.svc.ListBack(r.Context(), eventID, allocationID, p.Sub)
	if err != nil {
		fail(w, err)
		return
	}
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf") // UTF-8 BOM: spreadsheet apps then read UTF-8 names correctly
	if err := WriteListBack(&buf, lb.Rows, lb.loc); err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+lb.Filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}
