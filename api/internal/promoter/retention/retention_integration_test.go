package retention_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
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
	"github.com/klubhub/dj/api/internal/promoter/retention"
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

// stack is one test's services and router over the shared database. The
// retention clock is movable; the others use the real clock.
type stack struct {
	keys      *envelope.Keyring
	db        *tenantdb.DB
	clock     *clock
	id        *identity.Service
	events    *event.Service
	guests    *guest.Service
	retention *retention.Service
	mux       http.Handler
}

func newStack(t *testing.T) *stack {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	keys := envelope.NewKeyring(kek)
	db := tenantdb.New(testDB.App)
	s := &stack{
		keys: keys, db: db, clock: &clock{t: time.Now().UTC()},
		id: identity.NewService(db, keys, identity.Options{
			Argon2: auth.Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
		}),
		events: event.NewService(db, keys, nil),
		guests: guest.NewService(db, keys, nil),
	}
	s.retention = retention.NewService(db, s.clock.now)
	engine, err := authz.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	reg := authz.NewRegistry()
	identity.NewHandler(s.id).Mount(r, engine, reg, nil)
	guest.NewHandler(s.guests).Mount(r, engine, reg, nil)
	door.NewHandler(door.NewService(db, keys, s.id, nil)).Mount(r, engine, reg, nil)
	report.NewHandler(report.NewService(db, keys, nil)).Mount(r, engine, reg, nil)
	retention.NewHandler(s.retention).Mount(r, engine, reg, nil)
	s.mux = r
	return s
}

// org is one tenant with an owner principal.
type org struct {
	s   *stack
	t   *testing.T
	id  uuid.UUID
	ctx context.Context
}

func (s *stack) newOrg(t *testing.T) *org {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if err := tenantdb.WithTenant(context.Background(), testDB.App, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Nachtwerk', $2)`, id, "nw-"+id.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return &org{s: s, t: t, id: id, ctx: tenantdb.ContextWithTenant(context.Background(), id)}
}

func (o *org) owner() authz.Principal {
	return authz.Principal{Sub: staffSub, OrgID: o.id.String(), Roles: []string{"owner"}, AMR: []string{"pwd", "otp"}, AuthTime: time.Now()}
}

func (o *org) call(p authz.Principal, method, path string, body any) *httptest.ResponseRecorder {
	o.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	ctx := authz.ContextWithPrincipal(req.Context(), p)
	req = req.WithContext(tenantdb.ContextWithTenant(ctx, o.id))
	rec := httptest.NewRecorder()
	o.s.mux.ServeHTTP(rec, req)
	return rec
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func (o *org) night(title string, start time.Time) event.Detail {
	o.t.Helper()
	capacity := 300
	return must(o.s.events.CreateEvent(o.ctx, event.EventInput{
		Title: title, StartsAt: start, EndsAt: start.Add(8 * time.Hour), Timezone: "Europe/Berlin", City: "Berlin", Capacity: &capacity,
	}))
}

// device registers a door device, sets staff and manager PINs for the
// event and returns the door principal.
func (o *org) device(eventID uuid.UUID) authz.Principal {
	o.t.Helper()
	ctx := context.Background()
	mgr := authz.Principal{OrgID: o.id.String(), Roles: []string{"owner"}}
	d := must(o.s.id.RegisterDoorDevice(ctx, mgr, "Door 1"))
	pin := must(o.s.id.SetDoorPIN(ctx, mgr, eventID, time.Now().Add(10*time.Hour)))
	must(o.s.id.SetManagerPIN(ctx, mgr, eventID, time.Now().Add(10*time.Hour)))
	res := must(o.s.id.DoorLogin(ctx, d.Token, eventID, pin))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: res.Cookie})
	return must(o.s.id.Authenticate(r))
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
	"9001;T-3;Jo;Tickets;jo@example.org;Tier 1;RA-SECRET-3;Valid\n" +
	"9002;T-2;Kofi;Mensah;;Tier 2;RA-SECRET-2;Refunded\n"

type seeded struct {
	ev       event.Detail
	artists  guest.List
	ben      guest.Allocation
	lena     guest.Guest
	dan      guest.Guest
	ticket   uuid.UUID
	doorP    authz.Principal
	guestIDs []uuid.UUID
}

// seed builds an event that ended two days ago: a contact list with an
// allocation (submitter contact), guests with email, phone and note,
// imported tickets with secrets, door PINs, and check-ins and counters.
func (o *org) seed(title string) seeded {
	o.t.Helper()
	var sd seeded
	start := time.Now().UTC().Add(-50 * time.Hour).Truncate(time.Hour)
	sd.ev = o.night(title, start)
	contact := "ben@agency.example"
	sd.artists = must(o.s.guests.CreateList(o.ctx, sd.ev.ID, guest.ListInput{Name: "Artists", Type: "artist", CollectContact: true}))
	comp := must(o.s.guests.CreateList(o.ctx, sd.ev.ID, guest.ListInput{Name: "Comp", Type: "comp"}))
	sd.ben = must(o.s.guests.CreateAllocation(o.ctx, sd.ev.ID, sd.artists.ID, guest.AllocationInput{Label: "Ben Klock", Quota: 10, PlusNMax: 2, SubmitterContact: &contact}))
	added := must(o.s.guests.AddGuests(o.ctx, sd.ev.ID, guest.AddInput{ListID: sd.artists.ID, AllocationID: &sd.ben.ID, Guests: []guest.GuestInput{
		{Name: "Lena Vogt", PlusN: 2, Email: "lena.vogt@example.org", Phone: "+49 30 1234567", Note: "backstage"},
		{Name: "Kim Pending", PlusN: 1, Status: guest.StatusPending},
	}}, staffSub)).Added
	sd.lena = added[0]
	added2 := must(o.s.guests.AddGuests(o.ctx, sd.ev.ID, guest.AddInput{ListID: comp.ID, Guests: []guest.GuestInput{{Name: "Dan", Note: "friend of the bar"}}}, staffSub)).Added
	sd.dan = added2[0]
	for _, g := range append(added, added2...) {
		sd.guestIDs = append(sd.guestIDs, g.ID)
	}
	tab := must(guest.ParseCSV([]byte(raCSV)))
	must(o.s.guests.ImportAttendees(o.ctx, sd.ev.ID, guest.ImportInput{Preset: guest.PresetRA, Table: tab}, false, staffSub))
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT id FROM order_positions WHERE event_id = $1 AND status = 'valid' ORDER BY external_ref LIMIT 1`,
		sd.ev.ID).Scan(&sd.ticket); err != nil {
		o.t.Fatal(err)
	}
	sd.doorP = o.device(sd.ev.ID)
	at := func(m int) string { return start.Add(time.Duration(m) * time.Minute).Format(time.RFC3339) }
	n := func() string { return "ret-" + uuid.NewString() }
	rec := o.call(sd.doorP, http.MethodPost, "/api/v1/door/checkins", door.SyncInput{Ops: []door.Op{
		{Nonce: n(), Type: door.OpCheckin, Subject: &door.Subject{Kind: door.KindGuest, ID: sd.lena.ID.String()}, Count: 2, Direction: door.DirIn, At: at(5)},
		{Nonce: n(), Type: door.OpCheckin, Subject: &door.Subject{Kind: door.KindGuest, ID: sd.dan.ID.String()}, Count: 1, Direction: door.DirIn, At: at(20)},
		{Nonce: n(), Type: door.OpCheckin, Subject: &door.Subject{Kind: door.KindTicket, ID: sd.ticket.String()}, Count: 1, Direction: door.DirIn, At: at(25)},
		{Nonce: n(), Type: door.OpCheckin, Subject: &door.Subject{Kind: door.KindGuest, ID: sd.dan.ID.String()}, Count: 1, Direction: door.DirOut, At: at(50)},
		{Nonce: n(), Type: door.OpCounter, Kind: door.CounterWalkup, Delta: 3, At: at(35)},
	}})
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"rejected"`) {
		o.t.Fatalf("sync: %d %s", rec.Code, rec.Body)
	}
	return sd
}

