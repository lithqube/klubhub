package finance

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeberg.org/go-pdf/fpdf"
	"github.com/google/uuid"
)

//go:embed fonts/DejaVuSansCondensed.ttf
var dejaVuSansRegularTTF []byte

//go:embed fonts/DejaVuSansCondensed-Bold.ttf
var dejaVuSansBoldTTF []byte

// InvoicePDFData contains all data needed to render an invoice PDF.
type InvoicePDFData struct {
	Invoice        *Invoice
	Lines          []*InvoiceLine
	BillingProfile *BillingProfile
	Payments       []*Payment
	Document       *Document
}

// BillingProfileSnapshot is the billing profile data snapshotted at invoice issuance.
type BillingProfileSnapshot struct {
	LegalName           string     `json:"legal_name"`
	TradingName         string     `json:"trading_name"`
	EntityKind          EntityKind `json:"entity_kind"`
	TaxID               string     `json:"tax_id"`
	TaxIDKind           TaxIDKind  `json:"tax_id_kind"`
	ContactEmail        string     `json:"contact_email"`
	ContactPhone        string     `json:"contact_phone"`
	AddressLine1        string     `json:"address_line1"`
	AddressLine2        string     `json:"address_line2"`
	AddressCity         string     `json:"address_city"`
	AddressRegion       string     `json:"address_region"`
	AddressPostal       string     `json:"address_postal"`
	AddressCountry      string     `json:"address_country"`
	PaymentInstructions string     `json:"payment_instructions"`
}

// FromJSON parses the JSON snapshot into the struct.
func (s *BillingProfileSnapshot) FromJSON(raw json.RawMessage) error {
	return json.Unmarshal(raw, s)
}

// PDFRenderer generates PDF documents for invoices, agreements, etc.
type PDFRenderer struct{}

// NewPDFRenderer creates a PDFRenderer.
func NewPDFRenderer() *PDFRenderer {
	return &PDFRenderer{}
}

