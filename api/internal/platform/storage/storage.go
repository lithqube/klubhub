package storage

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps a MinIO client with bucket configuration.
// publicEndpoint is used for generating presigned URLs visible to external clients.
// It should NOT be the internal Docker network endpoint.
type Client struct {
	mc             *minio.Client
	publicEndpoint string
	bucket         string
}

// New creates a new MinIO Client from the given configuration.
// The internal MinIO client uses cfg.MinioEndpoint for API calls.
// cfg.MinioPublicEndpoint is stored separately for presigned URL base generation.
func New(cfg *config.Config) (*Client, error) {
	mc, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		mc:             mc,
		publicEndpoint: cfg.MinioPublicEndpoint,
		bucket:         cfg.MinioBucket,
	}, nil
}

// EnsureBucket creates the configured bucket if it does not already exist.
func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}

	if !exists {
		if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// Bucket returns the configured bucket name.
func (c *Client) Bucket() string {
	return c.bucket
}

// PublicEndpoint returns the public endpoint used for presigned URL generation.
func (c *Client) PublicEndpoint() string {
	return c.publicEndpoint
}

// HealthCheck performs a lightweight probe against MinIO by checking if the
// configured bucket exists. Returns nil if MinIO is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.mc.BucketExists(ctx, c.bucket)
	return err
}

// PutObject wraps minio.Client.PutObject
func (c *Client) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return c.mc.PutObject(ctx, bucketName, objectName, reader, size, opts)
}

// GetObject wraps minio.Client.GetObject
func (c *Client) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error) {
	return c.mc.GetObject(ctx, bucketName, objectName, opts)
}

// StatObject wraps minio.Client.StatObject
func (c *Client) StatObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (minio.ObjectInfo, error) {
	return c.mc.StatObject(ctx, bucketName, objectName, opts)
}

// PresignedGetObject wraps minio.Client.PresignedGetObject
func (c *Client) PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams map[string]string) (string, error) {
	reqValues := url.Values{}
	for k, v := range reqParams {
		reqValues.Set(k, v)
	}
	presignedURL, err := c.mc.PresignedGetObject(ctx, bucketName, objectName, expiry, reqValues)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

// RemoveObject wraps minio.Client.RemoveObject with default remove options.
func (c *Client) RemoveObject(ctx context.Context, bucketName, objectName string) error {
	return c.mc.RemoveObject(ctx, bucketName, objectName, minio.RemoveObjectOptions{})
}
