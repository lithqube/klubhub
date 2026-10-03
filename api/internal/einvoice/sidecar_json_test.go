package einvoice

import (
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	cases := []struct {
		in       string
		sidecar  string
		xmlTwin  string
		mime     string
		ext      string
		xrechnng bool
	}{
		{"facturx", "Factur-X-EN16931", "CII", "application/pdf", ".pdf", false},
		{"xrechnung-cii", "XRECHNUNG-CII", "XRECHNUNG-CII", "application/xml", ".xml", true},
		{"xrechnung-ubl", "XRECHNUNG-UBL", "XRECHNUNG-UBL", "application/xml", ".xml", true},
	}
	for _, tc := range cases {
		f, ok := ParseFormat(tc.in)
		if !ok {
			t.Fatalf("ParseFormat(%q) rejected", tc.in)
		}
		if f.SidecarName() != tc.sidecar || f.ValidationName() != tc.xmlTwin || f.MimeType() != tc.mime ||
			f.Extension() != tc.ext || f.IsXRechnung() != tc.xrechnng {
			t.Errorf("%s: sidecar=%q twin=%q mime=%q ext=%q xr=%v", tc.in, f.SidecarName(), f.ValidationName(), f.MimeType(), f.Extension(), f.IsXRechnung())
		}
	}
	// Profiles that are not valid for German B2B (MINIMUM, BASIC WL) and
	// unknown names are never offered.
	for _, bad := range []string{"", "minimum", "basic-wl", "Factur-X-Minimum", "pdf", "XRECHNUNG-CII"} {
		if _, ok := ParseFormat(bad); ok {
			t.Errorf("ParseFormat(%q) should be rejected", bad)
		}
	}
	if len(Formats) != 3 {
		t.Errorf("Formats = %v", Formats)
	}
}

