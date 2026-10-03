package finance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/einvoice"
	"github.com/klubhub/dj/api/internal/finance/tax"
)

// --- test doubles ----------------------------------------------------------------

type fakeGenerator struct {
	calls []string
	out   map[string][]byte
	err   error
}

func (g *fakeGenerator) Generate(_ context.Context, name string, _ []byte, _ []byte) ([]byte, error) {
	g.calls = append(g.calls, name)
	if g.err != nil {
		return nil, g.err
	}
	return g.out[name], nil
}

func einvoiceFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../einvoice/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newFakeGenerator(t *testing.T) *fakeGenerator {
	return &fakeGenerator{out: map[string][]byte{
		"CII":              einvoiceFixture(t, "xrechnung-cii.xml"),
		"XRECHNUNG-CII":    einvoiceFixture(t, "xrechnung-cii.xml"),
		"XRECHNUNG-UBL":    einvoiceFixture(t, "xrechnung-ubl.xml"),
		"Factur-X-EN16931": []byte("%PDF-1.7 hybrid"),
	}}
}

// einvoiceHarness is the invoice harness with an e-invoice exporter behind the
// handler. The supplier is complete for XRechnung (phone, IBAN, BIC, tax number).
type einvoiceHarness struct {
	*invoiceHarness
	gen *fakeGenerator
	svc *InvoiceService
}

func newEInvoiceHarness(t *testing.T) *einvoiceHarness {
	t.Helper()
	hs := newInvoiceHarness(t)
	p := hs.billing.profile
	p.ContactPhone, p.IBAN, p.BIC, p.TaxNumber = "+49 30 1234567", "DE02120300000000202051", "BYLADEM1001", "37/123/45678"
	p.AddressCountry, p.TradingName = "DE", "DJ Test"
	gen := newFakeGenerator(t)
	svc := NewInvoiceService(hs.repo, hs.billing, hs.gigs).WithEInvoice(&einvoice.Exporter{Gen: gen})
	hs.h = NewInvoiceHandler(svc)
	return &einvoiceHarness{invoiceHarness: hs, gen: gen, svc: svc}
}

// issued returns an issued invoice whose customer has what XRechnung needs.
func (hs *einvoiceHarness) issued(over map[string]any) Invoice {
	hs.t.Helper()
	draft := hs.createDraft("")
	cust := *completeCustomer()
	cust.Email = "buchhaltung@club-hamburg.example"
	body := map[string]any{"customer": cust, "buyer_reference": "04011000-12345-34"}
	for k, v := range over {
		body[k] = v
	}
	if code, out := hs.putDraft(draft, body); code != http.StatusOK {
		hs.t.Fatalf("put: %d %v", code, out)
	}
	return hs.issue(hs.getInvoice(draft.ID))
}

func (hs *einvoiceHarness) get(path string) *httptest.ResponseRecorder {
	hs.t.Helper()
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices"+path, nil))
	return rec
}

// --- the mapping ------------------------------------------------------------------

