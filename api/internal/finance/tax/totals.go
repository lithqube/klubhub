package tax

import (
	"math/big"
	"sort"
)

// Line is one invoice line for totals purposes.
type Line struct {
	// NetMinor is the line's pre-tax amount (quantity × unit) in minor units.
	NetMinor int64
	// TaxBps is the line's own rate; only used when ComputeTotals is called
	// with PerLineRates.
	TaxBps int64
}

// BreakdownRow is the tax for one rate.
type BreakdownRow struct {
	RateBps      int64 `json:"rate_bps"`
	TaxableMinor int64 `json:"taxable_minor"`
	TaxMinor     int64 `json:"tax_minor"`
}

// Totals are the computed invoice amounts.
type Totals struct {
	SubtotalMinor    int64
	TaxMinor         int64
	TotalMinor       int64
	WithholdingMinor int64
	NetPayableMinor  int64
	Breakdown        []BreakdownRow
	// LineTaxBps is the effective rate of each input line, in order; callers
	// persist it to invoice_lines.tax_bps.
	LineTaxBps []int64
}

// PerLineRates tells ComputeTotals to use each Line's own TaxBps instead of
// one invoice-wide rate (the per-line-tax extension point).
const PerLineRates int64 = -1

// ComputeTotals sums lines and applies tax and withholding.
//
// Rules (all integer arithmetic in minor units, no floats):
//   - Every line gets taxRateBps, unless taxRateBps is PerLineRates.
//   - Lines are grouped by rate; tax is computed once per rate group on the
//     group's taxable sum and rounded half-up (half away from zero), which is
//     the per-rate rounding EU invoices print in their VAT breakdown.
//   - tax = Σ group tax; total = subtotal + tax.
//   - withholding = round_half_up(subtotal × withholdingRateBps / 10000),
//     i.e. on the pre-VAT fee; net payable = total − withholding.
//   - Breakdown rows are sorted by rate ascending and include 0% groups.
func ComputeTotals(lines []Line, taxRateBps, withholdingRateBps int64) Totals {
	t := Totals{Breakdown: []BreakdownRow{}, LineTaxBps: make([]int64, len(lines))}
	taxable := map[int64]int64{}
	for i, l := range lines {
		rate := taxRateBps
		if taxRateBps == PerLineRates {
			rate = l.TaxBps
		}
		t.LineTaxBps[i] = rate
		taxable[rate] += l.NetMinor
		t.SubtotalMinor += l.NetMinor
	}
	rates := make([]int64, 0, len(taxable))
	for r := range taxable {
		rates = append(rates, r)
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i] < rates[j] })
	for _, r := range rates {
		tx := ApplyBps(taxable[r], r)
		t.Breakdown = append(t.Breakdown, BreakdownRow{RateBps: r, TaxableMinor: taxable[r], TaxMinor: tx})
		t.TaxMinor += tx
	}
	t.TotalMinor = t.SubtotalMinor + t.TaxMinor
	t.WithholdingMinor = ApplyBps(t.SubtotalMinor, withholdingRateBps)
	t.NetPayableMinor = t.TotalMinor - t.WithholdingMinor
	return t
}

// ApplyBps returns amount × bps / 10000 rounded half away from zero, using
// big integers so large amounts cannot overflow the intermediate product.
func ApplyBps(amount, bps int64) int64 {
	if amount == 0 || bps == 0 {
		return 0
	}
	p := new(big.Int).Mul(big.NewInt(amount), big.NewInt(bps))
	neg := p.Sign() < 0
	p.Abs(p)
	p.Add(p, big.NewInt(5000))
	p.Quo(p, big.NewInt(10000))
	if neg {
		p.Neg(p)
	}
	return p.Int64()
}
