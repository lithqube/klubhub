package tax

import (
	"strings"
	"testing"
)

// The German wording is bilingual (German / English) in one string: the
// customer's language is unknown, and the German phrase is the one the tax
// office and the customer's bookkeeper look for.
func TestNotes_GermanSupplier(t *testing.T) {
	cases := []struct {
		treatment Treatment
		want      []string // every fragment must appear
	}{
		{Exempt, []string{"Gemäß § 19 UStG wird keine Umsatzsteuer berechnet", "§ 19", "small-business"}},
		{ReverseCharge, []string{"Steuerschuldnerschaft des Leistungsempfängers", "Art. 196", "Reverse charge"}},
		{OutsideScope, []string{"Nicht steuerbar", "§ 3a Abs. 2 UStG", "place of supply"}},
	}
	for _, tc := range cases {
		got := Notes.Note("DE", tc.treatment)
		for _, frag := range tc.want {
			if !strings.Contains(got, frag) {
				t.Errorf("DE %s note %q lacks %q", tc.treatment, got, frag)
			}
		}
		if !strings.Contains(got, " / ") {
			t.Errorf("DE %s note %q should carry both languages, separated by \" / \"", tc.treatment, got)
		}
	}
}

func TestNotes_GermanSupplierLowercaseCountryAndNoNoteWhereNoneIsDue(t *testing.T) {
	if Notes.Note(" de ", Exempt) != Notes.Note("DE", Exempt) {
		t.Error("country lookup must ignore case and padding")
	}
	for _, tr := range []Treatment{Domestic, USSalesTax, None} {
		if n := Notes.Note("DE", tr); n != "" {
			t.Errorf("DE %s must carry no legal note, got %q", tr, n)
		}
	}
}

// Other countries keep the generic English wording until their own module
// is written; nothing here may invent legal text for them.
func TestNotes_OtherCountriesKeepTheGenericDefaults(t *testing.T) {
	for _, country := range []string{"AT", "FR", "NL", "US", ""} {
		if got := Notes.Note(country, Exempt); got != "VAT exempt: small business scheme." {
			t.Errorf("%q exempt = %q, want the generic default", country, got)
		}
		if got := Notes.Note(country, ReverseCharge); !strings.Contains(got, "Art. 196 Directive 2006/112/EC") || strings.Contains(got, "Steuerschuldnerschaft") {
			t.Errorf("%q reverse charge = %q, want the generic default", country, got)
		}
	}
}

func TestSuggest_UsesTheSuppliersCountryWording(t *testing.T) {
	de := Supplier{Country: "DE", VATID: "DE123456789", DefaultVATRateBps: 1900}

	rc := Suggest(de, Customer{Country: "FR", VATID: "FR12345678901", IsBusiness: true})
	if rc.VATTreatment != ReverseCharge || !strings.Contains(rc.TaxNote, "Steuerschuldnerschaft des Leistungsempfängers") {
		t.Errorf("DE→FR business = %+v", rc)
	}
	out := Suggest(de, Customer{Country: "CH", IsBusiness: true})
	if out.VATTreatment != OutsideScope || !strings.Contains(out.TaxNote, "§ 3a Abs. 2 UStG") {
		t.Errorf("DE→CH = %+v", out)
	}
	small := Suggest(Supplier{Country: "DE", VATExemptSmallBusiness: true}, Customer{Country: "DE"})
	if small.VATTreatment != Exempt || !strings.Contains(small.TaxNote, "§ 19 UStG") {
		t.Errorf("Kleinunternehmer = %+v", small)
	}
	if dom := Suggest(de, Customer{Country: "DE"}); dom.TaxNote != "" {
		t.Errorf("domestic VAT carries no note, got %q", dom.TaxNote)
	}

	// An Austrian supplier gets the generic wording, not German law.
	at := Suggest(Supplier{Country: "AT", VATID: "ATU12345678"}, Customer{Country: "FR", VATID: "FR12345678901", IsBusiness: true})
	if strings.Contains(at.TaxNote, "Steuerschuldnerschaft") {
		t.Errorf("AT supplier got German wording: %q", at.TaxNote)
	}
}

// Every note must fit the 1000-character tax_note column rule.
func TestNotes_FitTheNoteLengthLimit(t *testing.T) {
	for _, tr := range []Treatment{Exempt, ReverseCharge, OutsideScope} {
		for _, country := range []string{"DE", "AT"} {
			if n := len([]rune(Notes.Note(country, tr))); n > 1000 {
				t.Errorf("%s %s note is %d characters", country, tr, n)
			}
		}
	}
}

// NotesFor lists what a supplier country prints per treatment, for the UI.
func TestNotesFor(t *testing.T) {
	de := NotesFor("de")
	if len(de) != 3 || de[Exempt] == "" || de[ReverseCharge] == "" || de[OutsideScope] == "" {
		t.Fatalf("NotesFor(DE) = %v, want exempt, reverse_charge and outside_scope", de)
	}
	if de[Exempt] != Notes.Note("DE", Exempt) {
		t.Errorf("NotesFor must agree with Note: %q", de[Exempt])
	}
	if _, ok := de[Domestic]; ok {
		t.Error("treatments without a note are omitted")
	}
	at := NotesFor("AT")
	if at[Exempt] != "VAT exempt: small business scheme." {
		t.Errorf("AT falls back to the generic wording, got %q", at[Exempt])
	}
}
