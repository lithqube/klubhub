package guest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func fixture(t *testing.T, name string) *Table {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "import", name))
	if err != nil {
		t.Fatal(err)
	}
	tab, err := ParseCSV(raw)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return tab
}

// Each platform fixture maps with its preset: the accepted rows (name,
// email, order/ticket ref, barcode, type, status) and rejected lines.
func TestPresetsMapPlatformExports(t *testing.T) {
	type want struct {
		line                                            int
		name, email, order, ticket, secret, typ, status string
	}
	cases := []struct {
		file, preset string
		mapping      map[string]string
		accepted     []want
		rejected     map[int]string
	}{
		{file: "pretix.csv", preset: PresetPretix, accepted: []want{
			{2, "Mara Weiss", "mara@example.org", "AB12C", "1", "k2x9fvq8mzq7hs4p", "Early bird", TicketValid},
			{3, "José Müller", "buyer@example.org", "AB12C", "2", "p7wd3nn2c8x4ra1z", "Early bird", TicketValid},
			{4, "Tomasz Nowak", "tom@example.org", "XY34Z", "3", "q9ee4bb1v6m2kd8t", "Regular", TicketCancelled},
			{5, `Ines "Nes" Duarte`, "ines@example.org", "QQ55R", "4", "z1aa2bb3cc4dd5ee", "Regular", TicketPending},
		}},
		{file: "ra.csv", preset: PresetRA, accepted: []want{
			{2, "Lena Vogt", "lena@example.org", "90001", "T-1", "RA-0001-AAA", "Tier 1", TicketValid},
			{3, "Sam Oduya", "lena@example.org", "90001", "T-2", "RA-0001-AAB", "Tier 1", TicketValid},
			{4, "Kofi Mensah", "kofi@example.org", "90002", "T-3", "RA-0002-AAA", "Tier 2", TicketRefunded},
		}, rejected: map[int]string{5: "no name or email"}},
		{file: "dice.csv", preset: PresetDICE, accepted: []want{
			{2, "Aiko Tanaka", "aiko@example.org", "D-7001", "TK-1", "", "General Admission", TicketValid},
			{3, "Rafael Ortiz", "aiko@example.org", "D-7001", "TK-2", "", "General Admission", TicketValid},
			{4, "Noor Haddad", "noor@example.org", "D-7002", "TK-3", "", "Late Entry", TicketRefunded},
		}, rejected: map[int]string{5: "is not an email address"}},
		{file: "shotgun.csv", preset: PresetShotgun, accepted: []want{
			{2, "Juno Park", "juno@example.org", "5501", "88001", "SG88001X", "Phase 1", TicketValid},
			{3, "Kim Lee", "kim@example.org", "5502", "88002", "SG88002X", "Phase 2", TicketCancelled},
		}, rejected: map[int]string{4: `unknown status "resold elsewhere"`}},
		{file: "luma.csv", preset: PresetLuma, accepted: []want{
			{2, "Mara Weiss", "mara@example.org", "gst-a1", "", "https://lu.ma/check-in/evt-1?pk=g-a1", "Free RSVP", TicketValid},
			{3, "Tom Novak", "tom@example.org", "gst-b2", "", "https://lu.ma/check-in/evt-1?pk=g-b2", "Free RSVP", TicketPending},
			{4, "Ines Duarte", "ines@example.org", "gst-c3", "", "https://lu.ma/check-in/evt-1?pk=g-c3", "VIP", TicketCancelled},
		}},
		{file: "generic.csv", preset: PresetGeneric,
			mapping: map[string]string{FieldName: "guest", FieldEmail: " MAIL ", FieldOrderRef: "Ref", FieldTicketType: "Kind"},
			accepted: []want{
				{2, "Anna Berg", "anna@example.org", "G-1", "", "", "Door list", TicketValid},
				{5, "Ben Roth", "", "G-2", "", "", "Door list", TicketValid},
			}, rejected: map[int]string{6: "no name or email"}},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			tab := fixture(t, c.file)
			m, err := ResolveMapping(c.preset, tab.Headers, c.mapping)
			if err != nil {
				t.Fatalf("mapping: %v", err)
			}
			var got []want
			rejected := map[int]string{}
			for _, row := range tab.Rows {
				r, reason := m.MapRow(row)
				if reason != "" {
					rejected[r.Line] = reason
					continue
				}
				got = append(got, want{r.Line, r.Name, r.Email, r.OrderRef, r.TicketRef, r.Secret, r.TicketType, r.Status})
			}
			if fmt.Sprint(got) != fmt.Sprint(c.accepted) {
				t.Errorf("accepted:\n got %v\nwant %v", got, c.accepted)
			}
			if len(rejected) != len(c.rejected) {
				t.Errorf("rejected: %v, want %v", rejected, c.rejected)
			}
			for line, frag := range c.rejected {
				if !strings.Contains(rejected[line], frag) {
					t.Errorf("line %d: %q, want it to contain %q", line, rejected[line], frag)
				}
			}
		})
	}
}

