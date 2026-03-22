package epk

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for the EPK package.
type Handler struct {
	svc serviceIface
}

// NewHandler creates a Handler backed by the given service.
func NewHandler(svc serviceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns an http.Handler with all EPK routes registered.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/content", h.handleGetContent)
	r.Put("/content", h.handlePutContent)
	r.Post("/photos", h.handlePostPhoto)
	r.Delete("/photos/{path}", h.handleDeletePhoto)
	r.Post("/stage-plot", h.handlePostStagePlot)
	r.Post("/export", h.handlePostExport)
	r.Get("/exports", h.handleGetExports)
	r.Delete("/exports/{id}", h.handleDeleteExport)

	return r
}

// handleGetContent returns the singleton EPK content.
func (h *Handler) handleGetContent(w http.ResponseWriter, r *http.Request) {
	content, err := h.svc.GetContent(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, contentToJSON(content))
}

// handlePutContent accepts a partial JSON body and returns the updated EPKContent.
func (h *Handler) handlePutContent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BioShort          *string          `json:"bio_short"`
		BioLong           *string          `json:"bio_long"`
		TechRider         *string          `json:"tech_rider"`
		StagePlotPath     *string          `json:"stage_plot_path"`
		GigHighlights     []string         `json:"gig_highlights"`
		PressQuotes       []PressQuote     `json:"press_quotes"`
		PhotoPaths        []string         `json:"photo_paths"`
		SectionVisibility map[string]bool  `json:"section_visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req := UpsertEPKContentRequest{
		BioShort:          body.BioShort,
		BioLong:           body.BioLong,
		TechRider:         body.TechRider,
		StagePlotPath:     body.StagePlotPath,
		GigHighlights:     body.GigHighlights,
		PressQuotes:       body.PressQuotes,
		PhotoPaths:        body.PhotoPaths,
		SectionVisibility: body.SectionVisibility,
	}

	content, err := h.svc.UpsertContent(r.Context(), req)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, contentToJSON(content))
}

// handlePostPhoto accepts multipart/form-data with a "photo" field.
func (h *Handler) handlePostPhoto(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10<<20 + 512); err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "photo field required")
		return
	}
	defer file.Close()

	data := make([]byte, header.Size)
	if _, err := file.Read(data); err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to read photo data")
		return
	}

	mimeType := header.Header.Get("Content-Type")

	path, err := h.svc.UploadPhoto(r.Context(), data, mimeType)
	if err != nil {
		if errors.Is(err, ErrPhotoLimitExceeded) {
			h.writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if errors.Is(err, ErrInvalidMIME) {
			h.writeError(w, http.StatusUnsupportedMediaType, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"path": path})
}

// handleDeletePhoto removes a photo by URL-encoded path.
func (h *Handler) handleDeletePhoto(w http.ResponseWriter, r *http.Request) {
	rawPath := chi.URLParam(r, "path")
	photoPath, err := url.QueryUnescape(rawPath)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid path encoding")
		return
	}

	if err := h.svc.DeletePhoto(r.Context(), photoPath); err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "photo not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handlePostStagePlot accepts multipart/form-data with an "image" field.
func (h *Handler) handlePostStagePlot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10<<20 + 512); err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "image field required")
		return
	}
	defer file.Close()

	data := make([]byte, header.Size)
	if _, err := file.Read(data); err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to read image data")
		return
	}

	mimeType := header.Header.Get("Content-Type")

	path, err := h.svc.UploadStagePlot(r.Context(), data, mimeType)
	if err != nil {
		if errors.Is(err, ErrInvalidMIME) {
			h.writeError(w, http.StatusUnsupportedMediaType, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handlePostExport generates a new PDF export and returns {id, downloadUrl, createdAt}.
func (h *Handler) handlePostExport(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GeneratePDF(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          result.ID.String(),
		"downloadUrl": result.DownloadURL,
		"createdAt":   result.CreatedAt.Format(time.RFC3339),
	})
}

// handleGetExports returns the export history list ordered newest first.
func (h *Handler) handleGetExports(w http.ResponseWriter, r *http.Request) {
	exports, err := h.svc.ListExports(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]map[string]interface{}, 0, len(exports))
	for _, e := range exports {
		resp = append(resp, map[string]interface{}{
			"id":        e.ID.String(),
			"minioPath": e.MinioPath,
			"createdAt": e.CreatedAt.Format(time.RFC3339),
		})
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// handleDeleteExport deletes an export record and MinIO object.
func (h *Handler) handleDeleteExport(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid export id")
		return
	}

	if err := h.svc.DeleteExport(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "export not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ─── Response helpers ─────────────────────────────────────────────────────────

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}

// contentToJSON converts EPKContent to a JSON-serialisable map.
func contentToJSON(c *EPKContent) map[string]interface{} {
	if c == nil {
		return nil
	}
	return map[string]interface{}{
		"id":                 c.ID.String(),
		"bio_short":          c.BioShort,
		"bio_long":           c.BioLong,
		"tech_rider":         c.TechRider,
		"stage_plot_path":    c.StagePlotPath,
		"gig_highlights":     c.GigHighlights,
		"press_quotes":       c.PressQuotes,
		"photo_paths":        c.PhotoPaths,
		"section_visibility": c.SectionVisibility,
		"created_at":         c.CreatedAt.Format(time.RFC3339),
		"updated_at":         c.UpdatedAt.Format(time.RFC3339),
	}
}
