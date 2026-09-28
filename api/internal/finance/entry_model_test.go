package finance

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateEntryRequest_AcceptsAnyNonEmptyCategory(t *testing.T) {
	tests := []struct {
		name string
		req  CreateEntryRequest
		ok   bool
	}{
		{"income category", CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-01-02", Description: "Club fee"}, true},
		{"expense category", CreateEntryRequest{Kind: EntryKindExpense, AmountMinor: 100, Currency: "EUR", Category: "equipment", EntryDate: "2026-01-02", Notes: "Cable"}, true},
		{"custom income category", CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "vinyl_sales", EntryDate: "2026-01-02", Description: "Records"}, true},
		{"custom expense category", CreateEntryRequest{Kind: EntryKindExpense, AmountMinor: 100, Currency: "EUR", Category: "stage_clothes", EntryDate: "2026-01-02", Notes: "Jacket"}, true},
		{"blank category", CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "   ", EntryDate: "2026-01-02", Description: "bad"}, false},
		{"lowercase currency", CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "eur", Category: "gig_fee", EntryDate: "2026-01-02", Description: "bad"}, false},
		{"non-letter currency", CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "12$", Category: "gig_fee", EntryDate: "2026-01-02", Description: "bad"}, false},
		{"zero amount", CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 0, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-01-02", Description: "bad"}, false},
		{"bad date", CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "gig_fee", EntryDate: "02-01-2026", Description: "bad"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateEntry(tt.req)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateEntryCurrency_AllInputPathsRequireThreeASCIIUppercaseLetters(t *testing.T) {
	validUpdate := UpdateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "custom", EntryDate: "2026-01-02", Description: "Fee", UpdatedAt: time.Now()}
	validGenerated := GeneratedEntryInput{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "custom", EntryDate: "2026-01-02", Description: "Fee", SourceKind: EntrySourceGigPayment, SourceID: uuid.New()}

	for _, currency := range []string{"12$", "ÉUR", "EU1", "EuR"} {
		t.Run("update_"+currency, func(t *testing.T) {
			req := validUpdate
			req.Currency = currency
			if err := ValidateUpdateEntry(req); err == nil {
				t.Fatalf("expected %q to be rejected", currency)
			}
		})
		t.Run("generated_"+currency, func(t *testing.T) {
			req := validGenerated
			req.Currency = currency
			if err := ValidateGeneratedEntry(req); err == nil {
				t.Fatalf("expected %q to be rejected", currency)
			}
		})
	}

	badSourceCurrency := "12$"
	generated := validGenerated
	generated.SourceCurrency = &badSourceCurrency
	if err := ValidateGeneratedEntry(generated); err == nil {
		t.Fatal("expected malformed source currency to be rejected")
	}
}

func TestValidateEntryGigID_RejectsNilUUIDAcrossInputs(t *testing.T) {
	nilID := uuid.Nil
	create := CreateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-01-02", Description: "Fee", GigID: &nilID}
	update := UpdateEntryRequest{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-01-02", Description: "Fee", GigID: &nilID, UpdatedAt: time.Now()}
	generated := GeneratedEntryInput{Kind: EntryKindIncome, AmountMinor: 100, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-01-02", Description: "Fee", GigID: &nilID, SourceKind: EntrySourceGigPayment, SourceID: uuid.New()}

	for name, err := range map[string]error{
		"create":    ValidateCreateEntry(create),
		"update":    ValidateUpdateEntry(update),
		"generated": ValidateGeneratedEntry(generated),
	} {
		if !errors.Is(err, ErrEntryValidation) {
			t.Errorf("%s error = %v, want validation error", name, err)
		}
	}
}

func TestScopeRange_UsesHalfOpenMonthAndYear(t *testing.T) {
	month, err := (ProfitLossFilter{Scope: ProfitLossScopeMonth, Year: 2026, Month: 2}).DateRange()
	if err != nil {
		t.Fatal(err)
	}
	if month.From != "2026-02-01" || month.To != "2026-03-01" {
		t.Fatalf("month range = %+v", month)
	}
	year, err := (ProfitLossFilter{Scope: ProfitLossScopeYear, Year: 2026}).DateRange()
	if err != nil {
		t.Fatal(err)
	}
	if year.From != "2026-01-01" || year.To != "2027-01-01" {
		t.Fatalf("year range = %+v", year)
	}
	if _, err := time.Parse("2006-01-02", month.To); err != nil {
		t.Fatalf("range is not an ISO date: %v", err)
	}
}
