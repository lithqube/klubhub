package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func plunkFor(t *testing.T, h http.HandlerFunc) *Plunk {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	settings, _ := json.Marshal(PlunkSettings{BaseURL: srv.URL, APIKey: "sk_test", WebhookSecret: "whsec"})
	p, err := NewPlunk(settings)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func campaignMsg() *Message {
	return &Message{
		Kind: KindMarketing, SendID: uuid.New(), To: "fan@example.com",
		FromEmail: "news@club.example", FromName: "Club", ReplyTo: "hi@club.example",
		Subject: "Friday", Text: "plain", HTML: "<p>Friday</p>",
		UnsubscribeURL: "https://club.example/u/abc",
	}
}

func TestPlunkSendsCampaignMessage(t *testing.T) {
	msg := campaignMsg()
	var got map[string]any
	p := plunkFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/send" || r.Header.Get("Authorization") != "Bearer sk_test" ||
			r.Header.Get("Idempotency-Key") != msg.SendID.String() {
			t.Errorf("request %s auth=%q key=%q", r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"data":{"emails":[{"contact":{"email":"fan@example.com"},"email":"em_1","status":"queued"}],"timestamp":"2026-11-26T10:00:00Z"}}`))
	})
	id, err := p.SendMarketing(context.Background(), msg)
	if err != nil || id != "em_1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	h, _ := got["headers"].(map[string]any)
	if got["to"] != "fan@example.com" || got["subject"] != "Friday" || got["body"] != "<p>Friday</p>" || got["reply"] != "hi@club.example" ||
		h["List-Unsubscribe"] != "<https://club.example/u/abc>" || h["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Fatalf("payload = %v", got)
	}
}

func TestPlunkRejectsNonMarketingKinds(t *testing.T) {
	called := false
	p := plunkFor(t, func(http.ResponseWriter, *http.Request) { called = true })
	for _, k := range []Kind{"", "transactional", "invoice_sent"} {
		msg := campaignMsg()
		msg.Kind = k
		if _, err := p.SendMarketing(context.Background(), msg); !errors.Is(err, ErrNotMarketing) {
			t.Errorf("kind %q: err = %v", k, err)
		}
	}
	if called {
		t.Fatal("non-marketing mail reached Plunk")
	}
}

func TestPlunkSendFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		wantID string
		ok     bool
	}{
		"server error":          {500, `{}`, "", false},
		"throttled":             {200, `{"success":true,"data":{"emails":[{"contact":{"email":"fan@example.com"},"email":null,"status":"throttled"}]}}`, "", false},
		"wrong contact":         {200, `{"success":true,"data":{"emails":[{"contact":{"email":"x@example.com"},"email":"em_1","status":"queued"}]}}`, "", false},
		"retried after success": {409, `{"success":false,"error":{"code":"IDEMPOTENCY_KEY_REUSED","details":{"originalStatusCode":200}}}`, "", true},
		"conflict in flight":    {409, `{"success":false,"error":{"code":"IDEMPOTENCY_KEY_REUSED","details":{"originalStatusCode":null}}}`, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			p := plunkFor(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			id, err := p.SendMarketing(context.Background(), campaignMsg())
			if (err == nil) != tc.ok || id != tc.wantID {
				t.Fatalf("id=%q err=%v", id, err)
			}
		})
	}
}

func TestNewPlunkNeedsKeyAndSecret(t *testing.T) {
	for _, s := range []string{`{}`, `{"api_key":"k"}`, `{"webhook_secret":"w"}`, `{"api_key":"k","webhook_secret":"w","base_url":"ftp://x"}`, `not json`} {
		if _, err := NewPlunk([]byte(s)); err == nil {
			t.Errorf("%s: accepted", s)
		}
	}
}

func webhookReq(auth, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/marketing/webhooks/x", strings.NewReader(body))
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	return r
}

func TestPlunkWebhookRejectsBadSecret(t *testing.T) {
	p := plunkFor(t, nil)
	body := `{"contact":{"email":"fan@example.com"},"event":{"emailId":"em_1","bounceType":"Permanent","bouncedAt":"2026-11-26T10:00:00Z"}}`
	for _, auth := range []string{"", "Bearer wrong", "whsec"} {
		if evs, err := p.HandleWebhook(webhookReq(auth, body)); err == nil || evs != nil {
			t.Errorf("auth %q: evs=%v err=%v", auth, evs, err)
		}
	}
}

func TestPlunkWebhookParsesEvents(t *testing.T) {
	p := plunkFor(t, nil)
	for name, tc := range map[string]struct {
		body string
		want EventType // "" = ignored
	}{
		"permanent bounce": {`{"contact":{"email":"fan@example.com","subscribed":true},"event":{"emailId":"em_1","bounceType":"Permanent","bouncedAt":"2026-11-26T10:00:00Z"}}`, EventBounce},
		"transient bounce": {`{"contact":{"email":"fan@example.com","subscribed":true},"event":{"emailId":"em_1","bounceType":"Transient","bouncedAt":"2026-11-26T10:00:00Z"}}`, ""},
		"complaint":        {`{"contact":{"email":"fan@example.com","subscribed":true},"event":{"emailId":"em_1","complainedAt":"2026-11-26T10:00:00Z"}}`, EventComplaint},
		"unsubscribe":      {`{"contact":{"email":"fan@example.com","subscribed":false},"event":{}}`, EventUnsubscribe},
		"delivery":         {`{"contact":{"email":"fan@example.com","subscribed":true},"event":{"emailId":"em_1","deliveredAt":"2026-11-26T10:00:00Z"}}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			evs, err := p.HandleWebhook(webhookReq("Bearer whsec", tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(evs) != 0 {
					t.Fatalf("evs = %v", evs)
				}
				return
			}
			if len(evs) != 1 || evs[0].Type != tc.want || evs[0].Email != "fan@example.com" {
				t.Fatalf("evs = %v", evs)
			}
			if tc.want != EventUnsubscribe && (evs[0].ProviderMessageID != "em_1" || evs[0].At.IsZero()) {
				t.Fatalf("evs = %v", evs)
			}
		})
	}
}

