package social

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// TestWorker_FailPostSanitizesReasonBeforePersisting verifies that the
// error string written to scheduled_posts.last_error via
// UpdatePostStatus has no credential sentinels — even when the
// underlying transport error includes them.
//
// The audit found (worker.go:247–250 pre-B.4) that transport errors
// were passed verbatim into UpdatePostStatus, persisting credentials
// to the DB. This test pins the fix.
func TestWorker_FailPostSanitizesReasonBeforePersisting(t *testing.T) {
	postID := uuid.New()
	accID := uuid.New()

	repo := &mockWorkerRepo{
		duePosts: []ScheduledPost{
			{ID: postID, AccountID: accID, Status: PostStatusScheduled, RetryCount: 0, PostType: PostTypeFeed, ImageMinioPath: "img.jpg"},
		},
		account: &SocialAccount{
			ID:          accID,
			IgUserID:    "ig_123",
			AccessToken: encryptTestToken(t, "any_token"),
		},
	}
	ig := &mockInstagram{
		publishFn: func(_ context.Context, _, _, _ string) (string, error) {
			return "", errors.New(`publish container: status 400: error=invalid&access_token=SECRET_TOKEN_LEAKED_XYZ&client_id=APP_ID_LEAKED`)
		},
	}
	store := &mockStorage{}
	w := makeTestWorker(repo, ig, store)

	err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick error: %v", err)
	}

	// Find the UpdatePostStatus call for this post.
	var persistedReason string
	for _, s := range repo.updatedStatuses {
		if s.id == postID && s.status == PostStatusFailed {
			persistedReason = s.errMsg
		}
	}
	if persistedReason == "" {
		t.Fatal("expected UpdatePostStatus to be called with a failed reason")
	}
	for _, sentinel := range []string{"SECRET_TOKEN_LEAKED_XYZ", "APP_ID_LEAKED"} {
		if strings.Contains(persistedReason, sentinel) {
			t.Errorf("persisted reason leaked credential %q: %s", sentinel, persistedReason)
		}
	}
}

// TestWorker_FailPost_LogsOriginalDetailSeparately verifies that the
// raw (credential-bearing) error is still available via the structured
// log, while only the sanitized string reaches persistent storage.
// Operators need the detail to debug provider errors, but DB consumers
// must not see it.
func TestWorker_FailPost_LogsOriginalDetailSeparately(t *testing.T) {
	postID := uuid.New()
	accID := uuid.New()

	repo := &mockWorkerRepo{
		duePosts: []ScheduledPost{
			{ID: postID, AccountID: accID, Status: PostStatusScheduled, RetryCount: 0, PostType: PostTypeFeed, ImageMinioPath: "img.jpg"},
		},
		account: &SocialAccount{
			ID:          accID,
			IgUserID:    "ig_123",
			AccessToken: encryptTestToken(t, "any_token"),
		},
	}
	ig := &mockInstagram{
		publishFn: func(_ context.Context, _, _, _ string) (string, error) {
			return "", errors.New(`network: access_token=SECRET_TOKEN_LOGGED`)
		},
	}
	store := &mockStorage{}

	logBuf := &strings.Builder{}
	logger := zerolog.New(logBuf).Level(zerolog.WarnLevel)

	w := &Worker{
		repo:      repo,
		instagram: ig,
		storage:   store,
		cryptoKey: []byte("test-key-32-bytes-for-aes256!!XX"),
		log:       logger,
	}

	err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick error: %v", err)
	}

	logged := logBuf.String()
	if !strings.Contains(logged, "SECRET_TOKEN_LOGGED") {
		t.Errorf("expected original credential in log buffer for ops debugging, got: %s", logged)
	}

	// But the persisted reason must NOT contain the secret.
	for _, s := range repo.updatedStatuses {
		if s.status == PostStatusFailed {
			if strings.Contains(s.errMsg, "SECRET_TOKEN_LOGGED") {
				t.Errorf("persisted reason must not contain secret, got: %s", s.errMsg)
			}
		}
	}
}
