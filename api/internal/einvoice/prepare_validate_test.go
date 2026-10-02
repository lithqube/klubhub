package einvoice

import (
	"errors"
	"strings"
	"testing"
)

// --- Prepare: friendly pre-checks, before anything is generated ------------------

func TestPrepare_CompleteDocumentIsReadyForEveryFormat(t *testing.T) {
	for _, f := range Formats {
		if ps := Prepare(sampleDoc(), f); len(ps) != 0 {
			t.Errorf("%s: unexpected problems %v", f, ps)
		}
	}
}

func TestPrepare_RequiredForEveryFormat(t *testing.T) {
	cases := map[string]func(*Document){
		"seller.name":        func(d *Document) { d.Seller.Name = " " },
		"seller.street":      func(d *Document) { d.Seller.Street = "" },
		"seller.city":        func(d *Document) { d.Seller.City = "" },
		"seller.postal_code": func(d *Document) { d.Seller.Postal = "" },
		"seller.country":     func(d *Document) { d.Seller.Country = "" },
		"seller.tax_id":      func(d *Document) { d.Seller.VATID, d.Seller.TaxNumber = "", "" },
		"buyer.name":         func(d *Document) { d.Buyer.Name = "" },
		"buyer.street":       func(d *Document) { d.Buyer.Street = "" },
		"buyer.city":         func(d *Document) { d.Buyer.City = "" },
		"buyer.postal_code":  func(d *Document) { d.Buyer.Postal = "" },
		"buyer.country":      func(d *Document) { d.Buyer.Country = "" },
		"delivery_date":      func(d *Document) { d.DeliveryDate = "" },
		"lines":              func(d *Document) { d.Lines = nil },
	}
	for field, mutate := range cases {
		for _, f := range Formats {
			d := sampleDoc()
			mutate(&d)
			ps := Prepare(d, f)
			if !hasProblem(ps, field) {
				t.Errorf("%s / %s: want a problem on %q, got %v", f, field, field, ps)
			}
		}
	}
}

func TestPrepare_SellerNeedsOnlyOneTaxIdentifier(t *testing.T) {
	d := sampleDoc()
	d.Seller.VATID, d.Seller.TaxNumber = "", "37/123/45678"
	if hasProblem(Prepare(d, FacturX), "seller.tax_id") {
		t.Error("a Steuernummer alone is enough")
	}
	d.Seller.VATID, d.Seller.TaxNumber = "DE123456789", ""
	if hasProblem(Prepare(d, FacturX), "seller.tax_id") {
		t.Error("a VAT ID alone is enough")
	}
}

// Category O forbids the VAT IDs on the document (BR-O-02), so the seller can
// only be identified by the Steuernummer.
func TestPrepare_OutsideScopeNeedsTheSellersTaxNumber(t *testing.T) {
	o := Category{Code: "O", ExemptionCode: "VATEX-EU-O", ExemptionReason: "Nicht steuerbar"}
	d := sampleDoc()
	d.Lines[0].Category, d.Lines[1].Category = o, o
	d.Taxes = []TaxLine{{Category: o, Taxable: "1200.00", Tax: "0.00"}}

	for _, f := range Formats {
		ps := Prepare(d, f) // VAT ID only
		if !hasProblem(ps, "seller.tax_id") {
			t.Errorf("%s: want a seller.tax_id problem, got %v", f, ps)
		}
	}
	for _, p := range Prepare(d, FacturX) {
		if p.Field == "seller.tax_id" && !strings.Contains(p.Message, "Steuernummer") {
			t.Errorf("the message should name the tax number: %q", p.Message)
		}
	}
	d.Seller.TaxNumber = "37/123/45678"
	for _, f := range Formats {
		if hasProblem(Prepare(d, f), "seller.tax_id") {
			t.Errorf("%s: a Steuernummer satisfies it", f)
		}
	}
}

