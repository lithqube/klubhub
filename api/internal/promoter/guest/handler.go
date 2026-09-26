package guest

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Handler exposes lists, standing lists, allocations and guests under /api/v1.
type Handler struct{ svc *Service }

// NewHandler builds the HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers every route through the authz guard.
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	route := func(method, pattern, action, resource string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: method, Pattern: pattern, Action: action, ResourceType: resource}, fn, onDeny)
	}
	const ev = "/api/v1/events/{eventID}"
	route(http.MethodGet, ev+"/lists", "guestlist.read", "guestlist", h.listLists)
	route(http.MethodPost, ev+"/lists", "guestlist.write", "guestlist", h.createList)
	route(http.MethodPut, ev+"/lists/{listID}", "guestlist.write", "guestlist", h.updateList)
	route(http.MethodDelete, ev+"/lists/{listID}", "guestlist.write", "guestlist", h.deleteList)

	route(http.MethodGet, ev+"/lists/{listID}/allocations", "guestlist.read", "guestlist", h.listAllocations)
	route(http.MethodPost, ev+"/lists/{listID}/allocations", "guestlist.write", "guestlist", h.createAllocation)
	route(http.MethodPut, ev+"/lists/{listID}/allocations/{allocationID}", "guestlist.write", "guestlist", h.updateAllocation)
	route(http.MethodDelete, ev+"/lists/{listID}/allocations/{allocationID}", "guestlist.write", "guestlist", h.revokeAllocation)

	route(http.MethodGet, ev+"/guests", "guestlist.read", "guest", h.listGuests)
	route(http.MethodPost, ev+"/guests", "guestlist.write", "guest", h.addGuests)
	route(http.MethodPost, ev+"/guests/bulk-status", "guestlist.write", "guest", h.bulkStatus)
	route(http.MethodGet, ev+"/guests/export.csv", "guestlist.read", "guest", h.exportCSV)
	route(http.MethodPut, ev+"/guests/{guestID}", "guestlist.write", "guest", h.updateGuest)
	route(http.MethodDelete, ev+"/guests/{guestID}", "guestlist.write", "guest", h.deleteGuest)

	route(http.MethodGet, "/api/v1/standing-lists", "guestlist.read", "guestlist", h.listStanding)
	route(http.MethodPost, "/api/v1/standing-lists", "guestlist.write", "guestlist", h.createStanding)
	route(http.MethodPut, "/api/v1/standing-lists/{listID}", "guestlist.write", "guestlist", h.updateStanding)
	route(http.MethodDelete, "/api/v1/standing-lists/{listID}", "guestlist.write", "guestlist", h.deleteStanding)

	route(http.MethodGet, "/api/v1/guests/overview", "guestlist.read", "guestlist", h.overview)
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
	var quota *QuotaError
	var notEmpty *NotEmptyError
	switch {
	case errors.As(err, &inv):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": inv.Field, "problem": inv.Problem})
	case errors.As(err, &quota):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "quota_exceeded", "allocation_id": quota.AllocationID,
			"label": quota.Label, "quota": quota.Quota, "used": quota.Used, "requested": quota.Requested})
	case errors.As(err, &notEmpty):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "list_not_empty", "guests": notEmpty.Guests})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.Is(err, ErrAllocationRevoked):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "allocation_revoked"})
	case errors.Is(err, ErrAllocationClosed):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "allocation_closed"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func idParam(w http.ResponseWriter, r *http.Request, names ...string) ([]uuid.UUID, bool) {
	out := make([]uuid.UUID, len(names))
	for i, n := range names {
		id, err := uuid.Parse(chi.URLParam(r, n))
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return nil, false
		}
		out[i] = id
	}
	return out, true
}

func actor(r *http.Request) string {
	p, _ := authz.PrincipalFrom(r.Context())
	return p.Sub
}

func filterOf(w http.ResponseWriter, r *http.Request) (GuestFilter, bool) {
	q := r.URL.Query()
	f := GuestFilter{Status: q.Get("status"), Q: q.Get("q")}
	if l := q.Get("list_id"); l != "" {
		id, err := uuid.Parse(l)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid", "field": "list_id", "problem": "not an id"})
			return f, false
		}
		f.ListID = &id
	}
	return f, true
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

// ---------------------------------------------------------------- lists ---

