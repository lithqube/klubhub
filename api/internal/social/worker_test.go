package social

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/platform/crypto"
	"github.com/rs/zerolog"
)

// cryptoEncryptHelper wraps crypto.Encrypt for use in worker tests.
func cryptoEncryptHelper(key []byte, plaintext string) (string, error) {
	return crypto.Encrypt(key, plaintext)
}

// --- Mock implementations ---

type mockWorkerRepo struct {
	duePosts         []ScheduledPost
	expiringAccounts []SocialAccount
	account          *SocialAccount

	updatedStatuses []struct {
		id     uuid.UUID
		status PostStatus
		errMsg string
	}
	setNextRetryCalls []struct {
		id          uuid.UUID
		retryCount  int
		nextRetryAt time.Time
	}
	updatedContainerIDs []struct {
		id          uuid.UUID
		containerID string
	}
	updatedAccountStatuses []struct {
		id     uuid.UUID
		status string
	}
	updatedAccountTokens []struct {
		id    uuid.UUID
		token string
	}
}

func (m *mockWorkerRepo) ListDuePosts(_ context.Context) ([]ScheduledPost, error) {
	return m.duePosts, nil
}

func (m *mockWorkerRepo) ListExpiringAccounts(_ context.Context) ([]SocialAccount, error) {
	return m.expiringAccounts, nil
}

func (m *mockWorkerRepo) UpdatePostStatus(_ context.Context, id uuid.UUID, status PostStatus, errMsg string) error {
	m.updatedStatuses = append(m.updatedStatuses, struct {
		id     uuid.UUID
		status PostStatus
		errMsg string
	}{id, status, errMsg})
	return nil
}

func (m *mockWorkerRepo) SetNextRetry(_ context.Context, id uuid.UUID, retryCount int, nextRetryAt time.Time) error {
	m.setNextRetryCalls = append(m.setNextRetryCalls, struct {
		id          uuid.UUID
		retryCount  int
		nextRetryAt time.Time
	}{id, retryCount, nextRetryAt})
	return nil
}

func (m *mockWorkerRepo) UpdateContainerID(_ context.Context, id uuid.UUID, containerID string) error {
	m.updatedContainerIDs = append(m.updatedContainerIDs, struct {
		id          uuid.UUID
		containerID string
	}{id, containerID})
	return nil
}

func (m *mockWorkerRepo) UpdateAccountStatus(_ context.Context, id uuid.UUID, status string) error {
	m.updatedAccountStatuses = append(m.updatedAccountStatuses, struct {
		id     uuid.UUID
		status string
	}{id, status})
	return nil
}

func (m *mockWorkerRepo) UpdateAccountToken(_ context.Context, id uuid.UUID, token string, _ time.Time) error {
	m.updatedAccountTokens = append(m.updatedAccountTokens, struct {
		id    uuid.UUID
		token string
	}{id, token})
	return nil
}

func (m *mockWorkerRepo) GetAccount(_ context.Context) (*SocialAccount, error) {
	return m.account, nil
}

// mockInstagram is a mock Instagram client for worker tests.
type mockInstagram struct {
	createContainerFn func(ctx context.Context, igUserID, accessToken, imageURL, caption string, postType PostType) (string, error)
	publishFn         func(ctx context.Context, igUserID, accessToken, containerID string) (string, error)
	checkStatusFn     func(ctx context.Context, containerID, accessToken string) (ContainerStatus, error)
	refreshTokenFn    func(ctx context.Context, currentToken string) (string, time.Time, error)

	createCallCount  int
	publishCallCount int
}

func (m *mockInstagram) CreateContainer(ctx context.Context, igUserID, accessToken, imageURL, caption string, postType PostType) (string, error) {
	m.createCallCount++
	if m.createContainerFn != nil {
		return m.createContainerFn(ctx, igUserID, accessToken, imageURL, caption, postType)
	}
	return "container_123", nil
}

