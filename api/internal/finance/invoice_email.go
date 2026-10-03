package finance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/einvoice"
)

// ErrEmailUnavailable means invoices cannot be emailed on this server: no mail
// transport, no sender address or no document storage is configured.
var ErrEmailUnavailable = errors.New("invoice email is not available")

// EmailInvoiceRequest is the body of POST /invoices/{id}/email. Every field is
// optional: the recipient defaults to the customer's email, the subject and
// text to a short bilingual note, and the e-invoice is attached when there is
// one.
type EmailInvoiceRequest struct {
	ToEmail string `json:"to_email,omitempty"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`
	// IncludeEInvoice nil attaches the e-invoice XML when the invoice has one;
	// true insists on it (the send is refused if there is none); false sends
	// the PDF alone.
	IncludeEInvoice *bool `json:"include_einvoice,omitempty"`
}

// InvoiceEmailer sends an issued invoice to its customer.
type InvoiceEmailer interface {
	SendInvoiceEmail(ctx context.Context, id uuid.UUID, req EmailInvoiceRequest) (*EmailMessage, error)
}

// InvoiceMailer emails an issued invoice or credit note with its archived PDF
// and, when it has one, the validated e-invoice XML. It attaches only that
// document's own archived files.
type InvoiceMailer struct {
	invoices  *InvoiceService
	emails    *EmailService
	fromEmail string
	fromName  string
}

// NewInvoiceMailer wires the mailer. fromEmail must be a sender the mail
// transport accepts (for Plunk, a verified domain).
func NewInvoiceMailer(invoices *InvoiceService, emails *EmailService, fromEmail, fromName string) *InvoiceMailer {
	return &InvoiceMailer{invoices: invoices, emails: emails, fromEmail: fromEmail, fromName: fromName}
}

var _ InvoiceEmailer = (*InvoiceMailer)(nil)

// SendInvoiceEmail delivers the message and returns it with its final status.
// A delivery failure is not an error here: the message is stored as failed and
// the worker retries it with the same attachments.
func (m *InvoiceMailer) SendInvoiceEmail(ctx context.Context, id uuid.UUID, req EmailInvoiceRequest) (*EmailMessage, error) {
	if m.emails == nil || m.fromEmail == "" || m.invoices.archive == nil {
		return nil, ErrEmailUnavailable
	}
	data, err := m.invoices.einvoiceSource(ctx, id) // numbered documents only
	if err != nil {
		return nil, err
	}
	inv := data.Invoice

	to := strings.TrimSpace(req.ToEmail)
	if to == "" {
		to = strings.TrimSpace(inv.Customer.Email)
	}
	if to == "" {
		return nil, InvoiceValidationErrors{{Field: "to_email", Message: "the customer has no email address; enter one to send to"}}
	}

	// Make sure the archive is complete, then attach what it holds. A partial
	// failure here (say the generator is down) still leaves the PDF.
	if err := m.invoices.Archive(ctx, id); err != nil {
		slog.Warn("archive before emailing was incomplete", "invoice_id", id, "err", err)
	}
	docs, err := m.invoices.archive.ListByOwner(ctx, DocumentOwnerInvoice, id)
	if err != nil {
		return nil, err
	}
	current := map[DocumentKind]*Document{}
	for _, d := range docs {
		if d.IsCurrent {
			current[d.Kind] = d
		}
	}
	pdf := current[DocumentKindInvoicePDF]
	if pdf == nil {
		return nil, ErrEmailUnavailable // nothing to attach: storage is not working
	}
	attach := []uuid.UUID{pdf.ID}
	withXML := false
	switch xml := current[DocumentKindEInvoiceXML]; {
	case req.IncludeEInvoice != nil && !*req.IncludeEInvoice:
	case xml != nil:
		attach, withXML = append(attach, xml.ID), true
	case req.IncludeEInvoice != nil && *req.IncludeEInvoice:
		return nil, &einvoice.NotExportableError{Problems: []einvoice.Problem{{
			Field: "einvoice", Message: "This invoice has no e-invoice to attach. See the e-invoice check for what is missing.",
		}}}
	}

	subject, body := strings.TrimSpace(req.Subject), strings.TrimSpace(req.Body)
	if subject == "" || body == "" {
		defSubject, defBody := invoiceEmailText(data, withXML)
		if subject == "" {
			subject = defSubject
		}
		if body == "" {
			body = defBody
		}
	}

	ownerID := id
	return m.emails.SendOwned(ctx, CreateEmailRequest{
		Kind: EmailKindInvoiceIssued, OwnerType: string(DocumentOwnerInvoice), OwnerID: &ownerID,
		FromEmail: m.fromEmail, FromName: m.fromName, ToEmail: to, ReplyTo: replyToFor(data, m.fromEmail),
		Subject: subject, Body: body, AttachmentIDs: attach,
	})
}

