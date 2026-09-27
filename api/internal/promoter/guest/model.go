// Package guest is the P2.1 guest-list domain: lists per event with entry
// terms, standing lists copied into new events, allocations per submitter
// (quota, +N, deadline, approval, revoke) and the guests themselves.
// Plan: .claude/plans/promoter-p2-guests-door.plan.md (P2.1).
//
// Guest names, contacts and notes are `personal` data (plan §13.4): sealed
// with the tenant DEK, with blind indexes for exact lookups. Lists are
// name-only by default; email and phone are refused unless the list
// collects contact details.
package guest

import (
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// List types (plan P2.1).
var ListTypes = []string{"artist", "promoter", "comp", "industry", "vip", "reduced", "crew"}

// Guest statuses. "checked_in" is derived from check-ins in P2.3.
const (
	StatusGoing    = "going"
	StatusPending  = "pending"
	StatusWaitlist = "waitlist"
	StatusInvited  = "invited"
	StatusDeclined = "declined"
)

// Statuses in tab order.
var Statuses = []string{StatusGoing, StatusPending, StatusWaitlist, StatusInvited, StatusDeclined}

// consumes reports whether a guest in this status holds allocation quota.
// Waitlisted and declined guests do not.
func consumes(status string) bool {
	return status == StatusGoing || status == StatusPending || status == StatusInvited
}

// Sources a guest row can come from.
const (
	SourceManual = "manual"
	SourcePaste  = "paste"
	// SourceDoor marks on-the-spot adds from the door (P2.3).
	SourceDoor = "door"
)

// Price modes of entry terms.
const (
	PriceFree    = "free"
	PriceReduced = "reduced"
)

// Limits.
const (
	MaxPlusN     = 10
	MaxBatch     = 500
	MaxPerEvent  = 5000
	MaxNameLen   = maxNameLen
	maxPerks     = 8
	maxPerkLen   = 24
	maxNameLen   = 120
	maxNoteLen   = 500
	maxEmailLen  = 254
	maxPhoneLen  = 40
	maxListName  = 80
	maxLabelLen  = 120
	maxContact   = 200
	maxQuota     = 1000
	maxPriceText = 60
)

// EntryTerms says how a list's guests get in. Event lists carry an absolute
// cutoff (CutoffAt); standing lists carry a local wall-clock time
// (CutoffLocal, "HH:MM") that becomes an absolute cutoff for each event.
type EntryTerms struct {
	PriceMode        string     `json:"price_mode"`
	ReducedPriceText string     `json:"reduced_price_text"`
	CutoffAt         *time.Time `json:"cutoff_at,omitempty"`
	CutoffLocal      *string    `json:"cutoff_local,omitempty"`
	Perks            []string   `json:"perks"`
}

var hhmmRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (t *EntryTerms) validate(template bool) error {
	if t.PriceMode == "" {
		t.PriceMode = PriceFree
	}
	t.ReducedPriceText = strings.TrimSpace(t.ReducedPriceText)
	switch {
	case t.PriceMode != PriceFree && t.PriceMode != PriceReduced:
		return invalid("entry_terms.price_mode", "free or reduced")
	case t.PriceMode == PriceReduced && t.ReducedPriceText == "":
		return invalid("entry_terms.reduced_price_text", "say what they pay, e.g. €10 before 01:00")
	case len(t.ReducedPriceText) > maxPriceText:
		return invalid("entry_terms.reduced_price_text", fmt.Sprintf("at most %d characters", maxPriceText))
	case template && t.CutoffAt != nil:
		return invalid("entry_terms.cutoff_at", "standing lists use a local cutoff time (cutoff_local)")
	case !template && t.CutoffLocal != nil:
		return invalid("entry_terms.cutoff_local", "event lists use an absolute cutoff (cutoff_at)")
	case t.CutoffLocal != nil && !hhmmRe.MatchString(*t.CutoffLocal):
		return invalid("entry_terms.cutoff_local", "HH:MM")
	case len(t.Perks) > maxPerks:
		return invalid("entry_terms.perks", fmt.Sprintf("at most %d", maxPerks))
	}
	if t.PriceMode == PriceFree {
		t.ReducedPriceText = ""
	}
	perks := []string{}
	for _, p := range t.Perks {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if len(p) > maxPerkLen {
			return invalid("entry_terms.perks", fmt.Sprintf("each at most %d characters", maxPerkLen))
		}
		if !slices.Contains(perks, p) {
			perks = append(perks, p)
		}
	}
	t.Perks = perks
	return nil
}

// CutoffFor turns a standing list's local cutoff ("01:00") into an instant
// for an event: the first occurrence of that wall-clock time in the event's
// timezone at or after 12 hours before the start. "01:00" for a 23:00 start
// is 01:00 the next morning; "22:30" is the same evening.
func CutoffFor(startsAt time.Time, timezone, hhmm string) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	var h, m int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil {
		return time.Time{}, err
	}
	from := startsAt.Add(-12 * time.Hour).In(loc)
	c := time.Date(from.Year(), from.Month(), from.Day(), h, m, 0, 0, loc)
	if c.Before(from) {
		c = time.Date(from.Year(), from.Month(), from.Day()+1, h, m, 0, 0, loc)
	}
	return c, nil
}

