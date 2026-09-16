package social

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	ValidateImage(data []byte, postType PostType) error
}

// Handler handles HTTP requests for the social package.
type Handler struct {
	svc serviceIface
}

// NewHandler creates a Handler with the given service.
func NewHandler(svc serviceIface) *Handler {
	return &Handler{svc: svc}
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

	return r
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
// (legacy-named Garage object key) or image_file (upload).
func (h *Handler) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	// Plan B.6: ParseMultipartForm's maxMemory is NOT a total request
	// limit; it spills larger payloads to disk. Cap the transport first.
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20+1024)
	// Parse multipart (max 12 MB to accommodate 8 MB image + metadata)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		// Fall back to URL-encoded / JSON for tests
		if err2 := r.ParseForm(); err2 != nil {
			h.writeError(w, http.StatusBadRequest, "cannot parse request body")
			return
		}
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
		PostType:       postType,
		Caption:        caption,
		ImageMinioPath: imageID,
		ScheduledAt:    scheduledAt,
		TimezoneName:   timezoneName,
	}

	// Handle image file upload if present
	if r.MultipartForm != nil {
		file, _, err := r.FormFile("image_file")
		if err == nil {
			defer file.Close()
			imageData, err := io.ReadAll(file)
			if err != nil {
				h.writeError(w, http.StatusInternalServerError, "failed to read image")
				return
			}
			// Validate image inline
			if err := h.svc.ValidateImage(imageData, postType); err != nil {
				h.writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			req.ImageData = imageData
		}
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

// handleRetryPost resets a failed post to scheduled status.
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
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
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