func (h *Handler) listLists(w http.ResponseWriter, r *http.Request) {
	if ids, ok := idParam(w, r, "eventID"); ok {
		v, err := h.svc.ListLists(r.Context(), ids[0])
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) createList(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID")
	var in ListInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.CreateList(r.Context(), ids[0], in)
		respond(w, http.StatusCreated, v, err)
	}
}

func (h *Handler) updateList(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID", "listID")
	var in ListInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.UpdateList(r.Context(), ids[0], ids[1], in)
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) deleteList(w http.ResponseWriter, r *http.Request) {
	if ids, ok := idParam(w, r, "eventID", "listID"); ok {
		force := r.URL.Query().Get("force") == "true"
		respond(w, 0, nil, h.svc.DeleteList(r.Context(), ids[0], ids[1], force))
	}
}

// ---------------------------------------------------------- allocations ---

func (h *Handler) listAllocations(w http.ResponseWriter, r *http.Request) {
	if ids, ok := idParam(w, r, "eventID", "listID"); ok {
		v, err := h.svc.ListAllocations(r.Context(), ids[0], ids[1])
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) createAllocation(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID", "listID")
	var in AllocationInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.CreateAllocation(r.Context(), ids[0], ids[1], in)
		respond(w, http.StatusCreated, v, err)
	}
}

func (h *Handler) updateAllocation(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID", "listID", "allocationID")
	var in AllocationInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.UpdateAllocation(r.Context(), ids[0], ids[1], ids[2], in)
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) revokeAllocation(w http.ResponseWriter, r *http.Request) {
	if ids, ok := idParam(w, r, "eventID", "listID", "allocationID"); ok {
		respond(w, 0, nil, h.svc.RevokeAllocation(r.Context(), ids[0], ids[1], ids[2]))
	}
}

// --------------------------------------------------------------- guests ---

func (h *Handler) listGuests(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	if f, ok := filterOf(w, r); ok {
		v, err := h.svc.ListGuests(r.Context(), ids[0], f)
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) addGuests(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID")
	var in AddInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.AddGuests(r.Context(), ids[0], in, actor(r))
		respond(w, http.StatusCreated, v, err)
	}
}

func (h *Handler) updateGuest(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID", "guestID")
	var in GuestInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.UpdateGuest(r.Context(), ids[0], ids[1], in)
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) deleteGuest(w http.ResponseWriter, r *http.Request) {
	if ids, ok := idParam(w, r, "eventID", "guestID"); ok {
		respond(w, 0, nil, h.svc.DeleteGuest(r.Context(), ids[0], ids[1]))
	}
}

func (h *Handler) bulkStatus(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID")
	var in BulkStatusInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.BulkStatus(r.Context(), ids[0], in)
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) exportCSV(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "eventID")
	if !ok {
		return
	}
	f, ok := filterOf(w, r)
	if !ok {
		return
	}
	slug, rows, err := h.svc.Export(r.Context(), ids[0], f, actor(r))
	if err != nil {
		fail(w, err)
		return
	}
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf") // UTF-8 BOM: spreadsheet apps then read UTF-8 names correctly
	if err := WriteCSV(&buf, rows); err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+slug+`-guests.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// ------------------------------------------------------- standing lists ---

func (h *Handler) listStanding(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.ListStanding(r.Context())
	respond(w, http.StatusOK, v, err)
}

func (h *Handler) createStanding(w http.ResponseWriter, r *http.Request) {
	var in ListInput
	if decode(w, r, &in) {
		v, err := h.svc.CreateStanding(r.Context(), in)
		respond(w, http.StatusCreated, v, err)
	}
}

func (h *Handler) updateStanding(w http.ResponseWriter, r *http.Request) {
	ids, ok := idParam(w, r, "listID")
	var in ListInput
	if ok && decode(w, r, &in) {
		v, err := h.svc.UpdateStanding(r.Context(), ids[0], in)
		respond(w, http.StatusOK, v, err)
	}
}

func (h *Handler) deleteStanding(w http.ResponseWriter, r *http.Request) {
	if ids, ok := idParam(w, r, "listID"); ok {
		respond(w, 0, nil, h.svc.DeleteStanding(r.Context(), ids[0]))
	}
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Overview(r.Context())
	respond(w, http.StatusOK, v, err)
}
