package report_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/door"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/guest"
	"github.com/klubhub/dj/api/internal/promoter/identity"
	"github.com/klubhub/dj/api/internal/promoter/report"
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

const staffSub = "local:0190f1d2-7c1a-7a00-9f00-00000000beef"

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type fixture struct {
	t      *testing.T
	org    uuid.UUID
	ctx    context.Context
	clock  *clock
	id     *identity.Service
	events *event.Service
	guests *guest.Service
	mux    http.Handler
}

func newOrg(t *testing.T) uuid.UUID {
	t.Helper()
	org := uuid.Must(uuid.NewV7())
	if err := tenantdb.WithTenant(context.Background(), testDB.App, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Nachtwerk', $2)`, org, "nw-"+org.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return org
}

func setup(t *testing.T) *fixture {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	org := newOrg(t)
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	keys := envelope.NewKeyring(kek)
	db := tenantdb.New(testDB.App)
	c := &clock{t: time.Now().UTC()}
	f := &fixture{
		t: t, org: org, ctx: tenantdb.ContextWithTenant(context.Background(), org), clock: c,
		id: identity.NewService(db, keys, identity.Options{
			Argon2: auth.Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
		}),
		events: event.NewService(db, keys, nil),
		guests: guest.NewService(db, keys, nil),
	}
	engine, err := authz.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	reg := authz.NewRegistry()
	door.NewHandler(door.NewService(db, keys, f.id, nil)).Mount(r, engine, reg, nil)
	report.NewHandler(report.NewService(db, keys, c.now)).Mount(r, engine, reg, nil)
	f.mux = r
	return f
}

func (f *fixture) staff() authz.Principal {
	return authz.Principal{Sub: staffSub, OrgID: f.org.String(), Roles: []string{"owner"}, AMR: []string{"pwd", "otp"}, AuthTime: time.Now()}
}

func (f *fixture) call(org uuid.UUID, p authz.Principal, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	ctx := authz.ContextWithPrincipal(req.Context(), p)
	req = req.WithContext(tenantdb.ContextWithTenant(ctx, org))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

// must unwraps a seeding call; a failure aborts the test binary's current
// test with the error (seeding never fails on a healthy database).
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func (f *fixture) night(t *testing.T, title string, start time.Time) event.Detail {
	t.Helper()
	capacity := 300
	return must(f.events.CreateEvent(f.ctx, event.EventInput{
		Title: title, StartsAt: start, EndsAt: start.Add(8 * time.Hour), Timezone: "Europe/Berlin", City: "Berlin", Capacity: &capacity,
	}))
}

// device registers a door device for the event and returns its principal.
func (f *fixture) device(t *testing.T, eventID uuid.UUID, label string) authz.Principal {
	t.Helper()
	ctx := context.Background()
	mgr := authz.Principal{OrgID: f.org.String(), Roles: []string{"owner"}}
	d := must(f.id.RegisterDoorDevice(ctx, mgr, label))
	pin := must(f.id.SetDoorPIN(ctx, mgr, eventID, time.Now().Add(10*time.Hour)))
	res := must(f.id.DoorLogin(ctx, d.Token, eventID, pin))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: res.Cookie})
	return must(f.id.Authenticate(r))
}

func nonce() string { return "rep-" + uuid.NewString() }

func at(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func in(kind string, id uuid.UUID, n int, dir string, when time.Time) door.Op {
	return door.Op{Nonce: nonce(), Type: door.OpCheckin, Subject: &door.Subject{Kind: kind, ID: id.String()}, Count: n, Direction: dir, At: at(when)}
}

func counter(kind string, n int, when time.Time) door.Op {
	return door.Op{Nonce: nonce(), Type: door.OpCounter, Kind: kind, Delta: n, At: at(when)}
}

func undo(target door.Op, when time.Time) door.Op {
	return door.Op{Nonce: nonce(), Type: door.OpUndo, Target: target.Nonce, At: at(when)}
}

func (f *fixture) sync(t *testing.T, p authz.Principal, ops ...door.Op) door.SyncResult {
	t.Helper()
	rec := f.call(f.org, p, http.MethodPost, "/api/v1/door/checkins", door.SyncInput{Ops: ops})
	var res door.SyncResult
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("sync: %d %s", rec.Code, rec.Body)
	}
	for _, r := range res.Results {
		if r.Status != door.StatusApplied {
			t.Fatalf("op %s: %+v", r.Nonce, r)
		}
	}
	return res
}

