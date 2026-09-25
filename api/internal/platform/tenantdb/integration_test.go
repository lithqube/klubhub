package tenantdb_test

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/migrations"
)

var (
	ownerPool *pgxpool.Pool // schema owner / superuser: migrations and fixtures only
	appPool   *pgxpool.Pool // klubhub_app: what the promoter binary uses at runtime
)

const appPassword = "app-test-password"

func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("SKIP_INTEGRATION") == "1" || testing.Short() {
		os.Exit(m.Run())
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
		println("testcontainers: cannot start postgres, skipping integration:", err.Error())
		os.Exit(m.Run())
	}
	ownerDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	db, err := sql.Open("pgx", ownerDSN)
	if err != nil {
		panic(err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		panic(fmt.Sprintf("migrations: %v", err))
	}
	// What the setup script does from a secret: allow the runtime role to log in.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER ROLE klubhub_app LOGIN PASSWORD '%s'", appPassword)); err != nil {
		panic(err)
	}
	_ = db.Close()

	ownerPool, err = pgxpool.New(ctx, ownerDSN)
	if err != nil {
		panic(err)
	}
	appDSN := strings.Replace(ownerDSN, "klubhub:test@", "klubhub_app:"+appPassword+"@", 1)
	appPool, err = pgxpool.New(ctx, appDSN)
	if err != nil {
		panic(err)
	}

	code := m.Run()
	appPool.Close()
	ownerPool.Close()
	_ = container.Terminate(context.Background())
	os.Exit(code)
}

func needDB(t *testing.T) {
	t.Helper()
	if testing.Short() || appPool == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
}

func createOrg(t *testing.T, id uuid.UUID, slug string) {
	t.Helper()
	err := tenantdb.WithTenant(context.Background(), appPool, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $3)`, id, slug, slug)
		return err
	})
	if err != nil {
		t.Fatalf("create org %s: %v", slug, err)
	}
}

func TestAppRoleIsRestricted(t *testing.T) {
	needDB(t)
	if err := tenantdb.AssertRuntimeRole(context.Background(), appPool); err != nil {
		t.Fatalf("app role should pass the runtime check: %v", err)
	}
	err := tenantdb.AssertRuntimeRole(context.Background(), ownerPool)
	if !errors.Is(err, tenantdb.ErrPrivilegedRole) {
		t.Fatalf("superuser pool must be refused, got %v", err)
	}
}

func TestTenantSeesOnlyItsOwnRows(t *testing.T) {
	needDB(t)
	a, b := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	createOrg(t, a, "tenant-a-"+a.String()[:8])
	createOrg(t, b, "tenant-b-"+b.String()[:8])

	var ids []uuid.UUID
	err := tenantdb.WithTenant(context.Background(), appPool, a, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT id FROM organizations`)
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != a {
		t.Fatalf("tenant A must see exactly its own row, got %v", ids)
	}
}

func TestWriteIntoAnotherTenantIsRejected(t *testing.T) {
	needDB(t)
	a, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	createOrg(t, a, "writer-"+a.String()[:8])
	err := tenantdb.WithTenant(context.Background(), appPool, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO organizations (id, name, slug) VALUES ($1, 'x', $2)`, other, "intruder-"+other.String()[:8])
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Fatalf("cross-tenant insert must violate RLS, got %v", err)
	}
}

func TestQueryWithoutTenantFailsClosed(t *testing.T) {
	needDB(t)
	// A raw query on the app pool (bypassing WithTenant) must error, not return rows.
	var n int
	err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM organizations`).Scan(&n)
	if err == nil {
		t.Fatalf("query without app.tenant_id must fail, returned count=%d", n)
	}
	// And after a WithTenant transaction, the pooled connection must not keep the tenant.
	a := uuid.Must(uuid.NewV7())
	createOrg(t, a, "leak-"+a.String()[:8])
	for i := 0; i < 5; i++ {
		if err := appPool.QueryRow(context.Background(), `SELECT count(*) FROM organizations`).Scan(&n); err == nil {
			t.Fatalf("tenant setting leaked to a pooled connection (count=%d)", n)
		}
	}
}

func TestNilTenantIsRejectedBeforeTheDatabase(t *testing.T) {
	needDB(t)
	called := false
	err := tenantdb.WithTenant(context.Background(), appPool, uuid.Nil, func(pgx.Tx) error {
		called = true
		return nil
	})
	if !errors.Is(err, tenantdb.ErrNoTenant) || called {
		t.Fatalf("nil tenant must be rejected without running fn, got err=%v called=%v", err, called)
	}
}

func TestErrorRollsBack(t *testing.T) {
	needDB(t)
	a := uuid.Must(uuid.NewV7())
	boom := errors.New("boom")
	err := tenantdb.WithTenant(context.Background(), appPool, a, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO organizations (id, name, slug) VALUES ($1, 'x', $2)`, a, "rollback-"+a.String()[:8]); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	var n int
	_ = tenantdb.WithTenant(context.Background(), appPool, a, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM organizations`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("insert must be rolled back, found %d rows", n)
	}
}

func TestTenantFromContext(t *testing.T) {
	needDB(t)
	a := uuid.Must(uuid.NewV7())
	createOrg(t, a, "ctx-"+a.String()[:8])
	ctx := tenantdb.ContextWithTenant(context.Background(), a)
	var got uuid.UUID
	if err := tenantdb.Run(ctx, appPool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM organizations`).Scan(&got)
	}); err != nil {
		t.Fatal(err)
	}
	if got != a {
		t.Fatalf("want %s, got %s", a, got)
	}
	if err := tenantdb.Run(context.Background(), appPool, func(pgx.Tx) error { return nil }); !errors.Is(err, tenantdb.ErrNoTenant) {
		t.Fatalf("context without tenant must be rejected, got %v", err)
	}
}
