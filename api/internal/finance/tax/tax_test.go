package tax

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestIsEU(t *testing.T) {
	for _, c := range []string{"DE", "de", " fr ", "GR", "SE", "IE"} {
		if !IsEU(c) {
			t.Errorf("IsEU(%q) = false", c)
		}
	}
	for _, c := range []string{"", "GB", "CH", "NO", "US", "EL", "XX"} {
		if IsEU(c) {
			t.Errorf("IsEU(%q) = true", c)
		}
	}
	if len(euCountries) != 27 {
		t.Fatalf("EU set has %d members, want 27", len(euCountries))
	}
}

func TestTreatmentIsValid(t *testing.T) {
	for _, tr := range []Treatment{Domestic, ReverseCharge, Exempt, OutsideScope, USSalesTax, None} {
		if !tr.IsValid() {
			t.Errorf("%q should be valid", tr)
		}
	}
	if Treatment("zero_rated").IsValid() || Treatment("").IsValid() {
		t.Error("unknown treatments must be invalid")
	}
}

func TestSuggest(t *testing.T) {
	de := Supplier{Country: "DE", VATID: "DE123456789", DefaultVATRateBps: 1900}
	cases := []struct {
		name       string
		supplier   Supplier
		customer   Customer
		want       Treatment
		wantRate   int64
		wantNote   bool   // tax_note non-empty
		reasonPart string // substring expected in reason
	}{
		{"EU exempt small business, any customer", Supplier{Country: "DE", VATExemptSmallBusiness: true, DefaultVATRateBps: 1900},
			Customer{Country: "FR", VATID: "FR1", IsBusiness: true}, Exempt, 0, true, "small-business"},
		{"EU exempt even for domestic customer", Supplier{Country: "AT", VATExemptSmallBusiness: true, DefaultVATRateBps: 2000},
			Customer{Country: "AT"}, Exempt, 0, true, "small-business"},
		{"EU same country", de, Customer{Country: "de"}, Domestic, 1900, false, "supplier's country"},
		{"EU other country business with VAT ID", de, Customer{Country: "FR", VATID: "FR12345678901", IsBusiness: true}, ReverseCharge, 0, true, "VAT ID"},
		{"EU other country, no VAT ID", de, Customer{Country: "FR", IsBusiness: true}, Domestic, 1900, false, "no customer VAT ID — confirm"},
		{"EU other country, VAT ID but not business", de, Customer{Country: "NL", VATID: "NL1"}, Domestic, 1900, false, "not marked as a business"},
		{"EU supplier, customer outside EU", de, Customer{Country: "GB", VATID: "GB1", IsBusiness: true}, OutsideScope, 0, true, "outside the EU"},
		{"EU supplier, customer country unknown", de, Customer{}, Domestic, 1900, false, "confirm"},
		{"US supplier", Supplier{Country: "US"}, Customer{Country: "US"}, None, 0, false, "us_sales_tax"},
		{"US supplier, EU customer", Supplier{Country: "us"}, Customer{Country: "DE", VATID: "DE1", IsBusiness: true}, None, 0, false, "US supplier"},
		{"other supplier country", Supplier{Country: "CH", DefaultVATRateBps: 810}, Customer{Country: "DE"}, None, 0, false, "outside the EU and US"},
		{"unknown supplier country", Supplier{}, Customer{Country: "DE"}, None, 0, false, "supplier country unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Suggest(tc.supplier, tc.customer)
			if got.VATTreatment != tc.want || got.TaxRateBps != tc.wantRate {
				t.Fatalf("Suggest = %+v, want %s @ %d", got, tc.want, tc.wantRate)
			}
			if (got.TaxNote != "") != tc.wantNote {
				t.Errorf("tax_note = %q, want non-empty=%v", got.TaxNote, tc.wantNote)
			}
			if !strings.Contains(got.Reason, tc.reasonPart) {
				t.Errorf("reason %q does not contain %q", got.Reason, tc.reasonPart)
			}
		})
	}
}

func TestDefaultRate(t *testing.T) {
	s := Supplier{DefaultVATRateBps: 2100}
	if DefaultRate(Domestic, s) != 2100 {
		t.Error("domestic should use the supplier default rate")
	}
	for _, tr := range []Treatment{ReverseCharge, Exempt, OutsideScope, USSalesTax, None} {
		if DefaultRate(tr, s) != 0 {
			t.Errorf("DefaultRate(%s) != 0", tr)
		}
	}
}