func TestSidecarJSON_DomesticInvoice(t *testing.T) {
	raw, err := sampleDoc().SidecarJSON()
	if err != nil {
		t.Fatal(err)
	}
	j := decodeJSON(t, raw)
	inv := at(j, "ubl:Invoice")
	if inv == nil {
		t.Fatalf("no ubl:Invoice root: %s", raw)
	}

	want := map[string]any{
		"cbc:ID": "INV-0042-EUR", "cbc:IssueDate": "2026-10-05", "cbc:DueDate": "2026-10-19",
		"cbc:InvoiceTypeCode": "380", "cbc:DocumentCurrencyCode": "EUR", "cbc:BuyerReference": "04011000-12345-34",
	}
	for k, v := range want {
		if got := at(inv, k); got != v {
			t.Errorf("%s = %v, want %v", k, got, v)
		}
	}
	// The CII schema requires a delivery block; the supply date fills it (BT-72).
	if got := at(inv, "cac:Delivery", "cbc:ActualDeliveryDate"); got != "2026-10-03" {
		t.Errorf("delivery date = %v", got)
	}
	if got := at(inv, "cac:PaymentTerms", "cbc:Note"); got != "Zahlbar bis 19.10.2026 ohne Abzug." {
		t.Errorf("payment terms = %v", got)
	}

	// Seller.
	seller := at(inv, "cac:AccountingSupplierParty", "cac:Party")
	if at(seller, "cbc:EndpointID") != "lina@example.com" || at(seller, "cbc:EndpointID@schemeID") != "EM" {
		t.Errorf("seller endpoint = %v / %v", at(seller, "cbc:EndpointID"), at(seller, "cbc:EndpointID@schemeID"))
	}
	if at(seller, "cac:PartyLegalEntity", "cbc:RegistrationName") != "Lina Vasquez" || at(seller, "cac:PartyName", "cbc:Name") != "DJ Lina V" {
		t.Errorf("seller names = %v", seller)
	}
	if at(seller, "cac:PostalAddress", "cbc:StreetName") != "Köpenicker Str. 70" ||
		at(seller, "cac:PostalAddress", "cbc:CityName") != "Berlin" ||
		at(seller, "cac:PostalAddress", "cbc:PostalZone") != "10179" ||
		at(seller, "cac:PostalAddress", "cac:Country", "cbc:IdentificationCode") != "DE" {
		t.Errorf("seller address = %v", at(seller, "cac:PostalAddress"))
	}
	if at(seller, "cac:Contact", "cbc:Name") != "Lina Vasquez" || at(seller, "cac:Contact", "cbc:Telephone") != "+49 30 1234567" ||
		at(seller, "cac:Contact", "cbc:ElectronicMail") != "lina@example.com" {
		t.Errorf("seller contact = %v", at(seller, "cac:Contact"))
	}
	// Repeatable nodes are arrays even with one element (the schema demands it).
	if at(seller, "cac:PartyTaxScheme", 0, "cbc:CompanyID") != "DE123456789" || at(seller, "cac:PartyTaxScheme", 0, "cac:TaxScheme", "cbc:ID") != "VAT" {
		t.Errorf("seller VAT = %v", at(seller, "cac:PartyTaxScheme"))
	}

	// Buyer.
	buyer := at(inv, "cac:AccountingCustomerParty", "cac:Party")
	if at(buyer, "cac:PartyLegalEntity", "cbc:RegistrationName") != "Club Beispiel GmbH" ||
		at(buyer, "cbc:EndpointID") != "buchhaltung@club-beispiel.example" ||
		at(buyer, "cac:PartyTaxScheme", "cbc:CompanyID") != "DE987654321" && at(buyer, "cac:PartyTaxScheme", 0, "cbc:CompanyID") != "DE987654321" {
		t.Errorf("buyer = %v", buyer)
	}

	// Payment means: SEPA credit transfer to the seller's account.
	if at(inv, "cac:PaymentMeans", 0, "cbc:PaymentMeansCode") != "58" ||
		at(inv, "cac:PaymentMeans", 0, "cbc:PaymentID") != "INV-0042-EUR" ||
		at(inv, "cac:PaymentMeans", 0, "cac:PayeeFinancialAccount", "cbc:ID") != "DE02120300000000202051" ||
		at(inv, "cac:PaymentMeans", 0, "cac:PayeeFinancialAccount", "cac:FinancialInstitutionBranch", "cbc:ID") != "BYLADEM1001" {
		t.Errorf("payment means = %v", at(inv, "cac:PaymentMeans"))
	}

	// Tax breakdown.
	if at(inv, "cac:TaxTotal", 0, "cbc:TaxAmount") != "228.00" || at(inv, "cac:TaxTotal", 0, "cbc:TaxAmount@currencyID") != "EUR" {
		t.Errorf("tax total = %v", at(inv, "cac:TaxTotal"))
	}
	sub := at(inv, "cac:TaxTotal", 0, "cac:TaxSubtotal", 0)
	if at(sub, "cbc:TaxableAmount") != "1200.00" || at(sub, "cbc:TaxAmount") != "228.00" ||
		at(sub, "cac:TaxCategory", "cbc:ID") != "S" || at(sub, "cac:TaxCategory", "cbc:Percent") != "19" ||
		at(sub, "cac:TaxCategory", "cac:TaxScheme", "cbc:ID") != "VAT" {
		t.Errorf("tax subtotal = %v", sub)
	}

	// Monetary totals.
	for k, v := range map[string]string{
		"cbc:LineExtensionAmount": "1200.00", "cbc:TaxExclusiveAmount": "1200.00",
		"cbc:TaxInclusiveAmount": "1428.00", "cbc:PayableAmount": "1428.00",
	} {
		if at(inv, "cac:LegalMonetaryTotal", k) != v || at(inv, "cac:LegalMonetaryTotal", k+"@currencyID") != "EUR" {
			t.Errorf("%s = %v", k, at(inv, "cac:LegalMonetaryTotal", k))
		}
	}

	// Lines.
	l0, l1 := at(inv, "cac:InvoiceLine", 0), at(inv, "cac:InvoiceLine", 1)
	if at(l0, "cbc:ID") != "1" || at(l0, "cbc:InvoicedQuantity") != "1" || at(l0, "cbc:InvoicedQuantity@unitCode") != "C62" ||
		at(l0, "cbc:LineExtensionAmount") != "1000.00" || at(l0, "cac:Price", "cbc:PriceAmount") != "1000.00" ||
		at(l0, "cac:Item", "cbc:Name") != "DJ performance 2026-10-03 (Äöü – Größe)" ||
		at(l0, "cac:Item", "cac:ClassifiedTaxCategory", "cbc:ID") != "S" || at(l0, "cac:Item", "cac:ClassifiedTaxCategory", "cbc:Percent") != "19" {
		t.Errorf("line 1 = %v", l0)
	}
	if at(l1, "cbc:InvoicedQuantity") != "2" || at(l1, "cbc:InvoicedQuantity@unitCode") != "KMT" || at(l1, "cac:Price", "cbc:PriceAmount") != "100.00" {
		t.Errorf("line 2 = %v", l1)
	}

	// Nothing optional that is empty leaks into the document.
	for _, k := range []string{"cac:OrderReference", "cac:ContractDocumentReference", "cac:BillingReference"} {
		if at(inv, k) != nil {
			t.Errorf("%s present although empty", k)
		}
	}
}

