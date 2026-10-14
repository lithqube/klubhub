package tracklist

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// ── Scriptable stubs ─────────────────────────────────────────────────────────

type uploadRepoStub struct {
	mu         sync.Mutex
	created    *Tracklist
	tracks     []Track
	createErr  error
	updateErr  error
	updatedReq UpdateTrackRequest
	getTracks  []Track
	getErr     error
}

func (r *uploadRepoStub) Create(_ context.Context, tl *Tracklist, tracks []Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	r.created = tl
	r.tracks = tracks
	return nil
}
func (r *uploadRepoStub) Get(_ context.Context, id uuid.UUID) (*Tracklist, []Track, error) {
	return &Tracklist{ID: id}, r.getTracks, r.getErr
}
func (r *uploadRepoStub) List(context.Context) ([]Tracklist, error) { return nil, nil }
func (r *uploadRepoStub) UpdateTrack(_ context.Context, _, _ uuid.UUID, req UpdateTrackRequest) error {
	r.updatedReq = req
	return r.updateErr
}
func (r *uploadRepoStub) UpdateTitle(context.Context, uuid.UUID, string) error { return nil }
func (r *uploadRepoStub) LinkedGigs(context.Context, uuid.UUID) ([]LinkedGig, error) {
	return nil, nil
}
func (r *uploadRepoStub) AddTrack(_ context.Context, _ uuid.UUID, req CreateTrackRequest) (*Track, error) {
	return &Track{Title: req.Title, Artist: req.Artist}, nil
}
func (r *uploadRepoStub) ReorderTracks(context.Context, uuid.UUID, []uuid.UUID) error {
	return nil
}
func (r *uploadRepoStub) SoftDelete(context.Context, uuid.UUID) error { return nil }
func (r *uploadRepoStub) UpdateArtworkStatus(context.Context, uuid.UUID, string, string, string) error {
	return nil
}
func (r *uploadRepoStub) SaveManualArtwork(_ context.Context, trackID uuid.UUID, artworkURL string) error {
	return nil
}

type uploadStorageStub struct {
	putErr       error
	putKey       string
	putSize      int64
	presignedURL string
	presignErr   error
}

func (s *uploadStorageStub) PutObject(_ context.Context, _, key string, _ io.Reader, size int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	s.putKey = key
	s.putSize = size
	if s.putErr != nil {
		return minio.UploadInfo{}, s.putErr
	}
	return minio.UploadInfo{}, nil
}
func (s *uploadStorageStub) PresignedGetObject(_ context.Context, _, key string, _ time.Duration, _ map[string]string) (string, error) {
	if s.presignErr != nil {
		return "", s.presignErr
	}
	return s.presignedURL, nil
}

type artworkFetchStub struct {
	done chan struct{}
	once sync.Once
}

func newArtworkFetchStub() *artworkFetchStub { return &artworkFetchStub{done: make(chan struct{})} }
func (a *artworkFetchStub) FetchAll(context.Context, []Track) {
	a.once.Do(func() { close(a.done) })
}

func sampleTSV(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/rekordbox_sample.txt")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	return raw
}

// ── Upload ───────────────────────────────────────────────────────────────────

func TestUpload_RejectsOversizedFile(t *testing.T) {
	repo := &uploadRepoStub{}
	store := &uploadStorageStub{}
	svc := NewService(repo, store, nil, ServiceConfig{StorageBucket: "klubhub", MaxUploadBytes: 10})

	_, _, _, err := svc.Upload(context.Background(), "big.txt", []byte("0123456789ABCDEF"), 11)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	if repo.created != nil {
		t.Error("repo.Create called despite oversized file")
	}
	if store.putKey != "" {
		t.Error("storage.PutObject called despite oversized file")
	}
}

func TestUpload_RejectsUnparseableFile(t *testing.T) {
	repo := &uploadRepoStub{}
	svc := NewService(repo, nil, nil, ServiceConfig{MaxUploadBytes: 1 << 20})

	_, _, _, err := svc.Upload(context.Background(), "bad.txt", []byte("hello world\nnot tsv"), 18)
	if !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("err = %v, want ErrInvalidFormat", err)
	}
	if repo.created != nil {
		t.Error("repo.Create called despite parse failure")
	}
}

