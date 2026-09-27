package guest_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/promoter/guest"
)

func csvTable(t *testing.T, text string) *guest.Table {
	t.Helper()
	tab, err := guest.ParseCSV([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

func fixtureCSV(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "import", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (f fixture) importCSV(t *testing.T, eventID uuid.UUID, preset, text string, mapping map[string]string, dry bool) guest.ImportResult {
	t.Helper()
	res, err := f.svc.ImportAttendees(f.ctx, eventID, guest.ImportInput{Preset: preset, Mapping: mapping, Table: csvTable(t, text)}, dry, staff)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.Owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func counts(r guest.ImportResult) string {
	c := r.Counts
	return fmt.Sprintf("rows=%d orders+%d~%d tickets+%d~%d=%d rejected=%d absent=%d onlist=%d",
		c.Rows, c.OrdersNew, c.OrdersUpdated, c.PositionsNew, c.PositionsUpdated, c.PositionsUnchanged, c.Rejected, c.NotInFile, c.OnGuestList)
}

func TestImportDryRunThenApply(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	pretix := fixtureCSV(t, "pretix.csv")

	dry := f.importCSV(t, d.ID, guest.PresetPretix, pretix, nil, true)
	if got := counts(dry); got != "rows=4 orders+3~0 tickets+4~0=0 rejected=0 absent=0 onlist=0" {
		t.Fatalf("dry-run counts: %s", got)
	}
	if !dry.DryRun || dry.ImportID != nil || fmt.Sprint(dry.TicketTypes) != "[{Early bird true 2 2} {Regular true 2 0}]" {
		t.Fatalf("dry run: %+v", dry)
	}
	if len(dry.Preview) != 4 || dry.Preview[0].Name != "Ma… W…" || dry.Preview[0].Email != "m…@e…" || dry.Preview[0].Action != "new" ||
		dry.Preview[2].Status != guest.TicketCancelled || dry.Preview[0].OrderRef != "AB12C" {
		t.Fatalf("preview must be masked: %+v", dry.Preview)
	}
	if dry.Mapping[guest.FieldSecret] != "Secret" || dry.Encoding != "utf-8" {
		t.Fatalf("mapping: %v %s", dry.Mapping, dry.Encoding)
	}
	if n := f.count(t, `SELECT (SELECT count(*) FROM order_positions WHERE tenant_id = $1) + (SELECT count(*) FROM ticket_types WHERE tenant_id = $1)
	  + (SELECT count(*) FROM attendee_imports WHERE tenant_id = $1) + (SELECT count(*) FROM outbox WHERE tenant_id = $1 AND subject LIKE '%attendees%')`, f.org); n != 0 {
		t.Fatalf("a dry run must write nothing, found %d rows", n)
	}

	res := f.importCSV(t, d.ID, guest.PresetPretix, pretix, nil, false)
	if res.DryRun || res.ImportID == nil || counts(res) != counts(dry) {
		t.Fatalf("apply: %+v", res)
	}
	if n := f.count(t, `SELECT count(*) FROM orders WHERE tenant_id = $1 AND source = 'pretix' AND import_id = $2`, f.org, *res.ImportID); n != 3 {
		t.Fatalf("orders: %d", n)
	}
	// Personal data is sealed; the barcode is only a blind index and ciphertext.
	rows, err := testDB.Owner.Query(context.Background(), `SELECT p.attendee_name_enc, COALESCE(p.attendee_email_enc, ''), p.secret_enc, p.secret_bidx,
	  o.buyer_name_enc, o.buyer_email_enc FROM order_positions p JOIN orders o ON o.id = p.order_id WHERE p.tenant_id = $1`, f.org)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, c, idx, e, g []byte
		if err := rows.Scan(&a, &b, &c, &idx, &e, &g); err != nil {
			t.Fatal(err)
		}
		for _, v := range [][]byte{a, b, c, e, g} {
			for _, plain := range []string{"Weiss", "Müller", "example.org", "k2x9fvq8", "p7wd3nn2"} {
				if bytes.Contains(v, []byte(plain)) {
					t.Fatalf("plaintext %q stored", plain)
				}
			}
		}
		if len(idx) != 32 {
			t.Fatal("every pretix ticket has a secret index")
		}
	}
	rows.Close()

	// The bus carries identifiers only; the import is audited with counts.
	var payload string
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT payload::text FROM outbox WHERE tenant_id = $1 AND subject LIKE '%.attendees.imported'`, f.org).Scan(&payload); err != nil {
		t.Fatalf("attendees.imported: %v", err)
	}
	if !strings.Contains(payload, res.ImportID.String()) || strings.Contains(payload, "example.org") || strings.Contains(payload, "Mara") {
		t.Fatalf("event payload: %s", payload)
	}
	var reason string
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT reason FROM audit_log WHERE tenant_id = $1 AND action = 'attendees.import'`, f.org).Scan(&reason); err != nil ||
		!strings.Contains(reason, "4 rows, 4 new and 0 updated tickets") {
		t.Fatalf("audit: %q %v", reason, err)
	}

	// Ticket holders appear in the guest table.
	page, err := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{})
	if err != nil || len(page.Tickets) != 4 || page.Counts.Tickets != 2 {
		t.Fatalf("guest table tickets: %+v %v", page.Counts, err)
	}
	byName := map[string]guest.Ticket{}
	for _, tk := range page.Tickets {
		byName[tk.Name] = tk
	}
	if tk := byName["José Müller"]; tk.Email != "buyer@example.org" || tk.TicketType != "Early bird" || tk.Source != "pretix" || tk.OrderRef != "AB12C" || tk.Status != guest.TicketValid {
		t.Fatalf("ticket: %+v", tk)
	}
	if byName[`Ines "Nes" Duarte`].Status != guest.TicketPending {
		t.Fatalf("pending ticket: %+v", byName)
	}
	for _, q := range []string{"TOM@example.org", "jose muller"} {
		if p, _ := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Q: q}); len(p.Tickets) != 1 {
			t.Fatalf("exact lookup %q: %d tickets", q, len(p.Tickets))
		}
	}
	if p, _ := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{Status: guest.StatusGoing}); len(p.Tickets) != 0 {
		t.Fatal("a guest-status filter leaves tickets out")
	}
	if ov, _ := f.svc.Overview(f.ctx); len(ov) != 1 || ov[0].Tickets != 2 {
		t.Fatalf("overview tickets: %+v", ov)
	}
}

