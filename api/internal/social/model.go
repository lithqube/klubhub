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
type ScheduledPost struct {
	ID             uuid.UUID  `json:"id"`
	AccountID      uuid.UUID  `json:"account_id"`
	Status         PostStatus `json:"status"`
	PostType       PostType   `json:"post_type"`
	Caption        string     `json:"caption"`
	ImageMinioPath string     `json:"image_minio_path"`
	ScheduledAtUTC time.Time  `json:"scheduled_at_utc"`
	TimezoneName   string     `json:"timezone_name"`
	RetryCount     int        `json:"retry_count"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty"`
	LastError      *string    `json:"last_error,omitempty"`
	ContainerID    *string    `json:"container_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}