func (o *org) reportJSON(eventID uuid.UUID) map[string]any {
	o.t.Helper()
	rec := o.call(o.owner(), http.MethodGet, "/api/v1/events/"+eventID.String()+"/report", nil)
	var m map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &m) != nil {
		o.t.Fatalf("report: %d %s", rec.Code, rec.Body)
	}
	delete(m, "generated_at")
	return m
}

func (o *org) purge(p authz.Principal, eventID uuid.UUID, confirm string) *httptest.ResponseRecorder {
	return o.call(p, http.MethodPost, "/api/v1/events/"+eventID.String()+"/purge", map[string]string{"confirm": confirm})
}

func errorOf(rec *httptest.ResponseRecorder) string {
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error
}

// personalLeft counts rows of the event that still hold any personal
// column or blind index.
func personalLeft(t *testing.T, eventID uuid.UUID) map[string]int {
	return map[string]int{
		"guests": count(t, `SELECT count(*) FROM guests WHERE event_id = $1 AND (name_enc IS NOT NULL OR email_enc IS NOT NULL OR phone_enc IS NOT NULL
		  OR note_enc IS NOT NULL OR name_bidx IS NOT NULL OR email_bidx IS NOT NULL OR purged_at IS NULL)`, eventID),
		"orders": count(t, `SELECT count(*) FROM orders WHERE event_id = $1 AND (buyer_name_enc IS NOT NULL OR buyer_email_enc IS NOT NULL
		  OR buyer_email_bidx IS NOT NULL OR purged_at IS NULL)`, eventID),
		"order_positions": count(t, `SELECT count(*) FROM order_positions WHERE event_id = $1 AND (attendee_name_enc IS NOT NULL
		  OR attendee_email_enc IS NOT NULL OR secret_enc IS NOT NULL OR attendee_name_bidx IS NOT NULL OR attendee_email_bidx IS NOT NULL
		  OR secret_bidx IS NOT NULL OR purged_at IS NULL)`, eventID),
		"guest_allocations": count(t, `SELECT count(*) FROM guest_allocations WHERE event_id = $1 AND submitter_contact_enc IS NOT NULL`, eventID),
		"door_pins":         count(t, `SELECT count(*) FROM door_pins WHERE event_id = $1`, eventID),
	}
}

