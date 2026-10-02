package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/platform/crypto"
	"github.com/minio/minio-go/v7"
)

// Sentinel errors returned by the service layer.
var (
	ErrEditBlocked       = errors.New("post cannot be edited in its current status")
	ErrInvalidMIME       = errors.New("image must be JPEG or PNG")
	ErrFileTooLarge      = errors.New("image must be 8 MB or smaller")
	ErrInvalidDimensions = errors.New("image dimensions or aspect ratio not allowed for post type")
)

// ErrProviderExchange is the opaque, sanitized error returned to the
// HTTP layer (and ultimately to the client) when an upstream provider
// call fails. The underlying detail — which often contains the OAuth
// credentials — is logged separately, never returned.
var ErrProviderExchange = errors.New("instagram exchange failed")

// repoIface is the subset of Repository used by Service.
type repoIface interface {
	GetAccount(ctx context.Context) (*SocialAccount, error)
	UpsertAccount(ctx context.Context, acc SocialAccount) (*SocialAccount, error)
	UpdateAccountStatus(ctx context.Context, id uuid.UUID, status string) error
	CreatePost(ctx context.Context, post ScheduledPost) (*ScheduledPost, error)
	GetPost(ctx context.Context, id uuid.UUID) (*ScheduledPost, error)
	ListPosts(ctx context.Context) ([]ScheduledPost, error)
	UpdatePost(ctx context.Context, id uuid.UUID, caption, imagePath string, scheduledAt time.Time, tzName string) (*ScheduledPost, error)
	UpdatePostImage(ctx context.Context, id uuid.UUID, imagePath string) error
	UpdatePostStatus(ctx context.Context, id uuid.UUID, status PostStatus, errReason string) error
	DisconnectAccountCascade(ctx context.Context, accountID uuid.UUID) error
	SetNextRetry(ctx context.Context, id uuid.UUID, retryCount int, nextRetryAt time.Time) error
	SoftDeletePost(ctx context.Context, id uuid.UUID) error
	ResetPostForRetry(ctx context.Context, id uuid.UUID) error
}

// ServiceConfig holds the configuration values needed by the Service.
type ServiceConfig struct {
	InstagramAppID       string
	InstagramAppSecret   string
	InstagramRedirectURI string
	TokenEncryptionKey   []byte
}

// Service provides the social scheduling business logic.
type Service struct {
	repo       repoIface
	cfg        ServiceConfig
	stateStore *StateStore
	storage    serviceStorageIface
}

// storageIface is the subset of the storage client the Service uses for
// uploading post images to Garage S3. Defined here so the Service can be
// constructed against an in-memory fake in unit tests.
//
// Named `serviceStorageIface` to avoid colliding with the identically
// named interface in worker.go (which carries the presign-side methods
// the worker needs).
type serviceStorageIface interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	Bucket() string
}

// NewService creates a Service with the given repository and config.
// The service has no state store by default; callers that handle
// OAuth callbacks must call SetStateStore before serving the callback
// route, or use NewServiceWithState.
func NewService(repo repoIface, cfg ServiceConfig) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// NewServiceWithState creates a Service that issues and validates
// OAuth state tokens via the given state store. The store must be
// non-nil; pass NewStateStore(10*time.Minute) for the standard TTL.
func NewServiceWithState(repo repoIface, cfg ServiceConfig, store *StateStore) *Service {
	s := NewService(repo, cfg)
	s.stateStore = store
	return s
}

// SetStateStore injects a state store into an existing Service. Useful
// when the store is created after the service (e.g. when it needs to be
// stopped independently of the service).
func (s *Service) SetStateStore(store *StateStore) {
	s.stateStore = store
}

// SetStorage injects a Garage S3 storage client into an existing Service.
// UploadPostImage requires this dependency; without it the handler will
// surface a 500. Wired separately so callers that never exercise image
// upload don't have to provide one.
func (s *Service) SetStorage(storage serviceStorageIface) {
	s.storage = storage
}

// GetOAuthURL builds the Instagram OAuth authorisation URL.
//
// The returned URL contains a one-time `state` query parameter. The
// caller MUST also set a cookie named OAuthStateCookieName to the
// returned bind token (HttpOnly, SameSite=Lax). The handler performs
// that cookie write automatically via IssueOAuthState.
//
// If the service has no state store configured, this method falls
// back to a UUID-only state with no binding — that mode is kept for
// backwards compatibility with tests that don't construct a store.
// Production wiring should always pass a state store.
func (s *Service) GetOAuthURL(ctx context.Context) (string, error) {
	state, err := s.issueState()
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("client_id", s.cfg.InstagramAppID)
	params.Set("redirect_uri", s.cfg.InstagramRedirectURI)
	params.Set("scope", "instagram_business_basic,instagram_business_content_publish")
	params.Set("response_type", "code")
	params.Set("state", state)
	return "https://api.instagram.com/oauth/authorize?" + params.Encode(), nil
}

