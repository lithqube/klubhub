package release_test

// Integration test for the release repository: CRUD round-trip against a
// real Postgres with the embedded goose migrations applied. -short skips it.

import (
	"context"
	"database/sql"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/release"
)

var (
	testPool     *pgxpool.Pool
	testTeardown func()
)

func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("SKIP_INTEGRATION") == "1" || testing.Short() {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		tcpostgres.WithDatabase("klubhub_test"),
		tcpostgres.WithUsername("klubhub"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		println("testcontainers: cannot start postgres, skipping integration:", err.Error())
		os.Exit(m.Run())
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	if err := migrations.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		println("RunMigrations:", err.Error())
		os.Exit(1)
	}
	_ = db.Close()

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	testTeardown = func() {
		testPool.Close()
		_ = container.Terminate(context.Background())
	}
	code := m.Run()
	if testTeardown != nil {
		testTeardown()
	}
	os.Exit(code)
}

func TestIntegration_ReleaseCRUDRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}

	ctx := context.Background()
	repo := release.NewRepository(testPool)
	svc := release.NewService(repo)

	day := time.Date(2026, 12, 11, 0, 0, 0, 0, time.UTC)
	created, err := svc.Create(ctx, release.Input{
		Title:       "Night Signal",
		Artists:     "Ada, Juno",
		Label:       "KlubHub",
		Distributor: "Self",
		ReleaseDate: day,
		Format:      release.FormatBoth,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("create did not assign an ID")
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Night Signal" || got.Artists != "Ada, Juno" || got.Format != release.FormatBoth {
		t.Errorf("round trip lost fields: %+v", got)
	}
	if !got.ReleaseDate.Equal(day) {
		t.Errorf("release_date = %s, want %s", got.ReleaseDate, day)
	}
	if got.Status != release.StatusDemoSent {
		t.Errorf("status = %s, want demo_sent", got.Status)
	}

	// The workflow moves through the real database, not just in memory.
	for _, to := range []release.ReleaseStatus{release.StatusAccepted, release.StatusMastering} {
		if _, err := svc.Transition(ctx, created.ID, to); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
	got, err = svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != release.StatusMastering {
		t.Errorf("status = %s, want mastering", got.Status)
	}

	// Edit, then delete; a deleted release disappears from reads.
	if _, err := svc.Update(ctx, created.ID, release.Input{Title: "Night Signal (EP)", ReleaseDate: day}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Night Signal (EP)" || got.Status != release.StatusMastering {
		t.Errorf("update changed the wrong things: %+v", got)
	}

	second, err := svc.Create(ctx, release.Input{Title: "Blue Hour", ReleaseDate: day.AddDate(0, 1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list has %d releases, want 2", len(list))
	}
	if list[0].ID != created.ID || list[1].ID != second.ID {
		t.Errorf("list order = %s, %s — want the earlier release date first", list[0].ID, list[1].ID)
	}

	if err := svc.Delete(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, second.ID); err != release.ErrNotFound {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
	list, err = svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list has %d releases after delete, want 1", len(list))
	}
}
