package tax

import (
	"fmt"
	"strings"
)

// Problem is one reason an invoice cannot be issued yet. Field is a dotted
// path the UI can attach the message to, e.g. "customer.vat_id".
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// IssueInput is everything ValidateForIssue looks at.
type IssueInput struct {
	Supplier           Supplier
	Customer           Customer
	Treatment          Treatment
	TaxRateBps         int64
	TaxNote            string
	WithholdingRateBps int64
	SupplyDateSet      bool
	TotalMinor         int64
	Currency           string
	// GigCurrency is the gig fee currency; '' skips the match check.
	GigCurrency string
}

// ValidateForIssue returns every rule of docs/INVOICING.md §3 the invoice
// violates; an empty (non-nil) slice means it can be issued.
func ValidateForIssue(in IssueInput) []Problem {
	ps := []Problem{}
	add := func(field, msg string) { ps = append(ps, Problem{Field: field, Message: msg}) }
	blank := func(s string) bool { return strings.TrimSpace(s) == "" }

	s, c := in.Supplier, in.Customer
	sc, cc := normCountry(s.Country), normCountry(c.Country)

	// Supplier identity (billing profile).
	if blank(s.LegalName) {
		add("supplier.legal_name", "supplier legal name is required")
	}
	if blank(s.AddressLine1) {
		add("supplier.address_line1", "supplier address is required")
	}
	if blank(s.City) {
		add("supplier.city", "supplier city is required")
	}
	if blank(s.PostalCode) {
		add("supplier.postal_code", "supplier postal code is required")
	}
	if sc == "" {
		add("supplier.country", "supplier country is required")
	}

	// Customer identity.
	if blank(c.LegalName) && blank(c.Company) {
		add("customer.legal_name", "customer legal name or company is required")
	}
	if blank(c.AddressLine1) {
		add("customer.address_line1", "customer address is required")
	}
	if blank(c.City) {
		add("customer.city", "customer city is required")
	}
	if cc == "" {
		add("customer.country", "customer country is required")
	}

	if !in.SupplyDateSet {
		add("supply_date", "supply date is required")
	}
	if in.TotalMinor <= 0 {
		add("total_minor", "total must be greater than zero")
	}
	if in.GigCurrency != "" && !strings.EqualFold(in.GigCurrency, in.Currency) {
		add("currency", fmt.Sprintf("currency %s does not match the gig fee currency %s", in.Currency, in.GigCurrency))
	}

	switch in.Treatment {
	case Domestic:
		if in.TaxRateBps <= 0 {
			add("tax_rate_bps", "domestic VAT needs a tax rate above 0%")
		}
		if IsEU(sc) && blank(s.VATID) {
			add("supplier.vat_id", "a VAT ID is required to charge domestic VAT")
		}
	case ReverseCharge:
		if blank(s.VATID) {
			add("supplier.vat_id", "reverse charge requires the supplier VAT ID")
		}
		if blank(c.VATID) {
			add("customer.vat_id", "reverse charge requires the customer VAT ID")
		}
		if sc != "" && !IsEU(sc) {
			add("supplier.country", "reverse charge requires an EU supplier")
		}
		if cc != "" && !IsEU(cc) {
			add("customer.country", "reverse charge requires an EU customer")
		}
		if sc != "" && sc == cc {
			add("customer.country", "reverse charge requires supplier and customer in different countries")
		}
		if in.TaxRateBps != 0 {
			add("tax_rate_bps", "reverse charge invoices carry 0% VAT")
		}
	case Exempt, OutsideScope, None:
		if in.TaxRateBps != 0 {
			add("tax_rate_bps", fmt.Sprintf("%s invoices carry no tax", in.Treatment))
		}
		if in.Treatment != None && blank(in.TaxNote) {
			add("tax_note", "a legal note explaining why no VAT is charged is required")
		}
	case USSalesTax:
		// State/local rules vary; any rate the user sets is accepted.
	default:
		add("vat_treatment", "unknown VAT treatment")
	}

	if in.WithholdingRateBps < 0 || in.WithholdingRateBps > MaxWithholdingRateBps {
		add("withholding_rate_bps", "withholding must be between 0% and 50%")
	}
	return ps
}
