package social_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/social"
)

// contractService is a fake social.serviceIface used by contract tests.
//
// It returns known shapes so the test can assert exact JSON keys and
// envelope shape. Method signatures mirror the unexported serviceIface
// (defined in handler.go) so the type satisfies the interface
// structurally. Note: serviceIface is intentionally unexported; do not
// rename or export it from this test file (C.1 may want to).
type contractService struct {
	account *social.SocialAccount
	posts   map[uuid.UUID]*social.ScheduledPost
}

func newContractService() *contractService {
	return &contractService{posts: make(map[uuid.UUID]*social.ScheduledPost)}
}

func (s *contractService) IssueOAuthState(_ context.Context) (string, string, string, error) {
	return "https://api.instagram.com/oauth/authorize?state=x", "test-state", "test-bind", nil
}

func (s *contractService) HandleOAuthCallback(_ context.Context, _, _, _ string) (*social.SocialAccount, error) {
	return s.account, nil
}

func (s *contractService) GetAccount(_ context.Context) (*social.SocialAccount, error) {
	return s.account, nil
}

func (s *contractService) DisconnectAccount(_ context.Context, _ uuid.UUID) error { return nil }

func (s *contractService) ListPosts(_ context.Context) ([]social.ScheduledPost, error) {
	out := make([]social.ScheduledPost, 0, len(s.posts))
	for _, p := range s.posts {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, nil
}

func (s *contractService) GetPost(_ context.Context, id uuid.UUID) (*social.ScheduledPost, error) {
	p, ok := s.posts[id]
	if !ok {
		return nil, social.ErrNotFound
	}
	return p, nil
}

func (s *contractService) SchedulePost(_ context.Context, req social.CreatePostRequest) (*social.ScheduledPost, error) {
	id := uuid.New()
	accountID := req.AccountID
	if accountID == uuid.Nil {
		accountID = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	}
	now := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	p := &social.ScheduledPost{
		ID:             id,
		AccountID:      accountID,
		Status:         social.PostStatusScheduled,
		PostType:       req.PostType,
		Caption:        req.Caption,
		ImageStorageKey: req.ImageStorageKey,
		ScheduledAtUTC: now,
		TimezoneName:   req.TimezoneName,
		RetryCount:     0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.posts[id] = p
	return p, nil
}

func (s *contractService) EditPost(_ context.Context, id uuid.UUID, req social.EditPostRequest) (*social.ScheduledPost, error) {
	p, ok := s.posts[id]
	if !ok {
		return nil, social.ErrNotFound
	}
	p.Caption = req.Caption
	return p, nil
}

func (s *contractService) SoftDeletePost(_ context.Context, id uuid.UUID) error {
	delete(s.posts, id)
	return nil
}

func (s *contractService) RetryPost(_ context.Context, id uuid.UUID) error {
	p, ok := s.posts[id]
	if !ok {
		return social.ErrNotFound
	}
	p.Status = social.PostStatusScheduled
	now := time.Now().UTC()
	p.NextRetryAt = &now
	return nil
}

func (s *contractService) ValidateImage(_ []byte, _ social.PostType) error { return nil }

// UploadPostImage simulates the real upload pipeline: validates via
// mimetype detection, generates a deterministic Garage-style object key
// for the test post, and updates the post's ImageStorageKey in the
// in-memory store. The contract test asserts the path that this returns.
func (s *contractService) UploadPostImage(_ context.Context, id uuid.UUID, data []byte, _ string) (*social.PostImageResult, error) {
	p, ok := s.posts[id]
	if !ok {
		return nil, social.ErrNotFound
	}
	// Derive a key using a UUID placeholder so the assertion can check
	// the exact prefix path without being sensitive to random UUIDs.
	// The contract test asserts the prefix only.
	path := "social/" + id.String() + "/test-uploaded.jpg"
	p.ImageStorageKey = path
	p.UpdatedAt = time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	_ = data // bytes are validated inside the service in production
	return &social.PostImageResult{Path: path}, nil
}

// postContractKeys is the set of keys the API contract guarantees on
// every ScheduledPost in a list / get / create / retry response.
//
// The contract requires camelCase keys for every multi-word field,
// regardless of the underlying struct tag. Tests assert EXACT membership
// — extra keys (legacy snake_case) or missing keys are both failures.
var postContractKeys = []string{
	"id",
	"accountId",
	"status",
	"postType",
	"caption",
	"imageStorageKey",
	"scheduledAtUtc",
	"timezoneName",
	"retryCount",
	"nextRetryAt",
	"lastError",
	"containerId",
	"createdAt",
	"updatedAt",
}

func newContractTestRouter(svc *contractService) http.Handler {
	h := social.NewHandler(svc, nil)
	r := chi.NewRouter()
	r.Mount("/api/v1/social", h.Routes())
	return r
}

func TestListPostsContract_UsesDataEnvelope(t *testing.T) {
	postID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	accountID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	now := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	nextRetry := now.Add(5 * time.Minute)
	lastErr := "transient"
	containerID := "ig-container-abc"

	svc := newContractService()
	svc.posts[postID] = &social.ScheduledPost{
		ID:             postID,
		AccountID:      accountID,
		Status:         social.PostStatusFailed,
		PostType:       social.PostTypeFeed,
		Caption:        "Friday night",
		ImageStorageKey: "social/feed.jpg",
		ScheduledAtUTC: now,
		TimezoneName:   "Europe/Berlin",
		RetryCount:     3,
		NextRetryAt:    &nextRetry,
		LastError:      &lastErr,
		ContainerID:    &containerID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	rec := httptest.NewRecorder()
	newContractTestRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/posts", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "data", "response must wrap posts in a data envelope")

	var posts []map[string]any
	require.NoError(t, json.Unmarshal(body["data"], &posts), "data must be a JSON array")
	require.Len(t, posts, 1)

	if got, want := len(posts[0]), len(postContractKeys); got != want {
		t.Errorf("post keys count = %d, want %d; keys = %v", got, want, sortedKeys(posts[0]))
	}
	for _, k := range postContractKeys {
		if _, ok := posts[0][k]; !ok {
			t.Errorf("missing camelCase key %q in post; keys = %v", k, sortedKeys(posts[0]))
		}
	}
	// Explicit snake_case negative assertions — the contract forbids legacy keys.
	for _, k := range []string{"account_id", "post_type", "image_minio_path", "image_storage_key", "scheduled_at_utc", "created_at", "updated_at"} {
		if _, ok := posts[0][k]; ok {
			t.Errorf("post must not expose legacy snake_case key %q; keys = %v", k, sortedKeys(posts[0]))
		}
	}
	if posts[0]["id"] != postID.String() {
		t.Errorf("id = %v, want %s", posts[0]["id"], postID)
	}
	if posts[0]["accountId"] != accountID.String() {
		t.Errorf("accountId = %v, want %s", posts[0]["accountId"], accountID)
	}
	if posts[0]["status"] != string(social.PostStatusFailed) {
		t.Errorf("status = %v, want %q", posts[0]["status"], social.PostStatusFailed)
	}
	if _, ok := posts[0]["scheduledAtUtc"].(string); !ok {
		t.Errorf("scheduledAtUtc type = %T, want JSON string", posts[0]["scheduledAtUtc"])
	}
}

func TestCreatePostContract_UsesDataEnvelope(t *testing.T) {
	svc := newContractService()
	router := newContractTestRouter(svc)

	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	require.NoError(t, mw.WriteField("post_type", "feed"))
	require.NoError(t, mw.WriteField("scheduled_at", "2026-12-01T12:00:00Z"))
	require.NoError(t, mw.WriteField("timezone_name", "Europe/Berlin"))
	require.NoError(t, mw.WriteField("caption", "test"))
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts", &bodyBuf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body = %s", rec.Body.String())

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "data", "POST /posts response must wrap the post in a data envelope")

	var post map[string]any
	require.NoError(t, json.Unmarshal(body["data"], &post))
	if got, want := len(post), len(postContractKeys); got != want {
		t.Errorf("post keys count = %d, want %d; keys = %v", got, want, sortedKeys(post))
	}
	for _, k := range postContractKeys {
		if _, ok := post[k]; !ok {
			t.Errorf("missing camelCase key %q in created post; keys = %v", k, sortedKeys(post))
		}
	}
	if post["caption"] != "test" {
		t.Errorf("caption = %v, want \"test\"", post["caption"])
	}
	if post["postType"] != string(social.PostTypeFeed) {
		t.Errorf("postType = %v, want \"feed\"", post["postType"])
	}
	if post["timezoneName"] != "Europe/Berlin" {
		t.Errorf("timezoneName = %v, want \"Europe/Berlin\"", post["timezoneName"])
	}
}

func TestGetAccountContract_ReturnsDataEnvelope(t *testing.T) {
	svc := newContractService()
	// Case A: no account connected → {data: null}
	rec := httptest.NewRecorder()
	newContractTestRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/accounts", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "data", "GET /accounts must wrap result in a data envelope")
	if string(body["data"]) != "null" {
		t.Errorf("data = %s, want null when no account connected", string(body["data"]))
	}

	// Case B: account connected → {data: {…}}
	svc.account = &social.SocialAccount{
		ID:        uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Platform:  "instagram",
		Status:    "connected",
		CreatedAt: time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC),
	}
	rec = httptest.NewRecorder()
	newContractTestRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/social/accounts", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "data")
	var acc map[string]any
	require.NoError(t, json.Unmarshal(body["data"], &acc))
	if acc["platform"] != "instagram" {
		t.Errorf("platform = %v, want \"instagram\"", acc["platform"])
	}
	if acc["status"] != "connected" {
		t.Errorf("status = %v, want \"connected\"", acc["status"])
	}
}

