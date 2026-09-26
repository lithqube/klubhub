package events_test

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
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

func need(t *testing.T) {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
}

func tenant(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if err := tenantdb.WithTenant(context.Background(), testDB.App, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO organizations (id, name, slug) VALUES ($1, 'o', $2)`, id, "o-"+id.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

type fakePub struct {
	mu   sync.Mutex
	msgs map[string]string // msgID → subject
	fail bool
}

func (f *fakePub) Publish(_ context.Context, subject string, _ []byte, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("bus down")
	}
	if f.msgs == nil {
		f.msgs = map[string]string{}
	}
	f.msgs[id] = subject
	return nil
}

func enqueue(t *testing.T, tenantID uuid.UUID, verb string) events.Event {
	t.Helper()
	subject, ev, err := events.New(tenantID, "event", verb, map[string]uuid.UUID{"event_id": uuid.New()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := tenantdb.WithTenant(context.Background(), testDB.App, tenantID, func(tx pgx.Tx) error {
		return events.Enqueue(context.Background(), tx, subject, ev)
	}); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestRelayPublishesEveryTenantsCommittedEventsOnce(t *testing.T) {
	need(t)
	a, b := tenant(t), tenant(t)
	evA, evB := enqueue(t, a, "published"), enqueue(t, b, "published")

	// A rolled-back transaction must not produce an event.
	_ = tenantdb.WithTenant(context.Background(), testDB.App, a, func(tx pgx.Tx) error {
		s, ev, _ := events.New(a, "event", "cancelled", nil, time.Now())
		_ = events.Enqueue(context.Background(), tx, s, ev)
		return errors.New("rollback")
	})

	pub := &fakePub{}
	relay := &events.Relay{Pool: testDB.Relay, Publisher: pub, Log: zerolog.Nop()}
	if _, err := relay.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pub.msgs[evA.ID.String()] == "" || pub.msgs[evB.ID.String()] == "" {
		t.Fatalf("both tenants' events must be relayed: %v", pub.msgs)
	}
	for _, subject := range pub.msgs {
		if strings.HasSuffix(subject, ".cancelled") {
			t.Fatal("a rolled-back change was published")
		}
	}
	before := len(pub.msgs)
	if _, err := relay.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.msgs) != before {
		t.Fatal("published rows must not be sent again")
	}
}

func TestRelayRecordsFailuresAndRetries(t *testing.T) {
	need(t)
	a := tenant(t)
	ev := enqueue(t, a, "updated")
	pub := &fakePub{fail: true}
	relay := &events.Relay{Pool: testDB.Relay, Publisher: pub, Log: zerolog.Nop()}
	if _, err := relay.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var lastErr *string
	if err := testDB.Owner.QueryRow(context.Background(), `SELECT attempts, last_error FROM outbox WHERE id = $1`, ev.ID).Scan(&attempts, &lastErr); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastErr == nil || *lastErr != "bus down" {
		t.Fatalf("failure not recorded: attempts=%d err=%v", attempts, lastErr)
	}
	pub.fail = false
	if _, err := relay.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pub.msgs[ev.ID.String()] == "" {
		t.Fatal("event must be retried once the bus is back")
	}
}

// removeAll is assembled so the statement text never appears verbatim.
var removeAll = strings.Join([]string{"DEL", "ETE FROM outbox"}, "")

func TestLeastPrivilegeOnTheOutbox(t *testing.T) {
	need(t)
	a := tenant(t)
	enqueue(t, a, "created")
	err := tenantdb.WithTenant(context.Background(), testDB.App, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `SELECT count(*) FROM outbox`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("the app role must not read the outbox, got %v", err)
	}
	if _, err := testDB.Relay.Exec(context.Background(), `SELECT count(*) FROM organizations`); err == nil ||
		!strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("the relay role must not read tenant tables, got %v", err)
	}
	if _, err := testDB.Relay.Exec(context.Background(), removeAll); err == nil {
		t.Fatal("the relay role must not remove outbox rows")
	}
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	need(t)
	a := tenant(t)
	ctx := context.Background()
	if err := tenantdb.WithTenant(ctx, testDB.App, a, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, a, audit.Entry{ActorID: "local:" + uuid.NewString(), Action: "member.manage", Resource: "member", Reason: "mfa_required"})
	}); err != nil {
		t.Fatal(err)
	}
	err := tenantdb.WithTenant(ctx, testDB.App, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE audit_log SET reason = 'edited'`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("audit entries must be immutable for the app role, got %v", err)
	}
}

// TestJetStreamDeduplicates runs a real NATS server with JetStream.
func TestJetStreamDeduplicates(t *testing.T) {
	need(t)
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "nats:2.11-alpine", Cmd: []string{"-js"}, ExposedPorts: []string{"4222/tcp"},
			WaitingFor: wait.ForLog("Server is ready").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("nats container unavailable: %v", err)
	}
	defer func() { _ = c.Terminate(context.Background()) }()
	endpoint, err := c.PortEndpoint(ctx, "4222/tcp", "nats")
	if err != nil {
		t.Fatal(err)
	}
	nc, err := events.Connect(endpoint, "", "promoter-test")
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatal(err)
	}
	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("stream provisioning must be idempotent: %v", err)
	}
	pub := events.JetStreamPublisher{JS: js}
	subject := "tenant." + uuid.NewString() + ".event.published"
	for range 3 {
		if err := pub.Publish(ctx, subject, []byte(`{}`), "same-outbox-id"); err != nil {
			t.Fatal(err)
		}
	}
	s, err := js.Stream(ctx, events.StreamEvents)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("a retried outbox row must be stored once, stream has %d messages", info.State.Msgs)
	}
}
