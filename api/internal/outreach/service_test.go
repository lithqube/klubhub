package outreach

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/docstore"
	"github.com/klubhub/dj/api/internal/platform/email"
)

// ─── Fakes ───────────────────────────────────────────────────────────────────

type fakeRepo struct {
	slots        []*Slot
	templates    []*Template
	requests     []*Request
	suppressions []*Suppression
	contacts     map[uuid.UUID]*MatchedContact
	venues       []*MatchedVenue
	epkExports   map[uuid.UUID]*EPKExportRef
	contactInfo  string

	insertCalls   int
	insertMax     int
	insertSince   time.Time
	insertErr     error
	updatedSent   []uuid.UUID
	suppressedSet map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		contacts:      map[uuid.UUID]*MatchedContact{},
		epkExports:    map[uuid.UUID]*EPKExportRef{},
		suppressedSet: map[string]bool{},
	}
}

func (f *fakeRepo) ListSlots(ctx context.Context) ([]*Slot, error) { return f.slots, nil }

func (f *fakeRepo) GetSlot(ctx context.Context, id uuid.UUID) (*Slot, error) {
	for _, s := range f.slots {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) CreateSlot(ctx context.Context, in *SlotCreate) (*Slot, error) {
	s := &Slot{ID: uuid.New(), Date: in.Date, City: in.City, Country: in.Country, Genres: in.Genres, FeeMin: in.FeeMin, FeeMax: in.FeeMax, Currency: in.Currency, Notes: in.Notes, CreatedAt: time.Now()}
	f.slots = append(f.slots, s)
	return s, nil
}

func (f *fakeRepo) DeleteSlot(ctx context.Context, id uuid.UUID) error {
	for i, s := range f.slots {
		if s.ID == id {
			f.slots = append(f.slots[:i], f.slots[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeRepo) ListTemplates(ctx context.Context) ([]*Template, error) { return f.templates, nil }

func (f *fakeRepo) GetTemplate(ctx context.Context, id uuid.UUID) (*Template, error) {
	for _, t := range f.templates {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) GetDefaultTemplate(ctx context.Context) (*Template, error) {
	for _, t := range f.templates {
		if t.IsDefault {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) CreateTemplate(ctx context.Context, in *TemplateCreate) (*Template, error) {
	t := &Template{ID: uuid.New(), Name: in.Name, Direction: "dj_to_promoter", Subject: in.Subject, Body: in.Body, IsDefault: in.IsDefault, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.templates = append(f.templates, t)
	return t, nil
}

func (f *fakeRepo) UpdateTemplate(ctx context.Context, id uuid.UUID, in *TemplateUpdate) (*Template, error) {
	t, err := f.GetTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Version != t.Version {
		return nil, ErrConflict
	}
	if in.Name != nil {
		t.Name = *in.Name
	}
	if in.Subject != nil {
		t.Subject = *in.Subject
	}
	if in.Body != nil {
		t.Body = *in.Body
	}
	if in.IsDefault != nil {
		t.IsDefault = *in.IsDefault
	}
	t.Version++
	t.UpdatedAt = time.Now()
	return t, nil
}

func (f *fakeRepo) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	for i, t := range f.templates {
		if t.ID == id {
			f.templates = append(f.templates[:i], f.templates[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeRepo) InsertRequestWithDailyCap(ctx context.Context, row *Request, maxPerDay int, since time.Time) error {
	f.insertCalls++
	f.insertMax = maxPerDay
	f.insertSince = since
	if f.insertErr != nil {
		return f.insertErr
	}
	row.ID = uuid.New()
	row.CreatedAt = time.Now()
	f.requests = append(f.requests, row)
	return nil
}

func (f *fakeRepo) ListRequests(ctx context.Context) ([]*Request, error) { return f.requests, nil }

func (f *fakeRepo) GetRequest(ctx context.Context, id uuid.UUID) (*Request, error) {
	for _, r := range f.requests {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepo) UpdateRequestSent(ctx context.Context, id, emailMessageID uuid.UUID, status string, sentAt *time.Time) error {
	f.updatedSent = append(f.updatedSent, id)
	for _, r := range f.requests {
		if r.ID == id {
			r.EmailMessageID = &emailMessageID
			r.Status = status
			r.SentAt = sentAt
			r.EmailStatus = status
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeRepo) ListSuppressions(ctx context.Context) ([]*Suppression, error) {
	return f.suppressions, nil
}

func (f *fakeRepo) AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error) {
	s := &Suppression{ID: uuid.New(), Email: in.Email, Reason: in.Reason, CreatedAt: time.Now()}
	f.suppressions = append(f.suppressions, s)
	f.suppressedSet[in.Email] = true
	return s, nil
}

func (f *fakeRepo) DeleteSuppression(ctx context.Context, id uuid.UUID) error {
	for i, s := range f.suppressions {
		if s.ID == id {
			delete(f.suppressedSet, s.Email)
			f.suppressions = append(f.suppressions[:i], f.suppressions[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeRepo) IsSuppressed(ctx context.Context, normalizedEmail string) (bool, error) {
	return f.suppressedSet[normalizedEmail], nil
}

func (f *fakeRepo) GetPromoterContact(ctx context.Context, id uuid.UUID) (*MatchedContact, error) {
	c, ok := f.contacts[id]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (f *fakeRepo) MatchContacts(ctx context.Context, city string) ([]*MatchedContact, error) {
	var out []*MatchedContact
	for _, c := range f.contacts {
		if strings.EqualFold(strings.TrimSpace(c.City), strings.TrimSpace(city)) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeRepo) MatchVenues(ctx context.Context, city string) ([]*MatchedVenue, error) {
	return f.venues, nil
}

func (f *fakeRepo) GetEPKExport(ctx context.Context, id uuid.UUID) (*EPKExportRef, error) {
	e, ok := f.epkExports[id]
	if !ok {
		return nil, ErrNotFound
	}
	return e, nil
}

func (f *fakeRepo) GetContactInfoJSON(ctx context.Context) (string, error) {
	return f.contactInfo, nil
}

type fakeEmails struct {
	sent     []email.CreateEmailRequest
	owned    []email.CreateEmailRequest
	sentMsg  *email.EmailMessage
	ownedMsg *email.EmailMessage
	err      error
}

func (f *fakeEmails) Send(ctx context.Context, req email.CreateEmailRequest) (*email.EmailMessage, error) {
	f.sent = append(f.sent, req)
	if f.err != nil {
		return nil, f.err
	}
	if f.sentMsg != nil {
		return f.sentMsg, nil
	}
	sentAt := time.Now()
	return &email.EmailMessage{ID: uuid.New(), Status: email.EmailStatusSent, SentAt: &sentAt}, nil
}

func (f *fakeEmails) SendOwned(ctx context.Context, req email.CreateEmailRequest) (*email.EmailMessage, error) {
	f.owned = append(f.owned, req)
	if f.err != nil {
		return nil, f.err
	}
	if f.ownedMsg != nil {
		return f.ownedMsg, nil
	}
	sentAt := time.Now()
	return &email.EmailMessage{ID: uuid.New(), Status: email.EmailStatusSent, SentAt: &sentAt}, nil
}

type fakeDocs struct {
	created   []docstore.CreateDocumentRequest
	details   []docstore.CreateDocumentDetails
	existing  *docstore.Document
	createErr error
}

func (f *fakeDocs) CreateWithDetails(ctx context.Context, req docstore.CreateDocumentRequest, details docstore.CreateDocumentDetails) (*docstore.Document, error) {
	f.created = append(f.created, req)
	f.details = append(f.details, details)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &docstore.Document{ID: uuid.New(), OwnerType: req.OwnerType, OwnerID: req.OwnerID, Kind: req.Kind, StorageKey: details.StorageKey, Filename: req.Filename, MimeType: req.MimeType, SizeBytes: details.SizeBytes, ChecksumSHA256: details.ChecksumSHA256, Version: 1, IsCurrent: true}, nil
}

func (f *fakeDocs) GetCurrentByKind(ctx context.Context, ownerType docstore.DocumentOwnerType, ownerID uuid.UUID, kind docstore.DocumentKind) (*docstore.Document, error) {
	if f.existing != nil {
		return f.existing, nil
	}
	return nil, docstore.ErrDocumentNotFound
}

type fakeObjects struct {
	data map[string][]byte
	err  error
}

func (f *fakeObjects) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.data[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return io.NopCloser(strings.NewReader(string(data))), nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newTestService(repo *fakeRepo, emails EmailServiceIface, docs DocumentRepoIface, objects ObjectReaderIface) *Service {
	return NewService(repo, emails, docs, objects, ServiceConfig{
		FromEmail:    "dj@klubhub.example",
		FromName:     "Test DJ",
		MaxPerDay:    50,
		EPKPublicURL: "https://epk.klubhub.example/abc",
		Bucket:       "klubhub",
	})
}

func promoterWithEmail(id uuid.UUID, email string) *MatchedContact {
	return &MatchedContact{ID: id, Name: "Promoter Pat", Company: "Club X", Email: email, HasEmail: email != "", City: "Berlin"}
}

func defaultTemplate() *Template {
	return &Template{
		ID:        uuid.New(),
		Name:      "Default",
		Direction: "dj_to_promoter",
		Subject:   "Booking request: {{dj_name}} in {{city}} on {{date}}",
		Body:      "Hi {{promoter_name}},\r\n\n{{dj_name}} can play {{city}} on {{date}}. Fee: {{fee}}.\r\n\r\n{{dj_email}}",
		IsDefault: true,
		Version:   1,
	}
}

func djInfoJSON() string {
	return `{"name":"Test DJ","email":"dj@klubhub.example","phone":"+49 123"}`
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestSendRequestHappyPath(t *testing.T) {
	repo := newFakeRepo()
	contactID := uuid.New()
	repo.contacts[contactID] = promoterWithEmail(contactID, "pat@club.example")
	repo.templates = []*Template{defaultTemplate()}
	repo.contactInfo = djInfoJSON()
	slot := &Slot{ID: uuid.New(), Date: Date(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)), City: "Berlin", Country: "DE", Genres: []string{"techno"}, FeeMin: ptrFloat(150), FeeMax: ptrFloat(150), Currency: ptrString("EUR")}
	repo.slots = []*Slot{slot}
	emails := &fakeEmails{}

	svc := newTestService(repo, emails, &fakeDocs{}, &fakeObjects{})
	row, err := svc.SendRequest(context.Background(), &SendRequestInput{
		ContactID:          contactID,
		AvailabilitySlotID: &slot.ID,
	})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}

	// One message, plain Send (no EPK), with the DJ's ReplyTo.
	if len(emails.sent) != 1 || len(emails.owned) != 0 {
		t.Fatalf("sent=%d owned=%d, want 1 plain send", len(emails.sent), len(emails.owned))
	}
	msg := emails.sent[0]
	if msg.ToEmail != "pat@club.example" {
		t.Fatalf("ToEmail = %q", msg.ToEmail)
	}
	if msg.ReplyTo != "dj@klubhub.example" {
		t.Fatalf("ReplyTo = %q, want the DJ's contact email", msg.ReplyTo)
	}
	if msg.FromEmail != "dj@klubhub.example" || msg.FromName != "Test DJ" {
		t.Fatalf("From = %q/%q", msg.FromEmail, msg.FromName)
	}
	if msg.Kind != EmailKindBookingRequest {
		t.Fatalf("Kind = %q", msg.Kind)
	}
	if msg.OwnerType != OwnerTypeOutreachRequest || msg.OwnerID == nil || *msg.OwnerID != row.ID {
		t.Fatalf("Owner = %q/%v", msg.OwnerType, msg.OwnerID)
	}
	// Rendered subject: variables substituted, template CRLF preserved.
	if msg.Subject != "Booking request: Test DJ in Berlin on 2026-11-01" {
		t.Fatalf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "Hi Promoter Pat,") {
		t.Fatalf("Body missing promoter name: %q", msg.Body)
	}
	if !strings.Contains(msg.Body, "Fee: 150 EUR.") {
		t.Fatalf("Body missing fee: %q", msg.Body)
	}
	if !strings.Contains(msg.Body, "dj@klubhub.example") {
		t.Fatalf("Body missing dj email: %q", msg.Body)
	}

	// The row carries the rendered copy and the synced delivery status.
	if row.Subject != msg.Subject || row.Body != msg.Body {
		t.Fatal("row must carry the rendered copy")
	}
	if row.Status != string(email.EmailStatusSent) || row.EmailStatus != string(email.EmailStatusSent) {
		t.Fatalf("row status = %q/%q", row.Status, row.EmailStatus)
	}
	if row.EmailMessageID == nil || row.SentAt == nil {
		t.Fatal("row must link the email message and sent_at")
	}
	if row.TemplateID == nil {
		t.Fatal("row must link the template")
	}

	// Cap + insert went through exactly once, with the configured cap and
	// the local-midnight window.
	if repo.insertCalls != 1 {
		t.Fatalf("insertCalls = %d", repo.insertCalls)
	}
	if repo.insertMax != 50 {
		t.Fatalf("insertMax = %d", repo.insertMax)
	}
	wantSince := localMidnight(time.Now())
	if !repo.insertSince.Equal(wantSince) {
		t.Fatalf("insertSince = %v, want %v", repo.insertSince, wantSince)
	}
	if len(repo.updatedSent) != 1 || repo.updatedSent[0] != row.ID {
		t.Fatalf("updatedSent = %v", repo.updatedSent)
	}
}

func TestSendRequestRenderStripsCRLFInjection(t *testing.T) {
	repo := newFakeRepo()
	contactID := uuid.New()
	// A malicious contact name carrying a header injection attempt.
	repo.contacts[contactID] = &MatchedContact{ID: contactID, Name: "Pat\r\nBcc: evil@example.com", Email: "pat@club.example", HasEmail: true, City: "Berlin"}
	tpl := defaultTemplate()
	tpl.Subject = "Hi {{promoter_name}}"
	tpl.Body = "Hello {{promoter_name}}"
	repo.templates = []*Template{tpl}
	repo.contactInfo = djInfoJSON()
	emails := &fakeEmails{}

	svc := newTestService(repo, emails, &fakeDocs{}, &fakeObjects{})
	if _, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID}); err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	msg := emails.sent[0]
	if strings.ContainsAny(msg.Subject, "\r\n") {
		t.Fatalf("subject carries CR/LF: %q", msg.Subject)
	}
	// The injection text survives as data on one line — it cannot become a
	// header because the CR/LF that would start one are gone.
	if !strings.Contains(msg.Subject, "Pat  Bcc: evil@example.com") {
		t.Fatalf("CRLF should be flattened to spaces: %q", msg.Subject)
	}
}

func TestSendRequestOverridesTemplateCopy(t *testing.T) {
	repo := newFakeRepo()
	contactID := uuid.New()
	repo.contacts[contactID] = promoterWithEmail(contactID, "pat@club.example")
	repo.templates = []*Template{defaultTemplate()}
	repo.contactInfo = djInfoJSON()
	emails := &fakeEmails{}

	svc := newTestService(repo, emails, &fakeDocs{}, &fakeObjects{})
	subject := "Custom subject {{dj_name}}"
	body := "Custom body"
	row, err := svc.SendRequest(context.Background(), &SendRequestInput{
		ContactID: contactID,
		Subject:   &subject,
		Body:      &body,
	})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if row.Subject != "Custom subject Test DJ" {
		t.Fatalf("Subject = %q", row.Subject)
	}
	if row.Body != "Custom body" {
		t.Fatalf("Body = %q", row.Body)
	}
	// The template itself is never mutated.
	if repo.templates[0].Subject == row.Subject {
		t.Fatal("template was mutated by the override")
	}
}

func TestSendRequestWithEPKAttach(t *testing.T) {
	repo := newFakeRepo()
	contactID := uuid.New()
	repo.contacts[contactID] = promoterWithEmail(contactID, "pat@club.example")
	repo.templates = []*Template{defaultTemplate()}
	repo.contactInfo = djInfoJSON()
	exportID := uuid.New()
	pdfBytes := []byte("%PDF-1.4 fake epk bytes")
	repo.epkExports[exportID] = &EPKExportRef{ID: exportID, GarageObjectKey: "epk/exports/" + exportID.String() + ".pdf"}
	emails := &fakeEmails{}
	docs := &fakeDocs{}
	objects := &fakeObjects{data: map[string][]byte{repo.epkExports[exportID].GarageObjectKey: pdfBytes}}

	svc := newTestService(repo, emails, docs, objects)
	row, err := svc.SendRequest(context.Background(), &SendRequestInput{
		ContactID:   contactID,
		EPKExportID: &exportID,
	})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}

	// SendOwned with the EPK ownership + the pointer document as attachment.
	if len(emails.owned) != 1 || len(emails.sent) != 0 {
		t.Fatalf("sent=%d owned=%d, want 1 owned send", len(emails.sent), len(emails.owned))
	}
	msg := emails.owned[0]
	if msg.OwnerType != string(docstore.DocumentOwnerEPK) || msg.OwnerID == nil || *msg.OwnerID != exportID {
		t.Fatalf("Owner = %q/%v", msg.OwnerType, msg.OwnerID)
	}
	if len(msg.AttachmentIDs) != 1 {
		t.Fatalf("AttachmentIDs = %v", msg.AttachmentIDs)
	}

	// The documents pointer row: owner_type='epk', kind='epk_pdf',
	// storage_key = the export's garage object key, sha256 of the bytes.
	if len(docs.created) != 1 {
		t.Fatalf("docs.created = %d", len(docs.created))
	}
	creq := docs.created[0]
	if creq.OwnerType != docstore.DocumentOwnerEPK || creq.OwnerID != exportID {
		t.Fatalf("pointer owner = %q/%v", creq.OwnerType, creq.OwnerID)
	}
	if creq.Kind != docstore.DocumentKindEPKPDF {
		t.Fatalf("pointer kind = %q", creq.Kind)
	}
	if creq.MimeType != "application/pdf" {
		t.Fatalf("pointer mime = %q", creq.MimeType)
	}
	cdet := docs.details[0]
	if cdet.StorageKey != repo.epkExports[exportID].GarageObjectKey {
		t.Fatalf("storage_key = %q", cdet.StorageKey)
	}
	sum := sha256.Sum256(pdfBytes)
	if cdet.ChecksumSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("checksum = %q", cdet.ChecksumSHA256)
	}
	if cdet.SizeBytes != int64(len(pdfBytes)) {
		t.Fatalf("size = %d", cdet.SizeBytes)
	}
	if row.EPKExportID == nil || *row.EPKExportID != exportID {
		t.Fatal("row must link the EPK export")
	}
}

func TestSendRequestEPKPointerReuse(t *testing.T) {
	repo := newFakeRepo()
	contactID := uuid.New()
	repo.contacts[contactID] = promoterWithEmail(contactID, "pat@club.example")
	repo.templates = []*Template{defaultTemplate()}
	repo.contactInfo = djInfoJSON()
	exportID := uuid.New()
	key := "epk/exports/" + exportID.String() + ".pdf"
	repo.epkExports[exportID] = &EPKExportRef{ID: exportID, GarageObjectKey: key}
	emails := &fakeEmails{}
	existing := &docstore.Document{ID: uuid.New(), OwnerType: docstore.DocumentOwnerEPK, OwnerID: exportID, Kind: docstore.DocumentKindEPKPDF, StorageKey: key, IsCurrent: true}
	docs := &fakeDocs{existing: existing}
	objects := &fakeObjects{data: map[string][]byte{key: []byte("pdf")}}

	svc := newTestService(repo, emails, docs, objects)
	if _, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID, EPKExportID: &exportID}); err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if len(docs.created) != 0 {
		t.Fatalf("no new pointer expected, created=%d", len(docs.created))
	}
	if len(emails.owned) != 1 || emails.owned[0].AttachmentIDs[0] != existing.ID {
		t.Fatalf("attachment should reuse the existing pointer: %+v", emails.owned)
	}
}

func TestSendRequestEPKConflictReuse(t *testing.T) {
	repo := newFakeRepo()
	contactID := uuid.New()
	repo.contacts[contactID] = promoterWithEmail(contactID, "pat@club.example")
	repo.templates = []*Template{defaultTemplate()}
	repo.contactInfo = djInfoJSON()
	exportID := uuid.New()
	key := "epk/exports/" + exportID.String() + ".pdf"
	repo.epkExports[exportID] = &EPKExportRef{ID: exportID, GarageObjectKey: key}
	emails := &fakeEmails{}
	existing := &docstore.Document{ID: uuid.New(), OwnerType: docstore.DocumentOwnerEPK, OwnerID: exportID, Kind: docstore.DocumentKindEPKPDF, StorageKey: key, IsCurrent: true}
	docs := &fakeDocs{createErr: docstore.ErrDocumentConflict, existing: existing}
	objects := &fakeObjects{data: map[string][]byte{key: []byte("pdf")}}

	svc := newTestService(repo, emails, docs, objects)
	if _, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID, EPKExportID: &exportID}); err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if len(emails.owned) != 1 || emails.owned[0].AttachmentIDs[0] != existing.ID {
		t.Fatalf("conflict should fall back to the existing pointer: %+v", emails.owned)
	}
}

func TestSendRequestRefusals(t *testing.T) {
	contactID := uuid.New()
	withEmail := promoterWithEmail(contactID, "pat@club.example")

	newSvc := func(mutate func(repo *fakeRepo, emails *fakeEmails)) (*Service, *fakeRepo, *fakeEmails) {
		repo := newFakeRepo()
		repo.contacts[contactID] = withEmail
		repo.templates = []*Template{defaultTemplate()}
		repo.contactInfo = djInfoJSON()
		emails := &fakeEmails{}
		if mutate != nil {
			mutate(repo, emails)
		}
		return newTestService(repo, emails, &fakeDocs{}, &fakeObjects{}), repo, emails
	}

	t.Run("email unconfigured", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newTestService(repo, nil, &fakeDocs{}, &fakeObjects{})
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrEmailUnavailable) {
			t.Fatalf("expected ErrEmailUnavailable, got %v", err)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		svc, _, _ := newSvc(nil)
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{})
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("contact not found", func(t *testing.T) {
		svc, _, _ := newSvc(nil)
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: uuid.New()})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("contact without email", func(t *testing.T) {
		svc, repo, _ := newSvc(nil)
		noEmail := promoterWithEmail(uuid.New(), "")
		repo.contacts[noEmail.ID] = noEmail
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: noEmail.ID})
		if !errors.Is(err, ErrNoContactEmail) {
			t.Fatalf("expected ErrNoContactEmail, got %v", err)
		}
	})

	t.Run("suppressed", func(t *testing.T) {
		svc, _, _ := newSvc(func(repo *fakeRepo, _ *fakeEmails) {
			repo.suppressedSet["pat@club.example"] = true
		})
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrSuppressed) {
			t.Fatalf("expected ErrSuppressed, got %v", err)
		}
	})

	t.Run("no DJ reply-to", func(t *testing.T) {
		svc, _, _ := newSvc(func(repo *fakeRepo, _ *fakeEmails) {
			repo.contactInfo = `{"name":"Test DJ","email":"","phone":""}`
		})
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrNoReplyTo) {
			t.Fatalf("expected ErrNoReplyTo, got %v", err)
		}
	})

	t.Run("no DJ contact info row", func(t *testing.T) {
		svc, _, _ := newSvc(func(repo *fakeRepo, _ *fakeEmails) {
			repo.contactInfo = ""
		})
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrNoReplyTo) {
			t.Fatalf("expected ErrNoReplyTo, got %v", err)
		}
	})

	t.Run("daily cap reached", func(t *testing.T) {
		svc, _, emails := newSvc(func(repo *fakeRepo, _ *fakeEmails) {
			repo.insertErr = ErrRateLimited
		})
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrRateLimited) {
			t.Fatalf("expected ErrRateLimited, got %v", err)
		}
		if len(emails.sent) != 0 {
			t.Fatal("no email may be sent when the cap refuses the insert")
		}
	})

	t.Run("legacy EPK export without object key", func(t *testing.T) {
		exportID := uuid.New()
		svc, _, _ := newSvc(func(repo *fakeRepo, _ *fakeEmails) {
			repo.epkExports[exportID] = &EPKExportRef{ID: exportID, GarageObjectKey: ""}
		})
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID, EPKExportID: &exportID})
		if !errors.Is(err, ErrEPKLegacyExport) {
			t.Fatalf("expected ErrEPKLegacyExport, got %v", err)
		}
	})

	t.Run("unknown template", func(t *testing.T) {
		svc, _, _ := newSvc(nil)
		missing := uuid.New()
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID, TemplateID: &missing})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("unknown slot", func(t *testing.T) {
		svc, _, _ := newSvc(nil)
		missing := uuid.New()
		_, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID, AvailabilitySlotID: &missing})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestSendRequestFailedDeliveryRecordedOnRow(t *testing.T) {
	repo := newFakeRepo()
	contactID := uuid.New()
	repo.contacts[contactID] = promoterWithEmail(contactID, "pat@club.example")
	repo.templates = []*Template{defaultTemplate()}
	repo.contactInfo = djInfoJSON()
	// The email service records delivery failure on the row (status=failed),
	// it does not return an error.
	emails := &fakeEmails{sentMsg: &email.EmailMessage{ID: uuid.New(), Status: email.EmailStatusFailed, LastError: "plunk 500"}}

	svc := newTestService(repo, emails, &fakeDocs{}, &fakeObjects{})
	row, err := svc.SendRequest(context.Background(), &SendRequestInput{ContactID: contactID})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if row.Status != string(email.EmailStatusFailed) {
		t.Fatalf("row status = %q, want failed", row.Status)
	}
	if row.EmailMessageID == nil {
		t.Fatal("row must link the failed message")
	}
}

func TestMatchSlots(t *testing.T) {
	repo := newFakeRepo()
	c1 := promoterWithEmail(uuid.New(), "a@club.example")
	c1.City = "Berlin"
	c2 := promoterWithEmail(uuid.New(), "b@club.example")
	c2.City = "Hamburg"
	repo.contacts[c1.ID] = c1
	repo.contacts[c2.ID] = c2
	repo.venues = []*MatchedVenue{{ID: uuid.New(), Name: "Berghain", City: "Berlin"}}
	slotBerlin := &Slot{ID: uuid.New(), Date: Date(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)), City: "Berlin"}
	slotHamburg := &Slot{ID: uuid.New(), Date: Date(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)), City: "Hamburg"}
	repo.slots = []*Slot{slotBerlin, slotHamburg}

	svc := newTestService(repo, &fakeEmails{}, &fakeDocs{}, &fakeObjects{})

	t.Run("all slots", func(t *testing.T) {
		matches, err := svc.MatchSlots(context.Background(), nil)
		if err != nil {
			t.Fatalf("MatchSlots: %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("matches = %d", len(matches))
		}
	})

	t.Run("one slot filters contacts by city", func(t *testing.T) {
		matches, err := svc.MatchSlots(context.Background(), &slotBerlin.ID)
		if err != nil {
			t.Fatalf("MatchSlots: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("matches = %d", len(matches))
		}
		m := matches[0]
		if m.Slot.ID != slotBerlin.ID {
			t.Fatalf("slot = %v", m.Slot.ID)
		}
		if len(m.Contacts) != 1 || m.Contacts[0].ID != c1.ID {
			t.Fatalf("contacts = %+v", m.Contacts)
		}
		if len(m.Venues) != 1 || m.Venues[0].Name != "Berghain" {
			t.Fatalf("venues = %+v", m.Venues)
		}
	})

	t.Run("unknown slot", func(t *testing.T) {
		missing := uuid.New()
		if _, err := svc.MatchSlots(context.Background(), &missing); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestSlotCRUDValidation(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeEmails{}, &fakeDocs{}, &fakeObjects{})
	svc.SetNow(func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local) })

	if _, err := svc.CreateSlot(context.Background(), &SlotCreate{City: "Berlin"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("past/missing date must fail validation, got %v", err)
	}
	slot, err := svc.CreateSlot(context.Background(), &SlotCreate{
		Date: Date(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)),
		City: "Berlin",
	})
	if err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}
	if err := svc.DeleteSlot(context.Background(), slot.ID); err != nil {
		t.Fatalf("DeleteSlot: %v", err)
	}
	if len(repo.slots) != 0 {
		t.Fatalf("slot not deleted: %d", len(repo.slots))
	}
}

func TestTemplateVersionConflict(t *testing.T) {
	repo := newFakeRepo()
	tpl := defaultTemplate()
	repo.templates = []*Template{tpl}
	svc := newTestService(repo, &fakeEmails{}, &fakeDocs{}, &fakeObjects{})

	name := "Renamed"
	if _, err := svc.UpdateTemplate(context.Background(), tpl.ID, &TemplateUpdate{Version: 99, Name: &name}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	updated, err := svc.UpdateTemplate(context.Background(), tpl.ID, &TemplateUpdate{Version: 1, Name: &name})
	if err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if updated.Name != "Renamed" || updated.Version != 2 {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestSuppressions(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeEmails{}, &fakeDocs{}, &fakeObjects{})

	if _, err := svc.AddSuppression(context.Background(), &SuppressionCreate{Email: "bad"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
	s, err := svc.AddSuppression(context.Background(), &SuppressionCreate{Email: " DJ@Club.example ", Reason: "asked"})
	if err != nil {
		t.Fatalf("AddSuppression: %v", err)
	}
	if s.Email != "dj@club.example" {
		t.Fatalf("email not normalized: %q", s.Email)
	}
	suppressed, err := repo.IsSuppressed(context.Background(), "dj@club.example")
	if err != nil || !suppressed {
		t.Fatalf("IsSuppressed = %v, %v", suppressed, err)
	}
	if err := svc.DeleteSuppression(context.Background(), s.ID); err != nil {
		t.Fatalf("DeleteSuppression: %v", err)
	}
	suppressed, _ = repo.IsSuppressed(context.Background(), "dj@club.example")
	if suppressed {
		t.Fatal("suppression not removed")
	}
}

func TestLocalMidnight(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*3600)
	tm := time.Date(2026, 10, 15, 23, 30, 0, 0, loc)
	mid := localMidnight(tm)
	if mid.Hour() != 0 || mid.Minute() != 0 || mid.Day() != 15 || mid.Location() != loc {
		t.Fatalf("localMidnight = %v", mid)
	}
}

func TestVariables(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeEmails{}, &fakeDocs{}, &fakeObjects{})
	slot := &Slot{
		City: "Berlin", Country: "DE",
		Date:   Date(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)),
		Genres: []string{"techno", "house"},
		FeeMin: ptrFloat(100), FeeMax: ptrFloat(200), Currency: ptrString("EUR"),
		Notes: "open deck",
	}
	vars := svc.variables(promoterWithEmail(uuid.New(), "pat@club.example"), slot, ContactInfo{Name: "DJ", Email: "dj@x.example", Phone: "123"})
	want := map[string]string{
		"dj_name":          "DJ",
		"dj_email":         "dj@x.example",
		"dj_phone":         "123",
		"promoter_name":    "Promoter Pat",
		"promoter_company": "Club X",
		"epk_link":         "https://epk.klubhub.example/abc",
		"city":             "Berlin",
		"country":          "DE",
		"date":             "2026-11-01",
		"genres":           "techno, house",
		"fee":              "100–200 EUR",
		"notes":            "open deck",
	}
	for k, v := range want {
		if vars[k] != v {
			t.Fatalf("vars[%q] = %q, want %q", k, vars[k], v)
		}
	}

	// Nil slot: slot variables are simply absent.
	vars = svc.variables(promoterWithEmail(uuid.New(), "pat@club.example"), nil, ContactInfo{})
	if _, ok := vars["city"]; ok {
		t.Fatal("city must be absent without a slot")
	}
	if vars["epk_link"] != "https://epk.klubhub.example/abc" {
		t.Fatalf("epk_link = %q", vars["epk_link"])
	}
}

func TestNewServiceDefaultsMaxPerDay(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeEmails{}, &fakeDocs{}, &fakeObjects{}, ServiceConfig{})
	if svc.cfg.MaxPerDay != 50 {
		t.Fatalf("MaxPerDay = %d, want 50", svc.cfg.MaxPerDay)
	}
}

func ptrString(s string) *string { return &s }
