package config

import "github.com/kelseyhightower/envconfig"

// Config holds all application configuration loaded from environment variables.
// Required fields will cause Load() to return an error if not set.
// Optional fields have sensible defaults for local development.
type Config struct {
	// Required: database and S3-compatible storage (MinIO or Garage)
	DatabaseURL     string `envconfig:"DATABASE_URL" required:"true"`
	S3Endpoint      string `envconfig:"S3_ENDPOINT" required:"true"`
	S3AccessKey     string `envconfig:"S3_ACCESS_KEY" required:"true"`
	S3SecretKey     string `envconfig:"S3_SECRET_KEY" required:"true"`

	// Optional: S3 public endpoint for presigned URL generation.
	// Defaults to the internal endpoint if not set.
	S3PublicEndpoint string `envconfig:"S3_PUBLIC_ENDPOINT" default:"http://127.0.0.1:39000"`
	S3Bucket         string `envconfig:"S3_BUCKET" default:"klubhub"`
	S3UseSSL         bool   `envconfig:"S3_USE_SSL" default:"false"`
	S3Region         string `envconfig:"S3_REGION" default:""`

	// Backward aliases (deprecated, use S3_* vars above)
	MinioEndpoint       string `envconfig:"MINIO_ENDPOINT"`
	MinioAccessKey       string `envconfig:"MINIO_ACCESS_KEY"`
	MinioSecretKey       string `envconfig:"MINIO_SECRET_KEY"`
	MinioPublicEndpoint  string `envconfig:"MINIO_PUBLIC_ENDPOINT"`
	MinioBucket          string `envconfig:"MINIO_BUCKET"`
	MinioUseSSL          bool   `envconfig:"MINIO_USE_SSL"`
	MinioRegion          string `envconfig:"MINIO_REGION"`

	// Optional: HTTP server settings
	BindAddress string `envconfig:"BIND_ADDRESS" default:"127.0.0.1"`
	Port        string `envconfig:"PORT" default:"8080"`

	// Optional: logging
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	// Optional: CORS
	CORSOrigin string `envconfig:"CORS_ORIGIN" default:"http://127.0.0.1:3000"`

	// Optional: token encryption key (for social OAuth tokens)
	TokenEncryptionKey string `envconfig:"TOKEN_ENCRYPTION_KEY"`

	// Optional: Spotify OAuth credentials
	SpotifyClientID     string `envconfig:"SPOTIFY_CLIENT_ID"`
	SpotifyClientSecret string `envconfig:"SPOTIFY_CLIENT_SECRET"`

	// Optional: Discogs API credentials
	DiscogsAPIKey string `envconfig:"DISCOGS_API_KEY"`

	// Optional: Instagram/Facebook OAuth credentials
	InstagramClientID     string `envconfig:"INSTAGRAM_CLIENT_ID"`
	InstagramClientSecret string `envconfig:"INSTAGRAM_CLIENT_SECRET"`
	InstagramRedirectURI  string `envconfig:"INSTAGRAM_REDIRECT_URI" default:"http://localhost:3000/auth/instagram/callback"`

	// Optional: Nuxt internal URL for screenshot generation
	NuxtInternalURL string `envconfig:"NUXT_INTERNAL_URL" default:"http://localhost:3000"`
}

// Load reads configuration from environment variables.
// Returns an error if any required field is missing.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