func TestNotesRegistry(t *testing.T) {
	r := NewNoteRegistry()
	if got := r.Note("DE", ReverseCharge); !strings.Contains(got, "Art. 196") {
		t.Errorf("reverse charge default note = %q", got)
	}
	if got := r.Note("FR", Exempt); got != "VAT exempt: small business scheme." {
		t.Errorf("exempt default note = %q", got)
	}
	if got := r.Note("DE", OutsideScope); !strings.Contains(got, "Outside the scope") {
		t.Errorf("outside scope note = %q", got)
	}
	if r.Note("DE", Domestic) != "" || r.Note("US", None) != "" {
		t.Error("domestic/none carry no default note")
	}
	r.Register("de", Exempt, "Gemäß § 19 UStG wird keine Umsatzsteuer berechnet.")
	if got := r.Note("DE", Exempt); !strings.Contains(got, "§ 19 UStG") {
		t.Errorf("country-specific note not used: %q", got)
	}
	if got := r.Note("AT", Exempt); got != "VAT exempt: small business scheme." {
		t.Errorf("other countries keep the default: %q", got)
	}
	// The global registry is untouched by a local registration.
	if strings.Contains(Notes.Note("DE", Exempt), "UStG") {
		t.Error("local registry leaked into Notes")
	}
}

func TestApplyBps_RoundsHalfUp(t *testing.T) {
	cases := []struct{ amount, bps, want int64 }{
		{0, 1900, 0},
		{100, 0, 0},
		{10000, 1900, 1900},
		{1, 5000, 1},      // 0.5 → 1
		{1, 4999, 0},      // 0.4999 → 0
		{3, 5000, 2},      // 1.5 → 2
		{2, 2500, 1},      // 0.5 → 1
		{105, 1000, 11},   // 10.5 → 11
		{104, 1000, 10},   // 10.4 → 10
		{-105, 1000, -11}, // half away from zero
		{12345, 700, 864}, // 864.15 → 864
		{12350, 700, 865}, // 864.5 → 865
		{math.MaxInt64 / 2, 10000, math.MaxInt64 / 2}, // no intermediate overflow
	}
	for _, tc := range cases {
		if got := ApplyBps(tc.amount, tc.bps); got != tc.want {
			t.Errorf("ApplyBps(%d, %d) = %d, want %d", tc.amount, tc.bps, got, tc.want)
		}
	}
}

