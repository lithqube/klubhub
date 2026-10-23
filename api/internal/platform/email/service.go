package email

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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
	repo        EmailRepositoryIface
	sender      EmailSender
	attachments AttachmentSource // nil: messages cannot carry attachments
	codec       FieldCodec       // nil: IdentityCodec (plaintext passthrough)
}

// The service is the FieldSealer the tenant-scoped repository calls back into
// inside its WithTenant transaction scope.
var _ FieldSealer = (*EmailService)(nil)

// WithAttachments lets the service send messages that carry stored documents
// (SendOwned). Without it such messages are refused.
func (s *EmailService) WithAttachments(src AttachmentSource) *EmailService {
	s.attachments = src
	return s
}

// WithFieldCodec sets the codec used to seal/unseal outbox PII columns.
// Nil (the default) means IdentityCodec: plaintext passthrough (DJ side).
func (s *EmailService) WithFieldCodec(codec FieldCodec) *EmailService {
	s.codec = codec
	return s
}

func (s *EmailService) fieldCodec() FieldCodec {
	if s.codec == nil {
		return IdentityCodec{}
	}
	return s.codec
}

func NewEmailService(repo EmailRepositoryIface, sender EmailSender) *EmailService {
	return &EmailService{repo: repo, sender: sender}
}

// Enqueue creates an email in 'queued' status without sending it.
func (s *EmailService) Enqueue(ctx context.Context, req CreateEmailRequest) (*EmailMessage, error) {
	return s.create(ctx, req, false)
}

func (s *EmailService) create(ctx context.Context, req CreateEmailRequest, claimed bool) (*EmailMessage, error) {
	return s.createMessage(ctx, req, claimed, false)
}

// createMessage validates and stores a message. trusted is for server-built
// requests only: it allows AttachmentIDs, which a client-supplied request
// never may (see ValidateCreateEmailRequest).
func (s *EmailService) createMessage(ctx context.Context, req CreateEmailRequest, claimed, trusted bool) (*EmailMessage, error) {
	attach := req.AttachmentIDs
	if trusted {
		req.AttachmentIDs = nil // checked by SendOwned; the generic rules would refuse them
	}
	if validator, ok := s.sender.(interface {
		ValidateRequest(CreateEmailRequest) error
	}); ok {
		if err := validator.ValidateRequest(req); err != nil {
			return nil, err
		}
	}
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
		AttachmentIDs: attach,
		ReplyTo:       req.ReplyTo,
		Status:        EmailStatusQueued,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if claimed {
		msg.Status = EmailStatusSending
		msg.Attempts = 1
	}
	return s.repo.Create(ctx, msg)
}

// Send enqueues + immediately delivers an email.
func (s *EmailService) Send(ctx context.Context, req CreateEmailRequest) (*EmailMessage, error) {
	// Insert an already claimed initial attempt: never expose this row as queued.
	msg, err := s.create(ctx, req, true)
	if err != nil {
		return nil, err
	}
	return s.finalizeDelivery(ctx, msg)
}

// SendOwned sends a message built by the server that carries stored
// documents. Every attachment must belong to the entity the message is about
// (OwnerType/OwnerID), so a message can never carry another entity's files.
// The ids are stored with the message and the content is loaded at delivery,
// so a failed send is retried with the same files.
func (s *EmailService) SendOwned(ctx context.Context, req CreateEmailRequest) (*EmailMessage, error) {
	if len(req.AttachmentIDs) > 0 {
		if s.attachments == nil {
			return nil, fmt.Errorf("%w: attachments are not available; no email was sent", ErrEmailValidation)
		}
		if req.OwnerType == "" || req.OwnerID == nil {
			return nil, fmt.Errorf("%w: a message with attachments needs the owner they belong to", ErrEmailValidation)
		}
		for _, id := range req.AttachmentIDs {
			doc, err := s.attachments.GetByID(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("%w: attachment %s: %v", ErrEmailValidation, id, err)
			}
			if err := checkAttachmentOwner(doc, req.OwnerType, *req.OwnerID); err != nil {
				return nil, err
			}
		}
	}
	msg, err := s.createMessage(ctx, req, true, true)
	if err != nil {
		return nil, err
	}
	return s.finalizeDelivery(ctx, msg)
}

func checkAttachmentOwner(doc *AttachmentDoc, ownerType string, ownerID uuid.UUID) error {
	if doc.OwnerType != ownerType || doc.OwnerID != ownerID {
		return fmt.Errorf("%w: attachment %s does not belong to this %s", ErrEmailValidation, doc.ID, ownerType)
	}
	return nil
}

