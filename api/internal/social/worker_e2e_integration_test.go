package social_test

// E1/E2 image-persistence verification — real Go integration tests.
//
// Runs against a real Postgres testcontainer provisioned by
// repository_test.go's TestMain:
//
//	go test -count=1 -run TestE2E_ ./internal/social/...
//
// The package-social companion (worker_e2e_helpers_test.go)
// provides NewWorkerForTest, which lets us construct a real Worker
// from outside package social. Without that constructor, Go's type
// checker would refuse to pass our stubs across the package boundary
// because worker.go's interfaces (workerRepoIface, instagramIface,
// storageIface) are unexported.
//
// The helpers file lives in `package social` (internal) but does NOT
// declare a TestMain — it only adds three exported stub interfaces
// and one exported constructor. The default TestMain in
// repository_test.go (package social_test) provides the testPool we
// reuse here. Tests skip cleanly when testPool is nil (no Docker).

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/platform/crypto"
	"github.com/klubhub/dj/api/internal/social"
	"github.com/minio/minio-go/v7"
	"github.com/rs/zerolog"
)

// encryptToken is a small wrapper around crypto.Encrypt with the
// e2e AES key. Kept here because the same-named helper in
// worker_test.go (package social) is unexported.
func encryptToken(t *testing.T, plaintext string) string {
	t.Helper()
	enc, err := crypto.Encrypt([]byte(e2eCryptoKey), plaintext)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	return enc
}

// e2eCryptoKey is the AES-256 key used both for the encrypted
// access_token on the social_accounts row and the Worker's
// crypto.Decrypt call. Must match the key the Worker is given.
const e2eCryptoKey = "test-key-32-bytes-for-aes256!!XX" // 32 bytes

// e2eClearSocial wipes both scheduled_posts and social_accounts so
// each subtest starts from a known empty state.
func e2eClearSocial(t *testing.T) {
	t.Helper()
	if testPool == nil {
		t.Skip("e2e: postgres pool unavailable (no docker / TestMain startup failed)")
	}
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "DELETE FROM scheduled_posts"); err != nil {
		t.Fatalf("clear scheduled_posts: %v", err)
	}
	if _, err := testPool.Exec(ctx, "DELETE FROM social_accounts"); err != nil {
		t.Fatalf("clear social_accounts: %v", err)
	}
}

// e2eValidJPEGBody builds a minimal-but-decodable 320x320 JPEG used
// as the upload payload.
func e2eValidJPEGBody(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 320, 320))
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return b.Bytes()
}

// presignCaptureStorage is a stub for the Worker that records every
// PresignedGetObject call so the test can verify the Worker asked
// the storage layer to sign exactly the persisted key (E2).
type presignCaptureStorage struct {
	bucket   string
	requests []presignRequest
	url      string
}

type presignRequest struct {
	bucket string
	key    string
	expiry time.Duration
}

func (s *presignCaptureStorage) Bucket() string { return s.bucket }
func (s *presignCaptureStorage) PresignedGetObject(_ context.Context, bucket, key string, expiry time.Duration, _ map[string]string) (string, error) {
	s.requests = append(s.requests, presignRequest{bucket: bucket, key: key, expiry: expiry})
	if s.url == "" {
		s.url = "https://garage.example/social/signed/" + key
	}
	return s.url, nil
}

// capturingPutStorage records PutObject invocations during upload
// (matches social.serviceStorageIface).
type capturingPutStorage struct {
	bucket   string
	putCalls int
	putKey   string
	putData  []byte
}

func (s *capturingPutStorage) Bucket() string { return s.bucket }
func (s *capturingPutStorage) PutObject(_ context.Context, _, key string, r io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	s.putCalls++
	s.putKey = key
	if r != nil {
		buf, _ := io.ReadAll(r)
		s.putData = buf
	}
	return minio.UploadInfo{}, nil
}

