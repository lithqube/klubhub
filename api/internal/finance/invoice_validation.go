package finance

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/klubhub/dj/api/internal/finance/tax"
)

// normalizeParty trims fields, upper-cases the country and compacts tax
// numbers so comparisons and printing are stable.
func normalizeParty(p *Party) {
	trim := func(s *string) { *s = strings.TrimSpace(*s) }
	for _, f := range []*string{&p.LegalName, &p.Company, &p.Email, &p.AddressLine1, &p.AddressLine2,
		&p.City, &p.Region, &p.PostalCode, &p.TaxID} {
		trim(f)
	}
	p.Country = strings.ToUpper(strings.TrimSpace(p.Country))
	p.VATID = strings.NewReplacer(" ", "", ".", "").Replace(strings.ToUpper(strings.TrimSpace(p.VATID)))
}

// validateParty reports shape problems (not issue readiness) for a party.
func validateParty(prefix string, p Party) InvoiceValidationErrors {
	var errs InvoiceValidationErrors
	limits := []struct {
		field string
		value string
		max   int
	}{
		{"legal_name", p.LegalName, 200}, {"company", p.Company, 200}, {"email", p.Email, 254},
		{"address_line1", p.AddressLine1, 200}, {"address_line2", p.AddressLine2, 200},
		{"city", p.City, 100}, {"region", p.Region, 100}, {"postal_code", p.PostalCode, 20},
		{"vat_id", p.VATID, 20}, {"tax_id", p.TaxID, 40},
	}
	for _, l := range limits {
		if utf8.RuneCountInString(l.value) > l.max {
			errs = append(errs, InvoiceFieldError{prefix + l.field, fmt.Sprintf("exceeds %d characters", l.max)})
		}
	}
	if p.Country != "" && !isUpperAlpha(p.Country, 2) {
		errs = append(errs, InvoiceFieldError{prefix + "country", "must be a 2-letter ISO 3166-1 alpha-2 code"})
	}
	return errs
}

func isUpperAlpha(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// normalizePrefix upper-cases a number prefix, defaulting to INV.
func normalizePrefix(p string) string {
	p = strings.ToUpper(strings.TrimSpace(p))
	if p == "" {
		return DefaultNumberingConfig().Prefix
	}
	return p
}

// validatePrefix checks an invoice number prefix: 1–20 of A–Z/0–9 (a
// hyphen would break the PREFIX-0001-CUR format) and not the reserved
// credit-note series.
func validatePrefix(p string) InvoiceValidationErrors {
	if len(p) == 0 || len(p) > 20 {
		return InvoiceValidationErrors{{"number_prefix", "must be 1 to 20 characters"}}
	}
	for _, r := range p {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return InvoiceValidationErrors{{"number_prefix", "may only contain letters and digits"}}
		}
	}
	if p == CreditNotePrefix {
		return InvoiceValidationErrors{{"number_prefix", "CN is reserved for credit notes"}}
	}
	return nil
}

// normalizeDate trims an optional YYYY-MM-DD date; "" becomes nil.
func normalizeDate(d *string) *string {
	if d == nil {
		return nil
	}
	v := strings.TrimSpace(*d)
	if v == "" {
		return nil
	}
	return &v
}

func validateDate(field string, d *string) InvoiceValidationErrors {
	if d == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", *d); err != nil {
		return InvoiceValidationErrors{{field, "must be a date in YYYY-MM-DD format"}}
	}
	return nil
}

// validateTaxFields checks the tax inputs a draft may hold. Issue readiness
// (e.g. withholding ≤ 50%, rate 0 for reverse charge) is tax.ValidateForIssue's job.
func validateTaxFields(t tax.Treatment, rateBps, withholdingBps int64, note string) InvoiceValidationErrors {
	var errs InvoiceValidationErrors
	if !t.IsValid() {
		errs = append(errs, InvoiceFieldError{"vat_treatment",
			"must be one of domestic, reverse_charge, exempt, outside_scope, us_sales_tax, none"})
	}
	if rateBps < 0 || rateBps > tax.MaxRateBps {
		errs = append(errs, InvoiceFieldError{"tax_rate_bps", "must be between 0 and 10000"})
	}
	if withholdingBps < 0 || withholdingBps > tax.MaxRateBps {
		errs = append(errs, InvoiceFieldError{"withholding_rate_bps", "must be between 0 and 10000"})
	}
	if utf8.RuneCountInString(note) > 1000 {
		errs = append(errs, InvoiceFieldError{"tax_note", "exceeds 1000 characters"})
	}
	return errs
}

// normalizeUpdate normalizes and validates a PUT body in place.
func normalizeUpdate(req *UpdateInvoiceRequest) error {
	normalizeParty(&req.Customer)
	req.NumberPrefix = normalizePrefix(req.NumberPrefix)
	req.SupplyDate = normalizeDate(req.SupplyDate)
	req.TaxNote = strings.TrimSpace(req.TaxNote)
	var errs InvoiceValidationErrors
	errs = append(errs, validateParty("customer.", req.Customer)...)
	errs = append(errs, validateTaxFields(req.VATTreatment, req.TaxRateBps, req.WithholdingRateBps, req.TaxNote)...)
	errs = append(errs, validatePrefix(req.NumberPrefix)...)
	errs = append(errs, validateDate("supply_date", req.SupplyDate)...)
	if utf8.RuneCountInString(req.InternalNotes) > 5000 {
		errs = append(errs, InvoiceFieldError{"internal_notes", "exceeds 5000 characters"})
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// supplierFromProfile maps the billing profile to the tax supplier view.
// The profile's tax_id counts as a VAT ID only when tax_id_kind is "vat".
func supplierFromProfile(p *BillingProfile) tax.Supplier {
	if p == nil {
		return tax.Supplier{}
	}
	s := tax.Supplier{
		LegalName:              p.LegalName,
		AddressLine1:           p.AddressLine1,
		City:                   p.AddressCity,
		PostalCode:             p.AddressPostal,
		Country:                p.AddressCountry,
		VATExemptSmallBusiness: p.VATExemptSmallBusiness,
		DefaultVATRateBps:      p.DefaultVATRateBps,
	}
	if p.TaxIDKind == TaxIDKindVAT {
		s.VATID = p.TaxID
	}
	return s
}
