package rider

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ServiceIface is the contract exposed to the HTTP layer. It is a
// package-internal interface; tests inject fakes.
type ServiceIface interface {
	ListTemplates(ctx context.Context) ([]*RiderTemplate, error)
	GetTemplate(ctx context.Context, id uuid.UUID) (*RiderTemplate, error)
	CreateTemplate(ctx context.Context, in CreateTemplateInput) (*RiderTemplate, error)
	UpdateTemplate(ctx context.Context, id uuid.UUID, in UpdateTemplateInput) (*RiderTemplate, error)
	DeleteTemplate(ctx context.Context, id uuid.UUID) error

	GetAttachmentByGig(ctx context.Context, gigID uuid.UUID) (*RiderAttachment, error)
	GetAttachment(ctx context.Context, id uuid.UUID) (*RiderAttachment, error)
	CreateAttachment(ctx context.Context, in CreateAttachmentInput) (*RiderAttachment, error)
	UpdateAttachment(ctx context.Context, id uuid.UUID, in UpdateAttachmentInput) (*RiderAttachment, error)
	DeleteAttachment(ctx context.Context, id uuid.UUID) error

	GeneratePDF(ctx context.Context, attachmentID uuid.UUID) (*ExportResult, error)
}

// Handler is the HTTP layer for the rider package.
type Handler struct {
	svc ServiceIface
}

// NewHandler creates a Handler backed by the given service. The gig details
// for PDFs come from the service's own GigReader (Service.SetGigReader), not
// from the handler.
func NewHandler(svc ServiceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns an http.Handler with all rider routes registered.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// Templates
	r.Get("/templates", h.handleListTemplates)
	r.Post("/templates", h.handleCreateTemplate)
	r.Get("/templates/{id}", h.handleGetTemplate)
	r.Put("/templates/{id}", h.handleUpdateTemplate)
	r.Delete("/templates/{id}", h.handleDeleteTemplate)

	// Attachments — static path registered before param path so chi's
	// matcher resolves /attachments/by-gig/{gigId} first.
	r.Get("/attachments/by-gig/{gigId}", h.handleGetAttachmentByGig)
	r.Post("/attachments", h.handleCreateAttachment)
	r.Get("/attachments/{id}", h.handleGetAttachment)
	r.Put("/attachments/{id}", h.handleUpdateAttachment)
	r.Delete("/attachments/{id}", h.handleDeleteAttachment)
	r.Post("/attachments/{id}/pdf", h.handleGeneratePDF)

	return r
}

// ─── Templates ─────────────────────────────────────────────────────────────

func (h *Handler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListTemplates(r.Context())
	if err != nil {
		h.writeInternalError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": templatesToJSON(out)})
}

func (h *Handler) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var body createTemplateRequest
	if !h.decodeJSON(w, r, &body) {
		return
	}
	tpl, err := h.svc.CreateTemplate(r.Context(), CreateTemplateInput{
		Name:               body.Name,
		RiderSectionValues: body.toSectionValues(),
	})
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			h.writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		h.writeInternalError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{"data": templateToJSON(tpl)})
}

func (h *Handler) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseUUID(w, chi.URLParam(r, "id"), "template")
	if err != nil {
		return
	}
	tpl, err := h.svc.GetTemplate(r.Context(), id)
	if err != nil {
		h.translateServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": templateToJSON(tpl)})
}

func (h *Handler) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseUUID(w, chi.URLParam(r, "id"), "template")
	if err != nil {
		return
	}
	var body updateTemplateRequest
	if !h.decodeJSON(w, r, &body) {
		return
	}
	tpl, err := h.svc.UpdateTemplate(r.Context(), id, UpdateTemplateInput{
		Name:              body.Name,
		RiderSectionPatch: body.sectionPatch(),
		UpdatedAt:         body.UpdatedAt,
	})
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			h.writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		h.translateServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": templateToJSON(tpl)})
}