func TestReimportUpdatesInsteadOfDuplicating(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	pretix := fixtureCSV(t, "pretix.csv")
	f.importCSV(t, d.ID, guest.PresetPretix, pretix, nil, false)

	again := f.importCSV(t, d.ID, guest.PresetPretix, pretix, nil, false)
	if got := counts(again); got != "rows=4 orders+0~0 tickets+0~0=4 rejected=0 absent=0 onlist=0" {
		t.Fatalf("same file again: %s", got)
	}
	if fmt.Sprint(again.TicketTypes) != "[{Early bird false 2 2} {Regular false 2 0}]" {
		t.Fatalf("known ticket types: %v", again.TicketTypes)
	}

	// Mara is refunded, José's name is corrected, AB12C gains a ticket,
	// XY34Z is gone from the export and the buyer of QQ55R changed email.
	changed := strings.NewReplacer(
		"AB12C,1,Paid", "AB12C,1,Refunded",
		"José Müller,", "José Müller-Ruiz,",
		"XY34Z,3,Canceled,tom@example.org,2026-09-02,Regular,,20.00,Tomasz Nowak,,q9ee4bb1v6m2kd8t\r\n", "",
		"ines@example.org", "nes@example.org",
	).Replace(pretix) + "AB12C,5,Paid,buyer@example.org,2026-09-01,Late,,25.00,Kim Lee,,newsecret0005\r\n"
	dry := f.importCSV(t, d.ID, guest.PresetPretix, changed, nil, true)
	if got := counts(dry); got != "rows=4 orders+0~1 tickets+1~3=0 rejected=0 absent=1 onlist=0" {
		t.Fatalf("changed file: %s", got)
	}
	actions := map[string]string{}
	for _, p := range dry.Preview {
		actions[p.Name] = p.Action
	}
	if actions["Ma… W…"] != "update" || actions["Ki… L…"] != "new" {
		t.Fatalf("preview actions: %v", actions)
	}
	f.importCSV(t, d.ID, guest.PresetPretix, changed, nil, false)
	if n := f.count(t, `SELECT count(*) FROM order_positions WHERE tenant_id = $1`, f.org); n != 5 {
		t.Fatalf("tickets after re-import: %d (no duplicates, nothing deleted)", n)
	}
	if n := f.count(t, `SELECT count(*) FROM ticket_types WHERE tenant_id = $1`, f.org); n != 3 {
		t.Fatalf("ticket types: %d", n)
	}
	page, _ := f.svc.ListGuests(f.ctx, d.ID, guest.GuestFilter{})
	status := map[string]string{}
	for _, tk := range page.Tickets {
		status[tk.Name] = tk.Status
	}
	if status["Mara Weiss"] != guest.TicketRefunded || status["José Müller-Ruiz"] != guest.TicketValid || status["Tomasz Nowak"] != guest.TicketCancelled ||
		status[`Ines "Nes" Duarte`] != guest.TicketPending || status["Kim Lee"] != guest.TicketValid {
		t.Fatalf("statuses after re-import: %v", status)
	}
	if again := f.importCSV(t, d.ID, guest.PresetPretix, changed, nil, true); counts(again) != "rows=4 orders+0~0 tickets+0~0=4 rejected=0 absent=1 onlist=0" {
		t.Fatalf("applied changes are stable: %s", counts(again))
	}
}

