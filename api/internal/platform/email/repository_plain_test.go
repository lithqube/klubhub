package email

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegration_PlainRepository_FullLifecycle exercises the DJ-side
// (tenantScoped=false) repository against real Postgres. This is the
// platform-level regression net for the extraction: the plain path is what
// finance runs in production, and its behavior must be identical to the
// pre-extraction finance repository.
func TestIntegration_PlainRepository_FullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx := context.Background()
	repo := newTestRepo()
	ownerID := uuid.New()
	attachmentID := uuid.New()

	// Create round-trips every column.
	msg, err := repo.Create(ctx, &EmailMessage{
		Kind:          testKindInvoiceIssued,
		OwnerType:     "invoice",
		OwnerID:       &ownerID,
		FromEmail:     "from@example.com",
		FromName:      "KlubHub",
		ToEmail:       "to@example.com",
		CCEmails:      []string{"cc@example.com"},
		BCCEmails:     []string{"bcc@example.com"},
		Subject:       "subj",
		Body:          "body",
		BodyHTML:      "<p>body</p>",
		AttachmentIDs: []uuid.UUID{attachmentID},
		ReplyTo:       "reply@example.com",
		Status:        EmailStatusQueued,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if msg.ID == uuid.Nil || msg.CreatedAt.IsZero() || msg.UpdatedAt.IsZero() {
		t.Fatalf("Create did not return server columns: %+v", msg)
	}
	if msg.Kind != testKindInvoiceIssued || msg.OwnerType != "invoice" || msg.OwnerID == nil || *msg.OwnerID != ownerID {
		t.Errorf("Create owner fields: %+v", msg)
	}
	if msg.FromName != "KlubHub" || msg.ToEmail != "to@example.com" || msg.Subject != "subj" ||
		msg.Body != "body" || msg.BodyHTML != "<p>body</p>" || msg.ReplyTo != "reply@example.com" {
		t.Errorf("Create content fields: %+v", msg)
	}
	if len(msg.CCEmails) != 1 || msg.CCEmails[0] != "cc@example.com" ||
		len(msg.BCCEmails) != 1 || msg.BCCEmails[0] != "bcc@example.com" {
		t.Errorf("Create cc/bcc: %+v", msg)
	}
	if len(msg.AttachmentIDs) != 1 || msg.AttachmentIDs[0] != attachmentID {
		t.Errorf("Create attachment ids: %+v", msg.AttachmentIDs)
	}

	// GetByID returns the same row; a missing id is ErrEmailNotFound.
	got, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ToEmail != "to@example.com" || got.Status != EmailStatusQueued || got.Attempts != 0 {
		t.Errorf("GetByID: %+v", got)
	}
	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, ErrEmailNotFound) {
		t.Errorf("GetByID missing = %v, want ErrEmailNotFound", err)
	}

	// ListByOwner filters by owner.
	list, err := repo.ListByOwner(ctx, "invoice", ownerID)
	if err != nil || len(list) != 1 || list[0].ID != msg.ID {
		t.Fatalf("ListByOwner: %+v, %v", list, err)
	}
	if list, err := repo.ListByOwner(ctx, "invoice", uuid.New()); err != nil || len(list) != 0 {
		t.Errorf("ListByOwner other owner: %+v, %v", list, err)
	}
	if list, err := repo.ListByOwner(ctx, "agreement", ownerID); err != nil || len(list) != 0 {
		t.Errorf("ListByOwner other type: %+v, %v", list, err)
	}

	// UpdateStatus transitions and stamps sent_at; a missing id errors.
	sentAt := time.Now().UTC().Truncate(time.Microsecond)
	updated, err := repo.UpdateStatus(ctx, msg.ID, EmailStatusSent, "", &sentAt)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if updated.Status != EmailStatusSent || updated.SentAt == nil {
		t.Errorf("UpdateStatus: %+v", updated)
	}
	if _, err := repo.UpdateStatus(ctx, uuid.New(), EmailStatusSent, "", nil); !errors.Is(err, ErrEmailNotFound) {
		t.Errorf("UpdateStatus missing = %v, want ErrEmailNotFound", err)
	}

	// ListByStatus filters by status.
	if list, err := repo.ListByStatus(ctx, EmailStatusQueued, 100); err != nil {
		t.Fatal(err)
	} else {
		for _, m := range list {
			if m.ID == msg.ID {
				t.Error("sent message listed as queued")
			}
		}
	}
	queued, err := repo.Create(ctx, &EmailMessage{
		Kind: testKindInvoiceIssued, FromEmail: "from@example.com",
		ToEmail: "to@example.com", Subject: "s", Body: "b", Status: EmailStatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err = repo.ListByStatus(ctx, EmailStatusQueued, 100)
	if err != nil || len(list) == 0 {
		t.Fatalf("ListByStatus queued: %+v, %v", list, err)
	}
	found := false
	for _, m := range list {
		if m.ID == queued.ID {
			found = true
		}
	}
	if !found {
		t.Error("queued message not listed")
	}
}

// TestIntegration_PlainRepository_ClaimAndRequeue covers the retry-claim
// and stale-reaper semantics of the plain path against real Postgres.
func TestIntegration_PlainRepository_ClaimAndRequeue(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx := context.Background()
	repo := newTestRepo()

	newRow := func(status EmailStatus, attempts int) *EmailMessage {
		msg, err := repo.Create(ctx, &EmailMessage{
			Kind: testKindInvoiceIssued, FromEmail: "from@example.com",
			ToEmail: "to@example.com", Subject: "s", Body: "b",
			Status: status, Attempts: attempts,
		})
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}
	age := func(id uuid.UUID, d time.Duration) {
		if _, err := testPool.Exec(ctx, `UPDATE email_messages SET updated_at=now()-($1::interval) WHERE id=$2`, d.String(), id); err != nil {
			t.Fatal(err)
		}
	}

	// A failed row past its backoff is claimable; a fresh one is not.
	eligible := newRow(EmailStatusFailed, 1)
	age(eligible.ID, time.Hour)
	fresh := newRow(EmailStatusFailed, 1)

	claimed, err := repo.ClaimFailedForRetry(ctx, 16, maxEmailAttempts, emailRetryBackoffBaseSeconds)
	if err != nil {
		t.Fatal(err)
	}
	var gotEligible bool
	for _, m := range claimed {
		if m.ID == fresh.ID {
			t.Fatal("fresh failed row claimed before its backoff elapsed")
		}
		if m.ID == eligible.ID {
			gotEligible = true
			if m.Status != EmailStatusSending || m.Attempts != 2 {
				t.Errorf("claimed row = %+v, want sending/attempts=2", m)
			}
		}
	}
	if !gotEligible {
		t.Fatal("eligible failed row not claimed")
	}

	// A row exhausted past maxAttempts is never claimed again.
	exhausted := newRow(EmailStatusFailed, maxEmailAttempts)
	age(exhausted.ID, 25*time.Hour)
	claimed, err = repo.ClaimFailedForRetry(ctx, 16, maxEmailAttempts, emailRetryBackoffBaseSeconds)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range claimed {
		if m.ID == exhausted.ID {
			t.Fatal("exhausted row claimed")
		}
	}

	// RequeueStale moves rows stuck in 'sending' past the timeout back to
	// 'failed' and leaves fresh ones alone.
	stuck := newRow(EmailStatusSending, 1)
	age(stuck.ID, 20*time.Minute)
	freshSending := newRow(EmailStatusSending, 1)
	n, err := repo.RequeueStale(ctx, emailStaleSendingTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("RequeueStale requeued %d, want >= 1", n)
	}
	got, err := repo.GetByID(ctx, stuck.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != EmailStatusFailed {
		t.Errorf("stuck row = %q, want failed", got.Status)
	}
	got, err = repo.GetByID(ctx, freshSending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != EmailStatusSending {
		t.Errorf("fresh sending row = %q, want sending", got.Status)
	}
}
