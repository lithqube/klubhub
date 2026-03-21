package social_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/social"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}
	defer container.Terminate(ctx) //nolint:errcheck

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("failed to get connection string: " + err.Error())
	}

	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		panic("failed to open sql.DB: " + err.Error())
	}
	defer sqlDB.Close()

	if err := migrations.RunMigrations(sqlDB); err != nil {
		panic("failed to run migrations: " + err.Error())
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		panic("failed to create pgxpool: " + err.Error())
	}
	defer testPool.Close()

	m.Run()
}

func clearSocial(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, "DELETE FROM scheduled_posts")
	if err != nil {
		t.Fatalf("failed to clear scheduled_posts: %v", err)
	}
	_, err = testPool.Exec(ctx, "DELETE FROM social_accounts")
	if err != nil {
		t.Fatalf("failed to clear social_accounts: %v", err)
	}
}

func createTestAccount(t *testing.T, repo *social.Repository) *social.SocialAccount {
	t.Helper()
	acc, err := repo.UpsertAccount(context.Background(), social.SocialAccount{
		Platform:    "instagram",
		AccountName: "testuser",
		IgUserID:    "12345",
		AccessToken: "tok",
		Status:      "connected",
	})
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}
	return acc
}

// TestSocialRepository_GetAccount_EmptyReturnsNil verifies GetAccount returns nil (not error) on empty table.
func TestSocialRepository_GetAccount_EmptyReturnsNil(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)

	acc, err := repo.GetAccount(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if acc != nil {
		t.Errorf("expected nil account on empty table, got: %+v", acc)
	}
}

// TestSocialRepository_CreatePost_Roundtrip verifies a post is stored and retrieved correctly.
func TestSocialRepository_CreatePost_Roundtrip(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)

	acc := createTestAccount(t, repo)

	scheduledAt := time.Date(2026, 10, 24, 21, 45, 0, 0, time.UTC)
	post, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:      acc.ID,
		Status:         social.PostStatusScheduled,
		PostType:       social.PostTypeFeed,
		Caption:        "Test caption",
		ImageMinioPath: "social/test.jpg",
		ScheduledAtUTC: scheduledAt,
		TimezoneName:   "Europe/Berlin",
		RetryCount:     0,
	})
	if err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}

	// Verify returned fields
	if post.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("expected non-zero UUID")
	}
	if post.Caption != "Test caption" {
		t.Errorf("caption mismatch: got %q", post.Caption)
	}
	if !post.ScheduledAtUTC.UTC().Equal(scheduledAt) {
		t.Errorf("scheduled_at_utc mismatch: got %v, want %v", post.ScheduledAtUTC.UTC(), scheduledAt)
	}
	if post.TimezoneName != "Europe/Berlin" {
		t.Errorf("timezone_name mismatch: got %q", post.TimezoneName)
	}

	// Roundtrip via GetPost
	fetched, err := repo.GetPost(context.Background(), post.ID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}
	if fetched.ID != post.ID {
		t.Errorf("ID mismatch: got %v, want %v", fetched.ID, post.ID)
	}
}

// TestSocialRepository_DisconnectAccountCascade verifies only 'scheduled' posts move to 'draft',
// not 'published' posts.
func TestSocialRepository_DisconnectAccountCascade(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)

	acc := createTestAccount(t, repo)

	scheduledAt := time.Now().Add(1 * time.Hour).UTC()

	// Create a scheduled post (should become draft)
	scheduledPost, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:      acc.ID,
		Status:         social.PostStatusScheduled,
		PostType:       social.PostTypeFeed,
		Caption:        "Scheduled post",
		ImageMinioPath: "social/s.jpg",
		ScheduledAtUTC: scheduledAt,
		TimezoneName:   "UTC",
	})
	if err != nil {
		t.Fatalf("CreatePost (scheduled) failed: %v", err)
	}

	// Create a published post (should NOT change)
	publishedPost, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:      acc.ID,
		Status:         social.PostStatusPublished,
		PostType:       social.PostTypeFeed,
		Caption:        "Published post",
		ImageMinioPath: "social/p.jpg",
		ScheduledAtUTC: scheduledAt.Add(-24 * time.Hour),
		TimezoneName:   "UTC",
	})
	if err != nil {
		t.Fatalf("CreatePost (published) failed: %v", err)
	}

	// Run cascade
	if err := repo.DisconnectAccountCascade(context.Background(), acc.ID); err != nil {
		t.Fatalf("DisconnectAccountCascade failed: %v", err)
	}

	// Scheduled post should now be draft
	sp, err := repo.GetPost(context.Background(), scheduledPost.ID)
	if err != nil {
		t.Fatalf("GetPost (scheduled) failed: %v", err)
	}
	if sp.Status != social.PostStatusDraft {
		t.Errorf("expected status=draft, got %q", sp.Status)
	}

	// Published post should remain published
	pp, err := repo.GetPost(context.Background(), publishedPost.ID)
	if err != nil {
		t.Fatalf("GetPost (published) failed: %v", err)
	}
	if pp.Status != social.PostStatusPublished {
		t.Errorf("expected status=published, got %q", pp.Status)
	}
}