func TestReimportWithoutTicketIdsMatchesByBarcodeThenPosition(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	mapping := map[string]string{guest.FieldName: "Name", guest.FieldOrderRef: "Order", guest.FieldSecret: "Code", guest.FieldTicketType: "Type"}
	first := "Name,Order,Code,Type\nAnna,O1,,GA\nBen,O1,,GA\nCleo,O2,C-2,GA\n"
	if got := counts(f.importCSV(t, d.ID, guest.PresetGeneric, first, mapping, false)); got != "rows=3 orders+2~0 tickets+3~0=0 rejected=0 absent=0 onlist=0" {
		t.Fatalf("first: %s", got)
	}
	// Same tickets, reordered, one renamed: matched by the n-th ticket of
	// the order and by barcode, so nothing is duplicated.
	second := "Name,Order,Code,Type\nCleo B,O2,C-2,GA\nAnna,O1,,GA\nBen,O1,,GA\n"
	if got := counts(f.importCSV(t, d.ID, guest.PresetGeneric, second, mapping, false)); got != "rows=3 orders+0~1 tickets+0~1=2 rejected=0 absent=0 onlist=0" { // Cleo is also the buyer of O2
		t.Fatalf("second: %s", got)
	}
	// A barcode appearing twice in the file, or one another source already
	// owns in this event, is refused by line.
	third := "Name,Order,Code,Type\nDan,O3,C-9,GA\nEva,O4,C-9,GA\n"
	res := f.importCSV(t, d.ID, guest.PresetGeneric, third, mapping, true)
	if res.Counts.Rejected != 1 || res.Rejected[0].Line != 3 || res.Rejected[0].Reason != "same barcode as line 2" {
		t.Fatalf("duplicate barcode: %+v", res.Rejected)
	}
	other := f.importCSV(t, d.ID, guest.PresetRA, "Name,Order ID,Barcode\nFay,R1,C-2\n", nil, true)
	if other.Counts.Rejected != 1 || !strings.Contains(other.Rejected[0].Reason, "already belongs to another ticket") {
		t.Fatalf("barcode owned by another source: %+v", other.Rejected)
	}
	dup := f.importCSV(t, d.ID, guest.PresetRA, "Name,Order ID,Ticket ID\nGil,R1,T1\nHal,R1,T1\n", nil, true)
	if dup.Counts.Rejected != 1 || dup.Rejected[0].Reason != "same ticket as line 2" {
		t.Fatalf("duplicate ticket id: %+v", dup.Rejected)
	}
}

