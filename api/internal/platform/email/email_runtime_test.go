package email

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/klubhub/dj/api/internal/platform/migrations"
)

// Real-Postgres coverage for the outbox runtime: claim semantics, worker
// drain, provider acknowledgement handling and crash recovery. The DJ
// migrations create the email_messages table this package persists to.

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("SKIP_INTEGRATION") == "1" || testing.Short() {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		tcpostgres.WithDatabase("klubhub_email_test"),
		tcpostgres.WithUsername("klubhub"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		println("testcontainers: cannot start postgres, skipping integration:", err.Error())
		os.Exit(m.Run())
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		println("testcontainers: connection string:", err.Error())
		os.Exit(1)
	}
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("sql.Open:", err.Error())
		os.Exit(1)
	}
	if err := migrations.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		println("RunMigrations:", err.Error())
		os.Exit(1)
	}
	_ = db.Close()
	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		println("pgxpool.New:", err.Error())
		os.Exit(1)
	}

	code := m.Run()
	testPool.Close()
	_ = container.Terminate(context.Background())
	os.Exit(code)
}

// newTestRepo builds the DJ-side repository: plaintext columns, no tenant
// scoping (tenantScoped=false).
func newTestRepo() *EmailRepository {
	return NewPlainEmailRepository(testPool)
}

const runtimePlunkAck = `{"success":true,"data":{"emails":[{"contact":{"email":"to@example.com"},"email":"ac32f08e-c6b9-45d3-9824-a73dff1e3bbf"}],"timestamp":"2025-01-15T10:30:00.000Z"}}`

func runtimeEmailRequest() CreateEmailRequest {
	return CreateEmailRequest{Kind: testKindInvoiceIssued, FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "test", Body: "test"}
}

func TestRuntimePlunkAcknowledgements(t *testing.T) {
	for _, body := range []string{"", "<html>proxy</html>", `{"success":true`, `{"success":false}`, `{"success":true}`, `{"success":true,"data":{"emails":[]}}`, runtimePlunkAck + "{}", runtimePlunkAck + strings.Repeat(" ", maxPlunkResponseBytes)} {
		t.Run(body[:min(len(body), 30)], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			p := NewPlunkSender(PlunkConfig{BaseURL: server.URL, FromEmail: "from@example.com"})
			repo := newFakeEmailRepo()
			msg, err := NewEmailService(repo, p).Send(context.Background(), runtimeEmailRequest())
			if err == nil && (msg == nil || msg.Status == EmailStatusSent) {
				t.Fatalf("invalid acknowledgement marked sent: %+v", msg)
			}
		})
	}
}

func TestRuntimePlunkOfficialContract(t *testing.T) {
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/send" {
			t.Errorf("unsupported endpoint %s", r.URL.Path)
		}
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		w.Write([]byte(runtimePlunkAck))
	}))
	defer server.Close()
	p := NewPlunkSender(PlunkConfig{BaseURL: server.URL, FromEmail: "from@example.com"})
	msg := &EmailMessage{ID: uuid.New(), ToEmail: "to@example.com", Subject: "test", Body: "test"}
	for range 2 {
		if err := p.Send(msg); err != nil {
			t.Fatal(err)
		}
	}
	if len(keys) != 2 || keys[0] != msg.ID.String() || keys[1] != keys[0] {
		t.Fatalf("unstable keys %v", keys)
	}
}

func TestRuntimeInitialAttemptCount(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	repo := newTestRepo()
	sender := newFakeSMTPSender()
	sender.Reset(true, errors.New("local failure"))
	msg, err := NewEmailService(repo, sender).Send(context.Background(), runtimeEmailRequest())
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := repo.GetByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != EmailStatusFailed || persisted.Attempts != 1 {
		t.Fatalf("failed initial attempt not counted: %+v", persisted)
	}
}

func TestRuntimeQueuedCrashRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx := context.Background()
	repo := newTestRepo()
	svc := NewEmailService(repo, newFakeSMTPSender())
	msg, err := svc.Enqueue(ctx, runtimeEmailRequest())
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimFailedForRetry(ctx, 16, 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range claimed {
		if m.ID == msg.ID {
			found = true
			if m.Attempts != 1 || m.Status != EmailStatusSending {
				t.Fatalf("bad claim %+v", m)
			}
		}
	}
	if !found {
		t.Fatal("queued message stranded after crash")
	}
	// Simulate process loss after claim, then restart delivery on the same durable ID.
	_, err = testPool.Exec(ctx, `UPDATE email_messages SET updated_at=now()-interval '20 minutes' WHERE id=$1`, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys <- r.Header.Get("Idempotency-Key")
		w.Write([]byte(runtimePlunkAck))
	}))
	defer server.Close()
	svc = NewEmailService(repo, NewPlunkSender(PlunkConfig{BaseURL: server.URL}))
	_, err = svc.RetryFailed(ctx, 16)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != EmailStatusSent || persisted.Attempts != 2 {
		t.Fatalf("crash not recovered %+v", persisted)
	}
	select {
	case key := <-keys:
		if key != msg.ID.String() {
			t.Fatalf("new key %s", key)
		}
	case <-time.After(time.Second):
		t.Fatal("no provider call")
	}
}

func TestRuntimePlunkDuplicateRecovery(t *testing.T) {
	msg := &EmailMessage{ID: uuid.New(), FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "test", Body: "test"}
	for _, status := range []string{"200", "null", "500"} {
		t.Run(status, func(t *testing.T) {
			body := `{"success":false,"error":{"code":"IDEMPOTENCY_KEY_REUSED","details":{"key":"` + msg.ID.String() + `","originalRequest":"POST /v1/send","originalStatusCode":` + status + `}}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
				w.Write([]byte(body))
			}))
			defer server.Close()
			err := NewPlunkSender(PlunkConfig{BaseURL: server.URL}).Send(msg)
			if (err == nil) != (status == "200") {
				t.Fatalf("original status %s got %v", status, err)
			}
		})
	}
}

func TestRuntimeWorkerCancelsAndDrains(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	defer server.Close()
	defer close(released)
	repo := newTestRepo()
	svc := NewEmailService(repo, NewPlunkSender(PlunkConfig{BaseURL: server.URL}))
	msg, err := svc.Enqueue(ctx, runtimeEmailRequest())
	if err != nil {
		t.Fatal(err)
	}
	done := svc.StartWorker(ctx, 10*time.Millisecond, func(err error) { t.Errorf("worker: %v", err) })
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never claimed queued email")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not cancel/drain")
	}
	persisted, err := repo.GetByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != EmailStatusFailed || persisted.Attempts != 1 {
		t.Fatalf("cancelled attempt stranded: %+v", persisted)
	}
}

func TestRuntimePlunkUnsupportedCopyRecipients(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := NewPlunkSender(PlunkConfig{BaseURL: "http://127.0.0.1:1"})
	req := runtimeEmailRequest()
	req.CCEmails = []string{"copy@example.com"}
	_, err := NewEmailService(repo, sender).Send(context.Background(), req)
	if !errors.Is(err, ErrEmailValidation) || len(repo.messages) != 0 {
		t.Fatalf("unsupported CC must fail before durable creation: %v rows=%d", err, len(repo.messages))
	}
	if err = sender.Send(&EmailMessage{ID: uuid.New(), BCCEmails: req.CCEmails}); !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("direct sender accepted unsupported BCC: %v", err)
	}
}

type runtimeFailFinalizationRepo struct {
	*EmailRepository
	fail bool
}

func (r *runtimeFailFinalizationRepo) FinishAttempt(ctx context.Context, msg *EmailMessage, status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error) {
	if r.fail {
		r.fail = false
		return nil, errors.New("controlled post-delivery persistence failure")
	}
	return r.EmailRepository.FinishAttempt(ctx, msg, status, lastError, sentAt)
}

func TestRuntimePostDeliveryPersistenceFailureRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx := context.Background()
	repo := &runtimeFailFinalizationRepo{EmailRepository: newTestRepo(), fail: true}
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		keys = append(keys, key)
		if len(keys) == 1 {
			w.Write([]byte(runtimePlunkAck))
			return
		}
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"success":false,"error":{"code":"IDEMPOTENCY_KEY_REUSED","details":{"key":"` + key + `","originalRequest":"POST /v1/send","originalStatusCode":200}}}`))
	}))
	defer server.Close()
	svc := NewEmailService(repo, NewPlunkSender(PlunkConfig{BaseURL: server.URL}))
	_, err := svc.Send(ctx, runtimeEmailRequest())
	if err == nil {
		t.Fatal("persistence failure hidden")
	}
	rows, err := repo.ListByStatus(ctx, EmailStatusSending, 100)
	if err != nil {
		t.Fatal(err)
	}
	var msg *EmailMessage
	for _, m := range rows {
		if m.ID.String() == keys[0] {
			msg = m
		}
	}
	if msg == nil || msg.Attempts != 1 {
		t.Fatal("lost durable claim after delivery")
	}
	_, err = testPool.Exec(ctx, `UPDATE email_messages SET updated_at=now()-interval '20 minutes' WHERE id=$1`, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.RetryFailed(ctx, 1)
	if err != nil || n != 1 {
		t.Fatalf("recovery n=%d err=%v", n, err)
	}
	persisted, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != EmailStatusSent || persisted.Attempts != 2 || len(keys) != 2 || keys[1] != keys[0] {
		t.Fatalf("incoherent recovery %+v keys=%v", persisted, keys)
	}
}

