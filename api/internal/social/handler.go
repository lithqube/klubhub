package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/platform/storage"
	"github.com/minio/minio-go/v7"
)

// serviceIface is the contract consumed by the HTTP handler.
//
// Note: HandleOAuthCallback takes an extra `bind` argument compared
// to the original signature — it is the value of the OAuthStateCookieName
// cookie that /auth/url set. The pair (state, bind) must match what
// the service issued, and the store enforces single-use.
type serviceIface interface {
	IssueOAuthState(ctx context.Context) (oauthURL, state, bind string, err error)
	HandleOAuthCallback(ctx context.Context, code, state, bind string) (*SocialAccount, error)
	GetAccount(ctx context.Context) (*SocialAccount, error)
	DisconnectAccount(ctx context.Context, id uuid.UUID) error
	ListPosts(ctx context.Context) ([]ScheduledPost, error)
	GetPost(ctx context.Context, id uuid.UUID) (*ScheduledPost, error)
	SchedulePost(ctx context.Context, req CreatePostRequest) (*ScheduledPost, error)
	EditPost(ctx context.Context, id uuid.UUID, req EditPostRequest) (*ScheduledPost, error)
	SoftDeletePost(ctx context.Context, id uuid.UUID) error
	RetryPost(ctx context.Context, id uuid.UUID) error
	UploadPostImage(ctx context.Context, id uuid.UUID, data []byte, mimeType string) (*PostImageResult, error)
	ValidateImage(data []byte, postType PostType) error
}

// Handler handles HTTP requests for the social package.
type Handler struct {
	svc     serviceIface
	storage *storage.Client
}

// NewHandler creates a Handler with the given service and storage client.
func NewHandler(svc serviceIface, storage *storage.Client) *Handler {
	return &Handler{svc: svc, storage: storage}
}

// Routes returns an http.Handler with all social routes mounted.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/auth/url", h.handleGetOAuthURL)
	r.Get("/auth/callback", h.handleOAuthCallback)
	r.Post("/auth/callback", h.handleOAuthCallback)

	r.Get("/accounts", h.handleGetAccount)
	r.Delete("/accounts/{id}", h.handleDisconnectAccount)

	r.Get("/posts", h.handleListPosts)
	r.Post("/posts", h.handleCreatePost)
	r.Get("/posts/{id}", h.handleGetPost)
	r.Put("/posts/{id}", h.handleEditPost)
	r.Delete("/posts/{id}", h.handleDeletePost)
	r.Post("/posts/{id}/retry", h.handleRetryPost)
	r.Get("/posts/{id}/image", h.handleGetPostImage)
	r.Post("/posts/{id}/image", h.handleUploadPostImage)

	return r
}

// handleUploadPostImage accepts multipart/form-data with an "image_file"
// field, validates the bytes, stores them in Garage via the Service,
// and updates the post's image_storage_key.
//
// Error → status mapping:
//   - 400: malformed multipart body or missing image_file field.
//   - 404: postID does not exist (or has been soft-deleted).
//   - 409: post is no longer scheduled (including a raced transition).
//   - 413: request body exceeds the 12 MiB + 1 KB cap
//     (ParseMultipartForm returns *http.MaxBytesError).
//   - 422: image fails MIME / size / dimension validation
//     (ErrInvalidMIME / ErrFileTooLarge / ErrInvalidDimensions).
//   - 500: storage write, DB update, or other unexpected error.
//
// On success the response is {data: {path: "<garage-object-key>"}}.
//
// Note: this handler trusts the Service to do all storage + validation
// work; it only parses the transport and translates errors to HTTP
// status codes. The contract test
// (TestUploadPostImageContract_ReturnsDataPathEnvelope) asserts the
// success envelope shape.
func (h *Handler) handleUploadPostImage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	// Plan B.6 lesson: ParseMultipartForm's maxMemory is a spill-to-disk
	// threshold, NOT a request-size limit. Cap the transport first.
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20+1024)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		var capErr *http.MaxBytesError
		if errors.As(err, &capErr) {
			h.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			h.writeError(w, http.StatusBadRequest, "cannot parse multipart form")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("image_file")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "image_file field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to read image data")
		return
	}

	mimeType := header.Header.Get("Content-Type")

	result, err := h.svc.UploadPostImage(r.Context(), id, data, mimeType)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			h.writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrEditBlocked):
			h.writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, ErrInvalidMIME),
			errors.Is(err, ErrFileTooLarge),
			errors.Is(err, ErrInvalidDimensions):
			h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

