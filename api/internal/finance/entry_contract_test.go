package finance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type stubEntryService struct {
	entry        *Entry
	entries      []*Entry
	summary      map[string]EntryTotals
	profitLoss   map[string]ProfitLossTotals
	err          error
	summaryRange DateRange
}

func (s *stubEntryService) Create(context.Context, CreateEntryRequest) (*Entry, error) {
	return s.entry, s.err
}
func (s *stubEntryService) Get(context.Context, uuid.UUID) (*Entry, error) { return s.entry, s.err }
func (s *stubEntryService) List(context.Context, EntryFilter) ([]*Entry, error) {
	return s.entries, s.err
}
func (s *stubEntryService) Update(context.Context, uuid.UUID, UpdateEntryRequest) (*Entry, error) {
	return s.entry, s.err
}
func (s *stubEntryService) Delete(context.Context, uuid.UUID, time.Time) error { return s.err }
func (s *stubEntryService) Void(context.Context, uuid.UUID, time.Time) (*Entry, error) {
	return s.entry, s.err
}
func (s *stubEntryService) Summary(_ context.Context, dr DateRange) (map[string]EntryTotals, error) {
	s.summaryRange = dr
	return s.summary, s.err
}
func (s *stubEntryService) ProfitLoss(context.Context, ProfitLossFilter) (map[string]ProfitLossTotals, error) {
	return s.profitLoss, s.err
}
func (s *stubEntryService) GetPendingReconciliationByGig(context.Context, uuid.UUID) (*EntryReconciliation, error) {
	return nil, s.err
}
func (s *stubEntryService) GetPendingReconciliationByEntry(context.Context, uuid.UUID) (*EntryReconciliation, error) {
	return nil, s.err
}
func (s *stubEntryService) ResolveReconciliation(context.Context, uuid.UUID, ResolveReconciliationRequest) (*EntryReconciliation, error) {
	return nil, s.err
}

func TestEntryHandler_CreateAndListContracts(t *testing.T) {
	id := uuid.New()
	entry := &Entry{ID: id, Kind: EntryKindIncome, AmountMinor: 12500, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-02-01", Description: "Club", Status: EntryStatusActive}
	h := NewEntryHandler(&stubEntryService{entry: entry, entries: []*Entry{entry}})

	create := httptest.NewRecorder()
	h.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/finance/entries", strings.NewReader(`{"kind":"income","amount_minor":12500,"currency":"EUR","category":"gig_fee","entry_date":"2026-02-01","description":"Club"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var createEnv struct {
		Data Entry `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &createEnv); err != nil || createEnv.Data.ID != id {
		t.Fatalf("create envelope: %+v err=%v", createEnv, err)
	}

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/finance/entries?kind=income&currency=EUR", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listEnv struct {
		Data []Entry `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listEnv); err != nil || len(listEnv.Data) != 1 {
		t.Fatalf("list envelope: %+v err=%v", listEnv, err)
	}
}

