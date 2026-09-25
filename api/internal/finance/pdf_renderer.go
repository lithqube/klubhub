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

	"github.com/klubhub/dj/api/internal/finance/tax"
)

//go:embed fonts/DejaVuSansCondensed.ttf
var dejaVuSansRegularTTF []byte

//go:embed fonts/DejaVuSansCondensed-Bold.ttf
var dejaVuSansBoldTTF []byte

// InvoicePDFData contains all data needed to render an invoice PDF.
type InvoicePDFData struct {
	Invoice *Invoice
	Lines   []*InvoiceLine
	// BillingProfile is the live profile, used for the supplier block only
	// when the invoice carries no snapshot (drafts).
	BillingProfile *BillingProfile
	Payments       []*Payment
	Document       *Document
	// CreditedInvoiceNumber is the number of the invoice a credit note
	// reverses (printed as the reference).
	CreditedInvoiceNumber string
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

func snapshotFromProfile(p *BillingProfile) BillingProfileSnapshot {
	return BillingProfileSnapshot{
		LegalName: p.LegalName, TradingName: p.TradingName, EntityKind: p.EntityKind,
		TaxID: p.TaxID, TaxIDKind: p.TaxIDKind, ContactEmail: p.ContactEmail, ContactPhone: p.ContactPhone,
		AddressLine1: p.AddressLine1, AddressLine2: p.AddressLine2, AddressCity: p.AddressCity,
		AddressRegion: p.AddressRegion, AddressPostal: p.AddressPostal, AddressCountry: p.AddressCountry,
		PaymentInstructions: p.PaymentInstructions,
	}
}

// pdfRow is a label/value pair in the totals or details block.
type pdfRow struct {
	Label string
	Value string
	Bold  bool
}

// invoiceView is everything printed on the document, resolved from the
// data. Building it is pure so tests can assert the legal content without
// parsing the PDF.
type invoiceView struct {
	Title       string // INVOICE, CREDIT NOTE, or DRAFT INVOICE
	Number      string
	Reference   string // credit notes: the reversed invoice
	Details     []pdfRow
	Supplier    []string
	Customer    []string
	Lines       [][]string // #, description, qty, unit, VAT %, net
	Totals      []pdfRow
	TaxNote     string
	Payment     []pdfRow
	Instruction string
	Filename    string
}

// sign returns -1 for credit notes (amounts are stored positive; the kind
// carries the sign) and +1 otherwise.
func sign(inv *Invoice) int64 {
	if inv.Kind == InvoiceKindCreditNote {
		return -1
	}
	return 1
}

// formatBps renders basis points as a percentage without trailing zeros
// (1900 → "19%", 550 → "5.5%", 0 → "0%").
func formatBps(bps int64) string {
	s := fmt.Sprintf("%d.%02d", bps/100, bps%100)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".") + "%"
}

var treatmentLabels = map[tax.Treatment]string{
	tax.Domestic:      "Domestic VAT",
	tax.ReverseCharge: "Reverse charge",
	tax.Exempt:        "VAT exempt",
	tax.OutsideScope:  "Outside the scope of EU VAT",
	tax.USSalesTax:    "US sales tax",
	tax.None:          "No tax",
}

