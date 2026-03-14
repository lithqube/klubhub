package migrations_test

import (
	"io/fs"
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
