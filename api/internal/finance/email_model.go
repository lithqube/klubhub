package finance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EmailKind enumerates the kinds of transactional emails the system sends.
type EmailKind string

const (
	EmailKindInvoiceIssued   EmailKind = "invoice_issued"
	EmailKindInvoicePaid     EmailKind = "invoice_paid"
	EmailKindInvoiceCancelled EmailKind = "invoice_cancelled"
	EmailKindAgreementSent    EmailKind = "agreement_sent"
	EmailKindAgreementSigned  EmailKind = "agreement_signed"
	EmailKindAgreementCompleted EmailKind = "agreement_completed"
)

var validEmailKinds = map[EmailKind]struct{}{
	EmailKindInvoiceIssued:     {},
	EmailKindInvoicePaid:       {},
	EmailKindInvoiceCancelled:  {},
	EmailKindAgreementSent:     {},
	EmailKindAgreementSigned:   {},
	EmailKindAgreementCompleted: {},
}

func (k EmailKind) IsValid() bool {
	_, ok := validEmailKinds[k]
	return ok
}

// EmailStatus enumerates the lifecycle states of an email message.
type EmailStatus string

const (
	EmailStatusQueued   EmailStatus = "queued"
	EmailStatusSending  EmailStatus = "sending"
	EmailStatusSent     EmailStatus = "sent"
	EmailStatusFailed   EmailStatus = "failed"
	EmailStatusBounced  EmailStatus = "bounced"
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
	ID            uuid.UUID  `json:"id"               db:"id"`
	Kind          EmailKind  `json:"kind"             db:"kind"`
	OwnerType     string     `json:"owner_type,omitempty" db:"owner_type"`
	OwnerID       *uuid.UUID `json:"owner_id,omitempty"   db:"owner_id"`
	FromEmail     string     `json:"from_email"       db:"from_email"`
	FromName      string     `json:"from_name"        db:"from_name"`
	ToEmail       string     `json:"to_email"         db:"to_email"`
	CCEmails      []string   `json:"cc_emails"        db:"cc_emails"`
	BCCEmails     []string   `json:"bcc_emails"       db:"bcc_emails"`
	Subject       string     `json:"subject"          db:"subject"`
	Body          string     `json:"body"             db:"body"`
	BodyHTML      string     `json:"body_html,omitempty" db:"body_html"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids"  db:"attachment_ids"`
	Status        EmailStatus `json:"status"          db:"status"`
	SentAt        *time.Time  `json:"sent_at,omitempty" db:"sent_at"`
	LastError     string      `json:"last_error,omitempty" db:"last_error"`
	Attempts      int         `json:"attempts"        db:"attempts"`
	CreatedAt     time.Time   `json:"created_at"      db:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"      db:"updated_at"`
}

var (
	ErrEmailNotFound   = errors.New("email not found")
	ErrEmailConflict   = errors.New("email updated by another writer")
	ErrEmailValidation = errors.New("invalid email")
)

// CreateEmailRequest is the body of POST /api/v1/finance/emails.
type CreateEmailRequest struct {
	Kind          EmailKind    `json:"kind"`
	OwnerType     string       `json:"owner_type,omitempty"`
	OwnerID       *uuid.UUID   `json:"owner_id,omitempty"`
	FromEmail     string       `json:"from_email"`
	FromName      string       `json:"from_name,omitempty"`
	ToEmail       string       `json:"to_email"`
	CCEmails      []string     `json:"cc_emails,omitempty"`
	BCCEmails     []string     `json:"bcc_emails,omitempty"`
	Subject       string       `json:"subject"`
	Body          string       `json:"body"`
	BodyHTML      string       `json:"body_html,omitempty"`
	AttachmentIDs []uuid.UUID  `json:"attachment_ids,omitempty"`
}

// SMTPConfig holds SMTP credentials for delivery.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// PlunkConfig holds Plunk credentials for delivery. Plunk is the open-source
// email platform built on AWS SES (https://github.com/useplunk/plunk) that we
// already use for the GitHub page. It exposes a public REST API:
//   POST {BaseURL}/api/v1/{ProjectID}/emails
//   Authorization: Bearer {APIKey}
//   Content-Type: application/json
//   Body: {"from": {...}, "to": "...", "subject": "...", "body": "...", ...}
type PlunkConfig struct {
	BaseURL   string // e.g. "https://app.useplunk.com" or self-hosted equivalent
	ProjectID string // Plunk project UUID
	APIKey    string // Bearer token
	FromEmail string // default sender (override-able per message)
	FromName  string
}

