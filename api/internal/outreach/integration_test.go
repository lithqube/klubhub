package outreach

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/docstore"
	"github.com/klubhub/dj/api/internal/platform/email"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// Real-Postgres coverage for the outreach package: migrations 033-035 apply
// cleanly, the repository round-trips against the real schema, and the send
// path runs end to end through the real email outbox + docstore pointer with
// a fake delivery sender and an in-memory object store.

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("SKIP_INTEGRATION") == "1" || testing.Short() {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		tcpostgres.WithDatabase("klubhub_outreach_test"),
		tcpostgres.WithUsername("klubhub"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		println("testcontainers: cannot start postgres, skipping integration:", err.Error())
		os.Exit(m.Run())
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		println("testcontainers: connection string:", err.Error())
		os.Exit(1)
	}
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("sql.Open:", err.Error())
		os.Exit(1)
	}
	if err := migrations.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		println("RunMigrations:", err.Error())
		os.Exit(1)
	}
	_ = db.Close()
	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("pgxpool.New:", err.Error())
		os.Exit(1)
	}

	code := m.Run()
	testPool.Close()
	_ = container.Terminate(context.Background())
	os.Exit(code)
}

// ─── Fakes (delivery + object storage) ───────────────────────────────────────

// fakeSender records delivered messages and can be told to fail.
type fakeSender struct {
	mu   sync.Mutex
	msgs []*email.EmailMessage
	fail bool
}

func (f *fakeSender) Send(msg *email.EmailMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("smtp: 550 mailbox unavailable")
	}
	f.msgs = append(f.msgs, msg)
	return nil
}

func (f *fakeSender) delivered() []*email.EmailMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*email.EmailMessage{}, f.msgs...)
}

// memStore is an in-memory docstore.ObjectStore.
type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemStore() *memStore { return &memStore{objects: map[string][]byte{}} }

func (m *memStore) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[bucket+"/"+key] = data
	return nil
}

func (m *memStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[bucket+"/"+key]
	if !ok {
		return nil, fmt.Errorf("object %s/%s not found", bucket, key)
	}
	return io.NopCloser(strings.NewReader(string(data))), nil
}

func (m *memStore) DeleteObject(ctx context.Context, bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, bucket+"/"+key)
	return nil
}

// objectReader adapts memStore to the outreach ObjectReaderIface.
type objectReader struct{ store *memStore }

func (o objectReader) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return o.store.GetObject(ctx, bucket, key)
}

// ─── Test rig ────────────────────────────────────────────────────────────────

const testBucket = "klubhub-test"

type rig struct {
	repo    *Repository
	svc     *Service
	sender  *fakeSender
	store   *memStore
	docRepo *docstore.DocumentRepository
}

func newRig(t *testing.T, maxPerDay int) *rig {
	t.Helper()
	if testPool == nil {
		t.Skip("integration: no test pool")
	}
	repo := NewRepository(testPool)
	docRepo := docstore.NewDocumentRepository(testPool)
	store := newMemStore()
	docSvc := docstore.NewDocumentService(docRepo, store, testBucket)
	sender := &fakeSender{}
	emailSvc := email.NewEmailService(email.NewEmailRepository(tenantdb.New(testPool), false), sender)
	emailSvc.WithAttachments(docstore.AttachmentSource{Svc: docSvc})
	svc := NewService(repo, emailSvc, docRepo, objectReader{store: store}, ServiceConfig{
		FromEmail:    "dj@klubhub.example",
		FromName:     "Integration DJ",
		MaxPerDay:    maxPerDay,
		EPKPublicURL: "https://epk.klubhub.example/pub",
		Bucket:       testBucket,
	})
	return &rig{repo: repo, svc: svc, sender: sender, store: store, docRepo: docRepo}
}

