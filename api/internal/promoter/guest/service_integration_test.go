package guest_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/guest"
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
	events *event.Service
	svc    *guest.Service
	ctx    context.Context
	clock  *clock
	org    uuid.UUID
}

const staff = "local:0190f1d2-7c1a-7a00-9f00-00000000beef"

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
	keys := envelope.NewKeyring(kek)
	c := &clock{t: at(1, 12, 0)}
	db := tenantdb.New(testDB.App)
	f := fixture{
		events: event.NewService(db, keys, c.now),
		svc:    guest.NewService(db, keys, c.now),
		ctx:    tenantdb.ContextWithTenant(context.Background(), org),
		clock:  c, org: org,
	}
	f.events.OnCreate(f.svc.CopyStandingLists)
	return f
}

func (f fixture) night(t *testing.T) event.Detail {
	t.Helper()
	d, err := f.events.CreateEvent(f.ctx, event.EventInput{
		Title: "Klubnacht 03", StartsAt: at(3, 23, 0), EndsAt: at(4, 7, 0), Timezone: "Europe/Berlin", City: "Berlin",
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f fixture) list(t *testing.T, eventID uuid.UUID, in guest.ListInput) guest.List {
	t.Helper()
	l, err := f.svc.CreateList(f.ctx, eventID, in)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func (f fixture) add(t *testing.T, eventID uuid.UUID, in guest.AddInput) guest.AddResult {
	t.Helper()
	r, err := f.svc.AddGuests(f.ctx, eventID, in, staff)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStandingListsAreCopiedIntoNewEvents(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.CreateStanding(f.ctx, guest.ListInput{Name: "Residents", Type: "artist",
		EntryTerms: guest.EntryTerms{CutoffLocal: ptr("01:00"), Perks: []string{"Drink token"}}}); err != nil {
		t.Fatal(err)
	}
	crew, err := f.svc.CreateStanding(f.ctx, guest.ListInput{Name: "Crew", Type: "crew", CollectContact: true})
	if err != nil {
		t.Fatal(err)
	}
	d := f.night(t)
	lists, err := f.svc.ListLists(f.ctx, d.ID)
	if err != nil || len(lists) != 2 {
		t.Fatalf("lists: %+v %v", lists, err)
	}
	r := lists[0]
	if r.Name != "Residents" || r.StandingTemplateID == nil || r.EntryTerms.CutoffLocal != nil ||
		r.EntryTerms.CutoffAt == nil || !r.EntryTerms.CutoffAt.Equal(at(4, 1, 0)) || r.EntryTerms.Perks[0] != "drink token" {
		t.Fatalf("copied list must carry an absolute cutoff for this night: %+v", r)
	}
	if !lists[1].CollectContact || lists[1].Type != "crew" {
		t.Fatalf("second copy: %+v", lists[1])
	}
	// Editing or deleting a template leaves existing copies alone.
	if _, err := f.svc.UpdateStanding(f.ctx, crew.ID, guest.ListInput{Name: "Crew & Staff", Type: "crew"}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteStanding(f.ctx, crew.ID); err != nil {
		t.Fatal(err)
	}
	lists, _ = f.svc.ListLists(f.ctx, d.ID)
	if len(lists) != 2 || lists[1].Name != "Crew" || lists[1].StandingTemplateID != nil {
		t.Fatalf("copies are independent of their template: %+v", lists[1])
	}
	if st, _ := f.svc.ListStanding(f.ctx); len(st) != 1 {
		t.Fatalf("standing lists after delete: %d", len(st))
	}
}

func TestGuestsAreSealedAndNameOnlyByDefault(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	nameOnly := f.list(t, d.ID, guest.ListInput{Name: "Comp", Type: "comp"})
	var inv *guest.InvalidError
	_, err := f.svc.AddGuests(f.ctx, d.ID, guest.AddInput{ListID: nameOnly.ID, Guests: []guest.GuestInput{{Name: "Mara", Email: "mara@example.org"}}}, staff)
	if !errors.As(err, &inv) || inv.Field != "guests[0].email" {
		t.Fatalf("a name-only list must refuse emails, got %v", err)
	}

	contact := f.list(t, d.ID, guest.ListInput{Name: "Industry", Type: "industry", CollectContact: true})
	res := f.add(t, d.ID, guest.AddInput{ListID: contact.ID, Guests: []guest.GuestInput{
		{Name: "José Müller", Email: "Jose@Label.example", Phone: "+49 170 1234567", Note: "label A&R", PlusN: 1},
	}})
	if len(res.Added) != 1 || res.Added[0].Status != guest.StatusGoing || res.Added[0].Source != guest.SourceManual {
		t.Fatalf("add: %+v", res)
	}
	id := res.Added[0].ID
	var name, email, phone, note, nameIdx, emailIdx []byte
	var createdBy string
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT name_enc, email_enc, phone_enc, note_enc, name_bidx, email_bidx, created_by FROM guests WHERE id = $1`, id).
		Scan(&name, &email, &phone, &note, &nameIdx, &emailIdx, &createdBy); err != nil {
		t.Fatal(err)
	}
	for _, v := range [][]byte{name, email, phone, note} {
		for _, plain := range []string{"Müller", "Label.example", "1234567", "A&R"} {
			if bytes.Contains(v, []byte(plain)) {
				t.Fatalf("personal data stored in plaintext: %q", plain)
			}
		}
	}
	if len(nameIdx) != 32 || len(emailIdx) != 32 || createdBy != staff {
		t.Fatalf("blind indexes / created_by: %d %d %q", len(nameIdx), len(emailIdx), createdBy)
	}

	// Exact lookups go through the blind indexes: case and accents fold.
	for _, q := range []string{"jose@label.EXAMPLE", "jose muller", "JOSÉ  MÜLLER"} {
		page, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Q: q})
		if err != nil || len(page.Guests) != 1 || page.Guests[0].Name != "José Müller" || page.Guests[0].Phone != "+49 170 1234567" {
			t.Fatalf("lookup %q: %+v %v", q, page.Guests, err)
		}
	}

	// Turning contact collection off erases stored contacts.
	if _, err := f.svc.UpdateList(f.ctx, d.ID, contact.ID, guest.ListInput{Name: "Industry", Type: "industry"}); err != nil {
		t.Fatal(err)
	}
	page, _ := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{})
	if len(page.Guests) != 1 || page.Guests[0].Email != "" || page.Guests[0].Phone != "" || page.Guests[0].Note != "label A&R" {
		t.Fatalf("contacts must be erased, the note kept: %+v", page.Guests)
	}
}

func TestAllocationRulesAreEnforced(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	l := f.list(t, d.ID, guest.ListInput{Name: "Artist guests", Type: "artist"})
	a, err := f.svc.CreateAllocation(f.ctx, d.ID, l.ID, guest.AllocationInput{
		Label: "Ben Klock", SubmitterContact: ptr("tour@agency.example"), Quota: 4, PlusNMax: 1,
		Deadline: ptr(at(3, 18, 0)), RequiresApproval: true,
	})
	if err != nil || a.SubmitterContact != "tour@agency.example" || a.Used != 0 {
		t.Fatalf("allocation: %+v %v", a, err)
	}
	var contact []byte
	_ = testDB.Owner.QueryRow(context.Background(), `SELECT submitter_contact_enc FROM guest_allocations WHERE id = $1`, a.ID).Scan(&contact)
	if len(contact) == 0 || bytes.Contains(contact, []byte("agency")) {
		t.Fatal("submitter contact must be sealed")
	}

	res := f.add(t, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &a.ID, Source: guest.SourcePaste, Guests: []guest.GuestInput{
		{Name: "Anna", PlusN: 1}, {Name: "Tom"},
	}})
	if len(res.Added) != 2 || res.Added[0].Status != guest.StatusPending || res.Added[0].Source != guest.SourcePaste {
		t.Fatalf("needs-approval allocations add pending guests: %+v", res.Added)
	}

	var inv *guest.InvalidError
	if _, err := f.svc.AddGuests(f.ctx, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &a.ID, Guests: []guest.GuestInput{{Name: "Kim", PlusN: 2}}}, staff); !errors.As(err, &inv) || inv.Field != "guests[0].plus_n" {
		t.Fatalf("+2 on a +1 allocation must be refused, got %v", err)
	}
	var quota *guest.QuotaError
	if _, err := f.svc.AddGuests(f.ctx, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &a.ID, Guests: []guest.GuestInput{{Name: "Kim", PlusN: 1}}}, staff); !errors.As(err, &quota) || quota.Used != 3 || quota.Requested != 2 || quota.Quota != 4 {
		t.Fatalf("3 used + 2 heads on a quota of 4 must be refused, got %v", err)
	}
	// Waitlisted guests do not hold quota, so they fit.
	res = f.add(t, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &a.ID, Guests: []guest.GuestInput{{Name: "Kim", PlusN: 1, Status: guest.StatusWaitlist}}})
	kim := res.Added[0]
	if _, err := f.svc.UpdateGuest(f.ctx, d.ID, kim.ID, guest.GuestInput{Name: "Kim", PlusN: 1, Status: guest.StatusGoing}); !errors.As(err, &quota) {
		t.Fatalf("moving a waitlisted +1 to going over quota must be refused, got %v", err)
	}
	if _, err := f.svc.UpdateGuest(f.ctx, d.ID, kim.ID, guest.GuestInput{Name: "Kim", Status: guest.StatusGoing}); err != nil {
		t.Fatalf("Kim alone fits exactly: %v", err)
	}
	if _, err := f.svc.UpdateAllocation(f.ctx, d.ID, l.ID, a.ID, guest.AllocationInput{Label: "Ben Klock", Quota: 3, PlusNMax: 1}); !errors.As(err, &inv) || inv.Field != "quota" {
		t.Fatalf("a quota below the heads on it must be refused, got %v", err)
	}

	f.clock.t = at(3, 18, 1)
	if _, err := f.svc.AddGuests(f.ctx, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &a.ID, Guests: []guest.GuestInput{{Name: "Late", Status: guest.StatusWaitlist}}}, staff); !errors.Is(err, guest.ErrAllocationClosed) {
		t.Fatalf("after the deadline the allocation is closed, got %v", err)
	}
	if err := f.svc.RevokeAllocation(f.ctx, d.ID, l.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddGuests(f.ctx, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &a.ID, Guests: []guest.GuestInput{{Name: "Late"}}}, staff); !errors.Is(err, guest.ErrAllocationRevoked) {
		t.Fatalf("a revoked allocation takes no guests, got %v", err)
	}
	lists, _ := f.svc.ListLists(f.ctx, d.ID)
	if len(lists[0].Allocations) != 1 || lists[0].Allocations[0].RevokedAt == nil || lists[0].Allocations[0].Guests != 3 || lists[0].Quota != 0 {
		t.Fatalf("revoked allocation keeps its guests and leaves the list quota: %+v", lists[0])
	}

	// An allocation of another list is refused.
	other := f.list(t, d.ID, guest.ListInput{Name: "VIP", Type: "vip"})
	if _, err := f.svc.AddGuests(f.ctx, d.ID, guest.AddInput{ListID: other.ID, AllocationID: &a.ID, Guests: []guest.GuestInput{{Name: "X"}}}, staff); !errors.As(err, &inv) || inv.Field != "allocation_id" {
		t.Fatalf("got %v", err)
	}
}

func TestDuplicatesAreSkipped(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	a := f.list(t, d.ID, guest.ListInput{Name: "A", Type: "comp", CollectContact: true})
	b := f.list(t, d.ID, guest.ListInput{Name: "B", Type: "comp", CollectContact: true})
	f.add(t, d.ID, guest.AddInput{ListID: a.ID, Guests: []guest.GuestInput{{Name: "Lena", Email: "lena@example.org"}, {Name: "Sam"}}})
	res := f.add(t, d.ID, guest.AddInput{ListID: b.ID, Guests: []guest.GuestInput{
		{Name: "Lena W", Email: "LENA@example.org"}, // same email, other list: duplicate
		{Name: "Sam"},  // same name, other list: allowed
		{Name: "Sam"},  // repeated in the batch: duplicate
		{Name: "Ines"}, // new
	}})
	if len(res.Added) != 2 || fmt.Sprint(res.Duplicates) != "[0 2]" {
		t.Fatalf("duplicates: added %d, dup %v", len(res.Added), res.Duplicates)
	}
}

func TestBulkStatusByEmailAndCounts(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	l := f.list(t, d.ID, guest.ListInput{Name: "Industry", Type: "industry", CollectContact: true})
	alloc, _ := f.svc.CreateAllocation(f.ctx, d.ID, l.ID, guest.AllocationInput{Label: "Label night", Quota: 2})
	f.add(t, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &alloc.ID, Guests: []guest.GuestInput{
		{Name: "A", Email: "a@example.org", Status: guest.StatusInvited},
		{Name: "B", Email: "b@example.org", Status: guest.StatusWaitlist},
		{Name: "C", Email: "c@example.org", Status: guest.StatusWaitlist},
	}})
	res, err := f.svc.BulkStatus(f.ctx, d.ID, guest.BulkStatusInput{Status: guest.StatusGoing, Emails: []string{" A@example.org", "b@example.org", "nobody@example.org", "a@example.org"}})
	if err != nil || res.Matched != 2 || res.Updated != 2 || len(res.Unmatched) != 1 || res.Unmatched[0] != "nobody@example.org" {
		t.Fatalf("bulk: %+v %v", res, err)
	}
	var quota *guest.QuotaError
	if _, err := f.svc.BulkStatus(f.ctx, d.ID, guest.BulkStatusInput{Status: guest.StatusGoing, Emails: []string{"c@example.org"}}); !errors.As(err, &quota) {
		t.Fatalf("bulk moves are quota-checked, got %v", err)
	}
	page, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Status: guest.StatusGoing})
	if err != nil || len(page.Guests) != 2 || page.Counts.Going != 2 || page.Counts.Waitlist != 1 || page.Counts.All != 3 || page.Counts.GoingHeads != 2 {
		t.Fatalf("filtered page / counts: %+v %v", page, err)
	}
	var inv *guest.InvalidError
	if _, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Status: "checked_in"}); !errors.As(err, &inv) {
		t.Fatalf("unknown status filter: %v", err)
	}

	// The outbox carries identifiers only.
	rows, _ := testDB.Owner.Query(context.Background(), `SELECT subject, payload::text FROM outbox WHERE tenant_id = $1`, f.org)
	defer rows.Close()
	changes := 0
	for rows.Next() {
		var subject, payload string
		_ = rows.Scan(&subject, &payload)
		if strings.HasSuffix(subject, ".guest.status_changed") {
			changes++
		}
		if strings.Contains(payload, "example.org") || strings.Contains(payload, "Label night") {
			t.Fatalf("personal data on the bus: %s", payload)
		}
	}
	if changes != 2 {
		t.Fatalf("guest.status_changed events: %d", changes)
	}
}

func TestExportIsAuditedAndListDeleteNeedsForce(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	l := f.list(t, d.ID, guest.ListInput{Name: "Comp", Type: "comp"})
	f.add(t, d.ID, guest.AddInput{ListID: l.ID, Guests: []guest.GuestInput{{Name: "=HYPERLINK(1)", PlusN: 2}}})
	slug, rows, err := f.svc.Export(f.ctx, d.ID, guest.GuestFilter{}, staff)
	if err != nil || slug != "klubnacht-03" || len(rows) != 1 || rows[0].List != "Comp" || rows[0].PlusN != 2 {
		t.Fatalf("export: %q %+v %v", slug, rows, err)
	}
	var n int
	_ = testDB.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'guestlist.export' AND tenant_id = $1`, f.org).Scan(&n)
	if n != 1 {
		t.Fatalf("export must be audited once, got %d", n)
	}

	var notEmpty *guest.NotEmptyError
	if err := f.svc.DeleteList(f.ctx, d.ID, l.ID, false); !errors.As(err, &notEmpty) || notEmpty.Guests != 1 {
		t.Fatalf("a list with guests needs force, got %v", err)
	}
	if err := f.svc.DeleteList(f.ctx, d.ID, l.ID, true); err != nil {
		t.Fatal(err)
	}
	if page, _ := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{}); len(page.Guests) != 0 {
		t.Fatal("forced delete removes the guests")
	}
}

func TestOverviewAndTenantIsolation(t *testing.T) {
	a, b := setup(t), setup(t)
	d := a.night(t)
	l := a.list(t, d.ID, guest.ListInput{Name: "Artist", Type: "artist"})
	al, _ := a.svc.CreateAllocation(a.ctx, d.ID, l.ID, guest.AllocationInput{Label: "Dasha Rush", Quota: 10, PlusNMax: 2, RequiresApproval: true})
	a.add(t, d.ID, guest.AddInput{ListID: l.ID, AllocationID: &al.ID, Guests: []guest.GuestInput{{Name: "P1", PlusN: 2}, {Name: "P2"}}})
	a.add(t, d.ID, guest.AddInput{ListID: l.ID, Guests: []guest.GuestInput{{Name: "G1", PlusN: 1}}})

	ov, err := a.svc.Overview(a.ctx)
	if err != nil || len(ov) != 1 {
		t.Fatalf("overview: %+v %v", ov, err)
	}
	if o := ov[0]; o.Lists != 1 || o.Guests != 3 || o.Pending != 2 || o.GoingHeads != 2 || o.Used != 4 || o.Quota != 10 {
		t.Fatalf("overview row: %+v", o)
	}

	if _, err := b.svc.ListLists(b.ctx, d.ID); !errors.Is(err, guest.ErrNotFound) {
		t.Fatalf("another tenant's event must be invisible, got %v", err)
	}
	if _, err := b.svc.ListGuests(b.ctx, d.ID, guest.GuestFilter{}); !errors.Is(err, guest.ErrNotFound) {
		t.Fatalf("another tenant's guests must be invisible, got %v", err)
	}
	if _, err := b.svc.AddGuests(b.ctx, d.ID, guest.AddInput{ListID: l.ID, Guests: []guest.GuestInput{{Name: "X"}}}, staff); !errors.Is(err, guest.ErrNotFound) {
		t.Fatalf("writing into another tenant's list must fail, got %v", err)
	}
	if ov, _ := b.svc.Overview(b.ctx); len(ov) != 0 {
		t.Fatalf("tenant B sees %d of A's events", len(ov))
	}
}