func TestPretixKeepsBuyerAndAttendeeApart(t *testing.T) {
	tab := fixture(t, "pretix.csv")
	m, _ := ResolveMapping(PresetPretix, tab.Headers, nil)
	if m.Header[FieldEmail] != "Attendee email" || m.Header[FieldBuyerEmail] != "Email" || m.Header[FieldTicketType] != "Product" {
		t.Fatalf("pretix mapping: %v", m.Header)
	}
	r, _ := m.MapRow(tab.Rows[0])
	if r.BuyerEmail != "buyer@example.org" || r.Email != "mara@example.org" {
		t.Fatalf("buyer vs attendee: %+v", r)
	}
}

func TestParseCSVTolerance(t *testing.T) {
	cases := []struct {
		name    string
		raw     []byte
		headers string
		rows    int
		enc     string
		err     string
	}{
		{name: "BOM and CRLF", raw: []byte("\xef\xbb\xbfName,Email\r\nA,a@x.io\r\n"), headers: "[Name Email]", rows: 1, enc: "utf-8"},
		{name: "semicolons", raw: []byte("Name;Email;Order\nA;a@x.io;1\n"), headers: "[Name Email Order]", rows: 1, enc: "utf-8"},
		{name: "excel sep line", raw: []byte("sep=|\nName|Order\nA,B|1\n"), headers: "[Name Order]", rows: 1, enc: "utf-8"},
		{name: "tabs", raw: []byte("Name\tOrder\nA\t1\n"), headers: "[Name Order]", rows: 1, enc: "utf-8"},
		{name: "quoted commas beat semicolons", raw: []byte("\"a;b\",c,d\n1,2,3\n"), headers: "[a;b c d]", rows: 1, enc: "utf-8"},
		{name: "blank lines and ragged rows", raw: []byte("\n\nName,Order\n\nA\n,,\nB,2,extra\n"), headers: "[Name Order]", rows: 2, enc: "utf-8"},
		{name: "windows-1252", raw: mustCP1252("Name,Order\nJosé Müller,1\n"), headers: "[Name Order]", rows: 1, enc: "windows-1252"},
		{name: "utf-16", raw: []byte{0xFF, 0xFE, 'N', 0}, err: "UTF-16"},
		{name: "empty", raw: []byte("\n \n"), err: "empty"},
		{name: "header only", raw: []byte("Name,Order\n"), err: "no rows"},
		{name: "binary", raw: []byte("Name\x00,Order\n1,2\n"), err: "not a CSV"},
		{name: "too large", raw: make([]byte, MaxImportBytes+1), err: "larger than"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab, err := ParseCSV(c.raw)
			if c.err != "" {
				var inv *InvalidError
				if !errors.As(err, &inv) || !strings.Contains(inv.Problem, c.err) {
					t.Fatalf("want error %q, got %v", c.err, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(tab.Headers) != c.headers || len(tab.Rows) != c.rows || tab.Encoding != c.enc {
				t.Fatalf("got %v, %d rows, %s", tab.Headers, len(tab.Rows), tab.Encoding)
			}
		})
	}
	tab, _ := ParseCSV(mustCP1252("Name,Order\nJosé Müller,1\n"))
	if tab.Rows[0].Cells[0] != "José Müller" {
		t.Fatalf("windows-1252 must be decoded: %q", tab.Rows[0].Cells[0])
	}
}

func mustCP1252(s string) []byte {
	b, err := charmap.Windows1252.NewEncoder().Bytes([]byte(s))
	if err != nil {
		panic(err)
	}
	return b
}

func TestParseCSVRowLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("Name,Order\n")
	for i := 0; i <= MaxImportRows; i++ {
		fmt.Fprintf(&b, "N%d,%d\n", i, i)
	}
	var inv *InvalidError
	if _, err := ParseCSV([]byte(b.String())); !errors.As(err, &inv) || !strings.Contains(inv.Problem, "at most 20000 rows") {
		t.Fatalf("row limit: %v", err)
	}
}

func TestResolveMappingErrors(t *testing.T) {
	var me *MappingError
	var inv *InvalidError
	if _, err := ResolveMapping(PresetRA, []string{"Name", "Email"}, nil); !errors.As(err, &me) || fmt.Sprint(me.Missing) != "[order number, ticket id or barcode]" || len(me.Headers) != 2 {
		t.Fatalf("no key column: %v", err)
	}
	if _, err := ResolveMapping(PresetDICE, []string{"Order ID", "Notes"}, nil); !errors.As(err, &me) || fmt.Sprint(me.Missing) != "[name or email]" {
		t.Fatalf("no name column: %v", err)
	}
	if _, err := ResolveMapping(PresetGeneric, []string{"A", "B"}, map[string]string{FieldName: "A", FieldOrderRef: "C"}); !errors.As(err, &me) || !strings.Contains(me.Problem, `"C"`) {
		t.Fatalf("unknown column: %v", err)
	}
	if _, err := ResolveMapping(PresetGeneric, []string{"A", "B"}, map[string]string{FieldName: "A", FieldOrderRef: "a"}); !errors.As(err, &inv) || inv.Field != "mapping" {
		t.Fatalf("a column mapped twice: %v", err)
	}
	if _, err := ResolveMapping(PresetGeneric, []string{"A"}, map[string]string{"shoe_size": "A"}); !errors.As(err, &inv) {
		t.Fatalf("unknown field: %v", err)
	}
	if _, err := ResolveMapping("eventbrite", []string{"A"}, nil); !errors.As(err, &inv) || inv.Field != "preset" {
		t.Fatalf("unknown preset: %v", err)
	}
	// Headers match whatever their case, spacing and punctuation.
	m, err := ResolveMapping(PresetShotgun, []string{" ORDER-ID ", "first_name", "LAST NAME"}, nil)
	if err != nil || m.Header[FieldOrderRef] != " ORDER-ID " || m.Header[FieldFirstName] != "first_name" || m.Header[FieldLastName] != "LAST NAME" {
		t.Fatalf("tolerant headers: %v %v", m.Header, err)
	}
}

func TestTicketStatus(t *testing.T) {
	cases := map[string]string{
		"": TicketValid, "Paid": TicketValid, "p": TicketValid, " Checked in ": TicketValid, "approved": TicketValid,
		"pending_approval": TicketPending, "n": TicketPending, "Waitlist": TicketPending,
		"Refunded": TicketRefunded, "Partially refunded": TicketRefunded, "c": TicketCancelled, "CANCELED": TicketCancelled,
		"Cancelled by organiser": TicketCancelled, "e": TicketCancelled, "transferred": TicketCancelled,
	}
	for raw, want := range cases {
		if got, ok := TicketStatus(raw); !ok || got != want {
			t.Errorf("%q → %q %v, want %q", raw, got, ok, want)
		}
	}
	if _, ok := TicketStatus("resold elsewhere"); ok {
		t.Error("unknown statuses must be refused, not guessed")
	}
}

func TestMapRowLimits(t *testing.T) {
	m, _ := ResolveMapping(PresetGeneric, []string{"n", "o", "t", "s"}, map[string]string{FieldName: "n", FieldOrderRef: "o", FieldTicketRef: "t", FieldSecret: "s"})
	cases := map[string][]string{
		"name longer":             {strings.Repeat("x", 121), "1", "", ""},
		"order or ticket id":      {"A", strings.Repeat("1", 121), "", ""},
		"must not start with #":   {"A", "1", "#2", ""},
		"barcode longer":          {"A", "1", "", strings.Repeat("s", 513)},
		"no order number, ticket": {"A", "", "", ""},
	}
	for want, cells := range cases {
		if _, reason := m.MapRow(TableRow{Line: 2, Cells: cells}); !strings.Contains(reason, want) {
			t.Errorf("%v: %q, want %q", cells[:1], reason, want)
		}
	}
	r, reason := m.MapRow(TableRow{Line: 2, Cells: []string{"  Mara   Weiss ", " 7 ", "", "  AbC-1  "}})
	if reason != "" || r.Name != "Mara Weiss" || r.OrderRef != "7" || r.Secret != "AbC-1" || r.TicketType != DefaultTicketType {
		t.Fatalf("normalised row: %+v %q", r, reason)
	}
}

func TestMasking(t *testing.T) {
	for in, want := range map[string]string{
		"John Doe": "Jo… D…", "José Müller-Lüdenscheidt": "Jo… M…", "Al": "Al", "Ines D": "In… D", "": "", "Élodie": "Él…",
	} {
		if got := MaskName(in); got != want {
			t.Errorf("MaskName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"mara@label.example": "m…@l…", "": "", "broken": "b…"} {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTableFromObjects(t *testing.T) {
	tab, err := TableFromObjects([]map[string]string{{"Name": "A", "Order": "1"}, {"Order": "2", "Email": "b@x.io"}})
	if err != nil || fmt.Sprint(tab.Headers) != "[Email Name Order]" || tab.Rows[1].Line != 2 || tab.Rows[1].Cells[0] != "b@x.io" {
		t.Fatalf("%+v %v", tab, err)
	}
	if _, err := TableFromObjects(nil); err == nil {
		t.Fatal("no rows must fail")
	}
}
