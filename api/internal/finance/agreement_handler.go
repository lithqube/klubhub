package finance

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// AgreementTemplateHandler implements http.Handler for
// /api/v1/finance/agreements/templates and /api/v1/finance/agreements/templates/{id}.
type AgreementTemplateHandler struct {
	svc *AgreementTemplateService
}

func NewAgreementTemplateHandler(svc *AgreementTemplateService) *AgreementTemplateHandler {
	return &AgreementTemplateHandler{svc: svc}
}

func (h *AgreementTemplateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Strip the prefix that may or may not end with a slash
	prefix := "/api/v1/finance/agreements/templates"
	path := strings.TrimPrefix(r.URL.Path, prefix)
	path = strings.Trim(path, "/")

	if path == "" {
		// /api/v1/finance/agreements/templates
		switch r.Method {
		case http.MethodPost:
			h.handleCreate(w, r)
		case http.MethodGet:
			activeOnly := r.URL.Query().Get("active") == "true"
			h.handleList(w, r, activeOnly)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST and GET supported")
		}
		return
	}

	// /api/v1/finance/agreements/templates/{id}
	id, err := uuid.Parse(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
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
}

func (h *AgreementTemplateHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CreateAgreementTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.CreatedBy == "" {
		req.CreatedBy = "system"
	}
	tpl, err := h.svc.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrAgreementValidation) {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": tpl})
}

func (h *AgreementTemplateHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	tpl, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrAgreementTemplateNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "template not found")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tpl})
}

func (h *AgreementTemplateHandler) handleList(w http.ResponseWriter, r *http.Request, activeOnly bool) {
	tpls, err := h.svc.List(r.Context(), activeOnly)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tpls})
}

func (h *AgreementTemplateHandler) handleUpdate(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req UpdateAgreementTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	tpl, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrAgreementConflict) {
			writeError(w, http.StatusConflict, "conflict", "template updated by another writer")
			return
		}
		if errors.Is(err, ErrAgreementValidation) {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tpl})
}

// AgreementInstanceHandler implements http.Handler for
// /api/v1/finance/agreements/instances/{id?} and /api/v1/finance/agreements/instances/{id}/sign.
type AgreementInstanceHandler struct {
	svc *AgreementInstanceService
}

func NewAgreementInstanceHandler(svc *AgreementInstanceService) *AgreementInstanceHandler {
	return &AgreementInstanceHandler{svc: svc}
}

func (h *AgreementInstanceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "/api/v1/finance/agreements/instances"
	path := strings.TrimPrefix(r.URL.Path, prefix)
	path = strings.Trim(path, "/")

	if path == "" {
		// /api/v1/finance/agreements/instances
		switch r.Method {
		case http.MethodPost:
			h.handleCreate(w, r)
		case http.MethodGet:
			status := AgreementStatus(r.URL.Query().Get("status"))
			h.handleList(w, r, status)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST and GET supported")
		}
		return
	}

	parts := strings.Split(path, "/")
	id, err := uuid.Parse(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	if len(parts) == 1 {
		// /api/v1/finance/agreements/instances/{id}
		switch r.Method {
		case http.MethodGet:
			h.handleGet(w, r, id)
		case http.MethodPut:
			h.handleUpdate(w, r, id)
		case http.MethodPost:
			h.handleGeneratePDF(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET, PUT, POST supported")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "sign" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST supported")
			return
		}
		h.handleSign(w, r, id)
		return
	}

	writeError(w, http.StatusNotFound, "not_found", "invalid path")
}

func (h *AgreementInstanceHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CreateAgreementInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	inst, err := h.svc.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrAgreementValidation) {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		if errors.Is(err, ErrAgreementTemplateNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "template not found")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": inst})
}

func (h *AgreementInstanceHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	inst, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrAgreementInstanceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "instance not found")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inst})
}

func (h *AgreementInstanceHandler) handleList(w http.ResponseWriter, r *http.Request, status AgreementStatus) {
	var insts []*AgreementInstance
	var err error
	if r.URL.Query().Get("gig_id") != "" {
		gigID, parseErr := uuid.Parse(r.URL.Query().Get("gig_id"))
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid gig_id")
			return
		}
		insts, err = h.svc.GetByGigID(r.Context(), gigID)
	} else {
		insts, err = h.svc.List(r.Context(), status)
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": insts})
}

func (h *AgreementInstanceHandler) handleUpdate(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req UpdateAgreementInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	inst, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrAgreementConflict) {
			writeError(w, http.StatusConflict, "conflict", "instance updated by another writer")
			return
		}
		if errors.Is(err, ErrAgreementValidation) {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		if errors.Is(err, ErrAgreementInstanceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "instance not found")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inst})
}

func (h *AgreementInstanceHandler) handleSign(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req SignAgreementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "bad_request", "updated_at is required")
		return
	}
	inst, err := h.svc.Sign(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrAgreementConflict) {
			writeError(w, http.StatusConflict, "conflict", "instance updated by another writer")
			return
		}
		if errors.Is(err, ErrAgreementValidation) {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		if errors.Is(err, ErrAgreementBadState) {
			writeError(w, http.StatusBadRequest, "bad_state", "agreement cannot be signed in its current state")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inst})
}

func (h *AgreementInstanceHandler) handleGeneratePDF(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if h.svc.docService == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "document service not configured")
		return
	}
	doc, err := h.svc.GeneratePDF(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrAgreementInstanceNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "instance not found")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": doc})
}

// EmailHandler implements http.Handler for /api/v1/finance/emails and /api/v1/finance/emails/{id}.
type EmailHandler struct {
	svc *EmailService
}

func NewEmailHandler(svc *EmailService) *EmailHandler {
	return &EmailHandler{svc: svc}
}

func (h *EmailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/finance/emails")
	path = strings.Trim(path, "/")

	if path == "" {
		// /api/v1/finance/emails
		if r.Method == http.MethodPost {
			h.handleSend(w, r)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST supported")
		return
	}

	id, err := uuid.Parse(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if r.Method == http.MethodGet {
		h.handleGet(w, r, id)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET supported")
}

func (h *EmailHandler) handleSend(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req CreateEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	// Default to Send (Enqueue + deliver) for the POST /emails endpoint.
	msg, err := h.svc.Send(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrEmailValidation) {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		writeInternalError(w, r, err)
		return
	}
	// Status code depends on delivery outcome
	if msg.Status == EmailStatusFailed {
		writeJSON(w, http.StatusBadGateway, map[string]any{"data": msg})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": msg})
}

func (h *EmailHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	msg, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrEmailNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "email not found")
			return
		}
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": msg})
}

// Silence unused-import warning when strconv is referenced later.
var _ = strconv.Atoi
