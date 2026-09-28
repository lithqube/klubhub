package finance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func requireEntryPG(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	_, _ = testPool.Exec(context.Background(), `DELETE FROM finance_entries`)
}

func createEntryForTest(t *testing.T, svc *EntryService, req CreateEntryRequest) *Entry {
	t.Helper()
	e, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return e
}

func TestIntegration_EntryMigrationConstraints(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	bad := []struct{ name, sql string }{
		{"zero amount", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('income',0,'EUR','gig_fee','2026-01-01','x','','manual')`},
		{"lowercase currency", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('income',1,'eur','gig_fee','2026-01-01','x','','manual')`},
		{"bad kind", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('refund',1,'EUR','gig_fee','2026-01-01','x','','manual')`},
		{"blank category", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('income',1,'EUR','   ','2026-01-01','x','','manual')`},
		{"tab category", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('income',1,'EUR',E'	','2026-01-01','x','','manual')`},
		{"newline income description", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('income',1,'EUR','gig_fee','2026-01-01',E'\n','','manual')`},
		{"tab expense notes", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('expense',1,'EUR','travel','2026-01-01','',E'	','manual')`},
		{"generated without source", `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,auto_generated,source_kind) VALUES('income',1,'EUR','gig_fee','2026-01-01','x','',true,'gig_payment')`},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := testPool.Exec(ctx, tc.sql); err == nil {
				t.Fatal("expected constraint error")
			}
		})
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes,source_kind) VALUES('income',1,'EUR','vinyl_sales','2026-01-01','x','','manual')`); err != nil {
		t.Fatalf("custom category must be accepted: %v", err)
	}
}

