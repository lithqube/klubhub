package bookings

// Pure-SQL pgx functions for the bookings tables. Every function takes the
// pgx.Tx of the caller's tenantdb.WithTenant scope: RLS (ENABLE + FORCE)
// applies, and tenant_id is written explicitly so the tenant_isolation policy
// re-checks it. The service layer owns WithTenant and the seal/unseal — the
// DEK lookup must share the transaction with the reads/writes it protects
// (the audience pattern). This file deliberately imports neither pgxpool nor
// tenantdb: the promoter dbaccess guard forbids a pool import under
// internal/promoter, and scoping is the service's job.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgUniqueViolation / pgForeignKeyViolation are the SQLSTATE Postgres
// returns for a unique-index collision / a missing referenced row.
const (
	pgUniqueViolation      = "23505"
	pgForeignKeyViolation  = "23503"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

// isForeignKeyViolation reports a missing referenced row. Inside WithTenant
// an invisible (cross-tenant) parent fails the FK the same way a missing one
// does, so callers map it to ErrNotFound — the response is identical for
// "does not exist" and "not yours", which leaks nothing.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation
}

// ─── Talent contacts ─────────────────────────────────────────────────────────

// talentRow is the sealed shape read straight off talent_contacts, before
// decryption.
type talentRow struct {
	id                   uuid.UUID
	nameEnc, emailEnc    []byte
	phoneEnc             []byte
	nameBidx, emailBidx  []byte
	city, country        string
	genres               []string
	source, notes        string
	createdAt, updatedAt time.Time
}

const talentColumns = `id, name_enc, email_enc, phone_enc, name_bidx, email_bidx,
	city, country, genres, source, notes, created_at, updated_at`

func scanTalentRow(row pgx.Row) (talentRow, error) {
	var r talentRow
	err := row.Scan(&r.id, &r.nameEnc, &r.emailEnc, &r.phoneEnc, &r.nameBidx, &r.emailBidx,
		&r.city, &r.country, &r.genres, &r.source, &r.notes, &r.createdAt, &r.updatedAt)
	return r, err
}

// talentQuery is the repository-level talent filter. The service resolves
// the blind indexes (they need the tenant DEK) before calling.
type talentQuery struct {
	city      string
	genre     string
	nameBidx  []byte
	emailBidx []byte
}

