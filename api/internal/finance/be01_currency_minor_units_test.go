package finance

// BE-01 regression: PGGigFeeProvider.GetGigFeeInfo and the derived draft
// subtotal must use currency-aware exponent conversion, not a flat ×100.
//
//   - 250.00 JPY → 250 minor units (zero-decimal currency)
//   - 250.00 BHD → 250000 minor units (three-decimal currency)
//   - 250.00 EUR / USD → 25012 minor units (two-decimal default)
//   - 250.50 JPY → ErrGigFeeMinorUnitPrecision (excess precision rejected)

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestBE01_GigFeeInfoCurrencyMinorUnits(t *testing.T) {
	requirePG(t)
	setBillingProfile(t, nil)
	prov := NewPGGigFeeProvider(testPool)
	svc := newTestInvoiceService()

	cases := []struct {
		currency  string
		feeAmount string // what we set gigs.fee_amount to
		wantMinor int64
	}{
		{"JPY", "250.00", 250},
		{"BHD", "250.00", 250000},
		{"EUR", "250.12", 25012},
		{"USD", "250.12", 25012},
	}
	for _, c := range cases {
		c := c
		t.Run(c.currency, func(t *testing.T) {
			id := insertTestGig(t, c.currency)
			if _, err := testPool.Exec(context.Background(),
				`UPDATE gigs SET fee_amount=$2 WHERE id=$1`, id, c.feeAmount); err != nil {
				t.Fatalf("seed fee_amount=%s: %v", c.feeAmount, err)
			}
			info, err := prov.GetGigFeeInfo(context.Background(), id)
			if err != nil {
				t.Fatalf("GetGigFeeInfo: %v", err)
			}
			if info.FeeMinor != c.wantMinor {
				t.Fatalf("info.FeeMinor currency=%s fee=%s got=%d want=%d",
					c.currency, c.feeAmount, info.FeeMinor, c.wantMinor)
			}
			draft := issuableDraft(t, svc, id, "BE01", nil)
			if draft.SubtotalMinor != c.wantMinor {
				t.Fatalf("draft.SubtotalMinor currency=%s fee=%s got=%d want=%d",
					c.currency, c.feeAmount, draft.SubtotalMinor, c.wantMinor)
			}
		})
	}
}

func TestBE01_GigFeeInfoRejectsExcessPrecision(t *testing.T) {
	requirePG(t)
	setBillingProfile(t, nil)
	prov := NewPGGigFeeProvider(testPool)

	id := insertTestGig(t, "JPY")
	if _, err := testPool.Exec(context.Background(),
		`UPDATE gigs SET fee_amount=$2 WHERE id=$1`, id, "250.50"); err != nil {
		t.Fatalf("seed fee_amount: %v", err)
	}
	_, err := prov.GetGigFeeInfo(context.Background(), id)
	if !errors.Is(err, ErrGigFeeMinorUnitPrecision) {
		t.Fatalf("want ErrGigFeeMinorUnitPrecision for ¥250.50 (JPY), got %v", err)
	}
}

// TestBE01_GigFeeToMinorChecksUnit ensures the shared conversion helper
// rejects more fractional digits than the currency allows.
func TestBE01_GigFeeToMinorChecksUnit(t *testing.T) {
	for _, c := range []struct {
		currency string
		value    string
		want     int64
	}{
		{"JPY", "250", 250},
		{"BHD", "250", 250000},
		{"EUR", "250.12", 25012},
	} {
		v, err := decimal.NewFromString(c.value)
		if err != nil {
			t.Fatal(err)
		}
		got, err := gigFeeToMinor(v, c.currency)
		if err != nil {
			t.Fatalf("%s %s: %v", c.currency, c.value, err)
		}
		if got != c.want {
			t.Fatalf("%s %s: got %d want %d", c.currency, c.value, got, c.want)
		}
	}
	v, _ := decimal.NewFromString("250.50")
	if _, err := gigFeeToMinor(v, "JPY"); !errors.Is(err, ErrGigFeeMinorUnitPrecision) {
		t.Fatalf("want ErrGigFeeMinorUnitPrecision, got %v", err)
	}
}