func (r *rig) exec(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func (r *rig) queryRow(t *testing.T, query string, args ...interface{}) pgxRow {
	t.Helper()
	return testPool.QueryRow(context.Background(), query, args...)
}

// pgxRow is the minimal row surface the assertions need.
type pgxRow interface {
	Scan(dest ...interface{}) error
}

// seedPromoter inserts a promoter contact and returns its id.
func (r *rig) seedPromoter(t *testing.T, name, email, city string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	r.exec(t, `INSERT INTO contacts (id, name, company, email, type, city) VALUES ($1, $2, 'Club', $3, 'promoter', $4)`,
		id, name, email, city)
	return id
}

// seedSlot inserts an availability slot directly (bypasses validation).
func (r *rig) seedSlot(t *testing.T, date, city string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	r.exec(t, `INSERT INTO outreach_availability_slots (id, date, city, country, genres) VALUES ($1, $2, $3, 'DE', '{techno}')`,
		id, date, city)
	return id
}

// seedEPKExport inserts an epk_exports row with a garage object key and
// stores fake PDF bytes under it.
func (r *rig) seedEPKExport(t *testing.T, withObjectKey bool) (uuid.UUID, []byte) {
	t.Helper()
	id := uuid.New()
	key := ""
	if withObjectKey {
		key = fmt.Sprintf("epk/exports/%s.pdf", id.String())
	}
	r.exec(t, `INSERT INTO epk_exports (id, minio_path, garage_object_key) VALUES ($1, $2, $3)`, id, key, key)
	pdf := []byte("%PDF-1.4 integration fake epk")
	if withObjectKey {
		if err := r.store.PutObject(context.Background(), testBucket, key, strings.NewReader(string(pdf)), int64(len(pdf)), "application/pdf"); err != nil {
			t.Fatalf("PutObject: %v", err)
		}
	}
	return id, pdf
}

// setDJContactInfo replaces the user_settings contact_info block.
func (r *rig) setDJContactInfo(t *testing.T, contactInfo string) {
	t.Helper()
	r.exec(t, `DELETE FROM user_settings`)
	r.exec(t, `INSERT INTO user_settings (contact_info) VALUES ($1)`, contactInfo)
}

// cleanRequests empties the sent log (sequential tests share the table).
func (r *rig) cleanRequests(t *testing.T) {
	t.Helper()
	r.exec(t, `DELETE FROM outreach_requests`)
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestIntegrationMigrationsSeededDefaultTemplate(t *testing.T) {
	r := newRig(t, 50)
	tpl, err := r.repo.GetDefaultTemplate(context.Background())
	if err != nil {
		t.Fatalf("seeded default template: %v", err)
	}
	if tpl.Direction != "dj_to_promoter" || !tpl.IsDefault || tpl.Version != 1 {
		t.Fatalf("template = %+v", tpl)
	}
	if !strings.Contains(tpl.Body, "{{dj_name}}") || !strings.Contains(tpl.Body, "{{promoter_name}}") {
		t.Fatalf("seeded body missing variables: %q", tpl.Body)
	}
	if !strings.Contains(tpl.Body, "stop") {
		t.Fatalf("seeded body missing the opt-out footer: %q", tpl.Body)
	}
}

func TestIntegrationSlotCRUD(t *testing.T) {
	r := newRig(t, 50)
	ctx := context.Background()
	future := time.Now().AddDate(0, 1, 0).UTC().Format(DateFormat)

	cur := "eur"
	slot, err := r.svc.CreateSlot(ctx, &SlotCreate{
		Date:     Date(time.Now().AddDate(0, 1, 0).UTC()),
		City:     "Integration City",
		Country:  "DE",
		Genres:   []string{"techno"},
		FeeMin:   ptrFloat(100),
		FeeMax:   ptrFloat(200),
		Currency: &cur,
		Notes:    "integration slot",
	})
	if err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}
	if slot.ID == uuid.Nil || slot.City != "Integration City" {
		t.Fatalf("slot = %+v", slot)
	}
	if slot.Currency == nil || *slot.Currency != "EUR" {
		t.Fatalf("currency not normalized: %+v", slot.Currency)
	}
	if got := time.Time(slot.Date).UTC().Format(DateFormat); got != future {
		t.Fatalf("date = %q, want %q", got, future)
	}

	got, err := r.repo.GetSlot(ctx, slot.ID)
	if err != nil {
		t.Fatalf("GetSlot: %v", err)
	}
	if got.Notes != "integration slot" || len(got.Genres) != 1 || got.Genres[0] != "techno" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.FeeMin == nil || *got.FeeMin != 100 || got.FeeMax == nil || *got.FeeMax != 200 {
		t.Fatalf("fee round-trip: %+v", got)
	}

	// UNIQUE (date, city, country) maps to ErrConflict.
	_, err = r.svc.CreateSlot(ctx, &SlotCreate{
		Date:  Date(time.Now().AddDate(0, 1, 0).UTC()),
		City:  "Integration City",
		Country: "DE",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate slot: expected ErrConflict, got %v", err)
	}

	// A different country is a different slot.
	other, err := r.svc.CreateSlot(ctx, &SlotCreate{
		Date:    Date(time.Now().AddDate(0, 1, 0).UTC()),
		City:    "Integration City",
		Country: "AT",
	})
	if err != nil {
		t.Fatalf("CreateSlot other country: %v", err)
	}

	// Past dates are refused by validation.
	_, err = r.svc.CreateSlot(ctx, &SlotCreate{
		Date: Date(time.Now().AddDate(0, 0, -2).UTC()),
		City: "Integration City",
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("past date: expected ErrValidation, got %v", err)
	}

	// List + delete.
	slots, err := r.svc.ListSlots(ctx)
	if err != nil {
		t.Fatalf("ListSlots: %v", err)
	}
	if len(slots) < 2 {
		t.Fatalf("slots = %d", len(slots))
	}
	if err := r.svc.DeleteSlot(ctx, slot.ID); err != nil {
		t.Fatalf("DeleteSlot: %v", err)
	}
	if _, err := r.repo.GetSlot(ctx, slot.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted slot: expected ErrNotFound, got %v", err)
	}
	if err := r.svc.DeleteSlot(ctx, other.ID); err != nil {
		t.Fatalf("DeleteSlot other: %v", err)
	}
}

func TestIntegrationTemplateCRUD(t *testing.T) {
	r := newRig(t, 50)
	ctx := context.Background()

	// The seeded default is listed first.
	templates, err := r.svc.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) == 0 || !templates[0].IsDefault {
		t.Fatalf("default must be listed first: %+v", templates)
	}
	seededDefault := templates[0]
	// Later tests rely on a default template existing — restore it.
	t.Cleanup(func() {
		r.exec(t, `UPDATE outreach_templates SET is_default = true, updated_at = now() WHERE id = $1`, seededDefault.ID)
	})

	// Create a second template as default: the seeded one is cleared.
	second, err := r.svc.CreateTemplate(ctx, &TemplateCreate{
		Name:      "Integration second",
		Subject:   "Second {{city}}",
		Body:      "Body {{dj_name}}",
		IsDefault: true,
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	def, err := r.repo.GetDefaultTemplate(ctx)
	if err != nil {
		t.Fatalf("GetDefaultTemplate: %v", err)
	}
	if def.ID != second.ID {
		t.Fatalf("default = %v, want the new template %v", def.ID, second.ID)
	}

	// Duplicate name maps to ErrConflict (unique partial index).
	_, err = r.svc.CreateTemplate(ctx, &TemplateCreate{Name: "Integration second", Subject: "s", Body: "b"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name: expected ErrConflict, got %v", err)
	}

	// Optimistic concurrency: stale version → ErrConflict, current → version+1.
	name := "Integration second renamed"
	_, err = r.svc.UpdateTemplate(ctx, second.ID, &TemplateUpdate{Version: 99, Name: &name})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version: expected ErrConflict, got %v", err)
	}
	updated, err := r.svc.UpdateTemplate(ctx, second.ID, &TemplateUpdate{Version: second.Version, Name: &name})
	if err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if updated.Name != name || updated.Version != second.Version+1 {
		t.Fatalf("updated = %+v", updated)
	}

	// Soft delete: gone from reads, still in the table.
	if err := r.svc.DeleteTemplate(ctx, second.ID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	if _, err := r.repo.GetTemplate(ctx, second.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted template: expected ErrNotFound, got %v", err)
	}
	var deletedAt *time.Time
	if err := r.queryRow(t, `SELECT deleted_at FROM outreach_templates WHERE id = $1`, second.ID).Scan(&deletedAt); err != nil {
		t.Fatalf("select deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("template must be soft-deleted, not removed")
	}
}

func TestIntegrationMatchContacts(t *testing.T) {
	r := newRig(t, 50)
	ctx := context.Background()

	city := "Match City"
	withGig := r.seedPromoter(t, "Zoe Pastgig", "zoe@match.example", city)
	plain := r.seedPromoter(t, "Aaron Plain", "aaron@match.example", " "+city+" ") // trimmed match
	noEmail := r.seedPromoter(t, "No Email", "", city)
	suppressed := r.seedPromoter(t, "Suppressed Sam", "sam@match.example", city)
	otherCity := r.seedPromoter(t, "Hamburg Hank", "hank@match.example", "Hamburg")

	// A past gig in the city whose promoter_email matches Zoe.
	r.exec(t, `INSERT INTO gigs (date, city, promoter_name, promoter_email, status)
		VALUES (now() - interval '30 days', $1, 'Zoe Pastgig', '  ZOE@match.example ', 'played')`, city)
	// A future gig must not count.
	r.exec(t, `INSERT INTO gigs (date, city, promoter_name, promoter_email, status)
		VALUES (now() + interval '30 days', $1, 'Aaron Plain', 'aaron@match.example', 'confirmed')`, city)
	// A past gig in another city must not count for Aaron.
	r.exec(t, `INSERT INTO gigs (date, city, promoter_name, promoter_email, status)
		VALUES (now() - interval '10 days', 'Hamburg', 'Aaron Plain', 'aaron@match.example', 'played')`)

	// A soft-deleted promoter in the city must not match.
	deleted := r.seedPromoter(t, "Deleted Dan", "dan@match.example", city)
	r.exec(t, `UPDATE contacts SET deleted_at = now() WHERE id = $1`, deleted)
	// A non-promoter in the city must not match.
	r.exec(t, `INSERT INTO contacts (id, name, email, type, city) VALUES ($1, 'Agent Amy', 'amy@match.example', 'agent', $2)`,
		uuid.New(), city)

	// Suppress Sam.
	if _, err := r.svc.AddSuppression(ctx, &SuppressionCreate{Email: "SAM@match.example", Reason: "asked"}); err != nil {
		t.Fatalf("AddSuppression: %v", err)
	}

	contacts, err := r.repo.MatchContacts(ctx, " match city ") // case + trim
	if err != nil {
		t.Fatalf("MatchContacts: %v", err)
	}
	if len(contacts) != 3 {
		t.Fatalf("contacts = %d (%+v), want 3 (Zoe, Aaron, No Email)", len(contacts), contacts)
	}
	// Ranking: past gig in city first, then alphabetical (Aaron < No Email).
	if contacts[0].ID != withGig || !contacts[0].PastGigInCity {
		t.Fatalf("first = %+v, want Zoe with past_gig_in_city", contacts[0])
	}
	if contacts[1].ID != plain || contacts[1].PastGigInCity {
		t.Fatalf("second = %+v, want Aaron without past gig (future/other-city gigs don't count)", contacts[1])
	}
	if contacts[2].ID != noEmail || contacts[2].HasEmail {
		t.Fatalf("third = %+v, want the no-email contact with has_email=false", contacts[2])
	}
	for _, c := range contacts {
		if c.ID == suppressed || c.ID == otherCity || c.ID == deleted {
			t.Fatalf("excluded contact matched: %+v", c)
		}
	}

	// Venues match by city too.
	venueID := uuid.New()
	r.exec(t, `INSERT INTO venues (id, name, city) VALUES ($1, 'Match Venue', $2)`, venueID, city)
	venues, err := r.repo.MatchVenues(ctx, city)
	if err != nil {
		t.Fatalf("MatchVenues: %v", err)
	}
	if len(venues) != 1 || venues[0].ID != venueID {
		t.Fatalf("venues = %+v", venues)
	}
}

func TestIntegrationSendRequestEndToEnd(t *testing.T) {
	r := newRig(t, 50)
	ctx := context.Background()
	r.cleanRequests(t)
	r.setDJContactInfo(t, `{"name":"Integration DJ","email":"dj@klubhub.example","phone":"+49 30 123"}`)

	contactID := r.seedPromoter(t, "Pat Promoter", "pat@e2e.example", "E2E City")
	slotID := r.seedSlot(t, time.Now().AddDate(0, 2, 0).UTC().Format(DateFormat), "E2E City")
	exportID, pdf := r.seedEPKExport(t, true)

	row, err := r.svc.SendRequest(ctx, &SendRequestInput{
		ContactID:          contactID,
		AvailabilitySlotID: &slotID,
		EPKExportID:        &exportID,
	})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if row.EmailMessageID == nil {
		t.Fatal("row must link the email message")
	}
	if row.Status != string(email.EmailStatusSent) {
		t.Fatalf("row status = %q", row.Status)
	}

	// The outbox row: kind, ReplyTo = the DJ's contact email. With an EPK
	// attach the message is owned by the export (SendOwned requires the
	// attachment owner to match the message owner); the sent-log row keeps
	// the link back to this message.
	var (
		kind, replyTo, ownerType string
		ownerID                  uuid.UUID
		attachmentIDs            []uuid.UUID
		subject                  string
	)
	if err := r.queryRow(t, `
		SELECT kind, reply_to, owner_type, owner_id, attachment_ids, subject
		FROM email_messages WHERE id = $1`, *row.EmailMessageID).
		Scan(&kind, &replyTo, &ownerType, &ownerID, &attachmentIDs, &subject); err != nil {
		t.Fatalf("select email_messages: %v", err)
	}
	if kind != string(EmailKindBookingRequest) {
		t.Fatalf("kind = %q", kind)
	}
	if replyTo != "dj@klubhub.example" {
		t.Fatalf("reply_to = %q, want the DJ's contact email", replyTo)
	}
	if ownerType != string(docstore.DocumentOwnerEPK) || ownerID != exportID {
		t.Fatalf("owner = %q/%v, want epk/%v", ownerType, ownerID, exportID)
	}
	if len(attachmentIDs) != 1 {
		t.Fatalf("attachment_ids = %v, want the EPK pointer", attachmentIDs)
	}
	if !strings.Contains(subject, "Integration DJ") || !strings.Contains(subject, "E2E City") {
		t.Fatalf("subject not rendered: %q", subject)
	}

	// The documents pointer row: owner_type='epk', kind='epk_pdf' (accepted by
	// the documents CHECK extended in migration 034), storage_key = the
	// export's garage object key, checksum = sha256 of the object bytes.
	var (
		docOwnerType, docKind, storageKey, checksum string
		docOwnerID                                  uuid.UUID
		docSize                                     int64
	)
	if err := r.queryRow(t, `
		SELECT owner_type, owner_id, kind, storage_key, checksum_sha256, size_bytes
		FROM documents WHERE id = $1`, attachmentIDs[0]).
		Scan(&docOwnerType, &docOwnerID, &docKind, &storageKey, &checksum, &docSize); err != nil {
		t.Fatalf("select documents: %v", err)
	}
	if docOwnerType != string(docstore.DocumentOwnerEPK) || docOwnerID != exportID {
		t.Fatalf("pointer owner = %q/%v", docOwnerType, docOwnerID)
	}
	if docKind != string(docstore.DocumentKindEPKPDF) {
		t.Fatalf("pointer kind = %q", docKind)
	}
	wantKey := fmt.Sprintf("epk/exports/%s.pdf", exportID.String())
	if storageKey != wantKey {
		t.Fatalf("storage_key = %q, want %q", storageKey, wantKey)
	}
	sum := sha256.Sum256(pdf)
	if checksum != hex.EncodeToString(sum[:]) || docSize != int64(len(pdf)) {
		t.Fatalf("checksum/size = %q/%d", checksum, docSize)
	}

	// The sender received the message with the attachment content loaded
	// from the object store.
	delivered := r.sender.delivered()
	if len(delivered) != 1 {
		t.Fatalf("delivered = %d", len(delivered))
	}
	msg := delivered[0]
	if msg.ToEmail != "pat@e2e.example" || msg.ReplyTo != "dj@klubhub.example" {
		t.Fatalf("msg to/reply = %q/%q", msg.ToEmail, msg.ReplyTo)
	}
	if len(msg.Attachments) != 1 || string(msg.Attachments[0].Content) != string(pdf) {
		t.Fatalf("attachments = %+v", msg.Attachments)
	}

	// The sent log joins the delivery status.
	got, err := r.svc.GetRequest(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.EmailStatus != string(email.EmailStatusSent) || got.EmailError != "" {
		t.Fatalf("joined status = %q/%q", got.EmailStatus, got.EmailError)
	}
	if got.TemplateID == nil {
		t.Fatal("row must link the template used")
	}
	if got.EPKExportID == nil || *got.EPKExportID != exportID {
		t.Fatalf("row epk link = %v", got.EPKExportID)
	}

	// The list endpoint returns the row, newest first.
	list, err := r.svc.ListRequests(ctx)
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if len(list) != 1 || list[0].ID != row.ID {
		t.Fatalf("list = %+v", list)
	}
	if _, err := r.svc.GetRequest(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown request: expected ErrNotFound, got %v", err)
	}

	// A second send for the same export reuses the pointer (no new document).
	row2, err := r.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID, EPKExportID: &exportID})
	if err != nil {
		t.Fatalf("second SendRequest: %v", err)
	}
	var docCount int
	if err := r.queryRow(t, `SELECT count(*) FROM documents WHERE owner_type = 'epk' AND owner_id = $1`, exportID).Scan(&docCount); err != nil {
		t.Fatalf("count documents: %v", err)
	}
	if docCount != 1 {
		t.Fatalf("documents for export = %d, want 1 (pointer reused)", docCount)
	}
	var attIDs2 []uuid.UUID
	if err := r.queryRow(t, `SELECT attachment_ids FROM email_messages WHERE id = $1`, *row2.EmailMessageID).Scan(&attIDs2); err != nil {
		t.Fatalf("select second message: %v", err)
	}
	if len(attIDs2) != 1 || attIDs2[0] != attachmentIDs[0] {
		t.Fatalf("second attachment_ids = %v, want the reused pointer", attIDs2)
	}
}

func TestIntegrationSendRequestRefusals(t *testing.T) {
	r := newRig(t, 50)
	ctx := context.Background()
	r.cleanRequests(t)
	r.setDJContactInfo(t, `{"name":"Integration DJ","email":"dj@klubhub.example"}`)

	contactID := r.seedPromoter(t, "Refusal Rita", "rita@refusal.example", "Refusal City")

	t.Run("suppressed", func(t *testing.T) {
		if _, err := r.svc.AddSuppression(ctx, &SuppressionCreate{Email: "rita@refusal.example"}); err != nil {
			t.Fatalf("AddSuppression: %v", err)
		}
		_, err := r.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrSuppressed) {
			t.Fatalf("expected ErrSuppressed, got %v", err)
		}
		// Clean up for the next subtests.
		suppressions, _ := r.svc.ListSuppressions(ctx)
		for _, s := range suppressions {
			if s.Email == "rita@refusal.example" {
				if err := r.svc.DeleteSuppression(ctx, s.ID); err != nil {
					t.Fatalf("DeleteSuppression: %v", err)
				}
			}
		}
	})

	t.Run("contact without email", func(t *testing.T) {
		noEmail := r.seedPromoter(t, "No Email Nora", "", "Refusal City")
		_, err := r.svc.SendRequest(ctx, &SendRequestInput{ContactID: noEmail})
		if !errors.Is(err, ErrNoContactEmail) {
			t.Fatalf("expected ErrNoContactEmail, got %v", err)
		}
	})

	t.Run("non-promoter contact", func(t *testing.T) {
		agent := uuid.New()
		r.exec(t, `INSERT INTO contacts (id, name, email, type, city) VALUES ($1, 'Agent', 'agent@refusal.example', 'agent', 'Refusal City')`, agent)
		_, err := r.svc.SendRequest(ctx, &SendRequestInput{ContactID: agent})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("no DJ reply-to", func(t *testing.T) {
		r.setDJContactInfo(t, `{"name":"Integration DJ","email":""}`)
		_, err := r.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrNoReplyTo) {
			t.Fatalf("expected ErrNoReplyTo, got %v", err)
		}
		r.setDJContactInfo(t, `{"name":"Integration DJ","email":"dj@klubhub.example"}`)
	})

	t.Run("legacy EPK export without object key", func(t *testing.T) {
		exportID, _ := r.seedEPKExport(t, false)
		_, err := r.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID, EPKExportID: &exportID})
		if !errors.Is(err, ErrEPKLegacyExport) {
			t.Fatalf("expected ErrEPKLegacyExport, got %v", err)
		}
	})

	t.Run("daily cap", func(t *testing.T) {
		r.cleanRequests(t)
		capped := newRig(t, 1)
		capped.setDJContactInfo(t, `{"name":"Integration DJ","email":"dj@klubhub.example"}`)
		if _, err := capped.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID}); err != nil {
			t.Fatalf("first send: %v", err)
		}
		_, err := capped.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrRateLimited) {
			t.Fatalf("expected ErrRateLimited, got %v", err)
		}
	})

	t.Run("failed attempts count against the cap", func(t *testing.T) {
		r.cleanRequests(t)
		capped := newRig(t, 1)
		capped.setDJContactInfo(t, `{"name":"Integration DJ","email":"dj@klubhub.example"}`)
		capped.sender.fail = true
		row, err := capped.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID})
		if err != nil {
			t.Fatalf("first send (failing sender): %v", err)
		}
		if row.Status != string(email.EmailStatusFailed) {
			t.Fatalf("row status = %q, want failed", row.Status)
		}
		// The failed attempt still inserted a row, so the cap is reached.
		_, err = capped.svc.SendRequest(ctx, &SendRequestInput{ContactID: contactID})
		if !errors.Is(err, ErrRateLimited) {
			t.Fatalf("expected ErrRateLimited after a failed attempt, got %v", err)
		}
		// The failure is visible in the sent log join.
		got, err := capped.repo.GetRequest(ctx, row.ID)
		if err != nil {
			t.Fatalf("GetRequest: %v", err)
		}
		if got.EmailStatus != string(email.EmailStatusFailed) || got.EmailError == "" {
			t.Fatalf("joined failure = %q/%q", got.EmailStatus, got.EmailError)
		}
	})
}

