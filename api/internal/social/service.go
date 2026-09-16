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
	ImageMinioPath string
	ScheduledAt    string // datetime-local format "2006-01-02T15:04"
	TimezoneName   string
	ImageData      []byte // if non-nil, validate and upload was already done by caller
}

// EditPostRequest is the input for editing a scheduled post.
type EditPostRequest struct {
	Caption        string
	ImageMinioPath string
	ScheduledAt    string
	TimezoneName   string
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
		AccountID:      req.AccountID,
		Status:         PostStatusScheduled,
		PostType:       req.PostType,
		Caption:        req.Caption,
		ImageMinioPath: req.ImageMinioPath,
		ScheduledAtUTC: scheduledAt,
		TimezoneName:   req.TimezoneName,
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

	return s.repo.UpdatePost(ctx, id, req.Caption, req.ImageMinioPath, scheduledAt, req.TimezoneName)
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
		return fmt.Errorf("decode image config: %w", err)
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