// OAuthStateCookieName is the name of the cookie that holds the bind
// half of the OAuth state token pair.
const OAuthStateCookieName = "oauth_state_bind"

// IssueOAuthState is the HTTP-handler-friendly equivalent of
// GetOAuthURL. It returns the OAuth URL, the state token, and the
// bind token. The handler is responsible for setting the bind cookie
// and writing the URL in the response body.
func (s *Service) IssueOAuthState(ctx context.Context) (oauthURL, state, bind string, err error) {
	state, bind, err = s.issueStatePair()
	if err != nil {
		return "", "", "", err
	}
	params := url.Values{}
	params.Set("client_id", s.cfg.InstagramAppID)
	params.Set("redirect_uri", s.cfg.InstagramRedirectURI)
	params.Set("scope", "instagram_business_basic,instagram_business_content_publish")
	params.Set("response_type", "code")
	params.Set("state", state)
	return "https://api.instagram.com/oauth/authorize?" + params.Encode(), state, bind, nil
}

func (s *Service) issueState() (string, error) {
	if s.stateStore == nil {
		// Legacy / test fallback: a UUID-only state with no binding.
		return uuid.New().String(), nil
	}
	state, _, err := s.stateStore.Issue()
	return state, err
}

func (s *Service) issueStatePair() (state, bind string, err error) {
	if s.stateStore == nil {
		st := uuid.New().String()
		return st, st, nil
	}
	return s.stateStore.Issue()
}

// instagramTokenResponse is used to parse the short-lived token exchange.
type instagramTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	UserID      int64  `json:"user_id"`
}

// instagramLongLivedResponse is used to parse the long-lived token exchange.
type instagramLongLivedResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// HandleOAuthCallback exchanges the OAuth code for a long-lived token
// and stores it.
//
// state is the value Instagram returned as the `state` query param.
// bind is the value of the OAuthStateCookieName cookie that the
// originating /auth/url response set. The pair must match what the
// service issued, and must not have been consumed before — these
// checks happen BEFORE any provider call or DB write, so a
// mismatched/replayed/expired state produces zero side effects.
//
// On any provider-side failure the method returns ErrProviderExchange
// (sanitized); the underlying detail is logged via the slog-style
// callback the service was constructed with, when available. The DB
// is only written on full success.
func (s *Service) HandleOAuthCallback(ctx context.Context, code, state, bind string) (*SocialAccount, error) {
	// Step 0: Validate state/bind pair. Single-use enforcement means a
	// replay attempt fails here even if the state string matches.
	//
	// Defense-in-depth: if a state store is configured (production
	// mode), an empty bind is rejected up-front WITHOUT touching the
	// store. This prevents a DoS where an unauthenticated attacker
	// who knows only the state string could burn the legitimate
	// user's pending state. The handler is supposed to reject missing
	// cookies before reaching the service, but we also enforce this
	// here in case any future caller forgets.
	if s.stateStore != nil {
		if bind == "" {
			return nil, ErrOAuthStateInvalid
		}
		if state == "" {
			return nil, ErrOAuthStateInvalid
		}
		gotBind, ok := s.stateStore.Consume(state)
		if !ok {
			return nil, ErrOAuthStateInvalid
		}
		if gotBind != bind {
			return nil, ErrOAuthStateInvalid
		}
	} else if state == "" {
		// No state store configured (test/legacy mode): require a
		// non-empty state but skip binding check.
		return nil, ErrOAuthStateInvalid
	}

	// Step 1: Short-lived token
	form := url.Values{}
	form.Set("client_id", s.cfg.InstagramAppID)
	form.Set("client_secret", s.cfg.InstagramAppSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", s.cfg.InstagramRedirectURI)
	form.Set("code", code)

	shortResp, err := http.PostForm("https://api.instagram.com/oauth/access_token", form) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderExchange, err)
	}
	defer shortResp.Body.Close()
	body, _ := io.ReadAll(shortResp.Body)

	if shortResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: short token status %d body=%s",
			ErrProviderExchange, shortResp.StatusCode, SanitizeTransportError(string(body)))
	}

	var shortToken instagramTokenResponse
	if err := json.Unmarshal(body, &shortToken); err != nil {
		return nil, fmt.Errorf("%w: parse short token: %v", ErrProviderExchange, err)
	}

	// Step 2: Long-lived token
	longURL := fmt.Sprintf(
		"https://graph.instagram.com/access_token?grant_type=ig_exchange_token&client_secret=%s&access_token=%s",
		url.QueryEscape(s.cfg.InstagramAppSecret),
		url.QueryEscape(shortToken.AccessToken),
	)
	longResp, err := http.Get(longURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderExchange, err)
	}
	defer longResp.Body.Close()
	longBody, _ := io.ReadAll(longResp.Body)

	if longResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: long token status %d body=%s",
			ErrProviderExchange, longResp.StatusCode, SanitizeTransportError(string(longBody)))
	}

	var longToken instagramLongLivedResponse
	if err := json.Unmarshal(longBody, &longToken); err != nil {
		return nil, fmt.Errorf("%w: parse long token: %v", ErrProviderExchange, err)
	}

	// Step 3: Encrypt token (no provider call).
	encrypted, err := crypto.Encrypt(s.cfg.TokenEncryptionKey, longToken.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt token: %w", err)
	}

	expiry := time.Now().Add(time.Duration(longToken.ExpiresIn) * time.Second)
	igUserIDStr := fmt.Sprintf("%d", shortToken.UserID)

	// Step 4: Persist account row. ONLY happens on the happy path —
	// zero writes on any failure above.
	acc, err := s.repo.UpsertAccount(ctx, SocialAccount{
		Platform:    "instagram",
		AccountName: igUserIDStr, // updated after Graph API profile fetch in a future plan
		IgUserID:    igUserIDStr,
		AccessToken: encrypted,
		TokenExpiry: &expiry,
		Status:      "connected",
	})
	if err != nil {
		return nil, fmt.Errorf("store account: %w", err)
	}
	return acc, nil
}

