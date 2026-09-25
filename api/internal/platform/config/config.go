package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application configuration loaded from environment variables.
// Required fields will cause Load() to return an error if not set.
// Optional fields have sensible defaults for local development.
//
// _FILE convention: for every secret env var the binary reads, a sibling
// `<NAME>_FILE` env var (path to a file) is honoured first. This lets ops
// mount Docker `secrets:` (or k8s Secret volumes) without baking the value
// into the compose `environment:` block. Plan C.3.
type Config struct {
	// Required: database and Garage's S3-compatible object storage.
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	S3Endpoint  string `envconfig:"S3_ENDPOINT" required:"true"`
	S3AccessKey string `envconfig:"S3_ACCESS_KEY" required:"true"`
	S3SecretKey string `envconfig:"S3_SECRET_KEY" required:"true"`

	// Optional: S3 public endpoint for presigned URL generation.
	// Defaults to the internal endpoint if not set.
	S3PublicEndpoint string `envconfig:"S3_PUBLIC_ENDPOINT" default:"http://127.0.0.1:39000"`
	S3Bucket         string `envconfig:"S3_BUCKET" default:"klubhub"`
	S3UseSSL         bool   `envconfig:"S3_USE_SSL" default:"false"`
	S3Region         string `envconfig:"S3_REGION" default:""`

	// Optional: HTTP server settings
	BindAddress string `envconfig:"BIND_ADDRESS" default:"127.0.0.1"`
	Port        string `envconfig:"PORT" default:"8080"`

	// Optional: graceful shutdown timeout. Bounds how long srv.Shutdown()
	// waits for in-flight requests to complete after a SIGTERM/SIGINT
	// before forcibly closing remaining connections. Should align with
	// docker-compose `stop_grace_period` for the api service.
	ShutdownTimeoutSec int `envconfig:"SHUTDOWN_TIMEOUT_SEC" default:"30"`

	// Optional: logging
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	// Optional: CORS
	CORSOrigin string `envconfig:"CORS_ORIGIN" default:"http://127.0.0.1:3000"`

	// Optional: token encryption key (for social OAuth tokens)
	TokenEncryptionKey string `envconfig:"TOKEN_ENCRYPTION_KEY"`

	// Optional: Postgres connection parts. When DATABASE_URL is not
	// provided, the api constructs it from these parts (sslmode defaults
	// to "disable" for the docker network, "require" otherwise). Plan
	// C.3 — lets the prod compose use Docker `secrets:` for the password
	// without templating a full DSN into the environment.
	PostgresUser     string `envconfig:"POSTGRES_USER"`
	PostgresPassword string `envconfig:"POSTGRES_PASSWORD"`
	PostgresHost     string `envconfig:"POSTGRES_HOST"`
	PostgresPort     string `envconfig:"POSTGRES_PORT" default:"5432"`
	PostgresDB       string `envconfig:"POSTGRES_DB"`
	PostgresSSLMode  string `envconfig:"POSTGRES_SSLMODE" default:""`

	// Plan B.6: HTTP server timeouts (defaults are conservative for a
	// self-hosted single-user app). Override via env when a reverse
	// proxy is in front.
	HTTPReadHeaderTimeoutSec int `envconfig:"HTTP_READ_HEADER_TIMEOUT_SEC" default:"5"`
	HTTPReadTimeoutSec       int `envconfig:"HTTP_READ_TIMEOUT_SEC" default:"30"`
	HTTPWriteTimeoutSec      int `envconfig:"HTTP_WRITE_TIMEOUT_SEC" default:"60"`
	HTTPIdleTimeoutSec       int `envconfig:"HTTP_IDLE_TIMEOUT_SEC" default:"120"`

	// Required: high-entropy shared secret gating the unauthenticated
	// calendar.ics and per-gig booking PDF endpoints. The Nuxt side
	// proxies these endpoints server-side using this same secret; the
	// value must NEVER be exposed to the client bundle.
	//
	// Rotation impact: any external calendar client (Apple Calendar,
	// Google Calendar, Thunderbird, etc.) that copied the
	// `/api/v1/gigs/calendar.ics?secret=…` URL will need to be updated.
	// Document in CONFIGURATION.md before rotation.
	ICALSecret string `envconfig:"ICAL_SECRET" required:"true"`

	// Optional: Spotify OAuth credentials
	SpotifyClientID     string `envconfig:"SPOTIFY_CLIENT_ID"`
	SpotifyClientSecret string `envconfig:"SPOTIFY_CLIENT_SECRET"`

	// Optional: Discogs API credentials
	DiscogsAPIKey string `envconfig:"DISCOGS_API_KEY"`

	// Optional: Instagram/Facebook OAuth credentials
	InstagramClientID     string `envconfig:"INSTAGRAM_CLIENT_ID"`
	InstagramClientSecret string `envconfig:"INSTAGRAM_CLIENT_SECRET"`
	InstagramRedirectURI  string `envconfig:"INSTAGRAM_REDIRECT_URI" default:"http://localhost:3000/auth/instagram/callback"`

	// Opt-in only: API-only development must not depend on Nuxt.
	ServeFrontend bool `envconfig:"SERVE_FRONTEND" default:"false"`

	// Optional: Nuxt internal URL for screenshot generation and frontend proxying.
	NuxtInternalURL string `envconfig:"NUXT_INTERNAL_URL" default:"http://localhost:3000"`

	// Optional: Plunk (https://github.com/useplunk/plunk) transactional
	// email — open-source self-hosted email platform built on AWS SES.
	//
	// If any of PLUNK_BASE_URL/PLUNK_PROJECT_ID/PLUNK_API_KEY_FILE are set,
	// the API delivers transactional email (invoices issued/paid/cancelled,
	// agreement sent/signed/completed) via Plunk's REST API:
	//
	//     POST {PLUNK_BASE_URL}/api/v1/{PLUNK_PROJECT_ID}/emails
	//     Authorization: Bearer {PLUNK_API_KEY}
	//
	// The same wire format works against hosted Plunk
	// (https://app.useplunk.com) and self-hosted Plunk (PLUNK_BASE_URL
	// points at the self-hosted host/port). Leave all fields blank to
	// disable transactional email — SMTP delivery is not exposed yet;
	// keep PLUNK_BASE_URL empty until credentials are provisioned.
	PlunkBaseURL    string `envconfig:"PLUNK_BASE_URL" default:""`
	PlunkProjectID  string `envconfig:"PLUNK_PROJECT_ID" default:""`
	PlunkAPIKeyFile string `envconfig:"PLUNK_API_KEY_FILE" default:""`
	PlunkFromEmail  string `envconfig:"PLUNK_FROM_EMAIL" default:""`
	PlunkFromName   string `envconfig:"PLUNK_FROM_NAME" default:""`
}

