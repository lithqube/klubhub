// Package report is the P2.4 post-event report: read-only aggregates over
// an event's guests, allocations, imported tickets, door check-ins and door
// counters, plus the per-allocation list-back CSV.
// Contract: .claude/plans/promoter-p2-guests-door.plan.md, "P2.4 contract".
//
// The report JSON carries no names except allocation submitter labels
// (public class, as in P2.1). Undone check-ins and counters never count.
// Only the list-back export decrypts guest names, and it is audited as
// guestlist.export with counts only.
package report

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

// BucketSize is the width of one check-in curve bucket.
const BucketSize = 15 * time.Minute

// curveMargin bounds the curve around the event: device clocks can be
// wrong, so activity stamped more than this before the start or after the
// end is drawn in the first or last bucket instead of stretching the curve
// over days of empty buckets. Totals are unaffected.
const curveMargin = 24 * time.Hour

var (
	// ErrNotFound: the event or allocation does not exist in this tenant
	// (or the allocation belongs to another event).
	ErrNotFound = errors.New("report: not found")
)

// InvalidError reports a request parameter that failed validation.
type InvalidError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *InvalidError) Error() string { return "report: invalid " + e.Field + ": " + e.Problem }

// Event is the report header.
type Event struct {
	ID       uuid.UUID `json:"id"`
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Timezone string    `json:"timezone"`
	Capacity *int      `json:"capacity"`
}

// Totals are the event-wide KPIs.
type Totals struct {
	GuestsGoing     int        `json:"guests_going"`
	GuestsArrived   int        `json:"guests_arrived"`
	NoShowRate      *float64   `json:"no_show_rate"`
	HeadsExpected   int        `json:"heads_expected"`
	HeadsAdmitted   int        `json:"heads_admitted"`
	PlusOnesAllowed int        `json:"plus_ones_allowed"`
	PlusOnesUsed    int        `json:"plus_ones_used"`
	TicketsValid    int        `json:"tickets_valid"`
	TicketsScanned  int        `json:"tickets_scanned"`
	Walkups         int        `json:"walkups"`
	PeakOccupancy   int        `json:"peak_occupancy"`
	PeakAt          *time.Time `json:"peak_at"`
	Conflicts       int        `json:"conflicts"`
}

