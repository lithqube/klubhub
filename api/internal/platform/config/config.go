package config

import "github.com/kelseyhightower/envconfig"

// Config holds all application configuration loaded from environment variables.
// Required fields will cause Load() to return an error if not set.
// Optional fields have sensible defaults for local development.
type Config struct {
	// Required: database and MinIO connection
	DatabaseURL    string `envconfig:"DATABASE_URL" required:"true"`
	MinioEndpoint  string `envconfig:"MINIO_ENDPOINT" required:"true"`
	MinioAccessKey string `envconfig:"MINIO_ACCESS_KEY" required:"true"`
	MinioSecretKey string `envconfig:"MINIO_SECRET_KEY" required:"true"`

	// Optional: MinIO public endpoint used for presigned URL generation.
	// Defaults to the internal endpoint if not set.
	MinioPublicEndpoint string `envconfig:"MINIO_PUBLIC_ENDPOINT" default:"http://127.0.0.1:9000"`
	MinioBucket         string `envconfig:"MINIO_BUCKET" default:"klubhub-dj"`
	MinioUseSSL         bool   `envconfig:"MINIO_USE_SSL" default:"false"`

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