func mapped(t *testing.T, hs *einvoiceHarness, inv Invoice) (einvoice.Document, []einvoice.Problem) {
	t.Helper()
	data, err := hs.svc.pdfData(context.Background(), inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return einvoiceDocument(data)
}

func TestEInvoiceDocument_DomesticInvoice(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(map[string]any{"purchase_order_ref": "PO-7781", "contract_ref": "C-9", "payment_terms": "14 days net"})
	doc, problems := mapped(t, hs, inv)
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}

	if doc.Number != inv.Number() || doc.TypeCode != einvoice.TypeInvoice || doc.Currency != "EUR" {
		t.Errorf("header = %+v", doc)
	}
	if doc.IssueDate != inv.IssuedAt.Format("2006-01-02") || doc.DeliveryDate != "2026-10-03" || doc.DueDate != "" && len(doc.DueDate) != 10 {
		t.Errorf("dates = %q %q %q", doc.IssueDate, doc.DeliveryDate, doc.DueDate)
	}
	if doc.BuyerReference != "04011000-12345-34" || doc.PurchaseOrderRef != "PO-7781" || doc.ContractRef != "C-9" || doc.PaymentTerms != "14 days net" {
		t.Errorf("references = %+v", doc)
	}

	s := doc.Seller
	if s.Name != "Test DJ" || s.TradingName != "DJ Test" || s.Street != "1 Test Street" || s.City != "Berlin" || s.Postal != "10115" ||
		s.Country != "DE" || s.VATID != "DE123456789" || s.TaxNumber != "37/123/45678" ||
		s.Email != "billing@test.com" || s.Phone != "+49 30 1234567" || s.ContactName != "Test DJ" {
		t.Errorf("seller = %+v", s)
	}
	b := doc.Buyer
	if b.Name != "Club Hamburg GmbH" || b.Street != "Reeperbahn 1" || b.City != "Hamburg" || b.Postal != "20359" || b.Country != "DE" ||
		b.Email != "buchhaltung@club-hamburg.example" {
		t.Errorf("buyer = %+v", b)
	}
	if doc.Payment == nil || doc.Payment.IBAN != "DE02120300000000202051" || doc.Payment.BIC != "BYLADEM1001" || doc.Payment.Reference != inv.Number() {
		t.Errorf("payment = %+v", doc.Payment)
	}

	// 250.00 net, 19 %.
	if len(doc.Lines) != 1 {
		t.Fatalf("lines = %+v", doc.Lines)
	}
	l := doc.Lines[0]
	if l.ID != 1 || l.Quantity != "1" || l.UnitCode != "C62" || l.UnitPrice != "250.00" || l.Net != "250.00" ||
		l.Category.Code != "S" || l.Category.Percent != "19" || !l.Category.HasPercent {
		t.Errorf("line = %+v", l)
	}
	if len(doc.Taxes) != 1 || doc.Taxes[0].Taxable != "250.00" || doc.Taxes[0].Tax != "47.50" || doc.Taxes[0].Category.Code != "S" {
		t.Errorf("taxes = %+v", doc.Taxes)
	}
	if doc.Totals != (einvoice.Totals{Lines: "250.00", TaxExclusive: "250.00", Tax: "47.50", TaxInclusive: "297.50", Payable: "297.50"}) {
		t.Errorf("totals = %+v", doc.Totals)
	}
}

func TestEInvoiceDocument_LinesCarryTheirUnitsAndAmounts(t *testing.T) {
	hs := newEInvoiceHarness(t)
	lines := []map[string]any{
		{"description": "DJ set", "quantity": 1, "unit_minor": 100000, "unit_code": "LS"},
		{"description": "Soundcheck", "quantity": 3, "unit_minor": 2050, "unit_code": "HUR"},
	}
	inv := hs.issued(map[string]any{"lines": lines})
	doc, _ := mapped(t, hs, inv)
	if len(doc.Lines) != 2 || doc.Lines[1].ID != 2 || doc.Lines[1].UnitCode != "HUR" || doc.Lines[1].Quantity != "3" ||
		doc.Lines[1].UnitPrice != "20.50" || doc.Lines[1].Net != "61.50" {
		t.Errorf("lines = %+v", doc.Lines)
	}
	if doc.Totals.Lines != "1061.50" || doc.Totals.Tax != "201.69" || doc.Totals.Payable != "1263.19" {
		t.Errorf("totals = %+v", doc.Totals)
	}
}

func TestEInvoiceDocument_TaxTreatments(t *testing.T) {
	hs := newEInvoiceHarness(t)
	fr := *completeCustomer()
	fr.Country, fr.VATID, fr.IsBusiness, fr.Email = "FR", "FR12345678901", true, "compta@club-paris.example"

	rc := hs.issued(map[string]any{"customer": fr, "vat_treatment": "reverse_charge", "tax_rate_bps": 0, "tax_note": tax.Notes.Note("DE", tax.ReverseCharge)})
	doc, problems := mapped(t, hs, rc)
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	c := doc.Lines[0].Category
	if c.Code != "AE" || c.Percent != "0" || !c.HasPercent || c.ExemptionCode != "VATEX-EU-AE" || !strings.Contains(c.ExemptionReason, "Steuerschuldnerschaft") {
		t.Errorf("reverse charge category = %+v", c)
	}
	if doc.Taxes[0].Category.Code != "AE" || doc.Taxes[0].Tax != "0.00" || doc.Totals.Payable != "250.00" {
		t.Errorf("reverse charge breakdown = %+v %+v", doc.Taxes, doc.Totals)
	}

	ch := fr
	ch.Country, ch.VATID = "CH", ""
	out := hs.issued(map[string]any{"customer": ch, "vat_treatment": "outside_scope", "tax_rate_bps": 0, "tax_note": tax.Notes.Note("DE", tax.OutsideScope)})
	doc, _ = mapped(t, hs, out)
	c = doc.Lines[0].Category
	if c.Code != "O" || c.HasPercent || c.ExemptionCode != "VATEX-EU-O" || !strings.Contains(c.ExemptionReason, "§ 3a") {
		t.Errorf("outside scope category = %+v", c)
	}

	ex := hs.issued(map[string]any{"vat_treatment": "exempt", "tax_rate_bps": 0, "tax_note": tax.Notes.Note("DE", tax.Exempt)})
	doc, _ = mapped(t, hs, ex)
	c = doc.Lines[0].Category
	if c.Code != "E" || c.Percent != "0" || !c.HasPercent || c.ExemptionCode != "" || !strings.Contains(c.ExemptionReason, "§ 19 UStG") {
		t.Errorf("exempt category = %+v", c)
	}
}