// EmailSender is the abstraction over delivery transports.
// Implementations: DefaultSMTPSender (SMTP), PlunkSender (REST).
type EmailSender interface {
	Send(msg *EmailMessage) error
}

// DefaultSMTPSender is the SMTP implementation (kept for local dev / fallback).
type DefaultSMTPSender struct {
	cfg SMTPConfig
}

func NewDefaultSMTPSender(cfg SMTPConfig) *DefaultSMTPSender {
	return &DefaultSMTPSender{cfg: cfg}
}

func (s *DefaultSMTPSender) Send(msg *EmailMessage) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s <%s>\r\n", msg.FromName, msg.FromEmail)
	fmt.Fprintf(&buf, "To: %s\r\n", msg.ToEmail)
	if len(msg.CCEmails) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(msg.CCEmails, ", "))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", msg.Subject)
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	if msg.BodyHTML != "" {
		fmt.Fprintf(&buf, "Content-Type: text/html; charset=UTF-8\r\n")
	} else {
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n")
	}
	fmt.Fprintf(&buf, "\r\n")
	if msg.BodyHTML != "" {
		fmt.Fprintf(&buf, "%s\r\n", msg.BodyHTML)
	} else {
		fmt.Fprintf(&buf, "%s\r\n", msg.Body)
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	recipients := append([]string{msg.ToEmail}, msg.CCEmails...)
	recipients = append(recipients, msg.BCCEmails...)
	return smtp.SendMail(addr, auth, msg.FromEmail, recipients, buf.Bytes())
}

// EmailRepositoryIface is the subset the service uses.
type EmailRepositoryIface interface {
	Create(ctx context.Context, msg *EmailMessage) (*EmailMessage, error)
	GetByID(ctx context.Context, id uuid.UUID) (*EmailMessage, error)
	ListByOwner(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error)
	ListByStatus(ctx context.Context, status EmailStatus, limit int) ([]*EmailMessage, error)
}

// EmailService coordinates email creation and delivery.
type EmailService struct {
	repo   EmailRepositoryIface
	sender EmailSender
}

func NewEmailService(repo EmailRepositoryIface, sender EmailSender) *EmailService {
	return &EmailService{repo: repo, sender: sender}
}

