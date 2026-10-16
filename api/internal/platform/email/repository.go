package email

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// EmailRepository persists EmailMessage rows.
//
// Two modes:
//   - tenantScoped=false (DJ side): plaintext columns, SQL byte-identical to
//     the original finance repository. Uses the pool directly.
//   - tenantScoped=true (promoter side): sealed columns (to_email_enc,
//     to_email_bidx, subject_enc, body_enc, body_html_enc, reply_to_enc; no
//     cc/bcc), every statement runs inside tenantdb.WithTenant so RLS
//     (ENABLE + FORCE) isolates rows. The values arrive already sealed by
//     the FieldSealer (the service's FieldCodec); the repository stores
//     bytes as-is and unseals on every read, inside the same transaction.
type EmailRepository struct {
	db           *tenantdb.DB
	tenantScoped bool
	sealer       FieldSealer // nil on the DJ side: plaintext columns
}

// NewEmailRepository creates an EmailRepository. The DJ side wraps its pool
// with tenantdb.New(pool) and passes tenantScoped=false; the promoter binary
// passes its existing *tenantdb.DB with tenantScoped=true.
func NewEmailRepository(db *tenantdb.DB, tenantScoped bool) *EmailRepository {
	return &EmailRepository{db: db, tenantScoped: tenantScoped}
}

// WithFieldSealer wires the callback the tenant-scoped paths invoke inside
// their WithTenant transaction scope to seal (insert) and unseal (reads) the
// outbox PII columns. *EmailService implements FieldSealer; the DJ side never
// wires one (plaintext columns, IdentityCodec).
func (r *EmailRepository) WithFieldSealer(s FieldSealer) *EmailRepository {
	r.sealer = s
	return r
}

var _ EmailRepositoryIface = (*EmailRepository)(nil)

// Create inserts a new email message row.
func (r *EmailRepository) Create(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	if r.tenantScoped {
		return r.createTenantScoped(ctx, msg)
	}
	return r.createPlain(ctx, msg)
}

func (r *EmailRepository) createPlain(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	const sql = `
		INSERT INTO email_messages (kind, owner_type, owner_id, from_email, from_name,
			to_email, cc_emails, bcc_emails, subject, body, body_html, attachment_ids, status, attempts, reply_to)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids, reply_to,
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
	err := r.db.Pool().QueryRow(ctx, sql,
		msg.Kind, ownerType, msg.OwnerID, msg.FromEmail, msg.FromName,
		msg.ToEmail, cc, bcc, msg.Subject, msg.Body, msg.BodyHTML, attachments,
		msg.Status, msg.Attempts, msg.ReplyTo,
	).Scan(
		&out.ID, &out.Kind, &out.OwnerType, &out.OwnerID, &out.FromEmail, &out.FromName,
		&out.ToEmail, &out.CCEmails, &out.BCCEmails, &out.Subject, &out.Body,
		&out.BodyHTML, &out.AttachmentIDs, &out.ReplyTo, &out.Status, &out.SentAt, &out.LastError,
		&out.Attempts, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert email: %w", err)
	}
	return &out, nil
}

// createTenantScoped inserts into the promoter's sealed-column email_messages.
// The msg fields are sealed by the FieldSealer inside the WithTenant
// transaction (so an envelope-backed codec resolves the tenant DEK from the
// same tx as the insert); the repository stores the bytes as-is and returns
// the row unsealed.
func (r *EmailRepository) createTenantScoped(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, tenantdb.ErrNoTenant
	}
	// The id is client-supplied (msg.ID): the FieldCodec seals the PII
	// columns with RowID = msg.ID as AAD, so the stored row must carry the
	// same id the values were sealed for.
	const sql = `
		INSERT INTO email_messages (id, tenant_id, kind, owner_type, owner_id, from_email, from_name,
			to_email_enc, to_email_bidx, subject_enc, body_enc, body_html_enc, attachment_ids, status, attempts, reply_to_enc)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id, tenant_id, kind, owner_type, owner_id, from_email, from_name,
			to_email_enc, to_email_bidx, subject_enc, body_enc, body_html_enc, attachment_ids, reply_to_enc,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	ownerType := ""
	if msg.OwnerType != "" {
		ownerType = msg.OwnerType
	}
	attachments := msg.AttachmentIDs
	if attachments == nil {
		attachments = []uuid.UUID{}
	}

	var out EmailMessage
	var toEnc, subjectEnc, bodyEnc, bodyHTMLEnc, replyToEnc []byte
	err := r.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		// Seal the outbox PII columns inside this transaction. The sealed
		// values are stored as the string-encoded bytes the codec produced;
		// the repository treats them as opaque []byte.
		var toBidx []byte
		if r.sealer != nil {
			var err error
			toBidx, err = r.sealer.SealMessageFields(ctx, tx, msg)
			if err != nil {
				return err
			}
		}
		toEnc = []byte(msg.ToEmail)
		subjectEnc = []byte(msg.Subject)
		bodyEnc = []byte(msg.Body)
		if msg.BodyHTML != "" {
			bodyHTMLEnc = []byte(msg.BodyHTML)
		}
		if msg.ReplyTo != "" {
			replyToEnc = []byte(msg.ReplyTo)
		}
		if err := tx.QueryRow(ctx, sql,
			msg.ID, tenantID, msg.Kind, ownerType, msg.OwnerID, msg.FromEmail, msg.FromName,
			toEnc, toBidx, subjectEnc, bodyEnc, bodyHTMLEnc, attachments,
			msg.Status, msg.Attempts, replyToEnc,
		).Scan(
			&out.ID, nil, &out.Kind, &out.OwnerType, &out.OwnerID, &out.FromEmail, &out.FromName,
			&toEnc, nil, &subjectEnc, &bodyEnc, &bodyHTMLEnc, &out.AttachmentIDs, &replyToEnc,
			&out.Status, &out.SentAt, &out.LastError, &out.Attempts, &out.CreatedAt, &out.UpdatedAt); err != nil {
			return err
		}
		// Return the row unsealed: callers never see ciphertext.
		out.ToEmail = string(toEnc)
		out.Subject = string(subjectEnc)
		out.Body = string(bodyEnc)
		if bodyHTMLEnc != nil {
			out.BodyHTML = string(bodyHTMLEnc)
		}
		if replyToEnc != nil {
			out.ReplyTo = string(replyToEnc)
		}
		if r.sealer != nil {
			return r.sealer.UnsealMessageFields(ctx, tx, &out)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("insert email (tenant-scoped): %w", err)
	}
	return &out, nil
}