// TestSocialRepository_ListPosts_OrderedByScheduledAtUTCDesc verifies ordering.
func TestSocialRepository_ListPosts_OrderedByScheduledAtUTCDesc(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)

	acc := createTestAccount(t, repo)
	base := time.Now().UTC()

	for i, offset := range []time.Duration{1 * time.Hour, 2 * time.Hour, 3 * time.Hour} {
		_, err := repo.CreatePost(context.Background(), social.ScheduledPost{
			AccountID:      acc.ID,
			Status:         social.PostStatusScheduled,
			PostType:       social.PostTypeFeed,
			Caption:        "post",
			ImageMinioPath: "social/x.jpg",
			ScheduledAtUTC: base.Add(offset),
			TimezoneName:   "UTC",
			RetryCount:     i,
		})
		if err != nil {
			t.Fatalf("CreatePost failed: %v", err)
		}
	}

	posts, err := repo.ListPosts(context.Background())
	if err != nil {
		t.Fatalf("ListPosts failed: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(posts))
	}
	// Verify descending order
	for i := 1; i < len(posts); i++ {
		if posts[i-1].ScheduledAtUTC.Before(posts[i].ScheduledAtUTC) {
			t.Errorf("posts not in DESC order at index %d", i)
		}
	}
}

// TestSocialRepository_UpdatePostStatus_Idempotent verifies no error on same-status update.
func TestSocialRepository_UpdatePostStatus_Idempotent(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)

	acc := createTestAccount(t, repo)
	post, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:      acc.ID,
		Status:         social.PostStatusPublished,
		PostType:       social.PostTypeFeed,
		Caption:        "Published",
		ImageMinioPath: "social/x.jpg",
		ScheduledAtUTC: time.Now().Add(-1 * time.Hour).UTC(),
		TimezoneName:   "UTC",
	})
	if err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}

	// Update to the same status — should not error
	if err := repo.UpdatePostStatus(context.Background(), post.ID, social.PostStatusPublished, ""); err != nil {
		t.Errorf("UpdatePostStatus idempotent failed: %v", err)
	}
}

// TestSocialRepository_ResetPostForRetry verifies status resets to scheduled.
func TestSocialRepository_ResetPostForRetry(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)

	acc := createTestAccount(t, repo)
	post, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID:      acc.ID,
		Status:         social.PostStatusFailed,
		PostType:       social.PostTypeFeed,
		Caption:        "Retrying",
		ImageMinioPath: "social/x.jpg",
		ScheduledAtUTC: time.Now().Add(1 * time.Hour).UTC(),
		TimezoneName:   "UTC",
	})
	if err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}

	if err := repo.ResetPostForRetry(context.Background(), post.ID); err != nil {
		t.Fatalf("ResetPostForRetry failed: %v", err)
	}

	updated, err := repo.GetPost(context.Background(), post.ID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}
	if updated.Status != social.PostStatusScheduled {
		t.Errorf("expected status=scheduled, got %q", updated.Status)
	}
	if updated.NextRetryAt == nil {
		t.Error("expected next_retry_at to be set")
	}
}