// Load reads configuration from environment variables. For each env var
// `NAME`, a sibling `NAME_FILE` env var (a path) is consulted first; if
// present, its file contents are loaded and exposed as `NAME`. This
// enables the Docker `secrets:` / Kubernetes Secret volume pattern in
// compose without baking values into the environment block. Plan C.3.
//
// If DATABASE_URL is empty after _FILE resolution but POSTGRES_USER +
// POSTGRES_PASSWORD + POSTGRES_HOST + POSTGRES_DB are all set, the
// loader synthesizes the DSN (Plan C.3). This lets the prod compose
// mount the password as a Docker secret file instead of templating a
// full connection string.
//
// Returns an error if any required field is missing after _FILE
// resolution and DSN synthesis.
func Load() (*Config, error) {
	if err := loadFileSecrets(); err != nil {
		return nil, err
	}
	cfg := &Config{}
	// We need to populate every other field so synthesizeDSN can read
	// POSTGRES_* from the struct, but DATABASE_URL is required:true and
	// we don't yet know if it's empty (synthesizable) or genuinely
	// missing. Pre-populate DATABASE_URL with a placeholder so the first
	// pass doesn't reject on the empty value, then strip it and
	// synthesize if appropriate.
	if os.Getenv("DATABASE_URL") == "" && os.Getenv("DATABASE_URL_FILE") == "" {
		_ = os.Setenv("DATABASE_URL", "_pending_synthesis_")
		// We can't defer os.Unsetenv here because the second pass below
		// would then re-read an empty value and clobber the synthesized
		// URL. Unsetenv *after* the second pass instead.
	}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	if cfg.DatabaseURL == "_pending_synthesis_" {
		cfg.DatabaseURL = ""
		// Note: we cannot Unsetenv here — the second envconfig.Process
		// pass below still needs the placeholder to be non-empty so the
		// required-key check passes. That pass is responsible for
		// clearing the placeholder via its own deferred Unsetenv.
	}
	if cfg.DatabaseURL == "" {
		dsn, err := synthesizeDSN(cfg)
		if err != nil {
			return nil, err
		}
		if dsn == "" {
			// Neither DATABASE_URL nor POSTGRES_USER/PASSWORD/HOST/DB
			// were set — fail closed. Returning a successful Load with
			// an empty DatabaseURL would just fail at the first DB call.
			return nil, fmt.Errorf("DATABASE_URL is required (or set POSTGRES_USER+POSTGRES_PASSWORD+POSTGRES_HOST+POSTGRES_DB)")
		}
		cfg.DatabaseURL = dsn
	}
	// Second pass: validate required-marked fields with DATABASE_URL now
	// populated (either by them or by us). The placeholder env var is
	// still in the environment; we leave it in place so the validation
	// pass sees a non-empty value, but it never overrides cfg.DatabaseURL
	// because the real value was already written by synthesizeDSN.
	//
	// The envconfig.Process call repopulates cfg from env. To avoid
	// overwriting our synthesized value with the placeholder, we
	// temporarily swap the env var for the synthesized DSN, then
	// restore.
	if got := os.Getenv("DATABASE_URL"); got == "_pending_synthesis_" && cfg.DatabaseURL != "" {
		_ = os.Setenv("DATABASE_URL", cfg.DatabaseURL)
		defer func() {
			if got == "_pending_synthesis_" {
				os.Unsetenv("DATABASE_URL")
			}
		}()
	} else if got := os.Getenv("DATABASE_URL"); got == "_pending_synthesis_" && cfg.DatabaseURL == "" {
		// No POSTGRES_* parts to synthesize from and no DATABASE_URL set
		// by the caller — leave the placeholder so the required-key
		// check fires with a clear message instead of silently failing
		// elsewhere. The placeholder is cleaned up by Load's exit path
		// below; sibling tests see a clean environment.
		defer func() {
			if os.Getenv("DATABASE_URL") == "_pending_synthesis_" {
				os.Unsetenv("DATABASE_URL")
			}
		}()
	}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// synthesizeDSN builds a Postgres DSN from the POSTGRES_* env vars. The
// password is URL-escaped so values containing `?`, `@`, `#`, or other
// reserved characters don't break the DSN.
func synthesizeDSN(cfg *Config) (string, error) {
	if cfg.PostgresUser == "" || cfg.PostgresHost == "" || cfg.PostgresDB == "" {
		// Either DATABASE_URL is required or all of USER/HOST/DB must
		// be set; we don't infer defaults for those (db host unknown).
		return "", nil
	}
	sslmode := cfg.PostgresSSLMode
	if sslmode == "" {
		// Loopback / docker-network style: disable. Public / unknown
		// host: require. Operators can override via POSTGRES_SSLMODE.
		if cfg.PostgresHost == "db" || cfg.PostgresHost == "localhost" || cfg.PostgresHost == "127.0.0.1" {
			sslmode = "disable"
		} else {
			sslmode = "require"
		}
	}
	port := cfg.PostgresPort
	if port == "" {
		port = "5432"
	}
	userInfo := url.UserPassword(cfg.PostgresUser, cfg.PostgresPassword).String()
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
		userInfo, cfg.PostgresHost, port, cfg.PostgresDB, sslmode), nil
}

// loadFileSecrets walks the current process env, finds every `*_FILE` var,
// reads the referenced file (trimmed of trailing whitespace), and sets the
// bare-name env var so the downstream envconfig.Process() call picks it up.
// A missing file is a hard error — silent fallback to an empty value would
// be worse than failing at boot.
func loadFileSecrets() error {
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		name := kv[:eq]
		if !strings.HasSuffix(name, "_FILE") {
			continue
		}
		path := kv[eq+1:]
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read secret file for %s: %w", name, err)
		}
		bare := strings.TrimSuffix(name, "_FILE")
		// Don't overwrite an explicitly-set bare value with a file-derived
		// one — explicit beats implicit. Only set if bare is empty.
		if os.Getenv(bare) == "" {
			if err := os.Setenv(bare, strings.TrimRight(string(raw), " 	\r\n")); err != nil {
				return fmt.Errorf("set env %s from file: %w", bare, err)
			}
		}
	}
	return nil
}
