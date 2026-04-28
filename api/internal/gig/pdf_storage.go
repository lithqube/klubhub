package gig

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/klubhub/dj/api/internal/platform/storage"
)

// StorageAdapter wraps storage.Client to implement the PDFStorageClientIface
// with gofpdf-compatible signature (contentType string instead of minio.PutObjectOptions).
type StorageAdapter struct {
	client *storage.Client
}

// NewStorageAdapter creates an adapter wrapping the given storage.Client.
func NewStorageAdapter(client *storage.Client) PDFStorageClientIface {
	return &StorageAdapter{client: client}
}

// PutObject implements PDFStorageClientIface.
func (a *StorageAdapter) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{}
	if contentType != "" {
		opts.ContentType = contentType
	}
	_, err := a.client.PutObject(ctx, bucket, key, r, size, opts)
	return err
}