func (m *mockInstagram) PublishContainer(ctx context.Context, igUserID, accessToken, containerID string) (string, error) {
	m.publishCallCount++
	if m.publishFn != nil {
		return m.publishFn(ctx, igUserID, accessToken, containerID)
	}
	return "post_123", nil
}

func (m *mockInstagram) CheckContainerStatus(ctx context.Context, containerID, accessToken string) (ContainerStatus, error) {
	if m.checkStatusFn != nil {
		return m.checkStatusFn(ctx, containerID, accessToken)
	}
	return ContainerStatusFinished, nil
}

func (m *mockInstagram) RefreshToken(ctx context.Context, currentToken string) (string, time.Time, error) {
	if m.refreshTokenFn != nil {
		return m.refreshTokenFn(ctx, currentToken)
	}
	return "new_token", time.Now().Add(60 * 24 * time.Hour), nil
}

func (m *mockInstagram) convertPNGToJPEG(pngData []byte) ([]byte, error) {
	return pngData, nil
}

// mockStorage is a minimal storage mock.
type mockStorage struct {
	presignedURL string
	err          error
}

func (m *mockStorage) PresignedGetObject(_ context.Context, _, _ string, _ time.Duration, _ map[string]string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	url := m.presignedURL
	if url == "" {
		url = "https://storage.example.com/presigned/image.jpg"
	}
	return url, nil
}

func (m *mockStorage) Bucket() string { return "test-bucket" }

// --- Test helpers ---

// makeTestWorker creates a Worker wired with all mocks.
func makeTestWorker(repo *mockWorkerRepo, ig *mockInstagram, store *mockStorage) *Worker {
	// Use a real-looking 32-byte key for AES-256 (not actually used to encrypt in unit tests
	// since the account access_token is pre-set as plaintext in mock repo).
	// We override GetAccount to return a plaintext token, and we need Decrypt to work.
	// Use a known key and encrypt the test token so Decrypt succeeds.
	return &Worker{
		repo:      repo,
		instagram: ig,
		storage:   store,
		cryptoKey: []byte("test-key-32-bytes-for-aes256!!XX"), // 32 bytes
		log:       zerolog.Nop(),
		// lastPublishedAt is zero — means no 30s rate limit initially
	}
}

// --- Test cases ---

func TestWorker_BackoffTime(t *testing.T) {
	cases := []struct {
		retryCount      int
		expectedMinutes int
	}{
		{0, 5},
		{1, 20},
		{2, 80},
	}
	before := time.Now()
	for _, tc := range cases {
		got := backoffTime(tc.retryCount)
		minExpected := before.Add(time.Duration(tc.expectedMinutes)*time.Minute - time.Second)
		maxExpected := time.Now().Add(time.Duration(tc.expectedMinutes)*time.Minute + time.Second)
		if got.Before(minExpected) || got.After(maxExpected) {
			t.Errorf("backoffTime(%d) = %v; expected ~%d minutes from now", tc.retryCount, got, tc.expectedMinutes)
		}
	}
}

func TestWorker_PermanentlyFailed_NeverCallsInstagram(t *testing.T) {
	postID := uuid.New()
	accID := uuid.New()

	repo := &mockWorkerRepo{
		duePosts: []ScheduledPost{
			{
				ID:         postID,
				AccountID:  accID,
				Status:     PostStatusScheduled,
				RetryCount: 3, // maxed out
				PostType:   PostTypeFeed,
			},
		},
	}
	ig := &mockInstagram{}
	store := &mockStorage{}

	w := makeTestWorker(repo, ig, store)

	err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick error: %v", err)
	}

	// Instagram client must NOT have been called.
	if ig.createCallCount > 0 {
		t.Error("expected CreateContainer to NOT be called for permanently_failed post")
	}
	if ig.publishCallCount > 0 {
		t.Error("expected PublishContainer to NOT be called for permanently_failed post")
	}

	// Post must be marked permanently_failed.
	if len(repo.updatedStatuses) == 0 {
		t.Fatal("expected UpdatePostStatus to be called")
	}
	last := repo.updatedStatuses[len(repo.updatedStatuses)-1]
	if last.status != PostStatusPermanentlyFailed {
		t.Errorf("expected permanently_failed, got %q", last.status)
	}
}

