package tenantdb_test

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

var (
	ownerPool *pgxpool.Pool // schema owner / superuser: fixtures only
	appPool   *pgxpool.Pool // klubhub_app: what the promoter binary uses at runtime
)

func TestMain(m *testing.M) {
	flag.Parse()
	db, err := pgtest.StartPromoter()
	if err != nil {
		panic(err)
	}
	if db != nil {
		ownerPool, appPool = db.Owner, db.App
	}
	code := m.Run()
	db.Close()
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
	createOrg(t, a, "tenant-a-"+a.String()[24:])
	createOrg(t, b, "tenant-b-"+b.String()[24:])

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
	createOrg(t, a, "writer-"+a.String()[24:])
	err := tenantdb.WithTenant(context.Background(), appPool, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO organizations (id, name, slug) VALUES ($1, 'x', $2)`, other, "intruder-"+other.String()[24:])
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
	createOrg(t, a, "leak-"+a.String()[24:])
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
			`INSERT INTO organizations (id, name, slug) VALUES ($1, 'x', $2)`, a, "rollback-"+a.String()[24:]); err != nil {
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
	createOrg(t, a, "ctx-"+a.String()[24:])
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
