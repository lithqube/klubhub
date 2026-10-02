package finance

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type EntryServiceIface interface {
	Create(context.Context, CreateEntryRequest) (*Entry, error)
	Get(context.Context, uuid.UUID) (*Entry, error)
	List(context.Context, EntryFilter) ([]*Entry, error)
	Update(context.Context, uuid.UUID, UpdateEntryRequest) (*Entry, error)
	Delete(context.Context, uuid.UUID, time.Time) error
	Void(context.Context, uuid.UUID, time.Time) (*Entry, error)
	Summary(context.Context, DateRange) (map[string]EntryTotals, error)
	ProfitLoss(context.Context, ProfitLossFilter) (map[string]ProfitLossTotals, error)
	GetPendingReconciliationByGig(context.Context, uuid.UUID) (*EntryReconciliation, error)
	GetPendingReconciliationByEntry(context.Context, uuid.UUID) (*EntryReconciliation, error)
	ResolveReconciliation(context.Context, uuid.UUID, ResolveReconciliationRequest) (*EntryReconciliation, error)
}

type EntryHandler struct{ svc EntryServiceIface }

func NewEntryHandler(svc EntryServiceIface) *EntryHandler { return &EntryHandler{svc: svc} }

func (h *EntryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/finance/"
	if len(r.URL.Path) < len(prefix) || r.URL.Path[:len(prefix)] != prefix {
		writeError(w, http.StatusNotFound, "not_found", "invalid finance entry path")
		return
	}
	parts := splitPath(r.URL.Path[len(prefix):])
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "finance entry path missing")
		return
	}
	switch parts[0] {
	case "summary":
		if len(parts) != 1 || r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
			return
		}
		h.handleSummary(w, r)
		return
	case "profit-loss":
		if len(parts) != 1 || r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
			return
		}
		h.handleProfitLoss(w, r)
		return
	case "entries":
	default:
		// /api/v1/finance/reconciliations is owned by the entry handler
		// because reconciliations are persisted alongside finance
		// entries.
		if parts[0] == "reconciliations" {
			h.serveReconciliations(w, r, parts[1:])
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "invalid finance entry path")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r)
		case http.MethodPost:
			h.handleCreate(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are supported")
		}
		return
	}
	if len(parts) > 3 {
		writeError(w, http.StatusNotFound, "not_found", "invalid finance entry path")
		return
	}
	id, err := uuid.Parse(parts[1])
	if err != nil || id == uuid.Nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid entry id")
		return
	}
	if len(parts) == 3 {
		if parts[2] != "void" {
			writeError(w, http.StatusNotFound, "not_found", "unknown entry action")
			return
		}
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
			return
		}
		h.handleVoid(w, r, id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, id)
	case http.MethodPut:
		h.handleUpdate(w, r, id)
	case http.MethodDelete:
		h.handleDelete(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET, PUT and DELETE are supported")
	}
}

func (h *EntryHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateEntryRequest
	if !decodeBody(w, r, &req) {
		return
	}
	e, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": e})
}

func (h *EntryHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	e, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": e})
}

func (h *EntryHandler) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := EntryFilter{Kind: EntryKind(q.Get("kind")), Status: EntryStatus(q.Get("status")), Currency: q.Get("currency"), Category: q.Get("category"), From: q.Get("from"), To: q.Get("to"), Receipt: ReceiptFilter(q.Get("receipt"))}
	if raw := q.Get("gig_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			writeError(w, http.StatusBadRequest, "validation_failed", "gig_id must be a UUID")
			return
		}
		f.GigID = &id
	}
	entries, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	if entries == nil {
		entries = []*Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
}

func (h *EntryHandler) handleUpdate(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req UpdateEntryRequest
	if !decodeBody(w, r, &req) {
		return
	}
	e, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": e})
}

type entryTokenRequest struct {
	UpdatedAt time.Time `json:"updated_at"`
}

