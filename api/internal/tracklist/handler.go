package tracklist

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// tracklistServiceIface defines the interface for the tracklist service
type tracklistServiceIface interface {
	Upload(ctx context.Context, filename string, body []byte, size int64) (*Tracklist, []Track, []ParseWarning, error)
	GetWithTracks(ctx context.Context, id uuid.UUID) (*Tracklist, []Track, error)
	List(ctx context.Context) ([]Tracklist, error)
	UpdateTrack(ctx context.Context, tracklistID, trackID uuid.UUID, req UpdateTrackRequest) (*Track, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	SaveManualArtwork(ctx context.Context, tracklistID, trackID uuid.UUID, imageData []byte, contentType string) error
}

// Handler handles HTTP requests for tracklists
type Handler struct {
	svc tracklistServiceIface
}

// NewHandler creates a new Handler with the given service
func NewHandler(svc tracklistServiceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns a chi router with all tracklist routes
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/upload", h.handleUpload)
	r.Get("/", h.handleList)
	r.Get("/{id}", h.handleGet)
	r.Put("/{id}/tracks/{track_id}", h.handleUpdateTrack)
	r.Delete("/{id}", h.handleDelete)
	r.Put("/{id}/tracks/{track_id}/artwork", h.handleTrackArtwork)
	return r
}

// handleUpload handles POST /upload
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	// Limit the request body to 50 MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20) // 50 MB
	defer r.Body.Close()

	// Parse multipart form
	err := r.ParseMultipartForm(50 << 20) // 50 MB
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get the file
	file, header, err := r.FormFile("file")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	// Get file size
	size := header.Size

	// Call service.Upload
	tracklist, tracks, warnings, err := h.svc.Upload(r.Context(), header.Filename, nil, size)
	if err != nil {
		switch err {
		case errors.New("file_too_large"):
			h.writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		default:
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Read the file content
	body := make([]byte, size)
	n, err := file.Read(body)
	if err != nil && err != errors.New("EOF") {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if int64(n) != size {
		h.writeError(w, http.StatusInternalServerError, "failed to read full file")
		return
	}

	// Re-call service.Upload with the actual body (since we need to parse it)
	tracklist, tracks, warnings, err = h.svc.Upload(r.Context(), header.Filename, body, size)
	if err != nil {
		switch err {
		case errors.New("file_too_large"):
			h.writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		case errors.New("invalid_format"):
			h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.New("zero_tracks"):
			h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Return success
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"tracklist": tracklist,
		"tracks":    tracks,
		"warnings":  warnings,
	})
}

// handleList handles GET /
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	tracklists, err := h.svc.List(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, tracklists)
}

// handleGet handles GET /{id}
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id := uuid.MustParse(idStr)

	tracklist, tracks, err := h.svc.GetWithTracks(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"tracklist": tracklist,
		"tracks":    tracks,
	})
}

// handleUpdateTrack handles PUT /{id}/tracks/{track_id}
func (h *Handler) handleUpdateTrack(w http.ResponseWriter, r *http.Request) {
	tracklistIDStr := chi.URLParam(r, "id")
	trackIDStr := chi.URLParam(r, "track_id")

	tracklistID := uuid.MustParse(tracklistIDStr)
	trackID := uuid.MustParse(trackIDStr)

	// Parse JSON body
	var req UpdateTrackRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Call service.UpdateTrack
	track, err := h.svc.UpdateTrack(r.Context(), tracklistID, trackID, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else if errors.Is(err, ErrConflict) {
			h.writeError(w, http.StatusConflict, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	h.writeJSON(w, http.StatusOK, track)
}

// handleDelete handles DELETE /{id}
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id := uuid.MustParse(idStr)

	// Call service.SoftDelete
	err := h.svc.SoftDelete(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTrackArtwork handles PUT /{id}/tracks/{track_id}/artwork
func (h *Handler) handleTrackArtwork(w http.ResponseWriter, r *http.Request) {
	tracklistIDStr := chi.URLParam(r, "id")
	trackIDStr := chi.URLParam(r, "track_id")

	tracklistID := uuid.MustParse(tracklistIDStr)
	trackID := uuid.MustParse(trackIDStr)

	// Limit the request body to 10 MB for images
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	defer r.Body.Close()

	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get the file
	file, header, err := r.FormFile("file")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	// Get file info
	contentType := header.Header.Get("Content-Type")
	size := header.Size

	// Read the file content
	imageData := make([]byte, size)
	n, err := file.Read(imageData)
	if err != nil && err != errors.New("EOF") {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if int64(n) != size {
		h.writeError(w, http.StatusInternalServerError, "failed to read full file")
		return
	}

	// Validate content type
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		h.writeError(w, http.StatusUnprocessableEntity, "unsupported image format")
		return
	}

	// Call service.SaveManualArtwork
	err = h.svc.SaveManualArtwork(r.Context(), tracklistID, trackID, imageData, contentType)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Get the updated track to return
	_, tracksResult, err := h.svc.GetWithTracks(r.Context(), tracklistID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Find the specific track
	var updatedTrack *Track
	for _, t := range tracksResult {
		if t.ID == trackID {
			updatedTrack = &t
			break
		}
	}
	if updatedTrack == nil {
		h.writeError(w, http.StatusNotFound, "track not found")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"track": updatedTrack,
	})
}

// writeJSON writes JSON response
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes error response
func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
