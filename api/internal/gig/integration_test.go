// Package gig_test — Plan C / v1.0.0 integration smoke test.
//
// Exercises the full gig lifecycle against a real Postgres (testcontainers)
// and runs the embedded goose migrations to validate the renamed
// 006..010_* numeric-prefixed files (Plan B.1) actually apply.
//
// Run with `go test ./...` (no -short). -short skips it.
package gig_test

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/gig"
	"github.com/klubhub/dj/api/internal/platform/migrations"
)

var (
	testPool     *pgxpool.Pool
	testDB       *sql.DB
	testTeardown func()
)

func TestMain(m *testing.M) {
	// TestMain runs before the testing package parses flags. Parse them
	// explicitly so -short really skips container startup.
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
		// No Docker — print and skip cleanly so contributors without
		// a local Docker socket can still run `go test -short ./...`.
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

func TestIntegration_FullGigLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := gig.NewRepository(testPool)
	svc := gig.NewService(repo, nil, nil, nil, nil)

	fee, _ := decimal.NewFromString("250.00")
	in := &gig.GigCreate{
		Date:        time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC),
		Venue:       "Test venue",
		City:        "Berlin",
		Country:     "DE",
		EventName:   "Test event",
		FeeAmount:   fee,
		FeeCurrency: "EUR",
	}

	created, err := svc.CreateGig(ctx, in)
	if err != nil {
		t.Fatalf("CreateGig: %v", err)
	}
	if created.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("CreateGig returned nil UUID")
	}

	got, err := svc.GetGig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetGig: %v", err)
	}
	if got.Venue != "Test venue" || got.EventName != "Test event" {
		t.Fatalf("GetGig returned wrong gig: %+v", got)
	}

	city := "Berlin"
	list, err := svc.ListGigs(ctx, gig.GigFilter{City: &city})
	if err != nil {
		t.Fatalf("ListGigs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListGigs returned %d, want 1", len(list))
	}

	played := gig.GigStatusPlayed
	updated, err := svc.UpdateGig(ctx, created.ID, &gig.GigUpdate{
		Status:    &played,
		UpdatedAt: got.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("UpdateGig to played: %v", err)
	}
	if updated.Status != gig.GigStatusPlayed {
		t.Fatalf("UpdateGig status: %s, want %s", updated.Status, gig.GigStatusPlayed)
	}

	cancelled := gig.GigStatusCancelled
	updated, err = svc.UpdateGig(ctx, created.ID, &gig.GigUpdate{
		Status:    &cancelled,
		UpdatedAt: updated.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("UpdateGig to cancelled: %v", err)
	}

	confirmed := gig.GigStatusConfirmed
	_, err = svc.UpdateGig(ctx, created.ID, &gig.GigUpdate{
		Status:    &confirmed,
		UpdatedAt: updated.UpdatedAt,
	})
	if err == nil {
		t.Fatal("UpdateGig cancelled -> confirmed must be rejected (cancelled is terminal)")
	}
	if !strings.Contains(err.Error(), "forbidden") &&
		!errors.Is(err, gig.ErrForbidden) {
		t.Fatalf("expected forbidden error from cancelled re-open, got: %v", err)
	}

	if err := svc.DeleteGig(ctx, created.ID); err != nil {
		t.Fatalf("DeleteGig: %v", err)
	}

	list, err = svc.ListGigs(ctx, gig.GigFilter{City: &city})
	if err != nil {
		t.Fatalf("ListGigs after delete: %v", err)
	}
	for _, g := range list {
		if g.ID == created.ID {
			t.Fatal("deleted gig still returned by ListGigs")
		}
	}
}

func TestIntegration_MigrationsRunClean(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testDB == nil {
		t.Skip("postgres unavailable")
	}
	// All migrations already ran in TestMain. Just assert that the
	// expected tables exist.
	rows, err := testDB.Query(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public'
		ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables[name] = true
	}

	expected := []string{"gigs", "venues", "contacts"}
	for _, want := range expected {
		if !tables[want] {
			t.Errorf("expected table %q to exist after migrations; got: %v", want, tables)
		}
	}
}