func count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.Owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const raCSV = "Order ID;Ticket ID;First name;Last name;Email;Ticket type;Barcode;Status\n" +
	"9001;T-1;Lena;Tickets;lena@example.org;Tier 1;RA-SECRET-1;Valid\n" +
	"9002;T-2;Kofi;Mensah;;Tier 2;RA-SECRET-2;Refunded\n"

type seeded struct {
	ev          event.Detail
	start       time.Time
	artists     guest.List
	comp        guest.List
	ben         guest.Allocation
	revoked     guest.Allocation
	elsewhere   guest.Allocation
	door1       authz.Principal
	door2       authz.Principal
	lena, sumG  guest.Guest
	kim         guest.Guest
	olga, dan   guest.Guest
	ticketValid uuid.UUID
}

// seed builds one event with two lists (one with an artist allocation, one
// with a revoked allocation), imported tickets, and check-ins from two
// devices including an undone row, an out, walk-ups, manual counters and a
// cross-device conflict. Door activity starts at the event start, which is
// on a full hour (Berlin is a whole-hour offset, so local :00 too).
func seed(t *testing.T, f *fixture) seeded {
	t.Helper()
	var s seeded
	s.start = time.Now().UTC().Add(time.Hour).Truncate(time.Hour)
	s.ev = f.night(t, "Klubnacht", s.start)
	other := f.night(t, "Other night", s.start.Add(48*time.Hour))

	s.artists = must(f.guests.CreateList(f.ctx, s.ev.ID, guest.ListInput{Name: "Artists", Type: "artist", CollectContact: true}))
	s.comp = must(f.guests.CreateList(f.ctx, s.ev.ID, guest.ListInput{Name: "Comp", Type: "comp"}))
	s.ben = must(f.guests.CreateAllocation(f.ctx, s.ev.ID, s.artists.ID, guest.AllocationInput{Label: "Ben Klock", Quota: 10, PlusNMax: 2}))
	s.revoked = must(f.guests.CreateAllocation(f.ctx, s.ev.ID, s.comp.ID, guest.AllocationInput{Label: "Old Promoter", Quota: 5}))
	if err := f.guests.RevokeAllocation(f.ctx, s.ev.ID, s.comp.ID, s.revoked.ID); err != nil {
		t.Fatal(err)
	}
	otherList := must(f.guests.CreateList(f.ctx, other.ID, guest.ListInput{Name: "Artists", Type: "artist"}))
	s.elsewhere = must(f.guests.CreateAllocation(f.ctx, other.ID, otherList.ID, guest.AllocationInput{Label: "Elsewhere", Quota: 3}))

	added := must(f.guests.AddGuests(f.ctx, s.ev.ID, guest.AddInput{ListID: s.artists.ID, AllocationID: &s.ben.ID, Guests: []guest.GuestInput{
		{Name: "Lena Vogt", PlusN: 2, Email: "lena.vogt@example.org", Phone: "+49 30 1234567"},
		{Name: "=SUM(A1)"},
		{Name: "Kim Pending", PlusN: 1, Status: guest.StatusPending},
	}}, staffSub)).Added
	s.lena, s.sumG, s.kim = added[0], added[1], added[2]
	added = must(f.guests.AddGuests(f.ctx, s.ev.ID, guest.AddInput{ListID: s.comp.ID, Guests: []guest.GuestInput{
		{Name: "Olga", PlusN: 1}, {Name: "Dan"},
	}}, staffSub)).Added
	s.olga, s.dan = added[0], added[1]
	tab := must(guest.ParseCSV([]byte(raCSV)))
	must(f.guests.ImportAttendees(f.ctx, s.ev.ID, guest.ImportInput{Preset: guest.PresetRA, Table: tab}, false, staffSub))
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT id FROM order_positions WHERE event_id = $1 AND status = 'valid'`, s.ev.ID).Scan(&s.ticketValid); err != nil {
		t.Fatal(err)
	}

	s.door1 = f.device(t, s.ev.ID, "Door 1")
	s.door2 = f.device(t, s.ev.ID, "Door 2")
	m := func(min int) time.Time { return s.start.Add(time.Duration(min) * time.Minute) }

	// Door 2 admits Dan first, so Door 1's later admit of Dan is a conflict.
	f.sync(t, s.door2, in(door.KindGuest, s.dan.ID, 1, door.DirIn, m(10)))
	sumIn := in(door.KindGuest, s.sumG.ID, 1, door.DirIn, m(20))
	wrongWalkup := counter(door.CounterWalkup, 5, m(30))
	res := f.sync(t, s.door1,
		in(door.KindGuest, s.lena.ID, 2, door.DirIn, m(5)),
		in(door.KindGuest, s.dan.ID, 1, door.DirIn, m(12)),
		sumIn, undo(sumIn, m(21)),
		in(door.KindGuest, s.olga.ID, 1, door.DirIn, m(20)),
		wrongWalkup, undo(wrongWalkup, m(31)),
		counter(door.CounterWalkup, 3, m(35)),
		in(door.KindTicket, s.ticketValid, 1, door.DirIn, m(40)),
		in(door.KindGuest, s.olga.ID, 1, door.DirOut, m(50)),
	)
	if !res.Results[1].Conflict {
		t.Fatalf("Dan on a second device must be a conflict: %+v", res.Results)
	}
	f.sync(t, s.door2,
		in(door.KindGuest, s.lena.ID, 1, door.DirIn, m(52)),
		counter(door.CounterIn, 2, m(55)),
		counter(door.CounterOut, 1, m(56)),
	)
	return s
}

func (f *fixture) report(t *testing.T, eventID uuid.UUID) (report.Report, string) {
	t.Helper()
	rec := f.call(f.org, f.staff(), http.MethodGet, "/api/v1/events/"+eventID.String()+"/report", nil)
	var rep report.Report
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &rep) != nil {
		t.Fatalf("report: %d %s", rec.Code, rec.Body)
	}
	return rep, rec.Body.String()
}

func rate(r *float64) any {
	if r == nil {
		return nil
	}
	return *r
}

func TestReportNumbers(t *testing.T) {
	f := setup(t)
	s := seed(t, f)

	rep, body := f.report(t, s.ev.ID)
	for _, leak := range []string{"Lena", "SUM", "Kim", "Olga", "Dan", "Kofi", "example.org", "1234567", "RA-SECRET"} {
		if strings.Contains(body, leak) {
			t.Errorf("report leaks %q", leak)
		}
	}
	if rep.Event.ID != s.ev.ID || rep.Event.Title != "Klubnacht" || rep.Event.Timezone != "Europe/Berlin" || *rep.Event.Capacity != 300 || !rep.Live {
		t.Fatalf("header: %+v live=%v", rep.Event, rep.Live)
	}
	peakAt := s.start.Add(45 * time.Minute)
	want := report.Totals{
		GuestsGoing: 4, GuestsArrived: 3, HeadsExpected: 3 + 1 + 2 + 1 + 1, HeadsAdmitted: 3 + 1 + 2 + 1,
		PlusOnesAllowed: 3, PlusOnesUsed: 2, TicketsValid: 1, TicketsScanned: 1, Walkups: 3,
		PeakOccupancy: 10, Conflicts: 1,
	}
	got := rep.Totals
	if got.NoShowRate == nil || *got.NoShowRate != 0.25 || got.PeakAt == nil || !got.PeakAt.Equal(peakAt) {
		t.Fatalf("no-show / peak: %v %v", rate(got.NoShowRate), got.PeakAt)
	}
	got.NoShowRate, got.PeakAt = nil, nil
	if got != want {
		t.Fatalf("totals:\n got %+v\nwant %+v", got, want)
	}

	if len(rep.ByList) != 2 {
		t.Fatalf("by_list: %+v", rep.ByList)
	}
	a, c := rep.ByList[0], rep.ByList[1]
	if a.ListID != s.artists.ID || a.Name != "Artists" || a.Type != "artist" || a.Going != 2 || a.Arrived != 1 || rate(a.NoShowRate) != 0.5 ||
		a.HeadsExpected != 4 || a.HeadsAdmitted != 3 || a.PlusOnesAllowed != 2 || a.PlusOnesUsed != 2 {
		t.Errorf("artists list: %+v", a)
	}
	if c.ListID != s.comp.ID || c.Going != 2 || c.Arrived != 2 || rate(c.NoShowRate) != 0.0 || c.HeadsExpected != 3 || c.HeadsAdmitted != 3 ||
		c.PlusOnesAllowed != 1 || c.PlusOnesUsed != 0 {
		t.Errorf("comp list: %+v", c)
	}

	if len(rep.BySubmitter) != 2 {
		t.Fatalf("by_submitter: %+v", rep.BySubmitter)
	}
	b, o := rep.BySubmitter[0], rep.BySubmitter[1]
	if b.AllocationID != s.ben.ID || b.Submitter != "Ben Klock" || b.ListName != "Artists" || b.ListType != "artist" || b.Quota != 10 ||
		b.Going != 2 || b.Arrived != 1 || rate(b.NoShowRate) != 0.5 || b.HeadsAdmitted != 3 || b.Revoked {
		t.Errorf("Ben Klock: %+v", b)
	}
	if o.AllocationID != s.revoked.ID || !o.Revoked || o.Going != 0 || o.NoShowRate != nil || o.ListType != "comp" {
		t.Errorf("revoked allocation: %+v", o)
	}

	if len(rep.TicketsByType) != 2 || rep.TicketsByType[0].Name != "Tier 1" || rep.TicketsByType[0].Valid != 1 || rep.TicketsByType[0].Scanned != 1 ||
		rep.TicketsByType[1].Name != "Tier 2" || rep.TicketsByType[1].Valid != 0 || rep.TicketsByType[1].Scanned != 0 {
		t.Errorf("tickets by type: %+v", rep.TicketsByType)
	}

	wantCurve := []report.Bucket{
		{BucketStart: s.start, In: 4, Out: 0, Walkups: 0, Occupancy: 4},
		{BucketStart: s.start.Add(15 * time.Minute), In: 1, Occupancy: 5},
		{BucketStart: s.start.Add(30 * time.Minute), In: 1, Walkups: 3, Occupancy: 9},
		{BucketStart: s.start.Add(45 * time.Minute), In: 3, Out: 2, Occupancy: 10},
	}
	if len(rep.Curve) != len(wantCurve) {
		t.Fatalf("curve: %+v", rep.Curve)
	}
	for i, w := range wantCurve {
		g := rep.Curve[i]
		if !g.BucketStart.Equal(w.BucketStart) || g.In != w.In || g.Out != w.Out || g.Walkups != w.Walkups || g.Occupancy != w.Occupancy {
			t.Errorf("bucket %d: %+v, want %+v", i, g, w)
		}
	}

	// After the end the report is final.
	f.clock.t = s.ev.EndsAt.Add(time.Hour)
	if rep, _ := f.report(t, s.ev.ID); rep.Live || !rep.GeneratedAt.Equal(f.clock.t) {
		t.Fatalf("after the end: live=%v generated_at=%s", rep.Live, rep.GeneratedAt)
	}
}

func TestReportWithNothingYet(t *testing.T) {
	f := setup(t)
	ev := f.night(t, "Quiet", time.Now().UTC().Add(24*time.Hour).Truncate(time.Hour))
	rep, body := f.report(t, ev.ID)
	if rep.Totals != (report.Totals{}) || len(rep.ByList) != 0 || len(rep.BySubmitter) != 0 || len(rep.TicketsByType) != 0 || len(rep.Curve) != 0 {
		t.Fatalf("empty report: %+v", rep)
	}
	for _, k := range []string{`"by_list":[]`, `"by_submitter":[]`, `"tickets_by_type":[]`, `"curve":[]`, `"no_show_rate":null`, `"peak_at":null`} {
		if !strings.Contains(body, k) {
			t.Errorf("empty report must contain %s: %s", k, body)
		}
	}
}

func TestListBackCSV(t *testing.T) {
	f := setup(t)
	s := seed(t, f)
	path := "/api/v1/events/" + s.ev.ID.String() + "/report/list-back.csv?allocation_id="

	rec := f.call(f.org, f.staff(), http.MethodGet, path+s.ben.ID.String(), nil)
	body := rec.Body.String()
	firstIn := s.start.Add(5 * time.Minute).In(must(time.LoadLocation("Europe/Berlin"))).Format("2006-01-02 15:04")
	want := "\xef\xbb\xbfname,plus_n,status,arrived,heads_admitted,first_in_local\n" +
		"Lena Vogt,2,going,yes,3," + firstIn + "\n" +
		"'=SUM(A1),0,going,no,0,\n" +
		"Kim Pending,1,pending,no,0,\n"
	if rec.Code != http.StatusOK || body != want || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		rec.Header().Get("Content-Disposition") != `attachment; filename="klubnacht-list-back-ben-klock.csv"` || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list-back: %d %v\n%q\nwant\n%q", rec.Code, rec.Header(), body, want)
	}
	for _, leak := range []string{"example.org", "1234567", "Olga", "Dan"} {
		if strings.Contains(body, leak) {
			t.Errorf("list-back leaks %q", leak)
		}
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'guestlist.export' AND decision = 'allow' AND actor_id = $2
	  AND resource = $3 AND reason = $4`, f.org, staffSub, "event:"+s.ev.ID.String(), "list-back allocation:"+s.ben.ID.String()+", 3 guests, 1 arrived"); n != 1 {
		t.Fatalf("the list-back must be audited with counts only, got %d", n)
	}

	// Revoked allocations still list back (empty here).
	rec = f.call(f.org, f.staff(), http.MethodGet, path+s.revoked.ID.String(), nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "\xef\xbb\xbfname,plus_n,status,arrived,heads_admitted,first_in_local\n" {
		t.Fatalf("revoked allocation: %d %q", rec.Code, rec.Body)
	}
	// An allocation of another event is not found through this event.
	if rec := f.call(f.org, f.staff(), http.MethodGet, path+s.elsewhere.ID.String(), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("allocation from another event: %d %s", rec.Code, rec.Body)
	}
	if rec := f.call(f.org, f.staff(), http.MethodGet, path+uuid.NewString(), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown allocation: %d", rec.Code)
	}
	if rec := f.call(f.org, f.staff(), http.MethodGet, path+"nope", nil); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"allocation_id"`) {
		t.Fatalf("bad allocation id: %d %s", rec.Code, rec.Body)
	}
	if rec := f.call(f.org, f.staff(), http.MethodGet, "/api/v1/events/"+uuid.NewString()+"/report/list-back.csv?allocation_id="+s.ben.ID.String(), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown event: %d", rec.Code)
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'guestlist.export'`, f.org); n != 2 {
		t.Fatalf("only successful exports are audited, got %d", n)
	}
}

