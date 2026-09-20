package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps the minio-go S3 client configured for Garage.
// publicEndpoint is used for generating presigned URLs visible to external clients.
// It should NOT be the internal Docker network endpoint.
type Client struct {
	mc             *minio.Client
	publicEndpoint string
	bucket         string
}

// New creates a Garage S3 client from the given configuration.
// The internal client uses cfg.S3Endpoint for API calls.
// cfg.S3PublicEndpoint is stored separately for presigned URL base generation.
func New(cfg *config.Config) (*Client, error) {
	// strip any path from the endpoint URL — minio-go rejects URLs with
	// path components ("Endpoint url cannot have fully qualified paths")
	endpoint := cfg.S3Endpoint
	if u, err := url.Parse(endpoint); err == nil {
		// minio.New expects bare host:port; strip scheme + path.
		endpoint = u.Host
	}

	fmt.Printf("[DEBUG] storage.New: S3Endpoint=[%s] -> bare=[%s]\n", cfg.S3Endpoint, endpoint)

	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		mc:             mc,
		publicEndpoint: cfg.S3PublicEndpoint,
		bucket:         cfg.S3Bucket,
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

// HealthCheck performs a lightweight probe against the S3-compatible storage
// by checking if the configured bucket exists. The bucket existence bool is
// tracked because a nil error with a missing bucket means the API is wired
// to a storage target that has not been bootstrapped — that should fail the
// healthcheck, not silently report ok. The caller still sees a non-nil
// error so the upstream health handler can flip the response to 503.
func (c *Client) HealthCheck(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("configured bucket %q does not exist on storage target", c.bucket)
	}
	return nil
}

// PutObject writes an object through Garage's S3 API.
func (c *Client) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return c.mc.PutObject(ctx, bucketName, objectName, reader, size, opts)
}

// GetObject reads an object through Garage's S3 API.
func (c *Client) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error) {
	return c.mc.GetObject(ctx, bucketName, objectName, opts)
}

// StatObject inspects an object through Garage's S3 API.
func (c *Client) StatObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (minio.ObjectInfo, error) {
	return c.mc.StatObject(ctx, bucketName, objectName, opts)
}

// PresignedGetObject returns a signed Garage S3 URL.
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

// RemoveObject deletes an object through Garage's S3 API.
func (c *Client) RemoveObject(ctx context.Context, bucketName, objectName string) error {
	return c.mc.RemoveObject(ctx, bucketName, objectName, minio.RemoveObjectOptions{})
}
