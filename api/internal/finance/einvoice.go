package finance

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/einvoice"
	"github.com/klubhub/dj/api/internal/finance/tax"
)

// WithEInvoice enables structured e-invoice export (Factur-X, XRechnung).
// Without it the e-invoice routes answer "unavailable".
func (s *InvoiceService) WithEInvoice(e *einvoice.Exporter) *InvoiceService {
	s.exporter = e
	return s
}

// EInvoiceCheck is what GET /invoices/{id}/einvoice-check returns.
type EInvoiceCheck struct {
	Format   einvoice.Format    `json:"format"`
	Ready    bool               `json:"ready"`
	Problems []einvoice.Problem `json:"problems"`
}

// EInvoiceFile is a finished e-invoice with the name to download it under.
type EInvoiceFile struct {
	Data     []byte
	Filename string
	MimeType string
	Report   *einvoice.Report
}

// einvoiceSource loads an invoice for export. Only numbered documents have an
// e-invoice: a draft is not a document yet and a cancelled one never was.
func (s *InvoiceService) einvoiceSource(ctx context.Context, id uuid.UUID) (*InvoicePDFData, error) {
	data, err := s.pdfData(ctx, id)
	if err != nil {
		return nil, err
	}
	switch data.Invoice.Status {
	case InvoiceStatusIssued, InvoiceStatusPaid, InvoiceStatusCredited, InvoiceStatusCorrected:
		return data, nil
	}
	return nil, ErrInvoiceBadState
}

// EInvoiceCheck reports what stops the invoice from being exported as f.
func (s *InvoiceService) EInvoiceCheck(ctx context.Context, id uuid.UUID, f einvoice.Format) (*EInvoiceCheck, error) {
	data, err := s.einvoiceSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.exporter == nil {
		return nil, einvoice.ErrUnavailable
	}
	doc, problems := einvoiceDocument(data)
	if len(problems) == 0 {
		if problems, err = s.exporter.Check(ctx, doc, f); err != nil {
			return nil, err
		}
	} else {
		problems = append(problems, einvoice.Prepare(doc, f)...)
	}
	if problems == nil {
		problems = []einvoice.Problem{}
	}
	return &EInvoiceCheck{Format: f, Ready: len(problems) == 0, Problems: problems}, nil
}

// EInvoice produces the validated e-invoice. Nothing invalid is returned: a
// document that cannot be exported comes back as *einvoice.NotExportableError.
func (s *InvoiceService) EInvoice(ctx context.Context, id uuid.UUID, f einvoice.Format) (*EInvoiceFile, error) {
	data, err := s.einvoiceSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.exporter == nil {
		return nil, einvoice.ErrUnavailable
	}
	doc, problems := einvoiceDocument(data)
	if len(problems) > 0 {
		return nil, &einvoice.NotExportableError{Problems: append(problems, einvoice.Prepare(doc, f)...)}
	}

	var pdf []byte
	if f == einvoice.FacturX {
		if pdf, _, _, err = NewPDFRenderer().RenderInvoice(ctx, data); err != nil {
			return nil, err
		}
	}
	file, err := s.exporter.Export(ctx, doc, f, pdf)
	if err != nil {
		return nil, err
	}
	name := strings.ReplaceAll(data.Invoice.Number(), "/", "-") + "-" + string(f) + file.Extension
	return &EInvoiceFile{Data: file.Data, Filename: name, MimeType: file.MimeType, Report: file.Report}, nil
}