func (h *EntryHandler) handleDelete(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var token time.Time
	if raw := r.URL.Query().Get("updated_at"); raw != "" {
		token, _ = time.Parse(time.RFC3339Nano, raw)
	} else {
		var req entryTokenRequest
		if !decodeBody(w, r, &req) {
			return
		}
		token = req.UpdatedAt
	}
	if err := h.svc.Delete(r.Context(), id, token); err != nil {
		writeEntryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *EntryHandler) handleVoid(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req entryTokenRequest
	if !decodeBody(w, r, &req) {
		return
	}
	e, err := h.svc.Void(r.Context(), id, req.UpdatedAt)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": e})
}

func (h *EntryHandler) handleSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := SummaryFilter{Scope: SummaryScope(q.Get("scope"))}
	f.Year, _ = strconv.Atoi(q.Get("year"))
	f.Month, _ = strconv.Atoi(q.Get("month"))
	dr, err := f.DateRange()
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	out, err := h.svc.Summary(r.Context(), dr)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *EntryHandler) handleProfitLoss(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := ProfitLossFilter{Scope: ProfitLossScope(q.Get("scope"))}
	f.Year, _ = strconv.Atoi(q.Get("year"))
	f.Month, _ = strconv.Atoi(q.Get("month"))
	if raw := q.Get("gig_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			writeError(w, http.StatusBadRequest, "validation_failed", "gig_id must be a UUID")
			return
		}
		f.GigID = &id
	}
	out, err := h.svc.ProfitLoss(r.Context(), f)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// serveReconciliations routes the /reconciliations/* endpoints used by
// the UI to fetch and resolve durable finance reconciliations after a
// page refresh. The persisted GET is the reliable source — gig update
// responses also include the metadata when it was just created in the
// same request, but the GET keeps the prompt consistent across reloads.
func (h *EntryHandler) serveReconciliations(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			h.handleGetReconciliation(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		}
		return
	}
	if len(parts) == 2 && parts[1] == "resolve" {
		id, err := uuid.Parse(parts[0])
		if err != nil || id == uuid.Nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid reconciliation id")
			return
		}
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
			return
		}
		h.handleResolveReconciliation(w, r, id)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "unknown reconciliation action")
}

func (h *EntryHandler) handleGetReconciliation(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("gig_id")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "gig_id is required")
		return
	}
	gigID, err := uuid.Parse(raw)
	if err != nil || gigID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "gig_id must be a UUID")
		return
	}
	rec, err := h.svc.GetPendingReconciliationByGig(r.Context(), gigID)
	if err != nil {
		if errors.Is(err, ErrReconciliationNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "no pending reconciliation for gig")
			return
		}
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rec})
}

func (h *EntryHandler) handleResolveReconciliation(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req ResolveReconciliationRequest
	if !decodeBody(w, r, &req) {
		return
	}
	rec, err := h.svc.ResolveReconciliation(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrReconciliationNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "reconciliation not found")
			return
		}
		if errors.Is(err, ErrReconciliationConflict) {
			writeError(w, http.StatusConflict, "conflict", "reconciliation changed; refresh and retry")
			return
		}
		if errors.Is(err, ErrReconciliationAction) {
			writeError(w, http.StatusBadRequest, "validation_failed", "action is not allowed for this reconciliation")
			return
		}
		writeEntryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rec})
}

func writeEntryError(w http.ResponseWriter, r *http.Request, err error) {
	var validation EntryValidationErrors
	switch {
	case errors.As(err, &validation), errors.Is(err, ErrEntryValidation):
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
	case errors.Is(err, ErrEntryNotFound):
		writeError(w, http.StatusNotFound, "not_found", "finance entry not found")
	case errors.Is(err, ErrEntryConflict):
		writeError(w, http.StatusConflict, "conflict", "finance entry was updated by another writer; refresh and retry")
	case errors.Is(err, ErrEntryImmutable):
		writeError(w, http.StatusConflict, "immutable", "auto-generated finance entries cannot be edited or deleted; void them instead")
	case errors.Is(err, ErrEntryInactive):
		writeError(w, http.StatusConflict, "inactive", "voided finance entries cannot be edited or deleted")
	case errors.Is(err, ErrGeneratedEntryConflict):
		writeError(w, http.StatusConflict, "generated_entry_conflict", "generated finance entry retry does not match the existing active entry")
	default:
		writeInternalError(w, r, err)
	}
}
