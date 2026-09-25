package finance

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/migrations"
)

// TestIntegration_InvoicingMigrationBackfillAndRollback runs migrations
// 020–022 against rows written under the 019 schema (existing dev DBs have
// data) on a private database, then rolls back and re-applies them.
func TestIntegration_InvoicingMigrationBackfillAndRollback(t *testing.T) {
	requirePG(t) // only when the shared container could start (Docker available)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		tcpostgres.WithDatabase("klubhub_migrations"),
		tcpostgres.WithUsername("klubhub"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, ".", 19); err != nil {
		t.Fatalf("up to 019: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	var gigID string
	if err := db.QueryRowContext(ctx, `INSERT INTO gigs (date, fee_amount, fee_currency)
		VALUES ('2026-07-01T20:00:00Z', 100, 'EUR') RETURNING id`).Scan(&gigID); err != nil {
		t.Fatal(err)
	}
	// Legacy rows: numbers were allocated at draft creation.
	legacy := []struct{ num, status, issued string }{
		{"OLD-0001-EUR", "issued", "now()"},
		{"OLD-0002-EUR", "draft", "NULL"},
		{"OLD-0003-EUR", "cancelled", "NULL"},  // cancelled draft
		{"OLD-0004-EUR", "cancelled", "now()"}, // issued then cancelled (old rules)
		{"OLD-0005-EUR", "paid", "now()"},
	}
	for i, l := range legacy {
		exec(`INSERT INTO invoices (gig_id, invoice_number, number_prefix, number_seq, currency,
			subtotal_minor, tax_rate_bps, tax_minor, total_minor, status, issued_at)
			VALUES ($1, $2, 'OLD', $3, 'EUR', 10000, 1900, 1900, 11900, $4, `+l.issued+`)`,
			gigID, l.num, i+1, l.status)
	}

	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("up: %v", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT number_seq, invoice_number, status, net_payable_minor,
		tax_breakdown::text, to_char(supply_date, 'YYYY-MM-DD'), kind, customer::text
		FROM invoices ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		seq            sql.NullInt64
		num, sup       sql.NullString
		status, kind   string
		net            int64
		breakdown, cus string
	}
	byStatusNum := map[string]row{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.seq, &r.num, &r.status, &r.net, &r.breakdown, &r.sup, &r.kind, &r.cus); err != nil {
			t.Fatal(err)
		}
		if r.net != 11900 || r.kind != "invoice" || r.cus != "{}" {
			t.Errorf("backfill: %+v", r)
		}
		if r.breakdown != `[{"rate_bps": 1900, "tax_minor": 1900, "taxable_minor": 10000}]` {
			t.Errorf("breakdown = %s", r.breakdown)
		}
		byStatusNum[r.status+"/"+r.num.String] = r
	}
	rows.Close()
	for key, r := range byStatusNum {
		switch r.status {
		case "draft":
			if r.num.Valid || r.seq.Valid || r.sup.String != "2026-07-01" {
				t.Errorf("draft must be unnumbered with a supply date: %s %+v", key, r)
			}
		case "issued", "paid":
			if !r.num.Valid || r.sup.Valid {
				t.Errorf("issued rows keep their number and stay immutable: %s %+v", key, r)
			}
		}
	}
	if _, ok := byStatusNum["cancelled/"]; !ok {
		t.Errorf("never-issued cancelled draft should give its number back: %v", byStatusNum)
	}
	if _, ok := byStatusNum["cancelled/OLD-0004-EUR"]; !ok {
		t.Errorf("issued-then-cancelled keeps its number: %v", byStatusNum)
	}
	if _, err := db.ExecContext(ctx, `UPDATE invoices SET status = 'draft' WHERE invoice_number = 'OLD-0001-EUR'`); err == nil {
		t.Error("a numbered draft must violate invoices_draft_unnumbered_check")
	}

	// Rollback: every row numbered again, uniquely, after the series max.
	if err := goose.DownTo(db, ".", 19); err != nil {
		t.Fatalf("down to 019: %v", err)
	}
	var total, distinct, maxSeq int
	if err := db.QueryRowContext(ctx, `SELECT count(*), count(DISTINCT number_seq), max(number_seq) FROM invoices
		WHERE invoice_number <> ''`).Scan(&total, &distinct, &maxSeq); err != nil {
		t.Fatal(err)
	}
	if total != 5 || distinct != 5 || maxSeq != 7 {
		t.Fatalf("after down: total %d distinct %d max %d (want 5/5/7)", total, distinct, maxSeq)
	}
	var cols int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name IN ('invoices', 'contacts', 'billing_profiles')
		  AND column_name IN ('kind', 'customer', 'vat_id', 'default_vat_rate_bps')`).Scan(&cols)
	if cols != 0 {
		t.Fatalf("down left %d new columns behind", cols)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("re-up: %v", err)
	}
}
