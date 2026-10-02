package tracklist

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	UpdateTitle(ctx context.Context, id uuid.UUID, title string) (*Tracklist, error)
	LinkedGigs(ctx context.Context, id uuid.UUID) ([]LinkedGig, error)
	AddTrack(ctx context.Context, tracklistID uuid.UUID, req CreateTrackRequest) (*Track, error)
	ReorderTracks(ctx context.Context, tracklistID uuid.UUID, ids []uuid.UUID) ([]Track, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	SaveManualArtwork(ctx context.Context, tracklistID, trackID uuid.UUID, imageData []byte, contentType string) error
	GenerateImage(ctx context.Context, id uuid.UUID, format string) (map[string]string, error)
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
	r.Put("/{id}", h.handleUpdateTracklist)
	r.Get("/{id}/gigs", h.handleLinkedGigs)
	r.Post("/{id}/tracks", h.handleAddTrack)
	r.Put("/{id}/tracks/order", h.handleReorderTracks)
	r.Put("/{id}/tracks/{track_id}", h.handleUpdateTrack)
	r.Delete("/{id}", h.handleDelete)
	r.Put("/{id}/tracks/{track_id}/artwork", h.handleTrackArtwork)
	r.Post("/{id}/generate-image", h.handleGenerateImage)
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

	// Read the whole file (already capped by MaxBytesReader above). A single
	// Read call is not guaranteed to fill the buffer, so use io.ReadAll.
	body, err := io.ReadAll(file)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	// Parse and persist in one call. Upload enforces the size limit itself;
	// calling it with a nil body first made every upload fail with
	// zero_tracks before the file was ever parsed.
	tracklist, tracks, warnings, err := h.svc.Upload(r.Context(), header.Filename, body, header.Size)
	if err != nil {
		// errors.Is against sentinels: comparing to errors.New(...) allocates
		// a fresh value that never matches, so everything became a 500.
		switch {
		case errors.Is(err, ErrFileTooLarge):
			h.writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		case errors.Is(err, ErrInvalidFormat), errors.Is(err, ErrZeroTracks):
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
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": tracklists})
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
		} else if errors.Is(err, ErrInvalidTrack) {
			h.writeError(w, http.StatusUnprocessableEntity, "invalid track fields")
		} else if errors.Is(err, ErrConflict) {
			h.writeError(w, http.StatusConflict, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	h.writeJSON(w, http.StatusOK, track)
}

// handleUpdateTracklist handles PUT /{id}: renames the tracklist and returns it.
func (h *Handler) handleUpdateTracklist(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tracklist id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req UpdateTracklistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Title == nil {
		h.writeError(w, http.StatusUnprocessableEntity, "title is required")
		return
	}
	tl, err := h.svc.UpdateTitle(r.Context(), id, *req.Title)
	switch {
	case err == nil:
		h.writeJSON(w, http.StatusOK, tl)
	case errors.Is(err, ErrInvalidTitle):
		h.writeError(w, http.StatusUnprocessableEntity, "title must be 1-200 characters")
	case errors.Is(err, ErrNotFound):
		h.writeError(w, http.StatusNotFound, err.Error())
	default:
		h.writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// handleLinkedGigs handles GET /{id}/gigs: the gigs this tracklist is linked to,
// as a bare array. Linking and unlinking use the gig endpoints.
func (h *Handler) handleLinkedGigs(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tracklist id")
		return
	}
	gigs, err := h.svc.LinkedGigs(r.Context(), id)
	switch {
	case err == nil:
		if gigs == nil {
			gigs = []LinkedGig{}
		}
		h.writeJSON(w, http.StatusOK, gigs)
	case errors.Is(err, ErrNotFound):
		h.writeError(w, http.StatusNotFound, err.Error())
	default:
		h.writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// handleAddTrack handles POST /{id}/tracks: adds a manually entered track at the end.
func (h *Handler) handleAddTrack(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tracklist id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var req CreateTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	track, err := h.svc.AddTrack(r.Context(), id, req)
	switch {
	case err == nil:
		h.writeJSON(w, http.StatusCreated, track)
	case errors.Is(err, ErrInvalidTrack):
		h.writeError(w, http.StatusUnprocessableEntity, "a track needs a title (up to 200 characters) and a valid media type")
	case errors.Is(err, ErrNotFound):
		h.writeError(w, http.StatusNotFound, err.Error())
	default:
		h.writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// handleReorderTracks handles PUT /{id}/tracks/order: body {"trackIds": [...]}
// lists every live track once, in the new order. Returns the tracks in order.
func (h *Handler) handleReorderTracks(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid tracklist id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	var req ReorderTracksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	tracks, err := h.svc.ReorderTracks(r.Context(), id, req.TrackIDs)
	switch {
	case err == nil:
		if tracks == nil {
			tracks = []Track{}
		}
		h.writeJSON(w, http.StatusOK, tracks)
	case errors.Is(err, ErrInvalidTrack):
		h.writeError(w, http.StatusUnprocessableEntity, "trackIds must list every track of the tracklist exactly once")
	case errors.Is(err, ErrNotFound):
		h.writeError(w, http.StatusNotFound, err.Error())
	default:
		h.writeError(w, http.StatusInternalServerError, err.Error())
	}
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

// handleGenerateImage handles POST /{id}/generate-image
func (h *Handler) handleGenerateImage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id := uuid.MustParse(idStr)

	// Get format from query param, default to "story"
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "story"
	}

	// Validate format
	validFormats := map[string]bool{"story": true, "square": true, "both": true}
	if !validFormats[format] {
		h.writeError(w, http.StatusBadRequest, "invalid format: must be story, square, or both")
		return
	}

	// Call service.GenerateImage
	result, err := h.svc.GenerateImage(r.Context(), id, format)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	h.writeJSON(w, http.StatusOK, result)
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
