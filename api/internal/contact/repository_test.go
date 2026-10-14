package contact_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/platform/migrations"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}
	defer container.Terminate(ctx) //nolint:errcheck

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("failed to get connection string: " + err.Error())
	}

	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		panic("failed to open sql.DB: " + err.Error())
	}
	defer sqlDB.Close()

	if err := migrations.RunMigrations(sqlDB); err != nil {
		panic("failed to run migrations: " + err.Error())
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		panic("failed to create pgxpool: " + err.Error())
	}
	defer testPool.Close()

	m.Run()
}

func clearContacts(t *testing.T) {
	t.Helper()
	// gig_contacts references contacts ON DELETE CASCADE and
	// invoices.customer_contact_id is ON DELETE SET NULL, so a plain
	// DELETE is safe.
	if _, err := testPool.Exec(context.Background(), "DELETE FROM contacts"); err != nil {
		t.Fatalf("failed to clear contacts: %v", err)
	}
}

func strPtr(s string) *string { return &s }

func TestRepositoryCreate_RoundTripsPartyFields(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &contact.ContactCreate{
		Name:    "Mara Promoter",
		Company: strPtr("Nachtwerk GmbH"),
		Email:   "mara@nachtwerk.example",
		Phone:   "+49 30 123",
		Type:    contact.ContactTypePromoter,
		Notes:   "Prefers email",
		PartyFields: contact.PartyFields{
			AddressLine1: "Hauptstr. 1",
			City:         "Berlin",
			PostalCode:   "10115",
			Country:      "DE",
			VATID:        "DE123456789",
			TaxID:        "STNR 12/345",
			IsBusiness:   true,
		},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Error("expected generated UUID, got nil UUID")
	}
	if created.Type != contact.ContactTypePromoter {
		t.Errorf("type = %q, want promoter", created.Type)
	}
	if !created.IsBusiness {
		t.Error("is_business = false, want true")
	}
	if created.VATID != "DE123456789" || created.TaxID != "STNR 12/345" {
		t.Errorf("vat/tax = %q/%q", created.VATID, created.TaxID)
	}

	fetched, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.Company == nil || *fetched.Company != "Nachtwerk GmbH" {
		t.Errorf("company = %v, want Nachtwerk GmbH", fetched.Company)
	}
	if fetched.AddressLine1 != "Hauptstr. 1" || fetched.City != "Berlin" || fetched.Country != "DE" {
		t.Errorf("party fields round-trip mismatch: %+v", fetched.PartyFields)
	}
}

