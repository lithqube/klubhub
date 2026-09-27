package audience_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/audience"
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

type fixture struct {
	svc *audience.Service
	ctx context.Context
	now time.Time
	org uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	org := uuid.Must(uuid.NewV7())
	if err := tenantdb.WithTenant(context.Background(), testDB.App, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Nachtwerk', $2)`, org, "nw-"+org.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	keys := envelope.NewKeyring(kek)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	db := tenantdb.New(testDB.App)
	return fixture{
		svc: audience.NewService(db, keys, func() time.Time { return now }),
		ctx: tenantdb.ContextWithTenant(context.Background(), org),
		now: now, org: org,
	}
}

func consent(basis string) audience.ConsentInput {
	return audience.ConsentInput{Basis: basis, RecordedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), FormText: "Sign up for our newsletter"}
}

func TestCreateContact_RoundTrips(t *testing.T) {
	f := setup(t)
	c, err := f.svc.CreateContact(f.ctx, audience.ContactInput{
		Name: "Nadia Voss", Email: "nadia@example.com", Source: audience.SourceFollow, Consent: consent(audience.BasisConsent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Nadia Voss" || c.Email != "nadia@example.com" || c.Status != audience.StatusActive {
		t.Fatalf("got %+v", c)
	}

	page, err := f.svc.ListContacts(f.ctx, audience.ContactFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Contacts) != 1 || page.Contacts[0].Name != "Nadia Voss" {
		t.Fatalf("expected the contact back decrypted, got %+v", page.Contacts)
	}
	if page.Counts.All != 1 || page.Counts.Active != 1 {
		t.Fatalf("expected counts to reflect the new active contact, got %+v", page.Counts)
	}
}

func TestCreateContact_RequiresNameOrEmail(t *testing.T) {
	f := setup(t)
	_, err := f.svc.CreateContact(f.ctx, audience.ContactInput{Source: audience.SourceCSV, Consent: consent(audience.BasisSoftOptIn)})
	var inv *audience.InvalidError
	if !errors.As(err, &inv) || inv.Field != "name" {
		t.Fatalf("want an InvalidError on name, got %v", err)
	}
}

func TestCreateContact_RejectsUnknownSource(t *testing.T) {
	f := setup(t)
	_, err := f.svc.CreateContact(f.ctx, audience.ContactInput{Name: "X", Source: "webhook", Consent: consent(audience.BasisConsent)})
	var inv *audience.InvalidError
	if !errors.As(err, &inv) || inv.Field != "source" {
		t.Fatalf("want an InvalidError on source, got %v", err)
	}
}

func TestCreateContact_DuplicateEmailIsRejected(t *testing.T) {
	f := setup(t)
	in := audience.ContactInput{Name: "Lars", Email: "lars@example.com", Source: audience.SourceRSVP, Consent: consent(audience.BasisConsent)}
	if _, err := f.svc.CreateContact(f.ctx, in); err != nil {
		t.Fatal(err)
	}
	in.Name = "Lars Again"
	if _, err := f.svc.CreateContact(f.ctx, in); !errors.Is(err, audience.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestSetStatus_Unsubscribe(t *testing.T) {
	f := setup(t)
	c, err := f.svc.CreateContact(f.ctx, audience.ContactInput{
		Name: "Priya", Email: "priya@example.com", Source: audience.SourceRSVP, Consent: consent(audience.BasisConsent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SetStatus(f.ctx, c.ID, audience.StatusInput{Status: audience.StatusUnsubscribed}); err != nil {
		t.Fatal(err)
	}
	page, err := f.svc.ListContacts(f.ctx, audience.ContactFilter{Status: audience.StatusUnsubscribed})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Contacts) != 1 || page.Contacts[0].Status != audience.StatusUnsubscribed {
		t.Fatalf("expected the contact to show unsubscribed, got %+v", page.Contacts)
	}
	if page.Counts.Unsubscribed != 1 || page.Counts.Active != 0 {
		t.Fatalf("expected counts to move from active to unsubscribed, got %+v", page.Counts)
	}
}

func TestSetStatus_UnknownContact(t *testing.T) {
	f := setup(t)
	err := f.svc.SetStatus(f.ctx, uuid.New(), audience.StatusInput{Status: audience.StatusBounced})
	if !errors.Is(err, audience.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteContact_RemovesIt(t *testing.T) {
	f := setup(t)
	c, err := f.svc.CreateContact(f.ctx, audience.ContactInput{
		Name: "Tomas", Source: audience.SourceDoor, Consent: consent(audience.BasisSoftOptIn),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteContact(f.ctx, c.ID, "local:staff"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteContact(f.ctx, c.ID, "local:staff"); !errors.Is(err, audience.ErrNotFound) {
		t.Fatalf("second delete should be not-found, got %v", err)
	}
}

func TestImportCSV_SkipsDuplicatesAndInvalidRows(t *testing.T) {
	f := setup(t)
	// One valid new row, one duplicate of an existing contact, one
	// invalid row (no name and no email).
	if _, err := f.svc.CreateContact(f.ctx, audience.ContactInput{
		Name: "Existing", Email: "existing@example.com", Source: audience.SourceRSVP, Consent: consent(audience.BasisConsent),
	}); err != nil {
		t.Fatal(err)
	}
	rows := []audience.ImportRow{
		{Name: "Fresh Contact", Email: "fresh@example.com"},
		{Name: "Existing Again", Email: "existing@example.com"},
		{Phone: "not a valid identity on its own without name or email"},
	}
	res, err := f.svc.ImportCSV(f.ctx, rows, consent(audience.BasisSoftOptIn))
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Duplicates != 1 || res.Invalid != 1 {
		t.Fatalf("got %+v", res)
	}
	page, err := f.svc.ListContacts(f.ctx, audience.ContactFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Counts.All != 2 {
		t.Fatalf("expected 2 contacts total (1 pre-existing + 1 imported), got %d", page.Counts.All)
	}
}

func TestImportCSV_RejectsBatchTooLarge(t *testing.T) {
	f := setup(t)
	rows := make([]audience.ImportRow, audience.MaxBatch+1)
	for i := range rows {
		rows[i] = audience.ImportRow{Email: "x@example.com"}
	}
	_, err := f.svc.ImportCSV(f.ctx, rows, consent(audience.BasisSoftOptIn))
	var inv *audience.InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("want an InvalidError over the batch, got %v", err)
	}
}

func TestSegments_MatchingCountReflectsRealContacts(t *testing.T) {
	f := setup(t)
	for i, source := range []string{audience.SourceRSVP, audience.SourceRSVP, audience.SourceFollow} {
		if _, err := f.svc.CreateContact(f.ctx, audience.ContactInput{
			Name: "Guest" + string(rune('A'+i)), Source: source, Consent: consent(audience.BasisConsent),
		}); err != nil {
			t.Fatal(err)
		}
	}
	seg, err := f.svc.CreateSegment(f.ctx, audience.SegmentInput{
		Name: "RSVP'd", Filter: audience.SegmentFilter{Source: audience.SourceRSVP},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seg.Matching != 2 {
		t.Fatalf("expected 2 RSVP contacts to match, got %d", seg.Matching)
	}

	segs, err := f.svc.ListSegments(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 || segs[0].Matching != 2 {
		t.Fatalf("got %+v", segs)
	}
}

func TestSegments_DuplicateNameRejected(t *testing.T) {
	f := setup(t)
	in := audience.SegmentInput{Name: "VIPs", Filter: audience.SegmentFilter{Status: audience.StatusActive}}
	if _, err := f.svc.CreateSegment(f.ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateSegment(f.ctx, in); !errors.Is(err, audience.ErrSegmentExists) {
		t.Fatalf("want ErrSegmentExists, got %v", err)
	}
}

func TestSegments_SinceDaysFilter(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.CreateContact(f.ctx, audience.ContactInput{
		Name: "Recent", Source: audience.SourceDoor, Consent: consent(audience.BasisSoftOptIn),
	}); err != nil {
		t.Fatal(err)
	}
	within := 30
	seg, err := f.svc.CreateSegment(f.ctx, audience.SegmentInput{Name: "New", Filter: audience.SegmentFilter{SinceDays: &within}})
	if err != nil {
		t.Fatal(err)
	}
	if seg.Matching != 1 {
		t.Fatalf("expected the just-created contact to match a 30-day window, got %d", seg.Matching)
	}
}

func TestWriteCSV_NeutralisesFormulaInjection(t *testing.T) {
	var buf bytes.Buffer
	if err := audience.WriteCSV(&buf, []audience.Contact{{Name: "=cmd|' /C calc'!A1", Email: "a@b.com", Status: "active", Source: "csv"}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("'=cmd")) {
		t.Fatalf("expected the leading = to be neutralised, got %q", out)
	}
}

func TestSegmentFilter_RejectsUnknownStatusAndSource(t *testing.T) {
	f := setup(t)
	_, err := f.svc.CreateSegment(f.ctx, audience.SegmentInput{Name: "Bad", Filter: audience.SegmentFilter{Status: "vip"}})
	var inv *audience.InvalidError
	if !errors.As(err, &inv) || inv.Field != "filter.status" {
		t.Fatalf("want an InvalidError on filter.status, got %v", err)
	}
}