// TestSchemaPersonalColumnsAreAllPurged keeps the purge in step with the
// schema: every *_enc / *_bidx column of an event-scoped table must be one
// the purge erases (or a table whose rows it deletes).
func TestSchemaPersonalColumnsAreAllPurged(t *testing.T) {
	newStack(t)
	purged := map[string]bool{
		"guests.name_enc": true, "guests.email_enc": true, "guests.phone_enc": true, "guests.note_enc": true,
		"guests.name_bidx": true, "guests.email_bidx": true,
		"orders.buyer_name_enc": true, "orders.buyer_email_enc": true, "orders.buyer_email_bidx": true,
		"order_positions.attendee_name_enc": true, "order_positions.attendee_email_enc": true, "order_positions.secret_enc": true,
		"order_positions.attendee_name_bidx": true, "order_positions.attendee_email_bidx": true, "order_positions.secret_bidx": true,
		"guest_allocations.submitter_contact_enc": true,
		"door_pins.pin_hash_enc":                  true, "door_pins.check_enc": true, // rows deleted
	}
	rows, err := testDB.Owner.Query(context.Background(), `SELECT c.table_name || '.' || c.column_name FROM information_schema.columns c
	  WHERE c.table_schema = 'public' AND (c.column_name LIKE '%\_enc' OR c.column_name LIKE '%\_bidx')
	    AND EXISTS (SELECT 1 FROM information_schema.columns e WHERE e.table_schema = 'public' AND e.table_name = c.table_name AND e.column_name = 'event_id')`)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) < len(purged) {
		t.Fatalf("found only %v", cols)
	}
	for _, c := range cols {
		if !purged[c] {
			t.Errorf("%s holds personal data in an event-scoped table but the retention purge does not erase it", c)
		}
	}
}

