package outreach

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/docstore"
	"github.com/klubhub/dj/api/internal/platform/email"
)

// Built-in fallback for a missing default template; mirrors the copy seeded
// by migration 034.
const (
	DefaultTemplateName    = "Default booking request"
	DefaultTemplateSubject = "Booking request: {{dj_name}} in {{city}} on {{date}}"
	DefaultTemplateBody    = `Hi {{promoter_name}},

I'm {{dj_name}}, a DJ playing {{genres}}. I'm available in {{city}} on {{date}} and would love to play for you.

Date: {{date}}
City: {{city}}
Genres: {{genres}}
Fee: {{fee}}
{{notes}}

My press kit (EPK) is attached to this email.
{{epk_link}}

If you're interested, just reply to this email — it lands directly in my inbox ({{dj_email}}).

If you'd rather not hear from us, reply "stop" and we'll remove you.

Best,
{{dj_name}}
{{dj_phone}}`
)

// maxEPKPointerBytes caps the in-memory read when computing the documents
// pointer checksum for an EPK attach. EPK PDFs are a few MB; the cap only
// guards against a pathological object.
const maxEPKPointerBytes = 64 << 20

// RepositoryIface is the DB surface the service depends on. Fakes in tests
// substitute it (no sqlmock).
type RepositoryIface interface {
	ListSlots(ctx context.Context) ([]*Slot, error)
	GetSlot(ctx context.Context, id uuid.UUID) (*Slot, error)
	CreateSlot(ctx context.Context, in *SlotCreate) (*Slot, error)
	DeleteSlot(ctx context.Context, id uuid.UUID) error

	ListTemplates(ctx context.Context) ([]*Template, error)
	GetTemplate(ctx context.Context, id uuid.UUID) (*Template, error)
	GetDefaultTemplate(ctx context.Context) (*Template, error)
	CreateTemplate(ctx context.Context, in *TemplateCreate) (*Template, error)
	UpdateTemplate(ctx context.Context, id uuid.UUID, in *TemplateUpdate) (*Template, error)
	DeleteTemplate(ctx context.Context, id uuid.UUID) error

	InsertRequestWithDailyCap(ctx context.Context, row *Request, maxPerDay int, since time.Time) error
	ListRequests(ctx context.Context) ([]*Request, error)
	GetRequest(ctx context.Context, id uuid.UUID) (*Request, error)
	UpdateRequestSent(ctx context.Context, id uuid.UUID, emailMessageID *uuid.UUID, status string, sentAt *time.Time) error

	ListSuppressions(ctx context.Context) ([]*Suppression, error)
	AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error)
	DeleteSuppression(ctx context.Context, id uuid.UUID) error
	IsSuppressed(ctx context.Context, normalizedEmail string) (bool, error)

	GetPromoterContact(ctx context.Context, id uuid.UUID) (*MatchedContact, error)
	MatchContacts(ctx context.Context, city string) ([]*MatchedContact, error)
	MatchVenues(ctx context.Context, city string) ([]*MatchedVenue, error)

	GetEPKExport(ctx context.Context, id uuid.UUID) (*EPKExportRef, error)
	GetContactInfoJSON(ctx context.Context) (string, error)
}

// EmailServiceIface is the outbox surface the service sends through. The
// shared *email.EmailService satisfies it; nil means Plunk is unconfigured
// and sends are refused with ErrEmailUnavailable (503).
type EmailServiceIface interface {
	Send(ctx context.Context, req email.CreateEmailRequest) (*email.EmailMessage, error)
	SendOwned(ctx context.Context, req email.CreateEmailRequest) (*email.EmailMessage, error)
}

// DocumentRepoIface is the documents-pointer surface for the EPK attach.
// *docstore.DocumentRepository satisfies it.
type DocumentRepoIface interface {
	CreateWithDetails(ctx context.Context, req docstore.CreateDocumentRequest, details docstore.CreateDocumentDetails) (*docstore.Document, error)
	GetCurrentByKind(ctx context.Context, ownerType docstore.DocumentOwnerType, ownerID uuid.UUID, kind docstore.DocumentKind) (*docstore.Document, error)
}