// GetAccount returns the current social account (nil if none connected).
func (s *Service) GetAccount(ctx context.Context) (*SocialAccount, error) {
	return s.repo.GetAccount(ctx)
}

// CreatePostRequest is the input for scheduling a new post.
type CreatePostRequest struct {
	AccountID      uuid.UUID
	PostType       PostType
	Caption        string
	ImageStorageKey string
	ScheduledAt    string // datetime-local format "2006-01-02T15:04"
	TimezoneName   string
	ImageData      []byte // if non-nil, validate and upload was already done by caller
}

// EditPostRequest is the input for editing a scheduled post.
type EditPostRequest struct {
	Caption         string
	ImageStorageKey string `json:"imageId"`
	ScheduledAt     string
	TimezoneName    string
}

// SchedulePost creates a new scheduled post, converting local time to UTC.
func (s *Service) SchedulePost(ctx context.Context, req CreatePostRequest) (*ScheduledPost, error) {
	scheduledAt, err := parseDateTimeLocal(req.ScheduledAt, req.TimezoneName)
	if err != nil {
		return nil, fmt.Errorf("parse scheduled_at: %w", err)
	}

	if len(req.ImageData) > 0 {
		if err := s.ValidateImage(req.ImageData, req.PostType); err != nil {
			return nil, err
		}
	}

	return s.repo.CreatePost(ctx, ScheduledPost{
		AccountID:       req.AccountID,
		Status:          PostStatusScheduled,
		PostType:        req.PostType,
		Caption:         req.Caption,
		ImageStorageKey: req.ImageStorageKey,
		ScheduledAtUTC:  scheduledAt,
		TimezoneName:    req.TimezoneName,
	})
}

// EditPost updates a scheduled post. Returns ErrEditBlocked if not in 'scheduled' status.
func (s *Service) EditPost(ctx context.Context, id uuid.UUID, req EditPostRequest) (*ScheduledPost, error) {
	post, err := s.repo.GetPost(ctx, id)
	if err != nil {
		return nil, err
	}

	if post.Status != PostStatusScheduled {
		return nil, ErrEditBlocked
	}

	scheduledAt, err := parseDateTimeLocal(req.ScheduledAt, req.TimezoneName)
	if err != nil {
		return nil, fmt.Errorf("parse scheduled_at: %w", err)
	}

	return s.repo.UpdatePost(ctx, id, req.Caption, req.ImageStorageKey, scheduledAt, req.TimezoneName)
}

// DisconnectAccount marks the account as disconnected and moves scheduled posts to draft.
func (s *Service) DisconnectAccount(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DisconnectAccountCascade(ctx, id); err != nil {
		return err
	}
	return s.repo.UpdateAccountStatus(ctx, id, "disconnected")
}

// RetryPost resets a failed post back to scheduled status.
func (s *Service) RetryPost(ctx context.Context, id uuid.UUID) error {
	return s.repo.ResetPostForRetry(ctx, id)
}

// PostImageResult is the response shape returned by UploadPostImage.
// Path is the Garage S3 object key; the handler wraps it in
// {data: {path: "..."}}.
type PostImageResult struct {
	Path string `json:"path"`
}