func TestPurgeAnonymisesEventAndKeepsReport(t *testing.T) {
	s := newStack(t)
	a := s.newOrg(t)
	sd := a.seed("Klubnacht")
	other := s.newOrg(t)
	osd := other.seed("Klubnacht")

	checkins := count(t, `SELECT count(*) FROM checkins WHERE event_id = $1`, sd.ev.ID)
	counters := count(t, `SELECT count(*) FROM door_counters WHERE event_id = $1`, sd.ev.ID)
	before := a.reportJSON(sd.ev.ID)
	if tot := before["totals"].(map[string]any); tot["guests_arrived"].(float64) != 2 || tot["tickets_scanned"].(float64) != 1 {
		t.Fatalf("seeded report: %v", tot)
	}
	evPath := "/api/v1/events/" + sd.ev.ID.String()

	// Privacy before: purge_after = end + 30 days, 3 guests + 3 positions.
	rec := a.call(a.owner(), http.MethodGet, evPath+"/privacy", nil)
	var pv retention.Privacy
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &pv) != nil || pv.PurgedAt != nil || pv.RetentionDays != 30 ||
		pv.PersonalRows != 6 || !pv.PurgeAfter.Equal(sd.ev.EndsAt.Add(30*24*time.Hour)) {
		t.Fatalf("privacy before: %d %s", rec.Code, rec.Body)
	}

	// Another tenant cannot purge (or even see) this event.
	if rec := other.purge(other.owner(), sd.ev.ID, "Klubnacht"); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant purge: %d %s", rec.Code, rec.Body)
	}

	rec = a.purge(a.owner(), sd.ev.ID, "  Klubnacht ")
	var res retention.PurgeResult
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("purge: %d %s", rec.Code, rec.Body)
	}
	want := retention.Counts{Guests: 3, Orders: 2, OrderPositions: 3, GuestAllocations: 1, DoorPins: 2}
	if res.Counts != want || res.Trigger != retention.TriggerManual || res.EventID != sd.ev.ID {
		t.Fatalf("purge result: %+v", res)
	}

	for table, n := range personalLeft(t, sd.ev.ID) {
		if n != 0 {
			t.Errorf("%s: %d rows still hold personal data", table, n)
		}
	}
	if n := count(t, `SELECT count(*) FROM guests WHERE event_id = $1`, sd.ev.ID); n != 3 {
		t.Errorf("guest rows must stay (anonymised), got %d", n)
	}
	if n := count(t, `SELECT count(*) FROM order_positions WHERE event_id = $1`, sd.ev.ID); n != 3 {
		t.Errorf("position rows must stay, got %d", n)
	}
	if n := count(t, `SELECT count(*) FROM checkins WHERE event_id = $1`, sd.ev.ID); n != checkins {
		t.Errorf("check-ins must stay: %d → %d", checkins, n)
	}
	if n := count(t, `SELECT count(*) FROM door_counters WHERE event_id = $1`, sd.ev.ID); n != counters {
		t.Errorf("counters must stay: %d → %d", counters, n)
	}
	if n := count(t, `SELECT count(*) FROM sessions WHERE event_scope = $1 AND revoked_at IS NULL`, sd.ev.ID); n != 0 {
		t.Errorf("door sessions of the event must be revoked, %d live", n)
	}
	if n := count(t, `SELECT count(*) FROM guest_allocations WHERE event_id = $1 AND label = 'Ben Klock'`, sd.ev.ID); n != 1 {
		t.Errorf("the public submitter label stays")
	}

	// The other tenant's identical event is untouched.
	left := personalLeft(t, osd.ev.ID)
	if left["guests"] != 3 || left["orders"] != 2 || left["order_positions"] != 3 || left["guest_allocations"] != 1 || left["door_pins"] != 2 {
		t.Fatalf("a purge in one tenant touched another: %v", left)
	}
	if n := count(t, `SELECT count(*) FROM event_purges WHERE tenant_id = $1`, other.id); n != 0 {
		t.Fatalf("the other tenant got purge rows")
	}

	// The report is identical, minus generated_at.
	if after := a.reportJSON(sd.ev.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("report changed by the purge:\nbefore %v\nafter  %v", before, after)
	}

	// Recorded, emitted (identifiers only) and audited (counts only).
	if n := count(t, `SELECT count(*) FROM event_purges WHERE event_id = $1 AND trigger = 'manual' AND purged_at IS NOT NULL
	  AND counts = '{"guests":3,"orders":2,"order_positions":3,"guest_allocations":1,"door_pins":2}'::jsonb`, sd.ev.ID); n != 1 {
		t.Fatalf("event_purges row: %d", n)
	}
	var payload string
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT payload::text FROM outbox WHERE tenant_id = $1 AND subject = $2`,
		a.id, "tenant."+a.id.String()+".retention.purged").Scan(&payload); err != nil {
		t.Fatalf("outbox: %v", err)
	}
	var ev struct {
		Type string            `json:"type"`
		Refs map[string]string `json:"refs"`
	}
	if json.Unmarshal([]byte(payload), &ev) != nil || ev.Type != "retention.purged" || len(ev.Refs) != 1 || ev.Refs["event_id"] != sd.ev.ID.String() {
		t.Fatalf("outbox payload: %s", payload)
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'retention.purged' AND actor_id = $2
	  AND resource = $3 AND reason = 'manual: 3 guests, 2 orders, 3 order positions, 1 allocation contacts, 2 door pins'`,
		a.id, staffSub, "event:"+sd.ev.ID.String()); n != 1 {
		t.Fatalf("audit entry: %d", n)
	}

	// Guest table: erased rows keep list, status and check-in state.
	rec = a.call(a.owner(), http.MethodGet, evPath+"/guests", nil)
	var page guest.GuestPage
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("guests: %d %s", rec.Code, rec.Body)
	}
	for _, leak := range []string{"Lena", "Kim", "Dan", "example.org", "1234567", "backstage", "friend", "Kofi", "RA-SECRET", "agency"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("guest table leaks %q after the purge", leak)
		}
	}
	if len(page.Guests) != 3 || len(page.Tickets) != 3 || page.Counts.CheckedIn != 1 {
		t.Fatalf("guest page: %+v", page)
	}
	for _, g := range page.Guests {
		if !g.Purged || g.Name != "" || g.Email != "" || g.Phone != "" || g.Note != "" {
			t.Errorf("guest %s not shown as erased: %+v", g.ID, g)
		}
		if g.ID == sd.lena.ID && (g.HeadsIn != 2 || g.Status != guest.StatusGoing || g.PlusN != 2 || g.ListID != sd.artists.ID) {
			t.Errorf("Lena keeps list, status, +N and check-in state: %+v", g)
		}
	}
	for _, tk := range page.Tickets {
		if !tk.Purged || tk.Name != "" || tk.Email != "" {
			t.Errorf("ticket not erased: %+v", tk)
		}
	}
	if !strings.Contains(rec.Body.String(), `"purged":true`) {
		t.Errorf("purged flag missing: %s", rec.Body)
	}
	rec = a.call(a.owner(), http.MethodGet, "/api/v1/events/"+osd.ev.ID.String()+"/guests", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other tenant's event from this tenant: %d", rec.Code)
	}
	rec = other.call(other.owner(), http.MethodGet, "/api/v1/events/"+osd.ev.ID.String()+"/guests", nil)
	if !strings.Contains(rec.Body.String(), "Lena Vogt") || !strings.Contains(rec.Body.String(), `"purged":false`) {
		t.Fatalf("other tenant keeps its names: %s", rec.Body)
	}

	// Exports and the door bundle: 409 event_purged.
	for _, p := range []string{evPath + "/guests/export.csv", evPath + "/report/list-back.csv?allocation_id=" + sd.ben.ID.String()} {
		if rec := a.call(a.owner(), http.MethodGet, p, nil); rec.Code != http.StatusConflict || errorOf(rec) != "event_purged" {
			t.Errorf("%s: %d %s", p, rec.Code, rec.Body)
		}
	}
	if rec := a.call(sd.doorP, http.MethodGet, "/api/v1/door/bundle", nil); rec.Code != http.StatusConflict || errorOf(rec) != "event_purged" {
		t.Errorf("bundle: %d %s", rec.Code, rec.Body)
	}

	// Writes that would add personal data: 409 event_purged.
	refused := map[string]*httptest.ResponseRecorder{
		"add guests":   a.call(a.owner(), http.MethodPost, evPath+"/guests", guest.AddInput{ListID: sd.artists.ID, Guests: []guest.GuestInput{{Name: "New"}}}),
		"update guest": a.call(a.owner(), http.MethodPut, evPath+"/guests/"+sd.dan.ID.String(), guest.GuestInput{Name: "Dan Again", Status: guest.StatusGoing}),
		"import": a.call(a.owner(), http.MethodPost, evPath+"/attendees/import?dry_run=false", map[string]any{
			"preset": "ra", "rows": []map[string]string{{"Order ID": "9003", "Ticket ID": "T-9", "First name": "New", "Last name": "Buyer",
				"Email": "", "Ticket type": "Tier 1", "Barcode": "RA-NEW", "Status": "Valid"}}}),
		"import dry run": a.call(a.owner(), http.MethodPost, evPath+"/attendees/import", map[string]any{
			"preset": "ra", "rows": []map[string]string{{"Order ID": "9003", "Ticket ID": "T-9", "First name": "New", "Last name": "Buyer",
				"Email": "", "Ticket type": "Tier 1", "Barcode": "RA-NEW", "Status": "Valid"}}}),
		"door adds": a.call(sd.doorP, http.MethodPost, "/api/v1/door/adds", door.AddsInput{Adds: []door.Add{{ID: uuid.NewString(),
			Nonce: "ret-" + uuid.NewString(), ListID: sd.artists.ID.String(), Name: "Walk In", ManagerPIN: "123456", At: time.Now().UTC().Format(time.RFC3339)}}}),
		"staff pin": a.call(a.owner(), http.MethodPost, "/api/v1/door/events/"+sd.ev.ID.String()+"/pin", map[string]any{"valid_until": time.Now().Add(time.Hour)}),
		"manager pin": a.call(a.owner(), http.MethodPost, "/api/v1/door/events/"+sd.ev.ID.String()+"/pin",
			map[string]any{"valid_until": time.Now().Add(time.Hour), "manager": true}),
		"allocation with contact": a.call(a.owner(), http.MethodPost, evPath+"/lists/"+sd.artists.ID.String()+"/allocations",
			map[string]any{"label": "Late", "quota": 2, "submitter_contact": "late@example.org"}),
		"purge again": a.purge(a.owner(), sd.ev.ID, "Klubnacht"),
	}
	for name, rec := range refused {
		if rec.Code != http.StatusConflict || errorOf(rec) != "event_purged" {
			t.Errorf("%s: want 409 event_purged, got %d %s", name, rec.Code, rec.Body)
		}
	}
	for table, n := range personalLeft(t, sd.ev.ID) {
		if n != 0 {
			t.Errorf("%s: refused writes still stored personal data (%d rows)", table, n)
		}
	}
	// Writes without personal data still work: a status change, an
	// allocation without contact.
	if rec := a.call(a.owner(), http.MethodPost, evPath+"/guests/bulk-status", guest.BulkStatusInput{GuestIDs: []uuid.UUID{sd.dan.ID}, Status: guest.StatusDeclined}); rec.Code != http.StatusOK {
		t.Errorf("bulk status on an erased event: %d %s", rec.Code, rec.Body)
	}
	if rec := a.call(a.owner(), http.MethodPost, evPath+"/lists/"+sd.artists.ID.String()+"/allocations", map[string]any{"label": "Late", "quota": 2}); rec.Code != http.StatusCreated {
		t.Errorf("allocation without contact: %d %s", rec.Code, rec.Body)
	}

	// Privacy after.
	rec = a.call(a.owner(), http.MethodGet, evPath+"/privacy", nil)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &pv) != nil || pv.PurgedAt == nil || pv.PersonalRows != 0 {
		t.Fatalf("privacy after: %d %s", rec.Code, rec.Body)
	}
	// Settings list it under recent.
	rec = a.call(a.owner(), http.MethodGet, "/api/v1/org/retention", nil)
	var st retention.Settings
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &st) != nil || len(st.Recent) != 1 || st.Recent[0].EventID != sd.ev.ID ||
		st.Recent[0].Title != "Klubnacht" || st.Recent[0].Trigger != "manual" || st.Recent[0].Counts != want || len(st.Upcoming) != 0 {
		t.Fatalf("settings after purge: %d %s", rec.Code, rec.Body)
	}
}

