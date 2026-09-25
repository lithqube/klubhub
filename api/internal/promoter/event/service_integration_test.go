package event_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/event"
)

var testDB *pgtest.DB

func TestMain(m *testing.M) {
	flag.Parse()
	db, err := pgtest.StartPromoter()
	if err != nil {
		panic(err)
	}
	testDB = db
	code := m.Run()
	db.Close()
	os.Exit(code)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

var berlin, _ = time.LoadLocation("Europe/Berlin")

func at(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, berlin) }
func ptr[T any](v T) *T          { return &v }

type fixture struct {
	svc   *event.Service
	ctx   context.Context
	clock *clock
	org   uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	org := uuid.Must(uuid.NewV7())
	if err := tenantdb.WithTenant(context.Background(), testDB.App, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Nachtwerk', $2)`, org, "nw-"+org.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	c := &clock{t: at(1, 12, 0)}
	return fixture{
		svc:   event.NewService(tenantdb.New(testDB.App), envelope.NewKeyring(kek), c.now),
		ctx:   tenantdb.ContextWithTenant(context.Background(), org),
		clock: c, org: org,
	}
}

func (f fixture) venue(t *testing.T) event.Venue {
	t.Helper()
	v, err := f.svc.CreateVenue(f.ctx, event.VenueInput{
		Name: "Tresor.West", City: "Berlin", Country: ptr("DE"), Timezone: "Europe/Berlin", Capacity: ptr(1200),
		CurfewLocal: ptr("05:00"), TechNotes: "2x CDJ-3000",
		Rooms:     []event.RoomInput{{Name: "Main Room", Capacity: ptr(800)}, {Name: "Garden"}},
		Protected: &event.VenueProtected{Address: "Unterstraße 3", ContactName: "Hans Tech", ContactPhone: "+49 30 1234567"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) night(t *testing.T, venueID *uuid.UUID, mode string, reveal *time.Time) event.Detail {
	t.Helper()
	d, err := f.svc.CreateEvent(f.ctx, event.EventInput{
		Title: "Klubnacht 03", StartsAt: at(3, 23, 0), EndsAt: at(4, 7, 0), DoorsAt: ptr(at(3, 22, 30)),
		Timezone: "Europe/Berlin", VenueID: venueID, City: "Berlin", LocationMode: mode, LocationRevealAt: reveal,
		ExternalTicketURL: ptr("https://tickets.example/k3"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestVenueProtectedFieldsAreSealedAndRevealIsAudited(t *testing.T) {
	f := setup(t)
	v := f.venue(t)
	if !v.Protected.Address || !v.Protected.Contact || v.Protected.Geo || len(v.Rooms) != 2 {
		t.Fatalf("masked flags / rooms: %+v", v)
	}
	var addr, phone []byte
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT address_enc, contact_phone_enc FROM venues WHERE id = $1`, v.ID).Scan(&addr, &phone); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(addr, []byte("Unterstraße")) || bytes.Contains(phone, []byte("1234567")) {
		t.Fatal("protected venue fields stored in plaintext")
	}
	p, err := f.svc.RevealVenue(f.ctx, v.ID, "local:"+uuid.NewString())
	if err != nil || p.Address != "Unterstraße 3" || p.ContactPhone != "+49 30 1234567" {
		t.Fatalf("reveal: %+v %v", p, err)
	}
	var n int
	_ = testDB.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'venue.reveal' AND tenant_id = $1`, f.org).Scan(&n)
	if n != 1 {
		t.Fatalf("reveal must be audited once, got %d", n)
	}
	// Omitting Protected leaves sealed fields untouched.
	v2, err := f.svc.UpdateVenue(f.ctx, v.ID, event.VenueInput{Name: "Tresor West", City: "Berlin", Timezone: "Europe/Berlin", Rooms: []event.RoomInput{{ID: &v.Rooms[0].ID, Name: "Main"}}})
	if err != nil || !v2.Protected.Address || len(v2.Rooms) != 1 || v2.Rooms[0].ID != v.Rooms[0].ID {
		t.Fatalf("update: %+v %v", v2, err)
	}
}

func TestEventCopiesRoomsAndKeepsSlugsUnique(t *testing.T) {
	f := setup(t)
	v := f.venue(t)
	a := f.night(t, &v.ID, event.LocationVenue, nil)
	b := f.night(t, &v.ID, event.LocationVenue, nil)
	if a.Slug != "klubnacht-03" || b.Slug != "klubnacht-03-2" {
		t.Fatalf("slugs %q %q", a.Slug, b.Slug)
	}
	if len(a.Stages) != 2 || a.Stages[0].Name != "Main Room" || a.Status != event.StatusDraft || a.Venue == nil {
		t.Fatalf("stages / defaults: %+v", a)
	}
	if err := f.svc.ArchiveVenue(f.ctx, v.ID); !errors.Is(err, event.ErrVenueInUse) {
		t.Fatalf("a venue with upcoming events must not archive, got %v", err)
	}
}

func TestConflictsSaveButBlockPublishAndExport(t *testing.T) {
	f := setup(t)
	v := f.venue(t)
	d := f.night(t, &v.ID, event.LocationVenue, nil)
	main := d.Stages[0].ID
	d, err := f.svc.ReplaceLineup(f.ctx, d.ID, d.Version, []event.LineupInput{
		{DisplayName: "Dasha Rush", StageID: &main, SetStart: ptr(at(4, 0, 30)), SetEnd: ptr(at(4, 2, 30))},
		{DisplayName: "Ben Klock", StageID: &main, SetStart: ptr(at(4, 2, 0)), SetEnd: ptr(at(4, 5, 0))},
		{DisplayName: "Oscar Mulero"},
	})
	if err != nil {
		t.Fatalf("a conflicted timetable must save (decision 3): %v", err)
	}
	if !event.HasErrors(d.Issues) || len(d.Lineup) != 3 {
		t.Fatalf("issues must come back with the save: %+v", d.Issues)
	}
	var blocked *event.BlockedError
	if _, err := f.svc.ChangeStatus(f.ctx, d.ID, d.Version, event.StatusPublished); !errors.As(err, &blocked) || blocked.Reason != "timetable_errors" {
		t.Fatalf("publish must be blocked, got %v", err)
	}
	if _, err := f.svc.Export(f.ctx, d.ID); !errors.As(err, &blocked) {
		t.Fatalf("export must be blocked, got %v", err)
	}

	// Fix: Ben Klock starts after the changeover.
	in := []event.LineupInput{}
	for _, e := range d.Lineup {
		x := event.LineupInput{ID: ptr(e.ID), StageID: e.StageID, DisplayName: e.DisplayName, SetStart: e.SetStart, SetEnd: e.SetEnd}
		if e.DisplayName == "Ben Klock" {
			x.SetStart, x.SetEnd = ptr(at(4, 2, 45)), ptr(at(4, 5, 0))
		}
		in = append(in, x)
	}
	d, err = f.svc.ReplaceLineup(f.ctx, d.ID, d.Version, in)
	if err != nil || event.HasErrors(d.Issues) {
		t.Fatalf("fixed timetable: %v %+v", err, d.Issues)
	}
	d, err = f.svc.ChangeStatus(f.ctx, d.ID, d.Version, event.StatusPublished)
	if err != nil || d.Status != event.StatusPublished {
		t.Fatalf("publish after fix: %v", err)
	}
	if _, err := f.svc.ChangeStatus(f.ctx, d.ID, d.Version, event.StatusDraft); !errors.Is(err, event.ErrBadTransition) {
		t.Fatalf("published → draft is not a transition, got %v", err)
	}
}

func TestOptimisticVersionAndCutSets(t *testing.T) {
	f := setup(t)
	d := f.night(t, nil, event.LocationVenue, nil)
	d, _ = f.svc.ReplaceStages(f.ctx, d.ID, d.Version, []event.StageInput{{Name: "Main"}})
	st := d.Stages[0].ID
	d, err := f.svc.ReplaceLineup(f.ctx, d.ID, d.Version, []event.LineupInput{{DisplayName: "Closer", StageID: &st, SetStart: ptr(at(4, 5, 0)), SetEnd: ptr(at(4, 7, 0))}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ReplaceLineup(f.ctx, d.ID, d.Version-1, nil); !errors.Is(err, event.ErrVersionConflict) {
		t.Fatalf("stale version must conflict, got %v", err)
	}
	var cut *event.CutsSetsError
	_, err = f.svc.UpdateEvent(f.ctx, d.ID, d.Version, event.EventInput{Title: "Klubnacht 03", StartsAt: at(3, 23, 0), EndsAt: at(4, 5, 0), Timezone: "Europe/Berlin", City: "Berlin"})
	if !errors.As(err, &cut) || len(cut.Entries) != 1 {
		t.Fatalf("ending at 05:00 must be refused because it cuts the closing set, got %v", err)
	}
	other := f.night(t, nil, event.LocationVenue, nil)
	other, _ = f.svc.ReplaceStages(f.ctx, other.ID, other.Version, []event.StageInput{{Name: "Other"}})
	foreign := other.Stages[0].ID
	var inv *event.InvalidError
	if _, err := f.svc.ReplaceLineup(f.ctx, d.ID, d.Version, []event.LineupInput{{DisplayName: "X", StageID: &foreign}}); !errors.As(err, &inv) {
		t.Fatalf("a stage of another event must be refused, got %v", err)
	}
}

func TestExportHonoursSecretLocation(t *testing.T) {
	f := setup(t)
	v := f.venue(t)
	d := f.night(t, &v.ID, event.LocationSecret, ptr(at(3, 12, 0)))
	x, err := f.svc.Export(f.ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !x.Location.Withheld || x.Location.Address != "" || x.Location.Name == "Tresor.West" || !x.Embargoed {
		t.Fatalf("before reveal: %+v", x)
	}
	f.clock.t = at(3, 12, 1)
	x, _ = f.svc.Export(f.ctx, d.ID)
	if x.Location.Withheld || x.Location.Address != "Unterstraße 3" || x.Location.Name != "Tresor.West" {
		t.Fatalf("after reveal: %+v", x.Location)
	}
}

func TestTenantsCannotSeeEachOthersEvents(t *testing.T) {
	a, b := setup(t), setup(t)
	d := a.night(t, nil, event.LocationVenue, nil)
	if _, err := b.svc.GetEvent(b.ctx, d.ID); !errors.Is(err, event.ErrNotFound) {
		t.Fatalf("another tenant's event must be invisible, got %v", err)
	}
	if list, _ := b.svc.ListEvents(b.ctx, "drafts"); len(list) != 0 {
		t.Fatalf("tenant B lists %d of A's events", len(list))
	}
	if list, _ := a.svc.ListEvents(a.ctx, "drafts"); len(list) != 1 {
		t.Fatalf("tenant A drafts: %d", len(list))
	}
}

func TestSchedulingNeedsAFuturePublishTime(t *testing.T) {
	f := setup(t)
	d := f.night(t, nil, event.LocationVenue, nil)
	var blocked *event.BlockedError
	if _, err := f.svc.ChangeStatus(f.ctx, d.ID, d.Version, event.StatusScheduled); !errors.As(err, &blocked) || blocked.Reason != "publish_at_required" {
		t.Fatalf("got %v", err)
	}
}
