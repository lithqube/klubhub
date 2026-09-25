package finance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EmailKind enumerates the kinds of transactional emails the system sends.
type EmailKind string

const (
	EmailKindInvoiceIssued      EmailKind = "invoice_issued"
	EmailKindInvoicePaid        EmailKind = "invoice_paid"
	EmailKindInvoiceCancelled   EmailKind = "invoice_cancelled"
	EmailKindAgreementSent      EmailKind = "agreement_sent"
	EmailKindAgreementSigned    EmailKind = "agreement_signed"
	EmailKindAgreementCompleted EmailKind = "agreement_completed"
)

var validEmailKinds = map[EmailKind]struct{}{
	EmailKindInvoiceIssued:      {},
	EmailKindInvoicePaid:        {},
	EmailKindInvoiceCancelled:   {},
	EmailKindAgreementSent:      {},
	EmailKindAgreementSigned:    {},
	EmailKindAgreementCompleted: {},
}

func (k EmailKind) IsValid() bool {
	_, ok := validEmailKinds[k]
	return ok
}

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
//
//	POST {BaseURL}/api/v1/{ProjectID}/emails
//	Authorization: Bearer {APIKey}
//	Content-Type: application/json
//	Body: {"from": {...}, "to": "...", "subject": "...", "body": "...", ...}
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
	// Defense in depth: ValidateCreateEmailRequest already rejects header
	// injection (CR/LF/control chars) and validates addresses at the API
	// boundary, but this sender must not trust that every caller went
	// through that path. Re-encode headers safely here too.
	if err := rejectHeaderInjection(msg.Subject); err != nil {
		return fmt.Errorf("smtp: subject: %w", err)
	}
	if err := rejectHeaderInjection(msg.FromName); err != nil {
		return fmt.Errorf("smtp: from name: %w", err)
	}

	from := formatAddress(msg.FromName, msg.FromEmail)
	if _, err := mail.ParseAddress(msg.FromEmail); err != nil {
		return fmt.Errorf("smtp: invalid from address: %w", err)
	}
	to, err := formatAddressList([]string{msg.ToEmail})
	if err != nil {
		return fmt.Errorf("smtp: invalid to address: %w", err)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	if len(msg.CCEmails) > 0 {
		cc, err := formatAddressList(msg.CCEmails)
		if err != nil {
			return fmt.Errorf("smtp: invalid cc address: %w", err)
		}
		fmt.Fprintf(&buf, "Cc: %s\r\n", cc)
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", msg.Subject))
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
	// ClaimFailedForRetry atomically transitions up to limit 'failed' rows
	// (that have fewer than maxAttempts attempts and have waited at least
	// attempts*backoffBaseSeconds since their last update) to 'sending',
	// incrementing attempts, and returns the claimed rows. Implementations
	// must use SELECT ... FOR UPDATE SKIP LOCKED (or equivalent) so two
	// concurrent callers never claim, and therefore never deliver, the
	// same row twice.
	ClaimFailedForRetry(ctx context.Context, limit, maxAttempts, backoffBaseSeconds int) ([]*EmailMessage, error)
	// RequeueStale moves rows stuck in 'sending' for longer than olderThan
	// back to 'failed' so a crashed/killed worker doesn't leave them
	// stranded forever. Returns the number of rows requeued.
	RequeueStale(ctx context.Context, olderThan time.Duration) (int, error)
}

const (
	// maxEmailAttempts caps delivery attempts; once a 'failed' row has
	// been attempted this many times it is excluded from further claims
	// and stays 'failed' permanently (there is no separate terminal
	// status — operators can inspect last_error/attempts to find these).
	maxEmailAttempts = 8
	// emailRetryBackoffBaseSeconds is the linear backoff unit between
	// retry attempts: a row with N attempts is eligible again only after
	// N*emailRetryBackoffBaseSeconds have elapsed since its last update.
	emailRetryBackoffBaseSeconds = 30
	// emailStaleSendingTimeout bounds how long a row may sit in 'sending'
	// before RequeueStale assumes the worker that claimed it died and
	// moves it back to 'failed' for retry.
	emailStaleSendingTimeout = 15 * time.Minute
)

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

// RetryFailed attempts redelivery of failed messages up to limit.
//
// Delivery is claimed atomically (ClaimFailedForRetry marks rows 'sending'
// in the same statement that selects them, using SELECT ... FOR UPDATE
// SKIP LOCKED under the hood) so that two concurrent callers — e.g. two
// cron/worker replicas — can never both claim and therefore never both
// deliver the same row. Rows stuck in 'sending' past
// emailStaleSendingTimeout (e.g. because a worker crashed mid-delivery)
// are requeued to 'failed' first so they become claimable again.
func (s *EmailService) RetryFailed(ctx context.Context, limit int) (int, error) {
	if _, err := s.repo.RequeueStale(ctx, emailStaleSendingTimeout); err != nil {
		return 0, err
	}
	msgs, err := s.repo.ClaimFailedForRetry(ctx, limit, maxEmailAttempts, emailRetryBackoffBaseSeconds)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, msg := range msgs {
		if _, err := s.finalizeDelivery(ctx, msg); err == nil {
			delivered++
		}
	}
	return delivered, nil
}

// deliver marks msg 'sending' (incrementing attempts) and then finalizes
// delivery. Used by Send(), where the message was just enqueued by this
// same request and is not visible to any concurrent claimer yet.
func (s *EmailService) deliver(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	msg.Status = EmailStatusSending
	msg.Attempts++
	if _, err := s.repo.UpdateStatus(ctx, msg.ID, msg.Status, "", nil); err != nil {
		return nil, err
	}
	return s.finalizeDelivery(ctx, msg)
}

// finalizeDelivery attempts delivery of a message that has already been
// atomically transitioned to 'sending' (either by deliver() or by
// ClaimFailedForRetry) and records the final status.
func (s *EmailService) finalizeDelivery(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
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
//
// This is the primary defense against email header injection: Subject,
// FromName and every address field are rejected outright if they contain
// CR, LF or other control characters, and every address is additionally
// parsed with net/mail.ParseAddress so a value like
// "victim@x.com\r\nBcc: attacker@evil.com" can never reach a sender.
func ValidateCreateEmailRequest(req CreateEmailRequest) error {
	if !req.Kind.IsValid() {
		return fmt.Errorf("%w: kind must be one of invoice/agreement", ErrEmailValidation)
	}
	if req.ToEmail == "" {
		return fmt.Errorf("%w: to_email is required", ErrEmailValidation)
	}
	if err := validateAddress("to_email", req.ToEmail); err != nil {
		return err
	}
	if req.FromEmail == "" {
		return fmt.Errorf("%w: from_email is required", ErrEmailValidation)
	}
	if err := validateAddress("from_email", req.FromEmail); err != nil {
		return err
	}
	if err := validateHeaderField("from_name", req.FromName); err != nil {
		return err
	}
	for _, cc := range req.CCEmails {
		if err := validateAddress("cc_emails", cc); err != nil {
			return err
		}
	}
	for _, bcc := range req.BCCEmails {
		if err := validateAddress("bcc_emails", bcc); err != nil {
			return err
		}
	}
	if req.Subject == "" {
		return fmt.Errorf("%w: subject is required", ErrEmailValidation)
	}
	if err := validateHeaderField("subject", req.Subject); err != nil {
		return err
	}
	if req.Body == "" && req.BodyHTML == "" {
		return fmt.Errorf("%w: body or body_html is required", ErrEmailValidation)
	}
	return nil
}

// validateHeaderField rejects values that could inject extra headers or
// SMTP commands if interpolated into a raw RFC 5322 header line.
func validateHeaderField(field, value string) error {
	if err := rejectHeaderInjection(value); err != nil {
		return fmt.Errorf("%w: %s %s", ErrEmailValidation, field, err)
	}
	return nil
}

// validateAddress rejects header-injection payloads and requires the value
// to parse as a single RFC 5322 address.
func validateAddress(field, value string) error {
	if err := rejectHeaderInjection(value); err != nil {
		return fmt.Errorf("%w: %s %s", ErrEmailValidation, field, err)
	}
	if _, err := mail.ParseAddress(value); err != nil {
		return fmt.Errorf("%w: %s is not a valid email address", ErrEmailValidation, field)
	}
	return nil
}

// rejectHeaderInjection reports an error if value contains CR, LF, or any
// other ASCII control character, which would otherwise let an attacker
// smuggle extra SMTP/MIME headers (e.g. an extra "Bcc:" line) into a
// message built by simple string interpolation.
func rejectHeaderInjection(value string) error {
	for _, r := range value {
		if r == '\r' || r == '\n' || (unicode.IsControl(r) && r != '\t') {
			return fmt.Errorf("must not contain control characters")
		}
	}
	return nil
}

// formatAddress renders a name + email pair as a single RFC 5322 address
// header value via net/mail, which handles quoting/escaping so a crafted
// name cannot break out of the header.
func formatAddress(name, email string) string {
	return (&mail.Address{Name: name, Address: email}).String()
}

// formatAddressList validates and formats a list of plain email addresses
// as a comma-separated RFC 5322 header value.
func formatAddressList(addrs []string) (string, error) {
	formatted := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if _, err := mail.ParseAddress(a); err != nil {
			return "", fmt.Errorf("invalid address %q: %w", a, err)
		}
		formatted = append(formatted, formatAddress("", a))
	}
	return strings.Join(formatted, ", "), nil
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

// ClaimFailedForRetry atomically claims up to limit 'failed' rows for
// redelivery: it selects eligible rows (attempts < maxAttempts, and at
// least attempts*backoffBaseSeconds elapsed since updated_at) with
// SELECT ... FOR UPDATE SKIP LOCKED, and in the same statement flips them
// to 'sending' and increments attempts. Because the row-lock, filter and
// mutation happen in one statement, two concurrent callers can never both
// claim the same row — one gets it, the other skips it via SKIP LOCKED.
func (r *EmailRepository) ClaimFailedForRetry(ctx context.Context, limit, maxAttempts, backoffBaseSeconds int) ([]*EmailMessage, error) {
	const sql = `
		UPDATE email_messages
		SET status = 'sending', attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM email_messages
			WHERE status = 'failed'
				AND attempts < $1
				AND updated_at < now() - make_interval(secs => (attempts * $2)::double precision)
			ORDER BY created_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	rows, err := r.db.Query(ctx, sql, maxAttempts, backoffBaseSeconds, limit)
	if err != nil {
		return nil, fmt.Errorf("claim failed emails for retry: %w", err)
	}
	defer rows.Close()

	var out []*EmailMessage
	for rows.Next() {
		var m EmailMessage
		if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
			&m.AttachmentIDs, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan claimed email: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// RequeueStale moves rows stuck in 'sending' for longer than olderThan
// back to 'failed' so a worker crash/kill mid-delivery doesn't strand
// them forever. It deliberately leaves updated_at untouched (rather than
// bumping it to now()): the row has already waited olderThan, which is
// far longer than any retry backoff window, so it becomes immediately
// eligible for ClaimFailedForRetry instead of having to wait out another
// backoff period on top of the time it was already stuck.
func (r *EmailRepository) RequeueStale(ctx context.Context, olderThan time.Duration) (int, error) {
	const sql = `
		UPDATE email_messages
		SET status = 'failed',
			last_error = 'requeued: stuck in sending past timeout'
		WHERE status = 'sending' AND updated_at < $1
	`
	cutoff := time.Now().UTC().Add(-olderThan)
	tag, err := r.db.Exec(ctx, sql, cutoff)
	if err != nil {
		return 0, fmt.Errorf("requeue stale sending emails: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

var _ = uuid.Nil
