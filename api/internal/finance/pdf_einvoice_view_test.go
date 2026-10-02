package finance

import (
	"strings"
	"testing"
)

// germanInvoiceForPDF is pdfTestInvoice with the EN 16931 fields filled in.
func germanInvoiceForPDF() *InvoicePDFData {
	inv, lines := pdfTestInvoice()
	inv.BillingProfile = []byte(`{"legal_name":"Lina Vasquez","address_line1":"Köpenicker Str. 70","address_city":"Berlin",
		"address_postal":"10179","address_country":"DE","tax_id":"DE123456789","tax_id_kind":"vat",
		"tax_number":"37/123/45678","iban":"DE02120300000000202051","bic":"BYLADEM1001",
		"payment_instructions":"Please quote the invoice number."}`)
	inv.BuyerReference, inv.PurchaseOrderRef, inv.ContractRef = "04011000-12345-34", "PO-7781", "C-2026-09"
	inv.PaymentTerms = "Payable within 14 days without deduction."
	lines = append(lines,
		&InvoiceLine{Description: "Travel", Quantity: 200, UnitMinor: 50, UnitCode: "KMT", LineTotalMinor: 10000},
		&InvoiceLine{Description: "Soundcheck", Quantity: 3, UnitMinor: 2000, UnitCode: "HUR", LineTotalMinor: 6000},
	)
	return &InvoicePDFData{Invoice: inv, Lines: lines}
}

func TestBuildInvoiceView_PrintsTaxNumberAndReferences(t *testing.T) {
	v := buildInvoiceView(germanInvoiceForPDF())

	if !contains(v.Supplier, "Tax no.: 37/123/45678") || !contains(v.Supplier, "VAT ID: DE123456789") {
		t.Errorf("supplier block = %q", v.Supplier)
	}
	for label, want := range map[string]string{
		"Buyer reference:": "04011000-12345-34", "Order no.:": "PO-7781", "Contract:": "C-2026-09",
	} {
		if got, ok := rowValue(v.Details, label); !ok || got != want {
			t.Errorf("details[%s] = %q (present=%v), want %q; rows=%+v", label, got, ok, want, v.Details)
		}
	}
}

func TestBuildInvoiceView_OmitsEmptyReferences(t *testing.T) {
	inv, lines := pdfTestInvoice()
	v := buildInvoiceView(&InvoicePDFData{Invoice: inv, Lines: lines})
	for _, label := range []string{"Buyer reference:", "Order no.:", "Contract:"} {
		if _, ok := rowValue(v.Details, label); ok {
			t.Errorf("empty %s should not be printed", label)
		}
	}
}

func TestBuildInvoiceView_PaymentBlockShowsBankAccountAndTerms(t *testing.T) {
	v := buildInvoiceView(germanInvoiceForPDF())

	if got, _ := rowValue(v.Payment, "IBAN:"); got != "DE02 1203 0000 0000 2020 51" {
		t.Errorf("IBAN row = %q (IBANs print in groups of four)", got)
	}
	if got, _ := rowValue(v.Payment, "BIC:"); got != "BYLADEM1001" {
		t.Errorf("BIC row = %q", got)
	}
	if !strings.Contains(v.Instruction, "Payable within 14 days without deduction.") ||
		!strings.Contains(v.Instruction, "Please quote the invoice number.") {
		t.Errorf("instruction = %q, want payment terms and the free-text instructions", v.Instruction)
	}
}

func TestBuildInvoiceView_NoBankRowsWithoutIBAN(t *testing.T) {
	inv, lines := pdfTestInvoice() // snapshot has no iban
	v := buildInvoiceView(&InvoicePDFData{Invoice: inv, Lines: lines})
	for _, label := range []string{"IBAN:", "BIC:"} {
		if _, ok := rowValue(v.Payment, label); ok {
			t.Errorf("%s printed without a bank account", label)
		}
	}
}

func TestBuildInvoiceView_CreditNoteHasNoPaymentBlock(t *testing.T) {
	data := germanInvoiceForPDF()
	data.Invoice.Kind = InvoiceKindCreditNote
	v := buildInvoiceView(data)
	if len(v.Payment) != 0 {
		t.Errorf("credit notes take no payment, got %+v", v.Payment)
	}
}

func TestBuildInvoiceView_QuantityShowsUnit(t *testing.T) {
	v := buildInvoiceView(germanInvoiceForPDF())
	// Columns: #, description, qty, unit price, VAT, net.
	for i, want := range []string{"1", "200 km", "3 h"} {
		if got := v.Lines[i][2]; got != want {
			t.Errorf("line %d quantity = %q, want %q", i+1, got, want)
		}
	}
}
