package finance

import "testing"

// BE-05: parseCappedInt64 reads a non-negative Postgres aggregate. A negative
// value is malformed input (amount_minor is constrained positive), so it must
// be an error rather than a successfully parsed negative number.
func TestParseCappedInt64(t *testing.T) {
	const cap = MaxSummaryAggregateMinor
	cases := []struct {
		name     string
		raw      string
		want     int64
		overflow bool
		wantErr  bool
	}{
		{name: "empty is zero", raw: ""},
		{name: "zero", raw: "0"},
		{name: "plain value", raw: "150000", want: 150000},
		{name: "exactly the cap", raw: "9007199254740991", want: cap},
		{name: "above the cap overflows", raw: "9007199254740992", overflow: true},
		{name: "far above int64 overflows", raw: "99999999999999999999999", overflow: true},
		{name: "negative is rejected", raw: "-5", wantErr: true},
		{name: "negative zero is rejected", raw: "-0", wantErr: true},
		{name: "lone sign is rejected", raw: "-", wantErr: true},
		{name: "negative beyond the cap is rejected", raw: "-9007199254740992", wantErr: true},
		{name: "non-digit is rejected", raw: "12.5", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, overflow, err := parseCappedInt64(tc.raw, cap)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseCappedInt64(%q) err = %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got != tc.want || overflow != tc.overflow {
				t.Fatalf("parseCappedInt64(%q) = (%d, %v), want (%d, %v)", tc.raw, got, overflow, tc.want, tc.overflow)
			}
		})
	}
}
