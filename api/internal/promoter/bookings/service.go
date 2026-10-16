package bookings

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/email"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// EmailServiceIface is the outbox surface the service sends through. The
// shared *email.EmailService satisfies it; nil means Plunk is unconfigured
// and sends are refused with ErrEmailUnavailable (503).
type EmailServiceIface interface {
	Send(ctx context.Context, req email.CreateEmailRequest) (*email.EmailMessage, error)
}

// ServiceConfig carries the sender identity and limits.
type ServiceConfig struct {
	FromEmail string // PLUNK_FROM_EMAIL — the instance sender
	FromName  string // PLUNK_FROM_NAME — fallback for {{promoter_name}}
	MaxPerDay int    // OUTREACH_MAX_PER_DAY (default 50)
}

// Service is the bookings business layer: talent contacts, open slots,
// matching, templates, and the send path (validate → suppression check →
// render → race-free daily cap + insert → EmailService.Send → sync status).
// Every method runs in the tenant carried by ctx (set by the auth
// middleware).
type Service struct {
	db     *tenantdb.DB
	keys   *envelope.Keyring
	emails EmailServiceIface
	cfg    ServiceConfig
	now    func() time.Time
}

// NewService builds the service; now defaults to time.Now. emails may be nil
// (Plunk unconfigured — reads work, sends answer ErrEmailUnavailable).
func NewService(db *tenantdb.DB, keys *envelope.Keyring, emails EmailServiceIface, cfg ServiceConfig, now func() time.Time) *Service {
	if cfg.MaxPerDay <= 0 {
		cfg.MaxPerDay = 50
	}
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, keys: keys, emails: emails, cfg: cfg, now: now}
}

func tenantOf(ctx context.Context) uuid.UUID {
	id, _ := tenantdb.TenantFromContext(ctx)
	return id
}

// seal and open are thin adapters over the shared envelope.SealString /
// Keyring.OpenString (the audience pattern) — kept local so the call sites
// don't each bundle table/col into an envelope.Field by hand.
func seal(dek *envelope.DEK, tenant uuid.UUID, table, col string, row uuid.UUID, val string) ([]byte, error) {
	return envelope.SealString(dek, tenant, envelope.Field{Table: table, Column: col, RowID: row}, val)
}

func (s *Service) open(ctx context.Context, tx pgx.Tx, table, col string, row uuid.UUID, sealed []byte) (string, error) {
	return s.keys.OpenString(ctx, tx, tenantOf(ctx), envelope.Field{Table: table, Column: col, RowID: row}, sealed)
}

// Blind-index scopes. The talent contact indexes use the physical column
// names; suppressions deliberately reuse the contact email scope so a
// suppression row matches the contact row directly in SQL (see
// insertSuppression). The request row's to_email_bidx uses its own scope.
func talentEmailIndex(dek *envelope.DEK, addr string) []byte {
	if addr == "" {
		return nil
	}
	return dek.BlindIndex("talent_contacts", "email_bidx", envelope.NormalizeEmail(addr))
}

func talentNameIndex(dek *envelope.DEK, name string) []byte {
	if name == "" {
		return nil
	}
	return dek.BlindIndex("talent_contacts", "name_bidx", envelope.NormalizeName(name))
}

func suppressionIndex(dek *envelope.DEK, addr string) []byte {
	return talentEmailIndex(dek, addr)
}

func offerEmailIndex(dek *envelope.DEK, addr string) []byte {
	if addr == "" {
		return nil
	}
	return dek.BlindIndex("outreach_requests", "to_email_bidx", envelope.NormalizeEmail(addr))
}

// unsealContact decrypts one talent row for display. Empty sealed columns
// decrypt to "" (a name-only contact has no email — HasEmail=false).
func (s *Service) unsealContact(ctx context.Context, tx pgx.Tx, r talentRow) (TalentContact, error) {
	name, err := s.open(ctx, tx, "talent_contacts", "name_enc", r.id, r.nameEnc)
	if err != nil {
		return TalentContact{}, err
	}
	email, err := s.open(ctx, tx, "talent_contacts", "email_enc", r.id, r.emailEnc)
	if err != nil {
		return TalentContact{}, err
	}
	phone, err := s.open(ctx, tx, "talent_contacts", "phone_enc", r.id, r.phoneEnc)
	if err != nil {
		return TalentContact{}, err
	}
	genres := r.genres
	if genres == nil {
		genres = []string{}
	}
	return TalentContact{
		ID: r.id, Name: name, Email: email, Phone: phone, HasEmail: r.emailEnc != nil,
		City: r.city, Country: r.country, Genres: genres, Source: r.source, Notes: r.notes,
		CreatedAt: r.createdAt, UpdatedAt: r.updatedAt,
	}, nil
}

// unsealOffer decrypts one sent-log row for display (SPEC §6.1 v2.1: the
// service unseals when rendering the log).
func (s *Service) unsealOffer(ctx context.Context, tx pgx.Tx, q offerRow) (OfferRequest, error) {
	toEmail, err := s.open(ctx, tx, "outreach_requests", "to_email_enc", q.id, q.toEmailEnc)
	if err != nil {
		return OfferRequest{}, err
	}
	subject, err := s.open(ctx, tx, "outreach_requests", "subject_enc", q.id, q.subjectEnc)
	if err != nil {
		return OfferRequest{}, err
	}
	body, err := s.open(ctx, tx, "outreach_requests", "body_enc", q.id, q.bodyEnc)
	if err != nil {
		return OfferRequest{}, err
	}
	out := OfferRequest{
		ID: q.id, EventID: q.eventID, OpenSlotID: q.openSlotID, TalentContactID: q.talentContactID,
		TemplateID: q.templateID, EmailMessageID: q.emailMessageID,
		ToEmail: toEmail, Subject: subject, Body: body,
		Status: q.status, SentAt: q.sentAt, CreatedAt: q.createdAt,
	}
	if q.emailStatus != nil {
		out.EmailStatus = *q.emailStatus
	}
	if q.emailError != nil {
		out.EmailError = *q.emailError
	}
	return out, nil
}