func TestEInvoiceDocument_SellerVATIDOnlyWhenTheProfileTaxIDIsOne(t *testing.T) {
	hs := newEInvoiceHarness(t)
	hs.billing.profile.TaxIDKind, hs.billing.profile.TaxID = TaxIDKindEIN, "12-3456789"
	// Without a VAT ID the supplier cannot charge domestic VAT, so it issues exempt.
	doc, _ := mapped(t, hs, hs.issued(map[string]any{"vat_treatment": "exempt", "tax_rate_bps": 0, "tax_note": tax.Notes.Note("DE", tax.Exempt)}))
	if doc.Seller.VATID != "" {
		t.Errorf("an EIN is not a VAT ID: %+v", doc.Seller)
	}
}

func TestEInvoiceDocument_CreditNoteNamesTheOriginal(t *testing.T) {
	hs := newEInvoiceHarness(t)
	orig := hs.issued(nil)
	_, out := hs.do(http.MethodPost, "/"+orig.ID.String()+"/credit-note", map[string]any{"reason": "cancelled", "updated_at": orig.UpdatedAt})
	var res CreditNoteResult
	decode(t, out["data"], &res)

	doc, problems := mapped(t, hs, *res.CreditNote)
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if doc.TypeCode != einvoice.TypeCreditNote || doc.PrecedingNumber != orig.Number() || doc.PrecedingIssueDate != orig.IssuedAt.Format("2006-01-02") {
		t.Errorf("credit note = type %s preceding %q %q", doc.TypeCode, doc.PrecedingNumber, doc.PrecedingIssueDate)
	}
	if doc.Totals.Payable != "297.50" {
		t.Errorf("credit notes keep positive amounts in EN 16931: %+v", doc.Totals)
	}
}

// What EN 16931 cannot express blocks the export instead of producing a file
// whose numbers disagree with the PDF.
func TestEInvoiceDocument_BlockedCases(t *testing.T) {
	hs := newEInvoiceHarness(t)
	withholding := hs.issued(map[string]any{"withholding_rate_bps": 1500})
	_, problems := mapped(t, hs, withholding)
	if !hasProblemField(problems, "withholding") {
		t.Errorf("withholding must block: %v", problems)
	}

	// A three-decimal currency does not fit the generator's two-decimal amounts.
	data, _ := hs.svc.pdfData(context.Background(), withholding.ID)
	data.Invoice.WithholdingRateBps, data.Invoice.WithholdingMinor = 0, 0
	data.Invoice.Currency = "KWD"
	if _, ps := einvoiceDocument(data); !hasProblemField(ps, "currency") {
		t.Errorf("a 3-decimal currency must block: %v", ps)
	}
	// A zero-decimal currency prints whole numbers.
	data.Invoice.Currency = "JPY"
	data.Invoice.SubtotalMinor, data.Invoice.TaxMinor, data.Invoice.TotalMinor = 25000, 4750, 29750
	data.Lines[0].UnitMinor, data.Lines[0].LineTotalMinor = 25000, 25000
	doc, ps := einvoiceDocument(data)
	if len(ps) != 0 || doc.Lines[0].UnitPrice != "25000" || doc.Totals.Payable != "29750" {
		t.Errorf("JPY = %v %+v", ps, doc.Totals)
	}
}

func hasProblemField(ps []einvoice.Problem, field string) bool {
	for _, p := range ps {
		if p.Field == field {
			return true
		}
	}
	return false
}

// --- the service ----------------------------------------------------------------------

