package settings_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/settings"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}
	defer container.Terminate(ctx) //nolint:errcheck

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("failed to get connection string: " + err.Error())
	}

	// Run migrations
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		panic("failed to open sql.DB: " + err.Error())
	}
	defer sqlDB.Close()

	if err := migrations.RunMigrations(sqlDB); err != nil {
		panic("failed to run migrations: " + err.Error())
	}

	// Create pgxpool for tests
	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		panic("failed to create pgxpool: " + err.Error())
	}
	defer testPool.Close()

	m.Run()
}

func clearSettings(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "DELETE FROM user_settings")
	if err != nil {
		t.Fatalf("failed to clear user_settings: %v", err)
	}
}

func TestGetOrCreate_EmptyTableInsertsRow(t *testing.T) {
	clearSettings(t)
	repo := settings.NewRepository(testPool)

	us, err := repo.GetOrCreate(context.Background())
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	if us.ID.String() == "" || us.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Errorf("expected non-nil UUID id, got %v", us.ID)
	}
}

func TestGetOrCreate_Idempotent(t *testing.T) {
	clearSettings(t)
	repo := settings.NewRepository(testPool)

	us1, err := repo.GetOrCreate(context.Background())
	if err != nil {
		t.Fatalf("first GetOrCreate failed: %v", err)
	}

	us2, err := repo.GetOrCreate(context.Background())
	if err != nil {
		t.Fatalf("second GetOrCreate failed: %v", err)
	}

	if us1.ID != us2.ID {
		t.Errorf("GetOrCreate not idempotent: got different UUIDs %v vs %v", us1.ID, us2.ID)
	}
}

func TestGet_AfterGetOrCreate_ReturnsSameRow(t *testing.T) {
	clearSettings(t)
	repo := settings.NewRepository(testPool)

	created, err := repo.GetOrCreate(context.Background())
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	fetched, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if created.ID != fetched.ID {
		t.Errorf("Get returned different UUID %v vs %v", created.ID, fetched.ID)
	}
}

func TestUpdate_WithCorrectTimestamp_Succeeds(t *testing.T) {
	clearSettings(t)
	repo := settings.NewRepository(testPool)

	us, err := repo.GetOrCreate(context.Background())
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	req := settings.UpdateSettingsRequest{
		DJName:    "Test DJ",
		UpdatedAt: us.UpdatedAt,
	}

	updated, err := repo.Update(context.Background(), req)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.DJName != "Test DJ" {
		t.Errorf("expected DJName=Test DJ, got %q", updated.DJName)
	}
}

func TestUpdate_WithStaleTimestamp_ReturnsErrConflict(t *testing.T) {
	clearSettings(t)
	repo := settings.NewRepository(testPool)

	us, err := repo.GetOrCreate(context.Background())
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// Provide a timestamp 1 second before the current updated_at
	staleTime := us.UpdatedAt.Add(-1 * time.Second)

	req := settings.UpdateSettingsRequest{
		DJName:    "Should Fail",
		UpdatedAt: staleTime,
	}

	_, err = repo.Update(context.Background(), req)
	if err != settings.ErrConflict {
		t.Errorf("expected ErrConflict for stale timestamp, got %v", err)
	}
}

func TestSoftDelete_GetReturnsErrNotFound(t *testing.T) {
	clearSettings(t)
	repo := settings.NewRepository(testPool)

	us, err := repo.GetOrCreate(context.Background())
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// Manually soft-delete the row
	_, err = testPool.Exec(
		context.Background(),
		"UPDATE user_settings SET deleted_at = NOW() WHERE id = $1",
		us.ID,
	)
	if err != nil {
		t.Fatalf("failed to soft-delete: %v", err)
	}

	// Get should now return ErrNotFound
	_, err = repo.Get(context.Background())
	if err != settings.ErrNotFound {
		t.Errorf("expected ErrNotFound after soft-delete, got %v", err)
	}
}
