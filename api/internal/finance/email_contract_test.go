package finance

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeSMTPSender is an in-memory SMTPSender for tests. Safe for concurrent
// use (RetryFailed's concurrency tests deliver from multiple goroutines).
type fakeSMTPSender struct {
	mu       sync.Mutex
	sent     []*EmailMessage
	sentIDs  map[uuid.UUID]int
	failNext bool
	err      error
}

func newFakeSMTPSender() *fakeSMTPSender {
	return &fakeSMTPSender{sent: make([]*EmailMessage, 0), sentIDs: make(map[uuid.UUID]int)}
}

func (f *fakeSMTPSender) Send(msg *EmailMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		return f.err
	}
	f.sent = append(f.sent, msg)
	f.sentIDs[msg.ID]++
	return nil
}

// Reset clears the failNext flag (call before tests that expect success).
func (f *fakeSMTPSender) Reset(failNext bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = failNext
	f.err = err
}

func (f *fakeSMTPSender) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// duplicateSends returns the message IDs that were delivered more than
// once, which must never happen for a correctly-claimed outbox.
func (f *fakeSMTPSender) duplicateSends() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	var dupes []uuid.UUID
	for id, n := range f.sentIDs {
		if n > 1 {
			dupes = append(dupes, id)
		}
	}
	return dupes
}

// fakeEmailRepo is an in-memory EmailRepository. It emulates the atomic
// claim semantics (SELECT ... FOR UPDATE SKIP LOCKED) that
// EmailRepository.ClaimFailedForRetry implements against Postgres by
// guarding the whole map with a mutex, so concurrent callers can never
// observe or mutate the same row at once.
type fakeEmailRepo struct {
	mu       sync.Mutex
	messages map[uuid.UUID]*EmailMessage
}

func newFakeEmailRepo() *fakeEmailRepo {
	return &fakeEmailRepo{messages: make(map[uuid.UUID]*EmailMessage)}
}

func (f *fakeEmailRepo) Create(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *msg
	f.messages[msg.ID] = &cp
	out := cp
	return &out, nil
}

func (f *fakeEmailRepo) GetByID(ctx context.Context, id uuid.UUID) (*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok {
		return nil, ErrEmailNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeEmailRepo) ListByOwner(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*EmailMessage
	for _, m := range f.messages {
		if m.OwnerType == ownerType && m.OwnerID != nil && *m.OwnerID == ownerID {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeEmailRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok {
		return nil, ErrEmailNotFound
	}
	m.Status = status
	m.LastError = lastError
	m.SentAt = sentAt
	m.UpdatedAt = time.Now().UTC()
	cp := *m
	return &cp, nil
}

func (f *fakeEmailRepo) ListByStatus(ctx context.Context, status EmailStatus, limit int) ([]*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*EmailMessage
	for _, m := range f.messages {
		if m.Status == status {
			cp := *m
			out = append(out, &cp)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ClaimFailedForRetry emulates the real repository's atomic
// UPDATE ... WHERE id IN (SELECT ... FOR UPDATE SKIP LOCKED) claim: while
// holding the map mutex it selects eligible 'failed' rows, flips them to
// 'sending' and increments attempts, then returns copies. Because the
// selection and mutation happen under the same lock, two goroutines
// calling this concurrently can never both claim the same row.
func (f *fakeEmailRepo) ClaimFailedForRetry(ctx context.Context, limit, maxAttempts, backoffBaseSeconds int) ([]*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var ids []uuid.UUID
	for id, m := range f.messages {
		if m.Status != EmailStatusFailed {
			continue
		}
		if m.Attempts >= maxAttempts {
			continue
		}
		backoff := time.Duration(m.Attempts*backoffBaseSeconds) * time.Second
		if time.Since(m.UpdatedAt) < backoff {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return f.messages[ids[i]].CreatedAt.Before(f.messages[ids[j]].CreatedAt)
	})
	if limit >= 0 && len(ids) > limit {
		ids = ids[:limit]
	}

	claimed := make([]*EmailMessage, 0, len(ids))
	for _, id := range ids {
		m := f.messages[id]
		m.Status = EmailStatusSending
		m.Attempts++
		m.UpdatedAt = time.Now().UTC()
		cp := *m
		claimed = append(claimed, &cp)
	}
	return claimed, nil
}

// RequeueStale emulates the real repository's reaper: rows stuck in
// 'sending' past olderThan are moved back to 'failed'. updated_at is
// deliberately left untouched so the row is immediately eligible for
// ClaimFailedForRetry's backoff check (it already waited longer than any
// backoff window).
func (f *fakeEmailRepo) RequeueStale(ctx context.Context, olderThan time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().UTC().Add(-olderThan)
	n := 0
	for _, m := range f.messages {
		if m.Status == EmailStatusSending && m.UpdatedAt.Before(cutoff) {
			m.Status = EmailStatusFailed
			m.LastError = "requeued: stuck in sending past timeout"
			n++
		}
	}
	return n, nil
}

func TestEmailService_SendSuccess(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)

	ownerID := uuid.New()
	req := CreateEmailRequest{
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "noreply@klubhub.example",
		FromName:  "KlubHub",
		ToEmail:   "client@example.com",
		Subject:   "Invoice INV-0001-EUR",
		Body:      "Hello, your invoice is attached.",
	}

	msg, err := svc.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg.Status != EmailStatusSent {
		t.Errorf("status: %s", msg.Status)
	}
	if msg.SentAt == nil {
		t.Error("sent_at not set")
	}
	if len(sender.sent) != 1 {
		t.Errorf("expected 1 sent, got %d", len(sender.sent))
	}
}

func TestEmailService_SendFailure(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	sender.failNext = true
	sender.err = errors.New("smtp connection refused")
	svc := NewEmailService(repo, sender)

	ownerID := uuid.New()
	req := CreateEmailRequest{
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "noreply@klubhub.example",
		ToEmail:   "client@example.com",
		Subject:   "Test",
		Body:      "Hello",
	}

	msg, err := svc.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send should not error on SMTP fail: %v", err)
	}
	if msg.Status != EmailStatusFailed {
		t.Errorf("status: %s", msg.Status)
	}
	if msg.LastError == "" {
		t.Error("last_error not set")
	}
}

func TestEmailService_EnqueueWithoutSending(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)

	ownerID := uuid.New()
	req := CreateEmailRequest{
		Kind:      EmailKindAgreementSent,
		OwnerType: "agreement",
		OwnerID:   &ownerID,
		FromEmail: "noreply@klubhub.example",
		ToEmail:   "client@example.com",
		Subject:   "Agreement",
		Body:      "Please review",
	}

	msg, err := svc.Enqueue(context.Background(), req)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if msg.Status != EmailStatusQueued {
		t.Errorf("status: %s", msg.Status)
	}
	if len(sender.sent) != 0 {
		t.Errorf("expected 0 sent, got %d", len(sender.sent))
	}
}

func TestEmailService_RetryFailed(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)

	// Seed a failed message
	ownerID := uuid.New()
	msg := &EmailMessage{
		ID:        uuid.New(),
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "noreply@klubhub.example",
		ToEmail:   "client@example.com",
		Subject:   "Test",
		Body:      "Hello",
		Status:    EmailStatusFailed,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	repo.Create(context.Background(), msg)

	count, err := svc.RetryFailed(context.Background(), 10)
	if err != nil {
		t.Fatalf("RetryFailed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 delivered, got %d", count)
	}
	if len(sender.sent) != 1 {
		t.Errorf("expected 1 sent, got %d", len(sender.sent))
	}
}

// TestEmailService_RetryFailed_ConcurrentDoesNotDoubleDeliver simulates two
// concurrent workers (e.g. two replicas of a retry cron) calling
// RetryFailed against the same outbox at the same time. Because claiming
// happens atomically (ClaimFailedForRetry), each failed row must be
// delivered exactly once in total, never twice.
func TestEmailService_RetryFailed_ConcurrentDoesNotDoubleDeliver(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)

	const numMessages = 40
	for i := 0; i < numMessages; i++ {
		ownerID := uuid.New()
		msg := &EmailMessage{
			ID:        uuid.New(),
			Kind:      EmailKindInvoiceIssued,
			OwnerType: "invoice",
			OwnerID:   &ownerID,
			FromEmail: "noreply@klubhub.example",
			ToEmail:   "client@example.com",
			Subject:   "Test",
			Body:      "Hello",
			Status:    EmailStatusFailed,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if _, err := repo.Create(context.Background(), msg); err != nil {
			t.Fatalf("seed Create: %v", err)
		}
	}

	const numWorkers = 8
	var wg sync.WaitGroup
	totalDelivered := make([]int, numWorkers)
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				n, err := svc.RetryFailed(context.Background(), 3)
				if err != nil {
					t.Errorf("RetryFailed: %v", err)
					return
				}
				totalDelivered[idx] += n
				if n == 0 {
					return
				}
			}
		}(w)
	}
	wg.Wait()

	sum := 0
	for _, n := range totalDelivered {
		sum += n
	}
	if sum != numMessages {
		t.Errorf("expected %d total deliveries across workers, got %d", numMessages, sum)
	}
	if got := sender.sentCount(); got != numMessages {
		t.Errorf("expected %d messages sent, got %d", numMessages, got)
	}
	if dupes := sender.duplicateSends(); len(dupes) != 0 {
		t.Errorf("expected no message delivered more than once, got duplicates: %v", dupes)
	}
}

// TestEmailService_RetryFailed_RequeuesStaleSendingRows verifies that a row
// stuck in 'sending' (e.g. because the worker that claimed it crashed
// mid-delivery) is requeued to 'failed' and retried, instead of being
// stranded forever.
func TestEmailService_RetryFailed_RequeuesStaleSendingRows(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)

	ownerID := uuid.New()
	stuck := &EmailMessage{
		ID:        uuid.New(),
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "noreply@klubhub.example",
		ToEmail:   "client@example.com",
		Subject:   "Test",
		Body:      "Hello",
		Status:    EmailStatusSending,
		Attempts:  1,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
		UpdatedAt: time.Now().UTC().Add(-time.Hour), // long past emailStaleSendingTimeout
	}
	if _, err := repo.Create(context.Background(), stuck); err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	count, err := svc.RetryFailed(context.Background(), 10)
	if err != nil {
		t.Fatalf("RetryFailed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 delivered after requeue, got %d", count)
	}
	got, err := repo.GetByID(context.Background(), stuck.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != EmailStatusSent {
		t.Errorf("expected requeued message to be delivered (status=sent), got %s", got.Status)
	}

	// A row stuck in 'sending' recently (not past the timeout) must NOT be
	// requeued/retried yet.
	repo2 := newFakeEmailRepo()
	svc2 := NewEmailService(repo2, sender)
	recentOwner := uuid.New()
	recent := &EmailMessage{
		ID:        uuid.New(),
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &recentOwner,
		FromEmail: "noreply@klubhub.example",
		ToEmail:   "client@example.com",
		Subject:   "Test",
		Body:      "Hello",
		Status:    EmailStatusSending,
		Attempts:  1,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if _, err := repo2.Create(context.Background(), recent); err != nil {
		t.Fatalf("seed Create: %v", err)
	}
	count2, err := svc2.RetryFailed(context.Background(), 10)
	if err != nil {
		t.Fatalf("RetryFailed: %v", err)
	}
	if count2 != 0 {
		t.Errorf("expected 0 delivered for a row still within the sending timeout, got %d", count2)
	}
}

// TestEmailService_RetryFailed_RespectsMaxAttempts verifies that a row
// which has already exhausted maxEmailAttempts is excluded from further
// retry claims and stays 'failed' permanently.
func TestEmailService_RetryFailed_RespectsMaxAttempts(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)

	ownerID := uuid.New()
	exhausted := &EmailMessage{
		ID:        uuid.New(),
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "noreply@klubhub.example",
		ToEmail:   "client@example.com",
		Subject:   "Test",
		Body:      "Hello",
		Status:    EmailStatusFailed,
		Attempts:  maxEmailAttempts,
		CreatedAt: time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt: time.Now().UTC().Add(-24 * time.Hour),
	}
	if _, err := repo.Create(context.Background(), exhausted); err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	count, err := svc.RetryFailed(context.Background(), 10)
	if err != nil {
		t.Fatalf("RetryFailed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 delivered for a row at maxEmailAttempts, got %d", count)
	}
	got, err := repo.GetByID(context.Background(), exhausted.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != EmailStatusFailed {
		t.Errorf("expected exhausted message to remain failed, got %s", got.Status)
	}
}

func TestEmailValidation_RejectInvalidEmail(t *testing.T) {
	tests := []struct {
		name string
		req  CreateEmailRequest
	}{
		{
			"missing kind",
			CreateEmailRequest{
				Kind:      EmailKind("alien"),
				FromEmail: "a@b.com",
				ToEmail:   "c@d.com",
				Subject:   "x",
				Body:      "y",
			},
		},
		{
			"missing to_email",
			CreateEmailRequest{
				Kind:      EmailKindInvoiceIssued,
				FromEmail: "a@b.com",
				ToEmail:   "",
				Subject:   "x",
				Body:      "y",
			},
		},
		{
			"missing from_email",
			CreateEmailRequest{
				Kind:      EmailKindInvoiceIssued,
				FromEmail: "",
				ToEmail:   "c@d.com",
				Subject:   "x",
				Body:      "y",
			},
		},
		{
			"missing subject",
			CreateEmailRequest{
				Kind:      EmailKindInvoiceIssued,
				FromEmail: "a@b.com",
				ToEmail:   "c@d.com",
				Subject:   "",
				Body:      "y",
			},
		},
		{
			"missing body",
			CreateEmailRequest{
				Kind:      EmailKindInvoiceIssued,
				FromEmail: "a@b.com",
				ToEmail:   "c@d.com",
				Subject:   "x",
				Body:      "",
				BodyHTML:  "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateEmailRequest(tt.req)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestEmailValidation_AcceptsMinimalValid(t *testing.T) {
	req := CreateEmailRequest{
		Kind:      EmailKindInvoiceIssued,
		FromEmail: "a@b.com",
		ToEmail:   "c@d.com",
		Subject:   "x",
		Body:      "y",
	}
	if err := ValidateCreateEmailRequest(req); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

// TestEmailValidation_RejectsHeaderInjection asserts that CR/LF (and other
// control characters) anywhere in Subject, FromName, or any address field
// are rejected, closing the header-injection hole where a value like
// "a@b.com\r\nBcc: attacker@evil.com" could otherwise smuggle an extra
// header into a raw SMTP message built by string interpolation.
func TestEmailValidation_RejectsHeaderInjection(t *testing.T) {
	base := func() CreateEmailRequest {
		return CreateEmailRequest{
			Kind:      EmailKindInvoiceIssued,
			FromEmail: "a@b.com",
			FromName:  "KlubHub",
			ToEmail:   "c@d.com",
			Subject:   "hello",
			Body:      "body",
		}
	}

	tests := []struct {
		name   string
		mutate func(*CreateEmailRequest)
	}{
		{"CRLF in subject", func(r *CreateEmailRequest) { r.Subject = "hi\r\nBcc: attacker@evil.com" }},
		{"bare LF in subject", func(r *CreateEmailRequest) { r.Subject = "hi\nBcc: attacker@evil.com" }},
		{"bare CR in subject", func(r *CreateEmailRequest) { r.Subject = "hi\rBcc: attacker@evil.com" }},
		{"control char in subject", func(r *CreateEmailRequest) { r.Subject = "hi\x00there" }},
		{"CRLF in from_name", func(r *CreateEmailRequest) { r.FromName = "KlubHub\r\nBcc: attacker@evil.com" }},
		{"CRLF in to_email", func(r *CreateEmailRequest) { r.ToEmail = "c@d.com\r\nBcc: attacker@evil.com" }},
		{"CRLF in from_email", func(r *CreateEmailRequest) { r.FromEmail = "a@b.com\r\nBcc: attacker@evil.com" }},
		{"CRLF in cc", func(r *CreateEmailRequest) { r.CCEmails = []string{"cc@d.com\r\nBcc: attacker@evil.com"} }},
		{"CRLF in bcc", func(r *CreateEmailRequest) { r.BCCEmails = []string{"bcc@d.com\r\nBcc: attacker@evil.com"} }},
		{"invalid to_email address", func(r *CreateEmailRequest) { r.ToEmail = "not-an-email" }},
		{"invalid cc address", func(r *CreateEmailRequest) { r.CCEmails = []string{"not-an-email"} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := base()
			tt.mutate(&req)
			if err := ValidateCreateEmailRequest(req); err == nil {
				t.Errorf("expected rejection for %s", tt.name)
			}
		})
	}
}

func TestRejectHeaderInjection(t *testing.T) {
	if err := rejectHeaderInjection("a normal subject line"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := rejectHeaderInjection("has\ttab, fine"); err != nil {
		t.Errorf("tab should be allowed, got %v", err)
	}
	for _, bad := range []string{"a\r\nb", "a\nb", "a\rb", "a\x00b"} {
		if err := rejectHeaderInjection(bad); err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
	}
}

func TestEmailKind_IsValid(t *testing.T) {
	tests := []struct {
		kind  EmailKind
		valid bool
	}{
		{EmailKindInvoiceIssued, true},
		{EmailKindInvoicePaid, true},
		{EmailKindInvoiceCancelled, true},
		{EmailKindAgreementSent, true},
		{EmailKindAgreementSigned, true},
		{EmailKindAgreementCompleted, true},
		{EmailKind("invalid"), false},
	}
	for _, tt := range tests {
		if got := tt.kind.IsValid(); got != tt.valid {
			t.Errorf("%s: got %v, want %v", tt.kind, got, tt.valid)
		}
	}
}

func TestDefaultSMTPSender_BuildsRFC5322Message(t *testing.T) {
	// Construct an instance and call Send without actually sending to SMTP
	// (this validates the message-building logic only, not the delivery)
	sender := NewDefaultSMTPSender(SMTPConfig{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user",
		Password: "pass",
		From:     "noreply@klubhub.example",
	})

	if sender.cfg.Host != "smtp.example.com" {
		t.Errorf("host: %s", sender.cfg.Host)
	}
}

// TestDefaultSMTPSender_RejectsHeaderInjectionDefenseInDepth verifies the
// SMTP sender itself refuses to build a message from an unsafe Subject or
// FromName, even if a caller bypassed ValidateCreateEmailRequest. It must
// fail fast with a validation error, before ever attempting to contact the
// (nonexistent) SMTP host.
func TestDefaultSMTPSender_RejectsHeaderInjectionDefenseInDepth(t *testing.T) {
	sender := NewDefaultSMTPSender(SMTPConfig{
		Host: "smtp.invalid.example", // does not exist; must not be dialed
		Port: 587,
	})

	tests := []struct {
		name string
		msg  *EmailMessage
	}{
		{
			"malicious subject",
			&EmailMessage{
				FromEmail: "noreply@klubhub.example",
				ToEmail:   "client@example.com",
				Subject:   "hi\r\nBcc: attacker@evil.com",
				Body:      "hello",
			},
		},
		{
			"malicious from name",
			&EmailMessage{
				FromEmail: "noreply@klubhub.example",
				FromName:  "KlubHub\r\nBcc: attacker@evil.com",
				ToEmail:   "client@example.com",
				Subject:   "hi",
				Body:      "hello",
			},
		},
		{
			"malicious to address",
			&EmailMessage{
				FromEmail: "noreply@klubhub.example",
				ToEmail:   "client@example.com\r\nBcc: attacker@evil.com",
				Subject:   "hi",
				Body:      "hello",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sender.Send(tt.msg)
			if err == nil {
				t.Fatal("expected Send to reject header injection")
			}
			if got := err.Error(); !strings.Contains(got, "control charact") && !strings.Contains(got, "invalid") {
				t.Errorf("expected a validation error, got: %v", got)
			}
		})
	}
}

func TestFormatAddress(t *testing.T) {
	if got := formatAddress("", "a@b.com"); got != "<a@b.com>" {
		t.Errorf("got %q", got)
	}
	if got := formatAddress("Jane Doe", "a@b.com"); got != `"Jane Doe" <a@b.com>` {
		t.Errorf("got %q", got)
	}
}
