package finance

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// InvoiceHandler implements http.Handler for /api/v1/finance/invoices/*.
type InvoiceHandler struct {
	svc *InvoiceService
}

// NewInvoiceHandler wires an InvoiceHandler.
func NewInvoiceHandler(svc *InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{svc: svc}
}

// ServeHTTP routes (docs/INVOICING.md §4):
//
//	GET    /invoices                     list (?status=&gig_id=&kind=&currency=)
//	POST   /invoices                     create draft
//	GET    /invoices/tax-suggestion      suggest VAT treatment for a customer
//	GET    /invoices/summaries           per-currency dashboard
//	GET    /invoices/{id}                invoice + lines
//	PUT    /invoices/{id}                update draft (totals recomputed)
//	GET    /invoices/{id}/issue-check    issue readiness
//	POST   /invoices/{id}/issue          draft → issued (number allocated)
//	POST   /invoices/{id}/pay            issued → paid
//	POST   /invoices/{id}/cancel         draft → cancelled
//	POST   /invoices/{id}/credit-note    issued|paid → credited + credit note
//	POST   /invoices/{id}/correct        issued|paid → corrected + credit note + new draft
func (h *InvoiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const base = "/api/v1/finance/invoices"
	path := r.URL.Path
	if len(path) < len(base) || path[:len(base)] != base || (len(path) > len(base) && path[len(base)] != '/') {
		writeError(w, http.StatusNotFound, "not_found", "invalid invoice path")
		return
	}
	parts := splitPath(path[len(base):])

	switch {
	case len(parts) == 0:
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r)
		case http.MethodPost:
			h.handleCreateDraft(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST supported on collection")
		}
		return
	case len(parts) == 1 && (parts[0] == "summaries" || parts[0] == "tax-suggestion"):
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET supported on "+parts[0])
			return
		}
		if parts[0] == "summaries" {
			h.handleSummaries(w, r)
		} else {
			h.handleTaxSuggestion(w, r)
		}
		return
	case len(parts) > 2:
		writeError(w, http.StatusNotFound, "not_found", "invalid invoice path")
		return
	}

	id, err := uuid.Parse(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
		return
	}
	if len(parts) == 1 {
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

	action := parts[1]
	if action == "issue-check" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET supported on issue-check")
			return
		}
		h.handleIssueCheck(w, r, id)
		return
	}
	handlers := map[string]func(http.ResponseWriter, *http.Request, uuid.UUID){
		"issue":       h.handleIssue,
		"pay":         h.handlePay,
		"cancel":      h.handleCancel,
		"credit-note": h.handleCreditNote,
		"correct":     h.handleCorrect,
	}
	fn, ok := handlers[action]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown invoice action: "+action)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST supported on "+action)
		return
	}
	fn(w, r, id)
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

// decodeBody reads a JSON body (1 MiB cap). It writes the 400 and returns
// false on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return false
	}
	return true
}

// requireToken writes a 400 when the optimistic-concurrency token is missing.
func requireToken(w http.ResponseWriter, t time.Time) bool {
	if t.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return false
	}
	return true
}

// writeInvoiceError maps service errors to the documented status codes.
func writeInvoiceError(w http.ResponseWriter, r *http.Request, err error) {
	var notIssuable *NotIssuableError
	switch {
	case errors.As(err, &notIssuable):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": "not_issuable", "message": "invoice is not ready to be issued", "problems": notIssuable.Problems,
		})
	case errors.Is(err, ErrInvoiceNotFound):
		writeError(w, http.StatusNotFound, "not_found", "invoice not found")
	case errors.Is(err, ErrInvoiceValidation):
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
	case errors.Is(err, ErrInvoiceConflict):
		writeError(w, http.StatusConflict, "conflict", "invoice was updated by another writer; refresh and retry")
	case errors.Is(err, ErrInvoiceBadState):
		writeError(w, http.StatusConflict, "bad_state", "action not allowed in the invoice's current status")
	default:
		writeInternalError(w, r, err)
	}
}

func (h *InvoiceHandler) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := InvoiceFilter{}
	if gigID := q.Get("gig_id"); gigID != "" {
		id, err := uuid.Parse(gigID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_failed", "gig_id must be a UUID")
			return
		}
		filter.GigID = &id
	}
	if status := q.Get("status"); status != "" {
		filter.Status = InvoiceStatus(status)
		if !filter.Status.IsValid() {
			writeError(w, http.StatusBadRequest, "validation_failed", "unknown status "+status)
			return
		}
	}
	if kind := q.Get("kind"); kind != "" {
		filter.Kind = InvoiceKind(kind)
		if !filter.Kind.IsValid() {
			writeError(w, http.StatusBadRequest, "validation_failed", "kind must be invoice or credit_note")
			return
		}
	}
	filter.Currency = q.Get("currency")
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
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": invs})
}

func (h *InvoiceHandler) handleCreateDraft(w http.ResponseWriter, r *http.Request) {
	var req CreateInvoiceRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.GigID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "gig_id is required")
		return
	}
	inv, err := h.svc.CreateDraft(r.Context(), req.GigID, req)
	if err != nil {
		if errors.Is(err, ErrInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "gig not found")
			return
		}
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleTaxSuggestion(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	isBusiness, _ := strconv.ParseBool(q.Get("customer_is_business"))
	sugg, err := h.svc.TaxSuggestion(r.Context(), Party{
		Country:    q.Get("customer_country"),
		VATID:      q.Get("customer_vat_id"),
		IsBusiness: isBusiness,
	})
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": sugg})
}

func (h *InvoiceHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	inv, lines, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	if lines == nil {
		lines = []*InvoiceLine{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv, "lines": lines})
}

func (h *InvoiceHandler) handleUpdateDraft(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req UpdateInvoiceRequest
	if !decodeBody(w, r, &req) || !requireToken(w, req.UpdatedAt) {
		return
	}
	inv, err := h.svc.UpdateDraft(r.Context(), id, req)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleIssueCheck(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	check, err := h.svc.IssueCheck(r.Context(), id)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": check})
}

func (h *InvoiceHandler) handleIssue(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req IssueInvoiceRequest
	if !decodeBody(w, r, &req) || !requireToken(w, req.UpdatedAt) {
		return
	}
	inv, err := h.svc.Issue(r.Context(), id, req)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handlePay(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req PayInvoiceRequest
	if !decodeBody(w, r, &req) || !requireToken(w, req.UpdatedAt) {
		return
	}
	if req.PaidAt.IsZero() {
		req.PaidAt = time.Now().UTC()
	}
	inv, err := h.svc.Pay(r.Context(), id, req)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleCancel(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req CancelInvoiceRequest
	if !decodeBody(w, r, &req) || !requireToken(w, req.UpdatedAt) {
		return
	}
	inv, err := h.svc.Cancel(r.Context(), id, req)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inv})
}

func (h *InvoiceHandler) handleCreditNote(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req CreditNoteRequest
	if !decodeBody(w, r, &req) || !requireToken(w, req.UpdatedAt) {
		return
	}
	res, err := h.svc.CreditNote(r.Context(), id, req)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": res})
}

func (h *InvoiceHandler) handleCorrect(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req CorrectInvoiceRequest
	if !decodeBody(w, r, &req) || !requireToken(w, req.UpdatedAt) {
		return
	}
	res, err := h.svc.Correct(r.Context(), id, req)
	if err != nil {
		writeInvoiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": res})
}

// handleSummaries returns per-currency dashboard aggregates keyed by currency.
func (h *InvoiceHandler) handleSummaries(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.svc.Summaries(r.Context())
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": summaries})
}
