package email

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// EmailStatus enumerates the lifecycle states of an email message.
type EmailStatus string

const (
	EmailStatusQueued  EmailStatus = "queued"
	EmailStatusSending EmailStatus = "sending"
	EmailStatusSent    EmailStatus = "sent"
	EmailStatusFailed  EmailStatus = "failed"
	EmailStatusBounced EmailStatus = "bounced"
)

var validEmailStatuses = map[EmailStatus]struct{}{
	EmailStatusQueued:  {},
	EmailStatusSending: {},
	EmailStatusSent:    {},
	EmailStatusFailed:  {},
	EmailStatusBounced: {},
}

func (s EmailStatus) IsValid() bool {
	_, ok := validEmailStatuses[s]
	return ok
}

// EmailMessage is a row in email_messages.
type EmailMessage struct {
	ID            uuid.UUID   `json:"id"               db:"id"`
	Kind          EmailKind   `json:"kind"             db:"kind"`
	OwnerType     string      `json:"owner_type,omitempty" db:"owner_type"`
	OwnerID       *uuid.UUID  `json:"owner_id,omitempty"   db:"owner_id"`
	FromEmail     string      `json:"from_email"       db:"from_email"`
	FromName      string      `json:"from_name"        db:"from_name"`
	ToEmail       string      `json:"to_email"         db:"to_email"`
	CCEmails      []string    `json:"cc_emails"        db:"cc_emails"`
	BCCEmails     []string    `json:"bcc_emails"       db:"bcc_emails"`
	Subject       string      `json:"subject"          db:"subject"`
	Body          string      `json:"body"             db:"body"`
	BodyHTML      string      `json:"body_html,omitempty" db:"body_html"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids"  db:"attachment_ids"`
	// ReplyTo is where a reply should go when it is not the sender. Empty: none.
	ReplyTo string `json:"reply_to,omitempty" db:"reply_to"`
	// Attachments is the content behind AttachmentIDs, loaded just before
	// delivery. It is never stored or serialised: the row keeps only the ids.
	Attachments []EmailAttachment `json:"-" db:"-"`
	Status      EmailStatus       `json:"status"          db:"status"`
	SentAt      *time.Time        `json:"sent_at,omitempty" db:"sent_at"`
	LastError   string            `json:"last_error,omitempty" db:"last_error"`
	Attempts    int               `json:"attempts"        db:"attempts"`
	CreatedAt   time.Time         `json:"created_at"      db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"      db:"updated_at"`
}

var (
	ErrEmailNotFound   = errors.New("email not found")
	ErrEmailConflict   = errors.New("email updated by another writer")
	ErrEmailValidation = errors.New("invalid email")
)

// CreateEmailRequest is the body of POST /api/v1/finance/emails.
type CreateEmailRequest struct {
	Kind          EmailKind   `json:"kind"`
	OwnerType     string      `json:"owner_type,omitempty"`
	OwnerID       *uuid.UUID  `json:"owner_id,omitempty"`
	FromEmail     string      `json:"from_email"`
	FromName      string      `json:"from_name,omitempty"`
	ToEmail       string      `json:"to_email"`
	CCEmails      []string    `json:"cc_emails,omitempty"`
	BCCEmails     []string    `json:"bcc_emails,omitempty"`
	Subject       string      `json:"subject"`
	Body          string      `json:"body"`
	BodyHTML      string      `json:"body_html,omitempty"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids,omitempty"`
	ReplyTo       string      `json:"reply_to,omitempty"`
}

// EmailAttachment is one file sent with a message.
type EmailAttachment struct {
	Filename string
	MimeType string
	Content  []byte
}

// maxEmailAttachmentBytes caps what one message carries (the content, before
// base64 inflates it by a third). Plunk's default limit is 10 MB of base64.
const maxEmailAttachmentBytes = 7 << 20
