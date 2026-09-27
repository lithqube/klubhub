// Package retention is P2.5 privacy and retention: each organisation keeps
// guest personal data for a retention period after an event ends (default
// 30 days, 1..365), then the event is anonymised in place. Every personal
// column and blind index of its guests, orders and ticket positions, and
// its allocations' submitter contacts, become NULL; the rows stay (marked
// purged_at) so the post-event report keeps its numbers. Door PINs of the
// event are deleted and its door sessions revoked.
//
// Contract: .claude/plans/promoter-p2-guests-door.plan.md, "P2.5 contract".
//
// The job runs in `promoter serve` (hourly and at start) and as
// `promoter purge [--dry-run]`. It visits tenants one by one through
// tenantdb.WithTenant and purges one event per transaction. After a purge,
// writes that would add personal data to the event are refused with
// ErrEventPurged (see EnsureNotPurged).
package retention

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultRetentionDays applies when an organisation has not chosen.
	DefaultRetentionDays = 30
	MinRetentionDays     = 1
	MaxRetentionDays     = 365
	// NoEndFallback: an event without an end is treated as ending this long
	// after it starts.
	NoEndFallback = 24 * time.Hour
	// listLimit caps the upcoming and recent lists of the settings view.
	listLimit = 20
)

// Triggers of a purge.
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

var (
	// ErrEventPurged: the event's personal data has been erased; writes that
	// would add personal data to it, and exports of it, are refused (409).
	ErrEventPurged = errors.New("retention: event purged")
	// ErrNotEnded: a manual purge of an event that has not ended (409).
	ErrNotEnded = errors.New("retention: event has not ended")
	// ErrNotFound: no such event in this tenant.
	ErrNotFound = errors.New("retention: not found")
)

// InvalidError reports a field that failed validation (422).
type InvalidError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *InvalidError) Error() string { return "retention: invalid " + e.Field + ": " + e.Problem }

// ValidateDays checks a retention period.
func ValidateDays(days int) error {
	if days < MinRetentionDays || days > MaxRetentionDays {
		return &InvalidError{Field: "retention_days", Problem: "between 1 and 365 days"}
	}
	return nil
}

// EndOf is when an event counts as ended: its end, or its start plus
// NoEndFallback when it has none.
func EndOf(startsAt time.Time, endsAt *time.Time) time.Time {
	if endsAt != nil && !endsAt.IsZero() {
		return endsAt.UTC()
	}
	return startsAt.Add(NoEndFallback).UTC()
}

// PurgeAfter is when an event becomes due: its end plus days × 24 h.
// Whole 24-hour days keep the moment independent of time zones and DST.
func PurgeAfter(startsAt time.Time, endsAt *time.Time, days int) time.Time {
	return EndOf(startsAt, endsAt).Add(time.Duration(days) * 24 * time.Hour)
}

// Due reports whether an event with this purge_after is due at now
// (purge_after ≤ now).
func Due(purgeAfter, now time.Time) bool { return !now.Before(purgeAfter) }

// Ended reports whether the event has ended at now (end ≤ now).
func Ended(startsAt time.Time, endsAt *time.Time, now time.Time) bool {
	return !now.Before(EndOf(startsAt, endsAt))
}

// Counts are the rows anonymised per table by one purge (door_pins:
// deleted). Stored in event_purges.counts and the audit entry.
type Counts struct {
	Guests           int `json:"guests"`
	Orders           int `json:"orders"`
	OrderPositions   int `json:"order_positions"`
	GuestAllocations int `json:"guest_allocations"`
	DoorPins         int `json:"door_pins"`
}

// Upcoming is an ended or running event not purged yet.
type Upcoming struct {
	EventID    uuid.UUID `json:"event_id"`
	Title      string    `json:"title"`
	EndsAt     time.Time `json:"ends_at"`
	PurgeAfter time.Time `json:"purge_after"`
}

// Recent is a purged event.
type Recent struct {
	EventID  uuid.UUID `json:"event_id"`
	Title    string    `json:"title"`
	PurgedAt time.Time `json:"purged_at"`
	Trigger  string    `json:"trigger"`
	Counts   Counts    `json:"counts"`
}

// Settings is GET/PUT /api/v1/org/retention.
type Settings struct {
	RetentionDays int        `json:"retention_days"`
	Upcoming      []Upcoming `json:"upcoming"`
	Recent        []Recent   `json:"recent"`
}

// Privacy is GET /api/v1/events/{eventID}/privacy. PersonalRows counts the
// event's guests and ticket positions that still hold personal data.
type Privacy struct {
	PurgeAfter    time.Time  `json:"purge_after"`
	PurgedAt      *time.Time `json:"purged_at"`
	RetentionDays int        `json:"retention_days"`
	PersonalRows  int        `json:"personal_rows"`
}

// PurgeResult is the outcome of purging one event.
type PurgeResult struct {
	TenantID uuid.UUID `json:"-"`
	EventID  uuid.UUID `json:"event_id"`
	PurgedAt time.Time `json:"purged_at"`
	Trigger  string    `json:"trigger"`
	Counts   Counts    `json:"counts"`
}

// DueEvent is an event the job would purge now (dry run).
type DueEvent struct {
	TenantID   uuid.UUID
	EventID    uuid.UUID
	Title      string
	EndsAt     time.Time
	PurgeAfter time.Time
}
