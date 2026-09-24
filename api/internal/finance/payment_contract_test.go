package finance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakePaymentRepo is an in-memory replacement for *PaymentRepository used
// by contract tests.
type fakePaymentRepo struct {
	payments map[uuid.UUID]*Payment
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{
		payments: make(map[uuid.UUID]*Payment),
	}
}

func (f *fakePaymentRepo) Create(ctx context.Context, invoiceID uuid.UUID, req CreatePaymentRequest) (*Payment, error) {
	p := &Payment{
		ID:          uuid.New(),
		InvoiceID:   invoiceID,
		Currency:    req.Currency,
		AmountMinor: req.AmountMinor,
		Kind:        req.Kind,
		Status:      PaymentStatusPending,
		Method:      req.Method,
		Reference:   req.Reference,
		ReceivedAt:  req.ReceivedAt,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	f.payments[p.ID] = p
	return p, nil
}

func (f *fakePaymentRepo) GetByID(ctx context.Context, id uuid.UUID) (*Payment, error) {
	p, ok := f.payments[id]
	if !ok {
		return nil, ErrPaymentNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakePaymentRepo) ListByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]*Payment, error) {
	var out []*Payment
	for _, p := range f.payments {
		if p.InvoiceID == invoiceID {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakePaymentRepo) Update(ctx context.Context, id uuid.UUID, req UpdatePaymentRequest) (*Payment, error) {
	p, ok := f.payments[id]
	if !ok || !req.UpdatedAt.Equal(p.UpdatedAt) {
		return nil, ErrPaymentConflict
	}
	p.Status = req.Status
	p.Method = req.Method
	p.Reference = req.Reference
	p.ReceivedAt = req.ReceivedAt
	p.UpdatedAt = time.Now().UTC()
	return p, nil
}

func (f *fakePaymentRepo) SumByInvoice(ctx context.Context, invoiceID uuid.UUID) (int64, error) {
	var sum int64
	for _, p := range f.payments {
		if p.InvoiceID == invoiceID && p.Status == PaymentStatusCompleted {
			sum += p.AmountMinor
		}
	}
	return sum, nil
}

func validCreatePaymentRequest() CreatePaymentRequest {
	return CreatePaymentRequest{
		Currency:    "EUR",
		AmountMinor: 5000,
		Kind:        PaymentKindDeposit,
		Method:      "bank_transfer",
		Reference:   "ref-001",
	}
}

func validUpdatePaymentRequest(token time.Time) UpdatePaymentRequest {
	return UpdatePaymentRequest{
		Status:      PaymentStatusCompleted,
		Method:      "bank_transfer",
		Reference:   "ref-001",
		ReceivedAt:  &token,
		UpdatedAt:   token,
	}
}

func TestPaymentHandler_CreateEnvelope(t *testing.T) {
	repo := newFakePaymentRepo()
	h := NewPaymentHandler(NewPaymentService(repo))

	invoiceID := uuid.New()
	body, _ := json.Marshal(validCreatePaymentRequest())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+invoiceID.String()+"/payments", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct{ Data Payment }
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Data.ID == uuid.Nil {
		t.Fatal("missing payment id")
	}
	if env.Data.InvoiceID != invoiceID {
		t.Errorf("invoice_id: %v", env.Data.InvoiceID)
	}
	if env.Data.Status != PaymentStatusPending {
		t.Errorf("status: %v", env.Data.Status)
	}
	if env.Data.AmountMinor != 5000 {
		t.Errorf("amount_minor: %d", env.Data.AmountMinor)
	}
}

func TestPaymentHandler_GetEnvelope(t *testing.T) {
	repo := newFakePaymentRepo()
	h := NewPaymentHandler(NewPaymentService(repo))

	invoiceID := uuid.New()
	// Create via handler
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+invoiceID.String()+"/payments", strings.NewReader(`{"currency":"EUR","amount_minor":5000,"kind":"deposit"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)

	var created struct{ Data Payment }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Get it
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/finance/payments/"+created.Data.ID.String(), nil)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("status: %d", getRec.Code)
	}
	var env struct{ Data Payment }
	json.Unmarshal(getRec.Body.Bytes(), &env)
	if env.Data.ID != created.Data.ID {
		t.Errorf("id mismatch")
	}
}

func TestPaymentHandler_ListEnvelope(t *testing.T) {
	repo := newFakePaymentRepo()
	h := NewPaymentHandler(NewPaymentService(repo))

	invoiceID := uuid.New()
	// Create two payments
	for i := 0; i < 2; i++ {
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+invoiceID.String()+"/payments", strings.NewReader(`{"currency":"EUR","amount_minor":5000,"kind":"deposit"}`))
		createRec := httptest.NewRecorder()
		h.ServeHTTP(createRec, createReq)
		if createRec.Code != http.StatusCreated {
			t.Fatalf("create %d: %d", i, createRec.Code)
		}
	}

	// List
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices/"+invoiceID.String()+"/payments", nil)
	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("status: %d", listRec.Code)
	}
	var env struct{ Data []*Payment }
	json.Unmarshal(listRec.Body.Bytes(), &env)
	if len(env.Data) != 2 {
		t.Errorf("expected 2 payments, got %d", len(env.Data))
	}
}

func TestPaymentHandler_UpdateEnvelope(t *testing.T) {
	repo := newFakePaymentRepo()
	h := NewPaymentHandler(NewPaymentService(repo))

	invoiceID := uuid.New()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+invoiceID.String()+"/payments", strings.NewReader(`{"currency":"EUR","amount_minor":5000,"kind":"deposit"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	var created struct{ Data Payment }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Update
	body, _ := json.Marshal(validUpdatePaymentRequest(created.Data.UpdatedAt))
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/finance/payments/"+created.Data.ID.String(), strings.NewReader(string(body)))
	putRec := httptest.NewRecorder()
	h.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", putRec.Code, putRec.Body.String())
	}
	var env struct{ Data Payment }
	json.Unmarshal(putRec.Body.Bytes(), &env)
	if env.Data.Status != PaymentStatusCompleted {
		t.Errorf("status: %v", env.Data.Status)
	}
	if !env.Data.UpdatedAt.After(created.Data.UpdatedAt) {
		t.Errorf("updated_at did not advance")
	}
}

func TestPaymentHandler_ConcurrencyConflict(t *testing.T) {
	repo := newFakePaymentRepo()
	h := NewPaymentHandler(NewPaymentService(repo))

	invoiceID := uuid.New()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+invoiceID.String()+"/payments", strings.NewReader(`{"currency":"EUR","amount_minor":5000,"kind":"deposit"}`))
	createRec := httptest.NewRecorder()
	h.ServeHTTP(createRec, createReq)
	var created struct{ Data Payment }
	json.Unmarshal(createRec.Body.Bytes(), &created)

	// Update with stale token
	stale := created.Data.UpdatedAt.Add(-time.Hour)
	body, _ := json.Marshal(UpdatePaymentRequest{
		Status:     PaymentStatusCompleted,
		Method:     "bank_transfer",
		Reference:  "ref-001",
		ReceivedAt: &stale,
		UpdatedAt:  stale,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/finance/payments/"+created.Data.ID.String(), strings.NewReader(string(body)))
	putRec := httptest.NewRecorder()
	h.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusConflict {
		t.Fatalf("status: %d body=%s", putRec.Code, putRec.Body.String())
	}
}

func TestPaymentService_SumCompleted(t *testing.T) {
	repo := newFakePaymentRepo()
	svc := NewPaymentService(repo)

	invoiceID := uuid.New()
	// Create and complete a deposit
	p1, _ := repo.Create(context.Background(), invoiceID, CreatePaymentRequest{
		Currency:    "EUR",
		AmountMinor: 10000,
		Kind:        PaymentKindDeposit,
	})
	_, _ = repo.Update(context.Background(), p1.ID, UpdatePaymentRequest{
		Status:     PaymentStatusCompleted,
		Method:     "bank_transfer",
		Reference:  "ref-001",
		ReceivedAt: &p1.CreatedAt,
		UpdatedAt:  p1.UpdatedAt,
	})

	// Create a pending payment (should not count)
	_, _ = repo.Create(context.Background(), invoiceID, CreatePaymentRequest{
		Currency:    "EUR",
		AmountMinor: 5000,
		Kind:        PaymentKindPayment,
	})

	sum, err := svc.SumCompleted(context.Background(), invoiceID)
	if err != nil {
		t.Fatalf("SumCompleted: %v", err)
	}
	if sum != 10000 {
		t.Errorf("sum: %d expected 10000", sum)
	}
}

func TestPaymentValidation_RejectsBadInput(t *testing.T) {
	req := CreatePaymentRequest{
		Currency:    "eu", // lowercase
		AmountMinor: -100, // negative
		Kind:        PaymentKind("alien"),
		Method:      strings.Repeat("m", 51),
		Reference:   strings.Repeat("r", 201),
	}
	errs := ValidatePaymentRequest(&req)
	if len(errs) == 0 {
		t.Fatalf("expected validation errors")
	}
	for _, e := range errs {
		t.Logf("err: %s", e.Error())
	}
	if len(errs) < 4 {
		t.Errorf("expected at least 4 errors, got %d", len(errs))
	}
}