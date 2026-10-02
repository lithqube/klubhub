package rider

import (
	"context"
	"io"
	"time"

	"github.com/klubhub/dj/api/internal/settings"
	"github.com/minio/minio-go/v7"
)

// storageClientIface is the subset of the minio-go S3 client the rider
// package needs. *storage.Client (which wraps the real Garage-compatible
// client) satisfies it; tests inject fakes.
type storageClientIface interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	RemoveObject(ctx context.Context, bucketName, objectName string) error
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams map[string]string) (string, error)
}

// StorageAdapter wraps a minio-go S3 client and adapts it to the
// StorageIface expected by the rider service. Same pattern as
// api/internal/epk/service.go's StorageClientAdapter.
type StorageAdapter struct {
	client storageClientIface
}

// NewStorageAdapter wraps a *storage.Client (or any other storageClientIface)
// in the small interface the rider service consumes.
func NewStorageAdapter(client storageClientIface) *StorageAdapter {
	return &StorageAdapter{client: client}
}

func (a *StorageAdapter) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	_, err := a.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (a *StorageAdapter) PresignedGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	return a.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
}

func (a *StorageAdapter) DeleteObject(ctx context.Context, bucket, key string) error {
	return a.client.RemoveObject(ctx, bucket, key)
}

// ─── Settings adapter ──────────────────────────────────────────────────────

// settingsServiceRealIface is the subset of settings.Service the rider
// service uses (same shape EPK consumes via NewSettingsServiceAdapter).
type settingsServiceRealIface interface {
	GetOrCreate(ctx context.Context) (*settings.UserSettings, error)
}

// settingsServiceAdapter wraps settings.Service to match the SettingsIface
// the rider service declares (which only needs GetSettings — the service
// reads the singleton DJ name + accent color for the PDF header).
type settingsServiceAdapter struct {
	svc settingsServiceRealIface
}

// NewSettingsServiceAdapter wraps a settings.Service for the rider package.
func NewSettingsServiceAdapter(svc settingsServiceRealIface) *settingsServiceAdapter {
	return &settingsServiceAdapter{svc: svc}
}

func (a *settingsServiceAdapter) GetSettings(ctx context.Context) (*settings.UserSettings, error) {
	return a.svc.GetOrCreate(ctx)
}