package migrations_test

import (
	"regexp"
	"strings"
	"testing"
)

// No ID images anywhere (P2.5 contract): the Promoter schema must not grow a
// column that could hold a photo or scan of a person or their documents.
// Guests and door check-ins are name-only; nothing accepts uploads for them.
//
// Reviewed exceptions go here as "table.column" with a reason, e.g. an event
// flyer. There are none today: events carry no image column.
var imageColumnAllowed = map[string]string{}

var (
	imageColumnName = regexp.MustCompile(`(?i)(^|_)(images?|photos?|pictures?|passports?|id_documents?|id_cards?|selfies?|scans?|scanned_ids?)($|_)`)
	addColumnRe     = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`)
)

// schemaColumns returns every "table.column" created or added by the SQL.
func schemaColumns(sql string) []string {
	var out []string
	for _, m := range createTableRe.FindAllStringSubmatch(sql, -1) {
		table := strings.ToLower(m[1])
		for _, c := range columnLineRe.FindAllStringSubmatch(m[2], -1) {
			col := strings.ToLower(c[1])
			if !sqlKeywords[col] {
				out = append(out, table+"."+col)
			}
		}
	}
	for _, m := range addColumnRe.FindAllStringSubmatch(sql, -1) {
		out = append(out, strings.ToLower(m[1]+"."+m[2]))
	}
	return out
}

func imageColumns(sql string) []string {
	var bad []string
	for _, tc := range schemaColumns(sql) {
		col := tc[strings.Index(tc, ".")+1:]
		if _, ok := imageColumnAllowed[tc]; !ok && imageColumnName.MatchString(col) {
			bad = append(bad, tc)
		}
	}
	return bad
}

func TestNoIDImageColumns(t *testing.T) {
	sql := allUpSQL(t)
	if len(schemaColumns(sql)) < 50 {
		t.Fatal("column parser found almost nothing; the guard would pass vacuously")
	}
	for _, c := range imageColumns(sql) {
		t.Errorf("%s: looks like a photo or ID image column; P2.5 forbids storing ID images (add a reviewed exception only for non-person images such as flyers)", c)
	}
}

func TestNoIDImageGuardCatchesViolations(t *testing.T) {
	sql := "CREATE TABLE guests (\n    id UUID PRIMARY KEY,\n    id_document_enc BYTEA,\n    selfie BYTEA\n);\n" +
		"ALTER TABLE checkins ADD COLUMN photo_url TEXT;\n" +
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS passport_scan_enc BYTEA;\n" +
		"CREATE TABLE ok (\n    id UUID PRIMARY KEY,\n    scanned_at TIMESTAMPTZ,\n    name_enc BYTEA\n);"
	got := strings.Join(imageColumns(sql), ",")
	for _, want := range []string{"guests.id_document_enc", "guests.selfie", "checkins.photo_url", "users.passport_scan_enc"} {
		if !strings.Contains(got, want) {
			t.Errorf("guard missed %s (got %s)", want, got)
		}
	}
	if strings.Contains(got, "ok.") {
		t.Errorf("false positive: %s", got)
	}
}
