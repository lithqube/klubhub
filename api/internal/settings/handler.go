package settings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	// Importing "net/http" shadows stdlib when this package also uses
	// the same name; alias the platform one for clarity.
	platformhttp "github.com/klubhub/dj/api/internal/platform/http"
)

// settingsServiceIface allows handler to use a real or test service.
type settingsServiceIface interface {
	GetOrCreate(ctx context.Context) (*UserSettings, error)
	Update(ctx context.Context, req UpdateSettingsRequest) (*UserSettings, error)
}

// Handler implements http.Handler for GET and PUT /api/v1/settings.
type Handler struct {
	svc settingsServiceIface
}

// NewHandler creates a new settings Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ServeHTTP routes GET → Get, PUT → Update.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPut:
		h.handlePut(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	us, err := h.svc.GetOrCreate(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, us)
}

func (h *Handler) handlePut(w http.ResponseWriter, r *http.Request) {
	// Settings payloads are small JSON documents. Apply a tight
	// route-specific cap even though the router's outer ceiling is 50 MiB.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Plan B.6: translate cap overflows into 413 so the middleware
		// can be observed end-to-end.
		if status := platformhttp.DetectMaxBytes413(err); status != http.StatusBadRequest {
			writeError(w, status, "request_too_large", "request body exceeds limit")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	updated, err := h.svc.Update(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			writeError(w, http.StatusConflict, "conflict",
				"Settings were modified by another request. Refresh and retry.")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// writeJSON serialises v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeError writes a standard JSON error envelope.
func writeError(w http.ResponseWriter, status int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"error":   errCode,
		"message": message,
	})
}
