package migrations_test

import (
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/platform/migrations"
)

func TestMigrationFilesEmbedded(t *testing.T) {
	// Verify that the embedded FS contains at least one SQL file
	var sqlFiles []string
	err := fs.WalkDir(migrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && len(path) > 4 && path[len(path)-4:] == ".sql" {
			sqlFiles = append(sqlFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk embedded FS: %v", err)
	}

	if len(sqlFiles) == 0 {
		t.Error("expected at least one .sql file embedded in migrations.FS, found none")
	}

	t.Logf("found %d embedded SQL file(s): %v", len(sqlFiles), sqlFiles)
}

func TestMigrationVersionOne(t *testing.T) {
	// Verify that at least one migration file exists with version 1 naming
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("failed to read embedded FS root: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected at least one entry in migrations.FS, found none")
	}

	// Check that the first migration file starts with "001_"
	found := false
	for _, entry := range entries {
		if len(entry.Name()) >= 4 && entry.Name()[:4] == "001_" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected a migration file starting with '001_', got entries: %v", entries)
	}
}

// TestMigrationNumericPrefixes asserts that every embedded SQL migration
// filename has a purely numeric version prefix (digits only, separated from
// the rest of the filename by a single underscore). This guards against the
// fatal-startup bug where Goose's NumericComponent calls
// strconv.ParseInt(before, 10, 64) and rejects any non-digit characters in
// the version segment — e.g. "005b_venues.sql" parses the prefix "005b" and
// returns `invalid syntax`, which surfaces in api/cmd/api/main.go as
// "failed to run migrations" on boot.
func TestMigrationNumericPrefixes(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("failed to read embedded FS root: %v", err)
	}

	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if len(name) < 5 || name[len(name)-4:] != ".sql" {
			continue
		}
		sep := strings.Index(name, "_")
		if sep <= 0 {
			offenders = append(offenders, name+" (missing version separator)")
			continue
		}
		prefix := name[:sep]
		for i, r := range prefix {
			if r < '0' || r > '9' {
				offenders = append(offenders, name+" (non-digit in version: "+string(r)+" at index "+strconv.Itoa(i)+")")
				break
			}
		}
	}

	if len(offenders) > 0 {
		t.Errorf("found %d migration file(s) with non-numeric version prefix(es): %v", len(offenders), offenders)
	}
}

// TestMigrationVersionsUnique asserts that every embedded SQL migration has
// a unique numeric version. Goose rejects duplicate versions during
// collection (provider_collect.go) with `found duplicate migration version
// %d`, so this is a cheap guard against accidental collisions.
func TestMigrationVersionsUnique(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("failed to read embedded FS root: %v", err)
	}

	seen := make(map[string]string) // version -> filename
	for _, entry := range entries {
		name := entry.Name()
		if len(name) < 5 || name[len(name)-4:] != ".sql" {
			continue
		}
		sep := strings.Index(name, "_")
		if sep <= 0 {
			continue
		}
		version := name[:sep]
		if existing, ok := seen[version]; ok {
			t.Errorf("duplicate migration version %q: %s and %s", version, existing, name)
		}
		seen[version] = name
	}
}
