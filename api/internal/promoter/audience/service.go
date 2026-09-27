package audience

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// Service implements the audience CRM. Every method runs in the tenant
// carried by ctx (set by the auth middleware).
type Service struct {
	db   *tenantdb.DB
	keys *envelope.Keyring
	now  func() time.Time
}

// NewService builds the service; now defaults to time.Now.
func NewService(db *tenantdb.DB, keys *envelope.Keyring, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, keys: keys, now: now}
}

func tenantOf(ctx context.Context) uuid.UUID {
	id, _ := tenantdb.TenantFromContext(ctx)
	return id
}

// emit enqueues an outbox event. Refs are identifiers only: the bus never
// carries names, contacts or notes.
func (s *Service) emit(ctx context.Context, tx pgx.Tx, verb string, refs map[string]uuid.UUID) error {
	subject, ev, err := events.New(tenantOf(ctx), "audience", verb, refs, s.now())
	if err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, subject, ev)
}

func seal(dek *envelope.DEK, tenant uuid.UUID, table, col string, row uuid.UUID, val string) ([]byte, error) {
	if val == "" {
		return nil, nil
	}
	return dek.Seal(tenant, envelope.Field{Table: table, Column: col, RowID: row}, []byte(val))
}

func (s *Service) open(ctx context.Context, tx pgx.Tx, table, col string, row uuid.UUID, sealed []byte) (string, error) {
	if sealed == nil {
		return "", nil
	}
	tenant := tenantOf(ctx)
	dek, err := s.keys.ForSealed(ctx, tx, tenant, sealed)
	if err != nil {
		return "", err
	}
	plain, err := dek.Open(tenant, envelope.Field{Table: table, Column: col, RowID: row}, sealed)
	return string(plain), err
}

func emailIndex(dek *envelope.DEK, email string) []byte {
	if email == "" {
		return nil
	}
	return dek.BlindIndex("audience_contacts", "email_bidx", envelope.NormalizeEmail(email))
}

func nameIndex(dek *envelope.DEK, name string) []byte {
	if name == "" {
		return nil
	}
	return dek.BlindIndex("audience_contacts", "name_bidx", envelope.NormalizeName(name))
}

// row is the sealed shape read straight off the table, before decryption.
type row struct {
	id                     uuid.UUID
	nameEnc, emailEnc      []byte
	phoneEnc, consentIPEnc []byte
	status, source         string
	consentBasis           string
	consentRecordedAt      time.Time
	consentFormText        string
	doiAt                  *time.Time
	createdAt, updatedAt   time.Time
}

func (s *Service) decrypt(ctx context.Context, tx pgx.Tx, r row) (Contact, error) {
	name, err := s.open(ctx, tx, "audience_contacts", "name_enc", r.id, r.nameEnc)
	if err != nil {
		return Contact{}, err
	}
	email, err := s.open(ctx, tx, "audience_contacts", "email_enc", r.id, r.emailEnc)
	if err != nil {
		return Contact{}, err
	}
	phone, err := s.open(ctx, tx, "audience_contacts", "phone_enc", r.id, r.phoneEnc)
	if err != nil {
		return Contact{}, err
	}
	ip, err := s.open(ctx, tx, "audience_contacts", "consent_ip_enc", r.id, r.consentIPEnc)
	if err != nil {
		return Contact{}, err
	}
	return Contact{
		ID: r.id, Name: name, Email: email, Phone: phone,
		Status: r.status, Source: r.source,
		ConsentBasis: r.consentBasis, ConsentRecordedAt: r.consentRecordedAt,
		ConsentIP: ip, ConsentFormText: r.consentFormText,
		DoubleOptInConfirmedAt: r.doiAt,
		CreatedAt:              r.createdAt, UpdatedAt: r.updatedAt,
	}, nil
}

const contactCols = `id, name_enc, email_enc, phone_enc, consent_ip_enc, status, source,
	consent_basis, consent_recorded_at, consent_form_text, double_opt_in_confirmed_at,
	created_at, updated_at`

func scanRow(rows pgx.Rows) (row, error) {
	var r row
	err := rows.Scan(&r.id, &r.nameEnc, &r.emailEnc, &r.phoneEnc, &r.consentIPEnc, &r.status, &r.source,
		&r.consentBasis, &r.consentRecordedAt, &r.consentFormText, &r.doiAt, &r.createdAt, &r.updatedAt)
	return r, err
}