func (h *Handler) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseUUID(w, chi.URLParam(r, "id"), "template")
	if err != nil {
		return
	}
	if err := h.svc.DeleteTemplate(r.Context(), id); err != nil {
		h.translateServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Attachments ────────────────────────────────────────────────────────────

func (h *Handler) handleGetAttachmentByGig(w http.ResponseWriter, r *http.Request) {
	gigID, err := h.parseUUID(w, chi.URLParam(r, "gigId"), "gig")
	if err != nil {
		return
	}
	att, err := h.svc.GetAttachmentByGig(r.Context(), gigID)
	if err != nil {
		// NotFound is the "no attachment yet" signal for this endpoint:
		// surface as 200 with data:null so the frontend renders an empty
		// state without an error path. Any other error is a real failure.
		if errors.Is(err, ErrNotFound) {
			h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": nil})
			return
		}
		h.translateServiceError(w, err)
		return
	}
	if att == nil {
		h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": nil})
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": attachmentToJSON(att)})
}

func (h *Handler) handleCreateAttachment(w http.ResponseWriter, r *http.Request) {
	var body createAttachmentRequest
	if !h.decodeJSON(w, r, &body) {
		return
	}
	gigID, err := uuid.Parse(body.GigID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid gigId")
		return
	}
	var templateID *uuid.UUID
	if body.TemplateID != nil && *body.TemplateID != "" {
		tid, err := uuid.Parse(*body.TemplateID)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid templateId")
			return
		}
		templateID = &tid
	}
	att, err := h.svc.CreateAttachment(r.Context(), CreateAttachmentInput{
		GigID:              gigID,
		TemplateID:         templateID,
		RiderSectionValues: body.toSectionValues(),
	})
	if err != nil {
		h.translateServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{"data": attachmentToJSON(att)})
}

func (h *Handler) handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseUUID(w, chi.URLParam(r, "id"), "attachment")
	if err != nil {
		return
	}
	att, err := h.svc.GetAttachment(r.Context(), id)
	if err != nil {
		h.translateServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": attachmentToJSON(att)})
}

func (h *Handler) handleUpdateAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseUUID(w, chi.URLParam(r, "id"), "attachment")
	if err != nil {
		return
	}
	var body updateAttachmentRequest
	if !h.decodeJSON(w, r, &body) {
		return
	}
	att, err := h.svc.UpdateAttachment(r.Context(), id, UpdateAttachmentInput{
		RiderSectionPatch: body.sectionPatch(),
		UpdatedAt:         body.UpdatedAt,
	})
	if err != nil {
		h.translateServiceError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": attachmentToJSON(att)})
}

func (h *Handler) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseUUID(w, chi.URLParam(r, "id"), "attachment")
	if err != nil {
		return
	}
	if err := h.svc.DeleteAttachment(r.Context(), id); err != nil {
		h.translateServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGeneratePDF(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseUUID(w, chi.URLParam(r, "id"), "attachment")
	if err != nil {
		return
	}
	res, err := h.svc.GeneratePDF(r.Context(), id)
	if err != nil {
		h.translateServiceError(w, err)
		return
	}
	// Match epk /export shape: bare id/downloadUrl/createdAt at top
	// level (no {data:...} envelope) — the frontend download UX is the
	// same component, so the wire shape must match.
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          res.ID.String(),
		"downloadUrl": res.DownloadURL,
		"createdAt":   res.CreatedAt.Format(time.RFC3339),
	})
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// parseUUID extracts a UUID from a chi route param, writes a 400 on
// parse failure, and returns the parsed UUID on success.
func (h *Handler) parseUUID(w http.ResponseWriter, raw, what string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid "+what+" id")
		return uuid.Nil, err
	}
	return id, nil
}

// translateServiceError maps service-layer sentinel errors to HTTP status
// codes. Centralises the pattern every handler funnels through.
func (h *Handler) translateServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		h.writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrConflict), errors.Is(err, ErrStaleUpdate):
		h.writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidInput):
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		h.writeInternalError(w, err)
	}
}

// writeInternalError logs the real cause and returns a generic 500. Raw
// database errors carry table, column and constraint names (and, for bad
// input, fragments of it) that must not reach API clients. Same approach as
// finance.writeInternalError.
func (h *Handler) writeInternalError(w http.ResponseWriter, err error) {
	slog.Error("rider: internal error", "err", err)
	h.writeError(w, http.StatusInternalServerError, "an internal error occurred")
}

// decodeJSON reads a request body into v with a size cap. Too large is 413;
// anything else that does not parse is 400. It reports whether the caller
// may continue (false means a response was already written).
func (h *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes {"error": "<code>", "message": "<text>"}, the shape the
// finance API and rider.Mux use. The code is derived from the status so
// clients can branch on it; message is for people.
func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": errorCode(status), "message": msg})
}

func errorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusUnprocessableEntity:
		return "validation_failed"
	case http.StatusInternalServerError:
		return "internal_error"
	default:
		return "error"
	}
}

// ─── JSON shapes ────────────────────────────────────────────────────────────

// createTemplateRequest matches the JSON the frontend POSTs.
type createTemplateRequest struct {
	Name        string `json:"name"`
	Technical   string `json:"technical"`
	Hospitality string `json:"hospitality"`
	Backline    string `json:"backline"`
	OtherNotes  string `json:"otherNotes"`
}

func (r *createTemplateRequest) toSectionValues() RiderSectionValues {
	return RiderSectionValues{
		Technical:   r.Technical,
		Hospitality: r.Hospitality,
		Backline:    r.Backline,
		OtherNotes:  r.OtherNotes,
	}
}

type updateTemplateRequest struct {
	UpdatedAt   time.Time `json:"updatedAt"`
	Name        *string `json:"name"`
	Technical   *string `json:"technical"`
	Hospitality *string `json:"hospitality"`
	Backline    *string `json:"backline"`
	OtherNotes  *string `json:"otherNotes"`
}

// sectionPatch forwards exactly the sections the client sent. A section that
// is absent stays nil (column untouched); one sent as "" is a pointer to ""
// (column cleared). Do not collapse these into plain strings.
func (r *updateTemplateRequest) sectionPatch() RiderSectionPatch {
	return RiderSectionPatch{
		Technical:   r.Technical,
		Hospitality: r.Hospitality,
		Backline:    r.Backline,
		OtherNotes:  r.OtherNotes,
	}
}

type createAttachmentRequest struct {
	GigID       string  `json:"gigId"`
	TemplateID  *string `json:"templateId"`
	Technical   string  `json:"technical"`
	Hospitality string  `json:"hospitality"`
	Backline    string  `json:"backline"`
	OtherNotes  string  `json:"otherNotes"`
}

func (r *createAttachmentRequest) toSectionValues() RiderSectionValues {
	return RiderSectionValues{
		Technical:   r.Technical,
		Hospitality: r.Hospitality,
		Backline:    r.Backline,
		OtherNotes:  r.OtherNotes,
	}
}

type updateAttachmentRequest struct {
	UpdatedAt   time.Time `json:"updatedAt"`
	Technical   *string `json:"technical"`
	Hospitality *string `json:"hospitality"`
	Backline    *string `json:"backline"`
	OtherNotes  *string `json:"otherNotes"`
}

// sectionPatch: see updateTemplateRequest.sectionPatch.
func (r *updateAttachmentRequest) sectionPatch() RiderSectionPatch {
	return RiderSectionPatch{
		Technical:   r.Technical,
		Hospitality: r.Hospitality,
		Backline:    r.Backline,
		OtherNotes:  r.OtherNotes,
	}
}

// ─── Response serialisation ─────────────────────────────────────────────────

func templateToJSON(t *RiderTemplate) map[string]interface{} {
	if t == nil {
		return nil
	}
	return map[string]interface{}{
		"id":          t.ID.String(),
		"name":        t.Name,
		"technical":   t.Technical,
		"hospitality": t.Hospitality,
		"backline":    t.Backline,
		"otherNotes":  t.OtherNotes,
		"createdAt":   t.CreatedAt.Format(time.RFC3339),
		"updatedAt":   t.UpdatedAt.Format(time.RFC3339),
	}
}

func templatesToJSON(ts []*RiderTemplate) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(ts))
	for _, t := range ts {
		out = append(out, templateToJSON(t))
	}
	return out
}

func attachmentToJSON(a *RiderAttachment) map[string]interface{} {
	if a == nil {
		return nil
	}
	m := map[string]interface{}{
		"id":          a.ID.String(),
		"gigId":       a.GigID.String(),
		"technical":   a.Technical,
		"hospitality": a.Hospitality,
		"backline":    a.Backline,
		"otherNotes":  a.OtherNotes,
		"createdAt":   a.CreatedAt.Format(time.RFC3339),
		"updatedAt":   a.UpdatedAt.Format(time.RFC3339),
	}
	if a.TemplateID != nil {
		m["templateId"] = a.TemplateID.String()
	} else {
		m["templateId"] = nil
	}
	return m
}

// strip removes ANSI control sequences. Currently unused but reserved
// for future template name normalisation (e.g. reject \r\n in name).
var _ = strings.TrimSpace