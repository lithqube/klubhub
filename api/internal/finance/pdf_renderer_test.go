package finance

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestRenderInvoice_NonLatin1 ensures the embedded UTF-8 font path renders
// characters outside the cp1252 range (accents, diacritics, symbols) without
// error and produces a well-formed PDF.
func TestRenderInvoice_NonLatin1(t *testing.T) {
	renderer := NewPDFRenderer()

	inv := &Invoice{
		ID:            uuid.New(),
		GigID:         uuid.New(),
		InvoiceNumber: "INV-2026-0001",
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