func TestManualPurgeRules(t *testing.T) {
	s := newStack(t)
	a := s.newOrg(t)
	running := a.night("Running Night", time.Now().UTC().Add(-2*time.Hour))
	ended := a.night("Ended Night", time.Now().UTC().Add(-30*time.Hour))
	must(a.s.guests.AddGuests(a.ctx, ended.ID, guest.AddInput{ListID: must(a.s.guests.CreateList(a.ctx, ended.ID,
		guest.ListInput{Name: "Comp", Type: "comp"})).ID, Guests: []guest.GuestInput{{Name: "Mo"}}}, staffSub))

	cases := []struct {
		name    string
		p       authz.Principal
		event   uuid.UUID
		confirm string
		status  int
		err     string
	}{
		{"not ended", a.owner(), running.ID, "Running Night", http.StatusConflict, "event_not_ended"},
		{"title mismatch", a.owner(), ended.ID, "Ended Nite", http.StatusUnprocessableEntity, "invalid"},
		{"title missing a word", a.owner(), ended.ID, "Ended", http.StatusUnprocessableEntity, "invalid"},
		{"empty confirm", a.owner(), ended.ID, "", http.StatusUnprocessableEntity, "invalid"},
		{"unknown event", a.owner(), uuid.Must(uuid.NewV7()), "x", http.StatusNotFound, "not_found"},
		{"stale sign-in needs step-up", authz.Principal{Sub: staffSub, OrgID: a.id.String(), Roles: []string{"owner"}, AMR: []string{"pwd", "otp"},
			AuthTime: time.Now().Add(-16 * time.Minute)}, ended.ID, "Ended Night", http.StatusForbidden, "reauthentication_required"},
		{"booker cannot erase", authz.Principal{Sub: staffSub, OrgID: a.id.String(), Roles: []string{"booker"}, AuthTime: time.Now()},
			ended.ID, "Ended Night", http.StatusForbidden, "no_role_grant"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := a.purge(c.p, c.event, c.confirm)
			if rec.Code != c.status || errorOf(rec) != c.err {
				t.Fatalf("got %d %s", rec.Code, rec.Body)
			}
		})
	}
	if n := count(t, `SELECT count(*) FROM guests WHERE event_id = $1 AND purged_at IS NULL`, ended.ID); n != 1 {
		t.Fatalf("refused purges must not erase anything")
	}
	// Case, dashes, quotes and spacing are normalised on both sides.
	if rec := a.purge(a.owner(), ended.ID, "  ended   NIGHT "); rec.Code != http.StatusOK {
		t.Fatalf("an owner with a fresh sign-in erases (normalised title): %d %s", rec.Code, rec.Body)
	}
	// Door staff never reach it.
	doorP := authz.Principal{Sub: "device:x", OrgID: a.id.String(), Roles: []string{"door"}, EventScope: ended.ID.String(), AuthTime: time.Now()}
	if rec := a.purge(doorP, ended.ID, "Ended Night"); rec.Code != http.StatusForbidden {
		t.Fatalf("door: %d", rec.Code)
	}
}

