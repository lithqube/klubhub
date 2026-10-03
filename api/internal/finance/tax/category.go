package tax

// Category is the EN 16931 VAT category of an invoice's lines and breakdown
// (BT-151 / BT-118) with the optional VATEX exemption reason code (BT-121 /
// BT-118-1 family). Codes are UNCL5305 subset values.
type Category struct {
	// Code is the UNCL5305 category: S standard, Z zero-rated, E exempt,
	// AE reverse charge, O outside the scope of VAT.
	Code string
	// ExemptionCode is the CEF VATEX code that EN 16931 accepts in place of
	// free text; "" when no standard code applies (the tax note is then the
	// exemption reason).
	ExemptionCode string
}

// CarriesRate reports whether the category prints a VAT percentage. AE, E and
// O must be exported without one (BR-AE-*, BR-E-*, BR-O-*).
func (c Category) CarriesRate() bool { return c.Code == "S" || c.Code == "Z" }

// CategoryFor maps an invoice's VAT treatment (and rate, for domestic) to its
// EN 16931 category. Treatment applies to the whole invoice, so every line and
// every breakdown row shares the result.
func CategoryFor(t Treatment, rateBps int64) Category {
	switch t {
	case Domestic:
		if rateBps == 0 {
			return Category{Code: "Z"}
		}
		return Category{Code: "S"}
	case ReverseCharge:
		return Category{Code: "AE", ExemptionCode: "VATEX-EU-AE"}
	case Exempt:
		return Category{Code: "E"}
	case OutsideScope:
		return Category{Code: "O", ExemptionCode: "VATEX-EU-O"}
	default: // USSalesTax, None: not subject to EU VAT.
		return Category{Code: "O"}
	}
}
