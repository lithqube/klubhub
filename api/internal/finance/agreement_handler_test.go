package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// /api/v1/finance/agreements/templates
// ---------------------------------------------------------------------------

func TestAgreementTemplateHandler_CreateEnvelope(t *testing.T) {
	repo := newFakeTemplateRepo()
	h := NewAgreementTemplateHandler(NewAgreementTemplateService(repo))

	body, _ := json.Marshal(CreateAgreementTemplateRequest{
		Name:        "Standard Performance",
		Description: "Default",
		ContentMD:   "# Performance Agreement\n\nFor {{gig.client_name}}.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/agreements/templates", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct{ Data AgreementTemplate }
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Data.ID == uuid.Nil {
		t.Fatal("missing id")
	}
	if env.Data.Version != 1 {
		t.Errorf("version: %d", env.Data.Version)
	}
}

func TestAgreementTemplateHandler_GetEnvelope(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)
	h := NewAgreementTemplateHandler(svc)

	tpl, _ := svc.Create(context.Background(), CreateAgreementTemplateRequest{
		Name: "Test", ContentMD: "# x",
	})

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/finance/agreements/templates/"+tpl.ID.String(), nil)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("status: %d", getRec.Code)
	}
	var env struct{ Data AgreementTemplate }
	json.Unmarshal(getRec.Body.Bytes(), &env)
	if env.Data.ID != tpl.ID {
		t.Errorf("id mismatch")
	}
}