func TestRetentionSettings(t *testing.T) {
	s := newStack(t)
	a := s.newOrg(t)
	now := time.Now().UTC()
	past := a.night("Past", now.Add(-72*time.Hour))
	live := a.night("Live", now.Add(-time.Hour))
	a.night("Future", now.Add(72*time.Hour))
	path := "/api/v1/org/retention"

	rec := a.call(a.owner(), http.MethodGet, path, nil)
	var st retention.Settings
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &st) != nil || st.RetentionDays != 30 || len(st.Recent) != 0 ||
		!strings.Contains(rec.Body.String(), `"recent":[]`) {
		t.Fatalf("default settings: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"timezone":"Europe/Berlin"`) || st.Upcoming[0].Timezone != "Europe/Berlin" {
		t.Fatalf("upcoming rows carry the event timezone: %s", rec.Body)
	}
	if len(st.Upcoming) != 2 || st.Upcoming[0].EventID != past.ID || st.Upcoming[1].EventID != live.ID ||
		!st.Upcoming[0].PurgeAfter.Equal(past.EndsAt.Add(30*24*time.Hour)) || !st.Upcoming[0].EndsAt.Equal(past.EndsAt) {
		t.Fatalf("upcoming (started events by purge_after): %+v", st.Upcoming)
	}

	for _, bad := range []any{0, 366, -5} {
		rec := a.call(a.owner(), http.MethodPut, path, map[string]any{"retention_days": bad})
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"retention_days"`) {
			t.Errorf("retention %v: %d %s", bad, rec.Code, rec.Body)
		}
	}
	if rec := a.call(a.owner(), http.MethodPut, path, map[string]any{"retention_days": 7, "extra": 1}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d", rec.Code)
	}
	for _, days := range []int{365, 7} {
		rec := a.call(a.owner(), http.MethodPut, path, map[string]any{"retention_days": days})
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &st) != nil || st.RetentionDays != days {
			t.Fatalf("retention %d: %d %s", days, rec.Code, rec.Body)
		}
	}
	// Recomputed purge_after for every unpurged event, including future ones.
	if n := count(t, `SELECT count(*) FROM event_purges p JOIN events e ON e.id = p.event_id
	  WHERE p.tenant_id = $1 AND p.purge_after = e.ends_at + interval '7 days' AND p.purged_at IS NULL`, a.id); n != 3 {
		t.Fatalf("purge_after after PUT: %d rows at ends_at + 7 days", n)
	}
	if !st.Upcoming[0].PurgeAfter.Equal(past.EndsAt.Add(7 * 24 * time.Hour)) {
		t.Fatalf("upcoming after PUT: %+v", st.Upcoming)
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'org.retention_updated' AND reason = 'retention 365 → 7 days'`, a.id); n != 1 {
		t.Fatalf("retention change audit: %d", n)
	}
	rec = a.call(a.owner(), http.MethodGet, "/api/v1/events/"+past.ID.String()+"/privacy", nil)
	var pv retention.Privacy
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &pv) != nil || pv.RetentionDays != 7 || !pv.PurgeAfter.Equal(past.EndsAt.Add(7*24*time.Hour)) {
		t.Fatalf("privacy: %d %s", rec.Code, rec.Body)
	}

	// Roles: marketing reads, cannot change; door neither.
	marketing := authz.Principal{Sub: staffSub, OrgID: a.id.String(), Roles: []string{"marketing"}, AuthTime: time.Now()}
	if rec := a.call(marketing, http.MethodGet, path, nil); rec.Code != http.StatusOK {
		t.Errorf("marketing read: %d", rec.Code)
	}
	if rec := a.call(marketing, http.MethodPut, path, map[string]any{"retention_days": 30}); rec.Code != http.StatusForbidden {
		t.Errorf("marketing update: %d", rec.Code)
	}
	if rec := a.call(a.owner(), http.MethodGet, "/api/v1/events/"+uuid.NewString()+"/privacy", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown event privacy: %d", rec.Code)
	}
}

