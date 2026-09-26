package envelope_test

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

var appPool *pgxpool.Pool

func TestMain(m *testing.M) {
	flag.Parse()
	db, err := pgtest.StartPromoter()
	if err != nil {
		panic(err)
	}
	if db != nil {
		appPool = db.App
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

func newKEK(t *testing.T, id string) *envelope.KEK {
	t.Helper()
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	k, err := envelope.NewKEK(id, raw)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func newTenant(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if err := tenantdb.WithTenant(context.Background(), appPool, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO organizations (id, name, slug) VALUES ($1, 'org', $2)`, id, "org-"+id.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func with(t *testing.T, tenant uuid.UUID, fn func(tx pgx.Tx) error) {
	t.Helper()
	if err := tenantdb.WithTenant(context.Background(), appPool, tenant, fn); err != nil {
		t.Fatal(err)
	}
}

func TestKeyringCreatesOnceAndReopensAcrossInstances(t *testing.T) {
	needDB(t)
	ctx := context.Background()
	kek := newKEK(t, "kek-a")
	tenant := newTenant(t)
	f := envelope.Field{Table: "guests", Column: "name_enc", RowID: uuid.New()}

	var sealed []byte
	with(t, tenant, func(tx pgx.Tx) error {
		dek, err := envelope.NewKeyring(kek).Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		sealed, err = dek.Seal(tenant, f, []byte("Anna"))
		return err
	})

	// A fresh keyring (a restarted process) finds the same key via the database.
	with(t, tenant, func(tx pgx.Tx) error {
		r := envelope.NewKeyring(kek)
		dek, err := r.ForSealed(ctx, tx, tenant, sealed)
		if err != nil {
			return err
		}
		pt, err := dek.Open(tenant, f, sealed)
		if err != nil || string(pt) != "Anna" {
			t.Fatalf("reopen failed: %q %v", pt, err)
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM tenant_keys`).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("want exactly one key row, got %d", n)
		}
		return nil
	})
}

func TestRotationKeepsOldDataReadable(t *testing.T) {
	needDB(t)
	ctx := context.Background()
	kek := newKEK(t, "kek-a")
	r := envelope.NewKeyring(kek)
	tenant := newTenant(t)
	f := envelope.Field{Table: "t", Column: "c", RowID: uuid.New()}

	var v1 []byte
	with(t, tenant, func(tx pgx.Tx) error {
		dek, err := r.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		v1, err = dek.Seal(tenant, f, []byte("old"))
		return err
	})
	with(t, tenant, func(tx pgx.Tx) error {
		dek, err := r.Rotate(ctx, tx, tenant)
		if err != nil {
			return err
		}
		if dek.Version() != 2 {
			t.Fatalf("rotated key version = %d, want 2", dek.Version())
		}
		return nil
	})
	with(t, tenant, func(tx pgx.Tx) error {
		cur, err := r.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		if cur.Version() != 2 {
			t.Fatalf("current version after rotation = %d", cur.Version())
		}
		old, err := r.ForSealed(ctx, tx, tenant, v1)
		if err != nil {
			return err
		}
		if pt, err := old.Open(tenant, f, v1); err != nil || string(pt) != "old" {
			t.Fatalf("v1 data unreadable after rotation: %v", err)
		}
		return nil
	})
}

func TestKEKRotationUnwrapsWithPreviousKEK(t *testing.T) {
	needDB(t)
	ctx := context.Background()
	oldKEK, newKEKv := newKEK(t, "kek-2026-01"), newKEK(t, "kek-2026-09")
	tenant := newTenant(t)
	f := envelope.Field{Table: "t", Column: "c", RowID: uuid.New()}
	var sealed []byte
	with(t, tenant, func(tx pgx.Tx) error {
		dek, err := envelope.NewKeyring(oldKEK).Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		sealed, err = dek.Seal(tenant, f, []byte("x"))
		return err
	})
	with(t, tenant, func(tx pgx.Tx) error {
		if _, err := envelope.NewKeyring(newKEKv).ForSealed(ctx, tx, tenant, sealed); !errors.Is(err, envelope.ErrUnknownKEK) {
			t.Fatalf("without the previous KEK the DEK must be unreachable, got %v", err)
		}
		dek, err := envelope.NewKeyring(newKEKv, oldKEK).ForSealed(ctx, tx, tenant, sealed)
		if err != nil {
			return err
		}
		_, err = dek.Open(tenant, f, sealed)
		return err
	})
}

func TestShredMakesDataPermanentlyUnreadable(t *testing.T) {
	needDB(t)
	ctx := context.Background()
	kek := newKEK(t, "kek-a")
	r := envelope.NewKeyring(kek)
	tenant := newTenant(t)
	f := envelope.Field{Table: "t", Column: "c", RowID: uuid.New()}
	var sealed []byte
	var cached *envelope.DEK
	with(t, tenant, func(tx pgx.Tx) error {
		dek, err := r.Current(ctx, tx, tenant)
		cached = dek
		if err != nil {
			return err
		}
		sealed, err = dek.Seal(tenant, f, []byte("gone"))
		return err
	})
	with(t, tenant, func(tx pgx.Tx) error { return r.Shred(ctx, tx, tenant) })

	if _, err := cached.Open(tenant, f, sealed); !errors.Is(err, envelope.ErrKeyDestroyed) {
		t.Fatalf("a DEK held in memory must be wiped by Shred, got %v", err)
	}
	with(t, tenant, func(tx pgx.Tx) error {
		if _, err := envelope.NewKeyring(kek).ForSealed(ctx, tx, tenant, sealed); !errors.Is(err, envelope.ErrNoKey) {
			t.Fatalf("after shredding, the key must be gone, got %v", err)
		}
		return nil
	})
}

func TestKeysAreInvisibleToOtherTenants(t *testing.T) {
	needDB(t)
	ctx := context.Background()
	r := envelope.NewKeyring(newKEK(t, "kek-a"))
	a, b := newTenant(t), newTenant(t)
	with(t, a, func(tx pgx.Tx) error { _, err := r.Current(ctx, tx, a); return err })
	with(t, b, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM tenant_keys WHERE tenant_id = $1`, a).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("tenant B can see %d of tenant A's keys", n)
		}
		return nil
	})
}