func TestInvoiceService_EInvoiceCheck(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(nil)
	for _, f := range einvoice.Formats {
		chk, err := hs.svc.EInvoiceCheck(context.Background(), inv.ID, f)
		if err != nil || !chk.Ready || len(chk.Problems) != 0 {
			t.Errorf("%s: %+v err=%v", f, chk, err)
		}
	}

	// A customer without an email can take Factur-X but not XRechnung.
	noMail := hs.issued(map[string]any{"customer": *completeCustomer()})
	fx, _ := hs.svc.EInvoiceCheck(context.Background(), noMail.ID, einvoice.FacturX)
	xr, _ := hs.svc.EInvoiceCheck(context.Background(), noMail.ID, einvoice.XRechnungCII)
	if !fx.Ready || xr.Ready || !hasProblemField(xr.Problems, "buyer.email") {
		t.Errorf("factur-x %+v, xrechnung %+v", fx, xr)
	}
}

func TestInvoiceService_EInvoiceExport(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(nil)

	f, err := hs.svc.EInvoice(context.Background(), inv.ID, einvoice.XRechnungCII)
	if err != nil {
		t.Fatal(err)
	}
	if f.Filename != inv.Number()+"-xrechnung-cii.xml" || f.MimeType != "application/xml" || !f.Report.OK() {
		t.Errorf("file = %s %s %+v", f.Filename, f.MimeType, f.Report)
	}

	f, err = hs.svc.EInvoice(context.Background(), inv.ID, einvoice.FacturX)
	if err != nil {
		t.Fatal(err)
	}
	if f.Filename != inv.Number()+"-facturx.pdf" || f.MimeType != "application/pdf" || string(f.Data) != "%PDF-1.7 hybrid" {
		t.Errorf("file = %s %s %q", f.Filename, f.MimeType, f.Data)
	}
}

func TestInvoiceService_EInvoiceRules(t *testing.T) {
	hs := newEInvoiceHarness(t)
	ctx := context.Background()

	// Only numbered documents: drafts and cancelled invoices have no e-invoice.
	draft := hs.createDraft("")
	if _, err := hs.svc.EInvoice(ctx, draft.ID, einvoice.FacturX); !errors.Is(err, ErrInvoiceBadState) {
		t.Errorf("draft: %v", err)
	}
	if _, err := hs.svc.EInvoiceCheck(ctx, draft.ID, einvoice.FacturX); !errors.Is(err, ErrInvoiceBadState) {
		t.Errorf("draft check: %v", err)
	}
	if _, err := hs.svc.EInvoice(ctx, uuid.New(), einvoice.FacturX); !errors.Is(err, ErrInvoiceNotFound) {
		t.Errorf("unknown: %v", err)
	}

	// Missing data comes back as a list, and nothing is generated.
	incomplete := hs.issued(map[string]any{"buyer_reference": ""})
	hs.gen.calls = nil
	_, err := hs.svc.EInvoice(ctx, incomplete.ID, einvoice.XRechnungUBL)
	var ne *einvoice.NotExportableError
	if !errors.As(err, &ne) || !hasProblemField(ne.Problems, "buyer_reference") {
		t.Errorf("err = %v", err)
	}
	if len(hs.gen.calls) != 0 {
		t.Errorf("generator called for an incomplete invoice: %v", hs.gen.calls)
	}

	// Withholding blocks even Factur-X, and says why.
	w := hs.issued(map[string]any{"withholding_rate_bps": 1500})
	if _, err := hs.svc.EInvoice(ctx, w.ID, einvoice.FacturX); !errors.As(err, &ne) || !hasProblemField(ne.Problems, "withholding") {
		t.Errorf("withholding: %v", err)
	}

	// Without a configured generator the feature is simply unavailable.
	bare := NewInvoiceService(hs.repo, hs.billing, hs.gigs)
	issued := hs.issued(nil)
	if _, err := bare.EInvoice(ctx, issued.ID, einvoice.FacturX); !errors.Is(err, einvoice.ErrUnavailable) {
		t.Errorf("no exporter: %v", err)
	}
	if _, err := bare.EInvoiceCheck(ctx, issued.ID, einvoice.FacturX); !errors.Is(err, einvoice.ErrUnavailable) {
		t.Errorf("no exporter (check): %v", err)
	}
}

// --- golden: real invoices through the real sidecar and the rule engine ------------------