// replyToFor is where the customer's answer should go: the supplier's contact
// email, so a reply reaches the DJ and not the platform's sending address. It
// comes from the supplier snapshot taken when the invoice was issued (the
// current profile for an invoice that has none), and is left out, never an
// error, when it is missing, malformed or the same as the sender.
func replyToFor(data *InvoicePDFData, fromEmail string) string {
	email := ""
	var snap BillingProfileSnapshot
	if err := snap.FromJSON(data.Invoice.BillingProfile); err == nil {
		email = snap.ContactEmail
	}
	if strings.TrimSpace(email) == "" && data.BillingProfile != nil {
		email = data.BillingProfile.ContactEmail
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || strings.EqualFold(addr.Address, fromEmail) {
		return ""
	}
	return addr.Address
}

// invoiceEmailText is the default subject and body: short, bilingual, with the
// supplier's contact details; replies are routed there by the reply-to header.
func invoiceEmailText(data *InvoicePDFData, withXML bool) (subject, body string) {
	inv := data.Invoice
	var snap BillingProfileSnapshot
	_ = snap.FromJSON(inv.BillingProfile)
	supplier := strings.TrimSpace(snap.TradingName)
	if supplier == "" {
		supplier = snap.LegalName
	}
	if supplier == "" {
		supplier = "us"
	}
	en, de := "invoice", "Rechnung"
	if inv.Kind == InvoiceKindCreditNote {
		en, de = "credit note", "Gutschrift"
	}
	number := inv.Number()
	amount := fmt.Sprintf("%s %s", decimalString(inv.TotalMinor, min(currencyMinorDigits(inv.Currency), 2)), inv.Currency)
	due := ""
	if inv.Kind == InvoiceKindInvoice {
		due = dateString(inv.DueAt)
	}

	subject = fmt.Sprintf("%s %s from %s / %s %s", strings.ToUpper(en[:1])+en[1:], number, supplier, de, number)

	var b strings.Builder
	fmt.Fprintf(&b, "Hello,\n\nplease find attached %s %s for %s", en, number, amount)
	if due != "" {
		fmt.Fprintf(&b, ", due %s", due)
	}
	b.WriteString(".\n")
	if withXML {
		b.WriteString("The structured e-invoice (XML, EN 16931) is attached as well, for automatic processing.\n")
	}
	fmt.Fprintf(&b, "\nGuten Tag,\n\nanbei erhalten Sie die %s %s über %s", de, number, amount)
	if due != "" {
		fmt.Fprintf(&b, ", fällig am %s", due)
	}
	b.WriteString(".\n")
	if withXML {
		b.WriteString("Die strukturierte E-Rechnung (XML, EN 16931) liegt zur automatischen Verarbeitung bei.\n")
	}
	fmt.Fprintf(&b, "\n%s", supplier)
	if snap.ContactEmail != "" {
		fmt.Fprintf(&b, "\n%s", snap.ContactEmail)
	}
	if snap.ContactPhone != "" {
		fmt.Fprintf(&b, "\n%s", snap.ContactPhone)
	}
	return subject, b.String()
}