func TestRetryPostContract_ReturnsDataEnvelope(t *testing.T) {
	postID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	now := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	svc := newContractService()
	svc.posts[postID] = &social.ScheduledPost{
		ID:             postID,
		AccountID:      uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Status:         social.PostStatusFailed,
		PostType:       social.PostTypeFeed,
		ScheduledAtUTC: now,
		TimezoneName:   "Europe/Berlin",
		RetryCount:     3,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	rec := httptest.NewRecorder()
	newContractTestRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/social/posts/"+postID.String()+"/retry", nil))
	require.Equal(t, http.StatusOK, rec.Code, "body = %s", rec.Body.String())

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "data", "POST /posts/{id}/retry must wrap the post in a data envelope")

	var post map[string]any
	require.NoError(t, json.Unmarshal(body["data"], &post), "data must be a post object")
	for _, k := range postContractKeys {
		if _, ok := post[k]; !ok {
			t.Errorf("missing camelCase key %q in retried post; keys = %v", k, sortedKeys(post))
		}
	}
	if post["status"] != string(social.PostStatusScheduled) {
		t.Errorf("status = %v, want \"scheduled\" after retry", post["status"])
	}
	if post["id"] != postID.String() {
		t.Errorf("id = %v, want %s", post["id"], postID)
	}
}