func TestWorker_RateLimitStopsRemainingPosts(t *testing.T) {
	post1 := uuid.New()
	post2 := uuid.New()
	accID := uuid.New()

	repo := &mockWorkerRepo{
		duePosts: []ScheduledPost{
			{ID: post1, AccountID: accID, Status: PostStatusScheduled, RetryCount: 0, PostType: PostTypeFeed, ImageMinioPath: "img1.jpg"},
			{ID: post2, AccountID: accID, Status: PostStatusScheduled, RetryCount: 0, PostType: PostTypeFeed, ImageMinioPath: "img2.jpg"},
		},
		account: &SocialAccount{
			ID:          accID,
			IgUserID:    "ig_123",
			AccessToken: encryptTestToken(t, "access_token_xyz"),
		},
	}
	ig := &mockInstagram{
		createContainerFn: func(_ context.Context, _, _, _, _ string, _ PostType) (string, error) {
			return "", &RateLimitError{RetryAfter: 15 * time.Minute}
		},
	}
	store := &mockStorage{}

	w := makeTestWorker(repo, ig, store)
	// Ensure rate limit cooldown is not active initially.
	w.lastPublishedAt = time.Time{}

	err := w.tick(context.Background())
	// tick should NOT return an error — it swallows the RateLimitError and stops.
	if err != nil {
		t.Fatalf("expected tick to return nil on rate limit, got: %v", err)
	}

	// Only ONE CreateContainer call should have been made (post2 should be skipped).
	if ig.createCallCount != 1 {
		t.Errorf("expected 1 CreateContainer call, got %d", ig.createCallCount)
	}

	// post2 should NOT have had UpdatePostStatus→publishing called.
	publishingForPost2 := 0
	for _, s := range repo.updatedStatuses {
		if s.id == post2 && s.status == PostStatusPublishing {
			publishingForPost2++
		}
	}
	if publishingForPost2 > 0 {
		t.Error("expected post2 to NOT be marked publishing after rate limit on post1")
	}
}

// TestWorker_RecordRateLimitSanitizesReason pins the B.4 fix-subagent
// "429 sanitization bypass" finding: when CreateContainer returns a
// RateLimitError, the worker MUST route the persisted reason through
// SanitizeTransportError. Today RateLimitError.Error() is clean, but
// the bypass is structural: any future change to the error string
// (e.g. embedding provider body fragments) would expose credentials
// in the DB.
//
// To exercise the sanitization path with a non-clean message, we
// use a wrapper type that satisfies errors.As(*RateLimitError) but
// overrides Error() to include a credential-shaped substring.
// errors.As walks the chain via Unwrap; the wrapper embeds
// *RateLimitError directly so the type assertion succeeds.
func TestWorker_RecordRateLimitSanitizesReason(t *testing.T) {
	postID := uuid.New()
	accID := uuid.New()

	repo := &mockWorkerRepo{}
	w := makeTestWorker(repo, &mockInstagram{}, &mockStorage{})
	post := ScheduledPost{ID: postID, AccountID: accID}

	inner := &RateLimitError{RetryAfter: 15 * time.Minute}
	tainted := &taintedRateLimitError{inner: inner}
	w.recordRateLimit(context.Background(), post, tainted)

	// Find the persisted reason.
	var reason string
	for _, s := range repo.updatedStatuses {
		if s.id == postID && s.status == PostStatusFailed {
			reason = s.errMsg
		}
	}
	if reason == "" {
		t.Fatal("expected UpdatePostStatus→failed with a reason")
	}
	// The fix routes through SanitizeTransportError; this asserts
	// the credential substring is gone from the persisted reason.
	if strings.Contains(reason, "LEAKED_TOKEN_429") {
		t.Errorf("persisted rate-limit reason leaked credential: %s", reason)
	}
}