// ListInput creates or replaces an event list.
type ListInput struct {
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	EntryTerms     EntryTerms `json:"entry_terms"`
	CollectContact bool       `json:"collect_contact"`
}

func (in *ListInput) validate(template bool) error {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "" || len(in.Name) > maxListName:
		return invalid("name", fmt.Sprintf("required, at most %d characters", maxListName))
	case !slices.Contains(ListTypes, in.Type):
		return invalid("type", strings.Join(ListTypes, ", "))
	}
	return in.EntryTerms.validate(template)
}

// List is an event's guest list with its allocations and counts.
type List struct {
	ID                 uuid.UUID    `json:"id"`
	EventID            uuid.UUID    `json:"event_id"`
	Name               string       `json:"name"`
	Type               string       `json:"type"`
	EntryTerms         EntryTerms   `json:"entry_terms"`
	CollectContact     bool         `json:"collect_contact"`
	StandingTemplateID *uuid.UUID   `json:"standing_template_id"`
	Position           int          `json:"position"`
	Allocations        []Allocation `json:"allocations"`
	// Guests counts rows that are not declined; Heads adds their +N; Quota
	// sums active allocation quotas (0 = unlimited list).
	Guests  int `json:"guests"`
	Heads   int `json:"heads"`
	Pending int `json:"pending"`
	Quota   int `json:"quota"`
}

// StandingList is an organisation template copied into each new event.
type StandingList struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	EntryTerms     EntryTerms `json:"entry_terms"`
	CollectContact bool       `json:"collect_contact"`
	Position       int        `json:"position"`
}

// AllocationInput creates or replaces an allocation. SubmitterContact is
// sealed; nil leaves it unchanged on update, "" clears it.
type AllocationInput struct {
	Label            string     `json:"label"`
	SubmitterContact *string    `json:"submitter_contact"`
	Quota            int        `json:"quota"`
	PlusNMax         int        `json:"plus_n_max"`
	Deadline         *time.Time `json:"deadline"`
	RequiresApproval bool       `json:"requires_approval"`
}

func (in *AllocationInput) validate() error {
	in.Label = strings.TrimSpace(in.Label)
	switch {
	case in.Label == "" || len(in.Label) > maxLabelLen:
		return invalid("label", fmt.Sprintf("required, at most %d characters", maxLabelLen))
	case in.Quota < 1 || in.Quota > maxQuota:
		return invalid("quota", fmt.Sprintf("1 to %d", maxQuota))
	case in.PlusNMax < 0 || in.PlusNMax > MaxPlusN:
		return invalid("plus_n_max", fmt.Sprintf("0 to %d", MaxPlusN))
	case in.SubmitterContact != nil && len(strings.TrimSpace(*in.SubmitterContact)) > maxContact:
		return invalid("submitter_contact", fmt.Sprintf("at most %d characters", maxContact))
	}
	return nil
}

// Allocation is a submitter's share of a list.
type Allocation struct {
	ID               uuid.UUID  `json:"id"`
	ListID           uuid.UUID  `json:"list_id"`
	Label            string     `json:"label"`
	SubmitterContact string     `json:"submitter_contact"`
	Quota            int        `json:"quota"`
	PlusNMax         int        `json:"plus_n_max"`
	Deadline         *time.Time `json:"deadline"`
	RequiresApproval bool       `json:"requires_approval"`
	RevokedAt        *time.Time `json:"revoked_at"`
	// Used counts heads (guest + N) holding quota: going, pending, invited.
	Used    int `json:"used"`
	Guests  int `json:"guests"`
	Pending int `json:"pending"`
}

// GuestInput is one guest to add or the new state of one guest.
type GuestInput struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	Note   string `json:"note"`
	PlusN  int    `json:"plus_n"`
	Status string `json:"status"`
}

var phoneRe = regexp.MustCompile(`^\+?[0-9][0-9 ().-]{4,}$`)

// validate normalises g. field prefixes error fields ("guests[3].").
func (g *GuestInput) validate(field string, collectContact bool) error {
	g.Name = strings.Join(strings.Fields(g.Name), " ")
	g.Email = strings.TrimSpace(g.Email)
	g.Phone = strings.TrimSpace(g.Phone)
	g.Note = strings.TrimSpace(g.Note)
	switch {
	case g.Name == "" || len(g.Name) > maxNameLen:
		return invalid(field+"name", fmt.Sprintf("required, at most %d characters", maxNameLen))
	case g.PlusN < 0 || g.PlusN > MaxPlusN:
		return invalid(field+"plus_n", fmt.Sprintf("0 to %d", MaxPlusN))
	case len(g.Note) > maxNoteLen:
		return invalid(field+"note", fmt.Sprintf("at most %d characters", maxNoteLen))
	case g.Status != "" && !slices.Contains(Statuses, g.Status):
		return invalid(field+"status", strings.Join(Statuses, ", "))
	case !collectContact && g.Email != "":
		return invalid(field+"email", "this list is name-only; turn on contact details for the list first")
	case !collectContact && g.Phone != "":
		return invalid(field+"phone", "this list is name-only; turn on contact details for the list first")
	case len(g.Email) > maxEmailLen:
		return invalid(field+"email", "too long")
	case len(g.Phone) > maxPhoneLen || (g.Phone != "" && !phoneRe.MatchString(g.Phone)):
		return invalid(field+"phone", "digits, spaces and an optional leading +")
	}
	if g.Email != "" {
		a, err := mail.ParseAddress(g.Email)
		if err != nil || a.Address != g.Email || !strings.Contains(g.Email[strings.LastIndex(g.Email, "@"):], ".") {
			return invalid(field+"email", "not an email address")
		}
	}
	return nil
}

