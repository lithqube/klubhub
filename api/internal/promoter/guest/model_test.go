package guest

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func TestListInputValidation(t *testing.T) {
	cutoff := time.Date(2026, 10, 4, 1, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		in       ListInput
		template bool
		field    string // "" = valid
	}{
		{"minimal free list", ListInput{Name: " Artist guests ", Type: "artist"}, false, ""},
		{"missing name", ListInput{Type: "artist"}, false, "name"},
		{"unknown type", ListInput{Name: "X", Type: "friends"}, false, "type"},
		{"bad price mode", ListInput{Name: "X", Type: "comp", EntryTerms: EntryTerms{PriceMode: "paid"}}, false, "entry_terms.price_mode"},
		{"reduced without price", ListInput{Name: "X", Type: "reduced", EntryTerms: EntryTerms{PriceMode: PriceReduced}}, false, "entry_terms.reduced_price_text"},
		{"reduced with price", ListInput{Name: "X", Type: "reduced", EntryTerms: EntryTerms{PriceMode: PriceReduced, ReducedPriceText: "€10"}}, false, ""},
		{"event list with absolute cutoff", ListInput{Name: "X", Type: "vip", EntryTerms: EntryTerms{CutoffAt: &cutoff}}, false, ""},
		{"event list with local cutoff", ListInput{Name: "X", Type: "vip", EntryTerms: EntryTerms{CutoffLocal: ptr("01:00")}}, false, "entry_terms.cutoff_local"},
		{"template with local cutoff", ListInput{Name: "X", Type: "vip", EntryTerms: EntryTerms{CutoffLocal: ptr("01:00")}}, true, ""},
		{"template with absolute cutoff", ListInput{Name: "X", Type: "vip", EntryTerms: EntryTerms{CutoffAt: &cutoff}}, true, "entry_terms.cutoff_at"},
		{"template with bad local cutoff", ListInput{Name: "X", Type: "vip", EntryTerms: EntryTerms{CutoffLocal: ptr("25:00")}}, true, "entry_terms.cutoff_local"},
		{"too many perks", ListInput{Name: "X", Type: "vip", EntryTerms: EntryTerms{Perks: strings.Split("a,b,c,d,e,f,g,h,i", ",")}}, false, "entry_terms.perks"},
		{"long perk", ListInput{Name: "X", Type: "vip", EntryTerms: EntryTerms{Perks: []string{strings.Repeat("x", 25)}}}, false, "entry_terms.perks"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.validate(c.template)
			assertField(t, err, c.field)
		})
	}
}

func TestEntryTermsNormalise(t *testing.T) {
	in := ListInput{Name: "  VIP  ", Type: "vip", EntryTerms: EntryTerms{ReducedPriceText: "€5", Perks: []string{" Drink Token", "drink token", "", "Coat check"}}}
	if err := in.validate(false); err != nil {
		t.Fatal(err)
	}
	if in.Name != "VIP" || in.EntryTerms.PriceMode != PriceFree || in.EntryTerms.ReducedPriceText != "" {
		t.Fatalf("defaults: %+v", in)
	}
	if strings.Join(in.EntryTerms.Perks, "|") != "drink token|coat check" {
		t.Fatalf("perks must be trimmed, lowercased and deduplicated: %q", in.EntryTerms.Perks)
	}
}

func TestAllocationInputValidation(t *testing.T) {
	cases := []struct {
		name  string
		in    AllocationInput
		field string
	}{
		{"valid", AllocationInput{Label: "Ben Klock", Quota: 10, PlusNMax: 1}, ""},
		{"no label", AllocationInput{Quota: 10}, "label"},
		{"zero quota", AllocationInput{Label: "A", Quota: 0}, "quota"},
		{"huge quota", AllocationInput{Label: "A", Quota: 1001}, "quota"},
		{"negative plus", AllocationInput{Label: "A", Quota: 1, PlusNMax: -1}, "plus_n_max"},
		{"plus too high", AllocationInput{Label: "A", Quota: 1, PlusNMax: 11}, "plus_n_max"},
		{"long contact", AllocationInput{Label: "A", Quota: 1, SubmitterContact: ptr(strings.Repeat("x", 201))}, "submitter_contact"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assertField(t, c.in.validate(), c.field) })
	}
}

func TestGuestInputValidation(t *testing.T) {
	cases := []struct {
		name    string
		in      GuestInput
		contact bool
		field   string
	}{
		{"name only", GuestInput{Name: "  Mara   Weiss "}, false, ""},
		{"empty name", GuestInput{Name: "   "}, false, "g.name"},
		{"plus too high", GuestInput{Name: "A", PlusN: 11}, false, "g.plus_n"},
		{"unknown status", GuestInput{Name: "A", Status: "checked_in"}, false, "g.status"},
		{"email on a name-only list", GuestInput{Name: "A", Email: "a@b.example"}, false, "g.email"},
		{"phone on a name-only list", GuestInput{Name: "A", Phone: "+49 30 123456"}, false, "g.phone"},
		{"note on a name-only list is fine", GuestInput{Name: "A", Note: "friend of the DJ"}, false, ""},
		{"email on a contact list", GuestInput{Name: "A", Email: "a@b.example"}, true, ""},
		{"bad email", GuestInput{Name: "A", Email: "not-an-email"}, true, "g.email"},
		{"display-name email", GuestInput{Name: "A", Email: "A <a@b.example>"}, true, "g.email"},
		{"email without dot", GuestInput{Name: "A", Email: "a@localhost"}, true, "g.email"},
		{"bad phone", GuestInput{Name: "A", Phone: "call me"}, true, "g.phone"},
		{"long note", GuestInput{Name: "A", Note: strings.Repeat("x", 501)}, false, "g.note"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assertField(t, c.in.validate("g.", c.contact), c.field) })
	}
	g := GuestInput{Name: "  Mara   Weiss "}
	_ = g.validate("", false)
	if g.Name != "Mara Weiss" {
		t.Fatalf("whitespace must collapse: %q", g.Name)
	}
}

