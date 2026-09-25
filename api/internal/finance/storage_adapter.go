package finance

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"

	"github.com/klubhub/dj/api/internal/platform/storage"
)

// StorageAdapter adapts *storage.Client (minio options in its signatures)
// to the narrow ObjectStore interface the document service depends on.
type StorageAdapter struct {
	client *storage.Client
}

// NewStorageAdapter wraps client as an ObjectStore.
func NewStorageAdapter(client *storage.Client) *StorageAdapter {
	return &StorageAdapter{client: client}
}

// PutObject implements ObjectStore.
func (a *StorageAdapter) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	_, err := a.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// GetObject implements ObjectStore.
func (a *StorageAdapter) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return a.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

// DeleteObject implements ObjectStore.
func (a *StorageAdapter) DeleteObject(ctx context.Context, bucket, key string) error {
	return a.client.RemoveObject(ctx, bucket, key)
}

var _ ObjectStore = (*StorageAdapter)(nil)
