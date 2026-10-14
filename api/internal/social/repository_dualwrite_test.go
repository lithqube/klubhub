package social_test

// Regression test: every write path that touches image_storage_key
// must also write the same value to the legacy image_minio_path
// column during the rollback window. CodeRabbit flagged this on
// the EPK repo; the same gap exists here for the social repo
// (CreatePost, UpdatePost via EditPost, UpdatePostImage via
// the dedicated upload route).
//
// Strategy
// ---------
// 1. Use the package's testcontainer-backed TestMain.
// 2. CreatePost: insert, then read both columns — both must equal
//    the canonical key (RED anchor: image_minio_path is '').
// 3. UpdatePost (caption edit): change the image via the edit
//     endpoint, both must reflect the new key.
// 4. UpdatePostImage (upload): replace the image, both must reflect
//    the new key.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/social"
)

// readPostColumns reads the image_storage_key and image_minio_path
// columns for the given post id. Returns both values as read from
// the actual DB so the test exercises the persistence layer.
func readPostColumns(t *testing.T, id uuid.UUID) (storageKey, minioPath string) {
	t.Helper()
	ctx := context.Background()
	err := testPool.QueryRow(ctx, `
		SELECT image_storage_key, image_minio_path
		FROM scheduled_posts
		WHERE id = $1`, id,
	).Scan(&storageKey, &minioPath)
	require.NoError(t, err)
	return storageKey, minioPath
}

func TestSocialRepository_CreatePost_DualWritesBothColumns(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)
	acc := createTestAccount(t, repo)

	const canonical = "social/feed/create-dualwrite.jpg"
	post, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:       acc.ID,
		Status:          social.PostStatusScheduled,
		PostType:        social.PostTypeFeed,
		Caption:         "dual-write create",
		ImageStorageKey: canonical,
		ScheduledAtUTC:  time.Date(2026, 10, 24, 21, 45, 0, 0, time.UTC),
		TimezoneName:    "UTC",
	})
	require.NoError(t, err)

	storageKey, minioPath := readPostColumns(t, post.ID)

	assert.Equal(t, canonical, storageKey,
		"image_storage_key should hold the canonical key")
	assert.Equal(t, canonical, minioPath,
		"legacy image_minio_path must also hold the canonical key "+
			"during the rollback window")
}

func TestSocialRepository_UpdatePost_DualWritesBothColumns(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)
	acc := createTestAccount(t, repo)

	const initial = "social/feed/update-initial.jpg"
	post, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:       acc.ID,
		Status:          social.PostStatusScheduled,
		PostType:        social.PostTypeFeed,
		Caption:         "dual-write update",
		ImageStorageKey: initial,
		ScheduledAtUTC:  time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC),
		TimezoneName:    "UTC",
	})
	require.NoError(t, err)

	const replacement = "social/feed/update-replacement.jpg"
	_, err = repo.UpdatePost(context.Background(), post.ID,
		"updated caption",
		replacement,
		time.Date(2026, 10, 25, 22, 0, 0, 0, time.UTC),
		"Europe/Berlin",
	)
	require.NoError(t, err)

	storageKey, minioPath := readPostColumns(t, post.ID)
	assert.Equal(t, replacement, storageKey,
		"image_storage_key should reflect the new image after edit")
	assert.Equal(t, replacement, minioPath,
		"legacy image_minio_path must also reflect the new image "+
			"after the edit endpoint")
}

func TestSocialRepository_UpdatePostImage_DualWritesBothColumns(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)
	acc := createTestAccount(t, repo)

	post, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:       acc.ID,
		Status:          social.PostStatusScheduled,
		PostType:        social.PostTypeFeed,
		Caption:         "dual-write upload",
		ImageStorageKey: "social/feed/upload-initial.jpg",
		ScheduledAtUTC:  time.Date(2026, 10, 24, 23, 0, 0, 0, time.UTC),
		TimezoneName:    "UTC",
	})
	require.NoError(t, err)

	const replacement = "social/feed/upload-replacement.jpg"
	require.NoError(t,
		repo.UpdatePostImage(context.Background(), post.ID, replacement))

	storageKey, minioPath := readPostColumns(t, post.ID)
	assert.Equal(t, replacement, storageKey,
		"image_storage_key should reflect the new image after upload")
	assert.Equal(t, replacement, minioPath,
		"legacy image_minio_path must also reflect the new image "+
			"after the upload endpoint")
}