// insertTalentContact inserts one contact. The unique (tenant_id, email_bidx)
// index maps to ErrDuplicate.
func insertTalentContact(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, id uuid.UUID,
	nameEnc, emailEnc, phoneEnc, nameBidx, emailBidx []byte, in *TalentContactInput,
) (createdAt, updatedAt time.Time, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO talent_contacts
			(id, tenant_id, name_enc, email_enc, phone_enc, name_bidx, email_bidx,
			 city, country, genres, source, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at`,
		id, tenant, nameEnc, emailEnc, phoneEnc, nameBidx, emailBidx,
		in.City, in.Country, in.Genres, in.Source, in.Notes,
	).Scan(&createdAt, &updatedAt)
	if isUniqueViolation(err) {
		return time.Time{}, time.Time{}, ErrDuplicate
	}
	return createdAt, updatedAt, err
}

// getTalentContact loads one contact, or ErrNotFound.
func getTalentContact(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (talentRow, error) {
	r, err := scanTalentRow(tx.QueryRow(ctx,
		`SELECT `+talentColumns+` FROM talent_contacts WHERE id = $1 AND tenant_id = $2`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return talentRow{}, ErrNotFound
	}
	return r, err
}

// listTalentContacts returns the contacts matching q, newest first. Filters:
// city (case-insensitive, trimmed), genre overlap, and Q (exact blind-index
// match on name or email — the service computes both indexes).
func listTalentContacts(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, q talentQuery) ([]talentRow, error) {
	where := `WHERE tenant_id = $1`
	args := []any{tenant}
	if q.city != "" {
		args = append(args, q.city)
		where += fmt.Sprintf(` AND lower(trim(city)) = lower(trim($%d))`, len(args))
	}
	if q.genre != "" {
		args = append(args, q.genre)
		where += fmt.Sprintf(` AND genres && ARRAY[$%d]::text[]`, len(args))
	}
	if q.nameBidx != nil || q.emailBidx != nil {
		where += fmt.Sprintf(` AND (name_bidx = $%d OR email_bidx = $%d)`, len(args)+1, len(args)+2)
		args = append(args, q.nameBidx, q.emailBidx)
	}
	rows, err := tx.Query(ctx,
		`SELECT `+talentColumns+` FROM talent_contacts `+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []talentRow
	for rows.Next() {
		r, err := scanTalentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// updateTalentContact replaces the editable fields of one contact (full
// replace, audience UpdateContact semantics). Zero rows: ErrNotFound; a
// unique-index collision on the new email: ErrDuplicate.
func updateTalentContact(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID,
	nameEnc, emailEnc, phoneEnc, nameBidx, emailBidx []byte, in *TalentContactInput,
) (time.Time, error) {
	var updatedAt time.Time
	err := tx.QueryRow(ctx, `
		UPDATE talent_contacts
		SET name_enc = $3, email_enc = $4, phone_enc = $5, name_bidx = $6, email_bidx = $7,
		    city = $8, country = $9, genres = $10, source = $11, notes = $12, updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING updated_at`,
		id, tenant, nameEnc, emailEnc, phoneEnc, nameBidx, emailBidx,
		in.City, in.Country, in.Genres, in.Source, in.Notes,
	).Scan(&updatedAt)
	if isUniqueViolation(err) {
		return time.Time{}, ErrDuplicate
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	return updatedAt, err
}

// deleteTalentContact hard-deletes one contact. The sent log survives via
// outreach_requests.talent_contact_id ON DELETE SET NULL (00017).
func deleteTalentContact(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `DELETE FROM talent_contacts WHERE id = $1 AND tenant_id = $2`, id, tenant)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Open slots ──────────────────────────────────────────────────────────────

const slotColumns = `id, event_id, role, genres, fee_min, fee_max, currency, notes, created_at`

func scanSlot(row pgx.Row) (OpenSlot, error) {
	var s OpenSlot
	var feeMin, feeMax *float64
	var currency *string
	err := row.Scan(&s.ID, &s.EventID, &s.Role, &s.Genres, &feeMin, &feeMax, &currency, &s.Notes, &s.CreatedAt)
	if err != nil {
		return OpenSlot{}, err
	}
	s.FeeMin, s.FeeMax, s.Currency = feeMin, feeMax, currency
	return s, nil
}

// insertOpenSlot inserts one slot on the tenant's event. UNIQUE (event_id,
// role) maps to ErrConflict; a missing/invisible event (FK) to ErrNotFound.
func insertOpenSlot(ctx context.Context, tx pgx.Tx, tenant, eventID uuid.UUID, in *OpenSlotInput) (OpenSlot, error) {
	s, err := scanSlot(tx.QueryRow(ctx, `
		INSERT INTO event_open_slots (id, tenant_id, event_id, role, genres, fee_min, fee_max, currency, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+slotColumns,
		uuid.New(), tenant, eventID, in.Role, in.Genres, in.FeeMin, in.FeeMax, in.Currency, in.Notes,
	))
	if isUniqueViolation(err) {
		return OpenSlot{}, ErrConflict
	}
	if isForeignKeyViolation(err) {
		return OpenSlot{}, ErrNotFound
	}
	if err != nil {
		return OpenSlot{}, err
	}
	return s, nil
}

