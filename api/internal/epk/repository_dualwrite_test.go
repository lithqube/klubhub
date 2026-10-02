package epk_test

// Regression test: InsertExport must write the canonical key into BOTH
// the new garage_object_key column AND the legacy minio_path column
// during the rollback window. CodeRabbit flagged that the original
// implementation only wrote the new column — so a rollback of
// migration 025 (which drops garage_object_key) would leave the
// previous release with an empty minio_path on every newly-created
// export. Dual-write keeps both columns populated so the previous
// release still finds the object key in the legacy column.
//
// Strategy
// ---------
// 1. Stand up a real Postgres container (testcontainers).
// 2. Apply migrations 001..025 via the embedded migrations.FS.
// 3. Insert an export via Repository.InsertExport.
// 4. Assert the row's minio_path and garage_object_key both equal
//    the canonical key passed in. Before the fix, minio_path is
//    '' — that is the RED anchor.
// 5. Assert the returned EPKExport.GarageObjectKey still equals the
//    canonical key (Scan contract preserved).

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/epk"
	"github.com/klubhub/dj/api/internal/platform/migrations"
)

func TestRepository_InsertExport_DualWritesBothColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testcontainer test in -short mode")
	}

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("dualwritedb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
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

	// Apply all migrations so the table exists with both columns.
	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, migrations.RunMigrations(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	repo := epk.NewRepository(pool)

	const canonical = "epk/exports/dualwrite.pdf"
	inserted, err := repo.InsertExport(ctx, canonical)
	require.NoError(t, err)
	require.NotNil(t, inserted)

	// Sanity: returned struct has the canonical key.
	assert.Equal(t, canonical, inserted.GarageObjectKey)

	// Persistence guarantee: BOTH columns contain the canonical key.
	// Before the fix minio_path is "" (RED). After the fix both
	// equal the canonical key (GREEN).
	var minioPath, garageKey string
	err = pool.QueryRow(ctx, `
		SELECT minio_path, garage_object_key
		FROM epk_exports
		WHERE id = $1`, inserted.ID,
	).Scan(&minioPath, &garageKey)
	require.NoError(t, err)

	assert.Equal(t, canonical, garageKey,
		"garage_object_key should hold the canonical key")
	assert.Equal(t, canonical, minioPath,
		"legacy minio_path must also hold the canonical key during "+
			"the rollback window — dropping migration 025 must not "+
			"leave the previous release with an empty key")
}