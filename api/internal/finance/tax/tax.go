// Package tax holds the pure VAT / sales-tax rules used by invoicing:
// treatment suggestion, legal notes, totals computation and issue
// validation. It has no I/O and no dependency on the finance package so
// every rule is unit-testable in isolation (docs/INVOICING.md §3).
//
// This is a product specification, not tax advice: the notes and rules
// must be reviewed by an accountant for each launch market.
package tax

import "strings"

// Treatment is the VAT treatment of a whole invoice.
type Treatment string

const (
	// Domestic: the supplier charges its own country's VAT.
	Domestic Treatment = "domestic"
	// ReverseCharge: EU B2B cross-border; the customer accounts for VAT.
	ReverseCharge Treatment = "reverse_charge"
	// Exempt: the supplier is VAT-exempt (e.g. small-business scheme).
	Exempt Treatment = "exempt"
	// OutsideScope: EU supplier, customer outside the EU.
	OutsideScope Treatment = "outside_scope"
	// USSalesTax: US supplier charging state/local sales tax.
	USSalesTax Treatment = "us_sales_tax"
	// None: no tax applies (e.g. US services, non-EU supplier).
	None Treatment = "none"
)

var validTreatments = map[Treatment]struct{}{
	Domestic: {}, ReverseCharge: {}, Exempt: {}, OutsideScope: {}, USSalesTax: {}, None: {},
}

// IsValid reports whether t is a known treatment.
func (t Treatment) IsValid() bool {
	_, ok := validTreatments[t]
	return ok
}

// MaxRateBps is 100% in basis points.
const MaxRateBps = 10000

// MaxWithholdingRateBps is the highest withholding rate an invoice may be
// issued with (50%).
const MaxWithholdingRateBps = 5000

// euCountries is the EU-27 member set by ISO 3166-1 alpha-2 code. Greece is
// "GR" here (its VAT prefix "EL" is not a country code).
var euCountries = map[string]struct{}{
	"AT": {}, "BE": {}, "BG": {}, "HR": {}, "CY": {}, "CZ": {}, "DK": {},
	"EE": {}, "FI": {}, "FR": {}, "DE": {}, "GR": {}, "HU": {}, "IE": {},
	"IT": {}, "LV": {}, "LT": {}, "LU": {}, "MT": {}, "NL": {}, "PL": {},
	"PT": {}, "RO": {}, "SK": {}, "SI": {}, "ES": {}, "SE": {},
}

// IsEU reports whether country (ISO alpha-2, any case) is an EU member state.
func IsEU(country string) bool {
	_, ok := euCountries[normCountry(country)]
	return ok
}

func normCountry(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// Supplier is the tax-relevant view of the billing profile.
type Supplier struct {
	LegalName    string
	AddressLine1 string
	City         string
	PostalCode   string
	Country      string
	// VATID is the supplier's VAT registration number ('' when none).
	VATID                  string
	VATExemptSmallBusiness bool
	DefaultVATRateBps      int64
}

// Customer is the tax-relevant view of the invoice customer party.
type Customer struct {
	LegalName    string
	Company      string
	AddressLine1 string
	City         string
	Country      string
	VATID        string
	IsBusiness   bool
}

// Suggestion is the recommended treatment for a supplier/customer pair.
type Suggestion struct {
	VATTreatment Treatment `json:"vat_treatment"`
	TaxRateBps   int64     `json:"tax_rate_bps"`
	TaxNote      string    `json:"tax_note"`
	// Reason explains the choice, or asks the user to confirm it.
	Reason string `json:"reason"`
}

// Suggest applies the rule table of docs/INVOICING.md §3.
//
//	EU supplier, small-business exempt     → exempt, 0
//	EU supplier, same-country customer     → domestic, default rate
//	EU supplier, other EU, business+VAT ID → reverse_charge, 0
//	EU supplier, other EU, otherwise       → domestic + "confirm" reason
//	EU supplier, customer outside the EU   → outside_scope, 0
//	EU supplier, customer country unknown  → domestic + "confirm" reason
//	US supplier                            → none, 0 (user may pick us_sales_tax)
//	other / unknown supplier country       → none, 0
func Suggest(s Supplier, c Customer) Suggestion {
	sc, cc := normCountry(s.Country), normCountry(c.Country)
	mk := func(t Treatment, rate int64, reason string) Suggestion {
		return Suggestion{VATTreatment: t, TaxRateBps: rate, TaxNote: Notes.Note(sc, t), Reason: reason}
	}
	if !IsEU(sc) {
		if sc == "US" {
			return mk(None, 0, "US supplier: services are generally not subject to sales tax — switch to us_sales_tax if your state taxes them")
		}
		if sc == "" {
			return mk(None, 0, "supplier country unknown — set the billing profile country")
		}
		return mk(None, 0, "supplier outside the EU and US: no tax rules configured")
	}
	if s.VATExemptSmallBusiness {
		return mk(Exempt, 0, "supplier uses the small-business VAT exemption")
	}
	switch {
	case cc == "":
		return mk(Domestic, s.DefaultVATRateBps, "customer country unknown — confirm")
	case cc == sc:
		return mk(Domestic, s.DefaultVATRateBps, "customer in the supplier's country")
	case IsEU(cc):
		vatID := strings.TrimSpace(c.VATID)
		if vatID != "" && c.IsBusiness {
			return mk(ReverseCharge, 0, "EU business customer in another member state with a VAT ID")
		}
		if vatID == "" {
			return mk(Domestic, s.DefaultVATRateBps, "no customer VAT ID — confirm")
		}
		return mk(Domestic, s.DefaultVATRateBps, "customer not marked as a business — confirm")
	default:
		return mk(OutsideScope, 0, "customer outside the EU")
	}
}

// DefaultRate is the rate to use when a treatment is chosen without an
// explicit rate: the supplier's domestic rate for domestic, else 0.
func DefaultRate(t Treatment, s Supplier) int64 {
	if t == Domestic {
		return s.DefaultVATRateBps
	}
	return 0
}