// loadAttachments fills msg.Attachments from the stored documents, checking
// ownership and size again at delivery time.
func (s *EmailService) loadAttachments(ctx context.Context, msg *EmailMessage) error {
	if len(msg.AttachmentIDs) == 0 || len(msg.Attachments) == len(msg.AttachmentIDs) {
		return nil
	}
	if s.attachments == nil || msg.OwnerID == nil {
		return fmt.Errorf("%w: attachments cannot be loaded; no email was sent", ErrEmailValidation)
	}
	var out []EmailAttachment
	total := 0
	for _, id := range msg.AttachmentIDs {
		rc, doc, err := s.attachments.Download(ctx, id)
		if err != nil {
			return fmt.Errorf("load attachment %s: %w", id, err)
		}
		if err := checkAttachmentOwner(doc, msg.OwnerType, *msg.OwnerID); err != nil {
			_ = rc.Close()
			return err
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxEmailAttachmentBytes+1))
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("read attachment %s: %w", id, err)
		}
		if total += len(content); total > maxEmailAttachmentBytes {
			return fmt.Errorf("%w: attachments exceed the %d MB limit", ErrEmailValidation, maxEmailAttachmentBytes>>20)
		}
		out = append(out, EmailAttachment{Filename: doc.Filename, MimeType: doc.MimeType, Content: content})
	}
	msg.Attachments = out
	return nil
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
	if limit <= 0 {
		return 0, nil
	}
	if limit > 16 {
		limit = 16
	}
	if _, err := s.repo.RequeueStale(ctx, emailStaleSendingTimeout); err != nil {
		return 0, err
	}
	msgs, err := s.repo.ClaimFailedForRetry(ctx, limit, maxEmailAttempts, emailRetryBackoffBaseSeconds)
	if err != nil {
		return 0, err
	}
	delivered := 0
	var failures error
	for _, msg := range msgs {
		out, err := s.finalizeDelivery(ctx, msg)
		if err != nil {
			failures = errors.Join(failures, err)
		} else if out.Status == EmailStatusSent {
			delivered++
		}
	}
	return delivered, failures
}

// finalizeDelivery attempts delivery of a message durably claimed as
// sending (either by initial insertion or by
// ClaimFailedForRetry) and records the final status.
func (s *EmailService) finalizeDelivery(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	deliveryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err := s.loadAttachments(deliveryCtx, msg)
	if err != nil {
		// Nothing is sent without its attachments; the row stays retryable.
	} else if sender, ok := s.sender.(interface {
		SendContext(context.Context, *EmailMessage) error
	}); ok {
		err = sender.SendContext(deliveryCtx, msg)
	} else {
		// Legacy synchronous adapters are only suitable for bounded test/local senders.
		err = s.sender.Send(msg)
	}
	// Finalize even if the caller disconnected or shutdown cancelled delivery.
	// A failed DB write leaves the claimed row recoverable with the SAME ID.
	persistCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	claim := *msg
	finish := func(status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error) {
		if repo, ok := s.repo.(interface {
			FinishAttempt(context.Context, *EmailMessage, EmailStatus, string, *time.Time) (*EmailMessage, error)
		}); ok {
			return repo.FinishAttempt(persistCtx, &claim, status, lastError, sentAt)
		}
		return s.repo.UpdateStatus(persistCtx, msg.ID, status, lastError, sentAt)
	}
	now := time.Now().UTC()
	if err != nil {
		// Mark failed
		msg.Status = EmailStatusFailed
		msg.LastError = err.Error()
		msg.UpdatedAt = now
		return finish(EmailStatusFailed, err.Error(), nil)
	}

	// Mark sent
	msg.Status = EmailStatusSent
	msg.SentAt = &now
	msg.LastError = ""
	msg.UpdatedAt = now
	return finish(EmailStatusSent, "", &now)
}

