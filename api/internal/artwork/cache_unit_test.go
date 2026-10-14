package artwork_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/artwork"
	"github.com/minio/minio-go/v7"
)

// fakeStorage implements artwork.StorageClient, recording PutObject calls
// and scripting StatObject/PresignedGetObject outcomes.
type fakeStorage struct {
	statErr        error
	presignedURL   string
	presignedErr   error
	putErr         error
	putObjectCalls []putObjectCall
}

type putObjectCall struct {
	bucket, objectName string
	size               int64
	contentType        string
}

func (f *fakeStorage) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	f.putObjectCalls = append(f.putObjectCalls, putObjectCall{bucketName, objectName, size, opts.ContentType})
	if f.putErr != nil {
		return minio.UploadInfo{}, f.putErr
	}
	return minio.UploadInfo{Bucket: bucketName, Key: objectName, Size: size}, nil
}

func (f *fakeStorage) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeStorage) StatObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (minio.ObjectInfo, error) {
	if f.statErr != nil {
		return minio.ObjectInfo{}, f.statErr
	}
	return minio.ObjectInfo{Key: objectName, Size: 1}, nil
}

func (f *fakeStorage) PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams map[string]string) (string, error) {
	if f.presignedErr != nil {
		return "", f.presignedErr
	}
	return f.presignedURL, nil
}

func TestArtworkCache_GetMissReturnsEmpty(t *testing.T) {
	store := &fakeStorage{statErr: errors.New("NoSuchKey")}
	cache := artwork.NewArtworkCache(store, "klubhub", "http://public:9000")

	if got := cache.Get(context.Background(), "Song", "Artist"); got != "" {
		t.Errorf("Get on miss = %q, want empty", got)
	}
}

func TestArtworkCache_GetHitReturnsPresignedURL(t *testing.T) {
	store := &fakeStorage{presignedURL: "https://public.example.com/signed?sig=abc"}
	cache := artwork.NewArtworkCache(store, "klubhub", "http://public:9000")

	got := cache.Get(context.Background(), "Song", "Artist")
	if got != "https://public.example.com/signed?sig=abc" {
		t.Errorf("Get on hit = %q, want the presigned URL", got)
	}
}

func TestArtworkCache_GetHitPresignFailureFallsBackToDirectURL(t *testing.T) {
	store := &fakeStorage{presignedErr: errors.New("signing failed")}
	cache := artwork.NewArtworkCache(store, "klubhub", "http://public:9000")

	got := cache.Get(context.Background(), "Song", "Artist")
	wantPrefix := "http://public:9000/klubhub/cover-art/"
	if !strings.HasPrefix(got, wantPrefix) || !strings.HasSuffix(got, ".jpg") {
		t.Errorf("Get fallback = %q, want direct URL %s<key>.jpg", got, wantPrefix)
	}
}

func TestArtworkCache_CacheKeyIsDeterministicAndNamespaced(t *testing.T) {
	ctx := context.Background()

	// Drive Put against a real local server so the upload path runs.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg-bytes"))
	}))
	defer srv.Close()

	store2 := &fakeStorage{}
	cache2 := artwork.NewArtworkCache(store2, "klubhub", "http://public:9000")
	if err := cache2.Put(ctx, "Song", "Artist", srv.URL+"/cover.jpg"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if len(store2.putObjectCalls) != 1 {
		t.Fatalf("PutObject calls = %d, want 1", len(store2.putObjectCalls))
	}
	call := store2.putObjectCalls[0]
	if call.bucket != "klubhub" {
		t.Errorf("bucket = %q, want klubhub", call.bucket)
	}
	if !strings.HasPrefix(call.objectName, "cover-art/") || !strings.HasSuffix(call.objectName, ".jpg") {
		t.Errorf("objectName = %q, want cover-art/<hash>.jpg", call.objectName)
	}
	if len(strings.TrimSuffix(strings.TrimPrefix(call.objectName, "cover-art/"), ".jpg")) != 16 {
		t.Errorf("objectName hash = %q, want 16 hex chars", call.objectName)
	}
	if call.size != int64(len("jpeg-bytes")) {
		t.Errorf("size = %d, want %d", call.size, len("jpeg-bytes"))
	}
	if call.contentType != "image/jpeg" {
		t.Errorf("contentType = %q, want image/jpeg", call.contentType)
	}

	// A different title|artist must produce a different key.
	store3 := &fakeStorage{}
	cache3 := artwork.NewArtworkCache(store3, "klubhub", "http://public:9000")
	if err := cache3.Put(ctx, "Other Song", "Artist", srv.URL+"/cover.jpg"); err != nil {
		t.Fatalf("Put (other) failed: %v", err)
	}
	if store3.putObjectCalls[0].objectName == call.objectName {
		t.Errorf("different title|artist produced the same key %q", call.objectName)
	}

	// The same title|artist must produce the same key again (deterministic).
	store4 := &fakeStorage{}
	cache4 := artwork.NewArtworkCache(store4, "klubhub", "http://public:9000")
	if err := cache4.Put(ctx, "Song", "Artist", srv.URL+"/cover.jpg"); err != nil {
		t.Fatalf("Put (repeat) failed: %v", err)
	}
	if store4.putObjectCalls[0].objectName != call.objectName {
		t.Errorf("same title|artist produced key %q, want the deterministic %q",
			store4.putObjectCalls[0].objectName, call.objectName)
	}
}

