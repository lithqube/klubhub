package epk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/klubhub/dj/api/internal/settings"
)

// StorageClientAdapter wraps a storage client that uses minio.PutObjectOptions and adapts
// it to the storageIface expected by the EPK service.
type StorageClientAdapter struct {
	client storageClientIface
	bucket string
}

// storageClientIface is the subset of storage.Client used by the adapter.
type storageClientIface interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	RemoveObject(ctx context.Context, bucketName, objectName string) error
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams map[string]string) (string, error)
}

// NewStorageAdapter wraps a storage.Client in the thin interface expected by the EPK service.
func NewStorageAdapter(client storageClientIface) *StorageClientAdapter {
	return &StorageClientAdapter{client: client}
}

func (a *StorageClientAdapter) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	_, err := a.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (a *StorageClientAdapter) DeleteObject(ctx context.Context, bucket, key string) error {
	return a.client.RemoveObject(ctx, bucket, key)
}

func (a *StorageClientAdapter) PresignedGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	return a.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
}

// settingsServiceAdapter wraps settings.Service to match settingsServiceIface.
// settings.Service exposes GetOrCreate/Update but not GetSettings/UpdateSettings.
type settingsServiceAdapter struct {
	svc settingsServiceRealIface
}

// settingsServiceRealIface is the subset of settings.Service the adapter uses.
type settingsServiceRealIface interface {
	GetOrCreate(ctx context.Context) (*settings.UserSettings, error)
	Update(ctx context.Context, req settings.UpdateSettingsRequest) (*settings.UserSettings, error)
}

// NewSettingsServiceAdapter wraps a settings.Service to match settingsServiceIface.
func NewSettingsServiceAdapter(svc settingsServiceRealIface) *settingsServiceAdapter {
	return &settingsServiceAdapter{svc: svc}
}

func (a *settingsServiceAdapter) GetSettings(ctx context.Context) (*settings.UserSettings, error) {
	return a.svc.GetOrCreate(ctx)
}

func (a *settingsServiceAdapter) UpdateSettings(ctx context.Context, req settings.UpdateSettingsRequest) (*settings.UserSettings, error) {
	return a.svc.Update(ctx, req)
}

// Sentinel errors returned by the service layer.
var (
	ErrPhotoLimitExceeded = errors.New("photo limit reached: max 20 photos")
	ErrInvalidMIME        = errors.New("photo must be JPEG or PNG")
)

// const for the MinIO bucket (consistent with existing usage).
const minioBucket = "klubhub"

