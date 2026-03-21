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
	ErrEditBlocked   = errors.New("post cannot be edited in its current status")
	ErrInvalidMIME   = errors.New("image must be JPEG or PNG")
	ErrFileTooLarge  = errors.New("image must be 8 MB or smaller")
	ErrInvalidDimensions = errors.New("image dimensions or aspect ratio not allowed for post type")
)

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
	InstagramAppID      string
	InstagramAppSecret  string
	InstagramRedirectURI string
	TokenEncryptionKey  []byte
}

// Service provides the social scheduling business logic.
type Service struct {
	repo repoIface
	cfg  ServiceConfig
}

// NewService creates a Service with the given repository and config.
func NewService(repo repoIface, cfg ServiceConfig) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// GetOAuthURL builds the Instagram OAuth authorisation URL.
func (s *Service) GetOAuthURL(ctx context.Context) (string, error) {
	state := uuid.New().String()
	params := url.Values{}
	params.Set("client_id", s.cfg.InstagramAppID)
	params.Set("redirect_uri", s.cfg.InstagramRedirectURI)
	params.Set("scope", "instagram_business_basic,instagram_business_content_publish")
	params.Set("response_type", "code")
	params.Set("state", state)
	return "https://api.instagram.com/oauth/authorize?" + params.Encode(), nil
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

// HandleOAuthCallback exchanges the OAuth code for a long-lived token and stores it.
func (s *Service) HandleOAuthCallback(ctx context.Context, code, state string) (*SocialAccount, error) {
	// Step 1: Short-lived token
	form := url.Values{}
	form.Set("client_id", s.cfg.InstagramAppID)
	form.Set("client_secret", s.cfg.InstagramAppSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", s.cfg.InstagramRedirectURI)
	form.Set("code", code)

	shortResp, err := http.PostForm("https://api.instagram.com/oauth/access_token", form)
	if err != nil {
		return nil, fmt.Errorf("short token exchange: %w", err)
	}
	defer shortResp.Body.Close()
	body, _ := io.ReadAll(shortResp.Body)

	var shortToken instagramTokenResponse
	if err := json.Unmarshal(body, &shortToken); err != nil {
		return nil, fmt.Errorf("parse short token: %w", err)
	}

	// Step 2: Long-lived token
	longURL := fmt.Sprintf(
		"https://graph.instagram.com/access_token?grant_type=ig_exchange_token&client_secret=%s&access_token=%s",
		url.QueryEscape(s.cfg.InstagramAppSecret),
		url.QueryEscape(shortToken.AccessToken),
	)
	longResp, err := http.Get(longURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("long token exchange: %w", err)
	}
	defer longResp.Body.Close()
	longBody, _ := io.ReadAll(longResp.Body)

	var longToken instagramLongLivedResponse
	if err := json.Unmarshal(longBody, &longToken); err != nil {
		return nil, fmt.Errorf("parse long token: %w", err)
	}

	// Encrypt token
	encrypted, err := crypto.Encrypt(s.cfg.TokenEncryptionKey, longToken.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt token: %w", err)
	}

	expiry := time.Now().Add(time.Duration(longToken.ExpiresIn) * time.Second)
	igUserIDStr := fmt.Sprintf("%d", shortToken.UserID)

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
