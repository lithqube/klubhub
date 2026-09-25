package migrations_test

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/promoter/migrations"
)

// Tables that are deliberately not tenant-scoped. Each entry needs a reason;
// adding a table here is a security review, not a convenience.
var globalTables = map[string]string{}

// tenantKey maps tables whose tenant column is not named tenant_id.
var tenantKey = map[string]string{
	"organizations": "id", // the tenant row itself
}

var (
	createTableRe = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\s*\((.*?)\n\);`)
	enableRe      = `(?i)ALTER\s+TABLE\s+%s\s+ENABLE\s+ROW\s+LEVEL\s+SECURITY`
	forceRe       = `(?i)ALTER\s+TABLE\s+%s\s+FORCE\s+ROW\s+LEVEL\s+SECURITY`
	policyRe      = `(?is)CREATE\s+POLICY\s+\w+\s+ON\s+%s\b.*?USING\s*\(\s*%s\s*=\s*current_setting\('app\.tenant_id'\)::uuid\s*\).*?WITH\s+CHECK\s*\(\s*%s\s*=\s*current_setting\('app\.tenant_id'\)::uuid\s*\)`
	missingOkRe   = regexp.MustCompile(`(?i)current_setting\(\s*'app\.tenant_id'\s*,\s*true\s*\)`)
)

func allUpSQL(t *testing.T) string {
	t.Helper()
	var names []string
	if err := fs.WalkDir(migrations.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".sql") {
			names = append(names, p)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		raw, err := fs.ReadFile(migrations.FS, n)
		if err != nil {
			t.Fatal(err)
		}
		up := string(raw)
		if i := strings.Index(up, "-- +goose Down"); i >= 0 {
			up = up[:i]
		}
		b.WriteString(up)
		b.WriteString("\n")
	}
	return b.String()
}

// TestEveryTableIsTenantIsolated is the RLS CI guard (plan §13.3): every
// table must have its tenant column, ENABLE + FORCE row level security and a
// fail-closed tenant_isolation policy, unless it is explicitly global.
func TestEveryTableIsTenantIsolated(t *testing.T) {
	sql := allUpSQL(t)
	tables := createTableRe.FindAllStringSubmatch(sql, -1)
	if len(tables) == 0 {
		t.Fatal("no CREATE TABLE statements found; the guard would pass vacuously")
	}
	for _, m := range tables {
		table, body := strings.ToLower(m[1]), m[2]
		if reason, ok := globalTables[table]; ok {
			t.Logf("%s is global: %s", table, reason)
			continue
		}
		key := tenantKey[table]
		if key == "" {
			key = "tenant_id"
		}
		t.Run(table, func(t *testing.T) {
			if !regexp.MustCompile(`(?im)^\s*` + key + `\s+UUID\b.*NOT\s+NULL`).MatchString(body) &&
				!(key == "id" && regexp.MustCompile(`(?im)^\s*id\s+UUID\s+PRIMARY\s+KEY`).MatchString(body)) {
				t.Errorf("%s: missing %s UUID NOT NULL column", table, key)
			}
			for _, pat := range []string{enableRe, forceRe} {
				if !regexp.MustCompile(strings.ReplaceAll(pat, "%s", table)).MatchString(sql) {
					t.Errorf("%s: missing %q", table, strings.ReplaceAll(pat, "%s", table))
				}
			}
			policy := regexp.MustCompile(sprintf3(policyRe, table, key, key))
			if !policy.MatchString(sql) {
				t.Errorf("%s: missing fail-closed tenant policy (USING and WITH CHECK on %s = current_setting('app.tenant_id')::uuid)", table, key)
			}
		})
	}
}

// TestNoMissingOkTenantSetting forbids current_setting('app.tenant_id', true):
// the lenient form turns a missing tenant into an empty result instead of an
// error (a known pitfall in the reference project).
func TestNoMissingOkTenantSetting(t *testing.T) {
	if missingOkRe.MatchString(allUpSQL(t)) {
		t.Fatal("current_setting('app.tenant_id', true) is not allowed; use the fail-closed form")
	}
}

func sprintf3(pat, a, b, c string) string {
	out := strings.Replace(pat, "%s", regexp.QuoteMeta(a), 1)
	out = strings.Replace(out, "%s", regexp.QuoteMeta(b), 1)
	return strings.Replace(out, "%s", regexp.QuoteMeta(c), 1)
}