func TestImportTicketTypesByRefAndGuestListOverlap(t *testing.T) {
	f := setup(t)
	d := f.night(t)
	l := f.list(t, d.ID, guest.ListInput{Name: "Industry", Type: "industry", CollectContact: true})
	f.add(t, d.ID, guest.AddInput{ListID: l.ID, Guests: []guest.GuestInput{{Name: "Mara", Email: "MARA@example.org"}}})

	luma := fixtureCSV(t, "luma.csv")
	res := f.importCSV(t, d.ID, guest.PresetLuma, luma, nil, false)
	if res.Counts.OnGuestList != 1 || res.Counts.PositionsNew != 3 || fmt.Sprint(res.TicketTypes) != "[{Free RSVP true 2 1} {VIP true 1 0}]" {
		t.Fatalf("luma: %s %v", counts(res), res.TicketTypes)
	}
	renamed := strings.ReplaceAll(luma, "Free RSVP", "RSVP (free)")
	again := f.importCSV(t, d.ID, guest.PresetLuma, renamed, nil, true)
	if fmt.Sprint(again.TicketTypes) != "[{Free RSVP false 2 1} {VIP false 1 0}]" || again.Counts.PositionsUnchanged != 3 {
		t.Fatalf("types match by external ref, not name: %v %s", again.TicketTypes, counts(again))
	}
	// Separate platforms keep separate order namespaces.
	if got := counts(f.importCSV(t, d.ID, guest.PresetDICE, "Name,Order ID\nZoe,gst-a1\n", nil, true)); got != "rows=1 orders+1~0 tickets+1~0=0 rejected=0 absent=0 onlist=0" {
		t.Fatalf("other source, same order ref: %s", got)
	}
}

func TestImportErrorsAndTenantIsolation(t *testing.T) {
	a, b := setup(t), setup(t)
	d := a.night(t)
	var me *guest.MappingError
	if _, err := a.svc.ImportAttendees(a.ctx, d.ID, guest.ImportInput{Preset: guest.PresetRA, Table: csvTable(t, "Name,Notes\nA,b\n")}, true, staff); !errors.As(err, &me) {
		t.Fatalf("mapping error: %v", err)
	}
	if _, err := a.svc.ImportAttendees(a.ctx, uuid.Must(uuid.NewV7()), guest.ImportInput{Preset: guest.PresetRA, Table: csvTable(t, fixtureCSV(t, "ra.csv"))}, true, staff); !errors.Is(err, guest.ErrNotFound) {
		t.Fatalf("unknown event: %v", err)
	}
	a.importCSV(t, d.ID, guest.PresetRA, fixtureCSV(t, "ra.csv"), nil, false)
	if _, err := b.svc.ImportAttendees(b.ctx, d.ID, guest.ImportInput{Preset: guest.PresetRA, Table: csvTable(t, fixtureCSV(t, "ra.csv"))}, false, staff); !errors.Is(err, guest.ErrNotFound) {
		t.Fatalf("importing into another tenant's event must fail: %v", err)
	}
	if n := b.count(t, `SELECT count(*) FROM order_positions WHERE tenant_id = $1`, b.org); n != 0 {
		t.Fatalf("tenant B wrote %d tickets", n)
	}
	if _, err := b.svc.ListGuests(b.ctx, d.ID, guest.GuestFilter{}); !errors.Is(err, guest.ErrNotFound) {
		t.Fatalf("tenant B must not see A's tickets: %v", err)
	}
	// The app role sees no other tenant's attendee rows at all (RLS).
	d2 := b.night(t)
	page, err := b.svc.ListGuests(b.ctx, d2.ID, guest.GuestFilter{})
	if err != nil || len(page.Tickets) != 0 || page.Counts.Tickets != 0 {
		t.Fatalf("tenant B's own event: %+v %v", page, err)
	}
}