// ObjectReaderIface reads object bytes (the EPK PDF) for the pointer
// checksum. docstore.StorageAdapter satisfies it.
type ObjectReaderIface interface {
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

// ServiceConfig carries the sender identity and limits.
type ServiceConfig struct {
	FromEmail    string // PLUNK_FROM_EMAIL
	FromName     string // OUTREACH_FROM_NAME, defaults to PLUNK_FROM_NAME
	MaxPerDay    int    // OUTREACH_MAX_PER_DAY (default 50)
	EPKPublicURL string // EPK_PUBLIC_URL — fills {{epk_link}}
	Bucket       string // Garage bucket holding EPK exports

	// OutreachGate is the fail-closed provider attestation. A nil gate
	// refuses all outreach (ErrOutreachNotPermitted). See
	// platform/email.OutreachGate for why this exists: booking requests are
	// cold outreach, and most hosted providers — Plunk's ToS §5 explicitly —
	// prohibit that regardless of endpoint.
	OutreachGate *email.OutreachGate
}

// AllowsOutreach reports whether the send path may deliver. Fail-closed: a
// nil gate blocks.
func (c ServiceConfig) AllowsOutreach() bool {
	return c.OutreachGate.AllowsOutreach()
}

// Service is the outreach business layer: slots, templates, matching, and
// the send path (validate → render → EPK pointer → Send/SendOwned → log).
type Service struct {
	repo    RepositoryIface
	emails  EmailServiceIface
	docs    DocumentRepoIface
	objects ObjectReaderIface
	cfg     ServiceConfig
	now     func() time.Time
}

// NewService creates a Service. emails may be nil (Plunk unconfigured —
// reads work, sends answer ErrEmailUnavailable).
func NewService(repo RepositoryIface, emails EmailServiceIface, docs DocumentRepoIface, objects ObjectReaderIface, cfg ServiceConfig) *Service {
	if cfg.MaxPerDay <= 0 {
		cfg.MaxPerDay = 50
	}
	return &Service{repo: repo, emails: emails, docs: docs, objects: objects, cfg: cfg, now: time.Now}
}

// SetNow overrides the clock (tests).
func (s *Service) SetNow(now func() time.Time) { s.now = now }

// ─── Slots ───────────────────────────────────────────────────────────────────

// ListSlots returns all availability slots.
func (s *Service) ListSlots(ctx context.Context) ([]*Slot, error) {
	return s.repo.ListSlots(ctx)
}

// CreateSlot validates and stores a new slot.
func (s *Service) CreateSlot(ctx context.Context, in *SlotCreate) (*Slot, error) {
	if err := ValidateSlotCreate(in, s.now()); err != nil {
		return nil, err
	}
	return s.repo.CreateSlot(ctx, in)
}

// DeleteSlot removes a slot.
func (s *Service) DeleteSlot(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSlot(ctx, id)
}

// ─── Templates ───────────────────────────────────────────────────────────────

// ListTemplates returns all non-deleted templates.
func (s *Service) ListTemplates(ctx context.Context) ([]*Template, error) {
	return s.repo.ListTemplates(ctx)
}

// CreateTemplate validates and stores a new template.
func (s *Service) CreateTemplate(ctx context.Context, in *TemplateCreate) (*Template, error) {
	if err := ValidateTemplateCreate(in); err != nil {
		return nil, err
	}
	return s.repo.CreateTemplate(ctx, in)
}

// UpdateTemplate validates and applies a pointer-patch update.
func (s *Service) UpdateTemplate(ctx context.Context, id uuid.UUID, in *TemplateUpdate) (*Template, error) {
	if err := ValidateTemplateUpdate(in); err != nil {
		return nil, err
	}
	return s.repo.UpdateTemplate(ctx, id, in)
}

// DeleteTemplate soft-deletes a template.
func (s *Service) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteTemplate(ctx, id)
}

// ─── Matching ────────────────────────────────────────────────────────────────

