package storage

import (
	"context"
	"net/url"
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