func (r *EmailRepository) GetByID(ctx context.Context, id uuid.UUID) (*EmailMessage, error) {
	if r.tenantScoped {
		return r.getByIDTenantScoped(ctx, id)
	}
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids, reply_to,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE id = $1
	`
	var m EmailMessage
	err := r.db.Pool().QueryRow(ctx, sql, id).Scan(
		&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
		&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
		&m.AttachmentIDs, &m.ReplyTo, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
		&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailNotFound
		}
		return nil, fmt.Errorf("get email: %w", err)
	}
	return &m, nil
}

func (r *EmailRepository) getByIDTenantScoped(ctx context.Context, id uuid.UUID) (*EmailMessage, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, tenantdb.ErrNoTenant
	}
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name,
			to_email_enc, subject_enc, body_enc, body_html_enc, attachment_ids, reply_to_enc,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE id = $1
	`
	var m EmailMessage
	err := r.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var toEnc, subjectEnc, bodyEnc, bodyHTMLEnc, replyToEnc []byte
		if err := tx.QueryRow(ctx, sql, id).Scan(
			&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&toEnc, &subjectEnc, &bodyEnc, &bodyHTMLEnc, &m.AttachmentIDs, &replyToEnc,
			&m.Status, &m.SentAt, &m.LastError, &m.Attempts, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return err
		}
		m.ToEmail = string(toEnc)
		m.Subject = string(subjectEnc)
		m.Body = string(bodyEnc)
		if bodyHTMLEnc != nil {
			m.BodyHTML = string(bodyHTMLEnc)
		}
		if replyToEnc != nil {
			m.ReplyTo = string(replyToEnc)
		}
		if r.sealer != nil {
			return r.sealer.UnsealMessageFields(ctx, tx, &m)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailNotFound
		}
		return nil, fmt.Errorf("get email (tenant-scoped): %w", err)
	}
	return &m, nil
}

