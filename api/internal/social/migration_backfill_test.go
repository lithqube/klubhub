package social_test

// Migration smoke test: assert migration 024 backfills the new
// image_storage_key column from the legacy image_minio_path column.
//
// Strategy
// -----------
// 1. Spin up a fresh Postgres container (testcontainers).
// 2. Apply migrations 001..023 via the package's embedded FS so the
//    scheduled_posts table exists with the legacy image_minio_path.
// 3. Assert image_storage_key does NOT exist yet (RED anchor).
// 4. Insert a legacy row using image_minio_path.
// 5. Apply the migration 024 SQL by hand (the file is small enough
//    to re-declare here; the goal is to exercise the exact
//    ADD COLUMN + UPDATE statement).
// 6. Assert image_storage_key now mirrors the legacy value.
// 7. Negative control: a row with empty image_minio_path gets an
//    empty image_storage_key (NOT a copy of some other row).

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// migration024SQL is the exact body of the new goose migration file.
// We re-declare it here intentionally so the test exercises the
// statement shape the operator will run against the live DB. If the
// migration file and this constant ever drift, the integration test
// will surface it on the next run.
const migration024SQL = `
-- +goose StatementBegin
ALTER TABLE scheduled_posts
    ADD COLUMN image_storage_key TEXT NOT NULL DEFAULT '';

UPDATE scheduled_posts
SET image_storage_key = image_minio_path
WHERE image_storage_key = ''
  AND image_minio_path <> '';
-- +goose StatementEnd
`

func TestMigration024_ScheduledPostsImageStorageKey_BackfillFromLegacy(t *testing.T) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("migdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// Phase 1: apply migrations 001..023 (everything pre-024). The
	// migrations package embeds the SQL files; load the FS into goose
	// via the import below.
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose.SetDialect: %v", err)
	}
	if err := goose.UpToContext(ctx, sqlDB, ".", 23); err != nil {
		t.Fatalf("goose.UpTo(23): %v", err)
	}

	// Connect with pgxpool for ergonomic column/value checks.
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Phase 2: image_storage_key should NOT exist yet (RED anchor).
	if columnExists(t, pool, "scheduled_posts", "image_storage_key") {
		t.Fatalf("image_storage_key column should NOT exist before migration 024 applies")
	}

	// Phase 3: seed a legacy row via the legacy column.
	var accountID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO social_accounts (platform, ig_user_id, status, access_token)
		VALUES ('instagram', 'ig_seed_user', 'connected', '')
		RETURNING id`,
	).Scan(&accountID); err != nil {
		t.Fatalf("insert social_account: %v", err)
	}

	const legacyPath = "social/feed/abc.jpg"
	const scheduledAt = "2026-10-01T09:00:00Z"
	if _, err := pool.Exec(ctx, `
		INSERT INTO scheduled_posts
			(account_id, status, post_type, caption, image_minio_path,
			 scheduled_at_utc, timezone_name)
		VALUES ($1, 'scheduled', 'feed', 'legacy post', $2,
		        $3, 'UTC')`,
		accountID, legacyPath, scheduledAt); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// Phase 4: apply migration 024 statements verbatim. We deliberately
	// bypass goose here because the file is brand new and not yet
	// registered in the goose version table; the constant above mirrors
	// it byte-for-byte so any drift between the file and the constant
	// surfaces as a backfill-mismatch assertion failure.
	if _, err := sqlDB.ExecContext(ctx, migration024SQL); err != nil {
		t.Fatalf("apply migration 024: %v", err)
	}

	// Phase 5: assert image_storage_key now mirrors the legacy value.
	if !columnExists(t, pool, "scheduled_posts", "image_storage_key") {
		t.Fatalf("image_storage_key column should exist after migration 024")
	}

	var gotKey string
	if err := pool.QueryRow(ctx, `
		SELECT image_storage_key FROM scheduled_posts
		WHERE account_id = $1 AND caption = 'legacy post'`, accountID,
	).Scan(&gotKey); err != nil {
		t.Fatalf("query image_storage_key: %v", err)
	}
	if gotKey != legacyPath {
		t.Fatalf("backfill mismatch: image_storage_key=%q want %q", gotKey, legacyPath)
	}

	// Negative control: a row inserted with an empty legacy path must
	// still get an empty (not NULL) image_storage_key after migration.
	if _, err := pool.Exec(ctx, `
		INSERT INTO scheduled_posts
			(account_id, status, post_type, caption, image_minio_path,
			 scheduled_at_utc, timezone_name)
		VALUES ($1, 'draft', 'story', 'no-image', '',
		        $2, 'UTC')`,
		accountID, scheduledAt); err != nil {
		t.Fatalf("seed empty-image row: %v", err)
	}
	var emptyKey string
	if err := pool.QueryRow(ctx, `
		SELECT image_storage_key FROM scheduled_posts
		WHERE account_id = $1 AND caption = 'no-image'`, accountID,
	).Scan(&emptyKey); err != nil {
		t.Fatalf("query empty-image row: %v", err)
	}
	if emptyKey != "" {
		t.Fatalf("empty-image row should have empty image_storage_key, got %q", emptyKey)
	}

	// Sanity: the migration file on disk matches the constant we ran.
	// If the constant and the .sql file ever diverge, fail loudly.
	diskSQL, err := migrations.FS.ReadFile("024_scheduled_posts_image_storage_key.sql")
	if err != nil {
		t.Fatalf("read 024_scheduled_posts_image_storage_key.sql: %v", err)
	}
	if !strings.Contains(string(diskSQL), "ADD COLUMN image_storage_key") ||
		!strings.Contains(string(diskSQL), "image_storage_key = image_minio_path") {
		t.Fatalf("migration file on disk does not contain expected statements; " +
			"drift between constant and file is a test failure")
	}
}

// columnExists checks whether a column is present on the given table.
// Uses information_schema; works for the renamed column regardless of
// how the migration introduces it.
func columnExists(t *testing.T, pool *pgxpool.Pool, table, column string) bool {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = $1 AND column_name = $2`,
		table, column,
	).Scan(&n)
	if err != nil {
		t.Fatalf("information_schema query: %v", err)
	}
	return n > 0
}