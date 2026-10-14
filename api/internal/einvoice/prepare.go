package einvoice

import (
	"fmt"
	"strings"
)

func blank(s string) bool { return strings.TrimSpace(s) == "" }

// Prepare lists, in plain words, what is missing from d before it can be
// exported as f. It checks only what the user can fix; everything finer (the
// 200-odd EN 16931 business rules) is the validator's job after generation.
// An empty result means the document is ready to generate.
func Prepare(d Document, f Format) []Problem {
	var ps []Problem
	add := func(field, msg string) { ps = append(ps, Problem{Field: field, Message: msg}) }

	// The seller: your billing profile.
	if blank(d.Seller.Name) {
		add("seller.name", "Your billing profile needs a legal name.")
	}
	if blank(d.Seller.Street) {
		add("seller.street", "Your billing profile needs a street address.")
	}
	if blank(d.Seller.City) {
		add("seller.city", "Your billing profile needs a city.")
	}
	if blank(d.Seller.Postal) {
		add("seller.postal_code", "Your billing profile needs a postal code.")
	}
	if blank(d.Seller.Country) {
		add("seller.country", "Your billing profile needs a country (2-letter code, e.g. DE).")
	}
	switch {
	case d.outsideScope():
		// BR-O-02: no VAT IDs may appear on a document with a "not subject to
		// VAT" line, so the Steuernummer is the only way to identify the seller.
		if blank(d.Seller.TaxNumber) {
			add("seller.tax_id", "An invoice outside the scope of VAT may not show your VAT ID in an e-invoice, so add your tax number (Steuernummer) to your billing profile.")
		}
	case blank(d.Seller.VATID) && blank(d.Seller.TaxNumber):
		add("seller.tax_id", "Add a VAT ID or a tax number (Steuernummer) to your billing profile.")
	}

	// The buyer: the invoice's customer.
	if blank(d.Buyer.Name) {
		add("buyer.name", "The customer needs a name.")
	}
	if blank(d.Buyer.Street) {
		add("buyer.street", "The customer needs a street address.")
	}
	if blank(d.Buyer.City) {
		add("buyer.city", "The customer needs a city.")
	}
	if blank(d.Buyer.Postal) {
		add("buyer.postal_code", "The customer needs a postal code.")
	}
	if blank(d.Buyer.Country) {
		add("buyer.country", "The customer needs a country.")
	}

	if blank(d.DeliveryDate) {
		add("delivery_date", "The invoice needs a supply date.")
	}
	if len(d.Lines) == 0 {
		add("lines", "The invoice needs at least one line.")
	}

	if f.IsXRechnung() {
		if blank(d.BuyerReference) {
			add("buyer_reference", "XRechnung needs a buyer reference: for a public-sector customer this is the Leitweg-ID; otherwise use their order or customer number. Add it under References and terms.")
		}
		if blank(d.Seller.ContactName) {
			add("seller.contact_name", "XRechnung needs a contact name for the seller.")
		}
		if blank(d.Seller.Phone) {
			add("seller.phone", "XRechnung needs a seller phone number: add one to your billing profile.")
		}
		if blank(d.Seller.Email) {
			add("seller.email", "XRechnung needs a seller email address: add one to your billing profile.")
		}
		if blank(d.Buyer.Email) {
			add("buyer.email", "XRechnung needs the customer's email address, which is their electronic address.")
		}
		if d.Payment == nil || blank(d.Payment.IBAN) {
			add("payment.iban", "XRechnung needs a bank account: add your IBAN to your billing profile.")
		}
	}
	return ps
}

// ProblemFor turns a validator finding into a Problem. Rules that point at a
// field the user can edit get that field and a plain message; the rest are
// reported on "document". The rule code always stays in the message, so a
// support request can quote it.
func ProblemFor(v Violation) Problem {
	field, hint := "document", ""
	switch v.Rule {
	case "BR-DE-15":
		field, hint = "buyer_reference", "Add the buyer reference (Leitweg-ID) under References and terms."
	case "BR-DE-2", "BR-DE-5":
		field, hint = "seller.contact_name", "The seller needs a contact name."
	case "BR-DE-6":
		field, hint = "seller.phone", "Add a phone number to your billing profile."
	case "BR-DE-7":
		field, hint = "seller.email", "Add an email address to your billing profile."
	case "BR-DE-1", "BR-DE-23":
		field, hint = "payment.iban", "Add your IBAN to your billing profile."
	case "BR-DE-3":
		field, hint = "seller.city", "Add a city to your billing profile."
	case "BR-DE-4":
		field, hint = "seller.postal_code", "Add a postal code to your billing profile."
	case "BR-DE-8":
		field, hint = "buyer.city", "The customer needs a city."
	case "BR-DE-9":
		field, hint = "buyer.postal_code", "The customer needs a postal code."
	case "BR-CO-26":
		field, hint = "seller.tax_id", "Add a VAT ID or a tax number (Steuernummer) to your billing profile."
	}
	if hint != "" {
		return Problem{Field: field, Message: fmt.Sprintf("%s (rule %s)", hint, v.Rule)}
	}
	return Problem{Field: field, Message: fmt.Sprintf("%s: %s", v.Rule, v.Text)}
}