func TestSidecarJSON_References(t *testing.T) {
	d := sampleDoc()
	d.PurchaseOrderRef, d.ContractRef = "PO-7781", "C-2026-09"
	raw, _ := d.SidecarJSON()
	inv := at(decodeJSON(t, raw), "ubl:Invoice")
	if at(inv, "cac:OrderReference", "cbc:ID") != "PO-7781" || at(inv, "cac:ContractDocumentReference", "cbc:ID") != "C-2026-09" {
		t.Errorf("references = %v / %v", at(inv, "cac:OrderReference"), at(inv, "cac:ContractDocumentReference"))
	}
}

// A Steuernummer-only seller (Kleinunternehmer) needs the number as BT-32
// (tax scheme FC) AND as a seller identifier BT-29, or BR-CO-26 rejects it.
func TestSidecarJSON_SteuernummerOnlySeller(t *testing.T) {
	d := sampleDoc()
	d.Seller.VATID, d.Seller.TaxNumber = "", "37/123/45678"
	raw, _ := d.SidecarJSON()
	seller := at(decodeJSON(t, raw), "ubl:Invoice", "cac:AccountingSupplierParty", "cac:Party")
	if at(seller, "cac:PartyTaxScheme", 0, "cbc:CompanyID") != "37/123/45678" || at(seller, "cac:PartyTaxScheme", 0, "cac:TaxScheme", "cbc:ID") != "FC" {
		t.Errorf("BT-32 = %v", at(seller, "cac:PartyTaxScheme"))
	}
	if at(seller, "cac:PartyIdentification", 0, "cbc:ID") != "37/123/45678" {
		t.Errorf("BT-29 = %v", at(seller, "cac:PartyIdentification"))
	}
	if len(at(seller, "cac:PartyTaxScheme").([]any)) != 1 {
		t.Errorf("no VAT scheme entry without a VAT ID: %v", at(seller, "cac:PartyTaxScheme"))
	}
}

func TestSidecarJSON_VATIDAndSteuernummerTogether(t *testing.T) {
	d := sampleDoc()
	d.Seller.TaxNumber = "37/123/45678"
	raw, _ := d.SidecarJSON()
	schemes := at(decodeJSON(t, raw), "ubl:Invoice", "cac:AccountingSupplierParty", "cac:Party", "cac:PartyTaxScheme").([]any)
	if len(schemes) != 2 || at(schemes, 0, "cac:TaxScheme", "cbc:ID") != "VAT" || at(schemes, 1, "cac:TaxScheme", "cbc:ID") != "FC" {
		t.Errorf("schemes = %v", schemes)
	}
}

func TestSidecarJSON_CreditNote(t *testing.T) {
	d := sampleDoc()
	d.TypeCode, d.Number = TypeCreditNote, "CN-0001-EUR"
	d.PrecedingNumber, d.PrecedingIssueDate = "INV-0042-EUR", "2026-10-05"
	raw, _ := d.SidecarJSON()
	inv := at(decodeJSON(t, raw), "ubl:Invoice")
	if at(inv, "cbc:InvoiceTypeCode") != "381" {
		t.Errorf("type = %v", at(inv, "cbc:InvoiceTypeCode"))
	}
	ref := at(inv, "cac:BillingReference", 0, "cac:InvoiceDocumentReference")
	if at(ref, "cbc:ID") != "INV-0042-EUR" || at(ref, "cbc:IssueDate") != "2026-10-05" {
		t.Errorf("billing reference = %v", at(inv, "cac:BillingReference"))
	}
}

