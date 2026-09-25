package finance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Real-Postgres coverage for the SQL added in the review fixes. The
// contract tests exercise the same rules against in-memory fakes; these
// prove the actual statements parse and enforce them.

func TestIntegration_AgreementSignGuard(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	gigID := insertTestGig(t, "EUR")
	tpl, err := NewAgreementTemplateRepository(testPool).Create(ctx, CreateAgreementTemplateRequest{
		Name: "sign-guard-" + uuid.NewString(), ContentMD: "# Booking"})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	repo := NewAgreementInstanceRepository(testPool)
	inst, err := repo.Create(ctx, CreateAgreementInstanceRequest{
		TemplateID: tpl.ID, GigID: gigID, RequiredSigners: []string{"dj", "client"}}, tpl.ContentMD, tpl.Version)
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	signed, err := repo.Sign(ctx, inst.ID, SignAgreementRequest{SignerRole: "dj", SignedBy: "DJ", UpdatedAt: inst.UpdatedAt})
	if err != nil {
		t.Fatalf("dj sign: %v", err)
	}
	// Same role again, with a fresh token: already signed.
	if _, err := repo.Sign(ctx, inst.ID, SignAgreementRequest{SignerRole: "dj", SignedBy: "Impostor", UpdatedAt: signed.UpdatedAt}); !errors.Is(err, ErrAgreementBadState) {
		t.Fatalf("double sign: want ErrAgreementBadState, got %v", err)
	}
	// Stale token is a conflict, not bad state.
	if _, err := repo.Sign(ctx, inst.ID, SignAgreementRequest{SignerRole: "client", SignedBy: "Promoter", UpdatedAt: inst.UpdatedAt}); !errors.Is(err, ErrAgreementConflict) {
		t.Fatalf("stale token: want ErrAgreementConflict, got %v", err)
	}
	// Cancelled agreements can't be revived by signing.
	if _, err := testPool.Exec(ctx, `UPDATE agreement_instances SET status = 'cancelled' WHERE id = $1`, inst.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	var token time.Time
	_ = testPool.QueryRow(ctx, `SELECT updated_at FROM agreement_instances WHERE id = $1`, inst.ID).Scan(&token)
	if _, err := repo.Sign(ctx, inst.ID, SignAgreementRequest{SignerRole: "client", SignedBy: "Promoter", UpdatedAt: token}); !errors.Is(err, ErrAgreementBadState) {
		t.Fatalf("sign cancelled: want ErrAgreementBadState, got %v", err)
	}
}

func TestIntegration_DocumentVersionsAreSequential(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewDocumentRepository(testPool)
	owner := uuid.New()

	const n = 6
	var wg sync.WaitGroup
	versions := make(chan int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := repo.CreateWithDetails(ctx,
				CreateDocumentRequest{OwnerType: DocumentOwnerInvoice, OwnerID: owner, Filename: "inv.pdf", MimeType: "application/pdf"},
				CreateDocumentDetails{StorageKey: "invoice/" + owner.String() + "/" + uuid.NewString(), SizeBytes: 1, ChecksumSHA256: "x"})
			switch {
			case err == nil:
				versions <- d.Version
			case errors.Is(err, ErrDocumentConflict):
				// acceptable under contention: the caller retries
			default:
				t.Errorf("CreateWithDetails: %v", err)
			}
		}()
	}
	wg.Wait()
	close(versions)
	seen := map[int]bool{}
	for v := range versions {
		if seen[v] {
			t.Fatalf("duplicate document version %d", v)
		}
		seen[v] = true
	}
	var current int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE owner_id = $1 AND is_current`, owner).Scan(&current); err != nil || current != 1 {
		t.Fatalf("current rows = %d err = %v, want exactly 1", current, err)
	}
}

func TestIntegration_EmailOutboxClaimAndRequeue(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEmailRepository(testPool)
	msg, err := repo.Create(ctx, &EmailMessage{Kind: EmailKindInvoiceIssued, FromEmail: "dj@example.com",
		ToEmail: "promoter@example.com", Subject: "Invoice", Body: "Hi", Status: EmailStatusQueued})
	if err != nil {
		t.Fatalf("create email: %v", err)
	}
	// Simulate a crash mid-send an hour ago.
	if _, err := testPool.Exec(ctx, `UPDATE email_messages SET status = 'sending', updated_at = now() - interval '1 hour' WHERE id = $1`, msg.ID); err != nil {
		t.Fatalf("stage stuck row: %v", err)
	}
	n, err := repo.RequeueStale(ctx, 15*time.Minute)
	if err != nil || n < 1 {
		t.Fatalf("RequeueStale = %d err = %v", n, err)
	}

	// Two concurrent claimers: the row must be handed out exactly once.
	var wg sync.WaitGroup
	claims := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := repo.ClaimFailedForRetry(ctx, 10, 8, 0)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			c := 0
			for _, m := range got {
				if m.ID == msg.ID {
					c++
				}
			}
			claims <- c
		}()
	}
	wg.Wait()
	close(claims)
	total := 0
	for c := range claims {
		total += c
	}
	if total != 1 {
		t.Fatalf("message claimed %d times, want 1", total)
	}
}