func TestShortenRetentionNeedsConfirmAndStepUp(t *testing.T) {
	s := newStack(t)
	a := s.newOrg(t)
	now := time.Now().UTC()
	s.clock.t = now
	recent := a.night("Recent Night", now.Add(-3*24*time.Hour)) // ended ~2.7 days ago
	a.night("Old Night", now.Add(-40*24*time.Hour))             // already due at 30 days
	a.night("Next Night", now.Add(5*24*time.Hour))              // not ended
	l := must(s.guests.CreateList(a.ctx, recent.ID, guest.ListInput{Name: "Comp", Type: "comp"}))
	must(s.guests.AddGuests(a.ctx, recent.ID, guest.AddInput{ListID: l.ID, Guests: []guest.GuestInput{{Name: "Ana"}}}, staffSub))
	path := "/api/v1/org/retention"
	stale := a.owner()
	stale.AuthTime = time.Now().Add(-16 * time.Minute)
	marketing := authz.Principal{Sub: staffSub, OrgID: a.id.String(), Roles: []string{"marketing"}, AuthTime: time.Now()}
	days := func() int {
		var st retention.Settings
		rec := a.call(a.owner(), http.MethodGet, path, nil)
		if json.Unmarshal(rec.Body.Bytes(), &st) != nil {
			t.Fatal(rec.Body)
		}
		return st.RetentionDays
	}

	// Preview (org.read): only events the shorter period makes due at once.
	var pv retention.Preview
	rec := a.call(marketing, http.MethodGet, path+"/preview?days=1", nil)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &pv) != nil || pv.Count != 1 || len(pv.WouldPurge) != 1 ||
		pv.WouldPurge[0].EventID != recent.ID || pv.WouldPurge[0].Title != "Recent Night" || !pv.WouldPurge[0].EndsAt.Equal(recent.EndsAt) {
		t.Fatalf("preview 1 day: %d %s", rec.Code, rec.Body)
	}
	for _, q := range []string{"30", "90", "5"} {
		rec := a.call(a.owner(), http.MethodGet, path+"/preview?days="+q, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"would_purge":[]`) || !strings.Contains(rec.Body.String(), `"count":0`) {
			t.Errorf("preview %s days: %d %s", q, rec.Code, rec.Body)
		}
	}
	for _, q := range []string{"0", "366", "x", ""} {
		rec := a.call(a.owner(), http.MethodGet, path+"/preview?days="+q, nil)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"days"`) {
			t.Errorf("preview %q: %d %s", q, rec.Code, rec.Body)
		}
	}

	refused := func(p authz.Principal, body map[string]any, status int, code string) {
		t.Helper()
		rec := a.call(p, http.MethodPut, path, body)
		if rec.Code != status || errorOf(rec) != code {
			t.Fatalf("%v: got %d %s", body, rec.Code, rec.Body)
		}
		if code == "retention_would_purge" {
			var got struct {
				retention.Preview
				Error string `json:"error"`
			}
			if json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Count != 1 || len(got.WouldPurge) != 1 || got.WouldPurge[0].EventID != recent.ID {
				t.Fatalf("409 body: %s", rec.Body)
			}
		}
		if d := days(); d != 30 {
			t.Fatalf("a refused change must not save: %d days", d)
		}
	}
	refused(a.owner(), map[string]any{"retention_days": 1}, http.StatusConflict, "retention_would_purge")
	refused(a.owner(), map[string]any{"retention_days": 1, "confirm_purge": 2}, http.StatusConflict, "retention_would_purge")
	refused(a.owner(), map[string]any{"retention_days": 1, "confirm_purge": 0}, http.StatusConflict, "retention_would_purge")
	refused(stale, map[string]any{"retention_days": 1, "confirm_purge": 1}, http.StatusForbidden, "reauthentication_required")
	refused(marketing, map[string]any{"retention_days": 1, "confirm_purge": 1}, http.StatusForbidden, "no_role_grant")
	if n := count(t, `SELECT count(*) FROM guests WHERE event_id = $1 AND purged_at IS NULL`, recent.ID); n != 1 {
		t.Fatal("nothing is erased by a refused change")
	}

	// Confirmed with the exact count and a fresh sign-in: saved and audited with the count.
	rec = a.call(a.owner(), http.MethodPut, path, map[string]any{"retention_days": 1, "confirm_purge": 1})
	if rec.Code != http.StatusOK || days() != 1 {
		t.Fatalf("confirmed shortening: %d %s", rec.Code, rec.Body)
	}
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'org.retention_updated'
	  AND reason = 'retention 30 → 1 days; confirmed erasing 1 ended events now'`, a.id); n != 1 {
		t.Fatalf("confirmed shortening audit: %d", n)
	}
	// The job erases it on its next run.
	if _, err := s.retention.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT count(*) FROM guests WHERE event_id = $1 AND purged_at IS NOT NULL`, recent.ID); n != 1 {
		t.Fatal("the confirmed event is erased by the job")
	}

	// Lengthening (or shortening without anything due) needs neither a count nor a recent sign-in.
	if rec := a.call(stale, http.MethodPut, path, map[string]any{"retention_days": 90}); rec.Code != http.StatusOK || days() != 90 {
		t.Fatalf("lengthening: %d %s", rec.Code, rec.Body)
	}
	if rec := a.call(stale, http.MethodPut, path, map[string]any{"retention_days": 60, "confirm_purge": 3}); rec.Code != http.StatusOK || days() != 60 {
		t.Fatalf("shortening with nothing due: %d %s", rec.Code, rec.Body)
	}
}

