package epk

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/ra"
)

// Handler handles HTTP requests for the EPK package.
type Handler struct {
	svc      serviceIface
	raClient *ra.RAClient
}

// NewHandler creates a Handler backed by the given service and RA client.
func NewHandler(svc serviceIface, raClient *ra.RAClient) *Handler {
	if raClient == nil {
		raClient = ra.NewRAClient()
	}
	return &Handler{
		svc:      svc,
		raClient: raClient,
	}
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
	r.Post("/import-ra", h.HandleImportFromRA)

	return r
}

// HandleImportFromRA handles POST /api/v1/epk/import-ra
func (h *Handler) HandleImportFromRA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ArtistSlug string `json:"artist_slug"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if body.ArtistSlug == "" {
		h.writeError(w, http.StatusBadRequest, "artist_slug is required")
		return
	}

		// Use the injected RA client (not a new instance, to preserve cache)
		artist, err := h.raClient.GetArtist(r.Context(), body.ArtistSlug)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "failed to fetch artist from RA: "+err.Error())
			return
		}

	// Build result
	result := RAImportResult{
		Success:      true,
		ArtistName:   artist.Name,
		Biography:    artist.Biography.Blurb,
		Instagram:    artist.Instagram,
		Soundcloud:   artist.Soundcloud,
		Bandcamp:     artist.Bandcamp,
		Discogs:      artist.Discogs,
		Website:      artist.Website,
		Facebook:     artist.Facebook,
		Twitter:      artist.Twitter,
		Followers:    artist.Followers,
		HeaderImage:  artist.HeaderImage,
		ProfileImage: artist.ProfileImage,
		NeedsUpdate:  true,
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": result,
	})
}

// handleGetContent returns the singleton EPK content.
func (h *Handler) handleGetContent(w http.ResponseWriter, r *http.Request) {
	content, err := h.svc.GetContent(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": contentToJSON(content)})
}

// handlePutContent accepts a partial JSON body and returns the updated EPKContent.
func (h *Handler) handlePutContent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BioShort          *string         `json:"bioShort"`
		BioLong           *string         `json:"bioLong"`
		TechRider         *string         `json:"techRider"`
		StagePlotPath     *string         `json:"stagePlotPath"`
		GigHighlights     []string        `json:"gigHighlights"`
		PressQuotes       []PressQuote    `json:"pressQuotes"`
		PhotoPaths        []string        `json:"photoPaths"`
		SectionVisibility map[string]bool `json:"sectionVisibility"`
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
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": contentToJSON(content)})
}

// handlePostPhoto accepts multipart/form-data with a "photo" field.
func (h *Handler) handlePostPhoto(w http.ResponseWriter, r *http.Request) {
	// ParseMultipartForm's maxMemory is not a request-size limit; apply
	// an explicit cap first (10 MiB payload + 512 bytes form overhead).
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20+512)
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

// handlePostStagePlot handles multipart/form-data with a "stagePlot" field.
func (h *Handler) handlePostStagePlot(w http.ResponseWriter, r *http.Request) {
	// Same transport cap as photos; prevents disk-spooling arbitrarily
	// large multipart bodies before service validation.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20+512)
	if err := r.ParseMultipartForm(10<<20 + 512); err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("stagePlot")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "stagePlot field required")
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

	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": resp})
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
		"id":                c.ID.String(),
		"bioShort":          c.BioShort,
		"bioLong":           c.BioLong,
		"techRider":         c.TechRider,
		"stagePlotPath":     c.StagePlotPath,
		"gigHighlights":     c.GigHighlights,
		"pressQuotes":       c.PressQuotes,
		"photoPaths":        c.PhotoPaths,
		"sectionVisibility": c.SectionVisibility,
		"createdAt":         c.CreatedAt.Format(time.RFC3339),
		"updatedAt":         c.UpdatedAt.Format(time.RFC3339),
	}
}
