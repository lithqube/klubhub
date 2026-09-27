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
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
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

// Upcoming is an ended or running event not purged yet. Timezone is the
// event's IANA zone, for showing its dates the way the event does.
type Upcoming struct {
	EventID    uuid.UUID `json:"event_id"`
	Title      string    `json:"title"`
	Timezone   string    `json:"timezone"`
	EndsAt     time.Time `json:"ends_at"`
	PurgeAfter time.Time `json:"purge_after"`
}

// Recent is a purged event.
type Recent struct {
	EventID  uuid.UUID `json:"event_id"`
	Title    string    `json:"title"`
	Timezone string    `json:"timezone"`
	PurgedAt time.Time `json:"purged_at"`
	Trigger  string    `json:"trigger"`
	Counts   Counts    `json:"counts"`
}

// WouldPurge is an unpurged event that a shorter retention period makes
// due at once (the job erases it within the hour).
type WouldPurge struct {
	EventID uuid.UUID `json:"event_id"`
	Title   string    `json:"title"`
	EndsAt  time.Time `json:"ends_at"`
}

// Preview is GET /api/v1/org/retention/preview and the body of the 409
// retention_would_purge refusal: what saving that period would erase now.
type Preview struct {
	WouldPurge []WouldPurge `json:"would_purge"`
	Count      int          `json:"count"`
}

// WouldPurgeError refuses a shorter retention period that would erase
// ended events at once, unless the request confirms their exact count
// (409 retention_would_purge).
type WouldPurgeError struct{ Preview Preview }

func (e *WouldPurgeError) Error() string {
	return fmt.Sprintf("retention: shortening would purge %d events", e.Preview.Count)
}

// DeniedError is a policy refusal raised inside the service (the step-up
// of a confirmed shortening): 403 with the policy reason.
type DeniedError struct{ Reason string }

func (e *DeniedError) Error() string { return "retention: denied: " + e.Reason }

// NormalizeTitle is how a typed confirmation is compared with the event
// title: NFKC, dashes to '-', curly quotes to straight ones, lower case,
// runs of whitespace to one space, trimmed. The web client applies the
// same rules (utils/privacy.ts normalizeTitle).
func NormalizeTitle(s string) string {
	s = norm.NFKC.String(s)
	s = titleReplacer.Replace(s)
	s = strings.ToLower(s)
	return strings.Join(strings.Fields(s), " ")
}

// TitleMatches reports whether typed confirms title (after NormalizeTitle;
// an empty title never matches).
func TitleMatches(typed, title string) bool {
	want := NormalizeTitle(title)
	return want != "" && NormalizeTitle(typed) == want
}

var titleReplacer = strings.NewReplacer(
	// Hyphens, dashes and minus signs.
	"\u2010", "-", "\u2011", "-", "\u2012", "-", "\u2013", "-", "\u2014", "-", "\u2015", "-",
	"\u2212", "-", "\ufe58", "-", "\ufe63", "-", "\uff0d", "-",
	// Curly and low quotes.
	"\u2018", "'", "\u2019", "'", "\u201a", "'", "\u201b", "'", "\u2032", "'",
	"\u201c", `"`, "\u201d", `"`, "\u201e", `"`, "\u201f", `"`, "\u2033", `"`,
)

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
