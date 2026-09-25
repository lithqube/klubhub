// Package event is the P1 events domain: venues, events, stages, lineup,
// timetable validation and the export pack data (JSON-LD, ICS).
// Plan: .claude/plans/promoter-p1-events.plan.md; UX: docs/design/promoter-p1-ux.md.
package event

import (
	"time"

	"github.com/google/uuid"
)

// Event statuses and the transitions between them.
const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusPublished = "published"
	StatusCancelled = "cancelled"
	StatusPostponed = "postponed"
)

// Location modes (UX decision 2).
const (
	LocationVenue    = "venue"     // venue shown and exported
	LocationCityOnly = "city_only" // city shown; venue never exported
	LocationSecret   = "secret"    // city shown; venue withheld until LocationRevealAt
)

// Event is one night / festival day.
type Event struct {
	ID                uuid.UUID  `json:"id"`
	Title             string     `json:"title"`
	Slug              string     `json:"slug"`
	Status            string     `json:"status"`
	Visibility        string     `json:"visibility"`
	PublishAt         *time.Time `json:"publish_at"`
	StartsAt          time.Time  `json:"starts_at"`
	EndsAt            time.Time  `json:"ends_at"`
	DoorsAt           *time.Time `json:"doors_at"`
	Timezone          string     `json:"timezone"`
	VenueID           *uuid.UUID `json:"venue_id"`
	City              string     `json:"city"`
	LocationMode      string     `json:"location_mode"`
	LocationRevealAt  *time.Time `json:"location_reveal_at"`
	MinAge            *int       `json:"min_age"`
	Genres            []string   `json:"genres"`
	DescriptionMD     string     `json:"description_md"`
	CostText          string     `json:"cost_text"`
	ExternalTicketURL *string    `json:"external_ticket_url"`
	Capacity          *int       `json:"capacity"`
	Version           int        `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Stage is a room / stage of one event.
type Stage struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	Position          int        `json:"position"`
	CurfewAt          *time.Time `json:"curfew_at"`
	ChangeoverMinutes int        `json:"changeover_minutes"`
}

// LineupEntry is one billed act; set times are optional until scheduled.
type LineupEntry struct {
	ID           uuid.UUID  `json:"id"`
	StageID      *uuid.UUID `json:"stage_id"`
	DisplayName  string     `json:"display_name"`
	ProfileURL   *string    `json:"profile_url"`
	BillingOrder int        `json:"billing_order"`
	B2BGroup     *int       `json:"b2b_group"`
	SetStart     *time.Time `json:"set_start"`
	SetEnd       *time.Time `json:"set_end"`
}

// Issue is a timetable finding. Errors block scheduling, publishing and
// exporting (UX decision 3); warnings only inform.
type Issue struct {
	Code     string      `json:"code"`
	Severity string      `json:"severity"`
	StageID  *uuid.UUID  `json:"stage_id,omitempty"`
	EntryIDs []uuid.UUID `json:"entry_ids,omitempty"`
	From     *time.Time  `json:"from,omitempty"`
	To       *time.Time  `json:"to,omitempty"`
	Minutes  int         `json:"minutes,omitempty"`
}

// Severities.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// HasErrors reports whether any issue blocks release.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}