func TestCutoffFor(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	start := time.Date(2026, 10, 3, 23, 0, 0, 0, berlin)
	cases := []struct {
		hhmm string
		want time.Time
	}{
		{"01:00", time.Date(2026, 10, 4, 1, 0, 0, 0, berlin)},   // after midnight: next morning
		{"23:30", time.Date(2026, 10, 3, 23, 30, 0, 0, berlin)}, // same evening
		{"22:00", time.Date(2026, 10, 3, 22, 0, 0, 0, berlin)},  // before the start still counts
		{"11:00", time.Date(2026, 10, 3, 11, 0, 0, 0, berlin)},  // exactly 12 h before
		{"10:59", time.Date(2026, 10, 4, 10, 59, 0, 0, berlin)}, // earlier wraps forward
	}
	for _, c := range cases {
		got, err := CutoffFor(start, "Europe/Berlin", c.hhmm)
		if err != nil || !got.Equal(c.want) {
			t.Errorf("%s: got %v (%v), want %v", c.hhmm, got, err, c.want)
		}
	}
	// A night across the DST change keeps the wall-clock time.
	dst := time.Date(2026, 10, 24, 23, 0, 0, 0, berlin)
	got, _ := CutoffFor(dst, "Europe/Berlin", "03:00")
	if got.In(berlin).Hour() != 3 || got.In(berlin).Day() != 25 {
		t.Fatalf("DST night: %v", got.In(berlin))
	}
	if _, err := CutoffFor(start, "Mars/Olympus", "01:00"); err == nil {
		t.Fatal("unknown timezone must fail")
	}
}

func TestQuotaHoldingStatuses(t *testing.T) {
	for st, want := range map[string]bool{StatusGoing: true, StatusPending: true, StatusInvited: true, StatusWaitlist: false, StatusDeclined: false} {
		if consumes(st) != want {
			t.Errorf("%s holds quota = %v, want %v", st, !want, want)
		}
	}
	a := allocInfo{Quota: 10, Used: 8, Label: "Ben Klock"}
	if err := a.checkQuota(2); err != nil {
		t.Fatalf("exactly full is fine: %v", err)
	}
	var q *QuotaError
	if err := a.checkQuota(3); !errors.As(err, &q) || q.Used != 8 || q.Requested != 3 || q.Label != "Ben Klock" {
		t.Fatalf("over quota: %v", err)
	}
}

func TestCountsAdd(t *testing.T) {
	var c Counts
	c.add(StatusGoing, 3, 5)
	c.add(StatusPending, 2, 2)
	c.add(StatusDeclined, 1, 1)
	c.add(StatusWaitlist, 1, 2)
	c.add(StatusInvited, 4, 4)
	if c.All != 11 || c.Going != 3 || c.GoingHeads != 5 || c.Pending != 2 || c.Declined != 1 || c.Waitlist != 1 || c.Invited != 4 {
		t.Fatalf("%+v", c)
	}
}

func TestSafeCell(t *testing.T) {
	cases := map[string]string{
		"Mara":                     "Mara",
		"=HYPERLINK(\"x\")":        "'=HYPERLINK(\"x\")",
		"+49 30 123":               "'+49 30 123",
		"-2":                       "'-2",
		"@SUM(A1)":                 "'@SUM(A1)",
		"\tTab":                    "'\tTab",
		"\rCR":                     "'\rCR",
		"":                         "",
		"Anne-Marie":               "Anne-Marie",
		"a@b.example":              "a@b.example",
		"O'Brien, \"Sam\"\nline 2": "O'Brien, \"Sam\"\nline 2",
	}
	for in, want := range cases {
		if got := SafeCell(in); got != want {
			t.Errorf("SafeCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteCSVIsInjectionSafeAndParses(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSV(&buf, []ExportRow{
		{Name: "=cmd|' /C calc'!A0", PlusN: 2, Status: "going", List: "Artist", Allocation: "Ben Klock", Note: "comma, \"quote\""},
		{Name: "José Müller", Status: "pending", List: "Comp", Phone: "+49 170 1234567"},
	})
	if err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || strings.Join(recs[0], ",") != strings.Join(ExportHeader, ",") {
		t.Fatalf("header/rows: %q", recs)
	}
	if recs[1][0] != "'=cmd|' /C calc'!A0" || recs[1][1] != "2" || recs[1][7] != "comma, \"quote\"" {
		t.Fatalf("row 1: %q", recs[1])
	}
	if recs[2][0] != "José Müller" || recs[2][6] != "'+49 170 1234567" {
		t.Fatalf("row 2: %q", recs[2])
	}
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	if field == "" {
		if err != nil {
			t.Fatalf("want valid, got %v", err)
		}
		return
	}
	var inv *InvalidError
	if !errors.As(err, &inv) || inv.Field != field {
		t.Fatalf("want invalid %q, got %v", field, err)
	}
}