func TestEntryHandler_ConflictAndRequestLimit(t *testing.T) {
	h := NewEntryHandler(&stubEntryService{err: ErrEntryConflict})
	id := uuid.New()
	body := `{"kind":"income","amount_minor":1,"currency":"EUR","category":"gig_fee","entry_date":"2026-01-01","description":"x","updated_at":"2026-01-01T00:00:00Z"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/finance/entries/"+id.String(), strings.NewReader(body)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	tooLarge := httptest.NewRecorder()
	h.ServeHTTP(tooLarge, httptest.NewRequest(http.MethodPost, "/api/v1/finance/entries", strings.NewReader(strings.Repeat("x", (1<<20)+1))))
	if tooLarge.Code != http.StatusBadRequest {
		t.Fatalf("large body status=%d body=%s", tooLarge.Code, tooLarge.Body.String())
	}
}

func TestEntryHandler_StrictJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	h := NewEntryHandler(&stubEntryService{entry: &Entry{ID: uuid.New()}})
	valid := `{"kind":"income","amount_minor":12500,"currency":"EUR","category":"gig_fee","entry_date":"2026-02-01","description":"Club"}`
	for _, tc := range []struct {
		name string
		body string
	}{
		{"unknown field", strings.TrimSuffix(valid, "}") + `,"unexpected":true}`},
		{"trailing garbage", valid + ` trailing`},
		{"second object", valid + `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/entries", strings.NewReader(tc.body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEntryHandler_RejectsNilRouteAndFilterUUIDs(t *testing.T) {
	h := NewEntryHandler(&stubEntryService{})
	for _, path := range []string{
		"/api/v1/finance/entries/00000000-0000-0000-0000-000000000000",
		"/api/v1/finance/entries?gig_id=00000000-0000-0000-0000-000000000000",
		"/api/v1/finance/profit-loss?scope=gig&gig_id=00000000-0000-0000-0000-000000000000",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestEntryHandler_MapsInactiveAndGeneratedRetryConflicts(t *testing.T) {
	id := uuid.New()
	for _, tc := range []struct {
		err      error
		wantCode string
	}{
		{ErrEntryInactive, "inactive"},
		{ErrGeneratedEntryConflict, "generated_entry_conflict"},
	} {
		h := NewEntryHandler(&stubEntryService{err: tc.err})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/finance/entries/"+id.String(), strings.NewReader(`{"kind":"income","amount_minor":1,"currency":"EUR","category":"gig_fee","entry_date":"2026-01-01","description":"x","updated_at":"2026-01-01T00:00:00Z"}`)))
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"error":"`+tc.wantCode+`"`) {
			t.Fatalf("err=%v status=%d body=%s", tc.err, rec.Code, rec.Body.String())
		}
	}
}

func TestEntryHandler_SummaryAndProfitLossStayPerCurrency(t *testing.T) {
	stub := &stubEntryService{
		summary: map[string]EntryTotals{
			"EUR": {Currency: "EUR", IncomeMinor: 1000, ExpenseMinor: 300},
			"USD": {Currency: "USD", IncomeMinor: 2000, ExpenseMinor: 500},
		},
		profitLoss: map[string]ProfitLossTotals{
			"EUR": {Currency: "EUR", IncomeMinor: 1000, ExpenseMinor: 300, ProfitLossMinor: 700},
			"USD": {Currency: "USD", IncomeMinor: 2000, ExpenseMinor: 500, ProfitLossMinor: 1500},
		},
	}
	h := NewEntryHandler(stub)
	for _, path := range []string{
		"/api/v1/finance/summary?scope=year&year=2026",
		"/api/v1/finance/profit-loss?scope=year&year=2026",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		var env struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if len(env.Data) != 2 || env.Data["EUR"] == nil || env.Data["USD"] == nil {
			t.Fatalf("currencies were aggregated: %s", rec.Body.String())
		}
	}
	if stub.summaryRange != (DateRange{From: "2026-01-01", To: "2027-01-01"}) {
		t.Fatalf("summary year range = %+v", stub.summaryRange)
	}
}

func TestEntryHandler_SummaryMonthUsesServerDerivedHalfOpenBoundary(t *testing.T) {
	stub := &stubEntryService{summary: map[string]EntryTotals{
		"EUR": {Currency: "EUR", IncomeMinor: 1000, ExpenseMinor: 200},
		"USD": {Currency: "USD", IncomeMinor: 3000, ExpenseMinor: 400},
	}}
	h := NewEntryHandler(stub)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/finance/summary?scope=month&year=2026&month=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if stub.summaryRange != (DateRange{From: "2026-02-01", To: "2026-03-01"}) {
		t.Fatalf("summary month range = %+v", stub.summaryRange)
	}
	var env struct {
		Data map[string]EntryTotals `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data) != 2 || env.Data["EUR"].IncomeMinor != 1000 || env.Data["USD"].ExpenseMinor != 400 {
		t.Fatalf("summary currencies = %+v", env.Data)
	}
}

type rejectingEntryRepository struct{}

func (rejectingEntryRepository) Create(context.Context, CreateEntryRequest) (*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) CreateGenerated(context.Context, GeneratedEntryInput) (*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) Get(context.Context, uuid.UUID) (*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) GetByGeneratedSource(context.Context, EntrySourceKind, uuid.UUID, EntryKind) (*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) List(context.Context, EntryFilter) ([]*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) ListByGig(context.Context, uuid.UUID) ([]*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) Update(context.Context, uuid.UUID, UpdateEntryRequest) (*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) Delete(context.Context, uuid.UUID, time.Time) error {
	return errors.New("repository must not be called")
}
func (rejectingEntryRepository) Void(context.Context, uuid.UUID, time.Time) (*Entry, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) Summary(context.Context, DateRange, *uuid.UUID) (map[string]EntryTotals, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) GetPendingReconciliationByGig(context.Context, uuid.UUID) (*EntryReconciliation, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) GetPendingReconciliationByEntry(context.Context, uuid.UUID) (*EntryReconciliation, error) {
	return nil, errors.New("repository must not be called")
}
func (rejectingEntryRepository) ResolveReconciliation(context.Context, uuid.UUID, ResolveReconciliationRequest) (*EntryReconciliation, error) {
	return nil, errors.New("repository must not be called")
}

func TestEntryHandler_MalformedCurrencyReturns400BeforeRepository(t *testing.T) {
	h := NewEntryHandler(NewEntryService(rejectingEntryRepository{}))
	id := uuid.New()
	requests := []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/finance/entries", strings.NewReader(`{"kind":"income","amount_minor":100,"currency":"12$","category":"custom","entry_date":"2026-02-01","description":"Fee"}`)),
		httptest.NewRequest(http.MethodPut, "/api/v1/finance/entries/"+id.String(), strings.NewReader(`{"kind":"income","amount_minor":100,"currency":"12$","category":"custom","entry_date":"2026-02-01","description":"Fee","updated_at":"2026-01-01T00:00:00Z"}`)),
		httptest.NewRequest(http.MethodGet, "/api/v1/finance/entries?currency=12%24", nil),
	}
	for _, req := range requests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d body=%s", req.Method, req.URL, rec.Code, rec.Body.String())
		}
	}
}

func TestEntryHandler_MapsNotFoundAndValidation(t *testing.T) {
	id := uuid.New()
	for _, tc := range []struct {
		err  error
		want int
	}{
		{ErrEntryNotFound, http.StatusNotFound},
		{EntryValidationErrors{{Field: "currency", Message: "invalid"}}, http.StatusBadRequest},
		{errors.New("db down"), http.StatusInternalServerError},
	} {
		h := NewEntryHandler(&stubEntryService{err: tc.err})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/finance/entries/"+id.String(), nil))
		if rec.Code != tc.want {
			t.Fatalf("err=%v status=%d body=%s", tc.err, rec.Code, rec.Body.String())
		}
	}
}
