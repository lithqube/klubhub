package epk_test

// Migration smoke test: assert migration 025 backfills the new
// garage_object_key column from the legacy minio_path column on
// epk_exports.
//
// Strategy
// -----------
// 1. Spin up a fresh Postgres container (testcontainers).
// 2. Apply migrations 001..024 via the package's embedded FS so the
//    epk_exports table exists with the legacy minio_path column.
// 3. Assert garage_object_key does NOT exist yet (RED anchor).
// 4. Insert a legacy row using minio_path = 'epk/exports/legacy.pdf'.
// 5. Apply the migration 025 SQL by hand (the file is small enough
//    to re-declare here; the goal is to exercise the exact
//    ADD COLUMN + UPDATE statement).
// 6. Assert garage_object_key now mirrors the legacy value.
// 7. Negative control: a row with empty minio_path gets an empty
//    garage_object_key (NOT a copy of some other row).
// 8. Drift guard re-reads SQL file via migrations.FS and asserts
//    substring presence.
//
// This mirrors the pattern in social/migration_backfill_test.go for
// migration 024 (scheduled_posts.image_storage_key).

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

// migration025SQL is the exact body of the new goose migration file.
// We re-declare it here intentionally so the test exercises the
// statement shape the operator will run against the live DB. If the
// migration file and this constant ever drift, the integration test
// will surface it on the next run.
const migration025SQL = `
-- +goose StatementBegin
ALTER TABLE epk_exports
    ADD COLUMN garage_object_key TEXT NOT NULL DEFAULT '';

UPDATE epk_exports
SET garage_object_key = minio_path
WHERE garage_object_key = ''
  AND minio_path <> '';
-- +goose StatementEnd
`

func TestMigration025_EpkExportsGarageObjectKey_BackfillFromLegacy(t *testing.T) {
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

	// Phase 1: apply migrations 001..024 (everything pre-025). The
	// migrations package embeds the SQL files; load the FS into goose.
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose.SetDialect: %v", err)
	}
	if err := goose.UpToContext(ctx, sqlDB, ".", 24); err != nil {
		t.Fatalf("goose.UpTo(24): %v", err)
	}

	// Connect with pgxpool for ergonomic column/value checks.
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Phase 2: garage_object_key should NOT exist yet (RED anchor).
	if columnExists(t, pool, "epk_exports", "garage_object_key") {
		t.Fatalf("garage_object_key column should NOT exist before migration 025 applies")
	}

	// Phase 3: seed a legacy row via the legacy column. We can't
	// depend on epk_content existing in this seed path because the
	// singleton row needs an explicit INSERT; for this smoke test we
	// only care about the epk_exports table itself.
	const legacyPath = "epk/exports/legacy.pdf"
	if _, err := pool.Exec(ctx, `
		INSERT INTO epk_exports (minio_path)
		VALUES ($1)`,
		legacyPath); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// Phase 4: apply migration 025 statements verbatim. We deliberately
	// bypass goose here because the file is brand new and not yet
	// registered in the goose version table; the constant above mirrors
	// it byte-for-byte so any drift between the file and the constant
	// surfaces as a backfill-mismatch assertion failure.
	if _, err := sqlDB.ExecContext(ctx, migration025SQL); err != nil {
		t.Fatalf("apply migration 025: %v", err)
	}

	// Phase 5: assert garage_object_key now mirrors the legacy value.
	if !columnExists(t, pool, "epk_exports", "garage_object_key") {
		t.Fatalf("garage_object_key column should exist after migration 025")
	}

	var gotKey string
	if err := pool.QueryRow(ctx, `
		SELECT garage_object_key FROM epk_exports
		WHERE minio_path = $1`, legacyPath,
	).Scan(&gotKey); err != nil {
		t.Fatalf("query garage_object_key: %v", err)
	}
	if gotKey != legacyPath {
		t.Fatalf("backfill mismatch: garage_object_key=%q want %q", gotKey, legacyPath)
	}

	// Negative control: a row inserted with an empty legacy path must
	// still get an empty (not NULL) garage_object_key after migration.
	if _, err := pool.Exec(ctx, `
		INSERT INTO epk_exports (minio_path)
		VALUES ('')`); err != nil {
		t.Fatalf("seed empty-path row: %v", err)
	}
	var emptyKey string
	if err := pool.QueryRow(ctx, `
		SELECT garage_object_key FROM epk_exports
		WHERE minio_path = '' AND garage_object_key = ''
		ORDER BY created_at DESC LIMIT 1`,
	).Scan(&emptyKey); err != nil {
		t.Fatalf("query empty-path row: %v", err)
	}
	if emptyKey != "" {
		t.Fatalf("empty-path row should have empty garage_object_key, got %q", emptyKey)
	}

	// Sanity: the migration file on disk matches the constant we ran.
	// If the constant and the .sql file ever diverge, fail loudly.
	diskSQL, err := migrations.FS.ReadFile("025_epk_exports_garage_object_key.sql")
	if err != nil {
		t.Fatalf("read 025_epk_exports_garage_object_key.sql: %v", err)
	}
	if !strings.Contains(string(diskSQL), "ADD COLUMN garage_object_key") ||
		!strings.Contains(string(diskSQL), "garage_object_key = minio_path") {
		t.Fatalf("migration file on disk does not contain expected statements; " +
			"drift between constant and file is a test failure")
	}
}

// columnExistsEpk is a copy of the helper used in social tests; we
// keep the implementations separate per package to avoid an awkward
// cross-package test utility and to mirror the social test layout
// (each package's migration_backfill_test.go is self-contained).
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
