package finance

import "github.com/klubhub/dj/api/internal/platform/email"

// EmailKind enumerates the kinds of transactional emails finance sends.
// It aliases the platform kind so finance's kinds share the platform's open
// registry (platform/email registers no kinds itself; each consumer package
// registers its own in init()).
type EmailKind = email.EmailKind

// Finance email kinds.
const (
	EmailKindInvoiceIssued      EmailKind = "invoice_issued"
	EmailKindInvoicePaid        EmailKind = "invoice_paid"
	EmailKindInvoiceCancelled   EmailKind = "invoice_cancelled"
	EmailKindAgreementSent      EmailKind = "agreement_sent"
	EmailKindAgreementSigned    EmailKind = "agreement_signed"
	EmailKindAgreementCompleted EmailKind = "agreement_completed"
)

func init() {
	email.RegisterKind(EmailKindInvoiceIssued)
	email.RegisterKind(EmailKindInvoicePaid)
	email.RegisterKind(EmailKindInvoiceCancelled)
	email.RegisterKind(EmailKindAgreementSent)
	email.RegisterKind(EmailKindAgreementSigned)
	email.RegisterKind(EmailKindAgreementCompleted)
}