// Enqueue creates an email in 'queued' status without sending it.
func (s *EmailService) Enqueue(ctx context.Context, req CreateEmailRequest) (*EmailMessage, error) {
	if err := ValidateCreateEmailRequest(req); err != nil {
		return nil, err
	}
	msg := &EmailMessage{
		ID:            uuid.New(),
		Kind:          req.Kind,
		OwnerType:     req.OwnerType,
		OwnerID:       req.OwnerID,
		FromEmail:     req.FromEmail,
		FromName:      req.FromName,
		ToEmail:       req.ToEmail,
		CCEmails:      req.CCEmails,
		BCCEmails:     req.BCCEmails,
		Subject:       req.Subject,
		Body:          req.Body,
		BodyHTML:      req.BodyHTML,
		AttachmentIDs: req.AttachmentIDs,
		Status:        EmailStatusQueued,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	return s.repo.Create(ctx, msg)
}

// Send enqueues + immediately delivers an email.
func (s *EmailService) Send(ctx context.Context, req CreateEmailRequest) (*EmailMessage, error) {
	msg, err := s.Enqueue(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.deliver(ctx, msg)
}

// RetryFailed attempts redelivery of all queued/failed messages up to limit.
func (s *EmailService) RetryFailed(ctx context.Context, limit int) (int, error) {
	msgs, err := s.repo.ListByStatus(ctx, EmailStatusFailed, limit)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, msg := range msgs {
		if _, err := s.deliver(ctx, msg); err == nil {
			delivered++
		}
	}
	return delivered, nil
}

func (s *EmailService) deliver(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	// Mark as sending
	msg.Status = EmailStatusSending
	msg.Attempts++
	if _, err := s.repo.UpdateStatus(ctx, msg.ID, msg.Status, "", nil); err != nil {
		return nil, err
	}

	// Attempt delivery
	err := s.sender.Send(msg)
	now := time.Now().UTC()
	if err != nil {
		// Mark failed
		msg.Status = EmailStatusFailed
		msg.LastError = err.Error()
		msg.UpdatedAt = now
		return s.repo.UpdateStatus(ctx, msg.ID, EmailStatusFailed, err.Error(), nil)
	}

	// Mark sent
	msg.Status = EmailStatusSent
	msg.SentAt = &now
	msg.LastError = ""
	msg.UpdatedAt = now
	return s.repo.UpdateStatus(ctx, msg.ID, EmailStatusSent, "", &now)
}

func (s *EmailService) GetByID(ctx context.Context, id uuid.UUID) (*EmailMessage, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *EmailService) ListByOwner(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error) {
	return s.repo.ListByOwner(ctx, ownerType, ownerID)
}

// ValidateCreateEmailRequest validates a CreateEmailRequest.
func ValidateCreateEmailRequest(req CreateEmailRequest) error {
	if !req.Kind.IsValid() {
		return fmt.Errorf("%w: kind must be one of invoice/agreement", ErrEmailValidation)
	}
	if !strings.Contains(req.ToEmail, "@") {
		return fmt.Errorf("%w: to_email is required", ErrEmailValidation)
	}
	if req.FromEmail == "" {
		return fmt.Errorf("%w: from_email is required", ErrEmailValidation)
	}
	if !strings.Contains(req.FromEmail, "@") {
		return fmt.Errorf("%w: from_email is invalid", ErrEmailValidation)
	}
	if req.Subject == "" {
		return fmt.Errorf("%w: subject is required", ErrEmailValidation)
	}
	if req.Body == "" && req.BodyHTML == "" {
		return fmt.Errorf("%w: body or body_html is required", ErrEmailValidation)
	}
	return nil
}

// EmailRepository persists EmailMessage rows.
type EmailRepository struct {
	db *pgxpool.Pool
}

func NewEmailRepository(db *pgxpool.Pool) *EmailRepository {
	return &EmailRepository{db: db}
}

var _ EmailRepositoryIface = (*EmailRepository)(nil)

func (r *EmailRepository) Create(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	const sql = `
		INSERT INTO email_messages (kind, owner_type, owner_id, from_email, from_name,
			to_email, cc_emails, bcc_emails, subject, body, body_html, attachment_ids, status, attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	ownerType := ""
	if msg.OwnerType != "" {
		ownerType = msg.OwnerType
	}
	cc := msg.CCEmails
	if cc == nil {
		cc = []string{}
	}
	bcc := msg.BCCEmails
	if bcc == nil {
		bcc = []string{}
	}
	attachments := msg.AttachmentIDs
	if attachments == nil {
		attachments = []uuid.UUID{}
	}
	var out EmailMessage
	err := r.db.QueryRow(ctx, sql,
		msg.Kind, ownerType, msg.OwnerID, msg.FromEmail, msg.FromName,
		msg.ToEmail, cc, bcc, msg.Subject, msg.Body, msg.BodyHTML, attachments,
		msg.Status, msg.Attempts,
	).Scan(
		&out.ID, &out.Kind, &out.OwnerType, &out.OwnerID, &out.FromEmail, &out.FromName,
		&out.ToEmail, &out.CCEmails, &out.BCCEmails, &out.Subject, &out.Body,
		&out.BodyHTML, &out.AttachmentIDs, &out.Status, &out.SentAt, &out.LastError,
		&out.Attempts, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert email: %w", err)
	}
	return &out, nil
}

func (r *EmailRepository) GetByID(ctx context.Context, id uuid.UUID) (*EmailMessage, error) {
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE id = $1
	`
	var m EmailMessage
	err := r.db.QueryRow(ctx, sql, id).Scan(
		&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
		&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
		&m.AttachmentIDs, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
		&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailNotFound
		}
		return nil, fmt.Errorf("get email: %w", err)
	}
	return &m, nil
}

func (r *EmailRepository) ListByOwner(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error) {
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE owner_type = $1 AND owner_id = $2 ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, sql, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list emails by owner: %w", err)
	}
	defer rows.Close()

	var out []*EmailMessage
	for rows.Next() {
		var m EmailMessage
		if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
			&m.AttachmentIDs, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (r *EmailRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error) {
	const sql = `
		UPDATE email_messages
		SET status = $1, last_error = $2, sent_at = $3, updated_at = now()
		WHERE id = $4
		RETURNING id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	var m EmailMessage
	err := r.db.QueryRow(ctx, sql, status, lastError, sentAt, id).Scan(
		&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
		&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
		&m.AttachmentIDs, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
		&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailNotFound
		}
		return nil, fmt.Errorf("update email status: %w", err)
	}
	return &m, nil
}

func (r *EmailRepository) ListByStatus(ctx context.Context, status EmailStatus, limit int) ([]*EmailMessage, error) {
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE status = $1 ORDER BY created_at ASC LIMIT $2
	`
	rows, err := r.db.Query(ctx, sql, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list emails by status: %w", err)
	}
	defer rows.Close()

	var out []*EmailMessage
	for rows.Next() {
		var m EmailMessage
		if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
			&m.AttachmentIDs, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

var _ = uuid.Nil