// EINVOICE_URL=http://127.0.0.1:3101 go test ./internal/finance -run EInvoiceGolden
// Everything the mapper produces from real finance invoices must pass the
// EN 16931 / XRechnung business rules, in every tax situation and format.
// keepGolden writes an export to $EINVOICE_OUT_DIR, if set, for the KoSIT run in
// scripts/einvoice-golden.sh (see the same helper in package einvoice).
func keepGolden(t *testing.T, name string, f einvoice.Format, file *EInvoiceFile) {
	t.Helper()
	dir := os.Getenv("EINVOICE_OUT_DIR")
	if dir == "" {
		return
	}
	base := dir + "/" + strings.NewReplacer("/", "_", " ", "_").Replace(name) + "__" + string(f)
	ext := ".xml"
	if f == einvoice.FacturX {
		ext = ".pdf"
	}
	if err := os.WriteFile(base+ext, file.Data, 0o600); err != nil {
		t.Fatalf("keep golden file: %v", err)
	}
}

func TestEInvoiceGolden_RealInvoicesPassTheRuleEngine(t *testing.T) {
	url := os.Getenv("EINVOICE_URL")
	if url == "" {
		t.Skip("EINVOICE_URL not set")
	}
	hs := newEInvoiceHarness(t)
	svc := NewInvoiceService(hs.repo, hs.billing, hs.gigs).WithEInvoice(&einvoice.Exporter{Gen: einvoice.NewSidecarClient(url, nil)})

	fr := *completeCustomer()
	fr.Country, fr.VATID, fr.IsBusiness, fr.Email = "FR", "FR12345678901", true, "compta@club-paris.example"
	ch := fr
	ch.Country, ch.VATID = "CH", ""
	multi := []map[string]any{
		{"description": "DJ set (Äöü – Größe)", "quantity": 1, "unit_minor": 100000, "unit_code": "LS"},
		{"description": "Soundcheck", "quantity": 3, "unit_minor": 2050, "unit_code": "HUR"},
		{"description": "Travel", "quantity": 200, "unit_minor": 50, "unit_code": "KMT"},
	}
	cases := map[string]map[string]any{
		"domestic":       nil,
		"multi-line":     {"lines": multi, "purchase_order_ref": "PO-7781", "contract_ref": "C-9", "payment_terms": "14 days net"},
		"reverse-charge": {"customer": fr, "vat_treatment": "reverse_charge", "tax_rate_bps": 0, "tax_note": tax.Notes.Note("DE", tax.ReverseCharge)},
		"exempt":         {"vat_treatment": "exempt", "tax_rate_bps": 0, "tax_note": tax.Notes.Note("DE", tax.Exempt)},
		"outside-scope":  {"customer": ch, "vat_treatment": "outside_scope", "tax_rate_bps": 0, "tax_note": tax.Notes.Note("DE", tax.OutsideScope)},
	}
	for name, over := range cases {
		inv := hs.issued(over)
		for _, f := range einvoice.Formats {
			t.Run(name+"/"+string(f), func(t *testing.T) {
				file, err := svc.EInvoice(context.Background(), inv.ID, f)
				if err != nil {
					t.Fatalf("export: %v", err)
				}
				if !file.Report.OK() {
					t.Fatalf("violations: %v", file.Report.Violations)
				}
				keepGolden(t, "invoice-"+name, f, file)
			})
		}
	}

	// A credit note, in every format.
	orig := hs.issued(nil)
	_, out := hs.do(http.MethodPost, "/"+orig.ID.String()+"/credit-note", map[string]any{"reason": "cancelled", "updated_at": orig.UpdatedAt})
	var res CreditNoteResult
	decode(t, out["data"], &res)
	for _, f := range einvoice.Formats {
		file, err := svc.EInvoice(context.Background(), res.CreditNote.ID, f)
		if err != nil {
			t.Errorf("credit note as %s: %v", f, err)
			continue
		}
		keepGolden(t, "invoice-credit-note", f, file)
	}
}

// --- the HTTP contract -----------------------------------------------------------------

