package marketing_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/audience"
	"github.com/klubhub/dj/api/internal/promoter/marketing"
)

type campFixture struct {
	org     uuid.UUID
	ctx     context.Context
	keys    *envelope.Keyring
	aud     *audience.Service
	svc     *marketing.CampaignService
	segment uuid.UUID
}

func newCampFixture(t *testing.T, permitted bool) campFixture {
	t.Helper()
	if testDB == nil {
		t.Skip("integration: -short")
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	keys := envelope.NewKeyring(kek)
	db := tenantdb.New(testDB.App)
	org := newOrg(t)
	f := campFixture{
		org: org, keys: keys, ctx: tenantdb.ContextWithTenant(context.Background(), org),
		aud: audience.NewService(db, keys, time.Now),
		svc: marketing.NewCampaignService(db, keys, marketing.NewGate(permitted), nil),
	}
	seg, err := f.aud.CreateSegment(f.ctx, audience.SegmentInput{Name: "everyone"})
	if err != nil {
		t.Fatal(err)
	}
	f.segment = seg.ID
	return f
}

func (f campFixture) contact(t *testing.T, email, basis, status string) {
	t.Helper()
	_, err := f.aud.CreateContact(f.ctx, audience.ContactInput{
		Name: "N", Email: email, Source: audience.SourceFollow, Status: status,
		Consent: audience.ConsentInput{Basis: basis, RecordedAt: time.Now(), FormText: "Sign up"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// queuedTo reports whether a marketing_sends row exists for the address.
func (f campFixture) queuedTo(t *testing.T, email string) bool {
	t.Helper()
	var got bool
	if err := tenantdb.WithTenant(f.ctx, testDB.App, f.org, func(tx pgx.Tx) error {
		dek, err := f.keys.Current(f.ctx, tx, f.org)
		if err != nil {
			return err
		}
		return tx.QueryRow(f.ctx, `SELECT EXISTS (SELECT 1 FROM marketing_sends WHERE to_email_bidx = $1)`,
			marketing.EmailIndex(dek, email)).Scan(&got)
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestSend_EnqueuesOnlyConsentedNonSuppressed(t *testing.T) {
	f := newCampFixture(t, true)
	f.contact(t, "yes@example.com", audience.BasisConsent, "")
	f.contact(t, "also@example.com", audience.BasisConsent, "")
	f.contact(t, "soft@example.com", audience.BasisSoftOptIn, "")
	f.contact(t, "unsub@example.com", audience.BasisConsent, audience.StatusUnsubscribed)
	f.contact(t, "suppressed@example.com", audience.BasisConsent, "")
	if err := tenantdb.WithTenant(f.ctx, testDB.App, f.org, func(tx pgx.Tx) error {
		dek, err := f.keys.Current(f.ctx, tx, f.org)
		if err != nil {
			return err
		}
		_, err = tx.Exec(f.ctx, `INSERT INTO marketing_suppressions (id, tenant_id, email_bidx, reason) VALUES ($1, $2, $3, 'manual')`,
			uuid.New(), f.org, marketing.EmailIndex(dek, "suppressed@example.com"))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	id, err := f.svc.Create(f.ctx, marketing.CampaignInput{SegmentID: f.segment, Subject: "Fri", Body: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	n, err := f.svc.Send(f.ctx, id)
	if err != nil || n != 2 {
		t.Fatalf("queued %d, err %v; want 2", n, err)
	}
	for email, want := range map[string]bool{
		"yes@example.com": true, "also@example.com": true,
		"soft@example.com": false, "unsub@example.com": false, "suppressed@example.com": false,
	} {
		if got := f.queuedTo(t, email); got != want {
			t.Errorf("%s queued=%v, want %v", email, got, want)
		}
	}
	if _, err := f.svc.Send(f.ctx, id); !errors.Is(err, marketing.ErrAlreadySent) {
		t.Fatalf("second send: %v, want ErrAlreadySent", err)
	}
	if got := count(t, f.org, "marketing_sends"); got != 2 {
		t.Fatalf("%d rows after a repeated send, want 2", got)
	}
}

func TestSend_WithoutAttestationQueuesNothing(t *testing.T) {
	f := newCampFixture(t, false)
	f.contact(t, "yes@example.com", audience.BasisConsent, "")
	id, err := f.svc.Create(f.ctx, marketing.CampaignInput{SegmentID: f.segment, Subject: "Fri", Body: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Send(f.ctx, id); !errors.Is(err, marketing.ErrNotPermitted) {
		t.Fatalf("got %v, want ErrNotPermitted", err)
	}
	if got := count(t, f.org, "marketing_sends"); got != 0 {
		t.Fatalf("%d rows queued without attestation", got)
	}
}
