package marketing_test

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/audience"
	"github.com/klubhub/dj/api/internal/promoter/marketing"
)

// sigProvider accepts a request only when X-Sig is "ok" and reports the
// event named in the body ("bounce <email> <msgid>").
type sigProvider struct{}

func (sigProvider) SendMarketing(context.Context, *marketing.Message) (string, error) {
	return "", nil
}

func (sigProvider) HandleWebhook(r *http.Request) ([]marketing.WebhookEvent, error) {
	if r.Header.Get("X-Sig") != "ok" {
		return nil, errors.New("bad signature")
	}
	buf := make([]byte, 256)
	n, _ := r.Body.Read(buf)
	f := strings.Fields(string(buf[:n]))
	return []marketing.WebhookEvent{{Type: marketing.EventType(f[0]), Email: f[1], ProviderMessageID: f[2]}}, nil
}

type whFixture struct {
	org  uuid.UUID
	keys *envelope.Keyring
	aud  *audience.Service
	ctx  context.Context
	srv  http.Handler
}

func newWebhookFixture(t *testing.T) whFixture {
	t.Helper()
	if testDB == nil {
		t.Skip("integration: -short")
	}
	org := newOrg(t)
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	keys := envelope.NewKeyring(kek)
	db := tenantdb.New(testDB.App)
	resolve := func(_ context.Context, tenant uuid.UUID) (marketing.MarketingProvider, error) {
		if tenant != org {
			return nil, marketing.ErrNoProvider
		}
		return sigProvider{}, nil
	}
	h := marketing.NewHandler(marketing.Checker{}).WithWebhooks(marketing.NewWebhookService(db, keys, resolve, time.Now))
	r := chi.NewRouter()
	h.Mount(r, (*authz.Engine)(nil), authz.NewRegistry(), nil)
	return whFixture{
		org: org, keys: keys, aud: audience.NewService(db, keys, time.Now),
		ctx: tenantdb.ContextWithTenant(context.Background(), org), srv: r,
	}
}

func (f whFixture) post(tenant uuid.UUID, sig, body string) int {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/marketing/webhooks/"+tenant.String(), strings.NewReader(body))
	req.Header.Set("X-Sig", sig)
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, req)
	return w.Code
}

func (f whFixture) suppressed(t *testing.T, email string) bool {
	t.Helper()
	var got bool
	if err := tenantdb.WithTenant(f.ctx, testDB.App, f.org, func(tx pgx.Tx) error {
		dek, err := f.keys.Current(f.ctx, tx, f.org)
		if err != nil {
			return err
		}
		got, err = marketing.Suppressed(f.ctx, tx, f.org, dek, email)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func (f whFixture) addContact(t *testing.T, email string) {
	t.Helper()
	_, err := f.aud.CreateContact(f.ctx, audience.ContactInput{
		Name: "Nadia", Email: email, Source: audience.SourceFollow,
		Consent: audience.ConsentInput{Basis: audience.BasisConsent, RecordedAt: time.Now(), FormText: "Sign up"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebhook_BounceSuppressesAndKeepsContactListed(t *testing.T) {
	f := newWebhookFixture(t)
	f.addContact(t, "Nadia@Example.com")
	if f.suppressed(t, "nadia@example.com") {
		t.Fatal("suppressed before any event")
	}
	if code := f.post(f.org, "ok", "bounce nadia@example.com msg-1"); code != http.StatusNoContent {
		t.Fatalf("status %d", code)
	}
	// Replayed delivery is idempotent.
	if code := f.post(f.org, "ok", "bounce nadia@example.com msg-1"); code != http.StatusNoContent {
		t.Fatalf("replay status %d", code)
	}
	if !f.suppressed(t, "nadia@example.com") {
		t.Fatal("not suppressed after bounce")
	}
	page, err := f.aud.ListContacts(f.ctx, audience.ContactFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Contacts) != 1 || page.Contacts[0].Status != "bounced" {
		t.Fatalf("contact must stay listed as bounced, got %+v", page.Contacts)
	}
}

func TestWebhook_BadSignatureChangesNothing(t *testing.T) {
	f := newWebhookFixture(t)
	f.addContact(t, "nadia@example.com")
	if code := f.post(f.org, "forged", "complaint nadia@example.com msg-1"); code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", code)
	}
	if f.suppressed(t, "nadia@example.com") {
		t.Fatal("forged webhook suppressed the address")
	}
	page, _ := f.aud.ListContacts(f.ctx, audience.ContactFilter{})
	if page.Contacts[0].Status != audience.StatusActive {
		t.Fatalf("status changed to %s", page.Contacts[0].Status)
	}
}

func TestWebhook_UnknownTenantIs404(t *testing.T) {
	f := newWebhookFixture(t)
	if code := f.post(uuid.New(), "ok", "bounce a@example.com m"); code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", code)
	}
}

func TestWebhook_EventTypesMapToContactStatus(t *testing.T) {
	for _, tc := range []struct{ event, want string }{
		{"complaint", "complained"}, {"unsubscribe", "unsubscribed"},
	} {
		t.Run(tc.event, func(t *testing.T) {
			f := newWebhookFixture(t)
			f.addContact(t, "nadia@example.com")
			if code := f.post(f.org, "ok", tc.event+" nadia@example.com m-1"); code != http.StatusNoContent {
				t.Fatalf("status %d", code)
			}
			if !f.suppressed(t, "nadia@example.com") {
				t.Fatal("not suppressed")
			}
			page, _ := f.aud.ListContacts(f.ctx, audience.ContactFilter{})
			if page.Contacts[0].Status != tc.want {
				t.Fatalf("status %s, want %s", page.Contacts[0].Status, tc.want)
			}
		})
	}
}

// An earlier unsubscribe must not be rewritten by a later bounce, and an
// address with no contact row is still suppressed (fail closed).
func TestWebhook_LaterEventKeepsFirstStatusAndUnknownAddressSuppressed(t *testing.T) {
	f := newWebhookFixture(t)
	f.addContact(t, "nadia@example.com")
	f.post(f.org, "ok", "unsubscribe nadia@example.com m-1")
	f.post(f.org, "ok", "bounce nadia@example.com m-2")
	page, _ := f.aud.ListContacts(f.ctx, audience.ContactFilter{})
	if page.Contacts[0].Status != "unsubscribed" {
		t.Fatalf("status %s, want unsubscribed", page.Contacts[0].Status)
	}
	f.post(f.org, "ok", "bounce stranger@example.com m-3")
	if !f.suppressed(t, "stranger@example.com") {
		t.Fatal("address without a contact must still be suppressed")
	}
}
