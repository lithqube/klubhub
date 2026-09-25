package event

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// slot is one timed performance on a stage; a b2b group is a single slot.
type slot struct {
	start, end time.Time
	entries    []uuid.UUID
}

// ValidateTimetable checks the lineup against the event window and each
// stage's curfew and changeover. It is the single source of truth: the UI
// mirrors it for instant feedback, but only this result gates release.
func ValidateTimetable(ev Event, stages []Stage, lineup []LineupEntry) []Issue {
	var issues []Issue
	byID := map[uuid.UUID]Stage{}
	for _, s := range stages {
		byID[s.ID] = s
	}
	windowStart := ev.StartsAt
	if ev.DoorsAt != nil {
		windowStart = *ev.DoorsAt
	}

	perStage := map[uuid.UUID][]slot{}
	type groupKey struct {
		stage uuid.UUID
		group int
	}
	groups := map[groupKey]*slot{}
	var order []groupKey

	for _, e := range lineup {
		if e.SetStart == nil || e.SetEnd == nil {
			issues = append(issues, Issue{Code: "untimed", Severity: SeverityWarning, EntryIDs: []uuid.UUID{e.ID}})
			continue
		}
		if e.StageID == nil {
			issues = append(issues, Issue{Code: "no_stage", Severity: SeverityError, EntryIDs: []uuid.UUID{e.ID}})
			continue
		}
		st, ok := byID[*e.StageID]
		if !ok {
			issues = append(issues, Issue{Code: "unknown_stage", Severity: SeverityError, EntryIDs: []uuid.UUID{e.ID}})
			continue
		}
		start, end := *e.SetStart, *e.SetEnd
		if start.Before(windowStart) || end.After(ev.EndsAt) {
			issues = append(issues, Issue{Code: "outside_event", Severity: SeverityError, StageID: &st.ID,
				EntryIDs: []uuid.UUID{e.ID}, From: &start, To: &end})
		}
		if st.CurfewAt != nil && end.After(*st.CurfewAt) {
			curfew := *st.CurfewAt
			issues = append(issues, Issue{Code: "past_curfew", Severity: SeverityError, StageID: &st.ID,
				EntryIDs: []uuid.UUID{e.ID}, From: &curfew, To: &end, Minutes: int(end.Sub(curfew).Minutes())})
		}
		if e.B2BGroup != nil {
			k := groupKey{st.ID, *e.B2BGroup}
			if g, ok := groups[k]; ok {
				if !g.start.Equal(start) || !g.end.Equal(end) {
					issues = append(issues, Issue{Code: "b2b_mismatch", Severity: SeverityError, StageID: &st.ID,
						EntryIDs: append(append([]uuid.UUID{}, g.entries...), e.ID)})
				}
				g.entries = append(g.entries, e.ID)
				continue
			}
			groups[k] = &slot{start: start, end: end, entries: []uuid.UUID{e.ID}}
			order = append(order, k)
			continue
		}
		perStage[st.ID] = append(perStage[st.ID], slot{start: start, end: end, entries: []uuid.UUID{e.ID}})
	}
	for _, k := range order {
		perStage[k.stage] = append(perStage[k.stage], *groups[k])
	}

	stageIDs := make([]uuid.UUID, 0, len(perStage))
	for id := range perStage {
		stageIDs = append(stageIDs, id)
	}
	sort.Slice(stageIDs, func(i, j int) bool { return byID[stageIDs[i]].Position < byID[stageIDs[j]].Position })

	for _, id := range stageIDs {
		st := byID[id]
		slots := perStage[id]
		sort.Slice(slots, func(i, j int) bool { return slots[i].start.Before(slots[j].start) })
		changeover := time.Duration(st.ChangeoverMinutes) * time.Minute
		for i := 1; i < len(slots); i++ {
			prev, next := slots[i-1], slots[i]
			stageID := st.ID
			entries := append(append([]uuid.UUID{}, prev.entries...), next.entries...)
			gap := next.start.Sub(prev.end)
			switch {
			case gap < 0:
				from, to := next.start, prev.end
				issues = append(issues, Issue{Code: "overlap", Severity: SeverityError, StageID: &stageID,
					EntryIDs: entries, From: &from, To: &to, Minutes: int(-gap.Minutes())})
			case gap < changeover:
				issues = append(issues, Issue{Code: "short_changeover", Severity: SeverityWarning, StageID: &stageID,
					EntryIDs: entries, Minutes: int(gap.Minutes())})
			case gap > changeover:
				from, to := prev.end.Add(changeover), next.start
				issues = append(issues, Issue{Code: "dead_air", Severity: SeverityWarning, StageID: &stageID,
					From: &from, To: &to, Minutes: int(to.Sub(from).Minutes())})
			}
		}
	}
	return issues
}