// taintedRateLimitError embeds *RateLimitError so errors.As finds it
// via the wrapper's Unwrap, while overriding Error() with a message
// that includes a fake access_token fragment. It is the test-side
// stand-in for a future RateLimitError whose Error() includes
// provider-supplied content.
type taintedRateLimitError struct {
	inner *RateLimitError
}

func (e *taintedRateLimitError) Error() string {
	return "instagram rate limited: retry after 15m0s; body=" +
		`{"error":"OAuthException","error_message":"see access_token=LEAKED_TOKEN_429"}`
}

// Unwrap exposes the embedded *RateLimitError so errors.As can find it.
func (e *taintedRateLimitError) Unwrap() error {
	return e.inner
}

func TestWorker_SuccessfulPublish(t *testing.T) {
	postID := uuid.New()
	accID := uuid.New()

	repo := &mockWorkerRepo{
		duePosts: []ScheduledPost{
			{ID: postID, AccountID: accID, Status: PostStatusScheduled, RetryCount: 0, PostType: PostTypeFeed, ImageMinioPath: "img.jpg"},
		},
		account: &SocialAccount{
			ID:          accID,
			IgUserID:    "ig_123",
			AccessToken: encryptTestToken(t, "access_token_xyz"),
		},
	}
	ig := &mockInstagram{}
	store := &mockStorage{}

	w := makeTestWorker(repo, ig, store)

	err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick error: %v", err)
	}

	// Should have published exactly once.
	if ig.createCallCount != 1 {
		t.Errorf("expected 1 CreateContainer call, got %d", ig.createCallCount)
	}
	if ig.publishCallCount != 1 {
		t.Errorf("expected 1 PublishContainer call, got %d", ig.publishCallCount)
	}

	// Final status should be published.
	foundPublished := false
	for _, s := range repo.updatedStatuses {
		if s.id == postID && s.status == PostStatusPublished {
			foundPublished = true
		}
	}
	if !foundPublished {
		t.Errorf("expected post %v to be marked published, got statuses: %+v", postID, repo.updatedStatuses)
	}
}

func TestWorker_RetryWithAlreadyPublishedContainer(t *testing.T) {
	postID := uuid.New()
	accID := uuid.New()
	containerIDVal := "existing_container_789"

	repo := &mockWorkerRepo{
		duePosts: []ScheduledPost{
			{
				ID:             postID,
				AccountID:      accID,
				Status:         PostStatusScheduled,
				RetryCount:     1, // retry
				ContainerID:    &containerIDVal,
				PostType:       PostTypeFeed,
				ImageMinioPath: "img.jpg",
			},
		},
		account: &SocialAccount{
			ID:          accID,
			IgUserID:    "ig_123",
			AccessToken: encryptTestToken(t, "access_token_xyz"),
		},
	}
	ig := &mockInstagram{
		checkStatusFn: func(_ context.Context, containerID, _ string) (ContainerStatus, error) {
			if containerID == containerIDVal {
				return ContainerStatusPublished, nil
			}
			return ContainerStatusFinished, nil
		},
	}
	store := &mockStorage{}

	w := makeTestWorker(repo, ig, store)

	err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick error: %v", err)
	}

	// Should NOT have called CreateContainer or PublishContainer.
	if ig.createCallCount > 0 {
		t.Error("expected CreateContainer to NOT be called when container already published")
	}
	if ig.publishCallCount > 0 {
		t.Error("expected PublishContainer to NOT be called when container already published")
	}

	// Should be marked published.
	foundPublished := false
	for _, s := range repo.updatedStatuses {
		if s.id == postID && s.status == PostStatusPublished {
			foundPublished = true
		}
	}
	if !foundPublished {
		t.Errorf("expected post %v to be marked published via container check", postID)
	}
}

