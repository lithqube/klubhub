package finance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/gig"
)

const be05JSONSafeMax int64 = 9007199254740991

func TestBE05_JSONSafeBoundaryAndRepositoryGuards(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)
	svc := NewEntryService(repo)
	req := CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: be05JSONSafeMax, Currency: "EUR", Category: "boundary", EntryDate: "2091-01-01", Description: "safe integer"}
	e, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("JSON safe boundary must be accepted: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM finance_entries WHERE entry_date='2091-01-01'`)
	})
	req.AmountMinor++
	if _, err := repo.Create(ctx, req); !errors.Is(err, ErrEntryValidation) {
		t.Errorf("repository create unsafe money: %v", err)
	}
	if _, err := repo.Update(ctx, e.ID, UpdateEntryRequest{Kind: req.Kind, AmountMinor: req.AmountMinor, Currency: req.Currency, Category: req.Category, EntryDate: req.EntryDate, Description: req.Description, UpdatedAt: e.UpdatedAt}); !errors.Is(err, ErrEntryValidation) {
		t.Errorf("repository update unsafe money: %v", err)
	}
	in := GeneratedEntryInput{Kind: req.Kind, AmountMinor: req.AmountMinor, Currency: req.Currency, Category: req.Category, EntryDate: req.EntryDate, Description: req.Description, SourceKind: EntrySourceGigPayment, SourceID: uuid.New()}
	if _, err := repo.CreateGenerated(ctx, in); !errors.Is(err, ErrEntryValidation) {
		t.Errorf("repository generated unsafe money: %v", err)
	}
	in.AmountMinor = 1
	source := be05JSONSafeMax + 1
	in.SourceAmountMinor = &source
	if _, err := svc.CreateGenerated(ctx, in); !errors.Is(err, ErrEntryValidation) {
		t.Errorf("unsafe source money: %v", err)
	}
	h := NewEntryHandler(svc)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/entries", strings.NewReader(`{"kind":"income","amount_minor":9007199254740992,"currency":"EUR","category":"test","entry_date":"2091-01-01","description":"unsafe"}`)))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"error":"validation_failed"`) {
		t.Errorf("unsafe JSON status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBE05_GeneratedGigHelperRejectsUnsafeMoney(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	snap := gig.PaymentSnapshot{GigID: insertTestGig(t, "EUR"), Date: time.Date(2094, 1, 1, 0, 0, 0, 0, time.UTC)}
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := createGeneratedGigIncome(ctx, tx, snap, be05JSONSafeMax+1, "EUR"); !errors.Is(err, ErrEntryValidation) {
		t.Fatalf("unsafe generated helper amount: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM finance_entries WHERE source_id=$1`, snap.GigID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unsafe helper persisted rows: %d err=%v", count, err)
	}
}

func TestBE05_AggregateOverflowWireContract(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewEntryRepository(testPool)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM finance_entries WHERE entry_date='2092-01-01'`)
	})
	// Legacy rows can exceed any new write bound. SUM must not scan-narrow,
	// truncate, or return partial currency results even beyond int64.
	for _, amt := range []int64{9223372036854775807, 9223372036854775807} {
		if _, err := testPool.Exec(ctx, `INSERT INTO finance_entries(kind,amount_minor,currency,category,entry_date,description,notes) VALUES('income',$1,'EUR','legacy','2092-01-01','legacy','')`, amt); err != nil {
			t.Fatal(err)
		}
	}
	totals, err := repo.Summary(ctx, DateRange{From: "2092-01-01", To: "2092-02-01"}, nil)
	if !errors.Is(err, ErrEntryAggregateOverflow) || totals != nil {
		t.Fatalf("overflow totals=%v err=%v", totals, err)
	}
	h := NewEntryHandler(NewEntryService(repo))
	for _, path := range []string{"summary?scope=year&year=2092", "profit-loss?scope=year&year=2092"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/finance/"+path, nil))
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"error":"validation_failed"`) || !strings.Contains(rec.Body.String(), "aggregate") {
			t.Errorf("%s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	// A one-connection pool proves overflow handling never self-deadlocks
	// by opening a second pool query while aggregate rows hold the first.
	cfg := testPool.Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	local, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	c, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := NewEntryRepository(local).Summary(c, DateRange{From: "2092-01-01", To: "2092-02-01"}, nil); !errors.Is(err, ErrEntryAggregateOverflow) {
		t.Fatalf("single-connection overflow: %v", err)
	}
	if _, err := NewEntryRepository(local).Summary(c, DateRange{From: "2093-01-01", To: "2093-02-01"}, nil); err != nil {
		t.Fatal(err)
	}
}
