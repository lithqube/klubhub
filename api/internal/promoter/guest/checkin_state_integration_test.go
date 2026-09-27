package guest_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/promoter/guest"
)

// checkin writes a door check-in row straight into the table (as the door
// sync would), optionally undone. subject is a guest or, with ticket, an
// order position.
func (f fixture) checkin(t *testing.T, eventID, device, subject uuid.UUID, ticket bool, count int, dir string, when time.Time, undone bool) {
	t.Helper()
	col := "guest_id"
	if ticket {
		col = "position_id"
	}
	var undoneAt *time.Time
	if undone {
		undoneAt = &when
	}
	if _, err := testDB.Owner.Exec(context.Background(), `INSERT INTO checkins (id, tenant_id, event_id, `+col+`, count, direction, client_nonce, device_id, at, undone_at)
	  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		uuid.Must(uuid.NewV7()), f.org, eventID, subject, count, dir, "gt-"+uuid.NewString(), device, when, undoneAt); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) doorDevice(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	hash := make([]byte, 32)
	_, _ = rand.Read(hash)
	if _, err := testDB.Owner.Exec(context.Background(), `INSERT INTO door_devices (id, tenant_id, label, token_hash) VALUES ($1, $2, 'Door 1', $3)`,
		id, f.org, hash); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGuestTableShowsCheckInState(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	crew := f.list(t, d.ID, guest.ListInput{Name: "Crew", Type: "crew"})
	vip := f.list(t, d.ID, guest.ListInput{Name: "VIP", Type: "vip"})
	added := f.add(t, d.ID, guest.AddInput{ListID: vip.ID, Guests: []guest.GuestInput{
		{Name: "Mara Weiss", PlusN: 2},                  // 3 in, 1 out → 2 inside
		{Name: "Tom Berg", PlusN: 1},                    // 1 in, then undone → not arrived
		{Name: "Ines Roth"},                             // 1 in, 1 out → left again
		{Name: "Pia Lang", Status: guest.StatusPending}, // pending, admitted anyway
		{Name: "Kai Noor"},                              // never came
	}}).Added
	crewGuest := f.add(t, d.ID, guest.AddInput{ListID: crew.ID, Guests: []guest.GuestInput{{Name: "Ola Sund"}}}).Added[0]
	mara, tom, ines, pia := added[0], added[1], added[2], added[3]
	dev := f.doorDevice(t)
	first := at(3, 23, 40)
	f.checkin(t, d.ID, dev, mara.ID, false, 1, "in", first.Add(5*time.Minute), false)
	f.checkin(t, d.ID, dev, mara.ID, false, 2, "in", first, false)
	f.checkin(t, d.ID, dev, mara.ID, false, 1, "out", first.Add(time.Hour), false)
	f.checkin(t, d.ID, dev, mara.ID, false, 3, "in", first.Add(-time.Hour), true) // undone: ignored, also for first_in_at
	f.checkin(t, d.ID, dev, tom.ID, false, 1, "in", first, true)
	f.checkin(t, d.ID, dev, ines.ID, false, 1, "in", first, false)
	f.checkin(t, d.ID, dev, ines.ID, false, 1, "out", first.Add(time.Minute), false)
	f.checkin(t, d.ID, dev, ines.ID, false, 1, "out", first.Add(2*time.Minute), false) // more out than in: floored at 0
	f.checkin(t, d.ID, dev, pia.ID, false, 1, "in", first, false)
	f.checkin(t, d.ID, dev, crewGuest.ID, false, 1, "in", first, false)

	page, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]guest.Guest{}
	for _, g := range page.Guests {
		byName[g.Name] = g
	}
	check := func(name string, heads int, firstIn *time.Time) {
		t.Helper()
		g := byName[name]
		if g.HeadsIn != heads || (firstIn == nil) != (g.FirstInAt == nil) || (firstIn != nil && !g.FirstInAt.Equal(*firstIn)) {
			t.Fatalf("%s: heads_in %d first_in_at %v, want %d %v", name, g.HeadsIn, g.FirstInAt, heads, firstIn)
		}
	}
	check("Mara Weiss", 2, &first)
	check("Tom Berg", 0, nil)
	check("Ines Roth", 0, &first)
	check("Pia Lang", 1, &first)
	check("Kai Noor", 0, nil)
	check("Ola Sund", 1, &first)
	if page.Counts.CheckedIn != 3 || page.Counts.All != 6 {
		t.Fatalf("counts: %+v", page.Counts)
	}

	// The filter: live heads in, any status; counts ignore the status filter.
	in, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Status: guest.StatusCheckedIn})
	if err != nil || len(in.Guests) != 3 || in.Guests[0].Name != "Mara Weiss" || in.Guests[1].Name != "Pia Lang" || in.Guests[2].Name != "Ola Sund" {
		t.Fatalf("checked_in filter: %+v %v", in.Guests, err)
	}
	if in.Counts.CheckedIn != 3 || in.Counts.Going != 5 || in.Counts.Pending != 1 || len(in.Tickets) != 0 {
		t.Fatalf("checked_in counts / tickets: %+v %d", in.Counts, len(in.Tickets))
	}
	inVIP, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Status: guest.StatusCheckedIn, ListID: &vip.ID})
	if err != nil || len(inVIP.Guests) != 2 || inVIP.Counts.CheckedIn != 2 || inVIP.Counts.All != 5 {
		t.Fatalf("checked_in + list filter: %+v %+v %v", inVIP.Guests, inVIP.Counts, err)
	}
	going, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Status: guest.StatusGoing, ListID: &crew.ID})
	if err != nil || len(going.Guests) != 1 || going.Guests[0].HeadsIn != 1 || going.Counts.CheckedIn != 1 {
		t.Fatalf("status filter keeps check-in state: %+v %v", going, err)
	}

	// An update returns the guest with its check-in state.
	upd, err := f.svc.UpdateGuest(f.ctx, d.ID, mara.ID, guest.GuestInput{Name: "Mara Weiss", PlusN: 2, Note: "at the bar"})
	if err != nil || upd.HeadsIn != 2 || upd.FirstInAt == nil || !upd.FirstInAt.Equal(first) {
		t.Fatalf("update: %+v %v", upd, err)
	}

	// The CSV export does not take the derived filter.
	var inv *guest.InvalidError
	if _, _, err := f.svc.Export(f.ctx, d.ID, guest.GuestFilter{Status: guest.StatusCheckedIn}, staff); !errors.As(err, &inv) {
		t.Fatalf("export with checked_in: %v", err)
	}

	// Tickets: checked_in once a non-undone `in` exists.
	f.importCSV(t, d.ID, guest.PresetPretix, fixtureCSV(t, "pretix.csv"), nil, false)
	page, err = f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{})
	if err != nil || len(page.Tickets) != 4 {
		t.Fatalf("tickets: %d %v", len(page.Tickets), err)
	}
	t0, t1 := page.Tickets[0], page.Tickets[1]
	f.checkin(t, d.ID, dev, t0.ID, true, 1, "in", first, false)
	f.checkin(t, d.ID, dev, t0.ID, true, 1, "out", first.Add(time.Minute), false)
	f.checkin(t, d.ID, dev, t1.ID, true, 1, "in", first, true)
	page, err = f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range page.Tickets {
		want := tk.ID == t0.ID
		if tk.CheckedIn != want || (tk.FirstInAt != nil) != want || (want && !tk.FirstInAt.Equal(first)) {
			t.Fatalf("ticket %s: checked_in %v first_in_at %v", tk.OrderRef, tk.CheckedIn, tk.FirstInAt)
		}
	}
	if page.Counts.CheckedIn != 3 {
		t.Fatalf("tickets are not guests: %+v", page.Counts)
	}
}