func TestAgreementTemplateHandler_ListEnvelope(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)
	h := NewAgreementTemplateHandler(svc)

	for i := 0; i < 3; i++ {
		svc.Create(context.Background(), CreateAgreementTemplateRequest{
			Name:      fmt.Sprintf("Tpl %d", i),
			ContentMD: fmt.Sprintf("# v%d", i),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/finance/agreements/templates", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var env struct{ Data []*AgreementTemplate }
	json.Unmarshal(rec.Body.Bytes(), &env)
	if len(env.Data) != 3 {
		t.Errorf("expected 3, got %d", len(env.Data))
	}
}

func TestAgreementTemplateHandler_UpdateEnvelope(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)
	h := NewAgreementTemplateHandler(svc)

	tpl, _ := svc.Create(context.Background(), CreateAgreementTemplateRequest{
		Name: "Test", ContentMD: "# v1",
	})

	body, _ := json.Marshal(UpdateAgreementTemplateRequest{
		Name: "Test", ContentMD: "# v2", IsActive: true, UpdatedAt: tpl.UpdatedAt,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/finance/agreements/templates/"+tpl.ID.String(), bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgreementTemplateHandler_ConcurrencyConflict(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)
	h := NewAgreementTemplateHandler(svc)

	tpl, _ := svc.Create(context.Background(), CreateAgreementTemplateRequest{
		Name: "Test", ContentMD: "# x",
	})

	// Stale token
	stale := tpl.UpdatedAt.Add(-time.Hour)
	body, _ := json.Marshal(UpdateAgreementTemplateRequest{
		Name: "Test", ContentMD: "# y", IsActive: true, UpdatedAt: stale,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/finance/agreements/templates/"+tpl.ID.String(), bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: %d", rec.Code)
	}
}

func TestAgreementTemplateHandler_ValidationFailure(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)
	h := NewAgreementTemplateHandler(svc)

	body, _ := json.Marshal(CreateAgreementTemplateRequest{Name: "", ContentMD: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/agreements/templates", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// /api/v1/finance/agreements/instances
// ---------------------------------------------------------------------------

func TestAgreementInstanceHandler_CreateEnvelope(t *testing.T) {
	tplRepo := newFakeTemplateRepo()
	instRepo := newFakeInstanceRepo()
	tplSvc := NewAgreementTemplateService(tplRepo)
	tpl, _ := tplSvc.Create(context.Background(), CreateAgreementTemplateRequest{
		Name: "Test", ContentMD: "Agreement for {{gig.client_name}}",
	})
	instSvc := NewAgreementInstanceService(instRepo, tplRepo, nil)
	h := NewAgreementInstanceHandler(instSvc)

	body, _ := json.Marshal(CreateAgreementInstanceRequest{
		TemplateID:      tpl.ID,
		GigID:           uuid.New(),
		RequiredSigners: []string{"dj", "client"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/agreements/instances", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct{ Data AgreementInstance }
	json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.ID == uuid.Nil {
		t.Fatal("missing id")
	}
	if env.Data.Status != AgreementStatusDraft {
		t.Errorf("status: %s", env.Data.Status)
	}
}

func TestAgreementInstanceHandler_SignEnvelope(t *testing.T) {
	tplRepo := newFakeTemplateRepo()
	instRepo := newFakeInstanceRepo()
	tplSvc := NewAgreementTemplateService(tplRepo)
	tpl, _ := tplSvc.Create(context.Background(), CreateAgreementTemplateRequest{
		Name: "Test", ContentMD: "x",
	})
	instSvc := NewAgreementInstanceService(instRepo, tplRepo, nil)
	h := NewAgreementInstanceHandler(instSvc)

	// Create instance
	createBody, _ := json.Marshal(CreateAgreementInstanceRequest{TemplateID: tpl.ID, GigID: uuid.New()})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/agreements/instances", bytes.NewReader(createBody))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	var created struct{ Data AgreementInstance }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// DJ signs
	signBody, _ := json.Marshal(SignAgreementRequest{SignerRole: "dj", SignedBy: "dj-user", UpdatedAt: created.Data.UpdatedAt})
	signReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/agreements/instances/"+created.Data.ID.String()+"/sign", bytes.NewReader(signBody))
	signRec := httptest.NewRecorder()
	h.ServeHTTP(signRec, signReq)

	if signRec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", signRec.Code, signRec.Body.String())
	}
	var env struct{ Data AgreementInstance }
	json.Unmarshal(signRec.Body.Bytes(), &env)
	if env.Data.Status != AgreementStatusSigned {
		t.Errorf("status: %s", env.Data.Status)
	}
}

// ---------------------------------------------------------------------------
// /api/v1/finance/emails
// ---------------------------------------------------------------------------

func TestEmailHandler_SendEnvelope(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)
	h := NewEmailHandler(svc)

	ownerID := uuid.New()
	body, _ := json.Marshal(CreateEmailRequest{
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "a@b.com",
		ToEmail:   "c@d.com",
		Subject:   "test",
		Body:      "hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/emails", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmailHandler_GetEnvelope(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)
	h := NewEmailHandler(svc)

	// Seed an email
	msg, _ := svc.Enqueue(context.Background(), CreateEmailRequest{
		Kind:      EmailKindInvoiceIssued,
		FromEmail: "a@b.com",
		ToEmail:   "c@d.com",
		Subject:   "x",
		Body:      "y",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/finance/emails/"+msg.ID.String(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var env struct{ Data EmailMessage }
	json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.ID != msg.ID {
		t.Errorf("id mismatch")
	}
}

func TestEmailHandler_SendFailsReturnsBadGateway(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	sender.failNext = true
	sender.err = errors.New("smtp connection refused")
	svc := NewEmailService(repo, sender)
	h := NewEmailHandler(svc)

	body, _ := json.Marshal(CreateEmailRequest{
		Kind:      EmailKindInvoiceIssued,
		FromEmail: "a@b.com",
		ToEmail:   "c@d.com",
		Subject:   "x",
		Body:      "y",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/emails", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmailHandler_ValidationFailure(t *testing.T) {
	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender)
	h := NewEmailHandler(svc)

	body, _ := json.Marshal(CreateEmailRequest{Kind: "invalid"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/emails", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
}

// Silence unused-import warning when strconv is referenced later.
var _ = strings.HasPrefix
var _ = io.Discard

func TestInvoiceHandler_SummariesEndpoint(t *testing.T) {
	repo := newFakeInvoiceRepo()
	svc := NewInvoiceService(repo, newFakeBillingService(), newFakeGigFeeProvider())
	h := NewInvoiceHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices/summaries", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data []CurrencySummary
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}