// TestPlunkSandboxSend sends one real campaign mail when a sandbox key is set.
func TestPlunkSandboxSend(t *testing.T) {
	key, to, from := os.Getenv("PLUNK_SANDBOX_API_KEY"), os.Getenv("PLUNK_SANDBOX_TO"), os.Getenv("PLUNK_SANDBOX_FROM")
	if key == "" || to == "" || from == "" {
		t.Skip("PLUNK_SANDBOX_API_KEY, PLUNK_SANDBOX_TO and PLUNK_SANDBOX_FROM not set")
	}
	settings, _ := json.Marshal(PlunkSettings{BaseURL: os.Getenv("PLUNK_SANDBOX_BASE_URL"), APIKey: key, WebhookSecret: "unused"})
	p, err := NewPlunk(settings)
	if err != nil {
		t.Fatal(err)
	}
	msg := campaignMsg()
	msg.To, msg.FromEmail, msg.ReplyTo = to, from, ""
	if id, err := p.SendMarketing(context.Background(), msg); err != nil || id == "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

// TestPlunkCampaignThroughQueue drives a queued campaign send through the
// Worker and the Plunk adapter to "sent".
func TestPlunkCampaignThroughQueue(t *testing.T) {
	tenant := uuid.New()
	calls := 0
	p := plunkFor(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		// memQueue.Sent only accepts the tenant id as the provider id.
		_, _ = w.Write([]byte(`{"success":true,"data":{"emails":[{"contact":{"email":"fan@example.com"},"email":"` + tenant.String() + `","status":"queued"}]}}`))
	})
	q := &memQueue{tenants: []uuid.UUID{tenant}}
	q.rows = append(q.rows, &memRow{send: Send{TenantID: tenant, Message: *campaignMsg()}, status: "queued"})
	w := &Worker{Queue: q, Provider: func(context.Context, uuid.UUID) (MarketingProvider, error) { return p, nil }}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || q.count(tenant, "sent") != 1 {
		t.Fatalf("calls=%d sent=%d", calls, q.count(tenant, "sent"))
	}
}
