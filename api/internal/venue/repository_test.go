package venue_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/venue"
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

func clearVenues(t *testing.T) {
	t.Helper()
	// gig_venues references venues ON DELETE CASCADE, so this is safe even
	// when a test linked a venue to a gig.
	if _, err := testPool.Exec(context.Background(), "DELETE FROM venues"); err != nil {
		t.Fatalf("failed to clear venues: %v", err)
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestRepositoryCreate_RoundTripsAllFields(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &venue.VenueCreate{
		Name:             "Funkhaus",
		City:             "Berlin",
		Country:          "DE",
		Capacity:         intPtr(800),
		Website:          strPtr("https://funkhaus.berlin"),
		TechContactName:  "Toni Technik",
		TechContactEmail: "toni@funkhaus.berlin",
		TechContactPhone: "+49 30 000",
		Notes:            "Load-in via courtyard",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("expected generated UUID, got nil UUID")
	}
	if created.Capacity == nil || *created.Capacity != 800 {
		t.Errorf("capacity = %v, want 800", created.Capacity)
	}
	if created.Website == nil || *created.Website != "https://funkhaus.berlin" {
		t.Errorf("website = %v, want https://funkhaus.berlin", created.Website)
	}
	if created.TechContactEmail != "toni@funkhaus.berlin" {
		t.Errorf("tech_contact_email = %q", created.TechContactEmail)
	}
	if created.DeletedAt != nil {
		t.Errorf("deleted_at = %v, want nil", created.DeletedAt)
	}

	fetched, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.Name != "Funkhaus" || fetched.City != "Berlin" || fetched.Country != "DE" {
		t.Errorf("round-trip mismatch: %+v", fetched)
	}
	if fetched.Notes != "Load-in via courtyard" {
		t.Errorf("notes = %q", fetched.Notes)
	}
}

func TestRepositoryCreate_OptionalFieldsDefaultToNull(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &venue.VenueCreate{Name: "Minimal", City: "", Country: ""})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.Capacity != nil {
		t.Errorf("capacity = %v, want nil", *created.Capacity)
	}
	if created.Website != nil {
		t.Errorf("website = %v, want nil", *created.Website)
	}
}

func TestRepositoryGetByID_NotFound(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, venue.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRepositoryList_FiltersAndOrdering(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	seed := []*venue.VenueCreate{
		{Name: "Berghain", City: "Berlin", Country: "DE"},
		{Name: "Funkhaus", City: "Berlin", Country: "DE"},
		{Name: "Paradiso", City: "Amsterdam", Country: "NL"},
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
		t.Fatalf("List returned %d venues, want 3", len(all))
	}
	// ORDER BY name ASC
	if all[0].Name != "Berghain" || all[1].Name != "Funkhaus" || all[2].Name != "Paradiso" {
		t.Errorf("List not ordered by name: %v, %v, %v", all[0].Name, all[1].Name, all[2].Name)
	}

	byCity, err := repo.List(ctx, nil, strPtr("amsterdam"))
	if err != nil {
		t.Fatalf("List by city failed: %v", err)
	}
	if len(byCity) != 1 || byCity[0].Name != "Paradiso" {
		t.Errorf("city filter returned %+v, want [Paradiso]", byCity)
	}

	byName, err := repo.List(ctx, strPtr("funk"), nil)
	if err != nil {
		t.Fatalf("List by name failed: %v", err)
	}
	if len(byName) != 1 || byName[0].Name != "Funkhaus" {
		t.Errorf("name filter returned %+v, want [Funkhaus]", byName)
	}

	both, err := repo.List(ctx, strPtr("haus"), strPtr("berlin"))
	if err != nil {
		t.Fatalf("List by name+city failed: %v", err)
	}
	if len(both) != 1 || both[0].Name != "Funkhaus" {
		t.Errorf("name+city filter returned %+v, want [Funkhaus]", both)
	}

	none, err := repo.List(ctx, strPtr("no-such-venue"), nil)
	if err != nil {
		t.Fatalf("List no-match failed: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 venues, got %d", len(none))
	}
}

func TestRepositoryUpdate_PartialUpdateLeavesOtherFields(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &venue.VenueCreate{
		Name: "Funkhaus", City: "Berlin", Country: "DE",
		Capacity: intPtr(800), Notes: "original notes",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, &venue.VenueUpdate{Name: strPtr("Funkhaus 2")})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "Funkhaus 2" {
		t.Errorf("name = %q, want Funkhaus 2", updated.Name)
	}
	if updated.City != "Berlin" || updated.Country != "DE" {
		t.Errorf("untouched fields changed: %+v", updated)
	}
	if updated.Capacity == nil || *updated.Capacity != 800 {
		t.Errorf("capacity = %v, want 800 (unchanged)", updated.Capacity)
	}
	if updated.Notes != "original notes" {
		t.Errorf("notes = %q, want unchanged", updated.Notes)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) && !updated.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("updated_at = %v, want >= created %v", updated.UpdatedAt, created.UpdatedAt)
	}
}

func TestRepositoryUpdate_EmptyUpdateReturnsCurrent(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &venue.VenueCreate{Name: "Funkhaus", City: "Berlin"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// An empty VenueUpdate must not produce invalid SQL (no SET clauses);
	// the repository falls back to GetByID.
	got, err := repo.Update(ctx, created.ID, &venue.VenueUpdate{})
	if err != nil {
		t.Fatalf("Update with empty body failed: %v", err)
	}
	if got.ID != created.ID || got.Name != "Funkhaus" {
		t.Errorf("empty update returned %+v, want the unchanged venue", got)
	}
}

func TestRepositoryUpdate_NotFound(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)

	_, err := repo.Update(context.Background(), uuid.New(), &venue.VenueUpdate{Name: strPtr("x")})
	if !errors.Is(err, venue.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRepositorySoftDelete_HidesFromGetListAndSearch(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	keep, err := repo.Create(ctx, &venue.VenueCreate{Name: "Keep Club", City: "Berlin"})
	if err != nil {
		t.Fatalf("Create keep failed: %v", err)
	}
	drop, err := repo.Create(ctx, &venue.VenueCreate{Name: "Drop Club", City: "Berlin"})
	if err != nil {
		t.Fatalf("Create drop failed: %v", err)
	}

	if err := repo.SoftDelete(ctx, drop.ID); err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	if _, err := repo.GetByID(ctx, drop.ID); !errors.Is(err, venue.ErrNotFound) {
		t.Errorf("GetByID after soft delete: err = %v, want ErrNotFound", err)
	}

	list, err := repo.List(ctx, nil, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != keep.ID {
		t.Errorf("List after soft delete = %+v, want only the kept venue", list)
	}

	found, err := repo.SearchByName(ctx, "drop", 5)
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("SearchByName found soft-deleted venue: %+v", found)
	}

	// The row still exists with deleted_at set (soft delete, not hard).
	var deletedAt *time.Time
	if err := testPool.QueryRow(ctx, "SELECT deleted_at FROM venues WHERE id = $1", drop.ID).Scan(&deletedAt); err != nil {
		t.Fatalf("deleted row missing: %v", err)
	}
	if deletedAt == nil {
		t.Error("deleted_at is NULL, want a timestamp (soft delete)")
	}

	// Deleting twice is a not-found, not an error-free no-op.
	if err := repo.SoftDelete(ctx, drop.ID); !errors.Is(err, venue.ErrNotFound) {
		t.Errorf("second SoftDelete: err = %v, want ErrNotFound", err)
	}
}

func TestRepositoryUpdate_SoftDeletedVenueIsNotFound(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	created, err := repo.Create(ctx, &venue.VenueCreate{Name: "Funkhaus", City: "Berlin"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.ID); err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	_, err = repo.Update(ctx, created.ID, &venue.VenueUpdate{Name: strPtr("x")})
	if !errors.Is(err, venue.ErrNotFound) {
		t.Fatalf("Update on soft-deleted venue: err = %v, want ErrNotFound", err)
	}
}

func TestRepositorySearchByName_CaseInsensitiveAndLimited(t *testing.T) {
	clearVenues(t)
	repo := venue.NewRepository(testPool)
	ctx := context.Background()

	seed := []string{"Funkhaus", "Funky Town", "Berghain"}
	for _, name := range seed {
		if _, err := repo.Create(ctx, &venue.VenueCreate{Name: name, City: "Berlin"}); err != nil {
			t.Fatalf("seed Create failed: %v", err)
		}
	}

	// Case-insensitive substring match, ordered by name.
	got, err := repo.SearchByName(ctx, "FUNK", 5)
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}
	if len(got) != 2 || got[0].Name != "Funkhaus" || got[1].Name != "Funky Town" {
		t.Errorf("SearchByName(FUNK) = %+v, want [Funkhaus Funky Town]", names(got))
	}

	// Limit is honoured.
	limited, err := repo.SearchByName(ctx, "funk", 1)
	if err != nil {
		t.Fatalf("SearchByName limited failed: %v", err)
	}
	if len(limited) != 1 || limited[0].Name != "Funkhaus" {
		t.Errorf("SearchByName(funk, 1) = %+v, want [Funkhaus]", names(limited))
	}

	// No match returns an empty slice, not nil-with-error.
	none, err := repo.SearchByName(ctx, "zzz-no-match", 5)
	if err != nil {
		t.Fatalf("SearchByName no-match failed: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("SearchByName(zzz) = %+v, want empty", names(none))
	}
}

func names(venues []*venue.Venue) []string {
	out := make([]string, 0, len(venues))
	for _, v := range venues {
		out = append(out, v.Name)
	}
	return out
}
