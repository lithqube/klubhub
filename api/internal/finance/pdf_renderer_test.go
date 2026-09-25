package finance

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/finance/tax"
)

// TestRenderInvoice_NonLatin1 ensures the embedded UTF-8 font path renders
// characters outside the cp1252 range (accents, diacritics, symbols) without
// error and produces a well-formed PDF.
func TestRenderInvoice_NonLatin1(t *testing.T) {
	renderer := NewPDFRenderer()

	num := "INV-2026-0001"
	inv := &Invoice{
		ID:            uuid.New(),
		GigID:         uuid.New(),
		InvoiceNumber: &num,
		Customer:      Party{LegalName: "Željko Café d.o.o.", AddressLine1: "Ulica 1", City: "Zagreb", Country: "HR"},
		Currency:      "EUR",
		SubtotalMinor: 10000,
		TaxMinor:      2000,
		TotalMinor:    12000,
		Status:        InvoiceStatusIssued,
	}
	lines := []*InvoiceLine{
		{
			Description:    "Łukasz Perić – Café Zürich ☆ set",
			Quantity:       1,
			UnitMinor:      10000,
			TaxBps:         2000,
			LineTotalMinor: 12000,
		},
	}
	billing := &BillingProfile{
		LegalName:    "Łukasz Perić",
		AddressLine1: "Café Zürich ☆",
		AddressCity:  "Zürich",
		TaxID:        "CHE-123.456.789",
	}

	data := &InvoicePDFData{
		Invoice:        inv,
		Lines:          lines,
		BillingProfile: billing,
	}

	out, filename, contentType, err := renderer.RenderInvoice(context.Background(), data)
	if err != nil {
		t.Fatalf("RenderInvoice returned error: %v", err)
	}
	if filename == "" {
		t.Error("expected non-empty filename")
	}
	if contentType != "application/pdf" {
		t.Errorf("expected content type application/pdf, got %q", contentType)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("expected output to start with %%PDF, got %q", out[:min(20, len(out))])
	}
}

func TestTruncate_RuneSafe(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short ascii unchanged", "hello", 10, "hello"},
		{"exact length unchanged", "hello", 5, "hello"},
		{"ascii truncated", "hello world", 8, "hello..."},
		{"multibyte runes not split", "Łukasz Perić – Café Zürich ☆", 10, "Łukasz ..."},
		{"maxLen smaller than ellipsis", "Łukasz", 2, "Łu"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.input, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
			for _, r := range got {
				if r == '�' {
					t.Errorf("truncate produced invalid rune replacement in %q", got)
				}
			}
		})
	}
}

func pdfTestInvoice() (*Invoice, []*InvoiceLine) {
	num, seq, supply := "INV-0007-EUR", int64(7), "2026-10-03"
	issued := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	inv := &Invoice{
		ID: uuid.New(), Kind: InvoiceKindInvoice, GigID: uuid.New(), InvoiceNumber: &num, NumberPrefix: "INV",
		NumberSeq: &seq, Currency: "EUR", Status: InvoiceStatusIssued, SupplyDate: &supply, IssuedAt: &issued,
		Customer: Party{LegalName: "Paris Club SAS", Company: "Paris Club SAS", AddressLine1: "1 Rue Oberkampf",
			City: "Paris", PostalCode: "75011", Country: "FR", VATID: "FR12345678901", IsBusiness: true},
		BillingProfile: []byte(`{"legal_name":"Lina Vasquez","address_line1":"Köpenicker Str. 70","address_city":"Berlin",
			"address_postal":"10179","address_country":"DE","tax_id":"DE123456789","tax_id_kind":"vat",
			"payment_instructions":"IBAN DE00 1234"}`),
		VATTreatment: tax.ReverseCharge, TaxRateBps: 0, TaxNote: tax.Notes.Note("DE", tax.ReverseCharge),
		SubtotalMinor: 100000, TotalMinor: 100000, WithholdingRateBps: 1500, WithholdingMinor: 15000, NetPayableMinor: 85000,
		TaxBreakdown:  []tax.BreakdownRow{{RateBps: 0, TaxableMinor: 100000, TaxMinor: 0}},
		ReceivedMinor: 20000,
	}
	lines := []*InvoiceLine{{Description: "DJ performance", Quantity: 1, UnitMinor: 100000, LineTotalMinor: 100000}}
	return inv, lines
}

func rowValue(rows []pdfRow, label string) (string, bool) {
	for _, r := range rows {
		if r.Label == label {
			return r.Value, true
		}
	}
	return "", false
}