// Guest as the guest table shows it (decrypted for org members).
type Guest struct {
	ID           uuid.UUID  `json:"id"`
	ListID       uuid.UUID  `json:"list_id"`
	AllocationID *uuid.UUID `json:"allocation_id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Phone        string     `json:"phone"`
	Note         string     `json:"note"`
	PlusN        int        `json:"plus_n"`
	Status       string     `json:"status"`
	Source       string     `json:"source"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Counts are the status-tab counts of an event (or one list of it).
type Counts struct {
	All      int `json:"all"`
	Going    int `json:"going"`
	Pending  int `json:"pending"`
	Waitlist int `json:"waitlist"`
	Invited  int `json:"invited"`
	Declined int `json:"declined"`
	// GoingHeads adds the +N of going guests (the door's expected headcount).
	GoingHeads int `json:"going_heads"`
	// Tickets counts valid imported tickets (not on any list, so 0 when
	// the list filter is set).
	Tickets int `json:"tickets"`
}

func (c *Counts) add(status string, n, heads int) {
	c.All += n
	switch status {
	case StatusGoing:
		c.Going += n
		c.GoingHeads += heads
	case StatusPending:
		c.Pending += n
	case StatusWaitlist:
		c.Waitlist += n
	case StatusInvited:
		c.Invited += n
	case StatusDeclined:
		c.Declined += n
	}
}

// GuestPage is the guest table: matching guests plus tab counts. Tickets
// (imported attendees, P2.2) come along when no status or list filter is
// set, since they have neither.
type GuestPage struct {
	Guests  []Guest  `json:"guests"`
	Tickets []Ticket `json:"tickets"`
	Counts  Counts   `json:"counts"`
}

// Ticket is an imported ticket holder as the guest table shows it. The
// ticket secret is never listed; the door bundle (P2.3) carries it.
type Ticket struct {
	ID           uuid.UUID `json:"id"`
	OrderID      uuid.UUID `json:"order_id"`
	Source       string    `json:"source"`
	OrderRef     string    `json:"order_ref"`
	TicketTypeID uuid.UUID `json:"ticket_type_id"`
	TicketType   string    `json:"ticket_type"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	Status       string    `json:"status"`
	ImportedAt   time.Time `json:"imported_at"`
}

// GuestFilter narrows the guest table. Q is an exact lookup through the
// blind indexes (an email, or a full name); fuzzy search runs in the
// browser over the decrypted page.
type GuestFilter struct {
	Status string
	ListID *uuid.UUID
	Q      string
}

// AddInput adds guests to a list, optionally under an allocation.
type AddInput struct {
	ListID       uuid.UUID    `json:"list_id"`
	AllocationID *uuid.UUID   `json:"allocation_id"`
	Source       string       `json:"source"`
	Guests       []GuestInput `json:"guests"`
}

// AddResult reports what was added and which inputs were skipped as
// duplicates (same email in the event, or same name in the list).
type AddResult struct {
	Added      []Guest `json:"added"`
	Duplicates []int   `json:"duplicates"`
}

// BulkStatusInput changes the status of guests picked by id or by email.
type BulkStatusInput struct {
	Status   string      `json:"status"`
	GuestIDs []uuid.UUID `json:"guest_ids"`
	Emails   []string    `json:"emails"`
}

// BulkResult reports how many guests changed and which emails matched none.
type BulkResult struct {
	Matched   int      `json:"matched"`
	Updated   int      `json:"updated"`
	Unmatched []string `json:"unmatched"`
}

// OverviewRow is one upcoming event on the cross-event guests page.
type OverviewRow struct {
	EventID  uuid.UUID `json:"event_id"`
	Title    string    `json:"title"`
	Status   string    `json:"status"`
	StartsAt time.Time `json:"starts_at"`
	Timezone string    `json:"timezone"`
	Capacity *int      `json:"capacity"`
	Lists    int       `json:"lists"`
	Guests   int       `json:"guests"`
	// GoingHeads is the expected headcount; Used/Quota is allocation fill.
	GoingHeads int `json:"going_heads"`
	Pending    int `json:"pending"`
	Used       int `json:"used"`
	Quota      int `json:"quota"`
	// Tickets counts valid imported tickets.
	Tickets int `json:"tickets"`
}