// TestUploadPostImageContract_ReturnsDataPathEnvelope asserts the
// image-upload route returns 200 + {data: {path: "<garage-object-key>"}}
// after the C.1 implementation landed. The handler parses the
// multipart form, delegates to svc.UploadPostImage (which validates,
// uploads to Garage, and updates scheduled_posts.image_storage_key),
// and wraps the resulting PostImageResult in the standard {data: …}
// envelope.
//
// Note on test stub behaviour: the contractService.UploadPostImage mock
// returns a deterministic path of the form
// `social/<post-id>/test-uploaded.jpg` so this assertion is stable
// without depending on UUID generation. In production the path uses a
// real UUID; the prefix (`social/<post-id>/`) is what the worker reads
// from the DB column to build the Instagram container URL — see
// worker.go's PresignedGetObject call.
//
// Edge cases NOT asserted today (still in the I5 backlog):
//   - Wrong content-type → 422 (ErrInvalidMIME).
//   - Oversize payload → 413 from MaxBytesReader / 422 from
//     ValidateImage's size check.
//   - Path-traversal filename → server-generated UUID key (no
//     trust in client-supplied filenames; see service.go comment).
//
// Follow-up C.2 will likely add these as separate tests once the
// upload race with the worker (worker reads "" from a too-fast row
// update) is documented. C.1 deliberately does not fix that race.
func TestUploadPostImageContract_ReturnsDataPathEnvelope(t *testing.T) {
	postID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	now := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	svc := newContractService()
	svc.posts[postID] = &social.ScheduledPost{
		ID:             postID,
		AccountID:      uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Status:         social.PostStatusDraft,
		PostType:       social.PostTypeFeed,
		ScheduledAtUTC: now,
		TimezoneName:   "Europe/Berlin",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Small in-memory JPEG (1x1 pixel). ValidateImage in the contract
	// mock is a no-op, but real validation runs through the same
	// mimetype detector — using a real JPEG keeps the test honest
	// about what the service receives.
	var imgBuf bytes.Buffer
	require.NoError(t, jpeg.Encode(&imgBuf, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil))

	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	fw, err := mw.CreateFormFile("image_file", "tiny.jpg")
	require.NoError(t, err)
	_, err = fw.Write(imgBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts/"+postID.String()+"/image", &bodyBuf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	newContractTestRouter(svc).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"expected 200 from image-upload handler after C.1 lands; got %d body=%s",
		rec.Code, rec.Body.String())

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "data",
		"POST /posts/{id}/image must wrap the result in a data envelope")

	var data map[string]any
	require.NoError(t, json.Unmarshal(body["data"], &data),
		"data envelope must be a JSON object")
	require.Contains(t, data, "path", "data envelope must carry the path key")

	path, ok := data["path"].(string)
	require.True(t, ok, "path must be a JSON string, got %T", data["path"])
	require.True(t, strings.HasPrefix(path, "social/"+postID.String()+"/"),
		"path must be a server-generated Garage object key under social/<post-id>/; got %q", path)

	// Negative: the legacy bare-path shape must not leak — the envelope
	// is {data: {path}} and {data: null} is not acceptable for a
	// successful upload.
	if string(body["data"]) == "null" {
		t.Errorf("data envelope must not be null on successful upload")
	}
}

