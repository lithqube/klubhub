package einvoice

import (
	"encoding/json"
	"os"
	"testing"
)

// sampleDoc is a German domestic B2B invoice that satisfies XRechnung: the
// same shape as the spike (docs: .claude/plans/e-invoicing.plan.md section 8).
// All names, addresses and IDs are fictional.
func sampleDoc() Document {
	std := Category{Code: "S", Percent: "19", HasPercent: true}
	return Document{
		Number: "INV-0042-EUR", TypeCode: TypeInvoice,
		IssueDate: "2026-10-05", DueDate: "2026-10-19", DeliveryDate: "2026-10-03",
		Currency: "EUR", BuyerReference: "04011000-12345-34",
		PaymentTerms: "Zahlbar bis 19.10.2026 ohne Abzug.",
		Seller: Party{
			Name: "Lina Vasquez", TradingName: "DJ Lina V",
			Street: "Köpenicker Str. 70", City: "Berlin", Postal: "10179", Country: "DE",
			VATID: "DE123456789", Email: "lina@example.com", ContactName: "Lina Vasquez", Phone: "+49 30 1234567",
		},
		Buyer: Party{
			Name: "Club Beispiel GmbH", Street: "Warschauer Str. 1", City: "Berlin", Postal: "10243", Country: "DE",
			VATID: "DE987654321", Email: "buchhaltung@club-beispiel.example",
		},
		Payment: &Payment{IBAN: "DE02120300000000202051", BIC: "BYLADEM1001", Reference: "INV-0042-EUR"},
		Lines: []Line{
			{ID: 1, Name: "DJ performance 2026-10-03 (Äöü – Größe)", Quantity: "1", UnitCode: "C62", UnitPrice: "1000.00", Net: "1000.00", Category: std},
			{ID: 2, Name: "Travel (2 x 100 km)", Quantity: "2", UnitCode: "KMT", UnitPrice: "100.00", Net: "200.00", Category: std},
		},
		Taxes:  []TaxLine{{Category: std, Taxable: "1200.00", Tax: "228.00"}},
		Totals: Totals{Lines: "1200.00", TaxExclusive: "1200.00", Tax: "228.00", TaxInclusive: "1428.00", Payable: "1428.00"},
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// decodeJSON parses the sidecar JSON into a generic tree.
func decodeJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	return m
}

// at walks a path of object keys and array indexes ("a", 0, "b") and returns
// the value, or nil when any step is missing.
func at(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[k]
		case int:
			a, ok := v.([]any)
			if !ok || k >= len(a) {
				return nil
			}
			v = a[k]
		}
	}
	return v
}

func hasProblem(ps []Problem, field string) bool {
	for _, p := range ps {
		if p.Field == field {
			return true
		}
	}
	return false
}