// CreateContact adds one contact with its consent record. ErrDuplicate is
// returned when an active contact with the same email already exists in
// the tenant (the unique blind-index constraint); callers may prefer
// ImportCSV for bulk uploads, which folds a duplicate into a skip rather
// than an error.
func (s *Service) CreateContact(ctx context.Context, in ContactInput) (*Contact, error) {
	if err := in.validate(true); err != nil {
		return nil, err
	}
	var out Contact
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		id := uuid.New()
		nameEnc, err := seal(dek, tenant, "audience_contacts", "name_enc", id, in.Name)
		if err != nil {
			return err
		}
		emailEnc, err := seal(dek, tenant, "audience_contacts", "email_enc", id, in.Email)
		if err != nil {
			return err
		}
		phoneEnc, err := seal(dek, tenant, "audience_contacts", "phone_enc", id, in.Phone)
		if err != nil {
			return err
		}
		ipEnc, err := seal(dek, tenant, "audience_contacts", "consent_ip_enc", id, in.Consent.IP)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO audience_contacts
			(id, tenant_id, name_enc, email_enc, phone_enc, name_bidx, email_bidx, status, source,
			 consent_basis, consent_recorded_at, consent_ip_enc, consent_form_text, double_opt_in_confirmed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			id, tenant, nameEnc, emailEnc, phoneEnc, nameIndex(dek, in.Name), emailIndex(dek, in.Email),
			in.Status, in.Source, in.Consent.Basis, in.Consent.RecordedAt, ipEnc, in.Consent.FormText, in.Consent.DoubleOptInAt)
		if err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicate
			}
			return err
		}
		if err := s.emit(ctx, tx, "created", map[string]uuid.UUID{"contact_id": id}); err != nil {
			return err
		}
		out = Contact{
			ID: id, Name: in.Name, Email: in.Email, Phone: in.Phone,
			Status: in.Status, Source: in.Source,
			ConsentBasis: in.Consent.Basis, ConsentRecordedAt: in.Consent.RecordedAt,
			ConsentIP: in.Consent.IP, ConsentFormText: in.Consent.FormText,
			DoubleOptInConfirmedAt: in.Consent.DoubleOptInAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListContacts returns the contact table matching filter, plus tab counts
// across the whole tenant (not narrowed by filter, matching the guest
// table's convention of counts that don't move as you filter the list).
func (s *Service) ListContacts(ctx context.Context, filter ContactFilter) (*Page, error) {
	var page Page
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		countRows, err := tx.Query(ctx, `SELECT status, count(*) FROM audience_contacts WHERE tenant_id = $1 GROUP BY status`, tenant)
		if err != nil {
			return err
		}
		for countRows.Next() {
			var status string
			var n int
			if err := countRows.Scan(&status, &n); err != nil {
				countRows.Close()
				return err
			}
			page.Counts.All += n
			switch status {
			case StatusActive:
				page.Counts.Active = n
			case StatusUnsubscribed:
				page.Counts.Unsubscribed = n
			case StatusBounced:
				page.Counts.Bounced = n
			case StatusComplained:
				page.Counts.Complained = n
			}
		}
		countRows.Close()
		if err := countRows.Err(); err != nil {
			return err
		}

		query := `SELECT ` + contactCols + ` FROM audience_contacts WHERE tenant_id = $1`
		args := []any{tenant}
		if filter.Status != "" {
			args = append(args, filter.Status)
			query += ` AND status = $` + strconv.Itoa(len(args))
		}
		if filter.Source != "" {
			args = append(args, filter.Source)
			query += ` AND source = $` + strconv.Itoa(len(args))
		}
		if filter.Q != "" {
			dek, err := s.keys.Current(ctx, tx, tenant)
			if err != nil {
				return err
			}
			args = append(args, emailIndex(dek, filter.Q), nameIndex(dek, filter.Q))
			query += ` AND (email_bidx = $` + strconv.Itoa(len(args)-1) + ` OR name_bidx = $` + strconv.Itoa(len(args)) + `)`
		}
		query += ` ORDER BY created_at DESC LIMIT 5000`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRow(rows)
			if err != nil {
				return err
			}
			c, err := s.decrypt(ctx, tx, r)
			if err != nil {
				return err
			}
			page.Contacts = append(page.Contacts, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &page, nil
}

// UpdateContact replaces a contact's identity fields. It does not touch the
// consent record — consent is captured once, at creation; correcting it is
// a support/manual-DB action, not a routine edit, matching the plan's "no
// silent modification" discipline for records with compliance weight.
func (s *Service) UpdateContact(ctx context.Context, id uuid.UUID, in ContactInput) (*Contact, error) {
	if err := in.validate(false); err != nil {
		return nil, err
	}
	var out Contact
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		nameEnc, err := seal(dek, tenant, "audience_contacts", "name_enc", id, in.Name)
		if err != nil {
			return err
		}
		emailEnc, err := seal(dek, tenant, "audience_contacts", "email_enc", id, in.Email)
		if err != nil {
			return err
		}
		phoneEnc, err := seal(dek, tenant, "audience_contacts", "phone_enc", id, in.Phone)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE audience_contacts SET
			name_enc = $3, email_enc = $4, phone_enc = $5, name_bidx = $6, email_bidx = $7,
			source = $8, updated_at = now()
			WHERE id = $1 AND tenant_id = $2`,
			id, tenant, nameEnc, emailEnc, phoneEnc, nameIndex(dek, in.Name), emailIndex(dek, in.Email), in.Source)
		if err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicate
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		out = Contact{ID: id, Name: in.Name, Email: in.Email, Phone: in.Phone, Source: in.Source}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetStatus changes a contact's subscription status (unsubscribe, or a
// bounce/complaint recorded from the email provider).
func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, in StatusInput) error {
	if err := in.validate(); err != nil {
		return err
	}
	return s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE audience_contacts SET status = $3, updated_at = now()
			WHERE id = $1 AND tenant_id = $2`, id, tenantOf(ctx), in.Status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return s.emit(ctx, tx, "status_changed", map[string]uuid.UUID{"contact_id": id})
	})
}

