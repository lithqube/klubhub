package migrations_test

import (
	"regexp"
	"strings"
	"testing"
)

// Data-class guard (plan §13.4). Every table declares one class with
//
//	COMMENT ON TABLE t IS 'data_class: public|internal|personal|financial';
//
// In personal/financial tables every column must be sealed (`*_enc BYTEA`), a
// blind index (`*_bidx BYTEA`), a key/timestamp, or carry a reviewed column
// exception (`COMMENT ON COLUMN t.c IS 'data_class: internal — <reason>'`).
// Anywhere in the schema, a plaintext column whose name looks like personal
// or financial data needs such an exception too. `sealed` (E2EE) values are
// `*_sealed BYTEA`: the server only ever stores the browser's ciphertext.

var (
	tableClassRe  = regexp.MustCompile(`(?i)COMMENT\s+ON\s+TABLE\s+([a-z_][a-z0-9_]*)\s+IS\s+'data_class:\s*(public|internal|personal|financial)\b`)
	columnClassRe = regexp.MustCompile(`(?i)COMMENT\s+ON\s+COLUMN\s+([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)\s+IS\s+'data_class:\s*(public|internal)\s*[—-]\s*\S`)
	columnLineRe  = regexp.MustCompile(`(?im)^\s*([a-z_][a-z0-9_]*)\s+([A-Z][A-Z0-9_]*(?:\s*\([^)]*\))?)`)
	sensitiveName = regexp.MustCompile(`(?i)(^|_)(email|phone|mobile|first_name|last_name|full_name|name|address|street|postcode|zip|iban|bic|account_number|vat_id|tax_id|birth|dob|passport|id_number|notes?|ip)($|_)`)
	keyColumn     = regexp.MustCompile(`^(id|tenant_id|[a-z0-9_]+_id|[a-z0-9_]+_at|key_version|version|position|sort_order)$`)
	protectedCol  = regexp.MustCompile(`_(enc|bidx|sealed)$`)
)

var sqlKeywords = map[string]bool{"primary": true, "unique": true, "constraint": true, "check": true, "foreign": true, "exclude": true}

func checkDataClasses(sql string) []string {
	var problems []string
	classes := map[string]string{}
	for _, m := range tableClassRe.FindAllStringSubmatch(sql, -1) {
		classes[strings.ToLower(m[1])] = strings.ToLower(m[2])
	}
	exceptions := map[string]bool{}
	for _, m := range columnClassRe.FindAllStringSubmatch(sql, -1) {
		exceptions[strings.ToLower(m[1]+"."+m[2])] = true
	}
	for _, m := range createTableRe.FindAllStringSubmatch(sql, -1) {
		table := strings.ToLower(m[1])
		class, ok := classes[table]
		if !ok {
			problems = append(problems, table+": missing COMMENT ON TABLE ... 'data_class: <class>'")
			continue
		}
		for _, c := range columnLineRe.FindAllStringSubmatch(m[2], -1) {
			col, typ := strings.ToLower(c[1]), strings.ToUpper(c[2])
			if sqlKeywords[col] {
				continue
			}
			if protectedCol.MatchString(col) {
				if !strings.HasPrefix(typ, "BYTEA") {
					problems = append(problems, table+"."+col+": protected columns must be BYTEA")
				}
				continue
			}
			if exceptions[table+"."+col] {
				continue
			}
			sensitiveTable := class == "personal" || class == "financial"
			switch {
			case sensitiveTable && !keyColumn.MatchString(col):
				problems = append(problems, table+"."+col+": plaintext column in a "+class+" table (seal it as *_enc or add a reviewed column exception)")
			case sensitiveName.MatchString(col) && !keyColumn.MatchString(col):
				problems = append(problems, table+"."+col+": looks like personal/financial data stored in plaintext")
			}
		}
	}
	return problems
}

func TestSchemaDataClasses(t *testing.T) {
	for _, p := range checkDataClasses(allUpSQL(t)) {
		t.Error(p)
	}
}

// The guard itself is tested on synthetic schemas so a regex regression
// cannot silently turn it into a no-op.
func TestDataClassGuardCatchesViolations(t *testing.T) {
	cases := map[string]struct {
		sql  string
		want string
	}{
		"unclassified table": {
			sql:  "CREATE TABLE a (\n    id UUID PRIMARY KEY\n);",
			want: "a: missing",
		},
		"plaintext in personal table": {
			sql: "CREATE TABLE g (\n    id UUID PRIMARY KEY,\n    tenant_id UUID NOT NULL,\n    display TEXT\n);\n" +
				"COMMENT ON TABLE g IS 'data_class: personal';",
			want: "g.display: plaintext column in a personal table",
		},
		"sensitive name in internal table": {
			sql: "CREATE TABLE v (\n    id UUID PRIMARY KEY,\n    contact_email TEXT\n);\n" +
				"COMMENT ON TABLE v IS 'data_class: internal';",
			want: "v.contact_email: looks like personal",
		},
		"enc column not bytea": {
			sql: "CREATE TABLE g (\n    id UUID PRIMARY KEY,\n    name_enc TEXT\n);\n" +
				"COMMENT ON TABLE g IS 'data_class: personal';",
			want: "g.name_enc: protected columns must be BYTEA",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(checkDataClasses(c.sql), "\n")
			if !strings.Contains(got, c.want) {
				t.Fatalf("want a problem containing %q, got:\n%s", c.want, got)
			}
		})
	}
}

func TestDataClassGuardAcceptsCompliantSchema(t *testing.T) {
	sql := "CREATE TABLE guests (\n    id UUID PRIMARY KEY,\n    tenant_id UUID NOT NULL,\n    event_id UUID NOT NULL,\n" +
		"    name_enc BYTEA NOT NULL,\n    name_bidx BYTEA NOT NULL,\n    party_size SMALLINT NOT NULL,\n    created_at TIMESTAMPTZ NOT NULL,\n" +
		"    CONSTRAINT x CHECK (party_size > 0)\n);\n" +
		"COMMENT ON TABLE guests IS 'data_class: personal';\n" +
		"COMMENT ON COLUMN guests.party_size IS 'data_class: internal — a head count, not identifying';"
	if p := checkDataClasses(sql); len(p) != 0 {
		t.Fatalf("compliant schema flagged: %v", p)
	}
}