// XRechnung (BR-DE-*) asks for more than the EN 16931 core that Factur-X needs.
func TestPrepare_XRechnungOnly(t *testing.T) {
	cases := map[string]func(*Document){
		"buyer_reference":     func(d *Document) { d.BuyerReference = "" },
		"seller.contact_name": func(d *Document) { d.Seller.ContactName = "" },
		"seller.phone":        func(d *Document) { d.Seller.Phone = "" },
		"seller.email":        func(d *Document) { d.Seller.Email = "" },
		"buyer.email":         func(d *Document) { d.Buyer.Email = "" },
		"payment.iban":        func(d *Document) { d.Payment = nil },
	}
	for field, mutate := range cases {
		d := sampleDoc()
		mutate(&d)
		for _, f := range []Format{XRechnungCII, XRechnungUBL} {
			if !hasProblem(Prepare(d, f), field) {
				t.Errorf("%s: want a problem on %q", f, field)
			}
		}
		if hasProblem(Prepare(d, FacturX), field) {
			t.Errorf("facturx must not require %q", field)
		}
	}
	d := sampleDoc()
	d.Payment = &Payment{BIC: "BYLADEM1001"} // an account without an IBAN is no account
	if !hasProblem(Prepare(d, XRechnungCII), "payment.iban") {
		t.Error("BIC alone is not a payment account")
	}
}

func TestPrepare_ProblemsExplainWhatToDo(t *testing.T) {
	d := sampleDoc()
	d.BuyerReference, d.Seller.Phone = "", ""
	for _, p := range Prepare(d, XRechnungCII) {
		if strings.TrimSpace(p.Message) == "" || p.Field == "" {
			t.Errorf("problem %+v needs a field and a message", p)
		}
	}
	for _, p := range Prepare(d, XRechnungCII) {
		if p.Field == "buyer_reference" && !strings.Contains(p.Message, "Leitweg") {
			t.Errorf("the buyer reference message should name the Leitweg-ID: %q", p.Message)
		}
	}
}

// --- ProblemFor: validator findings, in words the user can act on -----------------

func TestProblemFor(t *testing.T) {
	cases := map[string]string{
		"BR-DE-15": "buyer_reference",
		"BR-DE-2":  "seller.contact_name",
		"BR-DE-5":  "seller.contact_name",
		"BR-DE-6":  "seller.phone",
		"BR-DE-7":  "seller.email",
		"BR-DE-1":  "payment.iban",
		"BR-DE-23": "payment.iban",
		"BR-DE-3":  "seller.city",
		"BR-DE-4":  "seller.postal_code",
		"BR-DE-8":  "buyer.city",
		"BR-DE-9":  "buyer.postal_code",
		"BR-CO-26": "seller.tax_id",
		"BR-CO-16": "document",
		"BR-S-08":  "document",
	}
	for code, field := range cases {
		p := ProblemFor(Violation{Rule: code, Text: "something specific"})
		if p.Field != field {
			t.Errorf("%s -> %q, want %q", code, p.Field, field)
		}
		if !strings.Contains(p.Message, code) {
			t.Errorf("%s: the message keeps the rule code for support: %q", code, p.Message)
		}
	}
}

// --- Validate: the EN 16931 / XRechnung rule engine, in process -------------------

func TestValidate_AcceptsTheSpikeOutputs(t *testing.T) {
	for _, f := range []string{"xrechnung-cii.xml", "xrechnung-ubl.xml", "credit-note-cii.xml", "kleinunternehmer-cii.xml"} {
		r, err := Validate(readFixture(t, f))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if !r.OK() {
			t.Errorf("%s: violations %v", f, r.Violations)
		}
	}
}

func TestValidate_ReportsBusinessRuleViolations(t *testing.T) {
	r, err := Validate(readFixture(t, "bad-totals-cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if r.OK() || len(r.Violations) == 0 {
		t.Fatal("totals that do not add up must be reported")
	}
	var found bool
	for _, v := range r.Violations {
		if v.Rule == "BR-CO-16" && v.Text != "" {
			found = true
		}
	}
	if !found {
		t.Errorf("want BR-CO-16 (amount due = total - paid), got %v", r.Violations)
	}
}

func TestValidate_DetectsTheFormatAndProfile(t *testing.T) {
	cii, _ := Validate(readFixture(t, "xrechnung-cii.xml"))
	ubl, _ := Validate(readFixture(t, "xrechnung-ubl.xml"))
	if cii.Syntax != "CII" || ubl.Syntax != "UBL" {
		t.Errorf("syntax = %q / %q", cii.Syntax, ubl.Syntax)
	}
	if !cii.XRechnung || !ubl.XRechnung {
		t.Errorf("XRechnung profile not detected: %v / %v", cii.XRechnung, ubl.XRechnung)
	}
}

func TestValidate_RejectsWhatIsNotAnInvoice(t *testing.T) {
	for name, in := range map[string]string{
		"empty": "", "text": "hello", "other xml": `<?xml version="1.0"?><note>hi</note>`,
	} {
		if _, err := Validate([]byte(in)); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s: err = %v, want ErrUnreadable", name, err)
		}
	}
}