// MatchSlots matches promoter contacts (and venues as context) per slot.
// With slotID set, only that slot is matched; otherwise all slots.
func (s *Service) MatchSlots(ctx context.Context, slotID *uuid.UUID) ([]*SlotMatch, error) {
	var slots []*Slot
	if slotID != nil {
		slot, err := s.repo.GetSlot(ctx, *slotID)
		if err != nil {
			return nil, err
		}
		slots = []*Slot{slot}
	} else {
		var err error
		slots, err = s.repo.ListSlots(ctx)
		if err != nil {
			return nil, err
		}
	}
	out := make([]*SlotMatch, 0, len(slots))
	for _, slot := range slots {
		contacts, err := s.repo.MatchContacts(ctx, slot.City)
		if err != nil {
			return nil, err
		}
		venues, err := s.repo.MatchVenues(ctx, slot.City)
		if err != nil {
			return nil, err
		}
		out = append(out, &SlotMatch{Slot: *slot, Contacts: contacts, Venues: venues})
	}
	return out, nil
}

// ─── Send ────────────────────────────────────────────────────────────────────

// SendRequest sends one booking-request email and logs it. The flow:
// validate → load contact (must have an email, not suppressed) → resolve
// the DJ's ReplyTo → resolve template + render → race-free daily cap +
// insert the request row in one transaction → EPK pointer (optional) →
// SendOwned (with EPK) or Send (without) → sync status onto the row.
func (s *Service) SendRequest(ctx context.Context, in *SendRequestInput) (*Request, error) {
	if s.emails == nil {
		return nil, ErrEmailUnavailable
	}
	// Provider attestation gate — fail-closed. Checked before any work so a
	// blocked operator gets an actionable 422/503 rather than a silent send
	// that would breach their provider's terms.
	if !s.cfg.AllowsOutreach() {
		return nil, ErrOutreachNotPermitted
	}
	if err := ValidateSendRequest(in); err != nil {
		return nil, err
	}
	contact, err := s.repo.GetPromoterContact(ctx, in.ContactID)
	if err != nil {
		return nil, err
	}
	if !contact.HasEmail {
		return nil, ErrNoContactEmail
	}
	suppressed, err := s.repo.IsSuppressed(ctx, NormalizeEmail(contact.Email))
	if err != nil {
		return nil, err
	}
	if suppressed {
		return nil, ErrSuppressed
	}
	// ReplyTo = the DJ's own contact email (OUTR-06 v2.1): replies must reach
	// the DJ's mailbox, never the platform From. Refused when unset.
	dj, err := s.djContactInfo(ctx)
	if err != nil {
		return nil, err
	}
	if dj.Email == "" {
		return nil, ErrNoReplyTo
	}
	// Resolve the template and render BEFORE the cap transaction so the
	// inserted row carries the final copy (the template is never mutated).
	tpl, err := s.resolveTemplate(ctx, in.TemplateID)
	if err != nil {
		return nil, err
	}
	subject, body := tpl.Subject, tpl.Body
	if in.Subject != nil {
		subject = *in.Subject
	}
	if in.Body != nil {
		body = *in.Body
	}
	var slot *Slot
	if in.AvailabilitySlotID != nil {
		slot, err = s.repo.GetSlot(ctx, *in.AvailabilitySlotID)
		if err != nil {
			return nil, err
		}
	}
	vars := s.variables(contact, slot, dj)
	subject = email.RenderSubject(subject, vars)
	body = email.Render(body, vars)

	// EPK attach: register/reuse the documents pointer BEFORE the row is
	// inserted, so a bad export (legacy key, unreadable object) is refused
	// without leaving a queued row or spending a daily-cap slot.
	var docID *uuid.UUID
	if in.EPKExportID != nil {
		doc, err := s.epkDocumentPointer(ctx, *in.EPKExportID)
		if err != nil {
			return nil, err
		}
		docID = &doc.ID
	}

	// Race-free daily cap + insert (SPEC §9 v2.1). The window is local
	// midnight in the sender's timezone — the DJ side has no user-level
	// timezone column, so the process local timezone (TZ env) is used.
	row := &Request{
		ContactID:          in.ContactID,
		AvailabilitySlotID: in.AvailabilitySlotID,
		TemplateID:         templateIDOrNil(tpl),
		EPKExportID:        in.EPKExportID,
		ToEmail:            contact.Email,
		Subject:            subject,
		Body:               body,
		Status:             string(email.EmailStatusQueued),
	}
	if err := s.repo.InsertRequestWithDailyCap(ctx, row, s.cfg.MaxPerDay, localMidnight(s.now())); err != nil {
		return nil, err
	}

	// Send owned when an EPK is attached so the ownership guard passes at
	// enqueue and at delivery retry.
	req := email.CreateEmailRequest{
		Kind:      EmailKindBookingRequest,
		FromEmail: s.cfg.FromEmail,
		FromName:  s.cfg.FromName,
		ToEmail:   contact.Email,
		Subject:   subject,
		Body:      body,
		ReplyTo:   dj.Email,
	}
	var msg *email.EmailMessage
	if docID != nil {
		req.OwnerType = string(docstore.DocumentOwnerEPK)
		req.OwnerID = in.EPKExportID
		req.AttachmentIDs = []uuid.UUID{*docID}
		msg, err = s.emails.SendOwned(ctx, req)
	} else {
		req.OwnerType = OwnerTypeOutreachRequest
		req.OwnerID = &row.ID
		msg, err = s.emails.Send(ctx, req)
	}
	if err != nil {
		// Enqueue/validation failure: no outbox row exists. Mark the request
		// failed so the log does not show it queued forever (the cap already
		// counted it — failed attempts count, SPEC §9 v2.1).
		_ = s.repo.UpdateRequestSent(ctx, row.ID, nil, string(email.EmailStatusFailed), nil)
		return nil, err
	}
	// The mail is out: syncing the log row is best-effort. Failing the call
	// here would invite a retry and a duplicate booking request; the outbox
	// row (owner-linked) still carries the delivery status.
	_ = s.repo.UpdateRequestSent(ctx, row.ID, &msg.ID, string(msg.Status), msg.SentAt)
	row.EmailMessageID = &msg.ID
	row.Status = string(msg.Status)
	row.SentAt = msg.SentAt
	row.EmailStatus = string(msg.Status)
	return row, nil
}

