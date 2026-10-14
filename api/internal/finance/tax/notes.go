package tax

import "strings"

// NoteKey identifies a country-specific legal note.
type NoteKey struct {
	Country   string // supplier country, ISO alpha-2 uppercase
	Treatment Treatment
}

// NoteRegistry resolves the legal wording printed on an invoice for a
// treatment. Country-specific wording (e.g. DE §19 UStG for exempt) takes
// precedence over the treatment default. Registration is meant for package
// init; lookups are read-only and safe for concurrent use afterwards.
type NoteRegistry struct {
	byCountry   map[NoteKey]string
	byTreatment map[Treatment]string
}

// NewNoteRegistry returns a registry pre-loaded with the default notes.
func NewNoteRegistry() *NoteRegistry {
	return &NoteRegistry{
		byCountry: map[NoteKey]string{},
		byTreatment: map[Treatment]string{
			ReverseCharge: "Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC).",
			Exempt:        "VAT exempt: small business scheme.",
			OutsideScope:  "Outside the scope of EU VAT: place of supply outside the EU.",
		},
	}
}

// Register adds country-specific wording for (country, treatment).
func (r *NoteRegistry) Register(country string, t Treatment, note string) {
	r.byCountry[NoteKey{Country: strings.ToUpper(strings.TrimSpace(country)), Treatment: t}] = note
}

// Note returns the wording for (country, treatment): the country-specific
// entry if registered, else the treatment default, else "".
func (r *NoteRegistry) Note(country string, t Treatment) string {
	if n, ok := r.byCountry[NoteKey{Country: strings.ToUpper(strings.TrimSpace(country)), Treatment: t}]; ok {
		return n
	}
	return r.byTreatment[t]
}

// notedTreatments are the treatments that can carry a legal note; the others
// (domestic VAT, US sales tax, none) print none.
var notedTreatments = []Treatment{ReverseCharge, Exempt, OutsideScope}

// NotesFor returns the legal wording a supplier in country prints for each
// treatment that has one, so a client can swap the note when the user changes
// the treatment without carrying its own copy of the text.
func NotesFor(country string) map[Treatment]string {
	out := make(map[Treatment]string, len(notedTreatments))
	for _, t := range notedTreatments {
		if n := Notes.Note(country, t); n != "" {
			out[t] = n
		}
	}
	return out
}

// Notes is the process-wide registry used by Suggest. Add country-specific
// notes here (docs/INVOICING.md §6), e.g. in an init():
//
//	Notes.Register("DE", Exempt, "Gemäß § 19 UStG wird keine Umsatzsteuer berechnet.")
var Notes = NewNoteRegistry()