// einvoiceDocument maps a numbered invoice or credit note, with the supplier
// snapshot taken when it was issued, into an e-invoice document. The problems
// it returns are things the format cannot express at all, as opposed to
// missing data (einvoice.Prepare's job).
func einvoiceDocument(data *InvoicePDFData) (einvoice.Document, []einvoice.Problem) {
	inv := data.Invoice
	var problems []einvoice.Problem

	digits := currencyMinorDigits(inv.Currency)
	if digits > 2 {
		problems = append(problems, einvoice.Problem{Field: "currency",
			Message: fmt.Sprintf("%s has %d decimal places; e-invoices support currencies with up to two.", inv.Currency, digits)})
		digits = 2
	}
	// EN 16931 has no field for a deduction at source: exporting would give an
	// amount due that disagrees with the PDF.
	if inv.WithholdingRateBps > 0 || inv.WithholdingMinor > 0 {
		problems = append(problems, einvoice.Problem{Field: "withholding",
			Message: "This invoice deducts withholding tax, which an e-invoice cannot express (EN 16931 has no field for it), so it cannot be exported as one."})
	}

	var snap BillingProfileSnapshot
	if err := snap.FromJSON(inv.BillingProfile); err != nil || snap.LegalName == "" {
		problems = append(problems, einvoice.Problem{Field: "seller.name",
			Message: "This invoice has no supplier details from when it was issued."})
	}
	amt := func(minor int64) string { return decimalString(minor, digits) }

	doc := einvoice.Document{
		Number:           inv.Number(),
		TypeCode:         einvoice.TypeInvoice,
		Currency:         inv.Currency,
		IssueDate:        dateString(inv.IssuedAt),
		BuyerReference:   inv.BuyerReference,
		PurchaseOrderRef: inv.PurchaseOrderRef,
		ContractRef:      inv.ContractRef,
		PaymentTerms:     inv.PaymentTerms,
	}
	if inv.SupplyDate != nil {
		doc.DeliveryDate = *inv.SupplyDate
	}
	if inv.Kind == InvoiceKindCreditNote {
		doc.TypeCode = einvoice.TypeCreditNote
		doc.PrecedingNumber, doc.PrecedingIssueDate = data.CreditedInvoiceNumber, data.CreditedIssueDate
	} else {
		doc.DueDate = dateString(inv.DueAt)
	}

	seller := einvoice.Party{
		Name: snap.LegalName, TradingName: snap.TradingName,
		Street: snap.AddressLine1, Street2: snap.AddressLine2, City: snap.AddressCity, Postal: snap.AddressPostal,
		Region: snap.AddressRegion, Country: snap.AddressCountry,
		TaxNumber: snap.TaxNumber, Email: snap.ContactEmail, ContactName: snap.LegalName, Phone: snap.ContactPhone,
	}
	if snap.TaxIDKind == TaxIDKindVAT { // an EIN or ABN is not a VAT ID
		seller.VATID = snap.TaxID
	}
	doc.Seller = seller

	c := inv.Customer
	buyer := einvoice.Party{
		Name: c.LegalName, Street: c.AddressLine1, Street2: c.AddressLine2, City: c.City, Postal: c.PostalCode,
		Region: c.Region, Country: c.Country, VATID: c.VATID, Email: c.Email,
	}
	if buyer.Name == "" {
		buyer.Name = c.Company
	} else if c.Company != "" && c.Company != c.LegalName {
		buyer.TradingName = c.Company
	}
	doc.Buyer = buyer

	if snap.IBAN != "" {
		doc.Payment = &einvoice.Payment{IBAN: snap.IBAN, BIC: snap.BIC, Reference: inv.Number()}
	}

	for i, l := range data.Lines {
		doc.Lines = append(doc.Lines, einvoice.Line{
			ID: i + 1, Name: l.Description, Quantity: strconv.Itoa(l.Quantity), UnitCode: l.UnitCode,
			UnitPrice: amt(l.UnitMinor), Net: amt(l.LineTotalMinor),
			Category: einvoiceCategory(inv.VATTreatment, l.TaxBps, inv.TaxNote),
		})
	}
	for _, row := range inv.TaxBreakdown {
		doc.Taxes = append(doc.Taxes, einvoice.TaxLine{
			Category: einvoiceCategory(inv.VATTreatment, row.RateBps, inv.TaxNote),
			Taxable:  amt(row.TaxableMinor), Tax: amt(row.TaxMinor),
		})
	}
	doc.Totals = einvoice.Totals{
		Lines: amt(inv.SubtotalMinor), TaxExclusive: amt(inv.SubtotalMinor), Tax: amt(inv.TaxMinor),
		TaxInclusive: amt(inv.TotalMinor), Payable: amt(inv.TotalMinor),
	}
	return doc, problems
}

// einvoiceCategory maps the invoice's VAT treatment to its EN 16931 category.
// AE, E and O carry the invoice's legal note as the exemption reason.
func einvoiceCategory(t tax.Treatment, rateBps int64, note string) einvoice.Category {
	c := tax.CategoryFor(t, rateBps)
	out := einvoice.Category{Code: c.Code, ExemptionCode: c.ExemptionCode}
	switch c.Code {
	case "S":
		out.Percent, out.HasPercent = percentFromBps(rateBps), true
	case "Z", "AE", "E":
		out.Percent, out.HasPercent = "0", true
	}
	switch c.Code {
	case "AE", "E", "O":
		out.ExemptionReason = strings.TrimSpace(note)
	}
	return out
}

// dateString is YYYY-MM-DD, or "" for no date. It formats the timestamp as
// stored, exactly like the PDF does, so the two documents agree.
func dateString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// decimalString renders minor units with the currency's own decimals
// ("29750", "297.50"). Amounts are never negative here: credit notes are
// stored positive, as EN 16931 wants them.
func decimalString(minor int64, digits int) string {
	if digits == 0 {
		return strconv.FormatInt(minor, 10)
	}
	neg := minor < 0
	if neg {
		minor = -minor
	}
	s := fmt.Sprintf("%d.%02d", minor/100, minor%100)
	if neg {
		return "-" + s
	}
	return s
}

// percentFromBps prints basis points as a percentage without trailing zeros:
// 1900 -> "19", 550 -> "5.5", 725 -> "7.25".
func percentFromBps(bps int64) string {
	s := fmt.Sprintf("%d.%02d", bps/100, bps%100)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