func TestReportTenantIsolationAndDoorDenied(t *testing.T) {
	f := setup(t)
	s := seed(t, f)
	reportPath := "/api/v1/events/" + s.ev.ID.String() + "/report"
	listBack := reportPath + "/list-back.csv?allocation_id=" + s.ben.ID.String()

	intruderOrg := newOrg(t)
	intruder := authz.Principal{Sub: staffSub, OrgID: intruderOrg.String(), Roles: []string{"owner"}, AMR: []string{"pwd", "otp"}, AuthTime: time.Now()}
	for _, p := range []string{reportPath, listBack} {
		if rec := f.call(intruderOrg, intruder, http.MethodGet, p, nil); rec.Code != http.StatusNotFound {
			t.Errorf("another tenant must not see %s: %d %s", p, rec.Code, rec.Body)
		}
	}

	for _, p := range []string{reportPath, listBack} {
		rec := f.call(f.org, s.door1, http.MethodGet, p, nil)
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "Lena") {
			t.Errorf("door staff must not read %s: %d %s", p, rec.Code, rec.Body)
		}
	}
	if rec := f.call(f.org, f.staff(), http.MethodGet, "/api/v1/events/not-an-id/report", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("bad event id: %d", rec.Code)
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'guestlist.export'`, f.org); n != 0 {
		t.Fatalf("refused list-backs must not be audited as exports, got %d", n)
	}
}
