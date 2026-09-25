package finance

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// InvoiceHandler implements http.Handler for /api/v1/finance/invoices/*
type InvoiceHandler struct {
	svc *InvoiceService
}

// NewInvoiceHandler wires an InvoiceHandler.
func NewInvoiceHandler(svc *InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{svc: svc}
}

// ServeHTTP routes:
// GET    /api/v1/finance/invoices              -> list (with query filters)
// POST   /api/v1/finance/invoices              -> create draft
// GET    /api/v1/finance/invoices/{id}         -> get by id
// PUT    /api/v1/finance/invoices/{id}         -> update draft
// POST   /api/v1/finance/invoices/{id}/issue   -> issue (draft -> issued)
// POST   /api/v1/finance/invoices/{id}/pay     -> pay (issued -> paid)
// POST   /api/v1/finance/invoices/{id}/cancel  -> cancel (draft/issued -> cancelled)
// POST   /api/v1/finance/invoices/{id}/correct -> correct (issued/paid -> corrected + new draft)
func (h *InvoiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/v1/finance/invoices[/{id}[/action]] or /api/v1/finance/invoices/summaries
	path := r.URL.Path
	const base = "/api/v1/finance/invoices"
	if path == base || path == base+"/summaries" {
		// /api/v1/finance/invoices/summaries → per-currency dashboard
		if path == base+"/summaries" && r.Method == http.MethodGet {
			h.handleSummaries(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r)
		case http.MethodPost:
			h.handleCreateDraft(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST supported on collection")
		}
		return
	}

	// Has an ID and maybe an action
	if len(path) <= len(base)+1 || path[len(base)] != '/' {
		writeError(w, http.StatusNotFound, "not_found", "invalid invoice path")
		return
	}
	rest := path[len(base)+1:] // after /invoices/
	parts := splitPath(rest)
	if len(parts) == 1 {
		// /invoices/{id}
		id, err := uuid.Parse(parts[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.handleGet(w, r, id)
		case http.MethodPut:
			h.handleUpdateDraft(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and PUT supported on invoice")
		}
		return
	}
	if len(parts) == 2 {
		// /invoices/{id}/{action}
		id, err := uuid.Parse(parts[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
			return
		}
		action := parts[1]
		switch action {
		case "issue":
			if r.Method == http.MethodPost {
				h.handleIssue(w, r, id)
				return
			}
		case "pay":
			if r.Method == http.MethodPost {
				h.handlePay(w, r, id)
				return
			}
		case "cancel":
			if r.Method == http.MethodPost {
				h.handleCancel(w, r, id)
				return
			}
		case "correct":
			if r.Method == http.MethodPost {
				h.handleCorrect(w, r, id)
				return
			}
		}
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST supported on "+action)
		return
	}

	writeError(w, http.StatusNotFound, "not_found", "invalid invoice path")
}

func splitPath(s string) []string {
	var out []string
	for _, p := range splitBytes(s, '/') {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitBytes(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func (h *InvoiceHandler) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := InvoiceFilter{}

	if gigID := q.Get("gig_id"); gigID != "" {
		if id, err := uuid.Parse(gigID); err == nil {
			filter.GigID = &id
		}
	}
	if status := q.Get("status"); status != "" {
		filter.Status = InvoiceStatus(status)
	}
	if currency := q.Get("currency"); currency != "" {
		filter.Currency = currency
	}
	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			filter.From = t
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = t
		}
	}

	invs, err := h.svc.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": invs})
}

func (h *InvoiceHandler) handleCreateDraft(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CreateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.GigID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "bad_request", "gig_id is required")
		return
	}
	if len(req.Currency) != 3 {
		writeError(w, http.StatusBadRequest, "bad_request", "currency must be a 3-letter ISO 4217 code")
		return
	}

	inv, err := h.svc.CreateDraft(r.Context(), req.GigID, req)
	if err != nil {
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "gig not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	inv, lines, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"invoice": inv, "lines": lines}})
}

func (h *InvoiceHandler) handleUpdateDraft(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req UpdateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	inv, err := h.svc.UpdateDraft(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrInvoiceConflict) {
			writeError(w, http.StatusConflict, "conflict", "invoice updated by another writer or not a draft")
			return
		}
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleIssue(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req IssueInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	inv, err := h.svc.Issue(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrInvoiceConflict) {
			writeError(w, http.StatusConflict, "conflict", "invoice updated by another writer or not a draft")
			return
		}
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handlePay(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req PayInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	inv, err := h.svc.Pay(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrInvoiceConflict) {
			writeError(w, http.StatusConflict, "conflict", "invoice updated by another writer or not issued")
			return
		}
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleCancel(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CancelInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	inv, err := h.svc.Cancel(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrInvoiceConflict) {
			writeError(w, http.StatusConflict, "conflict", "invoice updated by another writer or wrong state")
			return
		}
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleCorrect(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CorrectInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	inv, err := h.svc.Correct(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrInvoiceConflict) {
			writeError(w, http.StatusConflict, "conflict", "invoice updated by another writer or not issued/paid")
			return
		}
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		if errors.Is(err, ErrInvoiceBadState) {
			writeError(w, http.StatusBadRequest, "bad_state", "only issued or paid invoices can be corrected")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

// handleSummaries returns per-currency dashboard aggregates.
func (h *InvoiceHandler) handleSummaries(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.svc.Summaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out := make([]CurrencySummary, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}