func TestRepositoryCreate_OptionalCompanyDefaultsToNull(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)

	created, err := repo.Create(context.Background(), &contact.ContactCreate{
		Name: "Solo Agent", Email: "a@b.c", Type: contact.ContactTypeAgent,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.Company != nil {
		t.Errorf("company = %q, want nil", *created.Company)
	}
	if created.Type != contact.ContactTypeAgent {
		t.Errorf("type = %q, want agent", created.Type)
	}
}

func TestRepositoryGetByID_NotFound(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, contact.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRepositoryList_FiltersByNameAndType(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)
	ctx := context.Background()

	seed := []*contact.ContactCreate{
		{Name: "Mara Promoter", Type: contact.ContactTypePromoter},
		{Name: "Max Agent", Type: contact.ContactTypeAgent},
		{Name: "Lena Label", Type: contact.ContactTypeLabel},
	}
	for _, s := range seed {
		if _, err := repo.Create(ctx, s); err != nil {
			t.Fatalf("seed Create failed: %v", err)
		}
	}

	all, err := repo.List(ctx, nil, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List returned %d contacts, want 3", len(all))
	}
	// ORDER BY name ASC
	names := []string{all[0].Name, all[1].Name, all[2].Name}
	if names[0] != "Lena Label" || names[1] != "Mara Promoter" || names[2] != "Max Agent" {
		t.Errorf("List not ordered by name: %v", names)
	}

	promoters := contact.ContactTypePromoter
	byType, err := repo.List(ctx, nil, &promoters)
	if err != nil {
		t.Fatalf("List by type failed: %v", err)
	}
	if len(byType) != 1 || byType[0].Name != "Mara Promoter" {
		t.Errorf("type filter returned %+v, want [Mara Promoter]", byType)
	}

	byName, err := repo.List(ctx, strPtr("agent"), nil)
	if err != nil {
		t.Fatalf("List by name failed: %v", err)
	}
	if len(byName) != 1 || byName[0].Name != "Max Agent" {
		t.Errorf("name filter returned %+v, want [Max Agent]", byName)
	}

	// Name + type combined: matching name but wrong type yields nothing.
	agents := contact.ContactTypeAgent
	both, err := repo.List(ctx, strPtr("mara"), &agents)
	if err != nil {
		t.Fatalf("List by name+type failed: %v", err)
	}
	if len(both) != 0 {
		t.Errorf("name+type filter returned %+v, want empty", both)
	}
}

func TestRepositoryUpdate_PartialUpdateLeavesOtherFields(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &contact.ContactCreate{
		Name: "Mara Promoter", Email: "mara@nachtwerk.example",
		Type: contact.ContactTypePromoter, Notes: "original",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, &contact.ContactUpdate{Email: strPtr("new@nachtwerk.example")})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Email != "new@nachtwerk.example" {
		t.Errorf("email = %q, want new@nachtwerk.example", updated.Email)
	}
	if updated.Name != "Mara Promoter" || updated.Type != contact.ContactTypePromoter || updated.Notes != "original" {
		t.Errorf("untouched fields changed: %+v", updated)
	}
}

func TestRepositoryUpdate_PartyFields(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &contact.ContactCreate{Name: "Biz Contact", Type: contact.ContactTypeOther})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	isBiz := true
	updated, err := repo.Update(ctx, created.ID, &contact.ContactUpdate{
		Country:    strPtr("de"), // stored as given; normalization is the service layer's job
		IsBusiness: &isBiz,
		VATID:      strPtr("DE999"),
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Country != "de" {
		t.Errorf("country = %q, want de (repository stores verbatim)", updated.Country)
	}
	if !updated.IsBusiness {
		t.Error("is_business = false, want true")
	}
	if updated.VATID != "DE999" {
		t.Errorf("vat_id = %q, want DE999", updated.VATID)
	}
}

func TestRepositoryUpdate_EmptyUpdateReturnsCurrent(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &contact.ContactCreate{Name: "Mara", Type: contact.ContactTypePromoter})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.Update(ctx, created.ID, &contact.ContactUpdate{})
	if err != nil {
		t.Fatalf("Update with empty body failed: %v", err)
	}
	if got.ID != created.ID || got.Name != "Mara" {
		t.Errorf("empty update returned %+v, want the unchanged contact", got)
	}
}

func TestRepositoryUpdate_NotFound(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)

	_, err := repo.Update(context.Background(), uuid.New(), &contact.ContactUpdate{Name: strPtr("x")})
	if !errors.Is(err, contact.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRepositorySoftDelete_HidesFromGetListAndSearch(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)
	ctx := context.Background()

	keep, err := repo.Create(ctx, &contact.ContactCreate{Name: "Keep", Type: contact.ContactTypeOther})
	if err != nil {
		t.Fatalf("Create keep failed: %v", err)
	}
	drop, err := repo.Create(ctx, &contact.ContactCreate{Name: "Drop", Type: contact.ContactTypeOther})
	if err != nil {
		t.Fatalf("Create drop failed: %v", err)
	}

	if err := repo.SoftDelete(ctx, drop.ID); err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	if _, err := repo.GetByID(ctx, drop.ID); !errors.Is(err, contact.ErrNotFound) {
		t.Errorf("GetByID after soft delete: err = %v, want ErrNotFound", err)
	}

	list, err := repo.List(ctx, nil, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != keep.ID {
		t.Errorf("List after soft delete = %+v, want only the kept contact", list)
	}

	found, err := repo.SearchByName(ctx, "drop", 5)
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("SearchByName found soft-deleted contact: %+v", found)
	}

	// Soft delete, not hard delete: the row survives with deleted_at set.
	var deletedAt *time.Time
	if err := testPool.QueryRow(ctx, "SELECT deleted_at FROM contacts WHERE id = $1", drop.ID).Scan(&deletedAt); err != nil {
		t.Fatalf("deleted row missing: %v", err)
	}
	if deletedAt == nil {
		t.Error("deleted_at is NULL, want a timestamp (soft delete)")
	}

	if err := repo.SoftDelete(ctx, drop.ID); !errors.Is(err, contact.ErrNotFound) {
		t.Errorf("second SoftDelete: err = %v, want ErrNotFound", err)
	}
}

func TestRepositorySearchByName_CaseInsensitiveAndLimited(t *testing.T) {
	clearContacts(t)
	repo := contact.NewRepository(testPool)
	ctx := context.Background()

	for _, name := range []string{"Mara", "Marco", "Lena"} {
		if _, err := repo.Create(ctx, &contact.ContactCreate{Name: name, Type: contact.ContactTypeOther}); err != nil {
			t.Fatalf("seed Create failed: %v", err)
		}
	}

	got, err := repo.SearchByName(ctx, "MA", 5)
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}
	if len(got) != 2 || got[0].Name != "Mara" || got[1].Name != "Marco" {
		t.Errorf("SearchByName(MA) = %+v, want [Mara Marco]", got)
	}

	limited, err := repo.SearchByName(ctx, "ma", 1)
	if err != nil {
		t.Fatalf("SearchByName limited failed: %v", err)
	}
	if len(limited) != 1 || limited[0].Name != "Mara" {
		t.Errorf("SearchByName(ma, 1) = %+v, want [Mara]", limited)
	}
}
