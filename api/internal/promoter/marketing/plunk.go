package marketing

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotMarketing is returned when a non-marketing message reaches a
// marketing adapter.
var ErrNotMarketing = errors.New("marketing: adapter only sends marketing mail")

// maxPlunkBody bounds Plunk responses and webhook bodies we read.
const maxPlunkBody = 1 << 20

// PlunkSettings is the plunk marketing_provider_configs.settings_enc JSON,
// unsealed. WebhookSecret is the bearer token the tenant puts on the
// Authorization header of its Plunk workflow webhook step.
type PlunkSettings struct {
	BaseURL       string `json:"base_url,omitempty"` // default https://api.useplunk.com; self-hosted Plunk otherwise
	APIKey        string `json:"api_key"`
	WebhookSecret string `json:"webhook_secret"`
}

// Plunk is the MarketingProvider for Plunk (https://docs.useplunk.com). Each
// queued send goes out as one POST /v1/send, so our queue keeps throttling,
// suppression and unsubscribe; Plunk campaigns would need Plunk-side contacts.
type Plunk struct {
	s      PlunkSettings
	client *http.Client
}

// NewPlunk builds the adapter from a tenant's unsealed settings JSON.
func NewPlunk(settings []byte) (*Plunk, error) {
	var s PlunkSettings
	if err := json.Unmarshal(settings, &s); err != nil {
		return nil, fmt.Errorf("plunk: settings: %w", err)
	}
	if s.APIKey == "" || s.WebhookSecret == "" {
		return nil, errors.New("plunk: settings need api_key and webhook_secret")
	}
	if s.BaseURL == "" {
		s.BaseURL = "https://api.useplunk.com"
	}
	if u, err := url.Parse(s.BaseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("plunk: base_url must be an absolute http(s) url")
	}
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")
	return &Plunk{s: s, client: &http.Client{
		Timeout: 30 * time.Second,
		// A redirect would replay the body and Idempotency-Key elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// SendMarketing posts one message; the Plunk email id is the provider id
// webhooks report back as event.emailId.
func (p *Plunk) SendMarketing(ctx context.Context, msg *Message) (string, error) {
	if msg.Kind != KindMarketing {
		return "", ErrNotMarketing
	}
	unsub, err := UnsubscribeHeaders(msg.UnsubscribeURL)
	if err != nil {
		return "", err
	}
	body := msg.HTML
	if body == "" {
		body = "<pre>" + html.EscapeString(msg.Text) + "</pre>" // Plunk takes HTML only
	}
	fields := map[string]any{
		"to":      msg.To,
		"from":    map[string]string{"email": msg.FromEmail, "name": msg.FromName},
		"subject": msg.Subject,
		"body":    body,
		"headers": map[string]string{
			"List-Unsubscribe":      unsub.Get("List-Unsubscribe"),
			"List-Unsubscribe-Post": unsub.Get("List-Unsubscribe-Post"),
		},
	}
	if msg.ReplyTo != "" {
		fields["reply"] = msg.ReplyTo // Plunk validates reply as an email; omit when unset
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.s.BaseURL+"/v1/send", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.s.APIKey)
	req.Header.Set("Idempotency-Key", msg.SendID.String())
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("plunk: send: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxPlunkBody))
	if err != nil {
		return "", fmt.Errorf("plunk: read: %w", err)
	}
	if resp.StatusCode == http.StatusConflict {
		// A retry of a send Plunk already accepted; the id was lost with the
		// first response, so the row is marked sent without one.
		var dup struct {
			Error struct {
				Code    string `json:"code"`
				Details struct {
					OriginalStatusCode *int `json:"originalStatusCode"`
				} `json:"details"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &dup) == nil && dup.Error.Code == "IDEMPOTENCY_KEY_REUSED" &&
			dup.Error.Details.OriginalStatusCode != nil && *dup.Error.Details.OriginalStatusCode == http.StatusOK {
			return "", nil
		}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("plunk: HTTP %d", resp.StatusCode)
	}
	var ack struct {
		Success bool `json:"success"`
		Data    struct {
			Emails []struct {
				Contact struct {
					Email string `json:"email"`
				} `json:"contact"`
				Email  *string `json:"email"`
				Status string  `json:"status"`
			} `json:"emails"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &ack); err != nil {
		return "", fmt.Errorf("plunk: invalid acknowledgement: %w", err)
	}
	if !ack.Success || len(ack.Data.Emails) != 1 {
		return "", errors.New("plunk: missing acknowledgement")
	}
	e := ack.Data.Emails[0]
	if e.Email == nil || *e.Email == "" || !strings.EqualFold(e.Contact.Email, msg.To) {
		return "", fmt.Errorf("plunk: not accepted (status %q)", e.Status) // throttled: retried with backoff
	}
	return *e.Email, nil
}

// HandleWebhook reads one Plunk workflow webhook (default payload). Plunk
// does not sign webhooks; the tenant's secret rides as a bearer token.
// The payload has no event name, so the type comes from its fields:
// permanent bouncedAt, complainedAt, or an emailless unsubscribed contact.
// Other events (deliveries, opens, transient bounces) yield no events.
func (p *Plunk) HandleWebhook(r *http.Request) ([]WebhookEvent, error) {
	got := []byte(r.Header.Get("Authorization"))
	if subtle.ConstantTimeCompare(got, []byte("Bearer "+p.s.WebhookSecret)) != 1 {
		return nil, errors.New("plunk: bad webhook secret")
	}
	var in struct {
		Contact struct {
			Email      string `json:"email"`
			Subscribed *bool  `json:"subscribed"`
		} `json:"contact"`
		Event struct {
			EmailID      string    `json:"emailId"`
			BounceType   string    `json:"bounceType"`
			BouncedAt    time.Time `json:"bouncedAt"`
			ComplainedAt time.Time `json:"complainedAt"`
		} `json:"event"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxPlunkBody)).Decode(&in); err != nil {
		return nil, fmt.Errorf("plunk: webhook body: %w", err)
	}
	ev := WebhookEvent{Email: in.Contact.Email, ProviderMessageID: in.Event.EmailID}
	switch {
	case !in.Event.ComplainedAt.IsZero():
		ev.Type, ev.At = EventComplaint, in.Event.ComplainedAt
	case !in.Event.BouncedAt.IsZero() && in.Event.BounceType == "Permanent":
		ev.Type, ev.At = EventBounce, in.Event.BouncedAt
	case in.Event.EmailID == "" && in.Event.BouncedAt.IsZero() && in.Contact.Subscribed != nil && !*in.Contact.Subscribed:
		ev.Type = EventUnsubscribe
	default:
		return []WebhookEvent{}, nil
	}
	if ev.Email == "" {
		return []WebhookEvent{}, nil
	}
	return []WebhookEvent{ev}, nil
}