func buildInvoiceView(data *InvoicePDFData) invoiceView {
	inv := data.Invoice
	cur := inv.Currency
	sg := sign(inv)
	money := func(m int64) string { return formatMoney(cur, sg*m) }

	v := invoiceView{Title: "INVOICE", Number: inv.Number()}
	switch {
	case inv.Kind == InvoiceKindCreditNote:
		v.Title = "CREDIT NOTE"
		ref := data.CreditedInvoiceNumber
		if ref == "" && inv.CreditsInvoiceID != nil {
			ref = inv.CreditsInvoiceID.String()
		}
		if ref != "" {
			v.Reference = "Credit note for invoice " + ref
		}
	case inv.Status == InvoiceStatusDraft:
		v.Title = "DRAFT INVOICE"
	}
	if v.Number == "" {
		v.Number = "DRAFT — not a valid invoice"
	}

	date := func(t *time.Time) string {
		if t == nil {
			return "—"
		}
		return t.Format("2006-01-02")
	}
	supply := "—"
	if inv.SupplyDate != nil && *inv.SupplyDate != "" {
		supply = *inv.SupplyDate
	}
	v.Details = []pdfRow{
		{Label: "Issue date:", Value: date(inv.IssuedAt)},
		{Label: "Supply date:", Value: supply},
	}
	if inv.Kind != InvoiceKindCreditNote {
		v.Details = append(v.Details, pdfRow{Label: "Due date:", Value: date(inv.DueAt)})
	}
	v.Details = append(v.Details,
		pdfRow{Label: "Currency:", Value: cur},
		pdfRow{Label: "VAT treatment:", Value: treatmentLabels[inv.VATTreatment]})

	// Supplier: the snapshot taken at issue; drafts fall back to the live profile.
	var snap BillingProfileSnapshot
	haveSnap := len(inv.BillingProfile) > 0 && snap.FromJSON(inv.BillingProfile) == nil && snap.LegalName != ""
	if !haveSnap && data.BillingProfile != nil {
		snap, haveSnap = snapshotFromProfile(data.BillingProfile), true
	}
	if haveSnap {
		v.Supplier = addressLines(snap.LegalName, snap.TradingName, snap.AddressLine1, snap.AddressLine2,
			snap.AddressPostal, snap.AddressCity, snap.AddressRegion, snap.AddressCountry)
		if snap.TaxID != "" {
			label := "Tax ID"
			if snap.TaxIDKind == TaxIDKindVAT {
				label = "VAT ID"
			}
			v.Supplier = append(v.Supplier, label+": "+snap.TaxID)
		}
		if snap.ContactEmail != "" {
			v.Supplier = append(v.Supplier, snap.ContactEmail)
		}
		v.Instruction = snap.PaymentInstructions
	} else {
		v.Supplier = []string{"[Billing profile not set]"}
	}

	c := inv.Customer
	if c.LegalName == "" && c.Company == "" && c.AddressLine1 == "" {
		v.Customer = []string{"[Customer not set]"}
	} else {
		company := c.Company
		if company == c.LegalName {
			company = ""
		}
		v.Customer = addressLines(c.LegalName, company, c.AddressLine1, c.AddressLine2,
			c.PostalCode, c.City, c.Region, c.Country)
		if c.VATID != "" {
			v.Customer = append(v.Customer, "VAT ID: "+c.VATID)
		}
		if c.TaxID != "" {
			v.Customer = append(v.Customer, "Tax ID: "+c.TaxID)
		}
	}

	for i, l := range data.Lines {
		v.Lines = append(v.Lines, []string{
			fmt.Sprintf("%d", i+1), truncate(l.Description, 50), fmt.Sprintf("%d", l.Quantity),
			money(l.UnitMinor), formatBps(l.TaxBps), money(l.LineTotalMinor),
		})
	}

	v.Totals = []pdfRow{{Label: "Subtotal", Value: money(inv.SubtotalMinor)}}
	for _, b := range inv.TaxBreakdown {
		v.Totals = append(v.Totals, pdfRow{
			Label: fmt.Sprintf("VAT %s on %s", formatBps(b.RateBps), money(b.TaxableMinor)),
			Value: money(b.TaxMinor),
		})
	}
	v.Totals = append(v.Totals, pdfRow{Label: "Total", Value: money(inv.TotalMinor), Bold: true})
	if inv.WithholdingRateBps > 0 || inv.WithholdingMinor > 0 {
		v.Totals = append(v.Totals,
			pdfRow{Label: "Withholding tax " + formatBps(inv.WithholdingRateBps), Value: money(-inv.WithholdingMinor)},
			pdfRow{Label: "Net payable", Value: money(inv.NetPayableMinor), Bold: true})
	}
	v.TaxNote = inv.TaxNote

	if inv.Kind != InvoiceKindCreditNote {
		received := inv.ReceivedMinor
		if received == 0 {
			for _, p := range data.Payments {
				if p.Status != PaymentStatusCompleted {
					continue
				}
				if p.Kind == PaymentKindRefund {
					received -= p.AmountMinor
				} else {
					received += p.AmountMinor
				}
			}
		}
		outstanding := inv.NetPayableMinor - received
		if outstanding < 0 || inv.Status == InvoiceStatusPaid {
			outstanding = 0
		}
		v.Payment = []pdfRow{
			{Label: "Amount due:", Value: money(inv.NetPayableMinor), Bold: true},
			{Label: "Received:", Value: money(received)},
			{Label: "Outstanding:", Value: money(outstanding), Bold: true},
		}
	}

	kind := "invoice"
	if inv.Kind == InvoiceKindCreditNote {
		kind = "credit-note"
	}
	name := inv.Number()
	if name == "" {
		name = "draft-" + inv.ID.String()[:8]
	}
	v.Filename = fmt.Sprintf("%s-%s.pdf", kind, strings.ReplaceAll(name, "/", "-"))
	return v
}

