package migrations

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

// RunMigrations applies all pending goose migrations against the given database.
// SQL files are embedded in the binary at compile time.
// Returns an error if any migration fails; caller should treat non-nil as fatal.
func RunMigrations(db *sql.DB) error {
	goose.SetBaseFS(FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, ".")
}
