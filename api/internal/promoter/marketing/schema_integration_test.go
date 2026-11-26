package marketing_test

import (
	"context"
	"flag"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/marketing"
)

// Compile-time check of the interface shape; the SMTP and Plunk adapters
// (P3.2-3/4) add the same line for their own types.
var _ marketing.MarketingProvider = stubProvider{}

type stubProvider struct{}

func (stubProvider) SendMarketing(context.Context, *marketing.Message) (string, error) {
	return "", nil
}

func (stubProvider) HandleWebhook(*http.Request) ([]marketing.WebhookEvent, error) {
	return nil, nil
}

var testDB *pgtest.DB

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}
	db, err := pgtest.StartPromoter()
	if err != nil {
		panic(err)
	}
	testDB = db
	code := m.Run()
	db.Close()
	os.Exit(code)
}

var tables = []string{"marketing_provider_configs", "marketing_sends", "marketing_suppressions"}

func newOrg(t *testing.T) uuid.UUID {
	t.Helper()
	org := uuid.Must(uuid.NewV7())
	bg := context.Background()
	if err := tenantdb.WithTenant(bg, testDB.App, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(bg, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Nachtwerk', $2)`, org, "nw-"+org.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return org
}

// seed writes one row per marketing table as the given tenant.
func seed(t *testing.T, org uuid.UUID) {
	t.Helper()
	bg := context.Background()
	if err := tenantdb.WithTenant(bg, testDB.App, org, func(tx pgx.Tx) error {
		if _, err := tx.Exec(bg, `INSERT INTO marketing_provider_configs (id, tenant_id, provider, from_email, settings_enc)
			VALUES ($1, $2, 'smtp', 'news@nachtwerk.example', '\x01')`, uuid.New(), org); err != nil {
			return err
		}
		if _, err := tx.Exec(bg, `INSERT INTO marketing_sends (id, tenant_id, to_email_enc, to_email_bidx, subject_enc, body_enc)
			VALUES ($1, $2, '\x01', '\x02', '\x03', '\x04')`, uuid.New(), org); err != nil {
			return err
		}
		_, err := tx.Exec(bg, `INSERT INTO marketing_suppressions (id, tenant_id, email_bidx, reason)
			VALUES ($1, $2, '\x02', 'bounce')`, uuid.New(), org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, org uuid.UUID, table string) int {
	t.Helper()
	var n int
	bg := context.Background()
	if err := tenantdb.WithTenant(bg, testDB.App, org, func(tx pgx.Tx) error {
		return tx.QueryRow(bg, `SELECT count(*) FROM `+table).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMarketingTables_TenantIsolation(t *testing.T) {
	if testDB == nil {
		t.Skip("integration: -short")
	}
	orgA, orgB := newOrg(t), newOrg(t)
	seed(t, orgA)
	for _, table := range tables {
		if got := count(t, orgA, table); got != 1 {
			t.Errorf("%s: tenant A sees %d rows, want 1", table, got)
		}
		if got := count(t, orgB, table); got != 0 {
			t.Errorf("%s: tenant B sees %d of tenant A's rows", table, got)
		}
	}
}

// Without app.tenant_id the policy must error, never return an empty set.
func TestMarketingTables_FailClosedWithoutTenant(t *testing.T) {
	if testDB == nil {
		t.Skip("integration: -short")
	}
	bg := context.Background()
	for _, table := range tables {
		var n int
		if err := testDB.App.QueryRow(bg, `SELECT count(*) FROM `+table).Scan(&n); err == nil {
			t.Errorf("%s: query without a tenant succeeded (%d rows); want an error", table, n)
		}
	}
}

// WITH CHECK: a tenant cannot write a row stamped with another tenant's id.
func TestMarketingTables_CannotWriteForOtherTenant(t *testing.T) {
	if testDB == nil {
		t.Skip("integration: -short")
	}
	orgA, orgB := newOrg(t), newOrg(t)
	bg := context.Background()
	err := tenantdb.WithTenant(bg, testDB.App, orgA, func(tx pgx.Tx) error {
		_, err := tx.Exec(bg, `INSERT INTO marketing_suppressions (id, tenant_id, email_bidx, reason)
			VALUES ($1, $2, '\x02', 'manual')`, uuid.New(), orgB)
		return err
	})
	if err == nil {
		t.Fatal("insert for another tenant succeeded; want an RLS violation")
	}
}

// WITH CHECK on every table: re-stamping a row to another tenant is refused.
func TestMarketingTables_CannotMoveRowToOtherTenant(t *testing.T) {
	if testDB == nil {
		t.Skip("integration: -short")
	}
	orgA, orgB := newOrg(t), newOrg(t)
	seed(t, orgA)
	bg := context.Background()
	for _, table := range tables {
		err := tenantdb.WithTenant(bg, testDB.App, orgA, func(tx pgx.Tx) error {
			_, err := tx.Exec(bg, `UPDATE `+table+` SET tenant_id = $1`, orgB)
			return err
		})
		if err == nil {
			t.Errorf("%s: moving tenant A's row to tenant B succeeded; want an RLS violation", table)
		}
	}
}

// Erasing a contact deletes its marketing mail, sealed address included.
func TestMarketingSends_DeletedWithContact(t *testing.T) {
	if testDB == nil {
		t.Skip("integration: -short")
	}
	org := newOrg(t)
	contact := uuid.New()
	bg := context.Background()
	if err := tenantdb.WithTenant(bg, testDB.App, org, func(tx pgx.Tx) error {
		if _, err := tx.Exec(bg, `INSERT INTO audience_contacts (id, tenant_id, email_enc, email_bidx, source, consent_basis, consent_recorded_at)
			VALUES ($1, $2, '\x01', '\x02', 'csv', 'consent', now())`, contact, org); err != nil {
			return err
		}
		if _, err := tx.Exec(bg, `INSERT INTO marketing_sends (id, tenant_id, contact_id, to_email_enc, to_email_bidx, subject_enc, body_enc)
			VALUES ($1, $2, $3, '\x01', '\x02', '\x03', '\x04')`, uuid.New(), org, contact); err != nil {
			return err
		}
		_, err := tx.Exec(bg, `DELETE FROM audience_contacts WHERE id = $1`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := count(t, org, "marketing_sends"); got != 0 {
		t.Errorf("marketing_sends keeps %d rows after the contact was deleted, want 0", got)
	}
}