func TestWorkerPurgesDueEventsOnceAcrossTenants(t *testing.T) {
	s := newStack(t)
	a, b := s.newOrg(t), s.newOrg(t)
	now := time.Now().UTC()
	s.clock.t = now

	addGuest := func(o *org, ev event.Detail) {
		l := must(o.s.guests.CreateList(o.ctx, ev.ID, guest.ListInput{Name: "Comp", Type: "comp"}))
		must(o.s.guests.AddGuests(o.ctx, ev.ID, guest.AddInput{ListID: l.ID, Guests: []guest.GuestInput{{Name: "Guest " + ev.Title}}}, staffSub))
	}
	dueA := a.night("Due A", now.Add(-40*24*time.Hour))        // ended ~40 days ago, 30-day retention
	notDueA := a.night("Not due A", now.Add(-10*24*time.Hour)) // ended 10 days ago
	edgeA := a.night("Edge A", now.Add(-30*24*time.Hour-8*time.Hour))
	futureA := a.night("Future A", now.Add(24*time.Hour))
	dueB := b.night("Due B", now.Add(-40*24*time.Hour))
	shortB := b.night("Short B", now.Add(-3*24*time.Hour)) // due once B keeps 1 day only
	for _, x := range []struct {
		o  *org
		ev event.Detail
	}{{a, dueA}, {a, notDueA}, {a, edgeA}, {a, futureA}, {b, dueB}, {b, shortB}} {
		addGuest(x.o, x.ev)
	}
	// Short B becomes due at once (Due B already is): confirmed with its count.
	if rec := b.call(b.owner(), http.MethodPut, "/api/v1/org/retention", map[string]any{"retention_days": 1, "confirm_purge": 1}); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	// Edge A: purge_after is exactly now.
	s.clock.t = edgeA.EndsAt.Add(30 * 24 * time.Hour)

	dry, err := s.retention.DryRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mine := func(tenant uuid.UUID) bool { return tenant == a.id || tenant == b.id }
	dryIDs := map[uuid.UUID]uuid.UUID{}
	for _, d := range dry {
		if mine(d.TenantID) {
			dryIDs[d.EventID] = d.TenantID
		}
	}
	wantDue := map[uuid.UUID]uuid.UUID{dueA.ID: a.id, edgeA.ID: a.id, dueB.ID: b.id, shortB.ID: b.id}
	if !reflect.DeepEqual(dryIDs, wantDue) {
		t.Fatalf("dry run: %v, want %v", dryIDs, wantDue)
	}
	if n := count(t, `SELECT count(*) FROM guests WHERE tenant_id IN ($1, $2) AND purged_at IS NOT NULL`, a.id, b.id); n != 0 {
		t.Fatalf("the dry run purged %d guests", n)
	}

	res, err := s.retention.RunDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]uuid.UUID{}
	for _, r := range res {
		if mine(r.TenantID) {
			got[r.EventID] = r.TenantID
			if r.Trigger != retention.TriggerSchedule || r.Counts.Guests != 1 {
				t.Errorf("result: %+v", r)
			}
		}
	}
	if !reflect.DeepEqual(got, wantDue) {
		t.Fatalf("purged %v, want %v", got, wantDue)
	}
	for id, tenant := range wantDue {
		if n := count(t, `SELECT count(*) FROM event_purges WHERE event_id = $1 AND tenant_id = $2 AND trigger = 'schedule' AND purged_at IS NOT NULL`, id, tenant); n != 1 {
			t.Errorf("event %s: purge row in its own tenant: %d", id, n)
		}
		if n := count(t, `SELECT count(*) FROM guests WHERE event_id = $1 AND tenant_id = $2 AND purged_at IS NOT NULL AND name_enc IS NULL`, id, tenant); n != 1 {
			t.Errorf("event %s: guest not purged", id)
		}
		if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'retention.purged' AND actor_id = 'system:retention'
		  AND resource = $2`, tenant, "event:"+id.String()); n != 1 {
			t.Errorf("event %s: audit %d", id, n)
		}
	}
	for _, id := range []uuid.UUID{notDueA.ID, futureA.ID} {
		if n := count(t, `SELECT count(*) FROM guests WHERE event_id = $1 AND purged_at IS NULL AND name_enc IS NOT NULL`, id); n != 1 {
			t.Errorf("event %s was not due but lost its data", id)
		}
	}
	// Scheduled rows exist for the rest.
	if n := count(t, `SELECT count(*) FROM event_purges WHERE event_id = $1 AND purged_at IS NULL AND purge_after = $2`,
		notDueA.ID, notDueA.EndsAt.Add(30*24*time.Hour)); n != 1 {
		t.Errorf("not-due event schedule row: %d", n)
	}

	// A second run purges nothing again (idempotent).
	res, err = s.retention.RunDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if mine(r.TenantID) {
			t.Errorf("second run purged %s again", r.EventID)
		}
	}
	if n := count(t, `SELECT count(*) FROM outbox WHERE tenant_id IN ($1, $2) AND subject LIKE '%.retention.purged'`, a.id, b.id); n != 4 {
		t.Fatalf("one retention.purged event per purge, got %d", n)
	}

	// Moving an end later un-dues an event before the job reaches it.
	later := a.night("Moved", now.Add(-40*24*time.Hour))
	addGuest(a, later)
	if _, err := testDB.Owner.Exec(context.Background(), `UPDATE events SET ends_at = $2 WHERE id = $1`, later.ID, s.clock.t.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.retention.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT count(*) FROM guests WHERE event_id = $1 AND purged_at IS NULL`, later.ID); n != 1 {
		t.Fatal("an event whose end moved later must wait")
	}
}

func TestEnsureNotPurgedIgnoresUnknownEvents(t *testing.T) {
	s := newStack(t)
	a := s.newOrg(t)
	err := s.db.WithTenant(context.Background(), a.id, func(tx pgx.Tx) error {
		return retention.EnsureNotPurged(context.Background(), tx, uuid.New())
	})
	if err != nil || errors.Is(err, retention.ErrEventPurged) {
		t.Fatalf("unknown event: %v", err)
	}
}

func TestWorkerRunsAtStartAndStopsWithContext(t *testing.T) {
	s := newStack(t)
	a := s.newOrg(t)
	s.clock.t = time.Now().UTC()
	ev := a.night("Worker Night", s.clock.t.Add(-60*24*time.Hour))
	l := must(s.guests.CreateList(a.ctx, ev.ID, guest.ListInput{Name: "Comp", Type: "comp"}))
	must(s.guests.AddGuests(a.ctx, ev.ID, guest.AddInput{ListID: l.ID, Guests: []guest.GuestInput{{Name: "Ana"}}}, staffSub))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	w := &retention.Worker{Service: s.retention, Interval: time.Hour}
	go func() {
		defer close(done)
		w.Run(ctx)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for count(t, `SELECT count(*) FROM event_purges WHERE event_id = $1 AND purged_at IS NOT NULL`, ev.ID) == 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("the worker did not purge the due event at start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not stop with its context")
	}
}