func TestCreatePostContract_RejectsLegacyImageFile(t *testing.T) {
	svc := newContractService()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("post_type", "feed"))
	require.NoError(t, mw.WriteField("scheduled_at", "2026-12-01T12:00"))
	require.NoError(t, mw.WriteField("timezone_name", "UTC"))
	fw, err := mw.CreateFormFile("image_file", "image.jpg")
	require.NoError(t, err)
	_, err = fw.Write([]byte("image bytes"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	newContractTestRouter(svc).ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "/posts/{id}/image")
	require.Empty(t, svc.posts, "rejected legacy upload must not create a post")
}

func TestUploadPostImageContract_MissingImageFileReturns400(t *testing.T) {
	svc := newContractService()
	postID := uuid.New()
	svc.posts[postID] = &social.ScheduledPost{ID: postID, ImageStorageKey: "original.jpg"}
	for _, field := range []string{"caption", "wrong_file"} {
		t.Run(field, func(t *testing.T) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			if field == "caption" {
				require.NoError(t, mw.WriteField(field, "no image"))
			} else {
				fw, err := mw.CreateFormFile(field, "image.jpg")
				require.NoError(t, err)
				_, err = fw.Write([]byte("image"))
				require.NoError(t, err)
			}
			require.NoError(t, mw.Close())
			req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts/"+postID.String()+"/image", &buf)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			rec := httptest.NewRecorder()
			newContractTestRouter(svc).ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "image_file field is required", body["error"])
			require.NotContains(t, body, "data")
			require.Equal(t, "original.jpg", svc.posts[postID].ImageStorageKey)
		})
	}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
