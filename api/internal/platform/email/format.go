package email

import (
	"fmt"
	"strings"
	"time"
)

// FormatFee renders a fee range for the {{fee}} template variable. It is
// shared by both outreach directions so the two never drift.
func FormatFee(feeMin, feeMax *float64, currency *string) string {
	cur := ""
	if currency != nil && *currency != "" {
		cur = " " + *currency
	}
	switch {
	case feeMin != nil && feeMax != nil && *feeMin == *feeMax:
		return FormatMoney(*feeMin) + cur
	case feeMin != nil && feeMax != nil:
		return FormatMoney(*feeMin) + "–" + FormatMoney(*feeMax) + cur
	case feeMin != nil:
		return "from " + FormatMoney(*feeMin) + cur
	case feeMax != nil:
		return "up to " + FormatMoney(*feeMax) + cur
	}
	return ""
}

// FormatMoney renders an amount with up to two decimals and no trailing zeros.
func FormatMoney(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

// LocalMidnightIn returns local midnight of t's day in the named IANA
// timezone — the start of a daily-cap window. An unknown timezone falls back
// to UTC rather than failing the send.
func LocalMidnightIn(t time.Time, tzName string) time.Time {
	loc, err := time.LoadLocation(tzName)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}