// DeleteContact removes a contact outright (a data-subject deletion
// request), auditing the action since it destroys a compliance record.
func (s *Service) DeleteContact(ctx context.Context, id uuid.UUID, actor string) error {
	return s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		tag, err := tx.Exec(ctx, `DELETE FROM audience_contacts WHERE id = $1 AND tenant_id = $2`, id, tenant)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := audit.Record(ctx, tx, tenant, audit.Entry{
			ActorID: actor, Action: "audience.contact_deleted", Resource: id.String(),
		}); err != nil {
			return err
		}
		return s.emit(ctx, tx, "deleted", map[string]uuid.UUID{"contact_id": id})
	})
}

// ImportCSV adds contacts in bulk from a CSV upload. Every row shares one
// consent record: whoever uploads the list attests the lawful basis for
// the whole batch (typically soft_opt_in for past customers), a decision
// recorded once rather than fabricated per row from data the CSV does not
// carry. A row whose email already exists is counted as a duplicate and
// skipped, not overwritten — an import must not silently change an
// existing contact's identity fields.
func (s *Service) ImportCSV(ctx context.Context, rows []ImportRow, consent ConsentInput) (*ImportResult, error) {
	if err := consent.validate(); err != nil {
		return nil, err
	}
	if len(rows) > MaxBatch {
		return nil, invalid("rows", "at most "+strconv.Itoa(MaxBatch)+" per import")
	}
	var result ImportResult
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		for _, r := range rows {
			in := ContactInput{Name: r.Name, Email: r.Email, Phone: r.Phone, Source: SourceCSV, Status: StatusActive, Consent: consent}
			if err := in.validate(true); err != nil {
				result.Invalid++
				continue
			}
			if in.Email != "" {
				var exists bool
				err := tx.QueryRow(ctx, `SELECT true FROM audience_contacts WHERE tenant_id = $1 AND email_bidx = $2`,
					tenant, emailIndex(dek, in.Email)).Scan(&exists)
				if err == nil {
					result.Duplicates++
					continue
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			id := uuid.New()
			nameEnc, err := seal(dek, tenant, "audience_contacts", "name_enc", id, in.Name)
			if err != nil {
				return err
			}
			emailEnc, err := seal(dek, tenant, "audience_contacts", "email_enc", id, in.Email)
			if err != nil {
				return err
			}
			phoneEnc, err := seal(dek, tenant, "audience_contacts", "phone_enc", id, in.Phone)
			if err != nil {
				return err
			}
			ipEnc, err := seal(dek, tenant, "audience_contacts", "consent_ip_enc", id, consent.IP)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO audience_contacts
				(id, tenant_id, name_enc, email_enc, phone_enc, name_bidx, email_bidx, status, source,
				 consent_basis, consent_recorded_at, consent_ip_enc, consent_form_text, double_opt_in_confirmed_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
				id, tenant, nameEnc, emailEnc, phoneEnc, nameIndex(dek, in.Name), emailIndex(dek, in.Email),
				in.Status, in.Source, consent.Basis, consent.RecordedAt, ipEnc, consent.FormText, consent.DoubleOptInAt); err != nil {
				if isUniqueViolation(err) {
					result.Duplicates++
					continue
				}
				return err
			}
			result.Added++
		}
		if result.Added == 0 {
			return nil
		}
		return audit.Record(ctx, tx, tenant, audit.Entry{
			Action: "audience.imported", Resource: strconv.Itoa(result.Added) + " contacts",
		})
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateSegment saves a filter.
func (s *Service) CreateSegment(ctx context.Context, in SegmentInput) (*Segment, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	var out Segment
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		id := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO audience_segments (id, tenant_id, name, filter) VALUES ($1,$2,$3,$4)`,
			id, tenantOf(ctx), in.Name, filterJSON(in.Filter)); err != nil {
			if isUniqueViolation(err) {
				return ErrSegmentExists
			}
			return err
		}
		n, err := s.countMatching(ctx, tx, in.Filter)
		if err != nil {
			return err
		}
		out = Segment{ID: id, Name: in.Name, Filter: in.Filter, Matching: n}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListSegments returns every saved segment with its live matching count.
func (s *Service) ListSegments(ctx context.Context) ([]Segment, error) {
	var out []Segment
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, name, filter, created_at, updated_at FROM audience_segments
			WHERE tenant_id = $1 ORDER BY name`, tenantOf(ctx))
		if err != nil {
			return err
		}
		defer rows.Close()
		var segs []Segment
		for rows.Next() {
			var seg Segment
			var raw []byte
			if err := rows.Scan(&seg.ID, &seg.Name, &raw, &seg.CreatedAt, &seg.UpdatedAt); err != nil {
				return err
			}
			if err := parseFilterJSON(raw, &seg.Filter); err != nil {
				return err
			}
			segs = append(segs, seg)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range segs {
			n, err := s.countMatching(ctx, tx, segs[i].Filter)
			if err != nil {
				return err
			}
			segs[i].Matching = n
		}
		out = segs
		return nil
	})
	return out, err
}

// UpdateSegment replaces a segment's name and filter.
func (s *Service) UpdateSegment(ctx context.Context, id uuid.UUID, in SegmentInput) (*Segment, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	var out Segment
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE audience_segments SET name = $3, filter = $4, updated_at = now()
			WHERE id = $1 AND tenant_id = $2`, id, tenantOf(ctx), in.Name, filterJSON(in.Filter))
		if err != nil {
			if isUniqueViolation(err) {
				return ErrSegmentExists
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		n, err := s.countMatching(ctx, tx, in.Filter)
		if err != nil {
			return err
		}
		out = Segment{ID: id, Name: in.Name, Filter: in.Filter, Matching: n}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSegment removes a saved filter. It never deletes contacts.
func (s *Service) DeleteSegment(ctx context.Context, id uuid.UUID) error {
	return s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM audience_segments WHERE id = $1 AND tenant_id = $2`, id, tenantOf(ctx))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Service) countMatching(ctx context.Context, tx pgx.Tx, f SegmentFilter) (int, error) {
	query := `SELECT count(*) FROM audience_contacts WHERE tenant_id = $1`
	args := []any{tenantOf(ctx)}
	if f.Status != "" {
		args = append(args, f.Status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if f.Source != "" {
		args = append(args, f.Source)
		query += ` AND source = $` + strconv.Itoa(len(args))
	}
	if f.SinceDays != nil {
		// Computed in Go, not a Postgres interval cast: pgx has no default
		// encoding for time.Duration into `interval`, and a hand-rolled
		// cast is an easy place to get silently wrong.
		args = append(args, s.now().AddDate(0, 0, -*f.SinceDays))
		query += ` AND created_at >= $` + strconv.Itoa(len(args))
	}
	var n int
	err := tx.QueryRow(ctx, query, args...).Scan(&n)
	return n, err
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}