// ─── Requests (sent log) ─────────────────────────────────────────────────────

// ListRequests returns the sent log, newest first, with delivery status.
func (s *Service) ListRequests(ctx context.Context) ([]*Request, error) {
	return s.repo.ListRequests(ctx)
}

// GetRequest returns one sent-log row with delivery status.
func (s *Service) GetRequest(ctx context.Context, id uuid.UUID) (*Request, error) {
	return s.repo.GetRequest(ctx, id)
}

// resolveTemplate loads the requested template or the default.
func (s *Service) resolveTemplate(ctx context.Context, id *uuid.UUID) (*Template, error) {
	if id != nil {
		return s.repo.GetTemplate(ctx, *id)
	}
	tpl, err := s.repo.GetDefaultTemplate(ctx)
	if errors.Is(err, ErrNotFound) {
		// The default was deleted or unflagged: fall back to the built-in copy
		// (mirrors the seeded 034 template); template_id stays NULL.
		return &Template{Name: DefaultTemplateName, Subject: DefaultTemplateSubject, Body: DefaultTemplateBody}, nil
	}
	return tpl, err
}

// variables builds the {{var}} map for one send (SPEC §7).
func (s *Service) variables(contact *MatchedContact, slot *Slot, dj ContactInfo) map[string]string {
	vars := map[string]string{
		"dj_name":          dj.Name,
		"dj_email":         dj.Email,
		"dj_phone":         dj.Phone,
		"promoter_name":    contact.Name,
		"promoter_company": contact.Company,
		"epk_link":         s.cfg.EPKPublicURL,
	}
	if slot != nil {
		vars["city"] = slot.City
		vars["country"] = slot.Country
		vars["date"] = time.Time(slot.Date).UTC().Format(DateFormat)
		vars["genres"] = strings.Join(slot.Genres, ", ")
		vars["fee"] = formatFee(slot)
		vars["notes"] = slot.Notes
	}
	return vars
}