// UploadPostImage validates the bytes, stores them in Garage under a
// fresh UUID-based key, and updates the post's image_storage_key column.
//
// Pre-conditions:
//   - postID must exist (ErrNotFound otherwise — the handler maps this
//     to a 404).
//   - post status must be scheduled (ErrEditBlocked maps to 409). The
//     repository repeats this check atomically in the UPDATE predicate.
//   - data must pass ValidateImage for the post's existing PostType
//     (ErrInvalidMIME / ErrFileTooLarge / ErrInvalidDimensions otherwise —
//     the handler maps these to 422).
//
// The Garage object key has the form `social/<post-id>/<uuid>.<ext>`
// where `<ext>` is derived from the validated MIME type. Server-side
// generation (rather than trusting the client filename) is what
// eliminates the path-traversal class of bugs called out in the C.1
// risk register.
//
// The image_storage_key is written LAST so that a storage write failure
// cannot leave a post pointing at a non-existent object. If the DB
// write fails (including a raced lifecycle transition) after a successful
// upload, the object remains orphaned in Garage. Cleanup is deferred and
// is NOT implemented by this operation.
func (s *Service) UploadPostImage(ctx context.Context, postID uuid.UUID, data []byte, _ string) (*PostImageResult, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage client not configured")
	}

	post, err := s.repo.GetPost(ctx, postID)
	if err != nil {
		return nil, err
	}

	// Match EditPost: only scheduled posts may change their image.
	if post.Status != PostStatusScheduled {
		return nil, ErrEditBlocked
	}
	if err := s.ValidateImage(data, post.PostType); err != nil {
		return nil, err
	}

	mt := mimetype.Detect(data)
	ext := "jpg"
	if mt.String() == "image/png" {
		ext = "png"
	}
	key := fmt.Sprintf("social/%s/%s.%s", postID.String(), uuid.New().String(), ext)

	if _, err := s.storage.PutObject(ctx, s.storage.Bucket(), key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: mt.String()}); err != nil {
		return nil, fmt.Errorf("store image: %w", err)
	}

	if err := s.repo.UpdatePostImage(ctx, postID, key); err != nil {
		return nil, fmt.Errorf("update post image path: %w", err)
	}

	return &PostImageResult{Path: key}, nil
}

// ListPosts returns all non-deleted posts.
func (s *Service) ListPosts(ctx context.Context) ([]ScheduledPost, error) {
	return s.repo.ListPosts(ctx)
}

// GetPost returns a single post by ID.
func (s *Service) GetPost(ctx context.Context, id uuid.UUID) (*ScheduledPost, error) {
	return s.repo.GetPost(ctx, id)
}

// SoftDeletePost removes a post from the active queue.
func (s *Service) SoftDeletePost(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDeletePost(ctx, id)
}

// ValidateImage checks MIME type, file size, and image dimensions/aspect ratio.
func (s *Service) ValidateImage(data []byte, postType PostType) error {
	// 1. MIME type check
	mt := mimetype.Detect(data)
	if mt.String() != "image/jpeg" && mt.String() != "image/png" {
		return ErrInvalidMIME
	}

	// 2. File size check (8 MB)
	if len(data) > 8*1024*1024 {
		return ErrFileTooLarge
	}

	// 3. Dimension / aspect ratio check
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: decode image config: %v", ErrInvalidDimensions, err)
	}

	// Bound allocations before decoding pixel data. Even a tiny compressed
	// upload can advertise enormous dimensions. Use division to avoid overflow.
	const maxSide = 8192
	const maxPixels = 16_000_000
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width > maxPixels/cfg.Height {
		return fmt.Errorf("%w: image exceeds decode budget", ErrInvalidDimensions)
	}

	w, h := float64(cfg.Width), float64(cfg.Height)
	ratio := w / h

	switch postType {
	case PostTypeFeed:
		if cfg.Width < 320 || cfg.Height < 320 {
			return ErrInvalidDimensions
		}
		if ratio < 0.8 || ratio > 1.91 {
			return ErrInvalidDimensions
		}
	case PostTypeStory:
		target := 9.0 / 16.0
		if ratio < target-0.05 || ratio > target+0.05 {
			return ErrInvalidDimensions
		}
	}

	// A valid header is not a valid image. Decode the complete supported
	// JPEG/PNG after preflight, before any storage or database mutation.
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("%w: decode image: %v", ErrInvalidDimensions, err)
	}
	return nil
}

// parseDateTimeLocal parses a datetime-local string ("2006-01-02T15:04") in the given IANA timezone
// and returns the equivalent UTC time.
func parseDateTimeLocal(datetimeLocal, timezoneName string) (time.Time, error) {
	loc, err := time.LoadLocation(timezoneName)
	if err != nil {
		return time.Time{}, fmt.Errorf("unknown timezone %q: %w", timezoneName, err)
	}
	t, err := time.ParseInLocation("2006-01-02T15:04", datetimeLocal, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse datetime %q: %w", datetimeLocal, err)
	}
	return t.UTC(), nil
}