// ListRow is one guest list.
type ListRow struct {
	ListID          uuid.UUID `json:"list_id"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Going           int       `json:"going"`
	Arrived         int       `json:"arrived"`
	NoShowRate      *float64  `json:"no_show_rate"`
	HeadsExpected   int       `json:"heads_expected"`
	HeadsAdmitted   int       `json:"heads_admitted"`
	PlusOnesAllowed int       `json:"plus_ones_allowed"`
	PlusOnesUsed    int       `json:"plus_ones_used"`
}

// SubmitterRow is one allocation (a submitter's slice of a list).
type SubmitterRow struct {
	AllocationID  uuid.UUID `json:"allocation_id"`
	ListID        uuid.UUID `json:"list_id"`
	ListName      string    `json:"list_name"`
	ListType      string    `json:"list_type"`
	Submitter     string    `json:"submitter"`
	Quota         int       `json:"quota"`
	Going         int       `json:"going"`
	Arrived       int       `json:"arrived"`
	NoShowRate    *float64  `json:"no_show_rate"`
	HeadsAdmitted int       `json:"heads_admitted"`
	Revoked       bool      `json:"revoked"`
}

// TicketTypeRow is one imported ticket tier.
type TicketTypeRow struct {
	TicketTypeID uuid.UUID `json:"ticket_type_id"`
	Name         string    `json:"name"`
	Valid        int       `json:"valid"`
	Scanned      int       `json:"scanned"`
}

// Bucket is one 15-minute step of the check-in curve. Occupancy is the
// running value at the end of the bucket.
type Bucket struct {
	BucketStart time.Time `json:"bucket_start"`
	In          int       `json:"in"`
	Out         int       `json:"out"`
	Walkups     int       `json:"walkups"`
	Occupancy   int       `json:"occupancy"`
}

// Report is GET /api/v1/events/{eventID}/report.
type Report struct {
	Event         Event           `json:"event"`
	GeneratedAt   time.Time       `json:"generated_at"`
	Live          bool            `json:"live"`
	Totals        Totals          `json:"totals"`
	ByList        []ListRow       `json:"by_list"`
	BySubmitter   []SubmitterRow  `json:"by_submitter"`
	TicketsByType []TicketTypeRow `json:"tickets_by_type"`
	Curve         []Bucket        `json:"curve"`
}

// ------------------------------------------------------------ tallies ---

// guestTally aggregates guests. Arrived counts guests of any status with
// at least one live `in` head; GoingArrived counts only going guests and
// drives the no-show rate.
type guestTally struct {
	Going         int
	Arrived       int
	GoingArrived  int
	HeadsExpected int // Σ (1 + plus_n) of going guests
	HeadsAdmitted int // Σ live `in` counts
	PlusAllowed   int // Σ plus_n of going guests
	PlusUsed      int // Σ min(plus_n, heads − 1) of arrived guests
}

func (t *guestTally) add(o guestTally) {
	t.Going += o.Going
	t.Arrived += o.Arrived
	t.GoingArrived += o.GoingArrived
	t.HeadsExpected += o.HeadsExpected
	t.HeadsAdmitted += o.HeadsAdmitted
	t.PlusAllowed += o.PlusAllowed
	t.PlusUsed += o.PlusUsed
}

// guestFacts is one guest as the tally sees it.
type guestFacts struct {
	Status string
	PlusN  int
	Heads  int // Σ live `in` counts
}

// tallyGuest applies the contract definitions to one guest.
func tallyGuest(g guestFacts) guestTally {
	var t guestTally
	going := g.Status == "going"
	arrived := g.Heads >= 1
	t.HeadsAdmitted = g.Heads
	if going {
		t.Going = 1
		t.HeadsExpected = 1 + g.PlusN
		t.PlusAllowed = g.PlusN
	}
	if arrived {
		t.Arrived = 1
		t.PlusUsed = plusOnesUsed(g.PlusN, g.Heads)
		if going {
			t.GoingArrived = 1
		}
	}
	return t
}

// plusOnesUsed is min(plus_n, heads − 1) for an arrived guest, else 0.
func plusOnesUsed(plusN, heads int) int {
	if heads < 1 {
		return 0
	}
	return max(0, min(plusN, heads-1))
}

// noShowRate is going-not-arrived / going, rounded to 4 decimals; nil when
// nobody is going.
func noShowRate(going, goingArrived int) *float64 {
	if going <= 0 {
		return nil
	}
	r := math.Round(float64(going-goingArrived)/float64(going)*10000) / 10000
	return &r
}

// ------------------------------------------------------------- curve ---

// point is door activity summed over one minute (or any instant).
type point struct {
	At      time.Time
	In      int // check-in `in` heads + manual in
	Out     int // check-in `out` heads + manual out
	Walkups int
}

// bucketStart returns the start of the 15-minute bucket holding t, aligned
// to :00/:15/:30/:45 of the wall clock in loc. It subtracts the local
// remainder from the instant instead of rebuilding a wall time, so the
// repeated hour of a DST fall-back yields two distinct buckets and zones
// with :30 or :45 offsets align locally.
func bucketStart(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	rem := time.Duration(lt.Minute()%15)*time.Minute + time.Duration(lt.Second())*time.Second + time.Duration(lt.Nanosecond())
	return t.Add(-rem).UTC()
}

// nextBucket is the bucket after b. An offset change that is not a
// multiple of 15 minutes (historical zones) can shorten one bucket; the
// fallback keeps the series strictly increasing.
func nextBucket(b time.Time, loc *time.Location) time.Time {
	n := bucketStart(b.Add(BucketSize), loc)
	if !n.After(b) {
		n = b.Add(BucketSize)
	}
	return n
}

// clampPoints moves points outside [from, to] onto the nearest edge.
// Points must be sorted by At; the result stays sorted.
func clampPoints(ps []point, from, to time.Time) []point {
	out := make([]point, len(ps))
	for i, p := range ps {
		switch {
		case p.At.Before(from):
			p.At = from
		case p.At.After(to):
			p.At = to
		}
		out[i] = p
	}
	return out
}

// buildCurve buckets points (sorted by At) into 15-minute buckets in loc,
// from the first to the last activity, empty buckets included. Occupancy
// is the running Σ(in − out + walk-ups) at the end of each bucket.
func buildCurve(ps []point, loc *time.Location) []Bucket {
	out := []Bucket{}
	if len(ps) == 0 {
		return out
	}
	last := bucketStart(ps[len(ps)-1].At, loc)
	occ, i := 0, 0
	for b := bucketStart(ps[0].At, loc); !b.After(last); b = nextBucket(b, loc) {
		end := nextBucket(b, loc)
		bk := Bucket{BucketStart: b}
		for ; i < len(ps) && ps[i].At.Before(end); i++ {
			bk.In += ps[i].In
			bk.Out += ps[i].Out
			bk.Walkups += ps[i].Walkups
		}
		occ += bk.In - bk.Out + bk.Walkups
		bk.Occupancy = occ
		out = append(out, bk)
	}
	return out
}

// peak returns the highest occupancy of the curve and the start of the
// first bucket that reaches it; (0, nil) when occupancy never rises above 0.
func peak(curve []Bucket) (int, *time.Time) {
	best, at := 0, (*time.Time)(nil)
	for _, b := range curve {
		if b.Occupancy > best {
			best = b.Occupancy
			t := b.BucketStart
			at = &t
		}
	}
	return best, at
}
