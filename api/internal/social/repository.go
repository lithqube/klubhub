package social

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Repository provides DB access for the social package.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetAccount returns the single social account, or nil if none exists.
func (r *Repository) GetAccount(ctx context.Context) (*SocialAccount, error) {
	var a SocialAccount
	err := r.pool.QueryRow(ctx, `
		SELECT id, platform, account_name, ig_user_id, access_token,
		       token_expiry, status, user_id, created_at, updated_at
		FROM social_accounts
		WHERE deleted_at IS NULL
		LIMIT 1`,
	).Scan(
		&a.ID, &a.Platform, &a.AccountName, &a.IgUserID, &a.AccessToken,
		&a.TokenExpiry, &a.Status, &a.UserID, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// UpsertAccount inserts or updates a social account (keyed by platform).
func (r *Repository) UpsertAccount(ctx context.Context, acc SocialAccount) (*SocialAccount, error) {
	var result SocialAccount
	err := r.pool.QueryRow(ctx, `
		INSERT INTO social_accounts
			(platform, account_name, ig_user_id, access_token, token_expiry, status, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (platform) DO UPDATE
			SET account_name = EXCLUDED.account_name,
			    ig_user_id   = EXCLUDED.ig_user_id,
			    access_token = EXCLUDED.access_token,
			    token_expiry = EXCLUDED.token_expiry,
			    status       = EXCLUDED.status,
			    updated_at   = NOW()
		RETURNING id, platform, account_name, ig_user_id, access_token,
		          token_expiry, status, user_id, created_at, updated_at`,
		acc.Platform, acc.AccountName, acc.IgUserID, acc.AccessToken,
		acc.TokenExpiry, acc.Status, acc.UserID,
	).Scan(
		&result.ID, &result.Platform, &result.AccountName, &result.IgUserID,
		&result.AccessToken, &result.TokenExpiry, &result.Status, &result.UserID,
		&result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateAccountStatus sets the status field on the account with the given ID.
func (r *Repository) UpdateAccountStatus(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE social_accounts SET status=$1, updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`,
		status, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreatePost inserts a new scheduled post and returns it with DB-assigned fields.
func (r *Repository) CreatePost(ctx context.Context, post ScheduledPost) (*ScheduledPost, error) {
	var result ScheduledPost
	err := r.pool.QueryRow(ctx, `
		INSERT INTO scheduled_posts
			(account_id, status, post_type, caption, image_minio_path,
			 scheduled_at_utc, timezone_name, retry_count, next_retry_at, last_error, container_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, account_id, status, post_type, caption, image_minio_path,
		          scheduled_at_utc, timezone_name, retry_count, next_retry_at,
		          last_error, container_id, created_at, updated_at, deleted_at`,
		post.AccountID, post.Status, post.PostType, post.Caption, post.ImageMinioPath,
		post.ScheduledAtUTC.UTC(), post.TimezoneName, post.RetryCount,
		post.NextRetryAt, post.LastError, post.ContainerID,
	).Scan(
		&result.ID, &result.AccountID, &result.Status, &result.PostType, &result.Caption,
		&result.ImageMinioPath, &result.ScheduledAtUTC, &result.TimezoneName, &result.RetryCount,
		&result.NextRetryAt, &result.LastError, &result.ContainerID,
		&result.CreatedAt, &result.UpdatedAt, &result.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPost returns a single post by ID (non-deleted).
func (r *Repository) GetPost(ctx context.Context, id uuid.UUID) (*ScheduledPost, error) {
	var result ScheduledPost
	err := r.pool.QueryRow(ctx, `
		SELECT id, account_id, status, post_type, caption, image_minio_path,
		       scheduled_at_utc, timezone_name, retry_count, next_retry_at,
		       last_error, container_id, created_at, updated_at, deleted_at
		FROM scheduled_posts
		WHERE id=$1 AND deleted_at IS NULL`, id,
	).Scan(
		&result.ID, &result.AccountID, &result.Status, &result.PostType, &result.Caption,
		&result.ImageMinioPath, &result.ScheduledAtUTC, &result.TimezoneName, &result.RetryCount,
		&result.NextRetryAt, &result.LastError, &result.ContainerID,
		&result.CreatedAt, &result.UpdatedAt, &result.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListPosts returns all non-deleted posts ordered by scheduled_at_utc DESC.
func (r *Repository) ListPosts(ctx context.Context) ([]ScheduledPost, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, account_id, status, post_type, caption, image_minio_path,
		       scheduled_at_utc, timezone_name, retry_count, next_retry_at,
		       last_error, container_id, created_at, updated_at, deleted_at
		FROM scheduled_posts
		WHERE deleted_at IS NULL
		ORDER BY scheduled_at_utc DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []ScheduledPost
	for rows.Next() {
		var p ScheduledPost
		if err := rows.Scan(
			&p.ID, &p.AccountID, &p.Status, &p.PostType, &p.Caption,
			&p.ImageMinioPath, &p.ScheduledAtUTC, &p.TimezoneName, &p.RetryCount,
			&p.NextRetryAt, &p.LastError, &p.ContainerID,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// UpdatePost updates caption, image path, scheduled time, and timezone of a post.
func (r *Repository) UpdatePost(ctx context.Context, id uuid.UUID, caption, imagePath string, scheduledAt time.Time, tzName string) (*ScheduledPost, error) {
	var result ScheduledPost
	err := r.pool.QueryRow(ctx, `
		UPDATE scheduled_posts
		SET caption=$1, image_minio_path=$2, scheduled_at_utc=$3, timezone_name=$4, updated_at=NOW()
		WHERE id=$5 AND deleted_at IS NULL
		RETURNING id, account_id, status, post_type, caption, image_minio_path,
		          scheduled_at_utc, timezone_name, retry_count, next_retry_at,
		          last_error, container_id, created_at, updated_at, deleted_at`,
		caption, imagePath, scheduledAt.UTC(), tzName, id,
	).Scan(
		&result.ID, &result.AccountID, &result.Status, &result.PostType, &result.Caption,
		&result.ImageMinioPath, &result.ScheduledAtUTC, &result.TimezoneName, &result.RetryCount,
		&result.NextRetryAt, &result.LastError, &result.ContainerID,
		&result.CreatedAt, &result.UpdatedAt, &result.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdatePostStatus sets the status and optional error reason on a post.
// This is idempotent — updating to the same status is not an error.
func (r *Repository) UpdatePostStatus(ctx context.Context, id uuid.UUID, status PostStatus, errReason string) error {
	var lastErr *string
	if errReason != "" {
		lastErr = &errReason
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE scheduled_posts
		SET status=$1, last_error=$2, updated_at=NOW()
		WHERE id=$3`,
		status, lastErr, id,
	)
	return err
}

// DisconnectAccountCascade moves all 'scheduled' posts for an account to 'draft'.
func (r *Repository) DisconnectAccountCascade(ctx context.Context, accountID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE scheduled_posts
		SET status='draft', updated_at=NOW()
		WHERE account_id=$1 AND status='scheduled' AND deleted_at IS NULL`,
		accountID,
	)
	return err
}

// SetNextRetry updates retry_count and next_retry_at on a post.
func (r *Repository) SetNextRetry(ctx context.Context, id uuid.UUID, retryCount int, nextRetryAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE scheduled_posts
		SET retry_count=$1, next_retry_at=$2, updated_at=NOW()
		WHERE id=$3`,
		retryCount, nextRetryAt, id,
	)
	return err
}

// SoftDeletePost marks a post as deleted.
func (r *Repository) SoftDeletePost(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE scheduled_posts SET deleted_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetPostForRetry sets a post back to 'scheduled' with next_retry_at=NOW().
func (r *Repository) ResetPostForRetry(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE scheduled_posts
		SET status='scheduled', next_retry_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListDuePosts returns scheduled posts that are due for publishing:
// status='scheduled' AND scheduled_at_utc <= now AND (next_retry_at IS NULL OR next_retry_at <= now).
func (r *Repository) ListDuePosts(ctx context.Context) ([]ScheduledPost, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, account_id, status, post_type, caption, image_minio_path,
		       scheduled_at_utc, timezone_name, retry_count, next_retry_at,
		       last_error, container_id, created_at, updated_at, deleted_at
		FROM scheduled_posts
		WHERE deleted_at IS NULL
		  AND status = 'scheduled'
		  AND scheduled_at_utc <= NOW()
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY scheduled_at_utc ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []ScheduledPost
	for rows.Next() {
		var p ScheduledPost
		if err := rows.Scan(
			&p.ID, &p.AccountID, &p.Status, &p.PostType, &p.Caption,
			&p.ImageMinioPath, &p.ScheduledAtUTC, &p.TimezoneName, &p.RetryCount,
			&p.NextRetryAt, &p.LastError, &p.ContainerID,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// ListExpiringAccounts returns connected accounts whose token expires within 7 days.
func (r *Repository) ListExpiringAccounts(ctx context.Context) ([]SocialAccount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, platform, account_name, ig_user_id, access_token,
		       token_expiry, status, user_id, created_at, updated_at
		FROM social_accounts
		WHERE deleted_at IS NULL
		  AND status = 'connected'
		  AND token_expiry IS NOT NULL
		  AND token_expiry <= NOW() + INTERVAL '7 days'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []SocialAccount
	for rows.Next() {
		var a SocialAccount
		if err := rows.Scan(
			&a.ID, &a.Platform, &a.AccountName, &a.IgUserID, &a.AccessToken,
			&a.TokenExpiry, &a.Status, &a.UserID, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// UpdateContainerID stores the Instagram container ID for a post after creation.
func (r *Repository) UpdateContainerID(ctx context.Context, id uuid.UUID, containerID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE scheduled_posts
		SET container_id=$1, updated_at=NOW()
		WHERE id=$2`,
		containerID, id,
	)
	return err
}

// UpdateAccountToken updates the access_token and token_expiry for an account after a token refresh.
func (r *Repository) UpdateAccountToken(ctx context.Context, id uuid.UUID, encryptedToken string, expiry time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE social_accounts
		SET access_token=$1, token_expiry=$2, updated_at=NOW()
		WHERE id=$3`,
		encryptedToken, expiry, id,
	)
	return err
}
