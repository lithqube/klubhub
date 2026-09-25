// Package gig_test — Plan C / v1.0.0 integration smoke test.
//
// Exercises the full gig lifecycle against a real Postgres (testcontainers)
// and runs the embedded goose migrations to validate the renamed
// 006..010_* numeric-prefixed files (Plan B.1) actually apply.
//
// Run with `go test ./...` (no -short). -short skips it.
package gig_test

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

		"github.com/jackc/pgx/v5/pgxpool"
		_ "github.com/jackc/pgx/v5/stdlib"
		"github.com/shopspring/decimal"
		"github.com/testcontainers/testcontainers-go"
		tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
		"github.com/testcontainers/testcontainers-go/wait"

		"encoding/json"
		"net/http"
		"net/http/httptest"
		"github.com/google/uuid"

		"github.com/klubhub/dj/api/internal/gig"
		"github.com/klubhub/dj/api/internal/platform/migrations"
				"github.com/klubhub/dj/api/internal/tracklist"
	)

var (
	testPool     *pgxpool.Pool
	testDB       *sql.DB
	testTeardown func()
)

func TestMain(m *testing.M) {
	// TestMain runs before the testing package parses flags. Parse them
	// explicitly so -short really skips container startup.
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
		// No Docker — print and skip cleanly so contributors without
		// a local Docker socket can still run `go test -short ./...`.
		println("testcontainers: cannot start postgres, skipping integration:", err.Error())
		os.Exit(m.Run())
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		println("testcontainers: connection string:", err.Error())
		os.Exit(1)
	}

	testDB, err = sql.Open("pgx", connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("sql.Open:", err.Error())
		os.Exit(1)
	}

	if err := migrations.RunMigrations(testDB); err != nil {
		_ = testDB.Close()
		_ = container.Terminate(ctx)
		println("RunMigrations:", err.Error())
		os.Exit(1)
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		_ = testDB.Close()
		_ = container.Terminate(ctx)
		println("pgxpool.New:", err.Error())
		os.Exit(1)
	}

	testTeardown = func() {
		testPool.Close()
		testDB.Close()
		_ = container.Terminate(context.Background())
	}

	code := m.Run()
	if testTeardown != nil {
		testTeardown()
	}
	os.Exit(code)
}

