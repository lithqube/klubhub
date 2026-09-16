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

// StorageClient defines the minio-go S3 operations used against Garage.
type StorageClient interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error)
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (minio.ObjectInfo, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams map[string]string) (string, error)
}

// ArtworkCache provides Garage-backed artwork caching through S3.
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

// Put downloads an image and stores it in Garage. The download goes
// through artworkClient which enforces a 10s timeout, 5 MiB body cap,
// and no redirects (Plan B.7). On redirect the body is rejected.
func (c *ArtworkCache) Put(ctx context.Context, title, artist string, imageURL string) error {
	key := cacheKey(title, artist)

	// Build request with the artwork context so timeouts cascade.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build image request: %w", err)
	}
	resp, err := artworkClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	// Cap body read so a malicious / misconfigured provider cannot
	// stream us gigabytes of data. http.MaxBytesReader returns
	// *http.MaxBytesError when the cap is exceeded.
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, artworkBodyCap))
	if err != nil {
		return fmt.Errorf("failed to read image body: %w", err)
	}

	// Upload to Garage through S3.
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
		return fmt.Errorf("failed to upload to object storage: %w", err)
	}

	return nil
}

// Ensure ArtworkCache implements ArtworkCacher interface
var _ ArtworkCacher = (*ArtworkCache)(nil)

// artworkClient is the HTTP client used to fetch cover art from external
// providers (Spotify, Discogs, MusicBrainz, CAA). Plan B.7 enforces:
//   - 10s per-request timeout (covers TLS handshake + headers + body).
//   - Body capped at 5 MiB; oversized responses abort and return an error.
//   - Redirects denied by default (http.ErrUseLastResponse). Provider
//     URLs are expected to resolve directly; if a future provider
//     requires redirects, the policy must additionally validate that
//     the resolved IP is not private/link-local/loopback.
var artworkClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// artworkBodyCap is the maximum response body size we accept (5 MiB).
const artworkBodyCap int64 = 5 << 20