func TestBuildInvoiceView_InvoiceLegalContent(t *testing.T) {
	inv, lines := pdfTestInvoice()
	v := buildInvoiceView(&InvoicePDFData{Invoice: inv, Lines: lines})

	if v.Title != "INVOICE" || v.Number != "INV-0007-EUR" || v.Reference != "" || v.Filename != "invoice-INV-0007-EUR.pdf" {
		t.Fatalf("header = %q %q %q %q", v.Title, v.Number, v.Reference, v.Filename)
	}
	wantCustomer := []string{"Paris Club SAS", "1 Rue Oberkampf", "75011 Paris", "FR", "VAT ID: FR12345678901"}
	if strings.Join(v.Customer, "|") != strings.Join(wantCustomer, "|") {
		t.Errorf("customer block = %q", v.Customer)
	}
	if !contains(v.Supplier, "VAT ID: DE123456789") || v.Supplier[0] != "Lina Vasquez" || v.Instruction != "IBAN DE00 1234" {
		t.Errorf("supplier block = %q instr=%q", v.Supplier, v.Instruction)
	}
	for label, want := range map[string]string{"Supply date:": "2026-10-03", "Issue date:": "2026-10-05", "VAT treatment:": "Reverse charge"} {
		if got, _ := rowValue(v.Details, label); got != want {
			t.Errorf("%s = %q, want %q", label, got, want)
		}
	}
	for label, want := range map[string]string{
		"Subtotal": "€1000.00", "VAT 0% on €1000.00": "€0.00", "Total": "€1000.00",
		"Withholding tax 15%": "-€150.00", "Net payable": "€850.00",
	} {
		if got, ok := rowValue(v.Totals, label); !ok || got != want {
			t.Errorf("totals[%s] = %q (present=%v), want %q; rows=%+v", label, got, ok, want, v.Totals)
		}
	}
	if !strings.Contains(v.TaxNote, "Art. 196") {
		t.Errorf("tax note = %q", v.TaxNote)
	}
	if got, _ := rowValue(v.Payment, "Outstanding:"); got != "€650.00" {
		t.Errorf("outstanding = %q", got)
	}
	if v.Lines[0][4] != "0%" || v.Lines[0][5] != "€1000.00" {
		t.Errorf("line = %q", v.Lines[0])
	}
}

func TestBuildInvoiceView_CreditNoteAndDraft(t *testing.T) {
	inv, lines := pdfTestInvoice()
	orig := inv.ID
	cnNum := "CN-0001-EUR"
	inv.Kind, inv.InvoiceNumber, inv.CreditsInvoiceID = InvoiceKindCreditNote, &cnNum, &orig
	v := buildInvoiceView(&InvoicePDFData{Invoice: inv, Lines: lines, CreditedInvoiceNumber: "INV-0007-EUR"})
	if v.Title != "CREDIT NOTE" || v.Number != "CN-0001-EUR" || v.Reference != "Credit note for invoice INV-0007-EUR" {
		t.Fatalf("credit note header = %q %q %q", v.Title, v.Number, v.Reference)
	}
	if got, _ := rowValue(v.Totals, "Total"); got != "-€1000.00" {
		t.Errorf("credit note total = %q, want negative", got)
	}
	if got, _ := rowValue(v.Totals, "Net payable"); got != "-€850.00" {
		t.Errorf("credit note net payable = %q", got)
	}
	if len(v.Payment) != 0 || !strings.HasPrefix(v.Filename, "credit-note-CN-0001") {
		t.Errorf("credit note payment/filename = %+v %q", v.Payment, v.Filename)
	}
	if _, ok := rowValue(v.Details, "Due date:"); ok {
		t.Error("credit notes have no due date")
	}

	// Without the number, the reference falls back to the id.
	v = buildInvoiceView(&InvoicePDFData{Invoice: inv, Lines: lines})
	if !strings.Contains(v.Reference, orig.String()) {
		t.Errorf("fallback reference = %q", v.Reference)
	}

	draft, _ := pdfTestInvoice()
	draft.Status, draft.InvoiceNumber, draft.BillingProfile, draft.Customer = InvoiceStatusDraft, nil, []byte(`{}`), Party{}
	draft.WithholdingRateBps, draft.WithholdingMinor = 0, 0
	v = buildInvoiceView(&InvoicePDFData{Invoice: draft, BillingProfile: &BillingProfile{LegalName: "Live Profile", AddressCity: "Berlin"}})
	if v.Title != "DRAFT INVOICE" || !strings.HasPrefix(v.Number, "DRAFT") || v.Supplier[0] != "Live Profile" || v.Customer[0] != "[Customer not set]" {
		t.Fatalf("draft view = %+v", v)
	}
	if _, ok := rowValue(v.Totals, "Net payable"); ok {
		t.Error("no withholding rows without withholding")
	}
}

func TestRenderInvoice_CreditNotePDF(t *testing.T) {
	inv, lines := pdfTestInvoice()
	cn := "CN-0001-EUR"
	inv.Kind, inv.InvoiceNumber = InvoiceKindCreditNote, &cn
	out, filename, _, err := NewPDFRenderer().RenderInvoice(context.Background(),
		&InvoicePDFData{Invoice: inv, Lines: lines, CreditedInvoiceNumber: "INV-0007-EUR"})
	if err != nil || !bytes.HasPrefix(out, []byte("%PDF")) || filename != "credit-note-CN-0001-EUR.pdf" {
		t.Fatalf("render: %v %q", err, filename)
	}
	if _, _, _, err := NewPDFRenderer().RenderInvoice(context.Background(), &InvoicePDFData{}); err == nil {
		t.Fatal("expected error without invoice")
	}
}

func TestFormatHelpers(t *testing.T) {
	for in, want := range map[int64]string{0: "0%", 1900: "19%", 550: "5.5%", 10000: "100%", 725: "7.25%", 1000: "10%"} {
		if got := formatBps(in); got != want {
			t.Errorf("formatBps(%d) = %q, want %q", in, got, want)
		}
	}
	cases := []struct {
		cur   string
		minor int64
		want  string
	}{{"EUR", 29750, "€297.50"}, {"USD", 5, "$0.05"}, {"GBP", -1234, "-£12.34"}, {"CHF", 100, "1.00 CHF"}}
	for _, c := range cases {
		if got := formatMoney(c.cur, c.minor); got != c.want {
			t.Errorf("formatMoney(%s, %d) = %q, want %q", c.cur, c.minor, got, c.want)
		}
	}
}
