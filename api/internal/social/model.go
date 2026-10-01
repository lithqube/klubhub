package social

import (
	"time"

	"github.com/google/uuid"
)

// PostStatus represents the lifecycle state of a scheduled post.
type PostStatus string

const (
	PostStatusDraft             PostStatus = "draft"
	PostStatusScheduled         PostStatus = "scheduled"
	PostStatusPublishing        PostStatus = "publishing"
	PostStatusPublished         PostStatus = "published"
	PostStatusFailed            PostStatus = "failed"
	PostStatusPermanentlyFailed PostStatus = "permanently_failed"
)

// PostType represents whether a post targets Feed or Story.
type PostType string

const (
	PostTypeFeed  PostType = "feed"
	PostTypeStory PostType = "story"
)

// SocialAccount mirrors the social_accounts table row.
type SocialAccount struct {
	ID          uuid.UUID  `json:"id"`
	Platform    string     `json:"platform"`
	AccountName string     `json:"account_name"`
	IgUserID    string     `json:"ig_user_id"`
	AccessToken string     `json:"-"` // never serialised
	TokenExpiry *time.Time `json:"token_expiry"`
	Status      string     `json:"status"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ScheduledPost mirrors the scheduled_posts table row.
//
// JSON tags are camelCase to match the v1 frontend contract. The
// DB column names stay snake_case (see repository.go) and are not
// affected by this struct's tags.
type ScheduledPost struct {
	ID        uuid.UUID  `json:"id"`
	AccountID uuid.UUID  `json:"accountId"`
	Status    PostStatus `json:"status"`
	PostType  PostType   `json:"postType"`
	Caption   string     `json:"caption"`
	// ImageStorageKey is the Garage object key for the post's image.
	// Renamed from the legacy image_minio_path column in migration 024.
	// JSON tag is camelCase to match the v1 frontend contract.
	ImageStorageKey string     `json:"imageStorageKey"`
	ScheduledAtUTC  time.Time  `json:"scheduledAtUtc"`
	TimezoneName    string     `json:"timezoneName"`
	RetryCount      int        `json:"retryCount"`
	NextRetryAt     *time.Time `json:"nextRetryAt"`
	LastError       *string    `json:"lastError"`
	ContainerID     *string    `json:"containerId"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	DeletedAt       *time.Time `json:"deletedAt,omitempty"`
}