// addressLines formats a postal block, skipping empty parts.
func addressLines(name, company, line1, line2, postal, city, region, country string) []string {
	var out []string
	for _, s := range []string{name, company, line1, line2} {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	if cityLine := strings.Join(nonEmpty(postal, city, region), " "); cityLine != "" {
		out = append(out, cityLine)
	}
	if country != "" {
		out = append(out, country)
	}
	return out
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// PDFRenderer generates PDF documents for invoices, agreements, etc.
type PDFRenderer struct{}

// NewPDFRenderer creates a PDFRenderer.
func NewPDFRenderer() *PDFRenderer {
	return &PDFRenderer{}
}

// RenderInvoice generates an invoice or credit-note PDF.
// Returns the PDF bytes, filename, and content type.
func (r *PDFRenderer) RenderInvoice(ctx context.Context, data *InvoicePDFData) ([]byte, string, string, error) {
	if data == nil || data.Invoice == nil {
		return nil, "", "", fmt.Errorf("render invoice: no invoice")
	}
	v := buildInvoiceView(data)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	// Embedded DejaVu Sans so non-Latin1 characters render correctly. No
	// italic variants are embedded; those styles reuse the upright faces.
	pdf.AddUTF8FontFromBytes("DejaVu", "", dejaVuSansRegularTTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", dejaVuSansBoldTTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "I", dejaVuSansRegularTTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "BI", dejaVuSansBoldTTF)

	darkBlue := []int{0x1E, 0x3A, 0x5F}
	lightGray := []int{0xF0, 0xF0, 0xF0}
	white := []int{0xFF, 0xFF, 0xFF}
	black := []int{0x33, 0x33, 0x33}
	darkGray := []int{0x66, 0x66, 0x66}

	// --- HEADER ---
	pdf.SetFillColor(darkBlue[0], darkBlue[1], darkBlue[2])
	pdf.Rect(0, 0, 210, 35, "F")
	pdf.SetFont("DejaVu", "B", 24)
	pdf.SetTextColor(white[0], white[1], white[2])
	pdf.SetXY(20, 8)
	pdf.Cell(0, 12, "KLUBHUB")
	pdf.SetFont("DejaVu", "", 10)
	pdf.SetXY(20, 20)
	pdf.Cell(0, 6, "DJ Invoices & Agreements")

	pdf.SetFont("DejaVu", "B", 14)
	pdf.SetXY(110, 8)
	pdf.CellFormat(80, 8, v.Title, "", 0, "R", false, 0, "")
	pdf.SetFont("DejaVu", "", 10)
	pdf.SetXY(110, 16)
	pdf.CellFormat(80, 6, v.Number, "", 0, "R", false, 0, "")
	if v.Reference != "" {
		pdf.SetXY(90, 22)
		pdf.SetFont("DejaVu", "", 9)
		pdf.CellFormat(100, 6, v.Reference, "", 0, "R", false, 0, "")
	}

	section := func(left, right string) {
		pdf.SetFillColor(lightGray[0], lightGray[1], lightGray[2])
		pdf.Rect(20, pdf.GetY(), 170, 8, "F")
		pdf.SetFont("DejaVu", "B", 10)
		pdf.SetTextColor(darkBlue[0], darkBlue[1], darkBlue[2])
		pdf.SetXY(20, pdf.GetY()+2)
		pdf.Cell(85, 5, left)
		pdf.Cell(85, 5, right)
		pdf.Ln(8)
		pdf.SetTextColor(black[0], black[1], black[2])
	}
	column := func(x float64, lines []string) float64 {
		for i, l := range lines {
			pdf.SetX(x)
			if i == 0 {
				pdf.SetFont("DejaVu", "B", 10)
			} else {
				pdf.SetFont("DejaVu", "", 9)
			}
			pdf.Cell(85, 5, truncate(l, 60))
			pdf.Ln(5)
		}
		return pdf.GetY()
	}

	// --- FROM / BILL TO ---
	pdf.SetY(45)
	section("FROM", "BILL TO")
	top := pdf.GetY()
	yFrom := column(20, v.Supplier)
	pdf.SetY(top)
	yTo := column(105, v.Customer)
	pdf.SetY(max(yFrom, yTo) + 5)

	// --- DETAILS / PAYMENT ---
	rightTitle := "PAYMENT"
	if len(v.Payment) == 0 {
		rightTitle = ""
	}
	section("DETAILS", rightTitle)
	top = pdf.GetY()
	rows := func(x float64, rs []pdfRow) float64 {
		for _, row := range rs {
			pdf.SetX(x)
			pdf.SetFont("DejaVu", "", 9)
			pdf.Cell(35, 5, row.Label)
			pdf.SetFont("DejaVu", "B", 9)
			pdf.Cell(50, 5, row.Value)
			pdf.Ln(6)
		}
		return pdf.GetY()
	}
	yLeft := rows(20, v.Details)
	pdf.SetY(top)
	yRight := rows(105, v.Payment)
	if v.Instruction != "" {
		pdf.SetX(105)
		pdf.SetFont("DejaVu", "", 8)
		pdf.MultiCell(85, 4, v.Instruction, "", "L", false)
		yRight = pdf.GetY()
	}
	pdf.SetY(max(yLeft, yRight) + 6)

	// --- LINE ITEMS ---
	pdf.SetFillColor(darkBlue[0], darkBlue[1], darkBlue[2])
	pdf.SetTextColor(white[0], white[1], white[2])
	pdf.SetFont("DejaVu", "B", 9)
	colWidths := []float64{10, 80, 15, 25, 15, 25}
	for i, h := range []string{"#", "Description", "Qty", "Unit", "VAT", "Net"} {
		pdf.CellFormat(colWidths[i], 8, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(8)
	pdf.SetTextColor(black[0], black[1], black[2])
	pdf.SetFont("DejaVu", "", 8)
	aligns := []string{"C", "L", "R", "R", "R", "R"}
	for idx, line := range v.Lines {
		if idx%2 == 0 {
			pdf.SetFillColor(lightGray[0], lightGray[1], lightGray[2])
		} else {
			pdf.SetFillColor(white[0], white[1], white[2])
		}
		for i, cell := range line {
			pdf.CellFormat(colWidths[i], 7, cell, "1", 0, aligns[i], true, 0, "")
		}
		pdf.Ln(7)
	}

	// --- TOTALS ---
	pdf.Ln(3)
	for _, row := range v.Totals {
		pdf.SetX(90)
		style := ""
		if row.Bold {
			style = "B"
		}
		pdf.SetFont("DejaVu", style, 9)
		pdf.CellFormat(70, 6, row.Label, "", 0, "R", false, 0, "")
		pdf.CellFormat(30, 6, row.Value, "", 0, "R", false, 0, "")
		pdf.Ln(6)
	}

	// --- LEGAL NOTE ---
	if v.TaxNote != "" {
		pdf.Ln(4)
		pdf.SetX(20)
		pdf.SetFont("DejaVu", "B", 9)
		pdf.MultiCell(170, 5, v.TaxNote, "", "L", false)
	}

	// --- FOOTER ---
	pdf.Ln(8)
	pdf.SetDrawColor(darkGray[0], darkGray[1], darkGray[2])
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(4)
	pdf.SetFont("DejaVu", "I", 8)
	pdf.SetTextColor(darkGray[0], darkGray[1], darkGray[2])
	pdf.Cell(0, 5, "KlubHub DJ — generated "+time.Now().UTC().Format("2006-01-02 15:04 UTC"))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, "", "", fmt.Errorf("pdf output: %w", err)
	}
	return buf.Bytes(), v.Filename, "application/pdf", nil
}

// formatMoney formats minor units (2 decimals) with a currency symbol,
// using integer arithmetic only. Negative amounts print as "-€12.00".
func formatMoney(currency string, minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	amount := fmt.Sprintf("%d.%02d", minor/100, minor%100)
	var s string
	switch currency {
	case "EUR":
		s = "€" + amount
	case "USD":
		s = "$" + amount
	case "GBP":
		s = "£" + amount
	default:
		s = amount + " " + currency
	}
	if neg {
		return "-" + s
	}
	return s
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
