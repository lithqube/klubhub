package finance

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

// PaymentHandler implements http.Handler for /api/v1/finance/invoices/{id}/payments and /api/v1/finance/payments/{id}
type PaymentHandler struct {
	svc *PaymentService
}

// NewPaymentHandler wires a PaymentHandler.
func NewPaymentHandler(svc *PaymentService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

// ServeHTTP routes:
// POST   /api/v1/finance/invoices/{invoice_id}/payments   -> create payment
// GET    /api/v1/finance/invoices/{invoice_id}/payments   -> list payments for invoice
// GET    /api/v1/finance/payments/{id}                    -> get payment by id
// PUT    /api/v1/finance/payments/{id}                    -> update payment
func (h *PaymentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	const invoicePaymentsBase = "/api/v1/finance/invoices/"
	const paymentsBase = "/api/v1/finance/payments/"

	// /api/v1/finance/invoices/{id}/payments
	if len(path) > len(invoicePaymentsBase) && path[:len(invoicePaymentsBase)] == invoicePaymentsBase {
		rest := path[len(invoicePaymentsBase):]
		parts := splitPath(rest)
		if len(parts) == 2 && parts[1] == "payments" {
			invoiceID, err := uuid.Parse(parts[0])
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
				return
			}
			switch r.Method {
			case http.MethodPost:
				h.handleCreate(w, r, invoiceID)
			case http.MethodGet:
				h.handleList(w, r, invoiceID)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST and GET supported")
			}
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "invalid path")
		return
	}

	// /api/v1/finance/payments/{id}
	if len(path) > len(paymentsBase) && path[:len(paymentsBase)] == paymentsBase {
		idStr := path[len(paymentsBase):]
		parts := splitPath(idStr)
		if len(parts) == 1 {
			id, err := uuid.Parse(parts[0])
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid payment id")
				return
			}
			switch r.Method {
			case http.MethodGet:
				h.handleGet(w, r, id)
			case http.MethodPut:
				h.handleUpdate(w, r, id)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and PUT supported")
			}
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "invalid payment path")
		return
	}

	writeError(w, http.StatusNotFound, "not_found", "invalid payment path")
}

func (h *PaymentHandler) handleCreate(w http.ResponseWriter, r *http.Request, invoiceID uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CreatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	p, err := h.svc.Create(r.Context(), invoiceID, req)
	if err != nil {
		var vErrs PaymentValidationErrors
		if errors.As(err, &vErrs) {
			writeError(w, http.StatusBadRequest, "validation_failed", vErrs.Error())
			return
		}
		if errors.Is(err, ErrPaymentInvoiceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		if writePaymentRuleError(w, err) {
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": p})
}

func (h *PaymentHandler) handleList(w http.ResponseWriter, r *http.Request, invoiceID uuid.UUID) {
	payments, err := h.svc.ListByInvoice(r.Context(), invoiceID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": payments})
}

func (h *PaymentHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	p, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPaymentNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "payment not found")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

func (h *PaymentHandler) handleUpdate(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req UpdatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	p, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrPaymentConflict) {
			writeError(w, http.StatusConflict, "conflict", "payment updated by another writer")
			return
		}
		if errors.Is(err, ErrPaymentNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "payment not found")
			return
		}
		var vErrs PaymentValidationErrors
		if errors.As(err, &vErrs) {
			writeError(w, http.StatusBadRequest, "validation_failed", vErrs.Error())
			return
		}
		if writePaymentRuleError(w, err) {
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

// writePaymentRuleError maps business-rule rejections to 422. Returns
// false when err is not one of them.
func writePaymentRuleError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, ErrPaymentInvoiceState):
		writeError(w, http.StatusUnprocessableEntity, "invoice_not_payable", err.Error())
	case errors.Is(err, ErrPaymentExceedsBalance):
		writeError(w, http.StatusUnprocessableEntity, "exceeds_balance", err.Error())
	default:
		return false
	}
	return true
}

var _ = json.Marshal
