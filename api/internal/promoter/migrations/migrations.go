// Package migrations holds the KlubHub Promoter schema. It is separate from
// the DJ schema (platform/migrations): Promoter runs against its own
// database, and every table is tenant-isolated with Postgres row level
// security (see rls_guard_test.go and plan §13.3).
package migrations

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

// Up applies all pending Promoter migrations. It must run as the schema owner
// role, never as the runtime app role. A goose Provider is used instead of
// the package-level goose state so the DJ and Promoter schemas can never
// share configuration inside one process.
func Up(ctx context.Context, db *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, FS)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
