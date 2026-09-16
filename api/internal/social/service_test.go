package social_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/social"
)

// mockRepo is a minimal in-memory stub for service tests.
type mockRepo struct {
	account   *social.SocialAccount
	posts     map[uuid.UUID]*social.ScheduledPost
	upsertErr error
	resetErr  error
}

func newMockRepo() *mockRepo {
	return &mockRepo{posts: make(map[uuid.UUID]*social.ScheduledPost)}
}

func (m *mockRepo) GetAccount(_ context.Context) (*social.SocialAccount, error) {
	return m.account, nil
}

func (m *mockRepo) UpsertAccount(_ context.Context, acc social.SocialAccount) (*social.SocialAccount, error) {
	if m.upsertErr != nil {
		return nil, m.upsertErr
	}
	acc.ID = uuid.New()
	acc.CreatedAt = time.Now()
	acc.UpdatedAt = time.Now()
	m.account = &acc
	return &acc, nil
}

func (m *mockRepo) UpdateAccountStatus(_ context.Context, id uuid.UUID, status string) error {
	if m.account != nil && m.account.ID == id {
		m.account.Status = status
	}
	return nil
}

func (m *mockRepo) CreatePost(_ context.Context, post social.ScheduledPost) (*social.ScheduledPost, error) {
	post.ID = uuid.New()
	post.CreatedAt = time.Now()
	post.UpdatedAt = time.Now()
	m.posts[post.ID] = &post
	return &post, nil
}

func (m *mockRepo) GetPost(_ context.Context, id uuid.UUID) (*social.ScheduledPost, error) {
	p, ok := m.posts[id]
	if !ok {
		return nil, social.ErrNotFound
	}
	return p, nil
}

func (m *mockRepo) ListPosts(_ context.Context) ([]social.ScheduledPost, error) {
	posts := make([]social.ScheduledPost, 0, len(m.posts))
	for _, p := range m.posts {
		posts = append(posts, *p)
	}
	return posts, nil
}

func (m *mockRepo) UpdatePost(_ context.Context, id uuid.UUID, caption, imagePath string, scheduledAt time.Time, tzName string) (*social.ScheduledPost, error) {
	p, ok := m.posts[id]
	if !ok {
		return nil, social.ErrNotFound
	}
	p.Caption = caption
	p.ImageMinioPath = imagePath
	p.ScheduledAtUTC = scheduledAt
	p.TimezoneName = tzName
	return p, nil
}

func (m *mockRepo) UpdatePostStatus(_ context.Context, id uuid.UUID, status social.PostStatus, errReason string) error {
	p, ok := m.posts[id]
	if !ok {
		return social.ErrNotFound
	}
	p.Status = status
	return nil
}

func (m *mockRepo) DisconnectAccountCascade(_ context.Context, _ uuid.UUID) error {
	for _, p := range m.posts {
		if p.Status == social.PostStatusScheduled {
			p.Status = social.PostStatusDraft
		}
	}
	return nil
}

func (m *mockRepo) SetNextRetry(_ context.Context, id uuid.UUID, retryCount int, nextRetryAt time.Time) error {
	p, ok := m.posts[id]
	if !ok {
		return social.ErrNotFound
	}
	p.RetryCount = retryCount
	p.NextRetryAt = &nextRetryAt
	return nil
}

func (m *mockRepo) SoftDeletePost(_ context.Context, id uuid.UUID) error {
	if _, ok := m.posts[id]; !ok {
		return social.ErrNotFound
	}
	delete(m.posts, id)
	return nil
}

func (m *mockRepo) ResetPostForRetry(_ context.Context, id uuid.UUID) error {
	if m.resetErr != nil {
		return m.resetErr
	}
	p, ok := m.posts[id]
	if !ok {
		return social.ErrNotFound
	}
	p.Status = social.PostStatusScheduled
	now := time.Now()
	p.NextRetryAt = &now
	return nil
}

// TestSocialService_SchedulePost_UTCConversion verifies timezone-aware UTC conversion.
// Input "2026-10-24T23:45" with timezone "Europe/Berlin" (UTC+2 in summer) should store as "2026-10-24T21:45:00Z".
func TestSocialService_SchedulePost_UTCConversion(t *testing.T) {
	repo := newMockRepo()
	svc := social.NewService(repo, social.ServiceConfig{})

	accountID := uuid.New()
	post, err := svc.SchedulePost(context.Background(), social.CreatePostRequest{
		AccountID:    accountID,
		PostType:     social.PostTypeFeed,
		Caption:      "UTC test",
		ScheduledAt:  "2026-10-24T23:45",
		TimezoneName: "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("SchedulePost failed: %v", err)
	}

	// Europe/Berlin on Oct 24 is UTC+2 (CEST, last Sunday of October is DST change)
	// 23:45 local = 21:45 UTC
	expected := time.Date(2026, 10, 24, 21, 45, 0, 0, time.UTC)
	if !post.ScheduledAtUTC.Equal(expected) {
		t.Errorf("UTC conversion failed: got %v, want %v", post.ScheduledAtUTC, expected)
	}
}

// TestSocialService_EditPost_BlockedWhenNotScheduled verifies ErrEditBlocked for non-scheduled posts.
func TestSocialService_EditPost_BlockedWhenNotScheduled(t *testing.T) {
	repo := newMockRepo()
	svc := social.NewService(repo, social.ServiceConfig{})

	// Create a published post
	publishedPost, _ := repo.CreatePost(context.Background(), social.ScheduledPost{
		Status:         social.PostStatusPublished,
		PostType:       social.PostTypeFeed,
		Caption:        "published",
		ImageMinioPath: "path",
		ScheduledAtUTC: time.Now().UTC(),
		TimezoneName:   "UTC",
	})

	_, err := svc.EditPost(context.Background(), publishedPost.ID, social.EditPostRequest{
		Caption:      "updated",
		ScheduledAt:  "2026-12-01T12:00",
		TimezoneName: "UTC",
	})
	if err == nil {
		t.Fatal("expected ErrEditBlocked, got nil")
	}
	if err.Error() != social.ErrEditBlocked.Error() {
		t.Errorf("expected ErrEditBlocked, got: %v", err)
	}
}

// TestSocialService_ValidateImage_RejectsOversizedFile verifies >8MB is rejected.
func TestSocialService_ValidateImage_RejectsOversizedFile(t *testing.T) {
	svc := social.NewService(newMockRepo(), social.ServiceConfig{})

	// Create a minimal valid JPEG header then pad to >8MB
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 320, 320))
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	// Pad with zeros to exceed 8MB
	padding := make([]byte, 9*1024*1024)
	buf.Write(padding)

	err := svc.ValidateImage(buf.Bytes(), social.PostTypeFeed)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
	// Note: MIME is checked first (JPEG header present), then size
	if err != social.ErrFileTooLarge {
		t.Errorf("expected ErrFileTooLarge, got: %v", err)
	}
}

// TestSocialService_ValidateImage_RejectsInvalidMIME verifies non-image bytes are rejected.
func TestSocialService_ValidateImage_RejectsInvalidMIME(t *testing.T) {
	svc := social.NewService(newMockRepo(), social.ServiceConfig{})

	data := []byte("this is a plain text file, not an image")
	err := svc.ValidateImage(data, social.PostTypeFeed)
	if err != social.ErrInvalidMIME {
		t.Errorf("expected ErrInvalidMIME, got: %v", err)
	}
}

// TestSocialService_ValidateImage_RejectsFeedPortraitOutsideRatio verifies 1:3 portrait is rejected for feed.
func TestSocialService_ValidateImage_RejectsFeedPortraitOutsideRatio(t *testing.T) {
	svc := social.NewService(newMockRepo(), social.ServiceConfig{})

	// Create a 320x960 PNG (ratio 0.333 < 0.8 min for feed)
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 320, 960))
	// Set a pixel to make it non-trivial
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	err := svc.ValidateImage(buf.Bytes(), social.PostTypeFeed)
	if err != social.ErrInvalidDimensions {
		t.Errorf("expected ErrInvalidDimensions for 1:3 portrait feed, got: %v", err)
	}
}

// TestSocialService_ValidateImage_AcceptsFeedSquare verifies 1:1 (square) is valid for feed.
func TestSocialService_ValidateImage_AcceptsFeedSquare(t *testing.T) {
	svc := social.NewService(newMockRepo(), social.ServiceConfig{})

	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 600, 600))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	err := svc.ValidateImage(buf.Bytes(), social.PostTypeFeed)
	if err != nil {
		t.Errorf("expected no error for 1:1 square feed image, got: %v", err)
	}
}

// TestSocialService_ValidateImage_RejectsInvalidStoryRatio verifies non-9:16 story image is rejected.
func TestSocialService_ValidateImage_RejectsInvalidStoryRatio(t *testing.T) {
	svc := social.NewService(newMockRepo(), social.ServiceConfig{})

	// 1:1 square — ratio 1.0, far from 9:16 (0.5625)
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 600, 600))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	err := svc.ValidateImage(buf.Bytes(), social.PostTypeStory)
	if err != social.ErrInvalidDimensions {
		t.Errorf("expected ErrInvalidDimensions for 1:1 story, got: %v", err)
	}
}

// TestSocialService_GetOAuthURL_ContainsRequiredParams verifies the Instagram OAuth URL is well-formed.
func TestSocialService_GetOAuthURL_ContainsRequiredParams(t *testing.T) {
	svc := social.NewService(newMockRepo(), social.ServiceConfig{
		InstagramAppID:       "my_app_id",
		InstagramRedirectURI: "https://example.com/callback",
	})

	oauthURL, err := svc.GetOAuthURL(context.Background())
	if err != nil {
		t.Fatalf("GetOAuthURL failed: %v", err)
	}

	for _, want := range []string{"client_id=my_app_id", "redirect_uri=", "scope=", "state=", "response_type=code"} {
		if !contains(oauthURL, want) {
			t.Errorf("OAuth URL missing %q: %s", want, oauthURL)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
