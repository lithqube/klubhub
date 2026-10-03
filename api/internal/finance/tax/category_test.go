package tax

import "testing"

func TestCategoryFor(t *testing.T) {
	cases := []struct {
		name      string
		treatment Treatment
		rateBps   int64
		want      Category
	}{
		{"domestic standard rate", Domestic, 1900, Category{Code: "S"}},
		{"domestic reduced rate", Domestic, 700, Category{Code: "S"}},
		{"domestic zero rate is zero-rated", Domestic, 0, Category{Code: "Z"}},
		{"reverse charge", ReverseCharge, 0, Category{Code: "AE", ExemptionCode: "VATEX-EU-AE"}},
		{"small-business exemption", Exempt, 0, Category{Code: "E"}},
		{"outside the scope of EU VAT", OutsideScope, 0, Category{Code: "O", ExemptionCode: "VATEX-EU-O"}},
		{"US sales tax is outside EU VAT", USSalesTax, 0, Category{Code: "O"}},
		{"no tax", None, 0, Category{Code: "O"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CategoryFor(tc.treatment, tc.rateBps); got != tc.want {
				t.Errorf("CategoryFor(%q, %d) = %+v, want %+v", tc.treatment, tc.rateBps, got, tc.want)
			}
		})
	}
}

// Only the categories that carry a VAT amount may have a rate; the rest must
// be exported with no percentage (EN 16931 BR-AE-*, BR-E-*, BR-O-*).
func TestCategory_CarriesRate(t *testing.T) {
	for code, want := range map[string]bool{"S": true, "Z": true, "AE": false, "E": false, "O": false} {
		if got := (Category{Code: code}).CarriesRate(); got != want {
			t.Errorf("Category{%q}.CarriesRate() = %v, want %v", code, got, want)
		}
	}
}