// djContactInfo parses user_settings.contact_info ({name, email, phone}).
// A missing row or unparseable JSON yields an empty block — the send is
// then refused with ErrNoReplyTo when the email is empty.
func (s *Service) djContactInfo(ctx context.Context) (ContactInfo, error) {
	raw, err := s.repo.GetContactInfoJSON(ctx)
	if err != nil {
		return ContactInfo{}, err
	}
	var ci ContactInfo
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &ci)
	}
	return ci, nil
}

// epkDocumentPointer registers (or reuses) the documents pointer row for an
// EPK export: owner_type='epk', kind='epk_pdf', storage_key=the export's
// garage_object_key. The row is created from the object's bytes (size +
// sha256 — the export row stores neither, and an S3 ETag is not a sha256).
// Exports write a NEW object key per export (epk service: epk/exports/<uuid>.pdf),
// so a reused pointer's checksum stays valid. A legacy export with an empty
// garage_object_key (pre-025 minio_path rows) is refused.
func (s *Service) epkDocumentPointer(ctx context.Context, exportID uuid.UUID) (*docstore.Document, error) {
	if s.docs == nil || s.objects == nil {
		return nil, ErrEmailUnavailable
	}
	export, err := s.repo.GetEPKExport(ctx, exportID)
	if err != nil {
		return nil, err
	}
	if export.GarageObjectKey == "" {
		return nil, ErrEPKLegacyExport
	}
	// Reuse the existing current pointer when present.
	cur, err := s.docs.GetCurrentByKind(ctx, docstore.DocumentOwnerEPK, exportID, docstore.DocumentKindEPKPDF)
	if err == nil && cur != nil {
		return cur, nil
	}
	if err != nil && !errors.Is(err, docstore.ErrDocumentNotFound) {
		return nil, err
	}
	rc, err := s.objects.GetObject(ctx, s.cfg.Bucket, export.GarageObjectKey)
	if err != nil {
		return nil, fmt.Errorf("read epk object: %w", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxEPKPointerBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read epk object: %w", err)
	}
	if len(data) > maxEPKPointerBytes {
		return nil, fmt.Errorf("epk object exceeds %d bytes", maxEPKPointerBytes)
	}
	sum := sha256.Sum256(data)
	doc, err := s.docs.CreateWithDetails(ctx, docstore.CreateDocumentRequest{
		OwnerType:  docstore.DocumentOwnerEPK,
		OwnerID:    exportID,
		Kind:       docstore.DocumentKindEPKPDF,
		Filename:   path.Base(export.GarageObjectKey),
		MimeType:   "application/pdf",
		UploadedBy: "outreach",
	}, docstore.CreateDocumentDetails{
		StorageKey:     export.GarageObjectKey,
		SizeBytes:      int64(len(data)),
		ChecksumSHA256: hex.EncodeToString(sum[:]),
	})
	if errors.Is(err, docstore.ErrDocumentConflict) {
		// A concurrent send created the pointer first — reuse it.
		return s.docs.GetCurrentByKind(ctx, docstore.DocumentOwnerEPK, exportID, docstore.DocumentKindEPKPDF)
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// ─── Suppressions ────────────────────────────────────────────────────────────

// ListSuppressions returns all suppressions.
func (s *Service) ListSuppressions(ctx context.Context) ([]*Suppression, error) {
	return s.repo.ListSuppressions(ctx)
}

// AddSuppression validates, normalizes and stores a suppression. This is
// the endpoint behind the sent log's one-click "Do not contact".
func (s *Service) AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error) {
	if err := ValidateSuppression(in); err != nil {
		return nil, err
	}
	return s.repo.AddSuppression(ctx, in)
}

// DeleteSuppression removes a suppression by id.
func (s *Service) DeleteSuppression(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSuppression(ctx, id)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// templateIDOrNil is nil for the built-in fallback (no template row).
func templateIDOrNil(t *Template) *uuid.UUID {
	if t.ID == uuid.Nil {
		return nil
	}
	return &t.ID
}

// localMidnight returns local midnight of t's day in t's location — the
// start of the daily-cap window.
func localMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