func TestComputeTotals(t *testing.T) {
	cases := []struct {
		name        string
		lines       []Line
		rate, wh    int64
		wantSub     int64
		wantTax     int64
		wantWH      int64
		wantNet     int64
		wantRows    []BreakdownRow
		wantLineBps []int64
	}{
		{name: "no lines", lines: nil, rate: 1900, wantRows: []BreakdownRow{}, wantLineBps: []int64{}},
		{name: "single line 19%", lines: []Line{{NetMinor: 25000}}, rate: 1900,
			wantSub: 25000, wantTax: 4750, wantNet: 29750,
			wantRows: []BreakdownRow{{1900, 25000, 4750}}, wantLineBps: []int64{1900}},
		{name: "invoice rate overrides line rates", lines: []Line{{NetMinor: 100, TaxBps: 700}, {NetMinor: 200, TaxBps: 0}}, rate: 2000,
			wantSub: 300, wantTax: 60, wantNet: 360,
			wantRows: []BreakdownRow{{2000, 300, 60}}, wantLineBps: []int64{2000, 2000}},
		{name: "rounding per rate group, not per line", lines: []Line{{NetMinor: 5}, {NetMinor: 5}, {NetMinor: 5}}, rate: 1000,
			// per line: 0.5 → 1 each = 3; per group: 15 × 10% = 1.5 → 2
			wantSub: 15, wantTax: 2, wantNet: 17,
			wantRows: []BreakdownRow{{1000, 15, 2}}, wantLineBps: []int64{1000, 1000, 1000}},
		{name: "per-line rates grouped and sorted", lines: []Line{{NetMinor: 10000, TaxBps: 1900}, {NetMinor: 5000, TaxBps: 700}, {NetMinor: 333, TaxBps: 1900}, {NetMinor: 100, TaxBps: 0}},
			rate: PerLineRates,
			// 19%: 10333 → 1963.27 → 1963; 7%: 5000 → 350; 0%: 0
			wantSub: 15433, wantTax: 2313, wantNet: 17746,
			wantRows:    []BreakdownRow{{0, 100, 0}, {700, 5000, 350}, {1900, 10333, 1963}},
			wantLineBps: []int64{1900, 700, 1900, 0}},
		{name: "zero rate keeps a breakdown row", lines: []Line{{NetMinor: 50000}}, rate: 0,
			wantSub: 50000, wantNet: 50000,
			wantRows: []BreakdownRow{{0, 50000, 0}}, wantLineBps: []int64{0}},
		{name: "withholding on subtotal, not total", lines: []Line{{NetMinor: 100000}}, rate: 2100, wh: 1500,
			// total 121000; withholding 15% of 100000 = 15000
			wantSub: 100000, wantTax: 21000, wantWH: 15000, wantNet: 106000,
			wantRows: []BreakdownRow{{2100, 100000, 21000}}, wantLineBps: []int64{2100}},
		{name: "withholding rounds half-up", lines: []Line{{NetMinor: 333}}, rate: 0, wh: 1500,
			// 49.95 → 50
			wantSub: 333, wantWH: 50, wantNet: 283,
			wantRows: []BreakdownRow{{0, 333, 0}}, wantLineBps: []int64{0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeTotals(tc.lines, tc.rate, tc.wh)
			if got.SubtotalMinor != tc.wantSub || got.TaxMinor != tc.wantTax || got.TotalMinor != tc.wantSub+tc.wantTax ||
				got.WithholdingMinor != tc.wantWH || got.NetPayableMinor != tc.wantNet {
				t.Fatalf("totals = %+v", got)
			}
			if !reflect.DeepEqual(got.Breakdown, tc.wantRows) {
				t.Errorf("breakdown = %+v, want %+v", got.Breakdown, tc.wantRows)
			}
			if !reflect.DeepEqual(got.LineTaxBps, tc.wantLineBps) {
				t.Errorf("line bps = %v, want %v", got.LineTaxBps, tc.wantLineBps)
			}
			var sum int64
			for _, r := range got.Breakdown {
				sum += r.TaxMinor
			}
			if sum != got.TaxMinor {
				t.Errorf("breakdown tax %d != tax %d", sum, got.TaxMinor)
			}
		})
	}
}

// readyInput is a domestic German invoice that passes every rule.
func readyInput() IssueInput {
	return IssueInput{
		Supplier: Supplier{LegalName: "DJ Lina", AddressLine1: "1 Str", City: "Berlin", PostalCode: "10115",
			Country: "DE", VATID: "DE123456789", DefaultVATRateBps: 1900},
		Customer:      Customer{LegalName: "Club GmbH", AddressLine1: "2 Str", City: "Hamburg", Country: "DE"},
		Treatment:     Domestic,
		TaxRateBps:    1900,
		SupplyDateSet: true,
		TotalMinor:    29750,
		Currency:      "EUR",
		GigCurrency:   "EUR",
	}
}

func fields(ps []Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Field
	}
	return out
}

