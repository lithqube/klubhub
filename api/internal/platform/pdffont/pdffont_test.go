package pdffont

import (
	"bytes"
	"testing"

	"codeberg.org/go-pdf/fpdf"
)

// The embedded faces must load, every style must be selectable after
// Register, and non-Latin-1 text must render without an fpdf error.
func TestRegister_AllStylesUsable_AndRendersNonLatin1(t *testing.T) {
	f := fpdf.New("P", "mm", "A4", "")
	Register(f)
	f.AddPage()
	for _, style := range []string{"", "B", "I", "BI"} {
		f.SetFont(Family, style, 12)
		f.MultiCell(0, 6, "Тест Δοκιμή Zażółć € "+style, "", "L", false)
	}
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if f.Err() {
		t.Fatalf("fpdf error: %v", f.Error())
	}
	if !bytes.Contains(buf.Bytes(), []byte("/FontFile2")) {
		t.Fatal("expected an embedded TrueType font program in the PDF")
	}
	if bytes.Contains(buf.Bytes(), []byte("Helvetica")) {
		t.Fatal("a core font was used instead of the embedded one")
	}
}

func TestEmbeddedFontsAreNotEmpty(t *testing.T) {
	if len(regularTTF) < 100_000 || len(boldTTF) < 100_000 {
		t.Fatalf("embedded fonts look truncated: regular=%d bold=%d bytes", len(regularTTF), len(boldTTF))
	}
}