// oauthStateCookieMaxAge matches the StateStore TTL (10 minutes). The
// cookie lifetime must be at least as long as the state lifetime;
// otherwise a callback arriving near the edge of validity will pass
// the state check but the browser may have already evicted the
// cookie, producing a spurious failure.
const oauthStateCookieMaxAge = 600

// handleGetOAuthURL returns a JSON object with the Instagram OAuth URL.
// It also sets an HttpOnly `oauth_state_bind` cookie containing the
// bind half of the state pair. The browser MUST echo that cookie on
// the callback request; mismatches / missing cookies result in a 400.
func (h *Handler) handleGetOAuthURL(w http.ResponseWriter, r *http.Request) {
	oauthURL, _, bind, err := h.svc.IssueOAuthState(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthStateCookieName,
		Value:    bind,
		Path:     "/api/v1/social/",
		MaxAge:   oauthStateCookieMaxAge,
		HttpOnly: true,
		Secure:   true, // production is HTTPS; set to false in test helpers if needed
		SameSite: http.SameSiteLaxMode,
	})
	h.writeJSON(w, http.StatusOK, map[string]string{"url": oauthURL})
}

// handleOAuthCallback handles both GET (redirect) and POST (JSON body) callbacks.
func (h *Handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	var code, state string

	if r.Method == http.MethodGet {
		code = r.URL.Query().Get("code")
		state = r.URL.Query().Get("state")
	} else {
		var body struct {
			Code  string `json:"code"`
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		code = body.Code
		state = body.State
	}

	if code == "" {
		h.writeError(w, http.StatusBadRequest, "missing code parameter")
		return
	}

	// Read bind cookie AFTER code/state validation so we don't
	// leak cookie-presence signals via differential responses.
	//
	// The cookie is the proof that the callback originates from
	// the same browser that initiated /auth/url. If it is absent,
	// we MUST reject BEFORE forwarding to the service so the state
	// store is not consumed — otherwise an unauthenticated attacker
	// who has only the state string (e.g. via a leaked URL, a
	// referrer log, or a shoulder-surf) can grief the legitimate
	// user by burning their pending OAuth state (DoS).
	bindCookie, err := r.Cookie(OAuthStateCookieName)
	if err != nil || bindCookie.Value == "" {
		h.writeError(w, http.StatusBadRequest, "missing or empty oauth state cookie")
		return
	}
	bind := bindCookie.Value

	acc, err := h.svc.HandleOAuthCallback(r.Context(), code, state, bind)
	if err != nil {
		// Distinguish state-validation failures (client error) from
		// upstream provider failures (server error). Both come back
		// sanitized — no URLs, no credentials.
		if errors.Is(err, ErrOAuthStateInvalid) {
			h.writeError(w, http.StatusBadRequest, "invalid or expired oauth state")
			return
		}
		if errors.Is(err, ErrProviderExchange) {
			h.writeError(w, http.StatusBadGateway, ErrProviderExchange.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": acc})
}

// handleGetAccount returns the current social account (null data if none connected).
func (h *Handler) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := h.svc.GetAccount(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": acc})
}

// handleDisconnectAccount disconnects an account and moves scheduled posts to draft.
func (h *Handler) handleDisconnectAccount(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}

	if err := h.svc.DisconnectAccount(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// handleListPosts returns all scheduled posts.
func (h *Handler) handleListPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := h.svc.ListPosts(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		return
	}
	if posts == nil {
		posts = []ScheduledPost{}
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": posts})
}

// handleCreatePost handles multipart form POST /posts.
// Form fields: caption, post_type, scheduled_at, timezone_name, image_id
// (legacy-named Garage object key). Image bytes must be uploaded separately
// to POST /posts/{id}/image; legacy inline uploads are rejected, not ignored.
func (h *Handler) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	// Plan B.6: ParseMultipartForm's maxMemory is NOT a total request
	// limit; it spills larger payloads to disk. Cap the transport first.
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20+1024)
	// Preserve multipart metadata support for the frontend FormData caller.
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		var capErr *http.MaxBytesError
		if errors.As(err, &capErr) {
			h.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		// Only non-multipart requests may fall back to URL-encoded metadata.
		// Do not silently accept a partial form after multipart parsing failed.
		if !errors.Is(err, http.ErrNotMultipart) {
			h.writeError(w, http.StatusBadRequest, "cannot parse request body")
			return
		}
		if err2 := r.ParseForm(); err2 != nil {
			if errors.As(err2, &capErr) {
				h.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			} else {
				h.writeError(w, http.StatusBadRequest, "cannot parse request body")
			}
			return
		}
	}

	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	caption := r.FormValue("caption")
	postTypeStr := r.FormValue("post_type")
	scheduledAt := r.FormValue("scheduled_at")
	timezoneName := r.FormValue("timezone_name")
	imageID := r.FormValue("image_id")

	if postTypeStr == "" || scheduledAt == "" || timezoneName == "" {
		h.writeError(w, http.StatusBadRequest, "post_type, scheduled_at, and timezone_name are required")
		return
	}

	postType := PostType(postTypeStr)
	if postType != PostTypeFeed && postType != PostTypeStory {
		h.writeError(w, http.StatusBadRequest, "post_type must be 'feed' or 'story'")
		return
	}

	req := CreatePostRequest{
		PostType:        postType,
		Caption:         caption,
		ImageStorageKey: imageID,
		ScheduledAt:     scheduledAt,
		TimezoneName:    timezoneName,
	}

	// Reject retired inline uploads before scheduling anything. Do not
	// silently accept bytes that the dedicated upload pipeline must persist.
	if r.MultipartForm != nil && len(r.MultipartForm.File["image_file"]) > 0 {
		h.writeError(w, http.StatusUnprocessableEntity, "image_file uploads require POST /posts/{id}/image after creating the post")
		return
	}

	// Parse account_id if provided
	if accountIDStr := r.FormValue("account_id"); accountIDStr != "" {
		id, err := uuid.Parse(accountIDStr)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid account_id")
			return
		}
		req.AccountID = id
	}

	post, err := h.svc.SchedulePost(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidMIME) || errors.Is(err, ErrFileTooLarge) || errors.Is(err, ErrInvalidDimensions) {
			h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{"data": post})
}

// handleGetPost returns a single post by ID.
func (h *Handler) handleGetPost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}
	post, err := h.svc.GetPost(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": post})
}

// handleEditPost handles PUT /posts/{id}.
func (h *Handler) handleEditPost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	var body EditPostRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	post, err := h.svc.EditPost(r.Context(), id, body)
	if err != nil {
		switch {
		case errors.Is(err, ErrEditBlocked):
			h.writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, ErrNotFound):
			h.writeError(w, http.StatusNotFound, err.Error())
		default:
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": post})
}

// handleDeletePost soft-deletes a post.
func (h *Handler) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}
	if err := h.svc.SoftDeletePost(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRetryPost resets a failed post to scheduled status and
// returns the updated post wrapped in the standard {data: ...}
// envelope. The service interface keeps RetryPost signature-free of
// a post return value (it just resets state); the handler follows up
// with GetPost so the wire response carries the full updated record.
func (h *Handler) handleRetryPost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}
	if err := h.svc.RetryPost(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}
	post, err := h.svc.GetPost(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": post})
}

// handleGetPostImage streams the post's image from Garage S3.
func (h *Handler) handleGetPostImage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	post, err := h.svc.GetPost(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		}
		return
	}

	if post.ImageStorageKey == "" {
		h.writeError(w, http.StatusNotFound, "no image associated with this post")
		return
	}

	if h.storage == nil {
		h.writeError(w, http.StatusInternalServerError, "storage client not configured")
		return
	}

	ctx := r.Context()
	obj, err := h.storage.GetObject(ctx, h.storage.Bucket(), post.ImageStorageKey, minio.GetObjectOptions{})
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		return
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, SanitizeTransportError(err.Error()))
		return
	}

	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size))
	// Cache for 1 hour since images are immutable once uploaded
	w.Header().Set("Cache-Control", "public, max-age=3600")

	_, _ = io.Copy(w, obj)
}

// writeJSON writes a JSON response with the given status code.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