func TestValidateForIssue(t *testing.T) {
	if ps := ValidateForIssue(readyInput()); ps == nil || len(ps) != 0 {
		t.Fatalf("ready input: problems = %+v (want empty, non-nil)", ps)
	}
	cases := []struct {
		name   string
		mutate func(*IssueInput)
		want   []string
	}{
		{"supplier legal name", func(in *IssueInput) { in.Supplier.LegalName = " " }, []string{"supplier.legal_name"}},
		{"supplier address", func(in *IssueInput) { in.Supplier.AddressLine1 = "" }, []string{"supplier.address_line1"}},
		{"supplier city", func(in *IssueInput) { in.Supplier.City = "" }, []string{"supplier.city"}},
		{"supplier postal code", func(in *IssueInput) { in.Supplier.PostalCode = "" }, []string{"supplier.postal_code"}},
		{"supplier country", func(in *IssueInput) { in.Supplier.Country = "" }, []string{"supplier.country"}},
		{"customer name or company", func(in *IssueInput) { in.Customer.LegalName = "" }, []string{"customer.legal_name"}},
		{"company alone is enough", func(in *IssueInput) { in.Customer.LegalName = ""; in.Customer.Company = "Club" }, nil},
		{"customer address", func(in *IssueInput) { in.Customer.AddressLine1 = "" }, []string{"customer.address_line1"}},
		{"customer city", func(in *IssueInput) { in.Customer.City = "" }, []string{"customer.city"}},
		{"customer country", func(in *IssueInput) { in.Customer.Country = "" }, []string{"customer.country"}},
		{"supply date", func(in *IssueInput) { in.SupplyDateSet = false }, []string{"supply_date"}},
		{"total zero", func(in *IssueInput) { in.TotalMinor = 0 }, []string{"total_minor"}},
		{"gig currency mismatch", func(in *IssueInput) { in.GigCurrency = "USD" }, []string{"currency"}},
		{"gig currency unknown skips check", func(in *IssueInput) { in.GigCurrency = "" }, nil},
		{"domestic needs a rate", func(in *IssueInput) { in.TaxRateBps = 0 }, []string{"tax_rate_bps"}},
		{"domestic EU needs supplier VAT ID", func(in *IssueInput) { in.Supplier.VATID = "" }, []string{"supplier.vat_id"}},
		{"domestic non-EU supplier needs no VAT ID", func(in *IssueInput) { in.Supplier.VATID = ""; in.Supplier.Country = "GB"; in.Customer.Country = "GB" }, nil},
		{"reverse charge ok", func(in *IssueInput) {
			in.Treatment, in.TaxRateBps = ReverseCharge, 0
			in.Customer.Country, in.Customer.VATID = "FR", "FR12345678901"
		}, nil},
		{"reverse charge needs both VAT IDs", func(in *IssueInput) {
			in.Treatment, in.TaxRateBps = ReverseCharge, 0
			in.Customer.Country = "FR"
			in.Supplier.VATID = ""
		}, []string{"supplier.vat_id", "customer.vat_id"}},
		{"reverse charge needs EU parties", func(in *IssueInput) {
			in.Treatment, in.TaxRateBps = ReverseCharge, 0
			in.Supplier.Country = "US"
			in.Customer.Country, in.Customer.VATID = "GB", "GB1"
		}, []string{"supplier.country", "customer.country"}},
		{"reverse charge needs different countries", func(in *IssueInput) {
			in.Treatment, in.TaxRateBps = ReverseCharge, 0
			in.Customer.VATID = "DE999999999"
		}, []string{"customer.country"}},
		{"reverse charge is 0%", func(in *IssueInput) {
			in.Treatment = ReverseCharge
			in.Customer.Country, in.Customer.VATID = "FR", "FR1"
		}, []string{"tax_rate_bps"}},
		{"exempt needs rate 0 and a note", func(in *IssueInput) { in.Treatment = Exempt }, []string{"tax_rate_bps", "tax_note"}},
		{"exempt ok", func(in *IssueInput) { in.Treatment, in.TaxRateBps, in.TaxNote = Exempt, 0, "VAT exempt" }, nil},
		{"outside scope needs a note", func(in *IssueInput) { in.Treatment, in.TaxRateBps = OutsideScope, 0 }, []string{"tax_note"}},
		{"none needs rate 0 but no note", func(in *IssueInput) { in.Treatment = None }, []string{"tax_rate_bps"}},
		{"none ok without note", func(in *IssueInput) { in.Treatment, in.TaxRateBps = None, 0 }, nil},
		{"us sales tax accepts any rate", func(in *IssueInput) { in.Treatment, in.TaxRateBps = USSalesTax, 887 }, nil},
		{"unknown treatment", func(in *IssueInput) { in.Treatment = "zero" }, []string{"vat_treatment"}},
		{"withholding above 50%", func(in *IssueInput) { in.WithholdingRateBps = 5001 }, []string{"withholding_rate_bps"}},
		{"withholding negative", func(in *IssueInput) { in.WithholdingRateBps = -1 }, []string{"withholding_rate_bps"}},
		{"withholding at 50% ok", func(in *IssueInput) { in.WithholdingRateBps = 5000 }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := readyInput()
			tc.mutate(&in)
			got := fields(ValidateForIssue(in))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("problems = %v, want %v", got, tc.want)
			}
		})
	}
}
