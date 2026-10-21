package bookings_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/email"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/bookings"
)

var testDB *pgtest.DB

func TestMain(m *testing.M) {
	flag.Parse()
	db, err := pgtest.StartPromoter()
	if err != nil {
		panic(err)
	}
	testDB = db
	code := m.Run()
	db.Close()
	os.Exit(code)
}

// fakeSender is the delivery transport: it captures every message (the
// service hands it the unsealed copy) and can be told to fail.
type fakeSender struct {
	mu   sync.Mutex
	msgs []*email.EmailMessage
	err  error
}

func (f *fakeSender) Send(msg *email.EmailMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, msg)
	return f.err
}

func (f *fakeSender) last() *email.EmailMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.msgs) == 0 {
		return nil
	}
	return f.msgs[len(f.msgs)-1]
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

type fixture struct {
	t        *testing.T
	svc      *bookings.Service
	emailSvc *email.EmailService
	sender   *fakeSender
	db       *tenantdb.DB
	keys     *envelope.Keyring
	ctx      context.Context
	org      uuid.UUID
	member   uuid.UUID
	now      time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	org := uuid.Must(uuid.NewV7())
	bg := context.Background()
	if err := tenantdb.WithTenant(bg, testDB.App, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(bg, `INSERT INTO organizations (id, name, slug, timezone) VALUES ($1, 'Nachtwerk', $2, 'Europe/Berlin')`,
			org, "nw-"+org.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, err := envelope.NewKEK("kek-test", raw)
	if err != nil {
		t.Fatal(err)
	}
	keys := envelope.NewKeyring(kek)
	db := tenantdb.New(testDB.App)
	f := &fixture{
		t: t, db: db, keys: keys,
		ctx: tenantdb.ContextWithTenant(bg, org), org: org,
		now: time.Now().UTC(),
	}
	f.member = f.insertMember("Mara Booker", "mara@nachtwerk.example")

	// Wire the outbox exactly like cmd/promoter: tenant-scoped repository,
	// envelope-backed codec, the service as FieldSealer.
	sender := &fakeSender{}
	repo := email.NewEmailRepository(db)
	emailSvc := email.NewEmailService(repo, sender)
	emailSvc.WithFieldCodec(bookings.EnvelopeCodec{Keys: keys})
	repo.WithFieldSealer(emailSvc)
	f.sender = sender
	f.emailSvc = emailSvc
	f.svc = bookings.NewService(db, keys, emailSvc, bookings.ServiceConfig{
		FromEmail: "offers@klubhub.example",
		FromName:  "KlubHub Outreach",
		MaxPerDay: 50,
	}, func() time.Time { return f.now })
	return f
}

// insertMember seals the user row exactly like identity.insertUser.
func (f *fixture) insertMember(name, addr string) uuid.UUID {
	f.t.Helper()
	id := uuid.Must(uuid.NewV7())
	bg := context.Background()
	err := f.db.WithTenant(bg, f.org, func(tx pgx.Tx) error {
		dek, err := f.keys.Current(bg, tx, f.org)
		if err != nil {
			return err
		}
		norm := envelope.NormalizeEmail(addr)
		emailEnc, err := dek.Seal(f.org, envelope.Field{Table: "users", Column: "email_enc", RowID: id}, []byte(norm))
		if err != nil {
			return err
		}
		nameEnc, err := dek.Seal(f.org, envelope.Field{Table: "users", Column: "display_name_enc", RowID: id}, []byte(strings.TrimSpace(name)))
		if err != nil {
			return err
		}
		_, err = tx.Exec(bg, `INSERT INTO users (id, tenant_id, email_enc, email_bidx, display_name_enc) VALUES ($1, $2, $3, $4, $5)`,
			id, f.org, emailEnc, dek.BlindIndex("users", "email", norm), nameEnc)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) insertVenue(name, city string) uuid.UUID {
	f.t.Helper()
	id := uuid.Must(uuid.NewV7())
	err := f.db.WithTenant(context.Background(), f.org, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO venues (id, tenant_id, name, city, timezone) VALUES ($1, $2, $3, $4, 'Europe/Berlin')`,
			id, f.org, name, city)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) insertEvent(title, slug, city, locationMode string, revealAt *time.Time, genres []string, venueID *uuid.UUID) uuid.UUID {
	f.t.Helper()
	if genres == nil {
		genres = []string{}
	}
	id := uuid.Must(uuid.NewV7())
	start := f.now.Add(72 * time.Hour)
	err := f.db.WithTenant(context.Background(), f.org, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO events (id, tenant_id, title, slug, starts_at, ends_at, timezone, city, venue_id, location_mode, location_reveal_at, genres)
			VALUES ($1, $2, $3, $4, $5, $6, 'Europe/Berlin', $7, $8, $9, $10, $11)`,
			id, f.org, title, slug, start, start.Add(6*time.Hour), city, venueID, locationMode, revealAt, genres)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) principal() authz.Principal {
	return authz.Principal{
		Sub: "local:" + f.member.String(), OrgID: f.org.String(),
		Roles: []string{"owner"}, AMR: []string{"pwd", "otp"}, AuthTime: time.Now(),
	}
}

func (f *fixture) contact(name, addr, city string, genres []string) *bookings.TalentContact {
	f.t.Helper()
	c, err := f.svc.CreateTalentContact(f.ctx, bookings.TalentContactInput{
		Name: name, Email: addr, City: city, Genres: genres, Source: bookings.SourcePastGig,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}

func (f *fixture) slot(eventID uuid.UUID, role string, genres []string) *bookings.OpenSlot {
	f.t.Helper()
	s, err := f.svc.CreateOpenSlot(f.ctx, eventID, bookings.OpenSlotInput{Role: role, Genres: genres})
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

// ─── Talent contacts ─────────────────────────────────────────────────────────

func TestTalentContact_SealRoundTrip(t *testing.T) {
	f := setup(t)
	c, err := f.svc.CreateTalentContact(f.ctx, bookings.TalentContactInput{
		Name: "Nadia Voss", Email: "nadia@example.com", Phone: "+49 30 1",
		City: "Berlin", Country: "DE", Genres: []string{"techno"}, Source: bookings.SourceReferral, Notes: "met at Berghain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !c.HasEmail || c.Name != "Nadia Voss" || c.Email != "nadia@example.com" {
		t.Fatalf("create response: %+v", c)
	}

	got, err := f.svc.GetTalentContact(f.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Nadia Voss" || got.Email != "nadia@example.com" || got.Phone != "+49 30 1" ||
		got.City != "Berlin" || got.Country != "DE" || got.Source != bookings.SourceReferral || got.Notes != "met at Berghain" {
		t.Fatalf("round trip lost fields: %+v", got)
	}

	list, err := f.svc.ListTalentContacts(f.ctx, bookings.TalentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Email != "nadia@example.com" {
		t.Fatalf("list must return the contact unsealed: %+v", list)
	}

	// The PII columns are sealed at rest: the plaintext must not appear in
	// the stored bytes (owner pool bypasses RLS for the raw check).
	var nameEnc, emailEnc, phoneEnc []byte
	var nameBidx, emailBidx []byte
	err = testDB.Owner.QueryRow(context.Background(),
		`SELECT name_enc, email_enc, phone_enc, name_bidx, email_bidx FROM talent_contacts WHERE id = $1`, c.ID).
		Scan(&nameEnc, &emailEnc, &phoneEnc, &nameBidx, &emailBidx)
	if err != nil {
		t.Fatal(err)
	}
	for _, enc := range [][]byte{nameEnc, emailEnc, phoneEnc} {
		if len(enc) == 0 {
			t.Fatal("sealed column is empty")
		}
	}
	if bytes.Contains(nameEnc, []byte("Nadia")) || bytes.Contains(emailEnc, []byte("nadia@example.com")) {
		t.Fatal("plaintext PII found in a sealed column")
	}
	if len(nameBidx) == 0 || len(emailBidx) == 0 {
		t.Fatal("blind indexes must be stored")
	}
}

func TestTalentContact_Dedupe(t *testing.T) {
	f := setup(t)
	f.contact("Lars", "lars@example.com", "Berlin", nil)
	// Same address, different case and padding: the blind index dedupes.
	_, err := f.svc.CreateTalentContact(f.ctx, bookings.TalentContactInput{Name: "Lars Again", Email: " Lars@Example.COM "})
	if !errors.Is(err, bookings.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestTalentContact_NameOnlyHasNoEmail(t *testing.T) {
	f := setup(t)
	c, err := f.svc.CreateTalentContact(f.ctx, bookings.TalentContactInput{Name: "Resident DJ", City: "Berlin"})
	if err != nil {
		t.Fatal(err)
	}
	if c.HasEmail {
		t.Fatalf("name-only contact must have has_email=false: %+v", c)
	}
	list, err := f.svc.ListTalentContacts(f.ctx, bookings.TalentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].HasEmail || list[0].Email != "" {
		t.Fatalf("list must show the email-less contact with has_email=false: %+v", list)
	}
	// A name-only contact can never be sent to (OFFER-10).
	_, err = f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID})
	if !errors.Is(err, bookings.ErrNoContactEmail) {
		t.Fatalf("want ErrNoContactEmail, got %v", err)
	}
}

func TestTalentContact_Filters(t *testing.T) {
	f := setup(t)
	f.contact("Nadia Voss", "nadia@example.com", "Berlin", []string{"techno"})
	f.contact("Ben Klock", "ben@example.com", "berlin", []string{"house"})
	f.contact("Richie Hawtin", "richie@example.com", "Hamburg", []string{"techno"})

	byCity, err := f.svc.ListTalentContacts(f.ctx, bookings.TalentFilter{City: "BERLIN"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byCity) != 2 {
		t.Fatalf("city filter is case-insensitive: got %d", len(byCity))
	}
	byGenre, err := f.svc.ListTalentContacts(f.ctx, bookings.TalentFilter{Genre: "techno"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byGenre) != 2 {
		t.Fatalf("genre overlap filter: got %d", len(byGenre))
	}
	// Q matches the exact name or email blind index.
	byName, err := f.svc.ListTalentContacts(f.ctx, bookings.TalentFilter{Q: "Nadia Voss"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byName) != 1 || byName[0].Email != "nadia@example.com" {
		t.Fatalf("Q by name: %+v", byName)
	}
	byEmail, err := f.svc.ListTalentContacts(f.ctx, bookings.TalentFilter{Q: "BEN@EXAMPLE.COM"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byEmail) != 1 || byEmail[0].Name != "Ben Klock" {
		t.Fatalf("Q by email (case-insensitive): %+v", byEmail)
	}
}

func TestTalentContact_UpdateAndDelete(t *testing.T) {
	f := setup(t)
	a := f.contact("Lars", "lars@example.com", "Berlin", nil)
	f.contact("Priya", "priya@example.com", "Berlin", nil)

	upd, err := f.svc.UpdateTalentContact(f.ctx, a.ID, bookings.TalentContactInput{
		Name: "Lars M.", Email: "lars.m@example.com", City: "Leipzig", Genres: []string{"house"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "Lars M." || upd.Email != "lars.m@example.com" || upd.City != "Leipzig" {
		t.Fatalf("update: %+v", upd)
	}
	// The old email is free again; the new one dedupes against Priya.
	if _, err := f.svc.CreateTalentContact(f.ctx, bookings.TalentContactInput{Name: "X", Email: "lars@example.com"}); err != nil {
		t.Fatalf("the old email must be reusable after update: %v", err)
	}
	_, err = f.svc.UpdateTalentContact(f.ctx, a.ID, bookings.TalentContactInput{Name: "Lars", Email: "priya@example.com"})
	if !errors.Is(err, bookings.ErrDuplicate) {
		t.Fatalf("update to a taken email must be ErrDuplicate, got %v", err)
	}

	if err := f.svc.DeleteTalentContact(f.ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GetTalentContact(f.ctx, a.ID); !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("deleted contact must be gone, got %v", err)
	}
	if err := f.svc.DeleteTalentContact(f.ctx, a.ID); !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("second delete must be ErrNotFound, got %v", err)
	}
}

// ─── Open slots ──────────────────────────────────────────────────────────────

func TestOpenSlot_CRUD(t *testing.T) {
	f := setup(t)
	ev := f.insertEvent("Klubnacht", "klubnacht", "Berlin", "venue", nil, []string{"techno"}, nil)

	s1 := f.slot(ev, "Headliner", []string{"techno"})
	f.slot(ev, "Warm-up", nil)

	slots, err := f.svc.ListOpenSlots(f.ctx, ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 || slots[0].ID != s1.ID {
		t.Fatalf("slots: %+v", slots)
	}

	// UNIQUE (event_id, role): a second Headliner on the same event conflicts.
	_, err = f.svc.CreateOpenSlot(f.ctx, ev, bookings.OpenSlotInput{Role: "Headliner"})
	if !errors.Is(err, bookings.ErrConflict) {
		t.Fatalf("duplicate role must be ErrConflict, got %v", err)
	}
	// The same role on ANOTHER event is fine.
	ev2 := f.insertEvent("Zweite Nacht", "zweite-nacht", "Berlin", "venue", nil, nil, nil)
	if _, err := f.svc.CreateOpenSlot(f.ctx, ev2, bookings.OpenSlotInput{Role: "Headliner"}); err != nil {
		t.Fatalf("same role on another event: %v", err)
	}
	// A slot on an event that does not exist (or is not ours) is not found.
	_, err = f.svc.CreateOpenSlot(f.ctx, uuid.New(), bookings.OpenSlotInput{Role: "X"})
	if !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("unknown event must be ErrNotFound, got %v", err)
	}

	if err := f.svc.DeleteOpenSlot(f.ctx, s1.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteOpenSlot(f.ctx, s1.ID); !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("second delete must be ErrNotFound, got %v", err)
	}
}

// ─── Matching (SPEC §8) ──────────────────────────────────────────────────────

func TestMatchSlot_RankingAndFilters(t *testing.T) {
	f := setup(t)
	ev := f.insertEvent("Klubnacht", "klubnacht", "Berlin", "venue", nil, []string{"techno", "house"}, nil)
	slot := f.slot(ev, "Headliner", nil)

	// Two overlapping genres → ranked first.
	f.contact("Nadia Voss", "nadia@example.com", "berlin", []string{"techno", "house", "ambient"})
	// One overlapping genre.
	f.contact("Ben Klock", "ben@example.com", "Berlin", []string{"house"})
	// Both sides carry genres but no overlap → excluded.
	f.contact("Jazz Jane", "jane@example.com", "Berlin", []string{"jazz"})
	// No genres on the contact → city alone matches.
	f.contact("No Genre Nora", "nora@example.com", "Berlin", nil)
	// Wrong city → excluded.
	f.contact("Hamburg Harry", "harry@example.com", "Hamburg", []string{"techno"})
	// Suppressed → excluded.
	suppressed := f.contact("Stop Steve", "steve@example.com", "Berlin", []string{"techno"})
	if _, err := f.svc.SuppressTalentContact(f.ctx, suppressed.ID); err != nil {
		t.Fatal(err)
	}
	// Email-less contacts are matched with has_email=false (OFFER-10).
	nameOnly, err := f.svc.CreateTalentContact(f.ctx, bookings.TalentContactInput{Name: "Mute Mia", City: "Berlin", Genres: []string{"techno"}})
	if err != nil {
		t.Fatal(err)
	}

	matches, err := f.svc.MatchSlot(f.ctx, slot.ID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range matches {
		names = append(names, m.Contact.Name)
	}
	// Ranking: overlap count desc (Nadia 2; Mia and Ben 1 each), then most
	// recently added (Mia was created after Ben; Nora has no overlap).
	// Jane, Harry and Steve are excluded.
	want := []string{"Nadia Voss", "Mute Mia", "Ben Klock", "No Genre Nora"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("matches = %v, want %v", names, want)
	}
	if len(matches[0].OverlapGenres) != 2 || matches[0].OverlapGenres[0] != "techno" {
		t.Fatalf("overlap genres: %+v", matches[0].OverlapGenres)
	}
	if matches[1].Contact.HasEmail {
		t.Fatalf("email-less contact must match with has_email=false: %+v", matches[1].Contact)
	}
	if matches[1].Contact.ID != nameOnly.ID {
		t.Fatalf("expected the name-only contact second, got %+v", matches[1].Contact)
	}
}

func TestMatchSlot_EventWithoutGenresMatchesCityOnly(t *testing.T) {
	f := setup(t)
	// No genres on the event: every contact in the city matches, whatever
	// their genres (SPEC §8: overlap is required only when BOTH sides have
	// genres).
	ev := f.insertEvent("Klubnacht", "klubnacht", "Berlin", "venue", nil, nil, nil)
	slot := f.slot(ev, "Headliner", nil)
	f.contact("Jazz Jane", "jane@example.com", "Berlin", []string{"jazz"})
	f.contact("Techno Tom", "tom@example.com", "Berlin", []string{"techno"})
	f.contact("Hamburg Harry", "harry@example.com", "Hamburg", []string{"techno"})

	matches, err := f.svc.MatchSlot(f.ctx, slot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("city-only matching: got %d", len(matches))
	}
	for _, m := range matches {
		if len(m.OverlapGenres) != 0 {
			t.Fatalf("no event genres → no overlap to report: %+v", m.OverlapGenres)
		}
	}
}

// ─── SendOffer ───────────────────────────────────────────────────────────────

func TestSendOffer_HappyPath(t *testing.T) {
	f := setup(t)
	venue := f.insertVenue("Tresor.West", "Berlin")
	ev := f.insertEvent("Klubnacht", "klubnacht", "Berlin", "venue", nil, []string{"techno"}, &venue)
	fee := 600.0
	slot, err := f.svc.CreateOpenSlot(f.ctx, ev, bookings.OpenSlotInput{
		Role: "Headliner", Genres: []string{"techno"}, FeeMin: &fee, FeeMax: &fee, Currency: sptr("EUR"), Notes: "PA included",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", []string{"techno"})

	offer, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{
		TalentContactID: c.ID, OpenSlotID: &slot.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if offer.Status != "sent" || offer.EmailStatus != "sent" {
		t.Fatalf("status = %q/%q", offer.Status, offer.EmailStatus)
	}
	if offer.SentAt == nil || offer.EmailMessageID == nil {
		t.Fatalf("sent offer must carry sent_at + email_message_id: %+v", offer)
	}
	if offer.EventID == nil || *offer.EventID != ev || offer.OpenSlotID == nil || *offer.OpenSlotID != slot.ID {
		t.Fatalf("offer must link event + slot: %+v", offer)
	}
	if offer.ToEmail != "nadia@example.com" || !strings.Contains(offer.Subject, "Klubnacht") {
		t.Fatalf("offer copy: %+v", offer)
	}
	if !strings.Contains(offer.Body, "Nadia Voss") || !strings.Contains(offer.Body, "Tresor.West") ||
		!strings.Contains(offer.Body, "600 EUR") || !strings.Contains(offer.Body, "Headliner") {
		t.Fatalf("rendered body must carry dj/venue/fee/role:\n%s", offer.Body)
	}

	// The delivered message: kind, owner, sender, recipient, ReplyTo = the
	// sending member's email (OFFER-06 v2.1).
	msg := f.sender.last()
	if msg == nil {
		t.Fatal("no message delivered")
	}
	if msg.Kind != bookings.EmailKindBookingOffer || msg.OwnerType != bookings.OwnerTypeBookingOffer {
		t.Fatalf("kind/owner: %q/%q", msg.Kind, msg.OwnerType)
	}
	if msg.OwnerID == nil || *msg.OwnerID != offer.ID {
		t.Fatalf("owner id must be the request row: %+v", msg.OwnerID)
	}
	if msg.FromEmail != "offers@klubhub.example" || msg.ToEmail != "nadia@example.com" {
		t.Fatalf("from/to: %q → %q", msg.FromEmail, msg.ToEmail)
	}
	if msg.ReplyTo != "mara@nachtwerk.example" {
		t.Fatalf("ReplyTo must be the member's email, got %q", msg.ReplyTo)
	}
	if !strings.Contains(msg.Body, "mara@nachtwerk.example") {
		t.Fatalf("body must name the promoter's reply address:\n%s", msg.Body)
	}

	// The request row is sealed at rest and linked to the outbox row.
	var toEnc, subjectEnc, bodyEnc []byte
	var emailMessageID *uuid.UUID
	var status string
	err = testDB.Owner.QueryRow(context.Background(),
		`SELECT to_email_enc, subject_enc, body_enc, email_message_id, status FROM outreach_requests WHERE id = $1`, offer.ID).
		Scan(&toEnc, &subjectEnc, &bodyEnc, &emailMessageID, &status)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(toEnc, []byte("nadia@example.com")) || bytes.Contains(subjectEnc, []byte("Klubnacht")) ||
		bytes.Contains(bodyEnc, []byte("Nadia Voss")) {
		t.Fatal("plaintext PII found in the sealed request row")
	}
	if emailMessageID == nil || *emailMessageID != *offer.EmailMessageID || status != "sent" {
		t.Fatalf("request row link/status: %v %q", emailMessageID, status)
	}

	// The outbox row is sealed too (the EnvelopeCodec sealed it on write).
	var msgToEnc []byte
	if err := testDB.Owner.QueryRow(context.Background(),
		`SELECT to_email_enc FROM email_messages WHERE id = $1`, *emailMessageID).Scan(&msgToEnc); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(msgToEnc, []byte("nadia@example.com")) {
		t.Fatal("plaintext recipient found in the sealed outbox row")
	}

	// The sent log unseals and joins the live delivery status.
	got, err := f.svc.GetOffer(f.ctx, offer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ToEmail != "nadia@example.com" || got.Subject != offer.Subject || got.Body != offer.Body || got.EmailStatus != "sent" {
		t.Fatalf("log row: %+v", got)
	}
	list, err := f.svc.ListOffers(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != offer.ID {
		t.Fatalf("log list: %+v", list)
	}
}

func TestSendOffer_BuiltInDefaultTemplate(t *testing.T) {
	f := setup(t)
	// The org was created after the 00017 seed → no default template row →
	// the built-in default renders and template_id stays NULL.
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	offer, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if offer.TemplateID != nil {
		t.Fatalf("built-in default must not reference a template row: %v", offer.TemplateID)
	}
	if !strings.Contains(offer.Subject, "Booking offer:") || !strings.Contains(offer.Body, "stop") {
		t.Fatalf("built-in default must render with the opt-out footer: %q", offer.Subject)
	}
}

func TestSendOffer_TenantDefaultTemplate(t *testing.T) {
	f := setup(t)
	tpl, err := f.svc.CreateTemplate(f.ctx, bookings.TemplateCreate{
		Name: "Season offer", Subject: "Season offer: {{event_name}}", Body: "Hi {{dj_name}}, season greetings from {{org_name}}.", IsDefault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	ev := f.insertEvent("Klubnacht", "klubnacht", "Berlin", "venue", nil, nil, nil)
	slot := f.slot(ev, "Headliner", nil)

	offer, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID, OpenSlotID: &slot.ID})
	if err != nil {
		t.Fatal(err)
	}
	if offer.TemplateID == nil || *offer.TemplateID != tpl.ID {
		t.Fatalf("the tenant default must be used: %v", offer.TemplateID)
	}
	if !strings.Contains(offer.Subject, "Season offer: Klubnacht") {
		t.Fatalf("subject: %q", offer.Subject)
	}
}

func TestSendOffer_PerRequestOverrideKeepsTemplate(t *testing.T) {
	f := setup(t)
	tpl, err := f.svc.CreateTemplate(f.ctx, bookings.TemplateCreate{
		Name: "Base", Subject: "Base subject", Body: "Base body for {{dj_name}}", IsDefault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	subject, body := "One-off subject", "One-off body for Nadia"
	offer, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{
		TalentContactID: c.ID, TemplateID: &tpl.ID, Subject: &subject, Body: &body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if offer.Subject != subject || offer.Body != body {
		t.Fatalf("the override must be stored on the request: %+v", offer)
	}
	// The template itself is never mutated.
	tpls, err := f.svc.ListTemplates(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tpls) != 1 || tpls[0].Subject != "Base subject" || tpls[0].Body != "Base body for {{dj_name}}" || tpls[0].Version != 1 {
		t.Fatalf("template mutated by a per-request override: %+v", tpls)
	}
}

func TestSendOffer_TemplateVersionConflict(t *testing.T) {
	f := setup(t)
	tpl, err := f.svc.CreateTemplate(f.ctx, bookings.TemplateCreate{Name: "Base", Subject: "s", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	// Stale version → 409 conflict.
	if _, err := f.svc.UpdateTemplate(f.ctx, tpl.ID, bookings.TemplateUpdate{Name: &name, Version: 99}); !errors.Is(err, bookings.ErrConflict) {
		t.Fatalf("stale version must be ErrConflict, got %v", err)
	}
	// Correct version → applied, version incremented.
	upd, err := f.svc.UpdateTemplate(f.ctx, tpl.ID, bookings.TemplateUpdate{Name: &name, Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "Renamed" || upd.Version != 2 {
		t.Fatalf("update: %+v", upd)
	}
	// Duplicate name → conflict.
	if _, err := f.svc.CreateTemplate(f.ctx, bookings.TemplateCreate{Name: "renamed", Subject: "s", Body: "b"}); !errors.Is(err, bookings.ErrConflict) {
		t.Fatalf("duplicate template name must be ErrConflict, got %v", err)
	}
	// One default per tenant: setting a second default clears the first.
	t2, err := f.svc.CreateTemplate(f.ctx, bookings.TemplateCreate{Name: "Other", Subject: "s", Body: "b", IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	tpls, err := f.svc.ListTemplates(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defaults := 0
	for _, x := range tpls {
		if x.IsDefault {
			defaults++
			if x.ID != t2.ID {
				t.Fatalf("the previous default must be cleared: %+v", tpls)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("exactly one default, got %d", defaults)
	}
	// Soft delete.
	if err := f.svc.DeleteTemplate(f.ctx, tpl.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteTemplate(f.ctx, tpl.ID); !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("second delete must be ErrNotFound, got %v", err)
	}
}

func TestSendOffer_Suppressed(t *testing.T) {
	f := setup(t)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)

	// Opt-out by address (the sent log's one-click "Do not contact").
	if _, err := f.svc.AddSuppression(f.ctx, &bookings.SuppressionCreate{Email: "NADIA@example.com", Reason: "asked to stop"}); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID})
	if !errors.Is(err, bookings.ErrSuppressed) {
		t.Fatalf("want ErrSuppressed, got %v", err)
	}
	if f.sender.count() != 0 {
		t.Fatal("a suppressed address must never be sent to")
	}

	// Unsuppress → sends again.
	suppressions, err := f.svc.ListSuppressions(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(suppressions) != 1 || suppressions[0].Reason != "asked to stop" {
		t.Fatalf("suppressions: %+v", suppressions)
	}
	if err := f.svc.DeleteSuppression(f.ctx, suppressions[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID}); err != nil {
		t.Fatalf("after unsuppress the send must work: %v", err)
	}

	// The talent-side suppress endpoint refuses email-less contacts.
	nameOnly, err := f.svc.CreateTalentContact(f.ctx, bookings.TalentContactInput{Name: "No Email"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SuppressTalentContact(f.ctx, nameOnly.ID); !errors.Is(err, bookings.ErrNoContactEmail) {
		t.Fatalf("suppressing an email-less contact must be ErrNoContactEmail, got %v", err)
	}
}

func TestSendOffer_NoReplyTo(t *testing.T) {
	f := setup(t)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)

	// A device session has no "local:<uuid>" sub → no member email to reply to.
	device := f.principal()
	device.Sub = "device:door-iphone"
	if _, err := f.svc.SendOffer(f.ctx, device, &bookings.SendOfferInput{TalentContactID: c.ID}); !errors.Is(err, bookings.ErrNoReplyTo) {
		t.Fatalf("device session must be ErrNoReplyTo, got %v", err)
	}
	// A local sub that is not a member of the tenant → same refusal.
	ghost := f.principal()
	ghost.Sub = "local:" + uuid.NewString()
	if _, err := f.svc.SendOffer(f.ctx, ghost, &bookings.SendOfferInput{TalentContactID: c.ID}); !errors.Is(err, bookings.ErrNoReplyTo) {
		t.Fatalf("unknown member must be ErrNoReplyTo, got %v", err)
	}
	if f.sender.count() != 0 {
		t.Fatal("no message may be sent without a ReplyTo")
	}
}

func TestSendOffer_ForbiddenOrg(t *testing.T) {
	f := setup(t)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	// The principal's org is not the ctx tenant → refused before any read.
	other := f.principal()
	other.OrgID = uuid.NewString()
	if _, err := f.svc.SendOffer(f.ctx, other, &bookings.SendOfferInput{TalentContactID: c.ID}); !errors.Is(err, bookings.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestSendOffer_SlotFromOtherTenantIsForbidden(t *testing.T) {
	f := setup(t)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)

	// Another tenant's slot: RLS makes it invisible, and the service answers
	// ErrForbidden (not ErrNotFound) so its existence is not leaked.
	orgB := uuid.Must(uuid.NewV7())
	bg := context.Background()
	if err := tenantdb.WithTenant(bg, testDB.App, orgB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(bg, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Other Org', $2)`, orgB, "other-"+orgB.String()[24:]); err != nil {
			return err
		}
		evB := uuid.Must(uuid.NewV7())
		if _, err := tx.Exec(bg, `INSERT INTO events (id, tenant_id, title, slug, starts_at, ends_at, timezone, city)
			VALUES ($1, $2, 'Their Night', 'their-night', $3, $4, 'UTC', 'Berlin')`, evB, orgB, f.now.Add(72*time.Hour), f.now.Add(78*time.Hour)); err != nil {
			return err
		}
		_, err := tx.Exec(bg, `INSERT INTO event_open_slots (id, tenant_id, event_id, role) VALUES ($1, $2, $3, 'Headliner')`,
			uuid.Must(uuid.NewV7()), orgB, evB)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var slotB uuid.UUID
	if err := testDB.Owner.QueryRow(bg, `SELECT id FROM event_open_slots WHERE tenant_id = $1`, orgB).Scan(&slotB); err != nil {
		t.Fatal(err)
	}

	_, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID, OpenSlotID: &slotB})
	if !errors.Is(err, bookings.ErrForbidden) {
		t.Fatalf("cross-tenant slot must be ErrForbidden, got %v", err)
	}
}

func TestSendOffer_DailyCap(t *testing.T) {
	f := setup(t)
	// A tight cap: two per day.
	svc := bookings.NewService(f.db, f.keys, f.emailSvc, bookings.ServiceConfig{
		FromEmail: "offers@klubhub.example", FromName: "KlubHub Outreach", MaxPerDay: 2,
	}, func() time.Time { return f.now })
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	in := &bookings.SendOfferInput{TalentContactID: c.ID}

	// A failed attempt counts (the row exists before the send, SPEC §9 v2.1).
	f.sender.err = errors.New("smtp down")
	if _, err := svc.SendOffer(f.ctx, f.principal(), in); err != nil {
		t.Fatal(err)
	}
	f.sender.err = nil
	if _, err := svc.SendOffer(f.ctx, f.principal(), in); err != nil {
		t.Fatal(err)
	}
	// Third send of the day → rate limited.
	if _, err := svc.SendOffer(f.ctx, f.principal(), in); !errors.Is(err, bookings.ErrRateLimited) {
		t.Fatalf("want ErrRateLimited at the cap, got %v", err)
	}
	var count int
	if err := testDB.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM outreach_requests WHERE tenant_id = $1`, f.org).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("the failed attempt must count toward the cap: %d rows", count)
	}
}

func TestSendOffer_VenueWithheld(t *testing.T) {
	f := setup(t)
	venue := f.insertVenue("Tresor.West", "Berlin")
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	reveal := f.now.Add(24 * time.Hour)

	cases := []struct {
		name     string
		mode     string
		revealAt *time.Time
		venueID  *uuid.UUID
		want     string // "shown" | "hidden"
	}{
		{"venue mode shows the venue", "venue", nil, &venue, "shown"},
		{"secret before reveal hides the venue", "secret", &reveal, &venue, "hidden"},
		{"city_only hides the venue", "city_only", nil, &venue, "hidden"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := f.insertEvent("Night "+string(rune('A'+i)), "night-"+string(rune('a'+i)), "Berlin", tc.mode, tc.revealAt, nil, tc.venueID)
			slot := f.slot(ev, "Headliner", nil)
			if _, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID, OpenSlotID: &slot.ID}); err != nil {
				t.Fatal(err)
			}
			body := f.sender.last().Body
			if tc.want == "shown" && !strings.Contains(body, "Venue: Tresor.West") {
				t.Fatalf("venue must be disclosed:\n%s", body)
			}
			if tc.want == "hidden" {
				if strings.Contains(body, "Tresor") {
					t.Fatalf("a withheld venue must never be disclosed (OFFER-09):\n%s", body)
				}
				if !strings.Contains(body, "Venue: \n") {
					t.Fatalf("the venue line must render empty:\n%s", body)
				}
				if !strings.Contains(body, "City: Berlin") {
					t.Fatalf("the city must still render:\n%s", body)
				}
			}
		})
	}
}

func TestSendOffer_EmailUnavailable(t *testing.T) {
	f := setup(t)
	// Plunk unconfigured: reads work, sends are refused 503.
	svc := bookings.NewService(f.db, f.keys, nil, bookings.ServiceConfig{}, nil)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	if _, err := svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID}); !errors.Is(err, bookings.ErrEmailUnavailable) {
		t.Fatalf("want ErrEmailUnavailable, got %v", err)
	}
}

func TestSendOffer_DeliveryFailure(t *testing.T) {
	f := setup(t)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	f.sender.err = errors.New("plunk: 5xx")
	offer, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID})
	if err != nil {
		t.Fatalf("a delivery failure is a status, not an error: %v", err)
	}
	if offer.Status != "failed" || offer.EmailStatus != "failed" {
		t.Fatalf("status = %q/%q", offer.Status, offer.EmailStatus)
	}
	if !strings.Contains(offer.EmailError, "plunk: 5xx") {
		t.Fatalf("the delivery error must be surfaced: %q", offer.EmailError)
	}
	// The log row shows the failure, not a stuck "queued".
	got, err := f.svc.GetOffer(f.ctx, offer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.EmailStatus != "failed" || got.EmailError == "" {
		t.Fatalf("log row: %+v", got)
	}
}

func TestSentLogSurvivesTalentAndSlotDeletion(t *testing.T) {
	f := setup(t)
	ev := f.insertEvent("Klubnacht", "klubnacht", "Berlin", "venue", nil, nil, nil)
	slot := f.slot(ev, "Headliner", nil)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	offer, err := f.svc.SendOffer(f.ctx, f.principal(), &bookings.SendOfferInput{TalentContactID: c.ID, OpenSlotID: &slot.ID})
	if err != nil {
		t.Fatal(err)
	}

	// ON DELETE SET NULL: the log keeps its copy, the links go NULL.
	if err := f.svc.DeleteTalentContact(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteOpenSlot(f.ctx, slot.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetOffer(f.ctx, offer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TalentContactID != nil || got.OpenSlotID != nil {
		t.Fatalf("links must be SET NULL: %+v", got)
	}
	if got.ToEmail != "nadia@example.com" || got.Status != "sent" {
		t.Fatalf("the sealed copy survives: %+v", got)
	}
}

// ─── Suppressions (SPEC §9) ──────────────────────────────────────────────────

func TestSuppressions_StoreOnlyBlindIndex(t *testing.T) {
	f := setup(t)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	sup, err := f.svc.AddSuppression(f.ctx, &bookings.SuppressionCreate{Email: "nadia@example.com", Reason: "stop"})
	if err != nil {
		t.Fatal(err)
	}

	// The suppression's blind index is the CONTACT's stored index (same
	// scope) — that is what makes the SQL match work.
	var supBidx, contactBidx []byte
	if err := testDB.Owner.QueryRow(context.Background(),
		`SELECT email_bidx FROM outreach_suppressions WHERE id = $1`, sup.ID).Scan(&supBidx); err != nil {
		t.Fatal(err)
	}
	if err := testDB.Owner.QueryRow(context.Background(),
		`SELECT email_bidx FROM talent_contacts WHERE id = $1`, c.ID).Scan(&contactBidx); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(supBidx, contactBidx) {
		t.Fatal("suppression must reuse the contact email blind-index scope")
	}

	// The email itself is never stored: the table has no column for it and
	// the reason is free text the operator wrote.
	var cols []string
	rows, err := testDB.Owner.Query(context.Background(),
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'outreach_suppressions' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var col string
		_ = rows.Scan(&col)
		cols = append(cols, col)
	}
	rows.Close()
	for _, col := range cols {
		if strings.Contains(col, "email") && col != "email_bidx" {
			t.Fatalf("suppressions must not store the email, found column %q (all: %v)", col, cols)
		}
	}

	// A duplicate suppression is a conflict.
	if _, err := f.svc.AddSuppression(f.ctx, &bookings.SuppressionCreate{Email: "NADIA@example.com"}); !errors.Is(err, bookings.ErrDuplicate) {
		t.Fatalf("duplicate suppression must be ErrDuplicate, got %v", err)
	}
}

// ─── EnvelopeCodec ───────────────────────────────────────────────────────────

func TestEnvelopeCodec_RoundTrip(t *testing.T) {
	f := setup(t)
	codec := bookings.EnvelopeCodec{Keys: f.keys}
	rowID := uuid.New()
	ref := email.FieldRef{Table: "email_messages", Column: "to_email", RowID: rowID}

	err := f.db.WithTenant(f.ctx, f.org, func(tx pgx.Tx) error {
		enc, bidx, err := codec.SealEmail(f.ctx, tx, ref, "DJ@Example.com")
		if err != nil {
			return err
		}
		if bytes.Contains(enc, []byte("DJ@Example.com")) {
			t.Fatal("SealEmail must encrypt")
		}
		plain, err := codec.Open(f.ctx, tx, ref, enc)
		if err != nil {
			return err
		}
		if plain != "DJ@Example.com" {
			t.Fatalf("round trip = %q", plain)
		}
		// The blind index uses the physical <col>_bidx scope on the
		// normalised address.
		dek, err := f.keys.Current(f.ctx, tx, f.org)
		if err != nil {
			return err
		}
		want := dek.BlindIndex("email_messages", "to_email_bidx", "dj@example.com")
		if !bytes.Equal(bidx, want) {
			t.Fatal("blind index scope mismatch")
		}
		// Seal/Open for a plain field.
		subjRef := email.FieldRef{Table: "email_messages", Column: "subject", RowID: rowID}
		subjEnc, err := codec.Seal(f.ctx, tx, subjRef, "Hello")
		if err != nil {
			return err
		}
		subj, err := codec.Open(f.ctx, tx, subjRef, subjEnc)
		if err != nil {
			return err
		}
		if subj != "Hello" {
			t.Fatalf("subject round trip = %q", subj)
		}
		// A different row id must not decrypt (AAD binds the row).
		other := email.FieldRef{Table: "email_messages", Column: "subject", RowID: uuid.New()}
		if _, err := codec.Open(f.ctx, tx, other, subjEnc); err == nil {
			t.Fatal("opening with a different RowID must fail")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnvelopeCodec_Guards(t *testing.T) {
	// No keyring → ErrNoKeyring (before any DB access).
	codec := bookings.EnvelopeCodec{}
	ref := email.FieldRef{Table: "email_messages", Column: "subject", RowID: uuid.New()}
	if _, err := codec.Seal(context.Background(), nil, ref, "x"); !errors.Is(err, bookings.ErrNoKeyring) {
		t.Fatalf("want ErrNoKeyring, got %v", err)
	}
	if _, err := codec.Open(context.Background(), nil, ref, []byte("x")); !errors.Is(err, bookings.ErrNoKeyring) {
		t.Fatalf("want ErrNoKeyring, got %v", err)
	}
	// Keyring but no tenant in ctx → ErrNoTenant.
	f := setup(t)
	codec = bookings.EnvelopeCodec{Keys: f.keys}
	if _, err := codec.Seal(context.Background(), nil, ref, "x"); !errors.Is(err, tenantdb.ErrNoTenant) {
		t.Fatalf("want ErrNoTenant, got %v", err)
	}
}

// ─── RLS isolation ───────────────────────────────────────────────────────────

func TestRLS_Isolation(t *testing.T) {
	f := setup(t)
	c := f.contact("Nadia Voss", "nadia@example.com", "Berlin", nil)
	ev := f.insertEvent("Klubnacht", "klubnacht", "Berlin", "venue", nil, nil, nil)
	slot := f.slot(ev, "Headliner", nil)

	// A second tenant sees nothing of the first.
	orgB := uuid.Must(uuid.NewV7())
	bg := context.Background()
	if err := tenantdb.WithTenant(bg, testDB.App, orgB, func(tx pgx.Tx) error {
		_, err := tx.Exec(bg, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Other Org', $2)`, orgB, "other-"+orgB.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctxB := tenantdb.ContextWithTenant(bg, orgB)
	keysB := f.keys // same KEK; the DEK is per tenant
	svcB := bookings.NewService(f.db, keysB, f.emailSvc, bookings.ServiceConfig{
		FromEmail: "offers@klubhub.example", FromName: "Outreach", MaxPerDay: 50,
	}, func() time.Time { return f.now })

	list, err := svcB.ListTalentContacts(ctxB, bookings.TalentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("tenant B must not see tenant A's contacts: %+v", list)
	}
	if _, err := svcB.GetTalentContact(ctxB, c.ID); !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("cross-tenant get must be ErrNotFound, got %v", err)
	}
	if _, err := svcB.MatchSlot(ctxB, slot.ID); !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("cross-tenant slot must be ErrNotFound, got %v", err)
	}
	pB := authz.Principal{Sub: "local:" + uuid.NewString(), OrgID: orgB.String(), Roles: []string{"owner"}, AMR: []string{"pwd", "otp"}, AuthTime: time.Now()}
	if _, err := svcB.SendOffer(ctxB, pB, &bookings.SendOfferInput{TalentContactID: c.ID}); !errors.Is(err, bookings.ErrNotFound) {
		t.Fatalf("cross-tenant send must be ErrNotFound, got %v", err)
	}
	offers, err := svcB.ListOffers(ctxB)
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 0 {
		t.Fatalf("tenant B must not see tenant A's sent log: %+v", offers)
	}
}

func sptr(v string) *string { return &v }
