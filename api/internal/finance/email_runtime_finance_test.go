package finance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The pure-email runtime tests moved to platform/email/email_runtime_test.go
// with the extraction. These three stay in finance because they exercise
// finance-coupled behaviour: the agreement PDF generation stub (which must
// stay explicitly unsupported and must never write artifacts) and the
// finance email HTTP handler's rejection of client-supplied attachment ids.

func TestRuntimeAgreementGenerationUnavailable(t *testing.T) {
	svc := NewAgreementInstanceService(newFakeInstanceRepo(), newFakeTemplateRepo(), nil)
	_, err := svc.GeneratePDF(context.Background(), uuid.New())
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected explicit unsupported generation, got %v", err)
	}
	rec := httptest.NewRecorder()
	NewAgreementInstanceHandler(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/agreements/instances/"+uuid.NewString(), nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "not supported") {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRuntimeAgreementGenerationDoesNotWriteArtifacts(t *testing.T) {
	if testing.Short() {
		t.Skip("real migrated postgres")
	}
	if testPool == nil {
		t.Fatal("postgres required")
	}
	ctx := context.Background()
	var before, after int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// A configured real repository and document service still cannot enable the stub.
	svc := NewAgreementInstanceService(NewAgreementInstanceRepository(testPool), NewAgreementTemplateRepository(testPool), NewDocumentService(NewDocumentRepository(testPool), nil, "fixture"))
	for range 3 {
		rec := httptest.NewRecorder()
		NewAgreementInstanceHandler(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/agreements/instances/"+uuid.NewString(), nil))
		if rec.Code != 503 || !strings.Contains(rec.Body.String(), "generation_unsupported") {
			t.Fatalf("dishonest generation %d %s", rec.Code, rec.Body.String())
		}
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("orphan artifact created: before=%d after=%d", before, after)
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
	body := `{"kind":"invoice_issued","from_email":"from@example.com","to_email":"to@example.com","subject":"test","body":"test","attachment_ids":["` + req.AttachmentIDs[0].String() + `"]}`
	rec := httptest.NewRecorder()
	NewEmailHandler(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/emails", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}
