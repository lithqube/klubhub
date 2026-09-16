package gig

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"codeberg.org/go-pdf/fpdf"
)

// BookingPDFGenerator generates a booking confirmation PDF for a confirmed gig.
type BookingPDFGenerator struct {
	djName           string
	venueName        string
	city             string
	country          string
	eventDate        string
	feeAmount        string
	feeCurrency      string
	setLengthMinutes int
	techContactName  string
	techContactEmail string
}

// NewBookingPDFGenerator creates a generator from gig data.
func NewBookingPDFGenerator(gig *Gig, djName string) *BookingPDFGenerator {
	return &BookingPDFGenerator{
		djName:           djName,
		venueName:        gig.Venue,
		city:             gig.City,
		country:          gig.Country,
		eventDate:        gig.Date.Format("Monday, 2 January 2006"),
		feeAmount:        gig.FeeAmount.String(),
		feeCurrency:      gig.FeeCurrency,
		setLengthMinutes: gig.SetLengthMinutes,
		techContactName:  "", // Would come from linked venue record in future
		techContactEmail: "", // Would come from linked venue record in future
	}
}

// GenerateBookingPDF generates a booking confirmation PDF and returns the PDF bytes.
func GenerateBookingPDF(gig *Gig, djName string) ([]byte, error) {
	gen := NewBookingPDFGenerator(gig, djName)
	return gen.Generate()
}

// Generate creates the PDF and returns the byte slice.
func (g *BookingPDFGenerator) Generate() ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	f := fpdf.New("P", "mm", "A4", "")
	f.SetMargins(20, 20, 20)
	f.SetAutoPageBreak(true, 20)
	f.AddPage()

	// ── Header ──────────────────────────────────────────────────────────────
	f.SetFont("Helvetica", "B", 18)
	f.SetTextColor(26, 26, 26)
	f.MultiCell(0, 12, g.djName, "", "L", false)
	f.Ln(2)

	f.SetFont("Helvetica", "", 10)
	f.SetTextColor(80, 80, 80)
	f.MultiCell(0, 5, "BOOKING CONFIRMATION", "", "L", false)
	f.Ln(8)

	// Accent line
	f.SetDrawColor(150, 248, 255)
	f.SetLineWidth(0.8)
	f.Line(20, f.GetY(), 190, f.GetY())
	f.Ln(10)

	// ── Venue Section ──────────────────────────────────────────────────────
	g.writeSection(f, "VENUE")
	g.writeValue(f, g.venueName)
	if g.city != "" || g.country != "" {
		location := g.city
		if g.country != "" {
			if location != "" {
				location += ", " + g.country
			} else {
				location = g.country
			}
		}
		g.writeValue(f, location)
	}
	f.Ln(6)

	// ── Event Details Section ───────────────────────────────────────────────
	g.writeSection(f, "EVENT DETAILS")
	g.writeLabelValue(f, "Date:", g.eventDate)
	g.writeLabelValue(f, "Set Length:", fmt.Sprintf("%d minutes", g.setLengthMinutes))
	f.Ln(6)

	// ── Fee Section ─────────────────────────────────────────────────────────
	g.writeSection(f, "FEE")
	g.writeValue(f, fmt.Sprintf("%s %s", g.feeAmount, g.feeCurrency))
	f.Ln(6)

	// ── Technical Contact Section ─────────────────────────────────────────
	if g.techContactName != "" || g.techContactEmail != "" {
		g.writeSection(f, "TECHNICAL CONTACT")
		if g.techContactName != "" {
			g.writeValue(f, g.techContactName)
		}
		if g.techContactEmail != "" {
			g.writeValue(f, g.techContactEmail)
		}
	}

	_ = f.Output(buf)
	return buf.Bytes(), nil
}

// writeSection writes an ALL_CAPS section heading.
func (g *BookingPDFGenerator) writeSection(f *fpdf.Fpdf, title string) {
	f.SetFont("Helvetica", "B", 11)
	f.SetTextColor(150, 248, 255)
	f.MultiCell(0, 8, title, "", "L", false)
	f.SetDrawColor(150, 248, 255)
	f.SetLineWidth(0.3)
	f.Line(20, f.GetY(), 190, f.GetY())
	f.Ln(4)
}

// writeLabelValue writes a label: value pair on a single line.
func (g *BookingPDFGenerator) writeLabelValue(f *fpdf.Fpdf, label, value string) {
	f.SetFont("Helvetica", "", 10)
	f.SetTextColor(26, 26, 26)
	f.MultiCell(0, 5, fmt.Sprintf("%s %s", label, value), "", "L", false)
}

// writeValue writes a single value line (no label).
func (g *BookingPDFGenerator) writeValue(f *fpdf.Fpdf, value string) {
	f.SetFont("Helvetica", "", 10)
	f.SetTextColor(26, 26, 26)
	f.MultiCell(0, 5, value, "", "L", false)
}

// PDFStorageClientIface abstracts the storage client for PDF operations.
type PDFStorageClientIface interface {
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
}

// StoreBookingPDF stores a booking confirmation PDF in Garage via S3.
// Returns the storage path.
func StoreBookingPDF(ctx context.Context, gig *Gig, pdfData []byte, storage PDFStorageClientIface) (string, error) {
	bucket := "gig-exports"
	key := fmt.Sprintf("%d/%s.pdf", gig.Date.Year(), gig.ID.String())

	if storage != nil {
		err := storage.PutObject(ctx, bucket, key, bytes.NewReader(pdfData), int64(len(pdfData)), "application/pdf")
		if err != nil {
			return "", fmt.Errorf("storing booking PDF: %w", err)
		}
	}

	return key, nil
}
