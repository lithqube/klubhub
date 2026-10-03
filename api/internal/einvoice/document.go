// Package einvoice turns an invoice into a legally valid structured e-invoice
// (EN 16931: Factur-X / ZUGFeRD hybrid PDF, XRechnung in CII or UBL).
//
// It knows nothing about KlubHub's own invoice types. The caller maps its
// invoice into a Document of plain strings; this package then
//
//  1. lists in plain words what is still missing (Prepare),
//  2. has the generator (the e-invoice-eu sidecar, behind Generator) produce
//     the XML, and
//  3. validates that XML in process against the EN 16931 and German XRechnung
//     business rules (Validate) before anything is handed out.
//
// Nothing invalid is ever shipped: a document that fails a check comes back as
// a NotExportableError listing what to fix.
//
// The sidecar does not calculate totals or check business rules, so the
// caller's numbers are authoritative and Validate is the safety net
// (docs/INVOICING.md, .claude/plans/e-invoicing.plan.md section 8).
package einvoice

// Document type codes (UNTDID 1001).
const (
	TypeInvoice    = "380"
	TypeCreditNote = "381"
)

// Format is an output format. Only profiles that are valid for German B2B are
// offered: Factur-X MINIMUM and BASIC WL are not.
type Format string

const (
	// FacturX is a hybrid PDF/A-3 carrying EN 16931 CII XML (ZUGFeRD).
	FacturX Format = "facturx"
	// XRechnungCII and XRechnungUBL are bare XRechnung XML files.
	XRechnungCII Format = "xrechnung-cii"
	XRechnungUBL Format = "xrechnung-ubl"
)

// Formats lists every format, in the order a UI should show them.
var Formats = []Format{FacturX, XRechnungCII, XRechnungUBL}

// ParseFormat accepts exactly the three public names.
func ParseFormat(s string) (Format, bool) {
	for _, f := range Formats {
		if string(f) == s {
			return f, true
		}
	}
	return "", false
}

// Valid reports whether f is a known format.
func (f Format) Valid() bool { _, ok := ParseFormat(string(f)); return ok }

// SidecarName is the format name the e-invoice-eu server expects.
func (f Format) SidecarName() string {
	switch f {
	case FacturX:
		return "Factur-X-EN16931"
	case XRechnungCII:
		return "XRECHNUNG-CII"
	case XRechnungUBL:
		return "XRECHNUNG-UBL"
	}
	return ""
}

// ValidationName is the XML-only sidecar format carrying the same business
// content, which is what gets validated: for Factur-X the plain EN 16931 CII,
// for XRechnung the shipped file itself.
func (f Format) ValidationName() string {
	if f == FacturX {
		return "CII"
	}
	return f.SidecarName()
}

// MimeType of the file the format produces.
func (f Format) MimeType() string {
	if f == FacturX {
		return "application/pdf"
	}
	return "application/xml"
}

// Extension of the file the format produces.
func (f Format) Extension() string {
	if f == FacturX {
		return ".pdf"
	}
	return ".xml"
}

// IsXRechnung reports whether the German XRechnung rules (BR-DE-*) apply.
func (f Format) IsXRechnung() bool { return f == XRechnungCII || f == XRechnungUBL }

// outsideScope reports whether any line or breakdown row is category O ("not
// subject to VAT"). EN 16931 rule BR-O-02 then forbids the seller's and the
// buyer's VAT IDs anywhere on the document, so the seller is identified by
// the tax number (Steuernummer) alone.
func (d Document) outsideScope() bool {
	for _, l := range d.Lines {
		if l.Category.Code == "O" {
			return true
		}
	}
	for _, t := range d.Taxes {
		if t.Category.Code == "O" {
			return true
		}
	}
	return false
}

// Problem is one thing to fix before the invoice can be exported. Field names
// are stable identifiers a UI can attach the message to.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Document is an invoice in EN 16931 terms, everything as the strings that go
// into the file: amounts are decimals with the currency's own number of
// places ("1428.00"), dates are YYYY-MM-DD.
type Document struct {
	Number   string
	TypeCode string // TypeInvoice or TypeCreditNote
	Currency string

	IssueDate    string
	DueDate      string
	DeliveryDate string // BT-72, the supply date

	BuyerReference   string // BT-10, the Leitweg-ID for public-sector customers
	PurchaseOrderRef string // BT-13
	ContractRef      string // BT-12
	PaymentTerms     string // BT-20

	// Credit notes name the invoice they correct (BG-3).
	PrecedingNumber    string
	PrecedingIssueDate string

	Seller  Party
	Buyer   Party
	Payment *Payment // nil when there is no bank account

	Lines  []Line
	Taxes  []TaxLine
	Totals Totals
}

// Party is the seller or the buyer.
type Party struct {
	Name        string // legal name
	TradingName string
	Street      string
	Street2     string
	City        string
	Postal      string
	Region      string
	Country     string // ISO 3166-1 alpha-2
	VATID       string
	TaxNumber   string // national tax number (BT-32), e.g. the Steuernummer
	Email       string // also the electronic address (scheme EM)
	ContactName string // seller only
	Phone       string // seller only
}

// Payment is a SEPA credit transfer to the seller's account (payment means 58).
type Payment struct {
	IBAN      string
	BIC       string
	Reference string // remittance information, the invoice number
}

// Category is an EN 16931 VAT category. Percent is only printed when
// HasPercent (S, Z, E and AE print one; O must not).
type Category struct {
	Code            string
	Percent         string
	HasPercent      bool
	ExemptionCode   string // VATEX code
	ExemptionReason string // legal wording
}

// Line is one invoice line.
type Line struct {
	ID        int
	Name      string
	Quantity  string
	UnitCode  string
	UnitPrice string
	Net       string
	Category  Category
}

// TaxLine is one row of the VAT breakdown.
type TaxLine struct {
	Category Category
	Taxable  string
	Tax      string
}

// Totals are the document totals. Payable is the amount due: EN 16931 has no
// field for withholding, so the caller must not export such an invoice.
type Totals struct {
	Lines        string
	TaxExclusive string
	Tax          string
	TaxInclusive string
	Payable      string
}
