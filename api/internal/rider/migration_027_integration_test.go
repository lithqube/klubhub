package rider

import (
	"context"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/platform/migrations"
)

// Migration 027 adds the unique live-name index. A database that applied 026
// may already hold duplicate names, so the migration must rename them rather
// than fail (a failing migration blocks API startup). This replays 027's Up
// section against a table with duplicates, inside a transaction that is
// rolled back, so the shared test database is untouched.
func TestIntegration_Migration027_RenamesExistingDuplicates(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	ctx := context.Background()

	raw, err := migrations.FS.ReadFile("027_rider_template_unique_name.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	up := strings.SplitN(strings.SplitN(string(raw), "-- +goose Down", 2)[0], "-- +goose Up", 2)[1]

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Recreate the pre-027 state: no unique index, duplicates present.
	if _, err := tx.Exec(ctx, `DROP INDEX rider_templates_name_unique`); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	long := strings.Repeat("x", 200)
	if _, err := tx.Exec(ctx, `
		INSERT INTO rider_templates (name, created_at) VALUES
		  ('Dup Rider',  now() - interval '3 minutes'),
		  ('dup rider',  now() - interval '2 minutes'),
		  ('DUP RIDER',  now() - interval '1 minute'),
		  ($1,           now() - interval '2 minutes'),
		  ($1,           now() - interval '1 minute')`, long); err != nil {
		t.Fatalf("seed duplicates: %v", err)
	}
	// A soft-deleted twin must not count as a duplicate.
	if _, err := tx.Exec(ctx, `
		INSERT INTO rider_templates (name, deleted_at) VALUES ('Dup Rider', now())`); err != nil {
		t.Fatalf("seed deleted: %v", err)
	}

	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("migration 027 up failed on a database with duplicates: %v", err)
	}

	var oldest string
	if err := tx.QueryRow(ctx, `
		SELECT name FROM rider_templates
		 WHERE lower(name) LIKE 'dup rider%' AND deleted_at IS NULL
		 ORDER BY created_at LIMIT 1`).Scan(&oldest); err != nil {
		t.Fatal(err)
	}
	if oldest != "Dup Rider" {
		t.Errorf("oldest template renamed: got %q, want it untouched", oldest)
	}

	var distinct, total int
	if err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT lower(name)), count(*) FROM rider_templates
		 WHERE lower(name) LIKE 'dup rider%' AND deleted_at IS NULL`).Scan(&distinct, &total); err != nil {
		t.Fatal(err)
	}
	if total != 3 || distinct != 3 {
		t.Errorf("duplicates: %d rows, %d distinct names; want 3 and 3 (nothing deleted)", total, distinct)
	}

	var tooLong int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM rider_templates WHERE char_length(name) > 200`).Scan(&tooLong); err != nil {
		t.Fatal(err)
	}
	if tooLong != 0 {
		t.Errorf("%d names exceed the 200-character limit after renaming", tooLong)
	}

	// The index is back and enforced (last statement: a failure aborts the tx).
	if _, err := tx.Exec(ctx, `INSERT INTO rider_templates (name) VALUES ('DUP rider')`); err == nil {
		t.Error("unique index not enforced after migration 027")
	}
}
