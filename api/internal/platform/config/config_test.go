package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/platform/config"
)

func TestConfigLoadWithRequiredFieldsSucceeds(t *testing.T) {
	// Set required env vars
	os.Setenv("DATABASE_URL", "postgres://localhost:5432/testdb")
	os.Setenv("S3_ENDPOINT", "localhost:9000")
	os.Setenv("S3_ACCESS_KEY", "minioadmin")
	os.Setenv("S3_SECRET_KEY", "minioadmin")
	os.Setenv("ICAL_SECRET", "test-ical-secret")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("S3_ENDPOINT")
		os.Unsetenv("S3_ACCESS_KEY")
		os.Unsetenv("S3_SECRET_KEY")
		os.Unsetenv("ICAL_SECRET")
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
	os.Unsetenv("S3_ENDPOINT")
	os.Unsetenv("S3_ACCESS_KEY")
	os.Unsetenv("S3_SECRET_KEY")

	_, err := config.Load()
	if err == nil {
		t.Error("expected error when DATABASE_URL is not set, got nil")
	}
}

func TestConfigLoadRespectsFileEnvConvention(t *testing.T) {
	// Plan C.3: write the secret to a temp file, point *_FILE at it,
	// expect the bare env var to be populated by Load().
	dir := t.TempDir()
	dbFile := dir + "/db"
	icalFile := dir + "/ical"
	if err := os.WriteFile(dbFile, []byte("postgres://klubhub:fromfile@db:5432/klubhub"), 0o600); err != nil {
		t.Fatalf("write db file: %v", err)
	}
	if err := os.WriteFile(icalFile, []byte("file-ical-secret"), 0o600); err != nil {
		t.Fatalf("write ical file: %v", err)
	}

	// Unset the bare env vars so file resolution takes effect.
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("ICAL_SECRET")
	os.Setenv("DATABASE_URL_FILE", dbFile)
	os.Setenv("S3_ENDPOINT", "localhost:9000")
	os.Setenv("S3_ACCESS_KEY", "k")
	os.Setenv("S3_SECRET_KEY", "s")
	os.Setenv("ICAL_SECRET_FILE", icalFile)
	defer func() {
		os.Unsetenv("DATABASE_URL_FILE")
		os.Unsetenv("ICAL_SECRET_FILE")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("ICAL_SECRET")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected Load to succeed with _FILE env, got: %v", err)
	}
	if cfg.DatabaseURL != "postgres://klubhub:fromfile@db:5432/klubhub" {
		t.Errorf("DatabaseURL = %q, want file-derived value", cfg.DatabaseURL)
	}
	if cfg.ICALSecret != "file-ical-secret" {
		t.Errorf("ICALSecret = %q, want file-derived value", cfg.ICALSecret)
	}
}

func TestConfigLoadFileEnvErrorOnMissingFile(t *testing.T) {
	// If *_FILE points at a non-existent path, Load must fail closed
	// rather than silently treating the value as empty.
	os.Unsetenv("DATABASE_URL")
	os.Setenv("DATABASE_URL_FILE", "/no/such/file/exists/here")
	defer os.Unsetenv("DATABASE_URL_FILE")

	_, err := config.Load()
	if err == nil {
		t.Error("expected error when _FILE points at missing path, got nil")
	}
}

func TestConfigLoadFileEnvDoesNotOverwriteExplicitValue(t *testing.T) {
	// Explicit env var wins over file-derived value (ops might want to
	// override a docker-secret by exporting directly).
	dir := t.TempDir()
	dbFile := dir + "/db"
	if err := os.WriteFile(dbFile, []byte("postgres://fromfile"), 0o600); err != nil {
		t.Fatalf("write db file: %v", err)
	}
	os.Setenv("DATABASE_URL", "postgres://explicit")
	os.Setenv("DATABASE_URL_FILE", dbFile)
	os.Setenv("S3_ENDPOINT", "x")
	os.Setenv("S3_ACCESS_KEY", "k")
	os.Setenv("S3_SECRET_KEY", "s")
	os.Setenv("ICAL_SECRET", "x")
	defer func() {
		os.Unsetenv("DATABASE_URL_FILE")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DatabaseURL != "postgres://explicit" {
		t.Errorf("explicit DATABASE_URL should win, got %q", cfg.DatabaseURL)
	}
}

func TestConfigLoadSynthesizesDSNFromPostgresParts(t *testing.T) {
	// Plan C.3: when DATABASE_URL is empty but POSTGRES_* parts are
	// present, the loader builds the DSN (using the password loaded from
	// a file via the _FILE convention).
	dir := t.TempDir()
	pwFile := dir + "/pw"
	if err := os.WriteFile(pwFile, []byte("s3cr3t!"), 0o600); err != nil {
		t.Fatalf("write pw file: %v", err)
	}
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Setenv("POSTGRES_USER", "klubhub")
	os.Setenv("POSTGRES_PASSWORD_FILE", pwFile)
	os.Setenv("POSTGRES_HOST", "db")
	os.Setenv("POSTGRES_DB", "klubhub")
	os.Setenv("S3_ENDPOINT", "x")
	os.Setenv("S3_ACCESS_KEY", "k")
	os.Setenv("S3_SECRET_KEY", "s")
	os.Setenv("ICAL_SECRET", "x")
	defer func() {
		os.Unsetenv("POSTGRES_PASSWORD_FILE")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_DB")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// url.UserPassword escapes reserved chars; `!` becomes %21.
	if !strings.Contains(cfg.DatabaseURL, "://klubhub:s3cr3t%21@db:5432/klubhub") {
		t.Errorf("DatabaseURL missing expected parts; got %q", cfg.DatabaseURL)
	}
	if !strings.Contains(cfg.DatabaseURL, "sslmode=disable") {
		t.Errorf("DatabaseURL should default sslmode=disable for docker host; got %q", cfg.DatabaseURL)
	}
}

func TestConfigLoadRequiresEitherDSNOrPostgresParts(t *testing.T) {
	// If DATABASE_URL is empty AND POSTGRES_* are not set either, Load
	// must fail (the previous envconfig.Process pass would have already
	// failed on DATABASE_URL required:true).
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_DB")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Setenv("S3_ENDPOINT", "x")
	os.Setenv("S3_ACCESS_KEY", "k")
	os.Setenv("S3_SECRET_KEY", "s")
	os.Setenv("ICAL_SECRET", "x")
	_, err := config.Load()
	if err == nil {
		t.Error("expected error when neither DATABASE_URL nor POSTGRES_* parts are set")
	}
}

// setRequiredEnv sets the minimum env for config.Load and returns a cleanup.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/testdb")
	t.Setenv("S3_ENDPOINT", "localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "test-access")
	t.Setenv("S3_SECRET_KEY", "test-secret")
	t.Setenv("ICAL_SECRET", "test-ical-secret")
}

func TestConfigFeaturesDefaultOff(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("FEATURE_RA_IMPORT")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Features.RAImport {
		t.Error("expected Features.RAImport to default to false")
	}
	for name, on := range cfg.Features.Enabled() {
		if on {
			t.Errorf("expected edition feature %q to be off by default", name)
		}
	}
}

func TestConfigFeatureRAImportFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FEATURE_RA_IMPORT", "true")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !cfg.Features.RAImport {
		t.Error("expected FEATURE_RA_IMPORT=true to enable Features.RAImport")
	}
	if !cfg.Features.Enabled()["ra_import"] {
		t.Error("expected Enabled()[\"ra_import\"] to be true")
	}
}