func TestRuntimeInitialSendIsExclusivelyClaimed(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx := context.Background()
	repo := newTestRepo()
	entered := make(chan string, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- r.Header.Get("Idempotency-Key")
		<-release
		w.Write([]byte(runtimePlunkAck))
	}))
	defer server.Close()
	svc := NewEmailService(repo, NewPlunkSender(PlunkConfig{BaseURL: server.URL}))
	done := make(chan error, 1)
	go func() { _, err := svc.Send(ctx, runtimeEmailRequest()); done <- err }()
	var key string
	select {
	case key = <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("initial send did not enter provider")
	}
	id := uuid.MustParse(key)
	row, err := repo.GetByID(ctx, id)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	if row.Status != EmailStatusSending || row.Attempts != 1 {
		close(release)
		t.Fatalf("initial row not durably claimed %+v", row)
	}
	claimed, err := repo.ClaimFailedForRetry(ctx, 16, 8, 0)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	for _, m := range claimed {
		if m.ID == id {
			close(release)
			t.Fatal("worker raced initial delivery")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeExpiredProviderKeyRequiresReview(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx := context.Background()
	repo := newTestRepo()
	msg, err := repo.Create(ctx, &EmailMessage{Kind: testKindInvoiceIssued, FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "test", Body: "test", Status: EmailStatusFailed, Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(ctx, `UPDATE email_messages SET created_at=now()-interval '25 hours',updated_at=now()-interval '25 hours' WHERE id=$1`, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimFailedForRetry(ctx, 16, 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range claimed {
		if m.ID == msg.ID {
			t.Fatal("expired provider key can duplicate an ambiguous send")
		}
	}
}

type runtimeReadFailure struct{}

func (runtimeReadFailure) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (runtimeReadFailure) Close() error             { return nil }

type runtimeRoundTrip func(*http.Request) (*http.Response, error)

func (f runtimeRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRuntimePlunkReadFailureAndUndocumentedStatuses(t *testing.T) {
	p := NewPlunkSender(PlunkConfig{BaseURL: "http://local.invalid", FromEmail: "from@example.com"})
	msg := &EmailMessage{ID: uuid.New(), ToEmail: "to@example.com", Subject: "test", Body: "test"}
	p.client = &http.Client{Transport: runtimeRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: runtimeReadFailure{}, Header: make(http.Header)}, nil
	})}
	if err := p.Send(msg); err == nil {
		t.Fatal("read failure accepted")
	}
	for _, status := range []int{201, 202, 204} {
		p.client = &http.Client{Transport: runtimeRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(runtimePlunkAck)), Header: make(http.Header)}, nil
		})}
		if err := p.Send(msg); err == nil {
			t.Fatalf("undocumented status %d accepted", status)
		}
	}
}

func TestRuntimeAttachmentsRejectedBeforeEnqueue(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)
	req := runtimeEmailRequest()
	req.AttachmentIDs = []uuid.UUID{uuid.New()}
	_, err := svc.Send(context.Background(), req)
	if !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if len(repo.messages) != 0 || sender.sentCount() != 0 {
		t.Fatal("unsupported attachment caused side effects")
	}
}

func TestRuntimeSendersRejectAttachments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("provider called for unsupported attachment")
		w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	msg := &EmailMessage{ID: uuid.New(), FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "test", Body: "test", AttachmentIDs: []uuid.UUID{uuid.New()}}
	for _, sender := range []EmailSender{NewPlunkSender(PlunkConfig{BaseURL: server.URL}), NewDefaultSMTPSender(SMTPConfig{})} {
		if err := sender.Send(msg); !errors.Is(err, ErrEmailValidation) {
			t.Errorf("expected attachment validation, got %v", err)
		}
	}
}
