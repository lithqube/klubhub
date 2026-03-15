package tracklist

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// Service provides business logic for tracklists
type Service struct {
	repo    tracklistRepoIface
	storage storageIface
	artwork artworkServiceIface
	config  serviceConfig
}

// serviceConfig holds configuration for the service
type serviceConfig struct {
	NuxtInternalURL string
	StorageBucket   string
	MaxUploadBytes  int64
}

// tracklistRepoIface defines the interface for the tracklist repository
type tracklistRepoIface interface {
	Create(ctx context.Context, tl *Tracklist, tracks []Track) error
	Get(ctx context.Context, id uuid.UUID) (*Tracklist, []Track, error)
	List(ctx context.Context) ([]Tracklist, error)
	UpdateTrack(ctx context.Context, tracklistID, trackID uuid.UUID, req UpdateTrackRequest) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	UpdateArtworkStatus(ctx context.Context, trackID uuid.UUID, status, url, source string) error
	SaveManualArtwork(ctx context.Context, trackID uuid.UUID, artworkURL string) error
}

// artworkServiceIface defines the interface for the artwork service
type artworkServiceIface interface {
	FetchAll(ctx context.Context, tracks []Track)
}

// storageIface defines the interface for object storage
type storageIface interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
}

// NewService creates a new Service with the given dependencies
func NewService(repo tracklistRepoIface, storage storageIface, artwork artworkServiceIface, config serviceConfig) *Service {
	return &Service{
		repo:    repo,
		storage: storage,
		artwork: artwork,
		config:  config,
	}
}

// Upload handles the upload of a tracklist file
func (s *Service) Upload(ctx context.Context, filename string, body []byte, size int64) (*Tracklist, []Track, []ParseWarning, error) {
	// Check file size
	if size > s.config.MaxUploadBytes {
		return nil, nil, nil, errors.New("file_too_large")
	}

	// Parse the TSV file
	tracks, warnings, err := ParseRekordboxTSV(bytes.NewReader(body))
	if err != nil {
		return nil, nil, nil, err
	}

	// Generate tracklist ID
	tlID := uuid.New()

	// Create tracklist
	tl := Tracklist{
		ID:              tlID,
		Title:           filename,
		SourceFormat:    "rekordbox", // TODO: detect format properly
		RawFilePath:     "",          // Will be set after storing in MinIO
		Preset:          "default",
		VisibleFields:   `["title","artist","bpm","key","time_played"]`,
		BgMode:          "solid",
		BgValue:         "",
		MaxTracks:       30,
		TrackRangeStart: 0,
		TrackRangeEnd:   0,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		DeletedAt:       nil,
	}

	// Store raw file in MinIO
	if s.storage != nil && s.config.StorageBucket != "" {
		objectName := "raw-uploads/" + tlID.String() + "/" + filename
		opts := minio.PutObjectOptions{ContentType: "application/octet-stream"}
		reader := bytes.NewReader(body)
		_, err := s.storage.PutObject(ctx, s.config.StorageBucket, objectName, reader, size, opts)
		if err != nil {
			return nil, nil, nil, err
		}
		tl.RawFilePath = objectName
	}

	// Save to database
	if err := s.repo.Create(ctx, &tl, tracks); err != nil {
		return nil, nil, nil, err
	}

	// Start artwork fetching as a fire-and-forget goroutine
	if s.artwork != nil {
		go func() {
			// Use background context to avoid cancellation from the request context
			s.artwork.FetchAll(context.Background(), tracks)
		}()
	}

	return &tl, tracks, warnings, nil
}

// GetWithTracks returns a tracklist with its tracks
func (s *Service) GetWithTracks(ctx context.Context, id uuid.UUID) (*Tracklist, []Track, error) {
	return s.repo.Get(ctx, id)
}

// List returns all tracklists
func (s *Service) List(ctx context.Context) ([]Tracklist, error) {
	return s.repo.List(ctx)
}

// UpdateTrack updates a track
func (s *Service) UpdateTrack(ctx context.Context, tracklistID, trackID uuid.UUID, req UpdateTrackRequest) error {
	return s.repo.UpdateTrack(ctx, tracklistID, trackID, req)
}

// SoftDelete soft-deletes a tracklist
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDelete(ctx, id)
}

// SaveManualArtwork saves manual artwork for a track
func (s *Service) SaveManualArtwork(ctx context.Context, tracklistID, trackID uuid.UUID, imageData []byte, contentType string) error {
	// Store image in MinIO
	if s.storage != nil && s.config.StorageBucket != "" {
		objectName := "cover-art/manual/" + trackID.String() + ".jpg"
		opts := minio.PutObjectOptions{ContentType: contentType}
		reader := bytes.NewReader(imageData)
		_, err := s.storage.PutObject(ctx, s.config.StorageBucket, objectName, reader, int64(len(imageData)), opts)
		if err != nil {
			return err
		}

		// Build artwork URL (in a real implementation, this would be a proper URL)
		artworkURL := "http://localhost:9000/" + s.config.StorageBucket + "/" + objectName

		// Save to database
		return s.repo.SaveManualArtwork(ctx, trackID, artworkURL)
	}

	// If no storage is configured, just save to database with a placeholder URL
	return s.repo.SaveManualArtwork(ctx, trackID, "placeholder://manual/"+trackID.String())
}