func (r *EmailRepository) ListByOwner(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error) {
	if r.tenantScoped {
		return r.listByOwnerTenantScoped(ctx, ownerType, ownerID)
	}
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids, reply_to,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE owner_type = $1 AND owner_id = $2 ORDER BY created_at DESC
	`
	rows, err := r.db.Pool().Query(ctx, sql, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list emails by owner: %w", err)
	}
	defer rows.Close()

	var out []*EmailMessage
	for rows.Next() {
		var m EmailMessage
		if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
			&m.AttachmentIDs, &m.ReplyTo, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (r *EmailRepository) listByOwnerTenantScoped(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, tenantdb.ErrNoTenant
	}
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name,
			to_email_enc, subject_enc, body_enc, body_html_enc, attachment_ids, reply_to_enc,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE owner_type = $1 AND owner_id = $2 ORDER BY created_at DESC
	`
	var out []*EmailMessage
	err := r.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, ownerType, ownerID)
		if err != nil {
			return fmt.Errorf("list emails by owner (tenant-scoped): %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m EmailMessage
			var toEnc, subjectEnc, bodyEnc, bodyHTMLEnc, replyToEnc []byte
			if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
				&toEnc, &subjectEnc, &bodyEnc, &bodyHTMLEnc, &m.AttachmentIDs, &replyToEnc,
				&m.Status, &m.SentAt, &m.LastError, &m.Attempts, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return fmt.Errorf("scan email (tenant-scoped): %w", err)
			}
			m.ToEmail = string(toEnc)
			m.Subject = string(subjectEnc)
			m.Body = string(bodyEnc)
			if bodyHTMLEnc != nil {
				m.BodyHTML = string(bodyHTMLEnc)
			}
			if replyToEnc != nil {
				m.ReplyTo = string(replyToEnc)
			}
			if r.sealer != nil {
				if err := r.sealer.UnsealMessageFields(ctx, tx, &m); err != nil {
					return err
				}
			}
			out = append(out, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *EmailRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error) {
	return r.updateStatus(ctx, id, status, lastError, sentAt, 0, time.Time{})
}

// FinishAttempt fences late completions from a superseded claim.
func (r *EmailRepository) FinishAttempt(ctx context.Context, claim *EmailMessage, status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error) {
	return r.updateStatus(ctx, claim.ID, status, lastError, sentAt, claim.Attempts, claim.UpdatedAt)
}

func (r *EmailRepository) updateStatus(ctx context.Context, id uuid.UUID, status EmailStatus, lastError string, sentAt *time.Time, attempts int, claimedAt time.Time) (*EmailMessage, error) {
	if r.tenantScoped {
		return r.updateStatusTenantScoped(ctx, id, status, lastError, sentAt, attempts, claimedAt)
	}
	const sql = `
		UPDATE email_messages
		SET status = $1, last_error = $2, sent_at = $3, updated_at = now()
		WHERE id = $4 AND ($5 = 0 OR (status = 'sending' AND attempts = $5 AND updated_at = $6))
		RETURNING id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids, reply_to,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	var m EmailMessage
	err := r.db.Pool().QueryRow(ctx, sql, status, lastError, sentAt, id, attempts, claimedAt).Scan(
		&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
		&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
		&m.AttachmentIDs, &m.ReplyTo, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
		&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailNotFound
		}
		return nil, fmt.Errorf("update email status: %w", err)
	}
	return &m, nil
}

func (r *EmailRepository) updateStatusTenantScoped(ctx context.Context, id uuid.UUID, status EmailStatus, lastError string, sentAt *time.Time, attempts int, claimedAt time.Time) (*EmailMessage, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, tenantdb.ErrNoTenant
	}
	const sql = `
		UPDATE email_messages
		SET status = $1, last_error = $2, sent_at = $3, updated_at = now()
		WHERE id = $4 AND ($5 = 0 OR (status = 'sending' AND attempts = $5 AND updated_at = $6))
		RETURNING id, kind, owner_type, owner_id, from_email, from_name,
			to_email_enc, subject_enc, body_enc, body_html_enc, attachment_ids, reply_to_enc,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	var m EmailMessage
	err := r.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var toEnc, subjectEnc, bodyEnc, bodyHTMLEnc, replyToEnc []byte
		if err := tx.QueryRow(ctx, sql, status, lastError, sentAt, id, attempts, claimedAt).Scan(
			&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&toEnc, &subjectEnc, &bodyEnc, &bodyHTMLEnc, &m.AttachmentIDs, &replyToEnc,
			&m.Status, &m.SentAt, &m.LastError, &m.Attempts, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return err
		}
		m.ToEmail = string(toEnc)
		m.Subject = string(subjectEnc)
		m.Body = string(bodyEnc)
		if bodyHTMLEnc != nil {
			m.BodyHTML = string(bodyHTMLEnc)
		}
		if replyToEnc != nil {
			m.ReplyTo = string(replyToEnc)
		}
		if r.sealer != nil {
			return r.sealer.UnsealMessageFields(ctx, tx, &m)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailNotFound
		}
		return nil, fmt.Errorf("update email status (tenant-scoped): %w", err)
	}
	return &m, nil
}

