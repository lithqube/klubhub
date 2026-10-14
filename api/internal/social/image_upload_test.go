package social_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/social"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

type uploadRepo struct {
	*mockRepo
	updateErr error
	updates   int
}

func (r *uploadRepo) UpdatePostImage(ctx context.Context, id uuid.UUID, key string) error {
	r.updates++
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.mockRepo.UpdatePostImage(ctx, id, key)
}

type uploadStorage struct {
	calls                    int
	key, bucket, contentType string
	data                     []byte
	size                     int64
	err                      error
	beforePut                func()
}

func (s *uploadStorage) Bucket() string { return "test-bucket" }
func (s *uploadStorage) PutObject(_ context.Context, bucket, key string, r io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	s.calls++
	s.key, s.bucket, s.contentType, s.size = key, bucket, opts.ContentType, size
	s.data, _ = io.ReadAll(r)
	if s.beforePut != nil {
		s.beforePut()
	}
	return minio.UploadInfo{}, s.err
}
func imageUploadFixture() (*social.Service, *uploadRepo, *uploadStorage, uuid.UUID) {
	repo := &uploadRepo{mockRepo: newMockRepo()}
	id := uuid.New()
	repo.posts[id] = &social.ScheduledPost{ID: id, Status: social.PostStatusScheduled, PostType: social.PostTypeFeed, ImageStorageKey: "old-key"}
	storage := &uploadStorage{}
	svc := social.NewService(repo, social.ServiceConfig{})
	svc.SetStorage(storage)
	return svc, repo, storage, id
}
func validUploadImage(t *testing.T, format string) []byte {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 320, 320))
	if format == "png" {
		require.NoError(t, png.Encode(&b, img))
	} else {
		require.NoError(t, jpeg.Encode(&b, img, nil))
	}
	return b.Bytes()
}
func uploadRequest(t *testing.T, endpoint string, data []byte, mime string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="image_file"; filename="../../unsafe.jpg"`)
	header.Set("Content-Type", mime)
	part, err := w.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, endpoint, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestUploadPostImageService_DetectsBytesAndGeneratesSafeKeys(t *testing.T) {
	for _, format := range []string{"png", "jpg"} {
		t.Run(format, func(t *testing.T) {
			svc, repo, storage, id := imageUploadFixture()
			data := validUploadImage(t, format)
			// The caller-controlled MIME and filename must not determine storage key/type.
			result, err := svc.UploadPostImage(context.Background(), id, data, "text/html")
			require.NoError(t, err)
			require.Equal(t, data, storage.data)
			require.Equal(t, int64(len(data)), storage.size)
			require.Equal(t, "test-bucket", storage.bucket)
			wantMime := "image/jpeg"
			if format == "png" {
				wantMime = "image/png"
			}
			require.Equal(t, wantMime, storage.contentType)
			require.True(t, strings.HasPrefix(result.Path, "social/"+id.String()+"/"))
			require.Equal(t, "."+format, path.Ext(result.Path))
			_, err = uuid.Parse(strings.TrimSuffix(path.Base(result.Path), "."+format))
			require.NoError(t, err)
			require.Equal(t, result.Path, repo.posts[id].ImageStorageKey)
			second, err := svc.UploadPostImage(context.Background(), id, data, "image/jpeg")
			require.NoError(t, err)
			require.NotEqual(t, result.Path, second.Path)
		})
	}
}

func TestUploadPostImageService_RejectsValidHeaderTruncatedBody(t *testing.T) {
	svc, repo, storage, id := imageUploadFixture()
	data := validUploadImage(t, "png")[:33] // signature and complete IHDR, but no IDAT/IEND
	_, _, err := image.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err, "fixture must pass header-only validation")
	_, err = svc.UploadPostImage(context.Background(), id, data, "image/png")
	require.ErrorIs(t, err, social.ErrInvalidDimensions)
	require.Zero(t, storage.calls)
	require.Zero(t, repo.updates)
}

func TestUploadPostImageService_RejectsExcessiveDimensionsBeforeDecode(t *testing.T) {
	for _, size := range [][2]uint32{{100000, 100000}, {5000, 5000}, {8193, 320}} {
		svc, repo, storage, id := imageUploadFixture()
		data := append([]byte(nil), validUploadImage(t, "png")[:33]...)
		binary.BigEndian.PutUint32(data[16:20], size[0])
		binary.BigEndian.PutUint32(data[20:24], size[1])
		binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
		_, err := svc.UploadPostImage(context.Background(), id, data, "image/png")
		require.ErrorIs(t, err, social.ErrInvalidDimensions)
		require.Contains(t, err.Error(), "decode budget", "preflight must reject before full decode of missing body")
		require.Zero(t, storage.calls)
		require.Zero(t, repo.updates)
	}
}

func TestUploadPostImageService_RejectsEmptySpoofedAndMalformedBytes(t *testing.T) {
	cases := map[string][]byte{
		"empty":               nil,
		"spoofed jpeg header": []byte("not an image"),
		"malformed png":       {0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a},
		"malformed jpeg":      {0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo, storage, id := imageUploadFixture()
			_, err := svc.UploadPostImage(context.Background(), id, data, "image/jpeg")
			require.Error(t, err)
			require.True(t, errors.Is(err, social.ErrInvalidMIME) || errors.Is(err, social.ErrInvalidDimensions), "validation error must be mapped to 422: %v", err)
			require.Zero(t, storage.calls)
			require.Zero(t, repo.updates)
			require.Equal(t, "old-key", repo.posts[id].ImageStorageKey)
		})
	}
}

func TestUploadPostImageService_StorageFailurePreventsDBUpdate(t *testing.T) {
	svc, repo, storage, id := imageUploadFixture()
	storage.err = errors.New("storage down")
	result, err := svc.UploadPostImage(context.Background(), id, validUploadImage(t, "png"), "image/png")
	require.Nil(t, result)
	require.ErrorIs(t, err, storage.err)
	require.Zero(t, repo.updates)
	require.Equal(t, "old-key", repo.posts[id].ImageStorageKey)
}
func TestUploadPostImageService_DBErrorAndDeleteRace(t *testing.T) {
	t.Run("db error", func(t *testing.T) {
		svc, repo, storage, id := imageUploadFixture()
		repo.updateErr = errors.New("database down")
		result, err := svc.UploadPostImage(context.Background(), id, validUploadImage(t, "png"), "image/png")
		require.Nil(t, result)
		require.ErrorIs(t, err, repo.updateErr)
		require.Equal(t, 1, storage.calls)
		require.Equal(t, 1, repo.updates)
		require.Equal(t, "old-key", repo.posts[id].ImageStorageKey)
	})
	t.Run("delete after lookup makes update affect zero rows", func(t *testing.T) {
		svc, repo, storage, id := imageUploadFixture()
		storage.beforePut = func() { delete(repo.posts, id) }
		result, err := svc.UploadPostImage(context.Background(), id, validUploadImage(t, "png"), "image/png")
		require.Nil(t, result)
		require.ErrorIs(t, err, social.ErrNotFound)
		require.Equal(t, 1, storage.calls)
		require.Equal(t, 1, repo.updates)
	})
	t.Run("missing post does not write storage", func(t *testing.T) {
		svc, repo, storage, id := imageUploadFixture()
		delete(repo.posts, id)
		_, err := svc.UploadPostImage(context.Background(), id, validUploadImage(t, "png"), "image/png")
		require.ErrorIs(t, err, social.ErrNotFound)
		require.Zero(t, storage.calls)
		require.Zero(t, repo.updates)
	})
}

func TestUploadPostImageHandler_MalformedDetectedImagesReturn422(t *testing.T) {
	for _, data := range [][]byte{{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, {0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}} {
		svc, repo, storage, id := imageUploadFixture()
		h := social.NewHandler(realServiceAdapter{svc: svc}, nil).Routes()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, uploadRequest(t, "/posts/"+id.String()+"/image", data, "image/jpeg"))
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
		require.Zero(t, storage.calls)
		require.Zero(t, repo.updates)
	}
}
func TestUploadPostImageHandler_BlocksNonScheduledLifecycle(t *testing.T) {
	for _, status := range []social.PostStatus{social.PostStatusPublished, social.PostStatusPublishing, social.PostStatusFailed, social.PostStatusDraft} {
		t.Run(string(status), func(t *testing.T) {
			svc, repo, storage, id := imageUploadFixture()
			repo.posts[id].Status = status
			h := social.NewHandler(realServiceAdapter{svc: svc}, nil).Routes()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, uploadRequest(t, "/posts/"+id.String()+"/image", validUploadImage(t, "png"), "image/png"))
			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			require.Zero(t, storage.calls)
			require.Zero(t, repo.updates)
		})
	}
}

func TestUploadPostImageRepository_StatusTransitionRace(t *testing.T) {
	clearSocial(t)
	repo := social.NewRepository(testPool)
	account := createTestAccount(t, repo)
	post, err := repo.CreatePost(context.Background(), social.ScheduledPost{
		AccountID: account.ID, Status: social.PostStatusScheduled, PostType: social.PostTypeFeed,
		ImageStorageKey: "old-key", TimezoneName: "UTC",
	})
	require.NoError(t, err)
	storage := &uploadStorage{beforePut: func() {
		require.NoError(t, repo.UpdatePostStatus(context.Background(), post.ID, social.PostStatusPublishing, ""))
	}}
	svc := social.NewService(repo, social.ServiceConfig{})
	svc.SetStorage(storage)
	_, err = svc.UploadPostImage(context.Background(), post.ID, validUploadImage(t, "png"), "image/png")
	require.ErrorIs(t, err, social.ErrEditBlocked)
	persisted, err := repo.GetPost(context.Background(), post.ID)
	require.NoError(t, err)
	require.Equal(t, "old-key", persisted.ImageStorageKey)
	require.Equal(t, 1, storage.calls) // raced upload leaves an orphan; cleanup is deferred.
	require.ErrorIs(t, repo.UpdatePostImage(context.Background(), uuid.New(), "key"), social.ErrNotFound)
	require.NoError(t, repo.SoftDeletePost(context.Background(), post.ID))
	require.ErrorIs(t, repo.UpdatePostImage(context.Background(), post.ID, "key"), social.ErrNotFound)
}

func TestSocialHandler_RequestCapReturns413(t *testing.T) {
	for _, endpoint := range []string{"/api/v1/social/posts", "/api/v1/social/posts/" + uuid.NewString() + "/image"} {
		t.Run(endpoint, func(t *testing.T) {
			svc := newHandlerMock()
			rec := httptest.NewRecorder()
			newTestRouter(svc).ServeHTTP(rec, uploadRequest(t, endpoint, make([]byte, 13<<20), "image/png"))
			require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
			require.Empty(t, svc.posts)
		})
	}
}
