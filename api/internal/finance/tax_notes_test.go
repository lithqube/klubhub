package finance

import (
	"net/http"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/finance/tax"
)

type taxNotesBody struct {
	Data struct {
		Country string                   `json:"country"`
		Notes   map[tax.Treatment]string `json:"notes"`
	} `json:"data"`
}

func (hs *invoiceHarness) taxNotes() (int, taxNotesBody) {
	hs.t.Helper()
	code, out := hs.do(http.MethodGet, "/tax-notes", nil)
	var body taxNotesBody
	if out != nil {
		decode(hs.t, out, &body)
	}
	return code, body
}

// The wording a draft prints depends on the supplier's country; the UI asks
// for it once and swaps the note itself when the user changes the treatment.
func TestInvoiceHandler_TaxNotesForAGermanSupplier(t *testing.T) {
	hs := newInvoiceHarness(t) // the harness supplier is German
	code, body := hs.taxNotes()
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if body.Data.Country != "DE" {
		t.Errorf("country = %q", body.Data.Country)
	}
	for tr, frag := range map[tax.Treatment]string{
		tax.Exempt: "§ 19 UStG", tax.ReverseCharge: "Steuerschuldnerschaft des Leistungsempfängers", tax.OutsideScope: "§ 3a Abs. 2 UStG",
	} {
		if !strings.Contains(body.Data.Notes[tr], frag) {
			t.Errorf("%s note = %q, want it to mention %q", tr, body.Data.Notes[tr], frag)
		}
	}
	if _, ok := body.Data.Notes[tax.Domestic]; ok || len(body.Data.Notes) != 3 {
		t.Errorf("only treatments with a note are listed: %v", body.Data.Notes)
	}
}

func TestInvoiceHandler_TaxNotesFallBackToGenericWording(t *testing.T) {
	hs := newInvoiceHarness(t)
	hs.billing.profile.AddressCountry = "AT"
	_, body := hs.taxNotes()
	if body.Data.Country != "AT" || body.Data.Notes[tax.Exempt] != "VAT exempt: small business scheme." {
		t.Errorf("AT supplier = %+v", body.Data)
	}

	hs.billing.profile.AddressCountry = ""
	_, body = hs.taxNotes()
	if body.Data.Country != "" || body.Data.Notes[tax.Exempt] == "" {
		t.Errorf("no country still returns the generic wording: %+v", body.Data)
	}
}

func TestInvoiceHandler_TaxNotesRouteIsGetOnly(t *testing.T) {
	hs := newInvoiceHarness(t)
	if code, _ := hs.do(http.MethodPost, "/tax-notes", nil); code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d, want 405", code)
	}
}

// A new draft for a German supplier prints the German note for the treatment
// the rules pick, and the note on a draft is the same text the endpoint lists.
func TestInvoiceHandler_DraftNoteMatchesTheTaxNotesEndpoint(t *testing.T) {
	hs := newInvoiceHarness(t)
	_, notes := hs.taxNotes()
	inv := hs.createDraft(`{"gig_id":"` + hs.gigID.String() + `","vat_treatment":"reverse_charge",
		"customer":{"legal_name":"Paris Club SAS","address_line1":"1 Rue X","city":"Paris","country":"fr","vat_id":"FR12345678901","is_business":true}}`)
	if inv.TaxNote != notes.Data.Notes[tax.ReverseCharge] {
		t.Errorf("draft note %q differs from the endpoint's %q", inv.TaxNote, notes.Data.Notes[tax.ReverseCharge])
	}
}
