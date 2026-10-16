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
	UpdateRequestSent(ctx context.Context, id, emailMessageID uuid.UUID, status string, sentAt *time.Time) error

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
	subject = email.Render(subject, vars)
	body = email.Render(body, vars)

	// Race-free daily cap + insert (SPEC §9 v2.1). The window is local
	// midnight in the sender's timezone — the DJ side has no user-level
	// timezone column, so the process local timezone (TZ env) is used.
	row := &Request{
		ContactID:          in.ContactID,
		AvailabilitySlotID: in.AvailabilitySlotID,
		TemplateID:         &tpl.ID,
		EPKExportID:        in.EPKExportID,
		ToEmail:            contact.Email,
		Subject:            subject,
		Body:               body,
		Status:             string(email.EmailStatusQueued),
	}
	if err := s.repo.InsertRequestWithDailyCap(ctx, row, s.cfg.MaxPerDay, localMidnight(s.now())); err != nil {
		return nil, err
	}

	// EPK attach: register/reuse the documents pointer, then send owned so
	// the ownership guard passes at enqueue and at delivery retry.
	var docID *uuid.UUID
	if in.EPKExportID != nil {
		doc, err := s.epkDocumentPointer(ctx, *in.EPKExportID)
		if err != nil {
			return nil, err
		}
		docID = &doc.ID
	}
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
		return nil, err
	}
	if err := s.repo.UpdateRequestSent(ctx, row.ID, msg.ID, string(msg.Status), msg.SentAt); err != nil {
		return nil, err
	}
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
	return s.repo.GetDefaultTemplate(ctx)
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

// localMidnight returns local midnight of t's day in t's location — the
// start of the daily-cap window.
func localMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
