package finance

import (
	"testing"

	"github.com/shopspring/decimal"
)

// gigFeeToMinor must scale the gig fee according to the currency's
// minor-unit exponent and reject fractional precision beyond what the
// currency allows. The previous hardcoded /100 miscomputed JPY/KWD/BHD.
func TestGigFeeToMinor_ScalesByCurrencyDigits(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		currency string
		want     int64
		wantErr  bool
	}{
		{name: "EUR 1500", value: "1500", currency: "EUR", want: 150000},
		{name: "EUR 1500.50", value: "1500.50", currency: "EUR", want: 150050},
		{name: "USD 99.99", value: "99.99", currency: "USD", want: 9999},
		{name: "JPY 1500 no fractional", value: "1500", currency: "JPY", want: 1500},
		{name: "JPY 1500.5 rejected", value: "1500.5", currency: "JPY", wantErr: true},
		{name: "KWD 12.345 accepted", value: "12.345", currency: "KWD", want: 12345},
		{name: "KWD 12.3456 rejected", value: "12.3456", currency: "KWD", wantErr: true},
		{name: "BHD 0.001 accepted", value: "0.001", currency: "BHD", want: 1},
		{name: "BHD 0.0001 rejected", value: "0.0001", currency: "BHD", wantErr: true},
		{name: "zero rejected", value: "0", currency: "EUR", wantErr: true},
		{name: "negative rejected", value: "-10", currency: "EUR", wantErr: true},
		{name: "unknown currency falls back to 2 digits", value: "10", currency: "ZZZ", want: 1000},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gigFeeToMinor(decimal.RequireFromString(tt.value), tt.currency)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s/%s", tt.value, tt.currency)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}