func TestIntegration_VoidedManualEntryCannotBeUpdatedOrDeleted(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)
	svc := NewEntryService(repo)
	original := createEntryForTest(t, svc, CreateEntryRequest{Kind: EntryKindExpense, AmountMinor: 1900, Currency: "EUR", Category: "travel", EntryDate: "2026-05-01", Notes: "Original notes"})
	voided, err := svc.Void(ctx, original.ID, original.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Update(ctx, voided.ID, UpdateEntryRequest{Kind: EntryKindExpense, AmountMinor: 9999, Currency: "USD", Category: "equipment", EntryDate: "2026-05-02", Notes: "Changed", UpdatedAt: voided.UpdatedAt})
	if !errors.Is(err, ErrEntryInactive) {
		t.Fatalf("update error = %v, want inactive conflict", err)
	}
	if err := svc.Delete(ctx, voided.ID, voided.UpdatedAt); !errors.Is(err, ErrEntryInactive) {
		t.Fatalf("delete error = %v, want inactive conflict", err)
	}

	preserved, err := repo.GetIncludingInactive(ctx, voided.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Status != EntryStatusVoided || preserved.DeletedAt != nil || preserved.AmountMinor != original.AmountMinor || preserved.Currency != original.Currency || preserved.Category != original.Category || preserved.EntryDate != original.EntryDate || preserved.Notes != original.Notes {
		t.Fatalf("voided row changed: original=%+v preserved=%+v", original, preserved)
	}
}

func TestIntegration_EntryCRUDStaleDeleteVoidAndTotals(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	svc := NewEntryService(NewEntryRepository(testPool))
	income := createEntryForTest(t, svc, CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 10000, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-02-10", Description: "Fee"})
	expense := createEntryForTest(t, svc, CreateEntryRequest{Kind: EntryKindExpense, AmountMinor: 2500, Currency: "EUR", Category: "equipment", EntryDate: "2026-02-11", Notes: "Cable"})
	usd := createEntryForTest(t, svc, CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 7000, Currency: "USD", Category: "gig_fee", EntryDate: "2026-02-12", Description: "US fee"})

	updated, err := svc.Update(ctx, income.ID, UpdateEntryRequest{Kind: income.Kind, AmountMinor: 11000, Currency: income.Currency, Category: income.Category, EntryDate: income.EntryDate, Description: "Updated", UpdatedAt: income.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(ctx, income.ID, UpdateEntryRequest{Kind: income.Kind, AmountMinor: 12000, Currency: income.Currency, Category: income.Category, EntryDate: income.EntryDate, Description: "stale", UpdatedAt: income.UpdatedAt})
	if !errors.Is(err, ErrEntryConflict) {
		t.Fatalf("stale update err=%v", err)
	}

	deleteCandidate := createEntryForTest(t, svc, CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 3000, Currency: "EUR", Category: "custom_income", EntryDate: "2026-04-13", Description: "Delete candidate"})
	deleteCurrent, err := svc.Update(ctx, deleteCandidate.ID, UpdateEntryRequest{Kind: deleteCandidate.Kind, AmountMinor: deleteCandidate.AmountMinor, Currency: deleteCandidate.Currency, Category: deleteCandidate.Category, EntryDate: deleteCandidate.EntryDate, Description: "Changed", UpdatedAt: deleteCandidate.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, deleteCandidate.ID, deleteCandidate.UpdatedAt); !errors.Is(err, ErrEntryConflict) {
		t.Fatalf("stale delete err=%v", err)
	}
	preservedDelete, err := svc.Get(ctx, deleteCandidate.ID)
	if err != nil || preservedDelete.DeletedAt != nil || !preservedDelete.UpdatedAt.Equal(deleteCurrent.UpdatedAt) {
		t.Fatalf("stale delete changed row: %+v err=%v", preservedDelete, err)
	}

	voidCandidate := createEntryForTest(t, svc, CreateEntryRequest{Kind: EntryKindExpense, AmountMinor: 900, Currency: "EUR", Category: "custom_expense", EntryDate: "2026-04-14", Notes: "Void candidate"})
	voidCurrent, err := svc.Update(ctx, voidCandidate.ID, UpdateEntryRequest{Kind: voidCandidate.Kind, AmountMinor: voidCandidate.AmountMinor, Currency: voidCandidate.Currency, Category: voidCandidate.Category, EntryDate: voidCandidate.EntryDate, Notes: "Changed", UpdatedAt: voidCandidate.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Void(ctx, voidCandidate.ID, voidCandidate.UpdatedAt); !errors.Is(err, ErrEntryConflict) {
		t.Fatalf("stale void err=%v", err)
	}
	preservedVoid, err := svc.Get(ctx, voidCandidate.ID)
	if err != nil || preservedVoid.Status != EntryStatusActive || !preservedVoid.UpdatedAt.Equal(voidCurrent.UpdatedAt) {
		t.Fatalf("stale void changed row: %+v err=%v", preservedVoid, err)
	}

	if _, err := svc.Void(ctx, expense.ID, expense.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, usd.ID, usd.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	totals, err := svc.Summary(ctx, DateRange{From: "2026-02-01", To: "2026-03-01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 1 || totals["EUR"].IncomeMinor != updated.AmountMinor || totals["EUR"].ExpenseMinor != 0 {
		t.Fatalf("totals include voided/deleted or aggregate currencies: %+v", totals)
	}
	voided, err := NewEntryRepository(testPool).GetIncludingInactive(ctx, expense.ID)
	if err != nil || voided.AmountMinor != 2500 || voided.Status != EntryStatusVoided {
		t.Fatalf("void must preserve amount: %+v err=%v", voided, err)
	}
}

func TestIntegration_EntryMonthYearAndGigProfitLoss(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	var gigID uuid.UUID
	if err := testPool.QueryRow(ctx, `INSERT INTO gigs(date, fee_amount, fee_currency) VALUES('2026-02-01T20:00:00Z',100,'EUR') RETURNING id`).Scan(&gigID); err != nil {
		t.Fatal(err)
	}
	svc := NewEntryService(NewEntryRepository(testPool))
	for _, req := range []CreateEntryRequest{
		{Kind: EntryKindIncome, AmountMinor: 10000, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-02-01", Description: "fee", GigID: &gigID},
		{Kind: EntryKindExpense, AmountMinor: 3000, Currency: "EUR", Category: "travel", EntryDate: "2026-02-28", Notes: "train", GigID: &gigID},
		{Kind: EntryKindIncome, AmountMinor: 9000, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-03-01", Description: "next month", GigID: &gigID},
		{Kind: EntryKindIncome, AmountMinor: 5000, Currency: "USD", Category: "gig_fee", EntryDate: "2026-02-15", Description: "USD", GigID: &gigID},
	} {
		createEntryForTest(t, svc, req)
	}

	month, err := svc.ProfitLoss(ctx, ProfitLossFilter{Scope: ProfitLossScopeMonth, Year: 2026, Month: 2})
	if err != nil {
		t.Fatal(err)
	}
	if month["EUR"].ProfitLossMinor != 7000 || month["USD"].ProfitLossMinor != 5000 {
		t.Fatalf("month: %+v", month)
	}
	year, err := svc.ProfitLoss(ctx, ProfitLossFilter{Scope: ProfitLossScopeYear, Year: 2026})
	if err != nil {
		t.Fatal(err)
	}
	if year["EUR"].ProfitLossMinor != 16000 {
		t.Fatalf("year: %+v", year)
	}
	gig, err := svc.ProfitLoss(ctx, ProfitLossFilter{Scope: ProfitLossScopeGig, GigID: &gigID})
	if err != nil {
		t.Fatal(err)
	}
	if gig["EUR"].ProfitLossMinor != 16000 || gig["USD"].ProfitLossMinor != 5000 {
		t.Fatalf("gig: %+v", gig)
	}
}

func TestIntegration_GeneratedEntryIdempotencyConcurrent(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)
	sourceID := uuid.New()
	input := GeneratedEntryInput{Kind: EntryKindIncome, AmountMinor: 4200, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-04-01", Description: "payment", SourceKind: EntrySourceGigPayment, SourceID: sourceID}
	const n = 8
	ids := make(chan uuid.UUID, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, err := repo.CreateGenerated(ctx, input)
			if err != nil {
				errs <- err
				return
			}
			ids <- e.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Errorf("CreateGenerated: %v", err)
	}
	var first uuid.UUID
	for id := range ids {
		if first == uuid.Nil {
			first = id
		}
		if id != first {
			t.Fatalf("different ids: %s %s", first, id)
		}
	}
	found, err := repo.GetByGeneratedSource(ctx, EntrySourceGigPayment, sourceID, EntryKindIncome)
	if err != nil || found.ID != first {
		t.Fatalf("lookup=%+v err=%v", found, err)
	}
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM finance_entries WHERE source_kind='gig_payment' AND source_id=$1`, sourceID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestIntegration_GeneratedEntryRetryRequiresExactActiveIdentity(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)
	gigID := uuid.New()
	if _, err := testPool.Exec(ctx, `INSERT INTO gigs(id,date,fee_amount,fee_currency) VALUES($1,'2026-04-01T20:00:00Z',100,'EUR')`, gigID); err != nil {
		t.Fatal(err)
	}
	sourceAmount := int64(4200)
	sourceCurrency := "EUR"
	input := GeneratedEntryInput{Kind: EntryKindIncome, AmountMinor: 4200, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-04-01", Description: "payment", Notes: "source note", GigID: &gigID, SourceKind: EntrySourceGigPayment, SourceID: uuid.New(), SourceAmountMinor: &sourceAmount, SourceCurrency: &sourceCurrency, SourceDescription: "snapshot"}
	created, err := repo.CreateGenerated(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := repo.CreateGenerated(ctx, input)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("exact retry = %+v err=%v", retried, err)
	}

	otherGigID := uuid.New()
	if _, err := testPool.Exec(ctx, `INSERT INTO gigs(id,date,fee_amount,fee_currency) VALUES($1,'2026-04-02T20:00:00Z',100,'EUR')`, otherGigID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*GeneratedEntryInput)
	}{
		{"amount", func(in *GeneratedEntryInput) { in.AmountMinor++ }},
		{"currency", func(in *GeneratedEntryInput) { in.Currency = "USD" }},
		{"gig", func(in *GeneratedEntryInput) { in.GigID = &otherGigID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mismatch := input
			tc.mutate(&mismatch)
			if _, err := repo.CreateGenerated(ctx, mismatch); !errors.Is(err, ErrGeneratedEntryConflict) {
				t.Fatalf("error = %v, want generated conflict", err)
			}
		})
	}

	current, err := repo.GetByGeneratedSource(ctx, input.SourceKind, input.SourceID, input.Kind)
	if err != nil || current.ID != created.ID || current.AmountMinor != input.AmountMinor || current.Currency != input.Currency || current.GigID == nil || *current.GigID != gigID {
		t.Fatalf("mismatched retry changed row: %+v err=%v", current, err)
	}
	voided, err := repo.Void(ctx, created.ID, created.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateGenerated(ctx, input); !errors.Is(err, ErrGeneratedEntryConflict) {
		t.Fatalf("voided retry error = %v, want generated conflict", err)
	}
	preserved, err := repo.GetIncludingInactive(ctx, created.ID)
	if err != nil || preserved.Status != EntryStatusVoided || !preserved.UpdatedAt.Equal(voided.UpdatedAt) {
		t.Fatalf("voided generated row changed: %+v err=%v", preserved, err)
	}
}

func TestIntegration_EntryGigForeignKeyViolationIsValidationError(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)
	svc := NewEntryService(repo)
	missingGig := uuid.New()
	_, err := svc.Create(ctx, CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-06-01", Description: "Fee", GigID: &missingGig})
	if !errors.Is(err, ErrEntryValidation) {
		t.Fatalf("create error = %v, want validation error", err)
	}

	existing := createEntryForTest(t, svc, CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 200, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-06-02", Description: "Existing"})
	_, err = svc.Update(ctx, existing.ID, UpdateEntryRequest{Kind: existing.Kind, AmountMinor: existing.AmountMinor, Currency: existing.Currency, Category: existing.Category, EntryDate: existing.EntryDate, Description: existing.Description, GigID: &missingGig, UpdatedAt: existing.UpdatedAt})
	if !errors.Is(err, ErrEntryValidation) {
		t.Fatalf("update error = %v, want validation error", err)
	}
	generated := GeneratedEntryInput{Kind: EntryKindIncome, AmountMinor: 300, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-06-03", Description: "Generated", GigID: &missingGig, SourceKind: EntrySourceGigPayment, SourceID: uuid.New()}
	if _, err := svc.CreateGenerated(ctx, generated); !errors.Is(err, ErrEntryValidation) {
		t.Fatalf("generated error = %v, want validation error", err)
	}

	h := NewEntryHandler(svc)
	rec := httptest.NewRecorder()
	body := `{"kind":"income","amount_minor":100,"currency":"EUR","category":"gig_fee","entry_date":"2026-06-01","description":"Fee","gig_id":"` + missingGig.String() + `"}`
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/entries", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error":"validation_failed"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEntryUniqueViolationHelperStillRecognizesPostgres(t *testing.T) {
	if !isUniqueViolation(&pgconn.PgError{Code: pgUniqueViolation}) {
		t.Fatal("expected unique violation")
	}
}