func TestIntegration_FullGigLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := gig.NewRepository(testPool)
	svc := gig.NewService(repo, nil, nil, nil, nil)

	fee, _ := decimal.NewFromString("250.00")
	in := &gig.GigCreate{
		Date:        time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC),
		Venue:       "Test venue",
		City:        "Berlin",
		Country:     "DE",
		EventName:   "Test event",
		FeeAmount:   fee,
		FeeCurrency: "EUR",
	}

	created, err := svc.CreateGig(ctx, in)
	if err != nil {
		t.Fatalf("CreateGig: %v", err)
	}
	if created.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("CreateGig returned nil UUID")
	}

	got, err := svc.GetGig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetGig: %v", err)
	}
	if got.Venue != "Test venue" || got.EventName != "Test event" {
		t.Fatalf("GetGig returned wrong gig: %+v", got)
	}

	city := "Berlin"
	list, err := svc.ListGigs(ctx, gig.GigFilter{City: &city})
	if err != nil {
		t.Fatalf("ListGigs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListGigs returned %d, want 1", len(list))
	}

	played := gig.GigStatusPlayed
	updated, err := svc.UpdateGig(ctx, created.ID, &gig.GigUpdate{
		Status:    &played,
		UpdatedAt: got.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("UpdateGig to played: %v", err)
	}
	if updated.Status != gig.GigStatusPlayed {
		t.Fatalf("UpdateGig status: %s, want %s", updated.Status, gig.GigStatusPlayed)
	}

	cancelled := gig.GigStatusCancelled
	updated, err = svc.UpdateGig(ctx, created.ID, &gig.GigUpdate{
		Status:    &cancelled,
		UpdatedAt: updated.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("UpdateGig to cancelled: %v", err)
	}

	confirmed := gig.GigStatusConfirmed
	_, err = svc.UpdateGig(ctx, created.ID, &gig.GigUpdate{
		Status:    &confirmed,
		UpdatedAt: updated.UpdatedAt,
	})
	if err == nil {
		t.Fatal("UpdateGig cancelled -> confirmed must be rejected (cancelled is terminal)")
	}
	if !strings.Contains(err.Error(), "forbidden") &&
		!errors.Is(err, gig.ErrForbidden) {
		t.Fatalf("expected forbidden error from cancelled re-open, got: %v", err)
	}

	if err := svc.DeleteGig(ctx, created.ID); err != nil {
		t.Fatalf("DeleteGig: %v", err)
	}

	list, err = svc.ListGigs(ctx, gig.GigFilter{City: &city})
	if err != nil {
		t.Fatalf("ListGigs after delete: %v", err)
	}
	for _, g := range list {
		if g.ID == created.ID {
			t.Fatal("deleted gig still returned by ListGigs")
		}
	}
}

func TestIntegration_MigrationsRunClean(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testDB == nil {
		t.Skip("postgres unavailable")
	}
	// All migrations already ran in TestMain. Just assert that the
	// expected tables exist.
	rows, err := testDB.Query(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public'
		ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables[name] = true
	}

		expected := []string{"gigs", "venues", "contacts"}
		for _, want := range expected {
			if !tables[want] {
				t.Errorf("expected table %q to exist after migrations; got: %v", want, tables)
			}
		}
	}

	// ─── Tracklist↔Gig linking integration tests ────────────────────────────────

	func TestIntegration_LinkTracklist_Success(t *testing.T) {
		if testing.Short() {
			t.Skip("integration test")
		}
		if testPool == nil {
			t.Skip("postgres unavailable")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Create tracklist repository and service (no storage/artwork needed for this test)
		trackRepo := tracklist.NewRepository(testPool)
		// tracklist service not needed for this test

		// Create a tracklist
		tlID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
		tl := &tracklist.Tracklist{
			ID:              tlID,
			Title:           "Closing Set",
			SourceFormat:    "rekordbox",
			RawFilePath:     "/tmp/set.txt",
			Preset:          "story",
			VisibleFields:   `["title","artist"]`,
			BgMode:          "solid",
			BgValue:         "#000000",
			MaxTracks:       20,
			TrackRangeStart: 1,
			TrackRangeEnd:   10,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}
		if err := trackRepo.Create(ctx, tl, []tracklist.Track{}); err != nil {
			t.Fatalf("Create tracklist: %v", err)
		}

		// Create gig repository and service
		gigRepo := gig.NewRepository(testPool)
		gigSvc := gig.NewService(gigRepo, nil, nil, trackRepo, nil)

		fee, _ := decimal.NewFromString("500.00")
		gigIn := &gig.GigCreate{
			Date:        time.Date(2026, 10, 1, 21, 0, 0, 0, time.UTC),
			Venue:       "Berghain",
			City:        "Berlin",
			Country:     "DE",
			EventName:   "Monday Night",
			FeeAmount:   fee,
			FeeCurrency: "EUR",
		}
		g, err := gigSvc.CreateGig(ctx, gigIn)
		if err != nil {
			t.Fatalf("CreateGig: %v", err)
		}

		// Verify tracklist is not linked yet
		detail, err := gigSvc.GetGigDetail(ctx, g.ID)
		if err != nil {
			t.Fatalf("GetGigDetail before link: %v", err)
		}
		if len(detail.Tracklists) != 0 {
			t.Fatalf("expected 0 tracklists before link, got %d", len(detail.Tracklists))
		}

		// Link tracklist to gig
		if err := gigSvc.LinkTracklist(ctx, g.ID, tlID); err != nil {
			t.Fatalf("LinkTracklist: %v", err)
		}

		// Verify linked — tracklist should appear in detail
		detail, err = gigSvc.GetGigDetail(ctx, g.ID)
		if err != nil {
			t.Fatalf("GetGigDetail after link: %v", err)
		}
		if len(detail.Tracklists) != 1 {
			t.Fatalf("expected 1 tracklist after link, got %d; detail: %+v", len(detail.Tracklists), detail)
		}
		if detail.Tracklists[0].ID != tlID {
			t.Fatalf("tracklist ID mismatch: got %s want %s", detail.Tracklists[0].ID, tlID)
		}
		if detail.Tracklists[0].Title != "Closing Set" {
			t.Fatalf("tracklist title mismatch: got %q", detail.Tracklists[0].Title)
		}

		// Duplicate link — must be idempotent, no error, no duplicate row
		if err := gigSvc.LinkTracklist(ctx, g.ID, tlID); err != nil {
			t.Fatalf("LinkTracklist (duplicate): %v", err)
		}
		detail, err = gigSvc.GetGigDetail(ctx, g.ID)
		if err != nil {
			t.Fatalf("GetGigDetail after duplicate link: %v", err)
		}
		if len(detail.Tracklists) != 1 {
			t.Fatalf("expected still 1 tracklist after duplicate link, got %d", len(detail.Tracklists))
		}

		// Unlink
		if err := gigSvc.UnlinkTracklist(ctx, g.ID, tlID); err != nil {
			t.Fatalf("UnlinkTracklist: %v", err)
		}

		// Verify unlinked
		detail, err = gigSvc.GetGigDetail(ctx, g.ID)
		if err != nil {
			t.Fatalf("GetGigDetail after unlink: %v", err)
		}
		if len(detail.Tracklists) != 0 {
			t.Fatalf("expected 0 tracklists after unlink, got %d", len(detail.Tracklists))
		}
	}

	func TestIntegration_LinkTracklist_NonexistentTracklist(t *testing.T) {
		if testing.Short() {
			t.Skip("integration test")
		}
		if testPool == nil {
			t.Skip("postgres unavailable")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		trackRepo := tracklist.NewRepository(testPool)
		gigRepo := gig.NewRepository(testPool)
		gigSvc := gig.NewService(gigRepo, nil, nil, trackRepo, nil)

		fee, _ := decimal.NewFromString("200.00")
		gigIn := &gig.GigCreate{
			Date: time.Date(2026, 11, 15, 22, 0, 0, 0, time.UTC),
			Venue: "Tresor", City: "Berlin", Country: "DE",
			EventName: "Weekend Special", FeeAmount: fee, FeeCurrency: "EUR",
		}
		g, err := gigSvc.CreateGig(ctx, gigIn)
		if err != nil {
			t.Fatalf("CreateGig: %v", err)
		}

		// Attempt to link a non-existent tracklist
		badID := uuid.MustParse("deadbeef-dead-beef-dead-beefdeadbeef")
		if err := gigSvc.LinkTracklist(ctx, g.ID, badID); err == nil {
			t.Fatal("LinkTracklist with nonexistent tracklist should fail")
		}
	}

	func TestIntegration_UnlinkTracklist_NonexistentLink(t *testing.T) {
		if testing.Short() {
			t.Skip("integration test")
		}
		if testPool == nil {
			t.Skip("postgres unavailable")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		trackRepo := tracklist.NewRepository(testPool)
		gigRepo := gig.NewRepository(testPool)
		gigSvc := gig.NewService(gigRepo, nil, nil, trackRepo, nil)

		// Create tracklist + gig, link, then unlink twice (second unlink is a no-op)
		tlID := uuid.MustParse("bbbbbbbb-cccc-dddd-eeee-ffffffffffff")
		tl := &tracklist.Tracklist{
			ID: tlID, Title: "Openers", SourceFormat: "export",
			RawFilePath: "/tmp/open.txt", Preset: "mosaic",
			VisibleFields: `["artist","bpm"]`, BgMode: "upload",
			BgValue: "#ffffff", MaxTracks: 15,
			TrackRangeStart: 1, TrackRangeEnd: 15,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := trackRepo.Create(ctx, tl, []tracklist.Track{}); err != nil {
			t.Fatalf("Create tracklist: %v", err)
		}

		fee, _ := decimal.NewFromString("300.00")
		gigIn := &gig.GigCreate{
			Date: time.Date(2026, 9, 20, 20, 0, 0, 0, time.UTC),
			Venue: "Watergate", City: "Berlin", Country: "DE",
			EventName: "Friday Night", FeeAmount: fee, FeeCurrency: "EUR",
		}
		g, err := gigSvc.CreateGig(ctx, gigIn)
		if err != nil {
			t.Fatalf("CreateGig: %v", err)
		}

		if err := gigSvc.LinkTracklist(ctx, g.ID, tlID); err != nil {
			t.Fatalf("LinkTracklist: %v", err)
		}
		if err := gigSvc.UnlinkTracklist(ctx, g.ID, tlID); err != nil {
			t.Fatalf("first UnlinkTracklist: %v", err)
		}
		// Second unlink of the same (now absent) link — must not error.
		if err := gigSvc.UnlinkTracklist(ctx, g.ID, tlID); err != nil {
			t.Fatalf("second UnlinkTracklist (nonexistent link) should not fail: %v", err)
		}
	}

	func TestIntegration_LinkTracklist_HandlerRoutes(t *testing.T) {
		if testing.Short() {
			t.Skip("integration test")
		}
		if testPool == nil {
			t.Skip("postgres unavailable")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		trackRepo := tracklist.NewRepository(testPool)
		gigRepo := gig.NewRepository(testPool)
		gigSvc := gig.NewService(gigRepo, nil, nil, trackRepo, nil)

		h := gig.NewHandler(gigSvc, "test-secret")
	h.SetRAImportHandler(gig.NewRAImportHandler(nil, nil, nil, nil))

	router := h.Routes()

		// Create tracklist + gig via services
		tlID := uuid.MustParse("cccccccc-cccc-dddd-eeee-ffffffffffff")
		tl := &tracklist.Tracklist{
			ID: tlID, Title: "DJ Set", SourceFormat: "rekordbox",
			RawFilePath: "/tmp/dj.txt", Preset: "story",
			VisibleFields: `["title","artist","bpm"]`, BgMode: "solid",
			BgValue: "#1a1a2e", MaxTracks: 25,
			TrackRangeStart: 1, TrackRangeEnd: 25,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := trackRepo.Create(ctx, tl, []tracklist.Track{}); err != nil {
			t.Fatalf("Create tracklist: %v", err)
		}

		fee, _ := decimal.NewFromString("450.00")
		gigIn := &gig.GigCreate{
			Date: time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC),
			Venue: "Khodok", City: "Berlin", Country: "DE",
			EventName: "Warm-up Night", FeeAmount: fee, FeeCurrency: "EUR",
		}
		g, err := gigSvc.CreateGig(ctx, gigIn)
		if err != nil {
			t.Fatalf("CreateGig: %v", err)
		}

		// POST /{id}/tracklists/{tracklistId}
		linkURL := "/" + g.ID.String() + "/tracklists/" + tlID.String()
		req := httptest.NewRequest(http.MethodPost, linkURL, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("POST link: status=%d body=%s", rec.Code, rec.Body.String())
		}

		// Verify via GET /{id}/detail
		detailURL := "/" + g.ID.String() + "/detail"
		req2 := httptest.NewRequest(http.MethodGet, detailURL, nil)
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("GET detail after link: status=%d body=%s", rec2.Code, rec2.Body.String())
		}
		t.Logf("GET detail body=%s", rec2.Body.String())
		var detailEnv struct {
			Data gig.GigDetailResponse `json:"data"`
		}
		if err := json.Unmarshal(rec2.Body.Bytes(), &detailEnv); err != nil {
			t.Fatalf("unmarshal detail: %v", err)
		}
		detail := detailEnv.Data
		if len(detail.Tracklists) != 1 {
			t.Fatalf("expected 1 tracklist in detail after link, got %d", len(detail.Tracklists))
		}

		// DELETE /{id}/tracklists/{tracklistId}
		req3 := httptest.NewRequest(http.MethodDelete, linkURL, nil)
		rec3 := httptest.NewRecorder()
		router.ServeHTTP(rec3, req3)
		if rec3.Code != http.StatusNoContent {
			t.Fatalf("DELETE unlink: status=%d body=%s", rec3.Code, rec3.Body.String())
		}

		// Verify gone
		req4 := httptest.NewRequest(http.MethodGet, detailURL, nil)
		rec4 := httptest.NewRecorder()
		router.ServeHTTP(rec4, req4)
		if rec4.Code != http.StatusOK {
			t.Fatalf("GET detail after unlink: status=%d", rec4.Code)
		}
		var detailEnv2 struct {
			Data gig.GigDetailResponse `json:"data"`
		}
		if err := json.Unmarshal(rec4.Body.Bytes(), &detailEnv2); err != nil {
			t.Fatalf("unmarshal detail after unlink: %v", err)
		}
		detail2 := detailEnv2.Data
		if len(detail2.Tracklists) != 0 {
			t.Fatalf("expected 0 tracklists after unlink, got %d", len(detail2.Tracklists))
		}
	}

	func TestIntegration_LinkTracklist_HandlerBadUUID(t *testing.T) {
		if testing.Short() {
			t.Skip("integration test")
		}
		if testPool == nil {
			t.Skip("postgres unavailable")
		}

	h := gig.NewHandler(nil, "test-secret")
	h.SetRAImportHandler(gig.NewRAImportHandler(nil, nil, nil, nil))

	router := h.Routes()

	// Bad gig UUID
	req := httptest.NewRequest(http.MethodPost, "/not-a-uuid/tracklists/some-tracklist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST bad gig UUID: status=%d want 400", rec.Code)
		}

		// Bad tracklist UUID
		req2 := httptest.NewRequest(http.MethodPost, "/00000000-0000-0000-0000-000000000001/tracklists/not-a-uuid", nil)
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusBadRequest {
			t.Fatalf("POST bad tracklist UUID: status=%d want 400", rec2.Code)
		}
	}
