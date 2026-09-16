package tracklist

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type garageStorageStub struct {
	putKey       string
	presignedKey string
	presignedURL string
}

func (s *garageStorageStub) PutObject(_ context.Context, _, key string, _ io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	s.putKey = key
	return minio.UploadInfo{}, nil
}

func (s *garageStorageStub) PresignedGetObject(_ context.Context, _, key string, _ time.Duration, _ map[string]string) (string, error) {
	s.presignedKey = key
	return s.presignedURL, nil
}

type artworkRepoStub struct {
	savedTrackID uuid.UUID
	savedURL     string
}

func (r *artworkRepoStub) Create(context.Context, *Tracklist, []Track) error { return nil }
func (r *artworkRepoStub) Get(context.Context, uuid.UUID) (*Tracklist, []Track, error) {
	return nil, nil, nil
}
func (r *artworkRepoStub) List(context.Context) ([]Tracklist, error) { return nil, nil }
func (r *artworkRepoStub) UpdateTrack(context.Context, uuid.UUID, uuid.UUID, UpdateTrackRequest) error {
	return nil
}
func (r *artworkRepoStub) SoftDelete(context.Context, uuid.UUID) error { return nil }
func (r *artworkRepoStub) UpdateArtworkStatus(context.Context, uuid.UUID, string, string, string) error {
	return nil
}
func (r *artworkRepoStub) SaveManualArtwork(_ context.Context, trackID uuid.UUID, artworkURL string) error {
	r.savedTrackID = trackID
	r.savedURL = artworkURL
	return nil
}

func TestSaveManualArtwork_UsesGaragePresignedURL(t *testing.T) {
	trackID := uuid.New()
	store := &garageStorageStub{presignedURL: "http://127.0.0.1:39000/klubhub/signed"}
	repo := &artworkRepoStub{}
	svc := NewService(repo, store, nil, ServiceConfig{StorageBucket: "klubhub"})

	if err := svc.SaveManualArtwork(context.Background(), uuid.New(), trackID, []byte("jpeg"), "image/jpeg"); err != nil {
		t.Fatalf("SaveManualArtwork: %v", err)
	}

	wantKey := "cover-art/manual/" + trackID.String() + ".jpg"
	if store.putKey != wantKey || store.presignedKey != wantKey {
		t.Fatalf("storage keys: put=%q presigned=%q, want %q", store.putKey, store.presignedKey, wantKey)
	}
	if repo.savedTrackID != trackID {
		t.Fatalf("saved track ID = %s, want %s", repo.savedTrackID, trackID)
	}
	if repo.savedURL != store.presignedURL {
		t.Fatalf("saved URL = %q, want Garage presigned URL %q", repo.savedURL, store.presignedURL)
	}
}
