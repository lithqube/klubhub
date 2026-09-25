package event

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Handler exposes venues, events, stages, lineup and export under /api/v1.
type Handler struct{ svc *Service }

// NewHandler builds the HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers every route through the authz guard.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	route := func(method, pattern, action, resource string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: resource}, fn, onDeny)
	}
	route(http.MethodGet, "/api/v1/venues", "event.read", "venue", h.listVenues)
	route(http.MethodPost, "/api/v1/venues", "event.write", "venue", h.createVenue)
	route(http.MethodGet, "/api/v1/venues/{venueID}", "event.read", "venue", h.getVenue)
	route(http.MethodPut, "/api/v1/venues/{venueID}", "event.write", "venue", h.updateVenue)
	route(http.MethodDelete, "/api/v1/venues/{venueID}", "event.write", "venue", h.archiveVenue)
	route(http.MethodPost, "/api/v1/venues/{venueID}/reveal", "venue.reveal", "venue", h.revealVenue)

	route(http.MethodGet, "/api/v1/events", "event.read", "event", h.listEvents)
	route(http.MethodPost, "/api/v1/events", "event.write", "event", h.createEvent)
	route(http.MethodGet, "/api/v1/events/{eventID}", "event.read", "event", h.getEvent)
	route(http.MethodPut, "/api/v1/events/{eventID}", "event.write", "event", h.updateEvent)
	route(http.MethodPost, "/api/v1/events/{eventID}/status", "event.write", "event", h.changeStatus)
	route(http.MethodPut, "/api/v1/events/{eventID}/stages", "event.write", "event", h.replaceStages)
	route(http.MethodPut, "/api/v1/events/{eventID}/lineup", "event.write", "event", h.replaceLineup)
	route(http.MethodGet, "/api/v1/events/{eventID}/export", "event.read", "event", h.export)
	route(http.MethodGet, "/api/v1/events/{eventID}/export/jsonld", "event.read", "event", h.exportJSONLD)
	route(http.MethodGet, "/api/v1/events/{eventID}/export/ics", "event.read", "event", h.exportICS)
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
	var cut *CutsSetsError
	var blocked *BlockedError
	switch {
	case errors.As(err, &inv):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": inv.Field, "problem": inv.Problem})
	case errors.As(err, &cut):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "cuts_sets", "entries": cut.Entries})
	case errors.As(err, &blocked):
		writeJSON(w, http.StatusConflict, map[string]any{"error": blocked.Reason, "issues": blocked.Issues})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.Is(err, ErrVersionConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_conflict"})
	case errors.Is(err, ErrVenueInUse):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "venue_in_use"})
	case errors.Is(err, ErrBadTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "bad_transition"})
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

func (h *Handler) listVenues(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.ListVenues(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) createVenue(w http.ResponseWriter, r *http.Request) {
	var in VenueInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.CreateVenue(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) getVenue(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "venueID")
	if !ok {
		return
	}
	v, err := h.svc.GetVenue(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) updateVenue(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "venueID")
	if !ok {
		return
	}
	var in VenueInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.UpdateVenue(r.Context(), id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) archiveVenue(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "venueID")
	if !ok {
		return
	}
	if err := h.svc.ArchiveVenue(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) revealVenue(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "venueID")
	if !ok {
		return
	}
	p, _ := authz.PrincipalFrom(r.Context())
	v, err := h.svc.RevealVenue(r.Context(), id, p.Sub)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "upcoming"
	}
	list, err := h.svc.ListEvents(r.Context(), view)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	var in EventInput
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.CreateEvent(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) getEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	d, err := h.svc.GetEvent(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	var in struct {
		Version int        `json:"version"`
		Event   EventInput `json:"event"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.UpdateEvent(r.Context(), id, in.Version, in.Event)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) changeStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	var in struct {
		Version int    `json:"version"`
		Status  string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.ChangeStatus(r.Context(), id, in.Version, in.Status)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) replaceStages(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	var in struct {
		Version int          `json:"version"`
		Stages  []StageInput `json:"stages"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.ReplaceStages(r.Context(), id, in.Version, in.Stages)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) replaceLineup(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	var in struct {
		Version int           `json:"version"`
		Lineup  []LineupInput `json:"lineup"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := h.svc.ReplaceLineup(r.Context(), id, in.Version, in.Lineup)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) exportData(w http.ResponseWriter, r *http.Request) (ExportData, bool) {
	id, ok := idParam(w, r, "eventID")
	if !ok {
		return ExportData{}, false
	}
	x, err := h.svc.Export(r.Context(), id)
	if err != nil {
		fail(w, err)
		return ExportData{}, false
	}
	return x, true
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	if x, ok := h.exportData(w, r); ok {
		writeJSON(w, http.StatusOK, x)
	}
}

// exportJSONLD serves the MusicEvent document. Private events have no
// public page, so no structured data (UX decision 7).
func (h *Handler) exportJSONLD(w http.ResponseWriter, r *http.Request) {
	x, ok := h.exportData(w, r)
	if !ok {
		return
	}
	if x.Event.Visibility == "private" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "private_event"})
		return
	}
	w.Header().Set("Content-Type", "application/ld+json")
	_ = json.NewEncoder(w).Encode(JSONLD(x.Event, x.Location, x.Profile, x.Lineup))
}

func (h *Handler) exportICS(w http.ResponseWriter, r *http.Request) {
	x, ok := h.exportData(w, r)
	if !ok {
		return
	}
	name := strings.TrimSuffix(x.Event.Slug, "-") + ".ics"
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write([]byte(ICS(x.Event, x.Location, h.svc.now())))
}