// listOpenSlots returns the slots of one event, creation order.
func listOpenSlots(ctx context.Context, tx pgx.Tx, tenant, eventID uuid.UUID) ([]OpenSlot, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+slotColumns+` FROM event_open_slots WHERE event_id = $1 AND tenant_id = $2 ORDER BY created_at ASC`,
		eventID, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OpenSlot
	for rows.Next() {
		s, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// getOpenSlot loads one slot, or ErrNotFound. RLS makes a cross-tenant slot
// invisible, so the service can map this to ErrForbidden without leaking
// existence.
func getOpenSlot(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (OpenSlot, error) {
	s, err := scanSlot(tx.QueryRow(ctx,
		`SELECT `+slotColumns+` FROM event_open_slots WHERE id = $1 AND tenant_id = $2`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return OpenSlot{}, ErrNotFound
	}
	if err != nil {
		return OpenSlot{}, err
	}
	return s, nil
}

// deleteOpenSlot removes one slot. Sent-log rows keep their history via
// ON DELETE SET NULL.
func deleteOpenSlot(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `DELETE FROM event_open_slots WHERE id = $1 AND tenant_id = $2`, id, tenant)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Matching (SPEC §8) ──────────────────────────────────────────────────────

// matchRow is one matched contact plus its genre overlap with the event.
type matchRow struct {
	talentRow
	overlapGenres []string
}

// matchTalentForSlot returns the tenant's talent contacts matching the
// event's city (case-insensitive, trimmed). When BOTH the event and the
// contact carry genres, overlap is required (event.genres && talent.genres);
// when either side is empty, the city alone matches. Suppressed addresses are
// excluded by blind index; email-less contacts are included (OFFER-10).
// Ranking: genre-overlap count desc, then most recently added.
func matchTalentForSlot(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, eventGenres []string, eventCity string) ([]matchRow, error) {
	if eventGenres == nil {
		eventGenres = []string{}
	}
	// Postgres has no array-intersection operator: the overlap list is the
	// contact's genres filtered by membership in the event's (unnest + ANY),
	// and the ranking counts that same intersection.
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.name_enc, t.email_enc, t.phone_enc, t.name_bidx, t.email_bidx,
		       t.city, t.country, t.genres, t.source, t.notes, t.created_at, t.updated_at,
		       COALESCE((SELECT array_agg(g) FROM unnest(t.genres) AS g WHERE g = ANY($2::text[])), '{}') AS overlap_genres
		FROM talent_contacts t
		WHERE t.tenant_id = $1
		  AND lower(trim(t.city)) = lower(trim($3))
		  AND (cardinality($2::text[]) = 0 OR cardinality(t.genres) = 0 OR t.genres && $2::text[])
		  AND (t.email_bidx IS NULL OR NOT EXISTS (
		    SELECT 1 FROM outreach_suppressions s
		    WHERE s.tenant_id = $1 AND s.email_bidx = t.email_bidx
		  ))
		ORDER BY (SELECT count(*) FROM unnest(t.genres) AS g WHERE g = ANY($2::text[])) DESC, t.created_at DESC`,
		tenant, eventGenres, eventCity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []matchRow
	for rows.Next() {
		var m matchRow
		err := rows.Scan(&m.id, &m.nameEnc, &m.emailEnc, &m.phoneEnc, &m.nameBidx, &m.emailBidx,
			&m.city, &m.country, &m.genres, &m.source, &m.notes, &m.createdAt, &m.updatedAt,
			&m.overlapGenres)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ─── Templates ───────────────────────────────────────────────────────────────

const templateColumns = `id, name, direction, subject, body, is_default, version, created_at, updated_at`

func scanTemplate(row pgx.Row) (Template, error) {
	var t Template
	err := row.Scan(&t.ID, &t.Name, &t.Direction, &t.Subject, &t.Body, &t.IsDefault, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// listTemplates returns all non-deleted templates, default first.
func listTemplates(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Template, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+templateColumns+`
		FROM outreach_templates
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY is_default DESC, name ASC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// getTemplate loads one non-deleted template, or ErrNotFound.
func getTemplate(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Template, error) {
	t, err := scanTemplate(tx.QueryRow(ctx, `
		SELECT `+templateColumns+`
		FROM outreach_templates
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	return t, nil
}

// getDefaultTemplate loads the tenant's default promoter_to_dj template, or
// ErrNotFound when none is set (the service then falls back to the built-in
// default — organisations created after the 00017 seed have no row).
func getDefaultTemplate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (Template, error) {
	t, err := scanTemplate(tx.QueryRow(ctx, `
		SELECT `+templateColumns+`
		FROM outreach_templates
		WHERE tenant_id = $1 AND direction = 'promoter_to_dj' AND is_default AND deleted_at IS NULL`, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	return t, nil
}

// createTemplate inserts a template. Setting is_default clears the previous
// default in the same transaction (one default per tenant, enforced by the
// partial unique index). A name collision maps to ErrConflict.
func createTemplate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, in *TemplateCreate) (Template, error) {
	if in.IsDefault {
		if _, err := tx.Exec(ctx, `
			UPDATE outreach_templates SET is_default = false, updated_at = now()
			WHERE tenant_id = $1 AND direction = 'promoter_to_dj' AND is_default AND deleted_at IS NULL`, tenant); err != nil {
			return Template{}, err
		}
	}
	t, err := scanTemplate(tx.QueryRow(ctx, `
		INSERT INTO outreach_templates (id, tenant_id, direction, name, subject, body, is_default)
		VALUES ($1, $2, 'promoter_to_dj', $3, $4, $5, $6)
		RETURNING `+templateColumns,
		uuid.New(), tenant, in.Name, in.Subject, in.Body, in.IsDefault,
	))
	if isUniqueViolation(err) {
		return Template{}, ErrConflict
	}
	if err != nil {
		return Template{}, err
	}
	return t, nil
}

// updateTemplate applies a pointer-patch update with optimistic concurrency
// on the version column: the client's version must match, the row's version
// is incremented. Zero rows updated means either the template is gone
// (ErrNotFound) or the version moved (ErrConflict). Setting is_default clears
// the other defaults in the same transaction.
func updateTemplate(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, in *TemplateUpdate) (Template, error) {
	if in.IsDefault != nil && *in.IsDefault {
		if _, err := tx.Exec(ctx, `
			UPDATE outreach_templates SET is_default = false, updated_at = now()
			WHERE tenant_id = $1 AND direction = 'promoter_to_dj' AND is_default AND deleted_at IS NULL AND id <> $2`,
			tenant, id); err != nil {
			return Template{}, err
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE outreach_templates
		SET name = COALESCE($3, name),
		    subject = COALESCE($4, subject),
		    body = COALESCE($5, body),
		    is_default = COALESCE($6, is_default),
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND version = $7 AND deleted_at IS NULL`,
		id, tenant, in.Name, in.Subject, in.Body, in.IsDefault, in.Version,
	)
	if isUniqueViolation(err) {
		return Template{}, ErrConflict
	}
	if err != nil {
		return Template{}, err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if qerr := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM outreach_templates WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL)`,
			id, tenant).Scan(&exists); qerr != nil {
			return Template{}, qerr
		}
		if !exists {
			return Template{}, ErrNotFound
		}
		return Template{}, ErrConflict
	}
	return getTemplate(ctx, tx, tenant, id)
}

// deleteTemplate soft-deletes a template. Sent-log rows keep their stored
// copy via ON DELETE SET NULL.
func deleteTemplate(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE outreach_templates SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenant)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Offers (sent log) ───────────────────────────────────────────────────────

// offerRow is the sealed shape of one outreach_requests row, plus the live
// delivery status joined from email_messages.
type offerRow struct {
	id              uuid.UUID
	eventID         *uuid.UUID
	openSlotID      *uuid.UUID
	talentContactID *uuid.UUID
	templateID      *uuid.UUID
	emailMessageID  *uuid.UUID
	toEmailEnc      []byte
	toEmailBidx     []byte
	subjectEnc      []byte
	bodyEnc         []byte
	status          string
	sentAt          *time.Time
	createdAt       time.Time
	emailStatus     *string
	emailError      *string
}

const offerColumns = `r.id, r.event_id, r.open_slot_id, r.talent_contact_id, r.template_id,
	r.email_message_id, r.to_email_enc, r.to_email_bidx, r.subject_enc, r.body_enc,
	r.status, r.sent_at, r.created_at, e.status, e.last_error`

func scanOffer(row pgx.Row) (offerRow, error) {
	var q offerRow
	err := row.Scan(
		&q.id, &q.eventID, &q.openSlotID, &q.talentContactID, &q.templateID,
		&q.emailMessageID, &q.toEmailEnc, &q.toEmailBidx, &q.subjectEnc, &q.bodyEnc,
		&q.status, &q.sentAt, &q.createdAt, &q.emailStatus, &q.emailError,
	)
	return q, err
}

// insertOfferWithDailyCap checks the daily cap and inserts the request row
// in the CALLER's transaction (SPEC §9 v2.1): a per-tenant advisory lock
// serializes concurrent sends, the count covers requests since `since`
// (failed attempts count — the row exists before the send), and the insert
// makes check-and-insert atomic so two concurrent sends cannot both pass at
// 49/50. Returns ErrRateLimited when the cap is reached.
func insertOfferWithDailyCap(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, row *offerRow, maxPerDay int, since time.Time) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text))`, tenant.String()); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM outreach_requests WHERE tenant_id = $1 AND created_at >= $2`,
		tenant, since).Scan(&count); err != nil {
		return err
	}
	if count >= maxPerDay {
		return ErrRateLimited
	}
	return tx.QueryRow(ctx, `
		INSERT INTO outreach_requests
			(id, tenant_id, event_id, open_slot_id, talent_contact_id, template_id,
			 to_email_enc, to_email_bidx, subject_enc, body_enc, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'queued')
		RETURNING created_at`,
		row.id, tenant, row.eventID, row.openSlotID, row.talentContactID, row.templateID,
		row.toEmailEnc, row.toEmailBidx, row.subjectEnc, row.bodyEnc,
	).Scan(&row.createdAt)
}

// listOffers returns the sent log, newest first, with the live delivery
// status joined from email_messages.
func listOffers(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]offerRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+offerColumns+`
		FROM outreach_requests r
		LEFT JOIN email_messages e ON e.id = r.email_message_id
		WHERE r.tenant_id = $1
		ORDER BY r.created_at DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offerRow
	for rows.Next() {
		q, err := scanOffer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// getOffer loads one sent-log row, or ErrNotFound.
func getOffer(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (offerRow, error) {
	q, err := scanOffer(tx.QueryRow(ctx, `
		SELECT `+offerColumns+`
		FROM outreach_requests r
		LEFT JOIN email_messages e ON e.id = r.email_message_id
		WHERE r.id = $1 AND r.tenant_id = $2`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return offerRow{}, ErrNotFound
	}
	if err != nil {
		return offerRow{}, err
	}
	return q, nil
}

// updateOfferSent links the outbox row (nil: no message was created) and
// syncs the delivery status.
func updateOfferSent(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, emailMessageID *uuid.UUID, status string, sentAt *time.Time) error {
	tag, err := tx.Exec(ctx, `
		UPDATE outreach_requests
		SET email_message_id = $3, status = $4, sent_at = $5
		WHERE id = $1 AND tenant_id = $2`, id, tenant, emailMessageID, status, sentAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Suppressions ────────────────────────────────────────────────────────────

// listSuppressions returns all suppressions, newest first. The email itself
// is never stored — only its blind index (SPEC §9) — so the list cannot show
// the address, by design.
func listSuppressions(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Suppression, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, reason, created_at
		FROM outreach_suppressions
		WHERE tenant_id = $1
		ORDER BY created_at DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Suppression
	for rows.Next() {
		var s Suppression
		if err := rows.Scan(&s.ID, &s.Reason, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// insertSuppression stores one do-not-contact blind index. The blind index
// deliberately reuses the ("talent_contacts", "email_bidx") scope so a
// suppression row matches the contact row directly in SQL
// (s.email_bidx = t.email_bidx) and SuppressFromContact can copy the
// contact's stored index. A duplicate maps to ErrDuplicate.
func insertSuppression(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, bidx []byte, reason string) (Suppression, error) {
	var s Suppression
	err := tx.QueryRow(ctx, `
		INSERT INTO outreach_suppressions (id, tenant_id, email_bidx, reason)
		VALUES ($1, $2, $3, $4)
		RETURNING id, reason, created_at`,
		uuid.New(), tenant, bidx, reason,
	).Scan(&s.ID, &s.Reason, &s.CreatedAt)
	if isUniqueViolation(err) {
		return Suppression{}, ErrDuplicate
	}
	if err != nil {
		return Suppression{}, err
	}
	return s, nil
}

// deleteSuppression removes a suppression by id.
func deleteSuppression(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `DELETE FROM outreach_suppressions WHERE id = $1 AND tenant_id = $2`, id, tenant)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// isSuppressedBidx reports whether the blind index is suppressed.
func isSuppressedBidx(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, bidx []byte) (bool, error) {
	if bidx == nil {
		return false, nil
	}
	var suppressed bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM outreach_suppressions WHERE tenant_id = $1 AND email_bidx = $2)`,
		tenant, bidx).Scan(&suppressed)
	return suppressed, err
}

// ─── Org / member / event lookups (send path) ───────────────────────────────

// getOrgProfile loads the org's display name and timezone (the daily-cap
// window is local midnight in the ORG's timezone — SPEC §9 v2.1).
func getOrgProfile(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (name, timezone string, err error) {
	err = tx.QueryRow(ctx, `SELECT name, timezone FROM organizations WHERE id = $1`, tenant).Scan(&name, &timezone)
	return name, timezone, err
}

// getMemberSealed loads the sending member's sealed email + display name.
// ReplyTo = the member's email (OFFER-06 v2.1): orgs have no contact email
// (00007 keeps contacts out of the public profile), so the member's address
// is where DJ replies must land.
func getMemberSealed(ctx context.Context, tx pgx.Tx, tenant, userID uuid.UUID) (emailEnc, displayNameEnc []byte, err error) {
	err = tx.QueryRow(ctx,
		`SELECT email_enc, display_name_enc FROM users WHERE id = $1 AND tenant_id = $2`,
		userID, tenant).Scan(&emailEnc, &displayNameEnc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	return emailEnc, displayNameEnc, err
}

// eventOffer is the event shape the offer render path needs.
type eventOffer struct {
	title            string
	startsAt         time.Time
	timezone         string
	city             string
	locationMode     string
	locationRevealAt *time.Time
	genres           []string
	venueName        string
}

// getEventForOffer loads one event with its venue name ('' when none).
func getEventForOffer(ctx context.Context, tx pgx.Tx, tenant, eventID uuid.UUID) (eventOffer, error) {
	var ev eventOffer
	err := tx.QueryRow(ctx, `
		SELECT e.title, e.starts_at, e.timezone, e.city, e.location_mode, e.location_reveal_at,
		       e.genres, COALESCE(v.name, '')
		FROM events e
		LEFT JOIN venues v ON v.id = e.venue_id
		WHERE e.id = $1 AND e.tenant_id = $2`, eventID, tenant).
		Scan(&ev.title, &ev.startsAt, &ev.timezone, &ev.city, &ev.locationMode, &ev.locationRevealAt,
			&ev.genres, &ev.venueName)
	if errors.Is(err, pgx.ErrNoRows) {
		return eventOffer{}, ErrNotFound
	}
	if err != nil {
		return eventOffer{}, err
	}
	return ev, nil
}
