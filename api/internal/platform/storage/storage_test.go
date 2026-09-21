package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/platform/config"
)

func TestPresignedGetObjectUsesPublicEndpoint(t *testing.T) {
	c, err := New(&config.Config{
		S3Endpoint:       "storage:3900",
		S3PublicEndpoint: "http://127.0.0.1:39000",
		S3AccessKey:      "test-access",
		S3SecretKey:      "test-secret",
		S3Bucket:         "klubhub",
		S3Region:         "garage", // explicit region keeps signing offline
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	raw, err := c.PresignedGetObject(context.Background(), "klubhub", "epk/exports/x.pdf", time.Minute, nil)
	if err != nil {
		t.Fatalf("PresignedGetObject returned error: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("presigned URL did not parse: %v", err)
	}
	if u.Host != "127.0.0.1:39000" {
		t.Errorf("presigned host = %q, want the public endpoint 127.0.0.1:39000", u.Host)
	}
	if u.Query().Get("X-Amz-Signature") == "" {
		t.Error("presigned URL is missing X-Amz-Signature")
	}
}

func TestNewEndpointNormalization(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{"bare host:port", "storage:3900", "storage:3900"},
		{"bare ip:port", "127.0.0.1:3900", "127.0.0.1:3900"},
		{"http url", "http://storage:3900", "storage:3900"},
		{"url with path", "https://s3.example.com/some/path", "s3.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(&config.Config{
				S3Endpoint:  tt.endpoint,
				S3AccessKey: "test-access",
				S3SecretKey: "test-secret",
				S3Bucket:    "klubhub",
			})
			if err != nil {
				t.Fatalf("New(%q) returned error: %v", tt.endpoint, err)
			}
			if got := c.mc.EndpointURL().Host; got != tt.want {
				t.Errorf("New(%q) endpoint host = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

// The region lookup uses the caller's request context. A first request that is
// cancelled must not poison the signer: the next presign has to retry and
// succeed, not return the stored cancellation until the process restarts.
func TestPresignedGetObject_RetriesAfterFailedRegionLookup(t *testing.T) {
	var lookups atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">garage</LocationConstraint>`))
	}))
	defer fake.Close()

	c, err := New(&config.Config{
		S3Endpoint:       fake.Listener.Addr().String(), // bare host:port, plain HTTP
		S3PublicEndpoint: "http://127.0.0.1:39000",
		S3AccessKey:      "test-access",
		S3SecretKey:      "test-secret",
		S3Bucket:         "klubhub",
		// No S3Region: forces the GetBucketLocation path.
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.PresignedGetObject(cancelled, "klubhub", "a.pdf", time.Minute, nil); err == nil {
		t.Fatal("expected an error when the region lookup context is cancelled")
	}

	raw, err := c.PresignedGetObject(context.Background(), "klubhub", "a.pdf", time.Minute, nil)
	if err != nil {
		t.Fatalf("second presign failed; the first failure was cached: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("presigned URL did not parse: %v", err)
	}
	if u.Host != "127.0.0.1:39000" {
		t.Errorf("presigned host = %q, want the public endpoint 127.0.0.1:39000", u.Host)
	}

	// A successful signer is cached: no further region lookups.
	before := lookups.Load()
	if _, err := c.PresignedGetObject(context.Background(), "klubhub", "b.pdf", time.Minute, nil); err != nil {
		t.Fatalf("third presign failed: %v", err)
	}
	if after := lookups.Load(); after != before {
		t.Errorf("region was looked up again after success (%d -> %d requests)", before, after)
	}
}