func TestInvoiceHandler_EInvoiceDownload(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(nil)

	rec := hs.get("/" + inv.ID.String() + "/einvoice?format=xrechnung-cii")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/xml" || h.Get("Cache-Control") != "private, no-store" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers = %v", h)
	}
	if cd := h.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, inv.Number()+"-xrechnung-cii.xml") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if h.Get("X-EInvoice-Validation") != "passed" {
		t.Errorf("X-EInvoice-Validation = %q", h.Get("X-EInvoice-Validation"))
	}
	if !strings.Contains(rec.Body.String(), "CrossIndustryInvoice") {
		t.Errorf("body is not CII: %.80s", rec.Body.String())
	}

	pdf := hs.get("/" + inv.ID.String() + "/einvoice?format=facturx")
	if pdf.Code != http.StatusOK || pdf.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("facturx = %d %s", pdf.Code, pdf.Header().Get("Content-Type"))
	}
}

func TestInvoiceHandler_EInvoiceErrors(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(map[string]any{"buyer_reference": ""})
	draft := hs.createDraft("")

	body := func(rec *httptest.ResponseRecorder) map[string]any {
		var m map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return m
	}
	cases := []struct {
		name, path string
		code       int
		err        string
	}{
		{"unknown format", "/" + inv.ID.String() + "/einvoice?format=pdf", http.StatusBadRequest, "validation_failed"},
		{"missing format", "/" + inv.ID.String() + "/einvoice", http.StatusBadRequest, "validation_failed"},
		{"draft", "/" + draft.ID.String() + "/einvoice?format=facturx", http.StatusConflict, "bad_state"},
		{"unknown invoice", "/" + uuid.NewString() + "/einvoice?format=facturx", http.StatusNotFound, "not_found"},
		{"not exportable", "/" + inv.ID.String() + "/einvoice?format=xrechnung-ubl", http.StatusUnprocessableEntity, "not_exportable"},
	}
	for _, tc := range cases {
		rec := hs.get(tc.path)
		if rec.Code != tc.code || body(rec)["error"] != tc.err {
			t.Errorf("%s: %d %v, want %d %s", tc.name, rec.Code, body(rec), tc.code, tc.err)
		}
	}

	rec := hs.get("/" + inv.ID.String() + "/einvoice?format=xrechnung-ubl")
	problems, _ := body(rec)["problems"].([]any)
	if len(problems) == 0 || problems[0].(map[string]any)["field"] != "buyer_reference" {
		t.Errorf("problems = %v", body(rec)["problems"])
	}

	// Outages and a generator that rejects our document are not the user's fault.
	hs.gen.err = einvoice.ErrUnavailable
	if rec := hs.get("/" + inv.ID.String() + "/einvoice?format=facturx"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("outage: %d", rec.Code)
	}
	hs.gen.err = &einvoice.GenerationError{Status: 400, Message: "Transformation failed."}
	rec = hs.get("/" + inv.ID.String() + "/einvoice?format=facturx")
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "Transformation") {
		t.Errorf("rejected document: %d %s (internals must not leak)", rec.Code, rec.Body.String())
	}
}

func TestInvoiceHandler_EInvoiceCheck(t *testing.T) {
	hs := newEInvoiceHarness(t)
	ready := hs.issued(nil)
	rec := hs.get("/" + ready.ID.String() + "/einvoice-check?format=xrechnung-cii")
	var got struct {
		Data struct {
			Format   string             `json:"format"`
			Ready    bool               `json:"ready"`
			Problems []einvoice.Problem `json:"problems"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || !got.Data.Ready || got.Data.Format != "xrechnung-cii" || got.Data.Problems == nil || len(got.Data.Problems) != 0 {
		t.Errorf("ready = %d %s", rec.Code, rec.Body.String())
	}

	notReady := hs.issued(map[string]any{"buyer_reference": ""})
	rec = hs.get("/" + notReady.ID.String() + "/einvoice-check?format=xrechnung-cii")
	got.Data.Problems = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || got.Data.Ready || len(got.Data.Problems) == 0 {
		t.Errorf("not ready = %d %s", rec.Code, rec.Body.String())
	}

	if rec := hs.get("/" + ready.ID.String() + "/einvoice-check"); rec.Code != http.StatusBadRequest {
		t.Errorf("missing format: %d", rec.Code)
	}
	hs.gen.err = einvoice.ErrUnavailable
	if rec := hs.get("/" + ready.ID.String() + "/einvoice-check?format=facturx"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("outage: %d", rec.Code)
	}
}

func TestInvoiceHandler_EInvoiceRoutesAreGetOnly(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(nil)
	for _, p := range []string{"/einvoice", "/einvoice-check"} {
		if code, _ := hs.do(http.MethodPost, "/"+inv.ID.String()+p+"?format=facturx", nil); code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s: %d", p, code)
		}
	}
}