func TestIntegrationSuppressions(t *testing.T) {
	r := newRig(t, 50)
	ctx := context.Background()

	s1, err := r.svc.AddSuppression(ctx, &SuppressionCreate{Email: " One@Example.COM ", Reason: "asked"})
	if err != nil {
		t.Fatalf("AddSuppression: %v", err)
	}
	if s1.Email != "one@example.com" {
		t.Fatalf("email not normalized: %q", s1.Email)
	}

	// Idempotent: re-adding returns the same row.
	s2, err := r.svc.AddSuppression(ctx, &SuppressionCreate{Email: "one@example.com", Reason: "again"})
	if err != nil {
		t.Fatalf("re-AddSuppression: %v", err)
	}
	if s2.ID != s1.ID {
		t.Fatalf("re-add created a new row: %v vs %v", s2.ID, s1.ID)
	}

	suppressed, err := r.repo.IsSuppressed(ctx, "one@example.com")
	if err != nil || !suppressed {
		t.Fatalf("IsSuppressed = %v, %v", suppressed, err)
	}

	list, err := r.svc.ListSuppressions(ctx)
	if err != nil {
		t.Fatalf("ListSuppressions: %v", err)
	}
	found := false
	for _, s := range list {
		if s.ID == s1.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("suppression missing from list")
	}

	if err := r.svc.DeleteSuppression(ctx, s1.ID); err != nil {
		t.Fatalf("DeleteSuppression: %v", err)
	}
	suppressed, _ = r.repo.IsSuppressed(ctx, "one@example.com")
	if suppressed {
		t.Fatal("suppression not removed")
	}
	if err := r.svc.DeleteSuppression(ctx, s1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete: expected ErrNotFound, got %v", err)
	}
}

func TestIntegrationMatchSlotsService(t *testing.T) {
	r := newRig(t, 50)
	ctx := context.Background()

	city := "SlotMatch City"
	contactID := r.seedPromoter(t, "Match Mia", "mia@slotmatch.example", city)
	slotID := r.seedSlot(t, time.Now().AddDate(0, 3, 0).UTC().Format(DateFormat), city)

	matches, err := r.svc.MatchSlots(ctx, &slotID)
	if err != nil {
		t.Fatalf("MatchSlots: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d", len(matches))
	}
	m := matches[0]
	if m.Slot.ID != slotID {
		t.Fatalf("slot = %v", m.Slot.ID)
	}
	if len(m.Contacts) != 1 || m.Contacts[0].ID != contactID {
		t.Fatalf("contacts = %+v", m.Contacts)
	}
}
