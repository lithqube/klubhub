package outreach

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgUniqueViolation is the SQLSTATE Postgres returns for a unique-index
// collision.
const pgUniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

// Repository provides DB access for the outreach package. Single-user DJ
// side — no tenant scoping (consistent with contacts/venues/gigs).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ─── Slots ───────────────────────────────────────────────────────────────────

const slotColumns = `id, date, city, country, genres, fee_min, fee_max, currency, notes, created_at`

func scanSlot(row pgx.Row) (*Slot, error) {
	var s Slot
	var d time.Time
	err := row.Scan(&s.ID, &d, &s.City, &s.Country, &s.Genres, &s.FeeMin, &s.FeeMax, &s.Currency, &s.Notes, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	s.Date = Date(d)
	return &s, nil
}

// ListSlots returns all availability slots, soonest first.
func (r *Repository) ListSlots(ctx context.Context) ([]*Slot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+slotColumns+` FROM outreach_availability_slots ORDER BY date ASC, city ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Slot
	for rows.Next() {
		s, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetSlot returns one slot, or ErrNotFound.
func (r *Repository) GetSlot(ctx context.Context, id uuid.UUID) (*Slot, error) {
	s, err := scanSlot(r.pool.QueryRow(ctx, `SELECT `+slotColumns+` FROM outreach_availability_slots WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// CreateSlot inserts a slot. The UNIQUE (date, city, country) constraint
// maps to ErrConflict.
func (r *Repository) CreateSlot(ctx context.Context, in *SlotCreate) (*Slot, error) {
	s, err := scanSlot(r.pool.QueryRow(ctx, `
		INSERT INTO outreach_availability_slots (date, city, country, genres, fee_min, fee_max, currency, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+slotColumns,
		time.Time(in.Date), in.City, in.Country, in.Genres, in.FeeMin, in.FeeMax, in.Currency, in.Notes,
	))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// DeleteSlot removes a slot. Sent-log rows keep their history via
// ON DELETE SET NULL.
func (r *Repository) DeleteSlot(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM outreach_availability_slots WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Templates ───────────────────────────────────────────────────────────────

const templateColumns = `id, name, direction, subject, body, is_default, version, created_at, updated_at`

func scanTemplate(row pgx.Row) (*Template, error) {
	var t Template
	err := row.Scan(&t.ID, &t.Name, &t.Direction, &t.Subject, &t.Body, &t.IsDefault, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTemplates returns all non-deleted templates, default first.
func (r *Repository) ListTemplates(ctx context.Context) ([]*Template, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+templateColumns+`
		FROM outreach_templates
		WHERE deleted_at IS NULL
		ORDER BY is_default DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTemplate returns one non-deleted template, or ErrNotFound.
func (r *Repository) GetTemplate(ctx context.Context, id uuid.UUID) (*Template, error) {
	t, err := scanTemplate(r.pool.QueryRow(ctx, `
		SELECT `+templateColumns+`
		FROM outreach_templates
		WHERE id = $1 AND deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetDefaultTemplate returns the default dj_to_promoter template, or
// ErrNotFound when none is set.
func (r *Repository) GetDefaultTemplate(ctx context.Context) (*Template, error) {
	t, err := scanTemplate(r.pool.QueryRow(ctx, `
		SELECT `+templateColumns+`
		FROM outreach_templates
		WHERE direction = 'dj_to_promoter' AND is_default AND deleted_at IS NULL`))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateTemplate inserts a template. Setting is_default clears the previous
// default in the same transaction (one default per direction).
func (r *Repository) CreateTemplate(ctx context.Context, in *TemplateCreate) (*Template, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if in.IsDefault {
		if _, err := tx.Exec(ctx, `
			UPDATE outreach_templates SET is_default = false, updated_at = now()
			WHERE direction = 'dj_to_promoter' AND is_default AND deleted_at IS NULL`); err != nil {
			return nil, err
		}
	}
	t, err := scanTemplate(tx.QueryRow(ctx, `
		INSERT INTO outreach_templates (name, direction, subject, body, is_default)
		VALUES ($1, 'dj_to_promoter', $2, $3, $4)
		RETURNING `+templateColumns,
		in.Name, in.Subject, in.Body, in.IsDefault,
	))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTemplate applies a pointer-patch update with optimistic concurrency
// on the version column: the client's version must match, the row's version
// is incremented. Zero rows updated means either the template is gone
// (ErrNotFound) or the version moved (ErrConflict).
func (r *Repository) UpdateTemplate(ctx context.Context, id uuid.UUID, in *TemplateUpdate) (*Template, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if in.IsDefault != nil && *in.IsDefault {
		if _, err := tx.Exec(ctx, `
			UPDATE outreach_templates SET is_default = false, updated_at = now()
			WHERE direction = 'dj_to_promoter' AND is_default AND deleted_at IS NULL AND id <> $1`, id); err != nil {
			return nil, err
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE outreach_templates
		SET name = COALESCE($2, name),
		    subject = COALESCE($3, subject),
		    body = COALESCE($4, body),
		    is_default = COALESCE($5, is_default),
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1 AND version = $6 AND deleted_at IS NULL`,
		id, in.Name, in.Subject, in.Body, in.IsDefault, in.Version,
	)
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if qerr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM outreach_templates WHERE id = $1 AND deleted_at IS NULL)`, id).Scan(&exists); qerr != nil {
			return nil, qerr
		}
		if !exists {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}
	t, err := scanTemplate(tx.QueryRow(ctx, `SELECT `+templateColumns+` FROM outreach_templates WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteTemplate soft-deletes a template. Sent-log rows keep their stored
// copy via ON DELETE SET NULL.
func (r *Repository) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE outreach_templates SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Requests (sent log) ─────────────────────────────────────────────────────

const requestColumns = `r.id, r.contact_id, r.availability_slot_id, r.template_id, r.epk_export_id,
	r.email_message_id, r.to_email, r.subject, r.body, r.status, r.sent_at, r.created_at,
	e.status, e.last_error`

func scanRequest(row pgx.Row) (*Request, error) {
	var q Request
	var emailStatus, emailError *string
	err := row.Scan(
		&q.ID, &q.ContactID, &q.AvailabilitySlotID, &q.TemplateID, &q.EPKExportID,
		&q.EmailMessageID, &q.ToEmail, &q.Subject, &q.Body, &q.Status, &q.SentAt, &q.CreatedAt,
		&emailStatus, &emailError,
	)
	if err != nil {
		return nil, err
	}
	if emailStatus != nil {
		q.EmailStatus = *emailStatus
	}
	if emailError != nil {
		q.EmailError = *emailError
	}
	return &q, nil
}

// InsertRequestWithDailyCap checks the daily cap and inserts the request row
// in ONE transaction (SPEC §9 v2.1): an advisory lock serializes concurrent
// sends, the count covers requests since `since` (failed attempts count —
// the row exists before the send), and the insert makes check-and-insert
// atomic so two concurrent sends cannot both pass at 49/50. Returns
// ErrRateLimited when the cap is reached.
func (r *Repository) InsertRequestWithDailyCap(ctx context.Context, row *Request, maxPerDay int, since time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('klubhub:outreach:daily_cap'))`); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM outreach_requests WHERE created_at >= $1`, since).Scan(&count); err != nil {
		return err
	}
	if count >= maxPerDay {
		return ErrRateLimited
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO outreach_requests (contact_id, availability_slot_id, template_id, epk_export_id, to_email, subject, body, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'queued')
		RETURNING id, created_at`,
		row.ContactID, row.AvailabilitySlotID, row.TemplateID, row.EPKExportID,
		row.ToEmail, row.Subject, row.Body,
	).Scan(&row.ID, &row.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListRequests returns the sent log, newest first, with delivery status
// joined from email_messages.
func (r *Repository) ListRequests(ctx context.Context) ([]*Request, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+requestColumns+`
		FROM outreach_requests r
		LEFT JOIN email_messages e ON e.id = r.email_message_id
		ORDER BY r.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Request
	for rows.Next() {
		q, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// GetRequest returns one sent-log row with delivery status, or ErrNotFound.
func (r *Repository) GetRequest(ctx context.Context, id uuid.UUID) (*Request, error) {
	q, err := scanRequest(r.pool.QueryRow(ctx, `
		SELECT `+requestColumns+`
		FROM outreach_requests r
		LEFT JOIN email_messages e ON e.id = r.email_message_id
		WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return q, nil
}

// UpdateRequestSent links the outbox row (nil when the send failed before an
// outbox row existed) and syncs the delivery status.
func (r *Repository) UpdateRequestSent(ctx context.Context, id uuid.UUID, emailMessageID *uuid.UUID, status string, sentAt *time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE outreach_requests
		SET email_message_id = $2, status = $3, sent_at = $4
		WHERE id = $1`, id, emailMessageID, status, sentAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Suppressions ────────────────────────────────────────────────────────────

// ListSuppressions returns all suppressions, newest first.
func (r *Repository) ListSuppressions(ctx context.Context) ([]*Suppression, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, email, reason, created_at
		FROM outreach_suppressions
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Suppression
	for rows.Next() {
		var s Suppression
		if err := rows.Scan(&s.ID, &s.Email, &s.Reason, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// AddSuppression inserts a suppression. Adding an already-suppressed
// address is idempotent: the existing row is returned.
func (r *Repository) AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error) {
	var s Suppression
	err := r.pool.QueryRow(ctx, `
		INSERT INTO outreach_suppressions (email, reason)
		VALUES ($1, $2)
		ON CONFLICT (email) DO NOTHING
		RETURNING id, email, reason, created_at`,
		in.Email, in.Reason,
	).Scan(&s.ID, &s.Email, &s.Reason, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.pool.QueryRow(ctx, `
			SELECT id, email, reason, created_at
			FROM outreach_suppressions
			WHERE email = $1`, in.Email,
		).Scan(&s.ID, &s.Email, &s.Reason, &s.CreatedAt)
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSuppression removes a suppression by id.
func (r *Repository) DeleteSuppression(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM outreach_suppressions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IsSuppressed reports whether the normalized email is suppressed.
func (r *Repository) IsSuppressed(ctx context.Context, normalizedEmail string) (bool, error) {
	var suppressed bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM outreach_suppressions WHERE email = $1)`, normalizedEmail).Scan(&suppressed)
	return suppressed, err
}

// ─── Matching (read-only against contacts/venues/gigs) ──────────────────────

// GetPromoterContact loads one contact for the send path. Only promoters
// are sendable; anything else is ErrNotFound.
func (r *Repository) GetPromoterContact(ctx context.Context, id uuid.UUID) (*MatchedContact, error) {
	var c MatchedContact
	var company *string
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, company, email, (email <> '') AS has_email, city
		FROM contacts
		WHERE id = $1 AND type = 'promoter' AND deleted_at IS NULL`, id,
	).Scan(&c.ID, &c.Name, &company, &c.Email, &c.HasEmail, &c.City)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if company != nil {
		c.Company = *company
	}
	return &c, nil
}

// MatchContacts returns the DJ's promoter contacts in the slot's city
// (case-insensitive, trimmed), excluding suppressed addresses. Contacts
// without an email are included with HasEmail=false (OUTR-05 v2.1). Ranking
// (SPEC §8): a past gig in that city first — the join key is normalized
// gigs.promoter_email = contacts.email (gigs has no promoter-contact FK) —
// then alphabetical.
func (r *Repository) MatchContacts(ctx context.Context, city string) ([]*MatchedContact, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.name, c.company, c.email, (c.email <> '') AS has_email, c.city,
		       EXISTS (
		         SELECT 1 FROM gigs g
		         WHERE g.deleted_at IS NULL
		           AND g.date < now()
		           AND c.email <> '' AND g.promoter_email <> ''
		           AND lower(trim(g.promoter_email)) = lower(trim(c.email))
		           AND lower(trim(g.city)) = lower(trim($1))
		       ) AS past_gig_in_city
		FROM contacts c
		WHERE c.type = 'promoter'
		  AND c.deleted_at IS NULL
		  AND lower(trim(c.city)) = lower(trim($1))
		  AND NOT EXISTS (
		    SELECT 1 FROM outreach_suppressions s
		    WHERE c.email <> '' AND s.email = lower(trim(c.email))
		  )
		ORDER BY past_gig_in_city DESC, c.name ASC`, city)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MatchedContact
	for rows.Next() {
		var c MatchedContact
		var company *string
		if err := rows.Scan(&c.ID, &c.Name, &company, &c.Email, &c.HasEmail, &c.City, &c.PastGigInCity); err != nil {
			return nil, err
		}
		if company != nil {
			c.Company = *company
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// MatchVenues returns venues in the slot's city as context.
func (r *Repository) MatchVenues(ctx context.Context, city string) ([]*MatchedVenue, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, city
		FROM venues
		WHERE deleted_at IS NULL AND lower(trim(city)) = lower(trim($1))
		ORDER BY name ASC`, city)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MatchedVenue
	for rows.Next() {
		var v MatchedVenue
		if err := rows.Scan(&v.ID, &v.Name, &v.City); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// ─── EPK exports + DJ contact info ───────────────────────────────────────────

// GetEPKExport loads one export row for the attach path.
func (r *Repository) GetEPKExport(ctx context.Context, id uuid.UUID) (*EPKExportRef, error) {
	var e EPKExportRef
	err := r.pool.QueryRow(ctx, `SELECT id, garage_object_key FROM epk_exports WHERE id = $1`, id).
		Scan(&e.ID, &e.GarageObjectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetContactInfoJSON returns the raw user_settings.contact_info JSON text
// (the {name, email, phone} block the EPK contact section writes), or "" when
// no settings row exists yet.
func (r *Repository) GetContactInfoJSON(ctx context.Context) (string, error) {
	var raw string
	err := r.pool.QueryRow(ctx, `
		SELECT contact_info
		FROM user_settings
		WHERE deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT 1`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return raw, nil
}
