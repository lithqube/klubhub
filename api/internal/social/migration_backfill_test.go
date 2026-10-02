package social_test

// Migration smoke test: assert migration 024 backfills the new
// image_storage_key column from the legacy image_minio_path column.
//
// Strategy
// ---------
// 1. Spin up a fresh Postgres container (testcontainers).
// 2. Apply migrations 001..023 via the package's embedded FS so the
//    scheduled_posts table exists with the legacy image_minio_path.
// 3. Assert image_storage_key does NOT exist yet (RED anchor).
// 4. Insert a legacy row using image_minio_path.
// 5. Apply migration 024 via goose.UpToContext(N+1) — the same
//    .sql file the operator runs against the live DB. This is the
//    real run path; the previous constant mirror was a drift risk
//    that did not prove equivalence.
// 6. Assert image_storage_key now mirrors the legacy value.
// 7. Negative control: a row with empty image_minio_path gets an
//    empty image_storage_key (NOT a copy of some other row).
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
	require.NoError(t, err)

	// Phase 1: apply migrations 001..023 (everything pre-024) via
	// goose against the real .sql files embedded in the migrations
	// package. This is the exact path the operator hits in production.
	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpToContext(ctx, sqlDB, ".", 23))

	// Connect with pgxpool for ergonomic column/value checks.
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	// Phase 2: image_storage_key should NOT exist yet (RED anchor).
	if columnExists(t, pool, "scheduled_posts", "image_storage_key") {
		t.Fatalf("image_storage_key column should NOT exist before migration 024 applies")
	}

	// Phase 3: seed a legacy row via the legacy column.
	var accountID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO social_accounts (platform, ig_user_id, status, access_token)
		VALUES ('instagram', 'ig_seed_user', 'connected', '')
		RETURNING id`,
	).Scan(&accountID))

	const legacyPath = "social/feed/abc.jpg"
	const scheduledAt = "2026-10-01T09:00:00Z"
	var legacyID, emptyID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO scheduled_posts
			(account_id, status, post_type, caption, image_minio_path,
			 scheduled_at_utc, timezone_name)
		VALUES ($1, 'scheduled', 'feed', 'legacy post', $2,
		        $3, 'UTC') RETURNING id`,
		accountID, legacyPath, scheduledAt).Scan(&legacyID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO scheduled_posts
			(account_id, status, post_type, caption, image_minio_path,
			 scheduled_at_utc, timezone_name)
		VALUES ($1, 'draft', 'story', 'no-image', '', $2, 'UTC') RETURNING id`,
		accountID, scheduledAt).Scan(&emptyID))

	// Phase 4: apply migration 024 via goose. This is the real run
	// path — the same .sql file is exercised here as the operator
	// runs against the live DB. The previous constant mirror was a
	// drift risk; we now depend on goose to apply the actual file.
	require.NoError(t, goose.UpToContext(ctx, sqlDB, ".", 24))

	// Phase 5: assert image_storage_key now mirrors the legacy value.
	if !columnExists(t, pool, "scheduled_posts", "image_storage_key") {
		t.Fatalf("image_storage_key column should exist after migration 024")
	}

	var gotKey string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT image_storage_key FROM scheduled_posts
		WHERE id = $1`, legacyID,
	).Scan(&gotKey))
	if gotKey != legacyPath {
		t.Fatalf("backfill mismatch: image_storage_key=%q want %q", gotKey, legacyPath)
	}

	// Negative control: a row inserted with an empty legacy path must
	// still get an empty (not NULL) image_storage_key after migration.
	var emptyKey string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT image_storage_key FROM scheduled_posts
		WHERE id = $1`, emptyID,
	).Scan(&emptyKey))
	if emptyKey != "" {
		t.Fatalf("empty-image row should have empty image_storage_key, got %q", emptyKey)
	}

	// Drift guard: re-read the SQL file via migrations.FS and assert
	// substring presence of the ADD COLUMN and the backfill UPDATE.
	// This catches a future rename of the new column without
	// re-running the full migration (the canonical run path is
	// goose.UpToContext above).
	diskSQL, err := migrations.FS.ReadFile("024_scheduled_posts_image_storage_key.sql")
	require.NoError(t, err)
	if !strings.Contains(string(diskSQL), "ADD COLUMN image_storage_key") ||
		!strings.Contains(string(diskSQL), "image_storage_key = image_minio_path") {
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