func TestArtworkCache_PutRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	store := &fakeStorage{}
	cache := artwork.NewArtworkCache(store, "klubhub", "http://public:9000")

	err := cache.Put(context.Background(), "Song", "Artist", srv.URL+"/missing.jpg")
	if err == nil {
		t.Fatal("Put with 404 provider response succeeded, want error")
	}
	if len(store.putObjectCalls) != 0 {
		t.Errorf("PutObject called despite failed download: %+v", store.putObjectCalls)
	}
}

func TestArtworkCache_PutRejectsRedirect(t *testing.T) {
	// The artwork client denies redirects, so a 302 surfaces as a non-200
	// response and Put must reject it rather than follow to the target.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer srv.Close()

	store := &fakeStorage{}
	cache := artwork.NewArtworkCache(store, "klubhub", "http://public:9000")

	err := cache.Put(context.Background(), "Song", "Artist", srv.URL+"/start")
	if err == nil {
		t.Fatal("Put followed/accepted a redirect, want error")
	}
	if len(store.putObjectCalls) != 0 {
		t.Errorf("PutObject called despite redirect: %+v", store.putObjectCalls)
	}
}

func TestArtworkCache_PutRejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// 6 MiB > 5 MiB cap.
		chunk := make([]byte, 64*1024)
		for i := 0; i < 6*1024/64; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	store := &fakeStorage{}
	cache := artwork.NewArtworkCache(store, "klubhub", "http://public:9000")

	err := cache.Put(context.Background(), "Song", "Artist", srv.URL+"/huge.jpg")
	if err == nil {
		t.Fatal("Put accepted a >5MiB body, want cap error")
	}
	if len(store.putObjectCalls) != 0 {
		t.Errorf("PutObject called despite oversized body: %+v", store.putObjectCalls)
	}
}

func TestArtworkCache_PutPropagatesStorageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("jpeg-bytes"))
	}))
	defer srv.Close()

	store := &fakeStorage{putErr: errors.New("garage unreachable")}
	cache := artwork.NewArtworkCache(store, "klubhub", "http://public:9000")

	err := cache.Put(context.Background(), "Song", "Artist", srv.URL+"/cover.jpg")
	if err == nil || !strings.Contains(err.Error(), "garage unreachable") {
		t.Fatalf("Put err = %v, want the storage error wrapped", err)
	}
}

func TestArtworkCache_DefaultPublicEndpoint(t *testing.T) {
	store := &fakeStorage{presignedErr: errors.New("no signing")}
	cache := artwork.NewArtworkCache(store, "klubhub", "")

	got := cache.Get(context.Background(), "Song", "Artist")
	if !strings.HasPrefix(got, "http://localhost:9000/klubhub/cover-art/") {
		t.Errorf("Get with empty public endpoint = %q, want the localhost:9000 default", got)
	}
}