// RenderInvoice generates a professional invoice PDF.
// Returns the PDF bytes, filename, and content type.
func (r *PDFRenderer) RenderInvoice(ctx context.Context, data *InvoicePDFData) ([]byte, string, string, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.AddPage()

	// Font setup: embed DejaVu Sans so non-Latin1 characters (accented
	// letters, currency symbols, etc.) render correctly. No italic/oblique
	// variants are embedded, so those styles fall back to the upright faces.
	pdf.AddUTF8FontFromBytes("DejaVu", "", dejaVuSansRegularTTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", dejaVuSansBoldTTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "I", dejaVuSansRegularTTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "BI", dejaVuSansBoldTTF)

	// Colors
	darkBlue := []int{0x1E, 0x3A, 0x5F}
	lightGray := []int{0xF0, 0xF0, 0xF0}
	white := []int{0xFF, 0xFF, 0xFF}
	black := []int{0x33, 0x33, 0x33}
	darkGray := []int{0x66, 0x66, 0x66}

	// --- HEADER ---
	// Top bar
	pdf.SetFillColor(darkBlue[0], darkBlue[1], darkBlue[2])
	pdf.Rect(0, 0, 210, 35, "F")

	// Logo placeholder / App name
	pdf.SetFont("DejaVu", "B", 24)
	pdf.SetTextColor(white[0], white[1], white[2])
	pdf.SetXY(20, 8)
	pdf.Cell(0, 12, "KLUBHUB")

	pdf.SetFont("DejaVu", "", 10)
	pdf.SetXY(20, 20)
	pdf.Cell(0, 6, "DJ Invoices & Agreements")

	// Invoice number and date on right
	pdf.SetFont("DejaVu", "B", 14)
	pdf.SetXY(130, 8)
	pdf.CellFormat(60, 8, "INVOICE", "", 0, "R", false, 0, "")

	pdf.SetFont("DejaVu", "", 10)
	pdf.SetXY(130, 16)
	invNum := data.Invoice.InvoiceNumber
	if invNum == "" {
		invNum = FormatInvoiceNumber(data.Invoice.NumberPrefix, data.Invoice.Currency, data.Invoice.NumberSeq)
	}
	pdf.CellFormat(60, 6, invNum, "", 0, "R", false, 0, "")

	pdf.SetXY(130, 22)
	issueDate := time.Now().UTC()
	if data.Invoice.IssuedAt != nil {
		issueDate = *data.Invoice.IssuedAt
	}
	pdf.CellFormat(60, 6, issueDate.Format("2006-01-02"), "", 0, "R", false, 0, "")

	// --- FROM / TO ---
	pdf.Ln(45)
	pdf.SetFillColor(lightGray[0], lightGray[1], lightGray[2])
	pdf.Rect(20, pdf.GetY(), 170, 8, "F")
	pdf.SetFont("DejaVu", "B", 10)
	pdf.SetTextColor(darkBlue[0], darkBlue[1], darkBlue[2])
	pdf.SetXY(20, pdf.GetY()+2)
	pdf.Cell(85, 5, "FROM")
	pdf.Cell(85, 5, "BILL TO")

	pdf.Ln(12)
	pdf.SetTextColor(black[0], black[1], black[2])
	pdf.SetFont("DejaVu", "B", 10)

	// FROM (billing profile)
	yFrom := pdf.GetY()
	pdf.SetX(20)
	if data.BillingProfile != nil {
		bp := data.BillingProfile
		pdf.Cell(85, 5, bp.LegalName)
		pdf.Ln(5)
		pdf.SetX(20)
		if bp.AddressLine1 != "" {
			pdf.Cell(85, 5, bp.AddressLine1)
			pdf.Ln(5)
		}
		if bp.AddressLine2 != "" {
			pdf.SetX(20)
			pdf.Cell(85, 5, bp.AddressLine2)
			pdf.Ln(5)
		}
		cityParts := []string{}
		if bp.AddressCity != "" {
			cityParts = append(cityParts, bp.AddressCity)
		}
		if bp.AddressRegion != "" {
			cityParts = append(cityParts, bp.AddressRegion)
		}
		if bp.AddressPostal != "" {
			cityParts = append(cityParts, bp.AddressPostal)
		}
		if len(cityParts) > 0 {
			pdf.SetX(20)
			pdf.Cell(85, 5, strings.Join(cityParts, " "))
			pdf.Ln(5)
		}
		if bp.AddressCountry != "" {
			pdf.SetX(20)
			pdf.Cell(85, 5, bp.AddressCountry)
			pdf.Ln(5)
		}
		if bp.TaxID != "" {
			pdf.SetX(20)
			pdf.SetFont("DejaVu", "", 9)
			pdf.Cell(85, 5, fmt.Sprintf("Tax ID: %s (%s)", bp.TaxID, bp.TaxIDKind))
			pdf.Ln(5)
		}
		pdf.SetFont("DejaVu", "B", 10)
	} else {
		pdf.Cell(85, 5, "[Billing profile not set]")
		pdf.Ln(5)
	}

	// TO (from billing profile snapshot on invoice)
	yTo := pdf.GetY()
	pdf.SetY(yFrom)
	pdf.SetX(105)
	if len(data.Invoice.BillingProfile) > 0 {
		var snap BillingProfileSnapshot
		if err := snap.FromJSON(data.Invoice.BillingProfile); err == nil {
			pdf.Cell(85, 5, snap.LegalName)
			pdf.Ln(5)
			pdf.SetX(105)
			if snap.AddressLine1 != "" {
				pdf.Cell(85, 5, snap.AddressLine1)
				pdf.Ln(5)
			}
			if snap.AddressLine2 != "" {
				pdf.SetX(105)
				pdf.Cell(85, 5, snap.AddressLine2)
				pdf.Ln(5)
			}
			cityParts := []string{}
			if snap.AddressCity != "" {
				cityParts = append(cityParts, snap.AddressCity)
			}
			if snap.AddressRegion != "" {
				cityParts = append(cityParts, snap.AddressRegion)
			}
			if snap.AddressPostal != "" {
				cityParts = append(cityParts, snap.AddressPostal)
			}
			if len(cityParts) > 0 {
				pdf.SetX(105)
				pdf.Cell(85, 5, strings.Join(cityParts, " "))
				pdf.Ln(5)
			}
			if snap.AddressCountry != "" {
				pdf.SetX(105)
				pdf.Cell(85, 5, snap.AddressCountry)
				pdf.Ln(5)
			}
			if snap.TaxID != "" {
				pdf.SetX(105)
				pdf.SetFont("DejaVu", "", 9)
				pdf.Cell(85, 5, fmt.Sprintf("Tax ID: %s (%s)", snap.TaxID, snap.TaxIDKind))
				pdf.Ln(5)
			}
			pdf.SetFont("DejaVu", "B", 10)
		}
	} else {
		pdf.Cell(85, 5, "[Counterparty not set]")
		pdf.Ln(5)
	}

	pdf.SetY(max(yFrom, pdf.GetY()) + 5)

	// --- INVOICE DETAILS ---
	pdf.SetFillColor(lightGray[0], lightGray[1], lightGray[2])
	pdf.Rect(20, pdf.GetY(), 170, 8, "F")
	pdf.SetFont("DejaVu", "B", 10)
	pdf.SetTextColor(darkBlue[0], darkBlue[1], darkBlue[2])
	pdf.SetXY(20, pdf.GetY()+2)
	pdf.Cell(85, 5, "INVOICE DETAILS")
	pdf.Cell(85, 5, "PAYMENT INFO")

	pdf.Ln(12)
	pdf.SetTextColor(black[0], black[1], black[2])
	pdf.SetFont("DejaVu", "", 9)

	// Left column: invoice details
	pdf.SetX(20)
	pdf.Cell(40, 5, "Status:")
	pdf.SetFont("DejaVu", "B", 9)
	pdf.Cell(45, 5, string(data.Invoice.Status))
	pdf.Ln(6)

	pdf.SetFont("DejaVu", "", 9)
	pdf.SetX(20)
	pdf.Cell(40, 5, "Currency:")
	pdf.SetFont("DejaVu", "B", 9)
	pdf.Cell(45, 5, data.Invoice.Currency)
	pdf.Ln(6)

	pdf.SetFont("DejaVu", "", 9)
	pdf.SetX(20)
	pdf.Cell(40, 5, "Issue Date:")
	pdf.SetFont("DejaVu", "B", 9)
	if data.Invoice.IssuedAt != nil {
		pdf.Cell(45, 5, data.Invoice.IssuedAt.Format("2006-01-02"))
	} else {
		pdf.Cell(45, 5, "—")
	}
	pdf.Ln(6)

	pdf.SetFont("DejaVu", "", 9)
	pdf.SetX(20)
	pdf.Cell(40, 5, "Due Date:")
	pdf.SetFont("DejaVu", "B", 9)
	if data.Invoice.DueAt != nil {
		pdf.Cell(45, 5, data.Invoice.DueAt.Format("2006-01-02"))
	} else {
		pdf.Cell(45, 5, "—")
	}
	pdf.Ln(6)

	if data.Invoice.GigID != uuid.Nil {
		pdf.SetFont("DejaVu", "", 9)
		pdf.SetX(20)
		pdf.Cell(40, 5, "Gig ID:")
		pdf.SetFont("DejaVu", "B", 9)
		pdf.Cell(45, 5, data.Invoice.GigID.String()[:8]+"...")
		pdf.Ln(6)
	}

	// Right column: payment info
	pdf.SetY(yTo)
	pdf.SetX(105)
	pdf.SetFont("DejaVu", "", 9)
	pdf.Cell(40, 5, "Total Due:")
	pdf.SetFont("DejaVu", "B", 9)
	pdf.Cell(45, 5, formatMoney(data.Invoice.Currency, data.Invoice.TotalMinor))
	pdf.Ln(6)

	pdf.SetFont("DejaVu", "", 9)
	pdf.SetX(105)
	pdf.Cell(40, 5, "Paid:")
	pdf.SetFont("DejaVu", "B", 9)
	// Paid amount would come from payments sum; for now show 0 for draft
	paidMinor := int64(0)
	for _, p := range data.Payments {
		if p.Status == PaymentStatusCompleted {
			paidMinor += p.AmountMinor
		}
	}
	pdf.Cell(45, 5, formatMoney(data.Invoice.Currency, paidMinor))
	pdf.Ln(6)

	pdf.SetFont("DejaVu", "", 9)
	pdf.SetX(105)
	pdf.Cell(40, 5, "Outstanding:")
	pdf.SetFont("DejaVu", "B", 9)
	pdf.Cell(45, 5, formatMoney(data.Invoice.Currency, data.Invoice.TotalMinor-paidMinor))
	pdf.Ln(6)

	if data.BillingProfile != nil && data.BillingProfile.PaymentInstructions != "" {
		pdf.SetFont("DejaVu", "", 9)
		pdf.SetX(105)
		pdf.Cell(40, 5, "Instructions:")
		pdf.SetFont("DejaVu", "B", 9)
		pdf.MultiCell(45, 5, data.BillingProfile.PaymentInstructions, "", "L", false)
	}

	pdf.Ln(8)

	// --- LINE ITEMS TABLE ---
	// Header
	pdf.SetFillColor(darkBlue[0], darkBlue[1], darkBlue[2])
	pdf.SetTextColor(white[0], white[1], white[2])
	pdf.SetFont("DejaVu", "B", 9)

	colWidths := []float64{10, 80, 20, 20, 20, 20}
	headers := []string{"#", "Description", "Qty", "Unit Price", "Tax %", "Total"}

	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 8, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(8)

	// Rows
	pdf.SetTextColor(black[0], black[1], black[2])
	pdf.SetFont("DejaVu", "", 8)

	for idx, line := range data.Lines {
		// Alternate row color
		if idx%2 == 0 {
			pdf.SetFillColor(lightGray[0], lightGray[1], lightGray[2])
		} else {
			pdf.SetFillColor(white[0], white[1], white[2])
		}

		pdf.CellFormat(colWidths[0], 7, fmt.Sprintf("%d", idx+1), "1", 0, "C", true, 0, "")
		pdf.CellFormat(colWidths[1], 7, truncate(line.Description, 50), "1", 0, "L", true, 0, "")
		pdf.CellFormat(colWidths[2], 7, fmt.Sprintf("%.2f", float64(line.Quantity)), "1", 0, "R", true, 0, "")
		pdf.CellFormat(colWidths[3], 7, formatMoney(data.Invoice.Currency, line.UnitMinor), "1", 0, "R", true, 0, "")
		taxPercent := float64(line.TaxBps) / 100.0
		pdf.CellFormat(colWidths[4], 7, fmt.Sprintf("%.2f%%", taxPercent), "1", 0, "R", true, 0, "")
		pdf.CellFormat(colWidths[5], 7, formatMoney(data.Invoice.Currency, line.LineTotalMinor), "1", 0, "R", true, 0, "")
		pdf.Ln(7)
	}

	// Totals row
	pdf.SetFillColor(darkBlue[0], darkBlue[1], darkBlue[2])
	pdf.SetTextColor(white[0], white[1], white[2])
	pdf.SetFont("DejaVu", "B", 9)
	pdf.CellFormat(colWidths[0]+colWidths[1]+colWidths[2]+colWidths[3]+colWidths[4], 8, "TOTAL", "1", 0, "R", true, 0, "")
	pdf.CellFormat(colWidths[5], 8, formatMoney(data.Invoice.Currency, data.Invoice.TotalMinor), "1", 0, "R", true, 0, "")
	pdf.Ln(8)

	// Tax breakdown if any tax
	if data.Invoice.TaxMinor > 0 {
		pdf.SetTextColor(black[0], black[1], black[2])
		pdf.SetFont("DejaVu", "", 9)
		pdf.SetX(130)
		pdf.Cell(40, 5, "Subtotal:")
		pdf.SetFont("DejaVu", "B", 9)
		pdf.Cell(20, 5, formatMoney(data.Invoice.Currency, data.Invoice.SubtotalMinor))
		pdf.Ln(6)

		pdf.SetFont("DejaVu", "", 9)
		pdf.SetX(130)
		pdf.Cell(40, 5, "Tax Total:")
		pdf.SetFont("DejaVu", "B", 9)
		pdf.Cell(20, 5, formatMoney(data.Invoice.Currency, data.Invoice.TaxMinor))
		pdf.Ln(6)
	}

	// --- PAYMENTS ---
	if len(data.Payments) > 0 {
		pdf.Ln(5)
		pdf.SetFillColor(lightGray[0], lightGray[1], lightGray[2])
		pdf.Rect(20, pdf.GetY(), 170, 8, "F")
		pdf.SetFont("DejaVu", "B", 10)
		pdf.SetTextColor(darkBlue[0], darkBlue[1], darkBlue[2])
		pdf.SetXY(20, pdf.GetY()+2)
		pdf.Cell(170, 5, "PAYMENTS RECEIVED")

		pdf.Ln(10)
		pdf.SetTextColor(black[0], black[1], black[2])
		pdf.SetFont("DejaVu", "B", 8)

		payColWidths := []float64{25, 25, 25, 25, 30, 40}
		payHeaders := []string{"Date", "Kind", "Method", "Status", "Amount", "Reference"}

		for i, h := range payHeaders {
			pdf.CellFormat(payColWidths[i], 7, h, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(7)

		pdf.SetFont("DejaVu", "", 8)
		for _, p := range data.Payments {
			if p.Status != PaymentStatusCompleted {
				continue
			}
			pdf.CellFormat(payColWidths[0], 7, p.ReceivedAt.Format("2006-01-02"), "1", 0, "C", false, 0, "")
			pdf.CellFormat(payColWidths[1], 7, string(p.Kind), "1", 0, "C", false, 0, "")
			pdf.CellFormat(payColWidths[2], 7, p.Method, "1", 0, "C", false, 0, "")
			pdf.CellFormat(payColWidths[3], 7, string(p.Status), "1", 0, "C", false, 0, "")
			pdf.CellFormat(payColWidths[4], 7, formatMoney(p.Currency, p.AmountMinor), "1", 0, "R", false, 0, "")
			pdf.CellFormat(payColWidths[5], 7, p.Reference, "1", 0, "L", false, 0, "")
			pdf.Ln(7)
		}
	}

	// --- FOOTER ---
	pdf.Ln(10)
	pdf.SetDrawColor(darkGray[0], darkGray[1], darkGray[2])
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(5)

	pdf.SetFont("DejaVu", "I", 8)
	pdf.SetTextColor(darkGray[0], darkGray[1], darkGray[2])
	pdf.Cell(0, 5, "Thank you for your business. Payment terms: net 30 days unless otherwise agreed.")
	pdf.Ln(5)
	pdf.Cell(0, 5, "KlubHub DJ — Automated invoice generated on "+time.Now().UTC().Format("2006-01-02 15:04 UTC"))

	// Output to bytes
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, "", "", fmt.Errorf("pdf output: %w", err)
	}

	filename := fmt.Sprintf("invoice-%s.pdf", strings.ReplaceAll(invNum, "/", "-"))
	return buf.Bytes(), filename, "application/pdf", nil
}

// formatMoney formats minor units as major with currency symbol.
func formatMoney(currency string, minor int64) string {
	major := float64(minor) / 100.0
	switch currency {
	case "EUR":
		return fmt.Sprintf("€%.2f", major)
	case "USD":
		return fmt.Sprintf("$%.2f", major)
	case "GBP":
		return fmt.Sprintf("£%.2f", major)
	default:
		return fmt.Sprintf("%.2f %s", major, currency)
	}
}

// truncate truncates a string to maxLen runes, appending "..." if it was cut.
// It operates on runes (not bytes) so multi-byte UTF-8 characters are never
// split mid-sequence.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
