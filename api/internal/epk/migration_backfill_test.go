package epk_test

// Migration smoke test: assert migration 025 backfills the new
// garage_object_key column from the legacy minio_path column on
// epk_exports.
//
// Strategy
// ---------
// 1. Spin up a fresh Postgres container (testcontainers).
// 2. Apply migrations 001..024 via the package's embedded FS so the
//    epk_exports table exists with the legacy minio_path column.
// 3. Assert garage_object_key does NOT exist yet (RED anchor).
// 4. Insert a legacy row using minio_path = 'epk/exports/legacy.pdf'.
// 5. Apply migration 025 via goose.UpToContext(N+1) — the same
//    .sql file the operator runs against the live DB. This is the
//    real run path; the previous constant mirror was a drift risk
//    that did not prove equivalence.
// 6. Assert garage_object_key now mirrors the legacy value.
// 7. Negative control: a row with empty minio_path gets an empty
//    garage_object_key (NOT a copy of some other row).
// 8. Drift guard re-reads the .sql file via migrations.FS and asserts
//    substring presence of ADD COLUMN <new_col> + the legacy copy
//    statement. The substring check is a sanity net, not the actual
//    run path.

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
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

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
	require.NoError(t, err)

	// Phase 1: apply migrations 001..024 (everything pre-025) via
	// goose against the real .sql files embedded in the migrations
	// package. This is the exact path the operator hits in production.
	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpToContext(ctx, sqlDB, ".", 24))

	// Connect with pgxpool for ergonomic column/value checks.
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	// Phase 2: garage_object_key should NOT exist yet (RED anchor).
	if columnExists(t, pool, "epk_exports", "garage_object_key") {
		t.Fatalf("garage_object_key column should NOT exist before migration 025 applies")
	}

	// Phase 3: seed a legacy row via the legacy column.
	const legacyPath = "epk/exports/legacy.pdf"
	var legacyID, emptyID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO epk_exports (minio_path) VALUES ($1) RETURNING id`, legacyPath,
	).Scan(&legacyID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO epk_exports (minio_path) VALUES ('') RETURNING id`,
	).Scan(&emptyID))

	// Phase 4: apply migration 025 via goose. This is the real run
	// path — the same .sql file is exercised here as the operator
	// runs against the live DB. The previous constant mirror was a
	// drift risk; we now depend on goose to apply the actual file.
	require.NoError(t, goose.UpToContext(ctx, sqlDB, ".", 25))

	// Phase 5: assert garage_object_key now mirrors the legacy value.
	if !columnExists(t, pool, "epk_exports", "garage_object_key") {
		t.Fatalf("garage_object_key column should exist after migration 025")
	}

	var gotKey string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT garage_object_key FROM epk_exports
		WHERE id = $1`, legacyID,
	).Scan(&gotKey))
	if gotKey != legacyPath {
		t.Fatalf("backfill mismatch: garage_object_key=%q want %q", gotKey, legacyPath)
	}

	// Negative control: a row inserted with an empty legacy path must
	// still get an empty (not NULL) garage_object_key after migration.
	var emptyKey string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT garage_object_key FROM epk_exports
		WHERE id = $1`, emptyID,
	).Scan(&emptyKey))
	if emptyKey != "" {
		t.Fatalf("empty-path row should have empty garage_object_key, got %q", emptyKey)
	}

	// Drift guard: re-read the SQL file via migrations.FS and assert
	// substring presence of the ADD COLUMN and the backfill UPDATE.
	// This catches a future rename of the new column without
	// re-running the full migration (the canonical run path is
	// goose.UpToContext above).
	diskSQL, err := migrations.FS.ReadFile("025_epk_exports_garage_object_key.sql")
	require.NoError(t, err)
	if !strings.Contains(string(diskSQL), "ADD COLUMN garage_object_key") ||
		!strings.Contains(string(diskSQL), "garage_object_key = minio_path") {
		t.Fatalf("migration file on disk does not contain expected statements; " +
			"drift between file and assertion is a test failure")
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