// storageIface is the thin interface the EPK service needs from MinIO.
type storageIface interface {
	// PutObject stores an object. contentType is the MIME type e.g. "image/jpeg".
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	// DeleteObject removes an object by key.
	DeleteObject(ctx context.Context, bucket, key string) error
	// PresignedGetObject returns a time-limited download URL.
	PresignedGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// settingsServiceIface is the thin interface the EPK service needs from the settings module.
type settingsServiceIface interface {
	UpdateSettings(ctx context.Context, req settings.UpdateSettingsRequest) (*settings.UserSettings, error)
	GetSettings(ctx context.Context) (*settings.UserSettings, error)
}

// serviceIface is the contract exposed to the HTTP handler layer.
type serviceIface interface {
	GetContent(ctx context.Context) (*EPKContent, error)
	UpsertContent(ctx context.Context, req UpsertEPKContentRequest) (*EPKContent, error)
	UploadPhoto(ctx context.Context, data []byte, mimeType string) (string, error)
	DeletePhoto(ctx context.Context, path string) error
	UploadStagePlot(ctx context.Context, data []byte, mimeType string) (string, error)
	GeneratePDF(ctx context.Context) (*ExportResult, error)
	ListExports(ctx context.Context) ([]EPKExport, error)
	DeleteExport(ctx context.Context, id uuid.UUID) error
}

// ExportResult is returned by GeneratePDF.
type ExportResult struct {
	ID          uuid.UUID `json:"id"`
	DownloadURL string    `json:"downloadUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Service implements serviceIface.
type Service struct {
	repo        repoIface
	storage     storageIface
	settingsSvc settingsServiceIface
}

// NewService creates a new Service with the given dependencies.
func NewService(repo repoIface, storage storageIface, settingsSvc settingsServiceIface) *Service {
	return &Service{
		repo:        repo,
		storage:     storage,
		settingsSvc: settingsSvc,
	}
}

// GetContent returns the singleton EPK content, seeding an empty default if none exists.
func (s *Service) GetContent(ctx context.Context) (*EPKContent, error) {
	content, err := s.repo.GetContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}
	if content == nil {
		// Seed an empty default row.
		content, err = s.repo.UpsertContent(ctx, UpsertEPKContentRequest{})
		if err != nil {
			return nil, fmt.Errorf("seed default content: %w", err)
		}
	}
	return content, nil
}

// UpsertContent saves partial fields. If BioShort or BioLong is non-nil, it also
// syncs the values to user_settings for single source of truth.
func (s *Service) UpsertContent(ctx context.Context, req UpsertEPKContentRequest) (*EPKContent, error) {
	content, err := s.repo.UpsertContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("upsert content: %w", err)
	}

	// Sync bio fields to user_settings if provided.
	if req.BioShort != nil || req.BioLong != nil {
		// Fetch current settings for the optimistic concurrency token.
		cur, err := s.settingsSvc.GetSettings(ctx)
		if err != nil {
			return nil, fmt.Errorf("get settings for bio sync: %w", err)
		}
		updateReq := settings.UpdateSettingsRequest{
			DJName:          cur.DJName,
			LogoPath:        cur.LogoPath,
			DefaultColors:   cur.DefaultColors,
			DefaultTemplate: cur.DefaultTemplate,
			VisibleFields:   cur.VisibleFields,
			SocialLinks:     cur.SocialLinks,
			BioShort:        cur.BioShort,
			BioLong:         cur.BioLong,
			ContactInfo:     cur.ContactInfo,
			InvoicePrefix:   cur.InvoicePrefix,
			UpdatedAt:       cur.UpdatedAt,
		}
		if req.BioShort != nil {
			updateReq.BioShort = *req.BioShort
		}
		if req.BioLong != nil {
			updateReq.BioLong = *req.BioLong
		}
		if _, err := s.settingsSvc.UpdateSettings(ctx, updateReq); err != nil {
			return nil, fmt.Errorf("sync bio to settings: %w", err)
		}
	}

	return content, nil
}

// UploadPhoto validates the MIME type and size, stores the file in MinIO, and appends
// the path to the photo_paths JSONB array. Returns ErrPhotoLimitExceeded at 20 photos.
func (s *Service) UploadPhoto(ctx context.Context, data []byte, mimeType string) (string, error) {
	// Validate MIME type.
	mt := mimetype.Detect(data)
	if mt.String() != "image/jpeg" && mt.String() != "image/png" {
		return "", ErrInvalidMIME
	}

	// Validate size (10 MB).
	if len(data) > 10*1024*1024 {
		return "", fmt.Errorf("photo exceeds 10 MB limit")
	}

	// Check current photo count.
	content, err := s.GetContent(ctx)
	if err != nil {
		return "", fmt.Errorf("get content for photo upload: %w", err)
	}
	if len(content.PhotoPaths) >= 20 {
		return "", ErrPhotoLimitExceeded
	}

	// Determine extension.
	ext := "jpg"
	if mt.String() == "image/png" {
		ext = "png"
	}

	// Store in MinIO.
	key := fmt.Sprintf("epk/photos/%s.%s", uuid.New().String(), ext)
	if err := s.storage.PutObject(ctx, minioBucket, key, bytes.NewReader(data), int64(len(data)), mt.String()); err != nil {
		return "", fmt.Errorf("store photo: %w", err)
	}

	// Append path to photo_paths.
	updatedPaths := append(content.PhotoPaths, key)
	if _, err := s.repo.UpsertContent(ctx, UpsertEPKContentRequest{PhotoPaths: updatedPaths}); err != nil {
		return "", fmt.Errorf("update photo paths: %w", err)
	}

	return key, nil
}

// DeletePhoto removes the photo from MinIO and from the photo_paths JSONB array.
func (s *Service) DeletePhoto(ctx context.Context, path string) error {
	// Remove from MinIO.
	if err := s.storage.DeleteObject(ctx, minioBucket, path); err != nil {
		return fmt.Errorf("delete photo from storage: %w", err)
	}

	// Remove from photo_paths.
	content, err := s.GetContent(ctx)
	if err != nil {
		return fmt.Errorf("get content for photo delete: %w", err)
	}

	updated := make([]string, 0, len(content.PhotoPaths))
	for _, p := range content.PhotoPaths {
		if p != path {
			updated = append(updated, p)
		}
	}
	if _, err := s.repo.UpsertContent(ctx, UpsertEPKContentRequest{PhotoPaths: updated}); err != nil {
		return fmt.Errorf("update photo paths after delete: %w", err)
	}

	return nil
}

// UploadStagePlot validates MIME type and stores the stage plot image in MinIO.
// Updates stage_plot_path on the EPK content.
func (s *Service) UploadStagePlot(ctx context.Context, data []byte, mimeType string) (string, error) {
	mt := mimetype.Detect(data)
	if mt.String() != "image/jpeg" && mt.String() != "image/png" {
		return "", ErrInvalidMIME
	}

	ext := "jpg"
	if mt.String() == "image/png" {
		ext = "png"
	}

	key := fmt.Sprintf("epk/stage-plot/%s.%s", uuid.New().String(), ext)
	if err := s.storage.PutObject(ctx, minioBucket, key, bytes.NewReader(data), int64(len(data)), mt.String()); err != nil {
		return "", fmt.Errorf("store stage plot: %w", err)
	}

	if _, err := s.repo.UpsertContent(ctx, UpsertEPKContentRequest{StagePlotPath: &key}); err != nil {
		return "", fmt.Errorf("update stage plot path: %w", err)
	}

	return key, nil
}

// GeneratePDF loads content + settings, calls renderEPKPDF, stores the result in MinIO,
// inserts an epk_exports row, and returns the presigned download URL (15 min expiry).
func (s *Service) GeneratePDF(ctx context.Context) (*ExportResult, error) {
	content, err := s.GetContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("get content for PDF: %w", err)
	}

	userSettings, err := s.settingsSvc.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get settings for PDF: %w", err)
	}

	var buf bytes.Buffer
	if err := renderEPKPDF(content, userSettings, &buf); err != nil {
		return nil, fmt.Errorf("render PDF: %w", err)
	}

	pdfData := buf.Bytes()
	key := fmt.Sprintf("epk/exports/%s.pdf", uuid.New().String())
	if err := s.storage.PutObject(ctx, minioBucket, key, bytes.NewReader(pdfData), int64(len(pdfData)), "application/pdf"); err != nil {
		return nil, fmt.Errorf("store PDF: %w", err)
	}

	export, err := s.repo.InsertExport(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("insert export record: %w", err)
	}

	downloadURL, err := s.storage.PresignedGetObject(ctx, minioBucket, key, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("generate presigned URL: %w", err)
	}

	return &ExportResult{
		ID:          export.ID,
		DownloadURL: downloadURL,
		CreatedAt:   export.CreatedAt,
	}, nil
}

// ListExports delegates to repo; returns newest-first.
func (s *Service) ListExports(ctx context.Context) ([]EPKExport, error) {
	return s.repo.ListExports(ctx)
}

// DeleteExport removes the MinIO object then deletes the DB row.
func (s *Service) DeleteExport(ctx context.Context, id uuid.UUID) error {
	exports, err := s.repo.ListExports(ctx)
	if err != nil {
		return fmt.Errorf("list exports for delete: %w", err)
	}

	var minioPath string
	for _, e := range exports {
		if e.ID == id {
			minioPath = e.MinioPath
			break
		}
	}
	if minioPath == "" {
		return ErrNotFound
	}

	if err := s.storage.DeleteObject(ctx, minioBucket, minioPath); err != nil {
		return fmt.Errorf("delete export from storage: %w", err)
	}

	return s.repo.DeleteExport(ctx, id)
}

// sectionEnabled returns true when the section key is absent (default enabled) or explicitly true.
func sectionEnabled(visibility map[string]bool, key string) bool {
	if visibility == nil {
		return true
	}
	enabled, ok := visibility[key]
	if !ok {
		return true
	}
	return enabled
}

// isBlank returns true when the string is empty or whitespace-only.
func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}
