package marketing

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// ErrNoProvider means the tenant has no marketing provider configured.
var ErrNoProvider = errors.New("marketing: no provider configured")

// ProviderResolver builds the tenant's MarketingProvider from its
// marketing_provider_configs row.
type ProviderResolver func(ctx context.Context, tenant uuid.UUID) (MarketingProvider, error)

// WebhookService turns verified provider events into suppressions.
type WebhookService struct {
	db      *tenantdb.DB
	keys    *envelope.Keyring
	resolve ProviderResolver
	now     func() time.Time
}

// NewWebhookService builds the service; now defaults to time.Now.
func NewWebhookService(db *tenantdb.DB, keys *envelope.Keyring, resolve ProviderResolver, now func() time.Time) *WebhookService {
	if now == nil {
		now = time.Now
	}
	return &WebhookService{db: db, keys: keys, resolve: resolve, now: now}
}

// contactStatus maps an event to the audience_contacts.status it sets.
var contactStatus = map[EventType]string{
	EventBounce: "bounced", EventComplaint: "complained", EventUnsubscribe: "unsubscribed",
}

// EmailIndex is the blind index of an address. It deliberately matches the
// audience contact index, so one lookup serves suppressions, contacts and
// the send path.
func EmailIndex(dek *envelope.DEK, email string) []byte {
	return dek.BlindIndex("audience_contacts", "email_bidx", envelope.NormalizeEmail(email))
}

// Suppressed reports whether the address may not receive marketing mail.
// The send path calls it before enqueueing.
func Suppressed(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, dek *envelope.DEK, email string) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM marketing_suppressions
		WHERE tenant_id = $1 AND email_bidx = $2)`, tenant, EmailIndex(dek, email)).Scan(&found)
	return found, err
}

// Apply records each event: a suppression row (idempotent), the contact's
// status, and the matching send's outcome. One transaction per call.
func (s *WebhookService) Apply(ctx context.Context, tenant uuid.UUID, evs []WebhookEvent) error {
	return s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		for _, ev := range evs {
			status, ok := contactStatus[ev.Type]
			if !ok || ev.Email == "" {
				continue
			}
			bidx := EmailIndex(dek, ev.Email)
			if _, err := tx.Exec(ctx, `INSERT INTO marketing_suppressions (id, tenant_id, email_bidx, reason)
				VALUES ($1, $2, $3, $4) ON CONFLICT (tenant_id, email_bidx) DO NOTHING`,
				uuid.Must(uuid.NewV7()), tenant, bidx, string(ev.Type)); err != nil {
				return err
			}
			// Only an active contact changes: an earlier unsubscribe is not
			// overwritten by a later bounce. Suppressed contacts stay
			// listed (ALL tab) with their status badge.
			rows, err := tx.Query(ctx, `UPDATE audience_contacts SET status = $3, updated_at = now()
				WHERE tenant_id = $1 AND email_bidx = $2 AND status = 'active' RETURNING id`, tenant, bidx, status)
			if err != nil {
				return err
			}
			var ids []uuid.UUID
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			for _, id := range ids {
				subject, e, err := events.New(tenant, "audience", "status_changed", map[string]uuid.UUID{"contact_id": id}, s.now())
				if err != nil {
					return err
				}
				if err := events.Enqueue(ctx, tx, subject, e); err != nil {
					return err
				}
			}
			if ev.Type != EventUnsubscribe && ev.ProviderMessageID != "" {
				if _, err := tx.Exec(ctx, `UPDATE marketing_sends SET status = $3, updated_at = now()
					WHERE tenant_id = $1 AND provider_message_id = $2`, tenant, ev.ProviderMessageID, status); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// WithWebhooks enables POST /api/v1/marketing/webhooks/{tenantID}.
func (h *Handler) WithWebhooks(s *WebhookService) *Handler {
	h.webhooks = s
	return h
}

// mountWebhooks registers the public route: the provider holds no session,
// so the tenant's own provider verifies the signature before anything is
// written (fail closed).
func (h *Handler) mountWebhooks(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	if h.webhooks == nil {
		return
	}
	e.Handle(r, reg, authz.Route{Method: http.MethodPost, Pattern: "/api/v1/marketing/webhooks/{tenantID}", Public: true}, h.webhook, onDeny)
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	tenant, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	p, err := h.webhooks.resolve(r.Context(), tenant)
	if err != nil {
		if errors.Is(err, ErrNoProvider) {
			writeErr(w, http.StatusNotFound, "not_found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal")
		}
		return
	}
	evs, err := p.HandleWebhook(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_signature")
		return
	}
	if err := h.webhooks.Apply(r.Context(), tenant, evs); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + code + `"}`))
}