// StartWorker runs one bounded delivery at a time, including an immediate
// startup recovery pass. The caller MUST cancel ctx and wait for done before
// closing the database. Production transports must implement SendContext.
func (s *EmailService) StartWorker(ctx context.Context, interval time.Duration, onError func(error)) <-chan struct{} {
	done := make(chan struct{})
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		defer close(done)
		if _, ok := s.sender.(interface {
			SendContext(context.Context, *EmailMessage) error
		}); !ok {
			if onError != nil {
				onError(errors.New("email worker requires a cancellable sender"))
			}
			return
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			err := s.workerTick(ctx)
			if err != nil && ctx.Err() == nil && onError != nil {
				onError(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

// workerTick runs one bounded retry pass. A panic in the repository or
// sender is converted to an error so it cannot crash the API process (chi's
// Recoverer does not cover this goroutine).
func (s *EmailService) workerTick(ctx context.Context) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("email worker panic: %v", rec)
		}
	}()
	passCtx, stop := context.WithTimeout(ctx, 40*time.Second)
	defer stop()
	_, err = s.RetryFailed(passCtx, 1)
	return err
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
	if len(req.AttachmentIDs) > 0 {
		return fmt.Errorf("%w: attachment_ids are not supported; no email was sent", ErrEmailValidation)
	}
	if !req.Kind.IsValid() {
		return fmt.Errorf("%w: unknown email kind %q", ErrEmailValidation, req.Kind)
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
	if req.ReplyTo != "" {
		if err := validateAddress("reply_to", req.ReplyTo); err != nil {
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

// SealMessageFields seals the outbox PII fields of a message through the
// service's FieldCodec and returns the blind index for to_email (the
// repository stores it in to_email_bidx). With IdentityCodec (the default)
// this is a no-op passthrough returning a nil bidx. The envelope-backed codec
// (promoter side) encrypts to_email, subject, body, body_html and reply_to.
//
// The tenant-scoped repository calls this inside its WithTenant transaction
// scope (via the FieldSealer callback) so the codec resolves the tenant DEK
// from the same transaction as the insert.
func (s *EmailService) SealMessageFields(ctx context.Context, tx pgx.Tx, msg *EmailMessage) ([]byte, error) {
	codec := s.fieldCodec()
	if _, isIdentity := codec.(IdentityCodec); isIdentity {
		return nil, nil
	}
	ref := FieldRef{Table: "email_messages", RowID: msg.ID}

	toEnc, toBidx, err := codec.SealEmail(ctx, tx, FieldRef{Table: ref.Table, Column: "to_email", RowID: ref.RowID}, msg.ToEmail)
	if err != nil {
		return nil, fmt.Errorf("seal to_email: %w", err)
	}
	msg.ToEmail = string(toEnc)

	if msg.Subject != "" {
		enc, err := codec.Seal(ctx, tx, FieldRef{Table: ref.Table, Column: "subject", RowID: ref.RowID}, msg.Subject)
		if err != nil {
			return nil, fmt.Errorf("seal subject: %w", err)
		}
		msg.Subject = string(enc)
	}
	if msg.Body != "" {
		enc, err := codec.Seal(ctx, tx, FieldRef{Table: ref.Table, Column: "body", RowID: ref.RowID}, msg.Body)
		if err != nil {
			return nil, fmt.Errorf("seal body: %w", err)
		}
		msg.Body = string(enc)
	}
	if msg.BodyHTML != "" {
		enc, err := codec.Seal(ctx, tx, FieldRef{Table: ref.Table, Column: "body_html", RowID: ref.RowID}, msg.BodyHTML)
		if err != nil {
			return nil, fmt.Errorf("seal body_html: %w", err)
		}
		msg.BodyHTML = string(enc)
	}
	if msg.ReplyTo != "" {
		enc, err := codec.Seal(ctx, tx, FieldRef{Table: ref.Table, Column: "reply_to", RowID: ref.RowID}, msg.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("seal reply_to: %w", err)
		}
		msg.ReplyTo = string(enc)
	}
	return toBidx, nil
}

// UnsealMessageFields reverses SealMessageFields for the delivery/log path.
func (s *EmailService) UnsealMessageFields(ctx context.Context, tx pgx.Tx, msg *EmailMessage) error {
	codec := s.fieldCodec()
	if _, isIdentity := codec.(IdentityCodec); isIdentity {
		return nil
	}
	ref := FieldRef{Table: "email_messages", RowID: msg.ID}

	if msg.ToEmail != "" {
		plain, err := codec.Open(ctx, tx, FieldRef{Table: ref.Table, Column: "to_email", RowID: ref.RowID}, []byte(msg.ToEmail))
		if err != nil {
			return fmt.Errorf("unseal to_email: %w", err)
		}
		msg.ToEmail = plain
	}
	if msg.Subject != "" {
		plain, err := codec.Open(ctx, tx, FieldRef{Table: ref.Table, Column: "subject", RowID: ref.RowID}, []byte(msg.Subject))
		if err != nil {
			return fmt.Errorf("unseal subject: %w", err)
		}
		msg.Subject = plain
	}
	if msg.Body != "" {
		plain, err := codec.Open(ctx, tx, FieldRef{Table: ref.Table, Column: "body", RowID: ref.RowID}, []byte(msg.Body))
		if err != nil {
			return fmt.Errorf("unseal body: %w", err)
		}
		msg.Body = plain
	}
	if msg.BodyHTML != "" {
		plain, err := codec.Open(ctx, tx, FieldRef{Table: ref.Table, Column: "body_html", RowID: ref.RowID}, []byte(msg.BodyHTML))
		if err != nil {
			return fmt.Errorf("unseal body_html: %w", err)
		}
		msg.BodyHTML = plain
	}
	if msg.ReplyTo != "" {
		plain, err := codec.Open(ctx, tx, FieldRef{Table: ref.Table, Column: "reply_to", RowID: ref.RowID}, []byte(msg.ReplyTo))
		if err != nil {
			return fmt.Errorf("unseal reply_to: %w", err)
		}
		msg.ReplyTo = plain
	}
	return nil
}