// scriptedInstagram is an Instagram stub matching the parent spec's
// expected fake responses. Captures CreateContainer / PublishContainer
// inputs so we can assert the presigned URL the Worker computed was
// forwarded end-to-end.
type scriptedInstagram struct {
	createContainerFn func(ctx context.Context, igUserID, accessToken, imageURL, caption string, postType social.PostType) (string, error)
	publishFn         func(ctx context.Context, igUserID, accessToken, containerID string) (string, error)

	createCalls []igCreateCall
	pubCalls    []igPublishCall
}

type igCreateCall struct {
	igUserID, accessToken, imageURL, caption string
	postType                                 social.PostType
}

type igPublishCall struct {
	igUserID, accessToken, containerID string
}

func (i *scriptedInstagram) CreateContainer(ctx context.Context, igUserID, accessToken, imageURL, caption string, postType social.PostType) (string, error) {
	i.createCalls = append(i.createCalls, igCreateCall{
		igUserID:    igUserID,
		accessToken: accessToken,
		imageURL:    imageURL,
		caption:     caption,
		postType:    postType,
	})
	if i.createContainerFn != nil {
		return i.createContainerFn(ctx, igUserID, accessToken, imageURL, caption, postType)
	}
	return "container-1", nil
}

func (i *scriptedInstagram) PublishContainer(ctx context.Context, igUserID, accessToken, containerID string) (string, error) {
	i.pubCalls = append(i.pubCalls, igPublishCall{
		igUserID:    igUserID,
		accessToken: accessToken,
		containerID: containerID,
	})
	if i.publishFn != nil {
		return i.publishFn(ctx, igUserID, accessToken, containerID)
	}
	return "post-1", nil
}

func (i *scriptedInstagram) CheckContainerStatus(_ context.Context, _, _ string) (social.ContainerStatus, error) {
	return social.ContainerStatusFinished, nil
}

func (i *scriptedInstagram) RefreshToken(_ context.Context, _ string) (string, time.Time, error) {
	return "refreshed-token", time.Now().Add(60 * 24 * time.Hour), nil
}

// uploadFlowFixture wires the full pipeline against the real
// Postgres pool: Repository → Service (with stub storage) → Worker
// (with stub Instagram and a separate presign-capturing storage).
type uploadFlowFixture struct {
	repo     *social.Repository
	service  *social.Service
	uploader *capturingPutStorage
	worker   *social.Worker
	ig       *scriptedInstagram
	storage  *presignCaptureStorage
	postID   uuid.UUID
	postKey  string
}