func (r *EmailRepository) ListByStatus(ctx context.Context, status EmailStatus, limit int) ([]*EmailMessage, error) {
	if r.tenantScoped {
		return r.listByStatusTenantScoped(ctx, status, limit)
	}
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids, reply_to,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE status = $1 ORDER BY created_at ASC LIMIT $2
	`
	rows, err := r.db.Pool().Query(ctx, sql, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list emails by status: %w", err)
	}
	defer rows.Close()

	var out []*EmailMessage
	for rows.Next() {
		var m EmailMessage
		if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
			&m.AttachmentIDs, &m.ReplyTo, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (r *EmailRepository) listByStatusTenantScoped(ctx context.Context, status EmailStatus, limit int) ([]*EmailMessage, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, tenantdb.ErrNoTenant
	}
	const sql = `
		SELECT id, kind, owner_type, owner_id, from_email, from_name,
			to_email_enc, subject_enc, body_enc, body_html_enc, attachment_ids, reply_to_enc,
			status, sent_at, last_error, attempts, created_at, updated_at
		FROM email_messages WHERE status = $1 ORDER BY created_at ASC LIMIT $2
	`
	var out []*EmailMessage
	err := r.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, status, limit)
		if err != nil {
			return fmt.Errorf("list emails by status (tenant-scoped): %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m EmailMessage
			var toEnc, subjectEnc, bodyEnc, bodyHTMLEnc, replyToEnc []byte
			if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
				&toEnc, &subjectEnc, &bodyEnc, &bodyHTMLEnc, &m.AttachmentIDs, &replyToEnc,
				&m.Status, &m.SentAt, &m.LastError, &m.Attempts, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return fmt.Errorf("scan email (tenant-scoped): %w", err)
			}
			m.ToEmail = string(toEnc)
			m.Subject = string(subjectEnc)
			m.Body = string(bodyEnc)
			if bodyHTMLEnc != nil {
				m.BodyHTML = string(bodyHTMLEnc)
			}
			if replyToEnc != nil {
				m.ReplyTo = string(replyToEnc)
			}
			if r.sealer != nil {
				if err := r.sealer.UnsealMessageFields(ctx, tx, &m); err != nil {
					return err
				}
			}
			out = append(out, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ClaimFailedForRetry atomically claims queued and eligible failed rows for
// redelivery: it selects eligible rows (attempts < maxAttempts, and at
// least attempts*backoffBaseSeconds elapsed since updated_at) with
// SELECT ... FOR UPDATE SKIP LOCKED, and in the same statement flips them
// to 'sending' and increments attempts. Because the row-lock, filter and
// mutation happen in one statement, two concurrent callers can never both
// claim the same row — one gets it, the other skips it via SKIP LOCKED.
func (r *EmailRepository) ClaimFailedForRetry(ctx context.Context, limit, maxAttempts, backoffBaseSeconds int) ([]*EmailMessage, error) {
	if r.tenantScoped {
		return r.claimFailedTenantScoped(ctx, limit, maxAttempts, backoffBaseSeconds)
	}
	const sql = `
		UPDATE email_messages
		SET status = 'sending', attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM email_messages
			WHERE (status = 'queued' OR (status = 'failed'
				AND updated_at < now() - make_interval(secs => (attempts * $2)::double precision)))
				AND attempts < $1
            -- Plunk's default key retention is 24h. Never blindly replay
            -- an ambiguous attempted delivery after its key may expire.
            AND (attempts = 0 OR created_at > now() - interval '23 hours')
            ORDER BY created_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, kind, owner_type, owner_id, from_email, from_name, to_email,
			cc_emails, bcc_emails, subject, body, body_html, attachment_ids, reply_to,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	rows, err := r.db.Pool().Query(ctx, sql, maxAttempts, backoffBaseSeconds, limit)
	if err != nil {
		return nil, fmt.Errorf("claim failed emails for retry: %w", err)
	}
	defer rows.Close()

	var out []*EmailMessage
	for rows.Next() {
		var m EmailMessage
		if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
			&m.ToEmail, &m.CCEmails, &m.BCCEmails, &m.Subject, &m.Body, &m.BodyHTML,
			&m.AttachmentIDs, &m.ReplyTo, &m.Status, &m.SentAt, &m.LastError, &m.Attempts,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan claimed email: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (r *EmailRepository) claimFailedTenantScoped(ctx context.Context, limit, maxAttempts, backoffBaseSeconds int) ([]*EmailMessage, error) {
	tenantID, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, tenantdb.ErrNoTenant
	}
	const sql = `
		UPDATE email_messages
		SET status = 'sending', attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM email_messages
			WHERE (status = 'queued' OR (status = 'failed'
				AND updated_at < now() - make_interval(secs => (attempts * $2)::double precision)))
				AND attempts < $1
            AND (attempts = 0 OR created_at > now() - interval '23 hours')
            ORDER BY created_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, kind, owner_type, owner_id, from_email, from_name,
			to_email_enc, subject_enc, body_enc, body_html_enc, attachment_ids, reply_to_enc,
			status, sent_at, last_error, attempts, created_at, updated_at
	`
	var out []*EmailMessage
	err := r.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, maxAttempts, backoffBaseSeconds, limit)
		if err != nil {
			return fmt.Errorf("claim failed emails for retry (tenant-scoped): %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m EmailMessage
			var toEnc, subjectEnc, bodyEnc, bodyHTMLEnc, replyToEnc []byte
			if err := rows.Scan(&m.ID, &m.Kind, &m.OwnerType, &m.OwnerID, &m.FromEmail, &m.FromName,
				&toEnc, &subjectEnc, &bodyEnc, &bodyHTMLEnc, &m.AttachmentIDs, &replyToEnc,
				&m.Status, &m.SentAt, &m.LastError, &m.Attempts, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return fmt.Errorf("scan claimed email (tenant-scoped): %w", err)
			}
			m.ToEmail = string(toEnc)
			m.Subject = string(subjectEnc)
			m.Body = string(bodyEnc)
			if bodyHTMLEnc != nil {
				m.BodyHTML = string(bodyHTMLEnc)
			}
			if replyToEnc != nil {
				m.ReplyTo = string(replyToEnc)
			}
			// The delivery path needs plaintext: unseal before the row
			// reaches the sender.
			if r.sealer != nil {
				if err := r.sealer.UnsealMessageFields(ctx, tx, &m); err != nil {
					return err
				}
			}
			out = append(out, &m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RequeueStale moves rows stuck in 'sending' for longer than olderThan
// back to 'failed' so a worker crash/kill mid-delivery doesn't strand
// them forever. It deliberately leaves updated_at untouched (rather than
// bumping it to now()): the row has already waited olderThan, which is
// far longer than any retry backoff window, so it becomes immediately
// eligible for ClaimFailedForRetry instead of having to wait out another
// backoff period on top of the time it was already stuck.
func (r *EmailRepository) RequeueStale(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	if r.tenantScoped {
		tenantID, ok := tenantdb.TenantFromContext(ctx)
		if !ok {
			return 0, tenantdb.ErrNoTenant
		}
		const sql = `
			UPDATE email_messages
			SET status = 'failed',
				last_error = 'requeued: stuck in sending past timeout'
			WHERE status = 'sending' AND updated_at < $1
		`
		var n int
		err := r.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, sql, cutoff)
			n = int(tag.RowsAffected())
			return err
		})
		if err != nil {
			return 0, fmt.Errorf("requeue stale sending emails (tenant-scoped): %w", err)
		}
		return n, nil
	}
	const sql = `
		UPDATE email_messages
		SET status = 'failed',
			last_error = 'requeued: stuck in sending past timeout'
		WHERE status = 'sending' AND updated_at < $1
	`
	tag, err := r.db.Pool().Exec(ctx, sql, cutoff)
	if err != nil {
		return 0, fmt.Errorf("requeue stale sending emails: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
