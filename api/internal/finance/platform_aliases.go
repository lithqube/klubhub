package finance

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/klubhub/dj/api/internal/platform/docstore"
	"github.com/klubhub/dj/api/internal/platform/email"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// Email transport and document storage moved to the shared platform packages
// (internal/platform/email, internal/platform/docstore) so the promoter
// binary can reuse them. These aliases keep every existing finance reference
// compiling unchanged: finance logic and tests keep using the unqualified
// names below.

// --- email (platform/email) ---

type (
	EmailMessage         = email.EmailMessage
	EmailStatus          = email.EmailStatus
	CreateEmailRequest   = email.CreateEmailRequest
	EmailAttachment      = email.EmailAttachment
	EmailSender          = email.EmailSender
	SMTPConfig           = email.SMTPConfig
	PlunkConfig          = email.PlunkConfig
	PlunkSender          = email.PlunkSender
	DefaultSMTPSender    = email.DefaultSMTPSender
	EmailService         = email.EmailService
	EmailRepository      = email.EmailRepository
	EmailRepositoryIface = email.EmailRepositoryIface
	AttachmentSource     = email.AttachmentSource
	AttachmentDoc        = email.AttachmentDoc
)

const (
	EmailStatusQueued  = email.EmailStatusQueued
	EmailStatusSending = email.EmailStatusSending
	EmailStatusSent    = email.EmailStatusSent
	EmailStatusFailed  = email.EmailStatusFailed
	EmailStatusBounced = email.EmailStatusBounced
)

var (
	ErrEmailNotFound   = email.ErrEmailNotFound
	ErrEmailConflict   = email.ErrEmailConflict
	ErrEmailValidation = email.ErrEmailValidation

	NewEmailService            = email.NewEmailService
	NewPlunkSender             = email.NewPlunkSender
	NewDefaultSMTPSender       = email.NewDefaultSMTPSender
	ValidateCreateEmailRequest = email.ValidateCreateEmailRequest
)

// NewEmailRepository is the DJ-side constructor: the platform repository
// takes a *tenantdb.DB, so finance wraps its pool (tenantScoped=false —
// plaintext columns, SQL byte-identical to the pre-extraction repository).
func NewEmailRepository(pool *pgxpool.Pool) *email.EmailRepository {
	return email.NewEmailRepository(tenantdb.New(pool), false)
}

// --- docstore (platform/docstore) ---

type (
	Document                = docstore.Document
	DocumentOwnerType       = docstore.DocumentOwnerType
	DocumentKind            = docstore.DocumentKind
	DocumentService         = docstore.DocumentService
	DocumentRepository      = docstore.DocumentRepository
	DocumentRepositoryIface = docstore.DocumentRepositoryIface
	CreateDocumentRequest   = docstore.CreateDocumentRequest
	CreateDocumentDetails   = docstore.CreateDocumentDetails
	ObjectStore             = docstore.ObjectStore
	StorageAdapter          = docstore.StorageAdapter
)

const (
	DocumentOwnerInvoice   = docstore.DocumentOwnerInvoice
	DocumentOwnerAgreement = docstore.DocumentOwnerAgreement
	DocumentOwnerEPK       = docstore.DocumentOwnerEPK
	DocumentOwnerGig       = docstore.DocumentOwnerGig
	DocumentOwnerOther     = docstore.DocumentOwnerOther

	DocumentKindGeneral          = docstore.DocumentKindGeneral
	DocumentKindInvoicePDF       = docstore.DocumentKindInvoicePDF
	DocumentKindEInvoiceXML      = docstore.DocumentKindEInvoiceXML
	DocumentKindValidationReport = docstore.DocumentKindValidationReport
	DocumentKindEPKPDF           = docstore.DocumentKindEPKPDF
)

var (
	ErrDocumentNotFound   = docstore.ErrDocumentNotFound
	ErrDocumentConflict   = docstore.ErrDocumentConflict
	ErrDocumentValidation = docstore.ErrDocumentValidation

	NewDocumentService            = docstore.NewDocumentService
	NewDocumentRepository         = docstore.NewDocumentRepository
	NewStorageAdapter             = docstore.NewStorageAdapter
	ValidateCreateDocumentRequest = docstore.ValidateCreateDocumentRequest
)
