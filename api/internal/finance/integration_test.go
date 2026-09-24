package finance

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/migrations"
)

var (
	testPool     *pgxpool.Pool
	testDB       *sql.DB
	testTeardown func()
)

func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("SKIP_INTEGRATION") == "1" || testing.Short() {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		tcpostgres.WithDatabase("klubhub_test"),
		tcpostgres.WithUsername("klubhub"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		println("testcontainers: cannot start postgres, skipping integration:", err.Error())
		os.Exit(m.Run())
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		println("testcontainers: connection string:", err.Error())
		os.Exit(1)
	}
	testDB, err = sql.Open("pgx", connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("sql.Open:", err.Error())
		os.Exit(1)
	}
	if err := migrations.RunMigrations(testDB); err != nil {
		_ = testDB.Close()
		_ = container.Terminate(ctx)
		println("RunMigrations:", err.Error())
		os.Exit(1)
	}
	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		_ = testDB.Close()
		_ = container.Terminate(ctx)
		println("pgxpool.New:", err.Error())
		os.Exit(1)
	}
	testTeardown = func() {
		testPool.Close()
		testDB.Close()
		_ = container.Terminate(context.Background())
	}

	code := m.Run()
	if testTeardown != nil {
		testTeardown()
	}
	os.Exit(code)
}

func TestIntegration_GetSeedsEmptyRow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}

	repo := NewRepository(testPool)
	first, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if first.LegalName != "" {
		t.Fatalf("expected empty seeded profile, got legal_name=%q", first.LegalName)
	}

	second, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("seed produced a new row on second Get: first=%v second=%v", first.ID, second.ID)
	}
}

func TestIntegration_UpdateHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}

	repo := NewRepository(testPool)
	ctx := context.Background()
	seeded, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	req := UpdateBillingProfileRequest{
		LegalName:       "Test Billing",
		EntityKind:      EntityKindSoleTrader,
		TaxIDKind:       TaxIDKindVAT,
		TaxID:           "DE123456789",
		ContactEmail:    "billing@example.com",
		AddressLine1:    "1 Sample Street",
		AddressCity:     "Berlin",
		AddressPostal:   "10115",
		AddressCountry:  "DE",
		Jurisdiction:    "DE",
		DefaultCurrency: "EUR",
		UpdatedAt:       seeded.UpdatedAt,
	}
	updated, err := repo.Update(ctx, &req)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.LegalName != "Test Billing" {
		t.Fatalf("LegalName: %q", updated.LegalName)
	}
	if updated.EntityKind != EntityKindSoleTrader {
		t.Fatalf("EntityKind: %q", updated.EntityKind)
	}
	if updated.DefaultCurrency != "EUR" {
		t.Fatalf("DefaultCurrency: %q", updated.DefaultCurrency)
	}
	if !updated.UpdatedAt.After(seeded.UpdatedAt) {
		t.Fatalf("UpdatedAt did not advance: %v -> %v", seeded.UpdatedAt, updated.UpdatedAt)
	}
}

func TestIntegration_StaleTokenConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}

	repo := NewRepository(testPool)
	ctx := context.Background()
	seeded, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Update once to advance the token.
	req := UpdateBillingProfileRequest{
		LegalName:       "First Writer",
		EntityKind:      EntityKindIndividual,
		ContactEmail:    "a@example.com",
		AddressLine1:    "1",
		AddressCity:     "Berlin",
		AddressPostal:   "10115",
		Jurisdiction:    "DE",
		DefaultCurrency: "EUR",
		UpdatedAt:       seeded.UpdatedAt,
	}
	if _, err := repo.Update(ctx, &req); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	// Second update with the original token must ErrConflict and not mutate.
	stale := UpdateBillingProfileRequest{
		LegalName:       "Stale Writer",
		EntityKind:      EntityKindIndividual,
		ContactEmail:    "b@example.com",
		AddressLine1:    "2",
		AddressCity:     "Berlin",
		AddressPostal:   "10115",
		Jurisdiction:    "DE",
		DefaultCurrency: "EUR",
		UpdatedAt:       seeded.UpdatedAt,
	}
	if _, err := repo.Update(ctx, &stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict on stale token, got %v", err)
	}

	// Confirm the first writer's value is still there.
	current, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get after stale: %v", err)
	}
	if current.LegalName != "First Writer" {
		t.Fatalf("stale PUT mutated legal_name to %q", current.LegalName)
	}
}
