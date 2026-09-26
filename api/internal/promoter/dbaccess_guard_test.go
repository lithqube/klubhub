package promoter_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Files allowed to hold a *pgxpool.Pool: wiring only. Domain packages get a
// tenant-scoped pgx.Tx from platform/tenantdb.
var poolAllowed = map[string]bool{}

// TestDomainCodeNeverImportsThePool enforces plan §13.3: tenantdb.WithTenant
// is the only way Promoter code reaches the database.
func TestDomainCodeNeverImportsThePool(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p == "github.com/jackc/pgx/v5/pgxpool" && !poolAllowed[filepath.ToSlash(path)] {
				t.Errorf("%s imports pgxpool; domain code must use platform/tenantdb", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
