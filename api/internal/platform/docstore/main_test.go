package docstore

import (
	"context"
	"database/sql"
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

// Real-Postgres coverage for the documents table: version sequencing,
// per-kind "current" tracking and the immutability trigger. The DJ
// migrations create the documents table this package persists to.

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("SKIP_INTEGRATION") == "1" || testing.Short() {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		tcpostgres.WithDatabase("klubhub_docstore_test"),
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
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("sql.Open:", err.Error())
		os.Exit(1)
	}
	if err := migrations.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		println("RunMigrations:", err.Error())
		os.Exit(1)
	}
	_ = db.Close()
	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("pgxpool.New:", err.Error())
		os.Exit(1)
	}

	code := m.Run()
	testPool.Close()
	_ = container.Terminate(context.Background())
	os.Exit(code)
}

func requirePG(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
}
