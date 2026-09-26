package event

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

var berlin, _ = time.LoadLocation("Europe/Berlin")

func at(day, hour, minute int) *time.Time {
	t := time.Date(2026, 10, day, hour, minute, 0, 0, berlin)
	return &t
}

func night() (Event, Stage, Stage) {
	ev := Event{StartsAt: *at(3, 23, 0), EndsAt: *at(4, 7, 0), DoorsAt: at(3, 22, 30), Timezone: "Europe/Berlin"}
	main := Stage{ID: uuid.New(), Name: "Main Room", Position: 0, CurfewAt: at(4, 5, 0), ChangeoverMinutes: 15}
	garden := Stage{ID: uuid.New(), Name: "Garden", Position: 1, ChangeoverMinutes: 10}
	return ev, main, garden
}

func set(stage Stage, name string, start, end *time.Time) LineupEntry {
	id := stage.ID
	return LineupEntry{ID: uuid.New(), StageID: &id, DisplayName: name, SetStart: start, SetEnd: end}
}

func codes(issues []Issue) map[string]int {
	m := map[string]int{}
	for _, i := range issues {
		m[i.Code]++
	}
	return m
}

func TestCleanOvernightTimetable(t *testing.T) {
	ev, main, garden := night()
	lineup := []LineupEntry{
		set(main, "Kaiser", at(3, 23, 0), at(4, 1, 0)),
		set(main, "Dasha Rush", at(4, 1, 15), at(4, 2, 45)),
		set(main, "Ben Klock", at(4, 3, 0), at(4, 5, 0)),
		set(garden, "Lena W", at(3, 23, 0), at(4, 1, 30)),
	}
	if issues := ValidateTimetable(ev, []Stage{main, garden}, lineup); len(issues) != 0 {
		t.Fatalf("clean overnight timetable flagged: %+v", issues)
	}
}

func TestErrorsBlockAndWarningsInform(t *testing.T) {
	ev, main, garden := night()
	orphan := uuid.New()
	lineup := []LineupEntry{
		set(main, "Dasha Rush", at(4, 0, 30), at(4, 2, 30)),
		set(main, "Ben Klock", at(4, 2, 0), at(4, 5, 30)), // overlap 30′ + past curfew 30′
		set(garden, "Early", at(3, 21, 0), at(3, 23, 0)),  // before doors
		set(garden, "A", at(4, 1, 0), at(4, 2, 0)),        // dead air after "Early"
		set(garden, "B", at(4, 2, 5), at(4, 3, 0)),        // short changeover (5 of 10)
		{ID: uuid.New(), DisplayName: "Untimed"},          // warning
		{ID: uuid.New(), StageID: &orphan, DisplayName: "Ghost", SetStart: at(4, 1, 0), SetEnd: at(4, 2, 0)},
	}
	issues := ValidateTimetable(ev, []Stage{main, garden}, lineup)
	c := codes(issues)
	for code, n := range map[string]int{"overlap": 1, "past_curfew": 1, "outside_event": 1, "dead_air": 1, "short_changeover": 1, "untimed": 1, "unknown_stage": 1} {
		if c[code] != n {
			t.Errorf("%s: got %d want %d (%v)", code, c[code], n, c)
		}
	}
	for _, i := range issues {
		if i.Code == "overlap" && i.Minutes != 30 {
			t.Errorf("overlap minutes = %d", i.Minutes)
		}
		if i.Code == "past_curfew" && i.Minutes != 30 {
			t.Errorf("curfew minutes = %d", i.Minutes)
		}
	}
	if !HasErrors(issues) {
		t.Fatal("errors must block release")
	}
	if HasErrors([]Issue{{Code: "dead_air", Severity: SeverityWarning}}) {
		t.Fatal("warnings must not block release")
	}
}

func TestB2BIsOneSlotAndMustMatch(t *testing.T) {
	ev, main, _ := night()
	g := 1
	a := set(main, "Nene H", at(4, 1, 0), at(4, 3, 0))
	b := set(main, "Rrose", at(4, 1, 0), at(4, 3, 0))
	a.B2BGroup, b.B2BGroup = &g, &g
	if c := codes(ValidateTimetable(ev, []Stage{main}, []LineupEntry{a, b})); c["overlap"] != 0 || c["b2b_mismatch"] != 0 {
		t.Fatalf("a b2b pair must not overlap itself: %v", c)
	}
	b.SetEnd = at(4, 3, 30)
	if c := codes(ValidateTimetable(ev, []Stage{main}, []LineupEntry{a, b})); c["b2b_mismatch"] != 1 {
		t.Fatalf("b2b partners with different times must be flagged: %v", c)
	}
}

func TestDSTNight(t *testing.T) {
	// 25 Oct 2026: clocks go back at 03:00 CEST → 02:00 CET. 23:00 → 07:00 is 9 real hours.
	ev := Event{StartsAt: time.Date(2026, 10, 24, 23, 0, 0, 0, berlin), EndsAt: time.Date(2026, 10, 25, 7, 0, 0, 0, berlin)}
	if h := ev.EndsAt.Sub(ev.StartsAt).Hours(); h != 9 {
		t.Fatalf("DST night length = %v h, want 9", h)
	}
	st := Stage{ID: uuid.New(), Position: 0}
	lineup := []LineupEntry{
		set(st, "A", ptr(time.Date(2026, 10, 24, 23, 0, 0, 0, berlin)), ptr(time.Date(2026, 10, 25, 3, 0, 0, 0, berlin))),
		set(st, "B", ptr(time.Date(2026, 10, 25, 3, 0, 0, 0, berlin)), ptr(time.Date(2026, 10, 25, 7, 0, 0, 0, berlin))),
	}
	if issues := ValidateTimetable(ev, []Stage{st}, lineup); len(issues) != 0 {
		t.Fatalf("absolute instants must make DST nights consistent: %+v", issues)
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestContainedSetOverlapsTheLongSetOnly(t *testing.T) {
	ev, main, _ := night()
	long := set(main, "Dasha Rush", at(4, 0, 0), at(4, 2, 0))
	inner := set(main, "Kaiser", at(4, 0, 15), at(4, 1, 0))
	after := set(main, "Ben Klock", at(4, 2, 15), at(4, 5, 0))
	issues := ValidateTimetable(ev, []Stage{main}, []LineupEntry{long, inner, after})
	c := codes(issues)
	if c["overlap"] != 1 || c["dead_air"] != 0 || len(issues) != 1 {
		t.Fatalf("a set inside a longer one is one overlap and no dead air: %+v", issues)
	}
	if issues[0].Minutes != 45 {
		t.Fatalf("overlap is the contained set's 45 min, got %d", issues[0].Minutes)
	}
}