func TestWorker_TokenRefreshFailureMarksDisconnected(t *testing.T) {
	accID := uuid.New()

	repo := &mockWorkerRepo{
		expiringAccounts: []SocialAccount{
			{
				ID:          accID,
				IgUserID:    "ig_456",
				AccessToken: encryptTestToken(t, "expiring_token"),
				Status:      "connected",
			},
		},
	}
	ig := &mockInstagram{
		refreshTokenFn: func(_ context.Context, _ string) (string, time.Time, error) {
			return "", time.Time{}, errors.New("invalid token")
		},
	}
	store := &mockStorage{}

	w := makeTestWorker(repo, ig, store)
	w.refreshExpiringTokens(context.Background())

	// Account should be marked disconnected.
	foundDisconnected := false
	for _, s := range repo.updatedAccountStatuses {
		if s.id == accID && s.status == "disconnected" {
			foundDisconnected = true
		}
	}
	if !foundDisconnected {
		t.Errorf("expected account %v to be marked disconnected after token refresh failure", accID)
	}
}

func TestWorker_TokenRefreshSuccess(t *testing.T) {
	accID := uuid.New()

	repo := &mockWorkerRepo{
		expiringAccounts: []SocialAccount{
			{
				ID:          accID,
				IgUserID:    "ig_456",
				AccessToken: encryptTestToken(t, "old_token"),
				Status:      "connected",
			},
		},
	}
	ig := &mockInstagram{
		refreshTokenFn: func(_ context.Context, currentToken string) (string, time.Time, error) {
			if currentToken == "old_token" {
				return "new_token_refreshed", time.Now().Add(60 * 24 * time.Hour), nil
			}
			return "", time.Time{}, errors.New("unexpected token")
		},
	}
	store := &mockStorage{}

	w := makeTestWorker(repo, ig, store)
	w.refreshExpiringTokens(context.Background())

	// Account should NOT be marked disconnected.
	for _, s := range repo.updatedAccountStatuses {
		if s.id == accID && s.status == "disconnected" {
			t.Error("account should NOT be disconnected on successful token refresh")
		}
	}

	// UpdateAccountToken should have been called.
	if len(repo.updatedAccountTokens) == 0 {
		t.Error("expected UpdateAccountToken to be called on successful refresh")
	}
}

func TestWorker_PublishFailureSetsRetry(t *testing.T) {
	postID := uuid.New()
	accID := uuid.New()

	repo := &mockWorkerRepo{
		duePosts: []ScheduledPost{
			{ID: postID, AccountID: accID, Status: PostStatusScheduled, RetryCount: 0, PostType: PostTypeFeed, ImageMinioPath: "img.jpg"},
		},
		account: &SocialAccount{
			ID:          accID,
			IgUserID:    "ig_123",
			AccessToken: encryptTestToken(t, "token"),
		},
	}
	ig := &mockInstagram{
		publishFn: func(_ context.Context, _, _, _ string) (string, error) {
			return "", errors.New("publish error: connection reset")
		},
	}
	store := &mockStorage{}

	w := makeTestWorker(repo, ig, store)

	err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick error: %v", err)
	}

	// SetNextRetry should have been called.
	if len(repo.setNextRetryCalls) == 0 {
		t.Fatal("expected SetNextRetry to be called on publish failure")
	}
	retry := repo.setNextRetryCalls[0]
	if retry.retryCount != 1 {
		t.Errorf("expected retryCount=1, got %d", retry.retryCount)
	}

	// Status should be failed.
	foundFailed := false
	for _, s := range repo.updatedStatuses {
		if s.id == postID && s.status == PostStatusFailed {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Error("expected post to be marked failed after publish error")
	}
}

// encryptTestToken produces an encrypted token for the given plaintext using the test key.
func encryptTestToken(t *testing.T, plaintext string) string {
	t.Helper()
	key := []byte("test-key-32-bytes-for-aes256!!XX")
	encrypted, err := cryptoEncryptHelper(key, plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt test token: %v", err)
	}
	return encrypted
}
