// Package events is the Promoter transactional outbox and its NATS
// JetStream relay (plan §13.6, decision D8).
//
// Subjects: tenant.{tenant_id}.{aggregate}.{verb} and audit.{class}.{name}.
// Payloads are events.Event: identifiers only, by construction, so the bus
// never carries personal data and a leaked stream reveals no guest, contact
// or finance values.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var token = regexp.MustCompile(`^[a-z][a-z_]{0,40}$`)

// ErrBadSubject is returned for subjects outside the taxonomy.
var ErrBadSubject = errors.New("events: invalid subject")

// Event is the only payload shape published to the bus.
type Event struct {
	ID         uuid.UUID         `json:"id"`
	Type       string            `json:"type"` // aggregate.verb
	TenantID   uuid.UUID         `json:"tenant_id"`
	OccurredAt time.Time         `json:"occurred_at"`
	Refs       map[string]string `json:"refs,omitempty"` // name → UUID
	Schema     int               `json:"schema"`
}

// TenantSubject builds tenant.{tenant}.{aggregate}.{verb}.
func TenantSubject(tenant uuid.UUID, aggregate, verb string) (string, error) {
	if tenant == uuid.Nil || !token.MatchString(aggregate) || !token.MatchString(verb) {
		return "", ErrBadSubject
	}
	return fmt.Sprintf("tenant.%s.%s.%s", tenant, aggregate, verb), nil
}

// New builds an event for tenant. Refs values must be UUIDs.
func New(tenant uuid.UUID, aggregate, verb string, refs map[string]uuid.UUID, now time.Time) (string, Event, error) {
	subject, err := TenantSubject(tenant, aggregate, verb)
	if err != nil {
		return "", Event{}, err
	}
	r := make(map[string]string, len(refs))
	for k, v := range refs {
		if !token.MatchString(k) || v == uuid.Nil {
			return "", Event{}, fmt.Errorf("events: bad ref %q", k)
		}
		r[k] = v.String()
	}
	return subject, Event{
		ID: uuid.Must(uuid.NewV7()), Type: aggregate + "." + verb, TenantID: tenant,
		OccurredAt: now.UTC(), Refs: r, Schema: 1,
	}, nil
}

// Enqueue appends ev to the outbox inside the caller's tenant transaction,
// so the event is published if and only if the change commits.
func Enqueue(ctx context.Context, tx pgx.Tx, subject string, ev Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox (id, tenant_id, subject, payload) VALUES ($1, $2, $3, $4)`,
		ev.ID, ev.TenantID, subject, payload)
	return err
}
