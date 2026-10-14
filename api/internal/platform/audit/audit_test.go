package audit_test

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

var testDB *pgtest.DB

func TestMain(m *testing.M) {
	flag.Parse()
	db, err := pgtest.StartPromoter()
	if err != nil {
		panic(err)
	}
	testDB = db
	code := m.Run()
	db.Close()
	os.Exit(code)
}

func needDB(t *testing.T) {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
}

func createOrg(t *testing.T, id uuid.UUID, slug string) {
	t.Helper()
	err := tenantdb.WithTenant(context.Background(), testDB.App, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $3)`, id, slug, slug)
		return err
	})
	if err != nil {
		t.Fatalf("create org %s: %v", slug, err)
	}
}

// record runs audit.Record inside a tenant-scoped transaction, the same way
// the promoter services call it.
func record(t *testing.T, tenant uuid.UUID, e audit.Entry) {
	t.Helper()
	err := tenantdb.WithTenant(context.Background(), testDB.App, tenant, func(tx pgx.Tx) error {
		return audit.Record(context.Background(), tx, tenant, e)
	})
	if err != nil {
		t.Fatalf("audit.Record failed: %v", err)
	}
}

type storedRow struct {
	TenantID  uuid.UUID
	ActorType string
	ActorID   string
	Action    string
	Resource  string
	Decision  string
	Reason    *string
}

func readAll(t *testing.T) []storedRow {
	t.Helper()
	rows, err := testDB.Owner.Query(context.Background(),
		`SELECT tenant_id, actor_type, actor_id, action, resource, decision, reason FROM audit_log ORDER BY occurred_at, id`)
	if err != nil {
		t.Fatalf("read audit_log: %v", err)
	}
	defer rows.Close()
	var out []storedRow
	for rows.Next() {
		var r storedRow
		if err := rows.Scan(&r.TenantID, &r.ActorType, &r.ActorID, &r.Action, &r.Resource, &r.Decision, &r.Reason); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	return out
}

func clearAuditLog(t *testing.T) {
	t.Helper()
	if _, err := testDB.Owner.Exec(context.Background(), "TRUNCATE audit_log, organizations CASCADE"); err != nil {
		t.Fatalf("clear: %v", err)
	}
}

func TestRecord_ActorTypeMapping(t *testing.T) {
	needDB(t)
	clearAuditLog(t)
	tenant := uuid.New()
	createOrg(t, tenant, "audit-actor-type")

	cases := []struct {
		actorID string
		want    string
	}{
		{"device:" + uuid.NewString(), "device"},
		{"local:" + uuid.NewString(), "user"},
		{"zitadel:auth0|abc123", "user"},
		{"system", "system"},
		{"", "system"},
		{"unknown-prefix:xyz", "system"},
	}
	for i, tc := range cases {
		record(t, tenant, audit.Entry{
			ActorID:  tc.actorID,
			Action:   "test.action",
			Resource: "test:" + string(rune('a'+i)),
			Allowed:  true,
		})
	}

	rows := readAll(t)
	if len(rows) != len(cases) {
		t.Fatalf("stored %d rows, want %d", len(rows), len(cases))
	}
	for i, tc := range cases {
		if rows[i].ActorType != tc.want {
			t.Errorf("actor %q: actor_type = %q, want %q", tc.actorID, rows[i].ActorType, tc.want)
		}
		if rows[i].ActorID != tc.actorID {
			t.Errorf("actor_id = %q, want %q (stored verbatim)", rows[i].ActorID, tc.actorID)
		}
		if rows[i].TenantID != tenant {
			t.Errorf("tenant_id = %v, want %v", rows[i].TenantID, tenant)
		}
	}
}

func TestRecord_DecisionMapping(t *testing.T) {
	needDB(t)
	clearAuditLog(t)
	tenant := uuid.New()
	createOrg(t, tenant, "audit-decision")

	record(t, tenant, audit.Entry{ActorID: "system", Action: "a.allow", Resource: "r:1", Allowed: true})
	record(t, tenant, audit.Entry{ActorID: "system", Action: "a.deny", Resource: "r:2", Allowed: false})

	rows := readAll(t)
	if len(rows) != 2 {
		t.Fatalf("stored %d rows, want 2", len(rows))
	}
	if rows[0].Decision != "allow" {
		t.Errorf("Allowed=true: decision = %q, want allow", rows[0].Decision)
	}
	if rows[1].Decision != "deny" {
		t.Errorf("Allowed=false: decision = %q, want deny", rows[1].Decision)
	}
}

func TestRecord_ReasonNullable(t *testing.T) {
	needDB(t)
	clearAuditLog(t)
	tenant := uuid.New()
	createOrg(t, tenant, "audit-reason")

	record(t, tenant, audit.Entry{ActorID: "system", Action: "a.no-reason", Resource: "r:1", Allowed: true})
	record(t, tenant, audit.Entry{ActorID: "system", Action: "a.with-reason", Resource: "r:2", Allowed: false, Reason: "pin mismatch"})

	rows := readAll(t)
	if len(rows) != 2 {
		t.Fatalf("stored %d rows, want 2", len(rows))
	}
	if rows[0].Reason != nil {
		t.Errorf("empty reason: stored %q, want NULL", *rows[0].Reason)
	}
	if rows[1].Reason == nil || *rows[1].Reason != "pin mismatch" {
		t.Errorf("reason = %v, want \"pin mismatch\"", rows[1].Reason)
	}
}

// The audit log is tenant-scoped by RLS: a row recorded for tenant A must be
// invisible to a klubhub_app session scoped to tenant B.
func TestRecord_TenantIsolation(t *testing.T) {
	needDB(t)
	clearAuditLog(t)
	tenantA := uuid.New()
	tenantB := uuid.New()
	createOrg(t, tenantA, "audit-tenant-a")
	createOrg(t, tenantB, "audit-tenant-b")

	record(t, tenantA, audit.Entry{ActorID: "system", Action: "a.secret", Resource: "r:1", Allowed: true})

	countFor := func(tenant uuid.UUID) int {
		var n int
		err := tenantdb.WithTenant(context.Background(), testDB.App, tenant, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&n)
		})
		if err != nil {
			t.Fatalf("count for tenant %v: %v", tenant, err)
		}
		return n
	}

	if n := countFor(tenantA); n != 1 {
		t.Errorf("tenant A sees %d rows, want 1", n)
	}
	if n := countFor(tenantB); n != 0 {
		t.Errorf("tenant B sees %d rows, want 0 (RLS isolation)", n)
	}
}

// audit.Record must run inside a tenant-scoped transaction: without
// app.tenant_id the RLS WITH CHECK rejects the insert, so a caller that
// forgets the tenant context gets an error instead of a cross-tenant row.
func TestRecord_WithoutTenantContextFails(t *testing.T) {
	needDB(t)
	clearAuditLog(t)
	tenant := uuid.New()
	createOrg(t, tenant, "audit-no-tenant")

	tx, err := testDB.App.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck

	err = audit.Record(context.Background(), tx, tenant, audit.Entry{
		ActorID: "system", Action: "a.orphan", Resource: "r:1", Allowed: true,
	})
	if err == nil {
		t.Fatal("Record without app.tenant_id succeeded, want RLS rejection")
	}
}

// The app role may append and read but never mutate: UPDATE and DELETE on
// audit_log must be refused by the database grants.
func TestAuditLog_AppendOnlyForAppRole(t *testing.T) {
	needDB(t)
	clearAuditLog(t)
	tenant := uuid.New()
	createOrg(t, tenant, "audit-append-only")
	record(t, tenant, audit.Entry{ActorID: "system", Action: "a.immutable", Resource: "r:1", Allowed: true})

	err := tenantdb.WithTenant(context.Background(), testDB.App, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE audit_log SET action = 'tampered'`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("UPDATE as klubhub_app: err = %v, want permission denied", err)
	}

	err = tenantdb.WithTenant(context.Background(), testDB.App, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `DELETE FROM audit_log`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("DELETE as klubhub_app: err = %v, want permission denied", err)
	}

	// The row survived both attempts.
	rows := readAll(t)
	if len(rows) != 1 || rows[0].Action != "a.immutable" {
		t.Errorf("audit_log after tamper attempts = %+v, want the original row", rows)
	}
}
