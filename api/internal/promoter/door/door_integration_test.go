package door_test

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

type fixture struct {
	t      *testing.T
	ctx    context.Context // carries the tenant, for the staff-side services
	org    uuid.UUID
	clock  *clock
	id     *identity.Service
	events *event.Service
	guests *guest.Service
	mux    http.Handler
}

const staffSub = "local:0190f1d2-7c1a-7a00-9f00-00000000beef"

func setup(t *testing.T) *fixture {
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
	keys := envelope.NewKeyring(kek)
	db := tenantdb.New(testDB.App)
	c := &clock{t: time.Now().UTC().Truncate(time.Second)}
	f := &fixture{
		t: t, ctx: tenantdb.ContextWithTenant(context.Background(), org), org: org, clock: c,
		id: identity.NewService(db, keys, identity.Options{
			Argon2: auth.Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
			Now:    c.now,
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
	guest.NewHandler(f.guests).Mount(r, engine, reg, nil)
	f.mux = r
	return f
}

func (f *fixture) staff() authz.Principal {
	return authz.Principal{Sub: staffSub, OrgID: f.org.String(), Roles: []string{"owner"}, AMR: []string{"pwd", "otp"}, AuthTime: time.Now()}
}

// manager is the staff principal used to register devices and set PINs
// (no user row exists in these fixtures, so no creator is recorded).
func (f *fixture) manager() authz.Principal {
	return authz.Principal{OrgID: f.org.String(), Roles: []string{"owner"}}
}

func (f *fixture) night(t *testing.T, title string) event.Detail {
	t.Helper()
	start := time.Now().Add(6 * time.Hour).Truncate(time.Hour)
	capacity := 300
	d, err := f.events.CreateEvent(f.ctx, event.EventInput{
		Title: title, StartsAt: start, EndsAt: start.Add(8 * time.Hour), Timezone: "Europe/Berlin", City: "Berlin", Capacity: &capacity,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *fixture) list(t *testing.T, eventID uuid.UUID, name string, contact bool) guest.List {
	t.Helper()
	l, err := f.guests.CreateList(f.ctx, eventID, guest.ListInput{Name: name, Type: "artist", CollectContact: contact,
		EntryTerms: guest.EntryTerms{Perks: []string{"Backstage"}}})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func (f *fixture) add(t *testing.T, eventID, listID uuid.UUID, g ...guest.GuestInput) []guest.Guest {
	t.Helper()
	r, err := f.guests.AddGuests(f.ctx, eventID, guest.AddInput{ListID: listID, Guests: g}, staffSub)
	if err != nil {
		t.Fatal(err)
	}
	return r.Added
}

// device registers a door device, sets the event's staff PIN and logs the
// device in; it returns the door principal of that session.
func (f *fixture) device(t *testing.T, eventID uuid.UUID, label string) authz.Principal {
	t.Helper()
	ctx := context.Background()
	d, err := f.id.RegisterDoorDevice(ctx, f.manager(), label)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := f.id.SetDoorPIN(ctx, f.manager(), eventID, f.clock.t.Add(10*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.id.DoorLogin(ctx, d.Token, eventID, pin)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: res.Cookie})
	p, err := f.id.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *fixture) call(p authz.Principal, method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	ctx := authz.ContextWithPrincipal(req.Context(), p)
	req = req.WithContext(tenantdb.ContextWithTenant(ctx, f.org))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func decodeOK[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (f *fixture) bundle(t *testing.T, p authz.Principal) door.Bundle {
	t.Helper()
	return decodeOK[door.Bundle](t, f.call(p, http.MethodGet, "/api/v1/door/bundle", nil))
}

func (f *fixture) sync(t *testing.T, p authz.Principal, since *string, ops ...door.Op) door.SyncResult {
	t.Helper()
	if ops == nil {
		ops = []door.Op{}
	}
	return decodeOK[door.SyncResult](t, f.call(p, http.MethodPost, "/api/v1/door/checkins", door.SyncInput{Since: since, Ops: ops}))
}

func (f *fixture) adds(t *testing.T, p authz.Principal, adds ...door.Add) door.AddsResult {
	t.Helper()
	return decodeOK[door.AddsResult](t, f.call(p, http.MethodPost, "/api/v1/door/adds", door.AddsInput{Adds: adds}))
}

func count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.Owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func nonce() string {
	return "n-" + uuid.NewString()[:8] + "-" + time.Now().Format("150405.000000")
}

func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func checkin(kind string, id uuid.UUID, n int, dir string) door.Op {
	return door.Op{Nonce: nonce(), Type: door.OpCheckin, Subject: &door.Subject{Kind: kind, ID: id.String()}, Count: n, Direction: dir, At: stamp()}
}

func statuses(rs []door.Result) string {
	var out []string
	for _, r := range rs {
		s := r.Status
		if r.Conflict {
			s += "+conflict"
		}
		if r.Error != "" {
			s += "(" + strings.SplitN(r.Error, ":", 2)[0] + ")"
		}
		out = append(out, s)
	}
	return strings.Join(out, ",")
}

const raCSV = "Order ID;Ticket ID;First name;Last name;Email;Ticket type;Barcode;Status\n" +
	"9001;T-1;Lena;Tickets;lena@example.org;Tier 1;RA-SECRET-1;Valid\n" +
	"9002;T-2;Kofi;Mensah;;Tier 2;RA-SECRET-2;Refunded\n"

func (f *fixture) importTickets(t *testing.T, eventID uuid.UUID) {
	t.Helper()
	tab, err := guest.ParseCSV([]byte(raCSV))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.guests.ImportAttendees(f.ctx, eventID, guest.ImportInput{Preset: guest.PresetRA, Table: tab}, false, staffSub); err != nil {
		t.Fatal(err)
	}
}

func ticketIDs(b door.Bundle) map[string]uuid.UUID {
	out := map[string]uuid.UUID{}
	for _, tk := range b.Tickets {
		if tk.Secret != "" {
			out[tk.Secret] = tk.ID
		}
	}
	return out
}

func TestBundleIsScopedToTheSessionEventAndNameOnly(t *testing.T) {
	f := setup(t)
	a, b := f.night(t, "Klubnacht A"), f.night(t, "Klubnacht B")
	names := f.list(t, a.ID, "Residents", false)
	contacts := f.list(t, a.ID, "Press", true)
	f.add(t, a.ID, names.ID, guest.GuestInput{Name: "Lena Vogt", PlusN: 2, Note: "Tall, red coat"},
		guest.GuestInput{Name: "Declined Dan", Status: guest.StatusDeclined})
	f.add(t, a.ID, contacts.ID, guest.GuestInput{Name: "Mara Press", Email: "mara@example.org", Phone: "+49 30 1234567"})
	f.add(t, b.ID, f.list(t, b.ID, "Other", false).ID, guest.GuestInput{Name: "Olga Elsewhere"})
	f.importTickets(t, a.ID)
	managerPIN, err := f.id.SetManagerPIN(context.Background(), f.manager(), a.ID, f.clock.t.Add(8*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	p := f.device(t, a.ID, "Door A")
	rec := f.call(p, http.MethodGet, "/api/v1/door/bundle", nil)
	body := rec.Body.String()
	bundle := decodeOK[door.Bundle](t, rec)
	for _, leak := range []string{"mara@example.org", "1234567", "Declined Dan", "Olga", "lena@example.org"} {
		if strings.Contains(body, leak) {
			t.Errorf("bundle leaks %q", leak)
		}
	}
	if bundle.Event.ID != a.ID || bundle.Event.Title != "Klubnacht A" || bundle.Event.Capacity == nil || *bundle.Event.Capacity != 300 ||
		bundle.DeviceID.String() != p.DeviceID || !bundle.SessionExpiresAt.Equal(f.clock.t.Add(10*time.Hour)) || bundle.Cursor == "" {
		t.Fatalf("bundle header: %+v", bundle)
	}
	if len(bundle.Lists) != 2 || bundle.Lists[0].EntryTerms.PriceMode != "free" || bundle.Lists[0].EntryTerms.Perks[0] != "backstage" {
		t.Fatalf("lists: %+v", bundle.Lists)
	}
	if len(bundle.Guests) != 2 || bundle.Guests[0].Name != "Lena Vogt" || bundle.Guests[0].PlusN != 2 || bundle.Guests[0].Note != "Tall, red coat" ||
		bundle.Guests[1].Name != "Mara Press" {
		t.Fatalf("guests (declined excluded, names decrypted): %+v", bundle.Guests)
	}
	secrets := ticketIDs(bundle)
	if len(bundle.Tickets) != 2 || len(secrets) != 2 || bundle.Tickets[0].Name != "Lena Tickets" || bundle.Tickets[0].OrderRef != "9001" ||
		bundle.Tickets[0].Source != "ra" || bundle.Tickets[0].TicketType != "Tier 1" || bundle.Tickets[1].Status != "refunded" {
		t.Fatalf("tickets (secrets decrypted, refunded kept with status): %+v", bundle.Tickets)
	}
	if len(bundle.Checkins) != 0 || bundle.Counters != (door.Counters{}) {
		t.Fatalf("empty door state: %+v %+v", bundle.Checkins, bundle.Counters)
	}
	m := bundle.ManagerPIN
	if m == nil || m.Iterations != identity.PINCheckIterations {
		t.Fatalf("manager pin verifier: %+v", m)
	}
	if h, _ := identity.ManagerPINVerifier(managerPIN, m.Salt, m.Iterations); !bytes.Equal(h, m.Hash) {
		t.Fatal("the offline verifier must match the manager PIN")
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'door.bundle_downloaded' AND reason = '2 lists, 2 guests, 2 tickets, 0 check-ins'`, f.org); n != 1 {
		t.Fatalf("the download must be audited with counts, got %d", n)
	}
	if n := count(t, `SELECT count(*) FROM outbox WHERE tenant_id = $1 AND subject LIKE '%.door.bundle_downloaded'
	  AND payload::text NOT LIKE '%Lena%'`, f.org); n != 1 {
		t.Fatalf("the download must reach the outbox (identifiers only), got %d", n)
	}
	if n := count(t, `SELECT count(*) FROM door_devices WHERE tenant_id = $1 AND last_seen_at IS NOT NULL`, f.org); n != 1 {
		t.Fatal("the device's last-seen time must move")
	}
}

func TestSyncIsIdempotentAndSupportsUndoAndCounters(t *testing.T) {
	f := setup(t)
	a := f.night(t, "Klubnacht")
	l := f.list(t, a.ID, "Residents", false)
	lena := f.add(t, a.ID, l.ID, guest.GuestInput{Name: "Lena Vogt", PlusN: 1})[0]
	p := f.device(t, a.ID, "Door A")

	in := checkin(door.KindGuest, lena.ID, 2, door.DirIn)
	walkup := door.Op{Nonce: nonce(), Type: door.OpCounter, Kind: door.CounterWalkup, Delta: 3, At: stamp()}
	bad := door.Op{Nonce: nonce(), Type: door.OpCheckin, Subject: &door.Subject{Kind: door.KindGuest, ID: lena.ID.String()}, Count: 0, Direction: door.DirIn, At: stamp()}
	first := f.sync(t, p, nil, in, walkup, bad)
	if got := statuses(first.Results); got != "applied,applied,rejected(invalid_op)" {
		t.Fatalf("first sync: %s", got)
	}
	if len(first.Checkins) != 1 || first.Checkins[0].Nonce != in.Nonce || first.Checkins[0].Count != 2 || first.Checkins[0].DeviceID.String() != p.DeviceID ||
		first.Counters.Walkups != 3 || first.Cursor == "" {
		t.Fatalf("first sync state: %+v", first)
	}

	// The device lost the response and sends the same batch again.
	again := f.sync(t, p, &first.Cursor, in, walkup)
	if got := statuses(again.Results); got != "duplicate,duplicate" {
		t.Fatalf("re-sync: %s", got)
	}
	if len(again.Checkins) != 0 || again.Counters.Walkups != 3 {
		t.Fatalf("nothing new since the cursor: %+v", again)
	}
	if n := count(t, `SELECT (SELECT count(*) FROM checkins WHERE tenant_id = $1) * 10 + (SELECT count(*) FROM door_counters WHERE tenant_id = $1)`, f.org); n != 11 {
		t.Fatalf("a repeated nonce must not write twice, got %d", n)
	}

	undo := door.Op{Nonce: nonce(), Type: door.OpUndo, Target: in.Nonce, At: stamp()}
	undoWalkup := door.Op{Nonce: nonce(), Type: door.OpUndo, Target: walkup.Nonce, At: stamp()}
	ghost := door.Op{Nonce: nonce(), Type: door.OpUndo, Target: "never-sent-1", At: stamp()}
	third := f.sync(t, p, &again.Cursor, undo, undoWalkup, ghost, undo)
	if got := statuses(third.Results); got != "applied,applied,rejected(unknown_target),duplicate" {
		t.Fatalf("undo: %s", got)
	}
	if len(third.Checkins) != 1 || !third.Checkins[0].Undone || third.Counters.Walkups != 0 {
		t.Fatalf("an undo is a change devices receive after the cursor: %+v", third)
	}
	if b := f.bundle(t, p); len(b.Checkins) != 1 || !b.Checkins[0].Undone {
		t.Fatalf("bundle check-ins: %+v", b.Checkins)
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'door.checkins_synced'`, f.org); n != 3 {
		t.Fatalf("each sync with ops is audited, got %d", n)
	}
	if n := count(t, `SELECT count(*) FROM outbox WHERE tenant_id = $1 AND subject LIKE '%.door.checkins_synced'`, f.org); n != 3 {
		t.Fatalf("each sync with ops emits door.checkins_synced, got %d", n)
	}
	f.sync(t, p, &third.Cursor)
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'door.checkins_synced'`, f.org); n != 3 {
		t.Fatal("a pull-only sync is not audited")
	}

	if rec := f.call(p, http.MethodPost, "/api/v1/door/checkins", map[string]any{"since": "garbage", "ops": []any{}}); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a bad cursor is a request error: %d %s", rec.Code, rec.Body)
	}
	many := make([]door.Op, door.MaxOps+1)
	if rec := f.call(p, http.MethodPost, "/api/v1/door/checkins", door.SyncInput{Ops: many}); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("more than 500 ops: %d", rec.Code)
	}
}

func TestConflictsAreFlaggedAcrossDevices(t *testing.T) {
	f := setup(t)
	a := f.night(t, "Klubnacht")
	l := f.list(t, a.ID, "Residents", false)
	g := f.add(t, a.ID, l.ID, guest.GuestInput{Name: "Solo Sam"}, guest.GuestInput{Name: "Party Pia", PlusN: 2})
	sam, pia := g[0], g[1]
	f.importTickets(t, a.ID)
	d1 := f.device(t, a.ID, "Door 1")
	d2 := f.device(t, a.ID, "Door 2")
	ticket := ticketIDs(f.bundle(t, d1))["RA-SECRET-1"]

	first := checkin(door.KindGuest, sam.ID, 1, door.DirIn)
	if got := statuses(f.sync(t, d1, nil, first, checkin(door.KindGuest, pia.ID, 1, door.DirIn), checkin(door.KindTicket, ticket, 1, door.DirIn)).Results); got != "applied,applied,applied" {
		t.Fatalf("device 1: %s", got)
	}
	// Device 2 was offline and admits Sam again, two more of Pia's party
	// (fine: 1 + 2 = 3 heads) and the same ticket.
	res := f.sync(t, d2, nil,
		checkin(door.KindGuest, sam.ID, 1, door.DirIn),
		checkin(door.KindGuest, pia.ID, 2, door.DirIn),
		checkin(door.KindTicket, ticket, 1, door.DirIn),
		checkin(door.KindGuest, pia.ID, 1, door.DirIn))
	if got := statuses(res.Results); got != "applied+conflict,applied,applied+conflict,applied+conflict" {
		t.Fatalf("device 2: %s", got)
	}
	flagged := 0
	for _, c := range res.Checkins {
		if c.Conflict {
			flagged++
			if c.DeviceID.String() != d2.DeviceID {
				t.Fatalf("the later row carries the flag: %+v", c)
			}
		}
	}
	if len(res.Checkins) != 7 || flagged != 3 {
		t.Fatalf("both devices' rows come back, 3 flagged: %+v", res.Checkins)
	}
	var conflictOf string
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT o.client_nonce FROM checkins c JOIN checkins o ON o.id = c.conflict_of
	  WHERE c.guest_id = $1 AND c.conflict_of IS NOT NULL`, sam.ID).Scan(&conflictOf); err != nil || conflictOf != first.Nonce {
		t.Fatalf("conflict_of must point at device 1's row: %q %v", conflictOf, err)
	}
	if again := f.sync(t, d1, nil); len(again.Checkins) != 7 {
		t.Fatal("a sync without cursor returns every device's rows")
	}
	// Re-entry: Sam goes out on device 1 and back in on device 2: no conflict.
	f.sync(t, d2, nil, door.Op{Nonce: nonce(), Type: door.OpUndo, Target: res.Results[0].Nonce, At: stamp()})
	f.sync(t, d1, nil, checkin(door.KindGuest, sam.ID, 1, door.DirOut))
	if got := statuses(f.sync(t, d2, nil, checkin(door.KindGuest, sam.ID, 1, door.DirIn)).Results); got != "applied" {
		t.Fatalf("re-entry after going out is not a conflict: %s", got)
	}
}

func TestDuplicateReportsTheOriginalConflict(t *testing.T) {
	f := setup(t)
	a := f.night(t, "Klubnacht")
	sam := f.add(t, a.ID, f.list(t, a.ID, "Residents", false).ID, guest.GuestInput{Name: "Solo Sam"})[0]
	d1, d2 := f.device(t, a.ID, "Door 1"), f.device(t, a.ID, "Door 2")
	f.sync(t, d1, nil, checkin(door.KindGuest, sam.ID, 1, door.DirIn))
	op := checkin(door.KindGuest, sam.ID, 1, door.DirIn)
	f.sync(t, d2, nil, op)
	if got := statuses(f.sync(t, d2, nil, op).Results); got != "duplicate+conflict" {
		t.Fatalf("re-sent conflicting op: %s", got)
	}
}

func TestSubjectsFromAnotherEventAreRejected(t *testing.T) {
	f := setup(t)
	a, b := f.night(t, "Klubnacht A"), f.night(t, "Klubnacht B")
	f.list(t, a.ID, "Residents", false)
	olga := f.add(t, b.ID, f.list(t, b.ID, "Other", false).ID, guest.GuestInput{Name: "Olga Elsewhere"})[0]
	f.importTickets(t, b.ID)
	pb := f.device(t, b.ID, "Door B")
	ticketB := ticketIDs(f.bundle(t, pb))["RA-SECRET-2"]

	pa := f.device(t, a.ID, "Door A")
	res := f.sync(t, pa, nil,
		checkin(door.KindGuest, olga.ID, 1, door.DirIn),
		checkin(door.KindTicket, ticketB, 1, door.DirIn),
		checkin(door.KindTicket, olga.ID, 1, door.DirIn),
		checkin(door.KindGuest, uuid.New(), 1, door.DirIn))
	if got := statuses(res.Results); got != "rejected(unknown_subject),rejected(unknown_subject),rejected(unknown_subject),rejected(unknown_subject)" {
		t.Fatalf("foreign subjects: %s", got)
	}
	// Undoing another event's check-in is refused too.
	bIn := checkin(door.KindGuest, olga.ID, 1, door.DirIn)
	f.sync(t, pb, nil, bIn)
	if got := statuses(f.sync(t, pa, nil, door.Op{Nonce: nonce(), Type: door.OpUndo, Target: bIn.Nonce, At: stamp()}).Results); got != "rejected(unknown_target)" {
		t.Fatalf("foreign undo: %s", got)
	}
	if n := count(t, `SELECT count(*) FROM checkins WHERE event_id = $1`, a.ID); n != 0 {
		t.Fatalf("nothing may be written for event A, got %d", n)
	}
}

func TestDoorRoutesNeedADoorSessionAndStayInItsEvent(t *testing.T) {
	f := setup(t)
	a, b := f.night(t, "Klubnacht A"), f.night(t, "Klubnacht B")
	staff := f.staff()
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/door/bundle"}, {http.MethodPost, "/api/v1/door/checkins"}, {http.MethodPost, "/api/v1/door/adds"},
	} {
		rec := f.call(staff, c.method, c.path, map[string]any{})
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"error":"door_session_required"`) {
			t.Fatalf("staff on %s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
	p := f.device(t, a.ID, "Door A")
	for _, path := range []string{"/api/v1/events/" + b.ID.String() + "/guests", "/api/v1/events/" + a.ID.String() + "/guests"} {
		if rec := f.call(p, http.MethodGet, path, nil); rec.Code != http.StatusForbidden {
			t.Fatalf("a door session must not read the guest table (%s): %d", path, rec.Code)
		}
	}
	// A door principal that somehow lost its scope is refused by the guard.
	unscoped := p
	unscoped.EventScope = ""
	if rec := f.call(unscoped, http.MethodGet, "/api/v1/door/bundle", nil); rec.Code != http.StatusForbidden ||
		!strings.Contains(rec.Body.String(), "outside_event_scope") {
		t.Fatalf("unscoped door principal: %d %s", rec.Code, rec.Body)
	}
	if b := f.bundle(t, p); b.Event.ID != a.ID {
		t.Fatalf("the bundle is always the session's event, got %s", b.Event.ID)
	}
}

func TestAddsNeedTheManagerPINAndShareItsLockout(t *testing.T) {
	f := setup(t)
	a := f.night(t, "Klubnacht")
	l := f.list(t, a.ID, "Door adds", false)
	p := f.device(t, a.ID, "Door A")
	add := func(pin string) door.Add {
		return door.Add{ID: uuid.NewString(), Nonce: nonce(), ListID: l.ID.String(), Name: "Walk  In Wanda", PlusN: 1, ManagerPIN: pin, At: stamp()}
	}
	if got := statuses(f.adds(t, p, add("123456")).Results); got != "rejected(manager_pin_invalid)" {
		t.Fatalf("without a manager PIN: %s", got)
	}
	pin, err := f.id.SetManagerPIN(context.Background(), f.manager(), a.ID, f.clock.t.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	wrong := func(i int) string {
		w := []string{"000000", "111111", "222222", "333333", "444444", "555555"}[i]
		if w == pin {
			w = "999999"
		}
		return w
	}
	// One mistyped PIN on a queue of three adds spends one attempt.
	if got := statuses(f.adds(t, p, add(wrong(0)), add(wrong(0)), add(wrong(0))).Results); got != "rejected(manager_pin_invalid),rejected(manager_pin_invalid),rejected(manager_pin_invalid)" {
		t.Fatalf("wrong PIN: %s", got)
	}
	if n := count(t, `SELECT failed_attempts FROM door_pins WHERE event_id = $1 AND manager`, a.ID); n != 1 {
		t.Fatalf("a batch spends one attempt per distinct PIN, got %d", n)
	}
	f.adds(t, p, add(wrong(1)), add(wrong(2)), add(wrong(3)), add(wrong(4)))
	if got := statuses(f.adds(t, p, add(pin)).Results); got != "rejected(manager_pin_invalid)" {
		t.Fatalf("five wrong PINs lock the manager PIN: %s", got)
	}
	if n := count(t, `SELECT failed_attempts FROM door_pins WHERE event_id = $1 AND NOT manager`, a.ID); n != 0 {
		t.Fatal("the staff PIN's lockout is separate")
	}
	f.clock.t = f.clock.t.Add(16 * time.Minute)

	ok := add(pin)
	res := f.adds(t, p, ok, door.Add{ID: uuid.NewString(), Nonce: nonce(), ListID: uuid.NewString(), Name: "Lost", ManagerPIN: pin, At: stamp()},
		door.Add{ID: "not-a-uuid", Nonce: nonce(), ListID: l.ID.String(), Name: "X", ManagerPIN: pin, At: stamp()})
	if got := statuses(res.Results); got != "applied,rejected(unknown_list),rejected(invalid_add)" || res.Results[0].ID != ok.ID {
		t.Fatalf("adds after the lockout: %s %+v", got, res.Results)
	}
	if got := statuses(f.adds(t, p, ok).Results); got != "duplicate" {
		t.Fatalf("re-sent add: %s", got)
	}
	page, err := f.guests.ListGuests(f.ctx, a.ID, guest.GuestFilter{})
	if err != nil || len(page.Guests) != 1 || page.Guests[0].ID.String() != ok.ID || page.Guests[0].Name != "Walk In Wanda" ||
		page.Guests[0].Source != guest.SourceDoor || page.Guests[0].Status != guest.StatusGoing || page.Guests[0].PlusN != 1 {
		t.Fatalf("the add is a going guest from the door: %+v %v", page.Guests, err)
	}
	// The device queued a check-in for the new guest; it syncs later.
	if got := statuses(f.sync(t, p, nil, checkin(door.KindGuest, uuid.MustParse(ok.ID), 2, door.DirIn)).Results); got != "applied" {
		t.Fatalf("check-in of the added guest: %s", got)
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'door.guests_added'`, f.org); n != 1 {
		t.Fatalf("adds are audited, got %d", n)
	}
	if rec := f.call(p, http.MethodPost, "/api/v1/door/adds", door.AddsInput{Adds: []door.Add{}}); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an empty batch: %d", rec.Code)
	}
}

func TestTenantsAreIsolated(t *testing.T) {
	x, y := setup(t), setup(t)
	ex, ey := x.night(t, "X night"), y.night(t, "Y night")
	xl := x.list(t, ex.ID, "Residents", false)
	xg := x.add(t, ex.ID, xl.ID, guest.GuestInput{Name: "Xavier"})[0]
	yl := y.list(t, ey.ID, "Residents", false)
	py := y.device(t, ey.ID, "Door Y")
	pin, _ := y.id.SetManagerPIN(context.Background(), y.manager(), ey.ID, y.clock.t.Add(4*time.Hour))

	if got := statuses(y.sync(t, py, nil, checkin(door.KindGuest, xg.ID, 1, door.DirIn)).Results); got != "rejected(unknown_subject)" {
		t.Fatalf("another tenant's guest: %s", got)
	}
	// Claiming another tenant's guest id writes nothing and reveals nothing.
	res := y.adds(t, py, door.Add{ID: xg.ID.String(), Nonce: nonce(), ListID: yl.ID.String(), Name: "Impostor", ManagerPIN: pin, At: stamp()})
	if got := statuses(res.Results); got != "rejected(id_conflict)" {
		t.Fatalf("an id taken in another tenant: %s", got)
	}
	if b := y.bundle(t, py); len(b.Guests) != 0 || b.Event.ID != ey.ID {
		t.Fatalf("tenant Y sees none of X: %+v", b.Guests)
	}
	if n := count(t, `SELECT count(*) FROM guests WHERE id = $1 AND tenant_id = $2`, xg.ID, x.org); n != 1 {
		t.Fatal("tenant X's guest must be untouched")
	}
	// A nonce is unique per tenant only: Y reusing X's nonce is not a duplicate.
	px := x.device(t, ex.ID, "Door X")
	shared := checkin(door.KindGuest, xg.ID, 1, door.DirIn)
	x.sync(t, px, nil, shared)
	yg := y.add(t, ey.ID, yl.ID, guest.GuestInput{Name: "Yara"})[0]
	mine := checkin(door.KindGuest, yg.ID, 1, door.DirIn)
	mine.Nonce = shared.Nonce
	if got := statuses(y.sync(t, py, nil, mine).Results); got != "applied" {
		t.Fatalf("another tenant's nonce: %s", got)
	}
}
