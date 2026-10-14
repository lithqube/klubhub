// Package pdffont holds the Unicode TrueType fonts shared by the server-side
// PDF renderers (finance invoices, rider PDFs). fpdf's built-in core fonts
// only cover cp1252, so anything else (Cyrillic, Greek, Polish/Czech/Turkish
// letters, ...) used to be silently replaced; DejaVu Sans Condensed covers
// those. Scripts DejaVu does not contain (CJK, emoji) still cannot render.
//
// The font files live next to this package so //go:embed can reach them. See
// fonts/LICENSE-DejaVu.txt.
package pdffont

import (
	_ "embed"

	"codeberg.org/go-pdf/fpdf"
)

//go:embed fonts/DejaVuSansCondensed.ttf
var regularTTF []byte

//go:embed fonts/DejaVuSansCondensed-Bold.ttf
var boldTTF []byte

// Family is the font family name to pass to SetFont after Register.
const Family = "DejaVu"

// Register adds the DejaVu family to pdf. No italic faces are embedded, so
// the italic styles reuse the upright ones.
func Register(pdf *fpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes(Family, "", regularTTF)
	pdf.AddUTF8FontFromBytes(Family, "B", boldTTF)
	pdf.AddUTF8FontFromBytes(Family, "I", regularTTF)
	pdf.AddUTF8FontFromBytes(Family, "BI", boldTTF)
}
