package config_test

import (
	"os"
	"testing"

	"github.com/klubhub/dj/api/internal/platform/config"
)

func TestConfigLoadWithRequiredFieldsSucceeds(t *testing.T) {
	// Set required env vars
	os.Setenv("DATABASE_URL", "postgres://localhost:5432/testdb")
	os.Setenv("MINIO_ENDPOINT", "localhost:9000")
	os.Setenv("MINIO_ACCESS_KEY", "minioadmin")
	os.Setenv("MINIO_SECRET_KEY", "minioadmin")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("MINIO_ENDPOINT")
		os.Unsetenv("MINIO_ACCESS_KEY")
		os.Unsetenv("MINIO_SECRET_KEY")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify optional social fields default to empty string
	if cfg.SpotifyClientID != "" {
		t.Errorf("expected SpotifyClientID to be empty, got %q", cfg.SpotifyClientID)
	}
	if cfg.SpotifyClientSecret != "" {
		t.Errorf("expected SpotifyClientSecret to be empty, got %q", cfg.SpotifyClientSecret)
	}
	if cfg.DiscogsAPIKey != "" {
		t.Errorf("expected DiscogsAPIKey to be empty, got %q", cfg.DiscogsAPIKey)
	}
	if cfg.InstagramClientID != "" {
		t.Errorf("expected InstagramClientID to be empty, got %q", cfg.InstagramClientID)
	}
}

func TestConfigLoadWithoutDatabaseURLReturnsError(t *testing.T) {
	// Ensure DATABASE_URL is not set
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("MINIO_ENDPOINT")
	os.Unsetenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_SECRET_KEY")

	_, err := config.Load()
	if err == nil {
		t.Error("expected error when DATABASE_URL is not set, got nil")
	}
}
