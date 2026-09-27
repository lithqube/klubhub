package retention_test

import (
	"errors"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/promoter/retention"
)

func TestPurgeAfter(t *testing.T) {
	start := time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		end  *time.Time
		days int
		want time.Time
	}{
		{"default 30 days after the end", &end, 30, end.Add(30 * 24 * time.Hour)},
		{"one day", &end, 1, time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)},
		{"a year", &end, 365, end.Add(365 * 24 * time.Hour)},
		{"retention change moves it", &end, 90, end.Add(90 * 24 * time.Hour)},
		{"no end: start + 24 h", nil, 30, start.Add(24*time.Hour + 30*24*time.Hour)},
		{"zero end counts as none", &time.Time{}, 1, start.Add(48 * time.Hour)},
		// Whole 24-hour days: the Europe DST switch on 25 Oct does not shift it.
		{"across DST", ptr(time.Date(2026, 10, 24, 23, 0, 0, 0, time.UTC)), 2, time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := retention.PurgeAfter(start, c.end, c.days); !got.Equal(c.want) {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestPurgeAfterKeepsInstantAcrossZones(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata")
	}
	start := time.Date(2026, 3, 28, 23, 0, 0, 0, berlin)
	end := time.Date(2026, 3, 29, 6, 0, 0, 0, berlin) // after the spring-forward switch
	got := retention.PurgeAfter(start, &end, 30)
	if !got.Equal(end.Add(720*time.Hour)) || got.Location() != time.UTC {
		t.Fatalf("got %s", got)
	}
}

func TestDueBoundaries(t *testing.T) {
	at := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	if retention.Due(at, at.Add(-time.Nanosecond)) {
		t.Error("a nanosecond before purge_after is not due")
	}
	if !retention.Due(at, at) {
		t.Error("exactly purge_after is due (purge_after ≤ now)")
	}
	if !retention.Due(at, at.Add(time.Hour)) {
		t.Error("after purge_after is due")
	}
}

func TestEndedBoundaries(t *testing.T) {
	start := time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Hour)
	switch {
	case retention.Ended(start, &end, end.Add(-time.Second)):
		t.Error("running event counted as ended")
	case !retention.Ended(start, &end, end):
		t.Error("an event has ended at its end time")
	case retention.Ended(start, nil, start.Add(23*time.Hour)):
		t.Error("without an end, the event runs for 24 h")
	case !retention.Ended(start, nil, start.Add(24*time.Hour)):
		t.Error("without an end, the event ends 24 h after the start")
	}
}

func TestValidateDays(t *testing.T) {
	for _, d := range []int{1, 7, 30, 365} {
		if err := retention.ValidateDays(d); err != nil {
			t.Errorf("%d: %v", d, err)
		}
	}
	for _, d := range []int{-1, 0, 366, 10000} {
		var inv *retention.InvalidError
		if err := retention.ValidateDays(d); !errors.As(err, &inv) || inv.Field != "retention_days" {
			t.Errorf("%d: want invalid retention_days, got %v", d, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }
