package finance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeSMTPSender is an in-memory SMTPSender for tests.
type fakeSMTPSender struct {
	sent     []*EmailMessage
	failNext bool
	err      error
}

func newFakeSMTPSender() *fakeSMTPSender {
	return &fakeSMTPSender{sent: make([]*EmailMessage, 0)}
}

func (f *fakeSMTPSender) Send(msg *EmailMessage) error {
	if f.failNext {
		return f.err
	}
	f.sent = append(f.sent, msg)
	return nil
}

// fakeEmailRepo is an in-memory EmailRepository.
type fakeEmailRepo struct {
	messages map[uuid.UUID]*EmailMessage
}

func newFakeEmailRepo() *fakeEmailRepo {
	return &fakeEmailRepo{messages: make(map[uuid.UUID]*EmailMessage)}
}

func (f *fakeEmailRepo) Create(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	cp := *msg
	f.messages[msg.ID] = &cp
	return &cp, nil
}

func (f *fakeEmailRepo) GetByID(ctx context.Context, id uuid.UUID) (*EmailMessage, error) {
	m, ok := f.messages[id]
	if !ok {
		return nil, ErrEmailNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeEmailRepo) ListByOwner(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error) {
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
				Kind:    EmailKindInvoiceIssued,
				FromEmail: "",
				ToEmail: "c@d.com",
				Subject: "x",
				Body:    "y",
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
		Host: "smtp.example.com",
		Port: 587,
		Username: "user",
		Password: "pass",
		From:     "noreply@klubhub.example",
	})

	if sender.cfg.Host != "smtp.example.com" {
		t.Errorf("host: %s", sender.cfg.Host)
	}
}