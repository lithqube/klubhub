package artwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
)

// StorageClient defines the minimal interface we need from MinIO client
type StorageClient interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error)
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (minio.ObjectInfo, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams map[string]string) (string, error)
}

// ArtworkCache provides MinIO-backed artwork caching
type ArtworkCache struct {
	store          StorageClient
	bucket         string
	publicEndpoint string
}

// NewArtworkCache creates a new artwork cache
func NewArtworkCache(store StorageClient, bucket, publicEndpoint string) *ArtworkCache {
	if publicEndpoint == "" {
		publicEndpoint = "http://localhost:9000"
	}
	return &ArtworkCache{
		store:          store,
		bucket:         bucket,
		publicEndpoint: publicEndpoint,
	}
}

// cacheKey generates a cache key from title and artist
func cacheKey(title, artist string) string {
	hash := sha256.Sum256([]byte(title + "|" + artist))
	hashStr := hex.EncodeToString(hash[:])
	return "cover-art/" + hashStr[:16] + ".jpg"
}

// Get retrieves artwork from cache, returning presigned URL if found
func (c *ArtworkCache) Get(ctx context.Context, title, artist string) string {
	key := cacheKey(title, artist)

	// Check if object exists
	_, err := c.store.StatObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		// Object doesn't exist
		return ""
	}

	// Generate presigned URL (7 days expiry)
	presignedURL, err := c.store.PresignedGetObject(ctx, c.bucket, key, 7*24*time.Hour, nil)
	if err != nil {
		// If presigned URL fails, return direct URL
		return c.publicEndpoint + "/" + c.bucket + "/" + key
	}

	return presignedURL
}

// Put downloads image from URL and stores in MinIO
func (c *ArtworkCache) Put(ctx context.Context, title, artist string, imageURL string) error {
	key := cacheKey(title, artist)

	// Download image from URL
	resp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read image body: %w", err)
	}

	// Upload to MinIO
	_, err = c.store.PutObject(
		ctx,
		c.bucket,
		key,
		nil,
		int64(len(body)),
		minio.PutObjectOptions{
			ContentType: "image/jpeg",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to upload to MinIO: %w", err)
	}

	return nil
}

// Ensure ArtworkCache implements ArtworkCacher interface
var _ ArtworkCacher = (*ArtworkCache)(nil)