func TestUpload_SuccessStoresFileAndAssignsTrackIDs(t *testing.T) {
	body := sampleTSV(t)
	repo := &uploadRepoStub{}
	store := &uploadStorageStub{}
	art := newArtworkFetchStub()
	svc := NewService(repo, store, art, ServiceConfig{StorageBucket: "klubhub", MaxUploadBytes: 10 << 20})

	tl, tracks, _, err := svc.Upload(context.Background(), "set.txt", body, int64(len(body)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Raw file stored under raw-uploads/<tracklistID>/<filename>.
	wantKey := "raw-uploads/" + tl.ID.String() + "/set.txt"
	if store.putKey != wantKey {
		t.Errorf("PutObject key = %q, want %q", store.putKey, wantKey)
	}
	if store.putSize != int64(len(body)) {
		t.Errorf("PutObject size = %d, want %d", store.putSize, len(body))
	}
	if tl.RawFilePath != wantKey {
		t.Errorf("RawFilePath = %q, want %q", tl.RawFilePath, wantKey)
	}

	// Every parsed track got an ID and the tracklist ID.
	if len(tracks) == 0 {
		t.Fatal("expected parsed tracks")
	}
	for _, tr := range tracks {
		if tr.ID == uuid.Nil {
			t.Error("track kept the zero UUID")
		}
		if tr.TracklistID != tl.ID {
			t.Errorf("track %v TracklistID = %v, want %v", tr.ID, tr.TracklistID, tl.ID)
		}
	}

	// Persisted through the repo.
	if repo.created == nil || repo.created.ID != tl.ID {
		t.Errorf("repo.Create got %+v, want the returned tracklist", repo.created)
	}

	// Artwork fetch kicked off asynchronously.
	select {
	case <-art.done:
	case <-time.After(2 * time.Second):
		t.Error("artwork FetchAll was not triggered")
	}
}

func TestUpload_StorageErrorAbortsBeforeRepo(t *testing.T) {
	body := sampleTSV(t)
	repo := &uploadRepoStub{}
	store := &uploadStorageStub{putErr: errors.New("garage down")}
	svc := NewService(repo, store, nil, ServiceConfig{StorageBucket: "klubhub", MaxUploadBytes: 10 << 20})

	_, _, _, err := svc.Upload(context.Background(), "set.txt", body, int64(len(body)))
	if err == nil {
		t.Fatal("Upload succeeded despite storage error")
	}
	if repo.created != nil {
		t.Error("repo.Create called despite storage failure — the tracklist would exist without its raw file")
	}
}

func TestUpload_RepoErrorPropagates(t *testing.T) {
	body := sampleTSV(t)
	repo := &uploadRepoStub{createErr: errors.New("db down")}
	svc := NewService(repo, nil, nil, ServiceConfig{MaxUploadBytes: 10 << 20})

	_, _, _, err := svc.Upload(context.Background(), "set.txt", body, int64(len(body)))
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("err = %v, want the repo error", err)
	}
}

func TestUpload_WithoutStorageSkipsRawFile(t *testing.T) {
	body := sampleTSV(t)
	repo := &uploadRepoStub{}
	// nil storage: the service must still persist the parsed tracklist.
	svc := NewService(repo, nil, nil, ServiceConfig{MaxUploadBytes: 10 << 20})

	tl, _, _, err := svc.Upload(context.Background(), "set.txt", body, int64(len(body)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if tl.RawFilePath != "" {
		t.Errorf("RawFilePath = %q, want empty without storage", tl.RawFilePath)
	}
	if repo.created == nil {
		t.Error("repo.Create not called")
	}
}

// ── UpdateTrack ──────────────────────────────────────────────────────────────

func TestUpdateTrack_ReturnsUpdatedTrack(t *testing.T) {
	trackID := uuid.New()
	repo := &uploadRepoStub{getTracks: []Track{{ID: trackID, Title: "New Title"}}}
	svc := NewService(repo, nil, nil, ServiceConfig{})

	got, err := svc.UpdateTrack(context.Background(), uuid.New(), trackID, UpdateTrackRequest{Title: strPtrS("New Title")})
	if err != nil {
		t.Fatalf("UpdateTrack failed: %v", err)
	}
	if got.Title != "New Title" {
		t.Errorf("title = %q, want New Title", got.Title)
	}
	if repo.updatedReq.Title == nil || *repo.updatedReq.Title != "New Title" {
		t.Errorf("repo got %+v, want the title update", repo.updatedReq)
	}
}

func TestUpdateTrack_RepoErrorPropagates(t *testing.T) {
	repo := &uploadRepoStub{updateErr: errors.New("db down")}
	svc := NewService(repo, nil, nil, ServiceConfig{})

	_, err := svc.UpdateTrack(context.Background(), uuid.New(), uuid.New(), UpdateTrackRequest{})
	if err == nil {
		t.Fatal("UpdateTrack succeeded despite repo error")
	}
}

func TestUpdateTrack_TrackMissingAfterUpdateIsNotFound(t *testing.T) {
	trackID := uuid.New()
	// The repo reports success but the track is absent from the re-read.
	repo := &uploadRepoStub{getTracks: []Track{{ID: uuid.New()}}}
	svc := NewService(repo, nil, nil, ServiceConfig{})

	_, err := svc.UpdateTrack(context.Background(), uuid.New(), trackID, UpdateTrackRequest{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ── SaveManualArtwork without storage ────────────────────────────────────────

func TestSaveManualArtwork_WithoutStorageUsesPlaceholder(t *testing.T) {
	trackID := uuid.New()
	repo := &manualArtworkRepoStub{}
	svc := NewService(repo, nil, nil, ServiceConfig{})

	if err := svc.SaveManualArtwork(context.Background(), uuid.New(), trackID, []byte("jpeg"), "image/jpeg"); err != nil {
		t.Fatalf("SaveManualArtwork failed: %v", err)
	}
	want := "placeholder://manual/" + trackID.String()
	if repo.savedURL != want {
		t.Errorf("saved URL = %q, want %q", repo.savedURL, want)
	}
}

type manualArtworkRepoStub struct {
	uploadRepoStub
	savedURL string
}

func (r *manualArtworkRepoStub) SaveManualArtwork(_ context.Context, trackID uuid.UUID, artworkURL string) error {
	r.savedURL = artworkURL
	return nil
}

// ── GenerateImage ────────────────────────────────────────────────────────────

func TestGenerateImage_RejectsInvalidFormat(t *testing.T) {
	svc := NewService(&uploadRepoStub{}, &uploadStorageStub{}, nil, ServiceConfig{})

	_, err := svc.GenerateImage(context.Background(), uuid.New(), "poster")
	if err == nil || !strings.Contains(err.Error(), "invalid format") {
		t.Fatalf("err = %v, want invalid format error", err)
	}
}

func TestGenerateImage_BothFormatsStoresTwoPNGs(t *testing.T) {
	nuxt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/screenshot/tracklist/") {
			t.Errorf("unexpected Nuxt path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer nuxt.Close()

	store := &uploadStorageStub{presignedURL: "http://public/signed"}
	svc := NewService(&uploadRepoStub{}, store, nil, ServiceConfig{
		NuxtInternalURL: nuxt.URL,
		StorageBucket:   "klubhub",
	})

	id := uuid.New()
	result, err := svc.GenerateImage(context.Background(), id, "both")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("result = %v, want story+square", result)
	}
	for _, f := range []string{"story", "square"} {
		if result[f] != "http://public/signed" {
			t.Errorf("result[%s] = %q", f, result[f])
		}
	}
	// "both" writes story first, then square — the last PutObject is square.
	wantKey := "tracklist-images/" + id.String() + "/square.png"
	if store.putKey != wantKey {
		t.Errorf("last PutObject key = %q, want %q (square is written last)", store.putKey, wantKey)
	}
}

func TestGenerateImage_NuxtErrorPropagates(t *testing.T) {
	nuxt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer nuxt.Close()

	store := &uploadStorageStub{}
	svc := NewService(&uploadRepoStub{}, store, nil, ServiceConfig{
		NuxtInternalURL: nuxt.URL,
		StorageBucket:   "klubhub",
	})

	_, err := svc.GenerateImage(context.Background(), uuid.New(), "story")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want the screenshot 500 surfaced", err)
	}
	if store.putKey != "" {
		t.Error("PutObject called despite screenshot failure")
	}
}

func strPtrS(s string) *string { return &s }