func newUploadFlowFixture(t *testing.T, igFactory func() *scriptedInstagram) *uploadFlowFixture {
	t.Helper()
	e2eClearSocial(t)
	ctx := context.Background()

	repo := social.NewRepository(testPool)

	acc, err := repo.UpsertAccount(ctx, social.SocialAccount{
		Platform:    "instagram",
		AccountName: "e2e-test-user",
		IgUserID:    "ig_e2e_user",
		AccessToken: encryptToken(t, "plain-e2e-token"),
		Status:      "connected",
	})
	if err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}

	// Pre-create the scheduled post 1 minute in the past so
	// ListDuePosts picks it up immediately.
	post, err := repo.CreatePost(ctx, social.ScheduledPost{
		AccountID:      acc.ID,
		Status:         social.PostStatusScheduled,
		PostType:       social.PostTypeFeed,
		Caption:        "hello e2e",
		ImageStorageKey: "placeholder.jpg",
		ScheduledAtUTC: time.Now().Add(-1 * time.Minute).UTC(),
		TimezoneName:   "UTC",
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	uploader := &capturingPutStorage{bucket: "klab-bucket"}
	svc := social.NewService(repo, social.ServiceConfig{
		InstagramAppID:       "e2e-app",
		InstagramAppSecret:   "e2e-secret",
		InstagramRedirectURI: "https://e2e.test/callback",
		TokenEncryptionKey:   []byte(e2eCryptoKey),
	})
	svc.SetStorage(uploader)

	// Upload step — this is what E1 asserts equivalence on.
	upload, err := svc.UploadPostImage(ctx, post.ID, e2eValidJPEGBody(t), "image/jpeg")
	if err != nil {
		t.Fatalf("UploadPostImage: %v", err)
	}

	// Confirm DB row got the same key the API returned (E1).
	persisted, err := repo.GetPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if persisted.ImageStorageKey != upload.Path {
		t.Fatalf("E1 image_storage_key mismatch: persisted=%q upload=%q",
			persisted.ImageStorageKey, upload.Path)
	}

	// Worker side: stub Instagram + presign-capturing storage.
	// Same bucket name as the uploader so storage.Bucket() returns
	// what the upload stub used.
	storage := &presignCaptureStorage{
		bucket: uploader.Bucket(),
		url:    "https://garage.example/social/signed/" + upload.Path,
	}
	ig := igFactory()

	w := social.NewWorkerForTest(
		repo,
		ig,
		storage,
		[]byte(e2eCryptoKey),
		zerolog.Nop(),
	)

	return &uploadFlowFixture{
		repo:     repo,
		service:  svc,
		uploader: uploader,
		worker:   w,
		ig:       ig,
		storage:  storage,
		postID:   post.ID,
		postKey:  upload.Path,
	}
}

// runWorkerTickOnce invokes the worker's tick once and returns any
// error. Mirrors a single 60s ticker iteration.
func runWorkerTickOnce(t *testing.T, w *social.Worker) {
	t.Helper()
	// The Worker's tick method is unexported; reach in via a small
	// helper exported under the e2e build tag.
	if err := social.TickWorkerForTest(w, context.Background()); err != nil {
		t.Fatalf("worker.tick: %v", err)
	}
}

// stringContains avoids a `strings` import for the two-call site.
func stringContainsPtr(s *string, sub string) bool {
	if s == nil {
		return false
	}
	for i := 0; i+len(sub) <= len(*s); i++ {
		if (*s)[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestE2E_UploadScheduleTick_PublishesWithKeyEquivalenceAndPresign pins
// the happy-path end-to-end flow described in the parent spec:
//
//  1. Upload a JPEG through the real Service against the real
//     Repository; assert the key returned by the Service equals the
//     value persisted on image_storage_key (E1 key equivalence).
//  2. Drive Worker.tick once; assert the storage layer was asked
//     to presign the EXACT persisted key (E2 presign attempt).
//  3. Assert Instagram CreateContainer received that presigned URL,
//     and PublishContainer returned "post-1".
//  4. Assert the scheduled_posts row is in `published` state with
//     container_id == "container-1", no last_error.
func TestE2E_UploadScheduleTick_PublishesWithKeyEquivalenceAndPresign(t *testing.T) {
	fix := newUploadFlowFixture(t, func() *scriptedInstagram {
		return &scriptedInstagram{} // default canned: container-1 / post-1
	})

	// Pre-tick: presign stub has zero requests.
	if got := len(fix.storage.requests); got != 0 {
		t.Fatalf("expected 0 presign calls before tick, got %d", got)
	}

	// Drive one worker cycle.
	runWorkerTickOnce(t, fix.worker)

	// Storage PresignedGetObject must have been asked exactly once
	// with the persisted key.
	if got := len(fix.storage.requests); got != 1 {
		t.Fatalf("expected exactly 1 presign call after tick, got %d", got)
	}
	req := fix.storage.requests[0]
	if req.bucket != fix.storage.bucket {
		t.Errorf("presign bucket=%q want %q", req.bucket, fix.storage.bucket)
	}
	if req.key != fix.postKey {
		t.Errorf("presign key=%q want persisted key %q", req.key, fix.postKey)
	}
	// Spec: 30-minute expiry (worker.go hard-codes 30*time.Minute).
	if req.expiry != 30*time.Minute {
		t.Errorf("presign expiry=%v want 30m", req.expiry)
	}

	// Instagram CreateContainer must have received the presigned URL.
	if len(fix.ig.createCalls) != 1 {
		t.Fatalf("expected 1 CreateContainer call, got %d", len(fix.ig.createCalls))
	}
	gotCreate := fix.ig.createCalls[0]
	if gotCreate.imageURL != fix.storage.url {
		t.Errorf("CreateContainer imageURL=%q want presigned URL %q",
			gotCreate.imageURL, fix.storage.url)
	}
	if gotCreate.caption != "hello e2e" {
		t.Errorf("CreateContainer caption=%q want %q", gotCreate.caption, "hello e2e")
	}

	// PublishContainer called once with container-1.
	if len(fix.ig.pubCalls) != 1 {
		t.Fatalf("expected 1 PublishContainer call, got %d", len(fix.ig.pubCalls))
	}
	if fix.ig.pubCalls[0].containerID != "container-1" {
		t.Errorf("PublishContainer containerID=%q want container-1", fix.ig.pubCalls[0].containerID)
	}

	// Persisted state: published, container_id set, no last_error.
	persisted, err := fix.repo.GetPost(context.Background(), fix.postID)
	if err != nil {
		t.Fatalf("GetPost after tick: %v", err)
	}
	if persisted.Status != social.PostStatusPublished {
		t.Errorf("status=%q want published", persisted.Status)
	}
	if persisted.ContainerID == nil || *persisted.ContainerID != "container-1" {
		got := "<nil>"
		if persisted.ContainerID != nil {
			got = *persisted.ContainerID
		}
		t.Errorf("container_id=%s want container-1", got)
	}
	if persisted.LastError != nil && *persisted.LastError != "" {
		t.Errorf("last_error should be empty on success, got %q", *persisted.LastError)
	}
}

// TestE2E_UploadScheduleTick_FailurePersistsSanitizedReason pins the
// E2 sanitization regression: when Instagram CreateContainer returns
// a credential-bearing transport error, the worker MUST persist a
// sanitized reason on scheduled_posts.last_error (no leaked
// access_token= or client_id= substrings).
//
// This is the real-Repository counterpart to
// TestWorker_FailPostSanitizesReasonBeforePersisting (which uses a
// mock repo).
func TestE2E_UploadScheduleTick_FailurePersistsSanitizedReason(t *testing.T) {
	const (
		leakedSecret = "SECRET_E2E_LEAK_TOKEN_XYZ"
		leakedAppID  = "SECRET_E2E_LEAK_APP_ID"
	)
	fix := newUploadFlowFixture(t, func() *scriptedInstagram {
		return &scriptedInstagram{
			createContainerFn: func(_ context.Context, _, _, _, _ string, _ social.PostType) (string, error) {
				return "", errors.New(
					`create container: status 400: error=invalid&access_token=` + leakedSecret +
						`&client_id=` + leakedAppID,
				)
			},
		}
	})

	runWorkerTickOnce(t, fix.worker)

	persisted, err := fix.repo.GetPost(context.Background(), fix.postID)
	if err != nil {
		t.Fatalf("GetPost after failed tick: %v", err)
	}
	if persisted.Status != social.PostStatusFailed {
		t.Errorf("status=%q want failed", persisted.Status)
	}
	if persisted.LastError == nil || *persisted.LastError == "" {
		t.Fatalf("expected last_error to be set after failed tick, got nil/empty")
	}
	for _, sentinel := range []string{leakedSecret, leakedAppID} {
		if stringContainsPtr(persisted.LastError, sentinel) {
			t.Errorf("persisted last_error leaked credential %q: %s", sentinel, *persisted.LastError)
		}
	}
	// SetNextRetry must have been called: next_retry_at in the future.
	if persisted.NextRetryAt == nil {
		t.Errorf("expected next_retry_at to be set after failure")
	} else if persisted.NextRetryAt.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("next_retry_at is in the past: %v", *persisted.NextRetryAt)
	}
}