func TestSidecarJSON_TaxCategories(t *testing.T) {
	// Reverse charge: category AE, 0 %, VATEX code and the legal text as reason.
	rc := sampleDoc()
	cat := Category{Code: "AE", Percent: "0", HasPercent: true, ExemptionCode: "VATEX-EU-AE", ExemptionReason: "Steuerschuldnerschaft des Leistungsempfängers"}
	rc.Lines[0].Category, rc.Lines[1].Category = cat, cat
	rc.Taxes = []TaxLine{{Category: cat, Taxable: "1200.00", Tax: "0.00"}}
	rc.Totals.Tax, rc.Totals.TaxInclusive, rc.Totals.Payable = "0.00", "1200.00", "1200.00"
	raw, _ := rc.SidecarJSON()
	sub := at(decodeJSON(t, raw), "ubl:Invoice", "cac:TaxTotal", 0, "cac:TaxSubtotal", 0, "cac:TaxCategory")
	if at(sub, "cbc:ID") != "AE" || at(sub, "cbc:Percent") != "0" || at(sub, "cbc:TaxExemptionReasonCode") != "VATEX-EU-AE" ||
		at(sub, "cbc:TaxExemptionReason") != "Steuerschuldnerschaft des Leistungsempfängers" {
		t.Errorf("AE category = %v", sub)
	}

	// Outside the scope of VAT: category O has no rate on a line (EN 16931
	// BR-O-05), but the breakdown must carry one, 0 (XRechnung BR-DE-14, found by
	// running the official KoSIT validator).
	out := sampleDoc()
	o := Category{Code: "O", ExemptionCode: "VATEX-EU-O", ExemptionReason: "Nicht steuerbar"}
	out.Lines[0].Category, out.Lines[1].Category = o, o
	out.Taxes = []TaxLine{{Category: o, Taxable: "1200.00", Tax: "0.00"}}
	raw, _ = out.SidecarJSON()
	inv := at(decodeJSON(t, raw), "ubl:Invoice")
	if p := at(inv, "cac:TaxTotal", 0, "cac:TaxSubtotal", 0, "cac:TaxCategory", "cbc:Percent"); p != "0" {
		t.Errorf("the category O breakdown must print a rate of 0, got %v", p)
	}
	if p := at(inv, "cac:InvoiceLine", 0, "cac:Item", "cac:ClassifiedTaxCategory", "cbc:Percent"); p != nil {
		t.Errorf("line category O must not print a percentage, got %v", p)
	}

	// Exempt (§ 19): category E, 0 %, reason text but no VATEX code.
	ex := sampleDoc()
	e := Category{Code: "E", Percent: "0", HasPercent: true, ExemptionReason: "Gemäß § 19 UStG wird keine Umsatzsteuer berechnet."}
	ex.Lines[0].Category, ex.Lines[1].Category = e, e
	ex.Taxes = []TaxLine{{Category: e, Taxable: "1200.00", Tax: "0.00"}}
	raw, _ = ex.SidecarJSON()
	ec := at(decodeJSON(t, raw), "ubl:Invoice", "cac:TaxTotal", 0, "cac:TaxSubtotal", 0, "cac:TaxCategory")
	if at(ec, "cbc:ID") != "E" || at(ec, "cbc:TaxExemptionReason") != "Gemäß § 19 UStG wird keine Umsatzsteuer berechnet." || at(ec, "cbc:TaxExemptionReasonCode") != nil {
		t.Errorf("E category = %v", ec)
	}
}

// EN 16931 BR-O-02: an invoice with a "not subject to VAT" line (category O)
// must not carry the seller's or the buyer's VAT ID. The seller is identified
// by the Steuernummer instead (found by the real sidecar + validator).
func TestSidecarJSON_OutsideScopeDropsVATIDs(t *testing.T) {
	d := sampleDoc()
	d.Seller.TaxNumber = "37/123/45678"
	d.Buyer.Country, d.Buyer.VATID = "CH", "CHE-123.456.789"
	o := Category{Code: "O", ExemptionCode: "VATEX-EU-O", ExemptionReason: "Nicht steuerbar"}
	d.Lines[0].Category, d.Lines[1].Category = o, o
	d.Taxes = []TaxLine{{Category: o, Taxable: "1200.00", Tax: "0.00"}}

	raw, _ := d.SidecarJSON()
	inv := at(decodeJSON(t, raw), "ubl:Invoice")
	seller := at(inv, "cac:AccountingSupplierParty", "cac:Party")
	schemes := at(seller, "cac:PartyTaxScheme").([]any)
	if len(schemes) != 1 || at(schemes, 0, "cac:TaxScheme", "cbc:ID") != "FC" || at(schemes, 0, "cbc:CompanyID") != "37/123/45678" {
		t.Errorf("only the Steuernummer remains on the seller: %v", schemes)
	}
	if at(seller, "cac:PartyIdentification", 0, "cbc:ID") != "37/123/45678" {
		t.Errorf("BT-29 = %v", at(seller, "cac:PartyIdentification"))
	}
	if at(inv, "cac:AccountingCustomerParty", "cac:Party", "cac:PartyTaxScheme") != nil {
		t.Errorf("the buyer's VAT ID must be dropped too")
	}
	if strings.Contains(string(raw), "DE123456789") || strings.Contains(string(raw), "CHE-123") {
		t.Errorf("a VAT ID leaked: %s", raw)
	}
}

func TestSidecarJSON_OmitsPaymentMeansWithoutAnAccount(t *testing.T) {
	d := sampleDoc()
	d.Payment = nil
	raw, _ := d.SidecarJSON()
	if at(decodeJSON(t, raw), "ubl:Invoice", "cac:PaymentMeans") != nil {
		t.Error("no payment means without an account")
	}
}

func TestSidecarJSON_EveryValueIsAString(t *testing.T) {
	raw, _ := sampleDoc().SidecarJSON()
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				walk(path+"/"+k, c)
			}
		case []any:
			for i, c := range x {
				walk(path+"/"+string(rune('0'+i)), c)
			}
		case string:
		default:
			t.Errorf("%s is %T; the sidecar's schema accepts strings only", path, v)
		}
	}
	walk("", decodeJSON(t, raw))
	if strings.Contains(string(raw), "null") {
		t.Errorf("null leaked into the document: %s", raw)
	}
}
