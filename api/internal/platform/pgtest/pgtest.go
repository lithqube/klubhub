// Package pgtest starts a disposable Postgres (testcontainers) with the
// Promoter schema for integration tests. It returns two pools: the schema
// owner (fixtures only) and klubhub_app, the restricted runtime role that
// row level security applies to. Tests should use App for anything that
// exercises production code paths.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/promoter/migrations"
)

// DB is a running Promoter test database.
type DB struct {
	Owner *pgxpool.Pool
	App   *pgxpool.Pool
	// Relay is klubhub_relay: BYPASSRLS, but granted only the outbox.
	Relay *pgxpool.Pool
	stop  func()
}

// Close stops the container and closes the pools.
func (d *DB) Close() {
	if d != nil && d.stop != nil {
		d.stop()
	}
}

const appPassword = "app-test-password"

// StartPromoter returns nil (and no error) when integration tests are
// disabled (-short or SKIP_INTEGRATION=1) or Docker is unavailable, so
// callers can skip cleanly. Call it from TestMain after flag.Parse().
func StartPromoter() (*DB, error) {
	if os.Getenv("SKIP_INTEGRATION") == "1" || testing.Short() {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("promoter_test"),
		tcpostgres.WithUsername("klubhub"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		println("pgtest: cannot start postgres, skipping integration:", err.Error())
		return nil, nil
	}
	fail := func(err error) (*DB, error) {
		_ = container.Terminate(context.Background())
		return nil, err
	}
	ownerDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fail(err)
	}
	db, err := sql.Open("pgx", ownerDSN)
	if err != nil {
		return fail(err)
	}
	defer db.Close()
	if err := migrations.Up(ctx, db); err != nil {
		return fail(fmt.Errorf("migrations: %w", err))
	}
	// What the setup script does from a secret: let the runtime role log in.
	for _, stmt := range []string{
		"ALTER ROLE klubhub_app LOGIN PASSWORD '" + appPassword + "'",
		"ALTER ROLE klubhub_relay LOGIN PASSWORD '" + appPassword + "'",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fail(err)
		}
	}
	owner, err := pgxpool.New(ctx, ownerDSN)
	if err != nil {
		return fail(err)
	}
	app, err := pgxpool.New(ctx, strings.Replace(ownerDSN, "klubhub:test@", "klubhub_app:"+appPassword+"@", 1))
	if err != nil {
		owner.Close()
		return fail(err)
	}
	relay, err := pgxpool.New(ctx, strings.Replace(ownerDSN, "klubhub:test@", "klubhub_relay:"+appPassword+"@", 1))
	if err != nil {
		app.Close()
		owner.Close()
		return fail(err)
	}
	return &DB{Owner: owner, App: app, Relay: relay, stop: func() {
		relay.Close()
		app.Close()
		owner.Close()
		_ = container.Terminate(context.Background())
	}}, nil
}
