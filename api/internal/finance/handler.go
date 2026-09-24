package finance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// ServiceIface is the subset of Service the handler uses.
type ServiceIface interface {
	Get(ctx context.Context) (*BillingProfile, error)
	Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error)
}

// Handler implements http.Handler for /api/v1/finance/billing-profile.
// GET returns the singleton profile; PUT replaces it under an optimistic
// concurrency token. Both endpoints return the same envelope shape
// `{ "data": ... }` as the rest of the v1 API.
type Handler struct {
	svc ServiceIface
}

// NewHandler wires a Handler.
func NewHandler(svc ServiceIface) *Handler { return &Handler{svc: svc} }

// ServeHTTP routes GET → handleGet, PUT → handleUpdate.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPut:
		h.handleUpdate(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and PUT are supported")
	}
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "billing profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

// handleUpdate enforces a 1 MiB body cap even though the router's outer
// ceiling is much larger.
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req UpdateBillingProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	p, err := h.svc.Update(r.Context(), &req)
	if err != nil {
		var vErrs ValidationErrors
		if errors.As(err, &vErrs) {
			writeError(w, http.StatusBadRequest, "validation_failed", vErrs.Error())
			return
		}
		if errors.Is(err, ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "billing profile updated by another writer; refresh and retry")
			return
		}
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "billing profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}
