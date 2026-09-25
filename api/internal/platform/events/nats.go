package events

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Stream names (plan §13.6).
const (
	StreamEvents = "PROMOTER_EVENTS"
	StreamAudit  = "PROMOTER_AUDIT"
)

// JetStreamPublisher publishes with Nats-Msg-Id so a relay retry after a
// crash never delivers an event twice inside the duplicate window.
type JetStreamPublisher struct{ JS jetstream.JetStream }

// Publish implements Publisher.
func (p JetStreamPublisher) Publish(ctx context.Context, subject string, data []byte, msgID string) error {
	_, err := p.JS.Publish(ctx, subject, data, jetstream.WithMsgID(msgID))
	return err
}

// Connect opens a NATS connection. credsFile is an nsc-generated user
// credentials file (JWT + NKey seed); empty only for local development.
func Connect(url, credsFile, name string) (*nats.Conn, error) {
	opts := []nats.Option{nats.Name(name), nats.MaxReconnects(-1), nats.ReconnectWait(2 * time.Second)}
	if credsFile != "" {
		opts = append(opts, nats.UserCredentials(credsFile))
	}
	return nats.Connect(url, opts...)
}

// EnsureStreams creates or updates the Promoter streams.
func EnsureStreams(ctx context.Context, js jetstream.JetStream) error {
	cfgs := []jetstream.StreamConfig{
		{
			Name: StreamEvents, Subjects: []string{"tenant.>"},
			Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy,
			MaxAge: 30 * 24 * time.Hour, Duplicates: 24 * time.Hour,
		},
		{
			Name: StreamAudit, Subjects: []string{"audit.>"},
			Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy,
			MaxAge: 7 * 24 * time.Hour, Duplicates: 24 * time.Hour,
		},
	}
	for _, c := range cfgs {
		if _, err := js.CreateOrUpdateStream(ctx, c); err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
			return err
		}
	}
	return nil
}
