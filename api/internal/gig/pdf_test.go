package gig

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestNewBookingPDFGenerator(t *testing.T) {
	now := time.Now()
	gig := &Gig{
		ID:              uuid.New(),
		Date:            now,
		Venue:            "Berghain",
		City:            "Berlin",
		Country:         "DE",
		EventName:        "Saturday Night",
		FeeAmount:        decimal.NewFromInt(2000),
		FeeCurrency:      "EUR",
		SetLengthMinutes: 480,
		Status:          GigStatusConfirmed,
	}

	gen := NewBookingPDFGenerator(gig, "DJ Test")

	if gen.djName != "DJ Test" {
		t.Errorf("djName = %q, want %q", gen.djName, "DJ Test")
	}
	if gen.venueName != "Berghain" {
		t.Errorf("venueName = %q, want %q", gen.venueName, "Berghain")
	}
	if gen.city != "Berlin" {
		t.Errorf("city = %q, want %q", gen.city, "Berlin")
	}
	if gen.country != "DE" {
		t.Errorf("country = %q, want %q", gen.country, "DE")
	}
	if gen.feeAmount != "2000" {
		t.Errorf("feeAmount = %q, want %q", gen.feeAmount, "2000")
	}
	if gen.feeCurrency != "EUR" {
		t.Errorf("feeCurrency = %q, want %q", gen.feeCurrency, "EUR")
	}
	if gen.setLengthMinutes != 480 {
		t.Errorf("setLengthMinutes = %d, want %d", gen.setLengthMinutes, 480)
	}
}

func TestGenerateBookingPDF(t *testing.T) {
	now := time.Now()
	gig := &Gig{
		ID:              uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		Date:            now,
		Venue:            "Berghain",
		City:            "Berlin",
		Country:         "DE",
		EventName:        "Saturday Night",
		PromoterName:     "Promoter Name",
		PromoterEmail:    "promoter@example.com",
		FeeAmount:        decimal.NewFromInt(2000),
		FeeCurrency:      "EUR",
		SetLengthMinutes: 480,
		Status:          GigStatusConfirmed,
	}

	pdfData, err := GenerateBookingPDF(gig, "DJ Test")
	if err != nil {
		t.Fatalf("GenerateBookingPDF failed: %v", err)
	}

	if len(pdfData) == 0 {
		t.Error("GenerateBookingPDF returned empty data")
	}

	// Check for PDF magic bytes (%PDF-)
	if len(pdfData) < 4 {
		t.Error("PDF data too short to be valid")
	}
	pdfHeader := string(pdfData[:4])
	if pdfHeader != "%PDF" {
		t.Errorf("PDF header = %q, want %q", pdfHeader, "%PDF")
	}
}

func TestBookingPDFContent(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	gig := &Gig{
		ID:              uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		Date:            now,
		Venue:            "Berghain",
		City:            "Berlin",
		Country:         "DE",
		EventName:        "Saturday Night",
		FeeAmount:        decimal.NewFromInt(2000),
		FeeCurrency:      "EUR",
		SetLengthMinutes: 480,
		Status:          GigStatusConfirmed,
	}

	gen := NewBookingPDFGenerator(gig, "DJ Test")

	if gen.eventDate != "Friday, 15 May 2026" {
		t.Errorf("eventDate = %q, want %q", gen.eventDate, "Friday, 15 May 2026")
	}
}
