package tax

// German legal notes for invoices issued by a supplier whose billing profile
// country is DE. They are registered at package init, so Suggest, new drafts
// and the tax-notes endpoint all print the same wording.
//
// Each note carries German first and English second, in one string separated
// by " / ": the customer's language is not known, the German phrase is the
// one the Finanzamt and the customer's bookkeeper look for, and the English
// half keeps the invoice understandable abroad. The wording is the common
// practice for these cases; it is not tax advice. Have a Steuerberater
// confirm it before relying on it (docs/INVOICING.md §3).
//
// Treatments with no entry here (domestic VAT, US sales tax, none) print no
// note, and other countries keep the generic English defaults until their
// own country module exists.
func init() {
	// Kleinunternehmer: no VAT is charged and none may be shown (§ 19 UStG).
	Notes.Register("DE", Exempt,
		"Gemäß § 19 UStG wird keine Umsatzsteuer berechnet. / "+
			"VAT is not charged under § 19 UStG (German small-business scheme).")

	// EU business customer in another member state: the recipient accounts for
	// the VAT. German law requires the words "Steuerschuldnerschaft des
	// Leistungsempfängers" on such an invoice (§ 14a Abs. 5 UStG).
	Notes.Register("DE", ReverseCharge,
		"Steuerschuldnerschaft des Leistungsempfängers (Reverse Charge): "+
			"Die Umsatzsteuer ist vom Leistungsempfänger zu entrichten (Art. 196 MwStSystRL). / "+
			"Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC).")

	// Business customer outside the EU: the place of supply of the service is
	// the customer's country (§ 3a Abs. 2 UStG), so German VAT does not apply.
	Notes.Register("DE", OutsideScope,
		"Nicht steuerbare sonstige Leistung, Leistungsort außerhalb Deutschlands (§ 3a Abs. 2 UStG). / "+
			"Not subject to German VAT: the place of supply is outside Germany (§ 3a(2) UStG).")
}
