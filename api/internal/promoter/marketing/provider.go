// Package marketing is the promoter marketing-email stream (P3.2): campaign
// mail to audience contacts, kept apart from the transactional outbox in
// platform/email. Rows live in marketing_sends / marketing_suppressions
// (migration 00018); delivery goes through a per-tenant MarketingProvider.
package marketing

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// MarketingProvider is one delivery backend (generic SMTP, Plunk). An
// instance is built from a tenant's marketing_provider_configs row, so it
// carries that tenant's credentials and webhook secret.
type MarketingProvider interface {
	// SendMarketing delivers one message and returns the provider's message
	// id ("" when the transport has none, e.g. SMTP).
	SendMarketing(ctx context.Context, msg *Message) (providerMessageID string, err error)
	// HandleWebhook verifies the provider's signature and parses the
	// delivery events in the request. A bad signature must return an error
	// and no events (fail closed).
	HandleWebhook(r *http.Request) ([]WebhookEvent, error)
}

// Kind names the mail stream a message belongs to.
type Kind string

// KindMarketing is the only kind a marketing adapter delivers; anything else
// (including the zero value) is refused with ErrNotMarketing, so
// transactional mail cannot leave through the campaign path.
const KindMarketing Kind = "marketing"

// Message is one unsealed marketing email, ready for delivery.
type Message struct {
	Kind Kind
	// SendID is the marketing_sends row id; providers that support it use it
	// as the idempotency key so a retried send is not delivered twice.
	SendID    uuid.UUID
	To        string
	FromEmail string
	FromName  string
	ReplyTo   string
	Subject   string
	Text      string
	HTML      string
	// UnsubscribeURL is the RFC 8058 one-click target; the adapter renders
	// both List-Unsubscribe and List-Unsubscribe-Post from it.
	UnsubscribeURL string
}

// EventType is a delivery outcome reported by a provider webhook. Each value
// matches a marketing_suppressions.reason.
type EventType string

const (
	EventBounce      EventType = "bounce"
	EventComplaint   EventType = "complaint"
	EventUnsubscribe EventType = "unsubscribe"
)

// WebhookEvent is one parsed, signature-verified provider event.
type WebhookEvent struct {
	Type              EventType
	Email             string
	ProviderMessageID string
	At                time.Time
}
