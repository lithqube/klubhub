// Package door is the P2.3 offline door: the bundle a door device downloads
// (the whole list of one event, decrypted, name-only), nonce-idempotent
// check-in sync and on-the-spot adds gated by the manager PIN.
// Contract: .claude/plans/promoter-p2-guests-door.plan.md, "P2.3 contract".
//
// Every call is made by a door session (authz.Principal with an EventScope)
// and acts on that event only: routes take the event from the session,
// never from the URL, and every subject a device sends is checked against
// it. Check-ins from different devices for the same guest are all stored;
// the later one points at the earlier one (conflict_of) so the door can flag
// a double entry after sync.
package door

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Limits (P2.3 contract).
const (
	MaxOps      = 500
	MaxAdds     = 50
	maxCount    = 50
	minNonceLen = 8
	maxNonceLen = 64
	maxPINLen   = 32
)

// Op types, subject kinds, directions and counter kinds.
const (
	OpCheckin = "checkin"
	OpUndo    = "undo"
	OpCounter = "counter"

	KindGuest  = "guest"
	KindTicket = "ticket"

	DirIn  = "in"
	DirOut = "out"

	CounterWalkup = "walkup"
	CounterIn     = "in"
	CounterOut    = "out"
)

// Result statuses.
const (
	StatusApplied   = "applied"
	StatusDuplicate = "duplicate"
	StatusRejected  = "rejected"
)

// Per-item error codes. Structural problems (bad JSON, too many ops, a bad
// cursor) fail the whole request instead.
const (
	ErrCodeInvalidOp         = "invalid_op"
	ErrCodeUnknownSubject    = "unknown_subject"
	ErrCodeUnknownTarget     = "unknown_target"
	ErrCodeInvalidAdd        = "invalid_add"
	ErrCodeUnknownList       = "unknown_list"
	ErrCodeIDConflict        = "id_conflict"
	ErrCodeManagerPINInvalid = "manager_pin_invalid"
	ErrCodeEventFull         = "event_full"
)

var (
	// ErrNoSession: the caller is not a door session (staff, or a door
	// principal without an event scope).
	ErrNoSession = errors.New("door: door session required")
	// ErrNotFound: the session's event no longer exists.
	ErrNotFound = errors.New("door: event not found")
)

// InvalidError reports a request-level validation failure.
type InvalidError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *InvalidError) Error() string { return "door: invalid " + e.Field + ": " + e.Problem }

func invalid(field, problem string) error { return &InvalidError{Field: field, Problem: problem} }

// Session is the verified door principal: tenant, event, device.
type Session struct {
	Tenant uuid.UUID
	Event  uuid.UUID
	Device uuid.UUID
	Sub    string
}

// Subject is what a check-in is about: a guest or an imported ticket.
type Subject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Op is one queued door action. Fields are loosely typed on purpose: a
// malformed op is rejected on its own instead of failing the whole batch,
// so one corrupt entry cannot block a device's queue forever.
type Op struct {
	Nonce     string   `json:"nonce"`
	Type      string   `json:"type"`
	Subject   *Subject `json:"subject,omitempty"`
	Count     int      `json:"count,omitempty"`
	Direction string   `json:"direction,omitempty"`
	Target    string   `json:"target,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Delta     int      `json:"delta,omitempty"`
	At        string   `json:"at"`
}

// parsedOp is a validated Op.
type parsedOp struct {
	Op
	at        time.Time
	subjectID uuid.UUID
}

func validNonce(n string) bool {
	if len(n) < minNonceLen || len(n) > maxNonceLen {
		return false
	}
	for i := 0; i < len(n); i++ {
		if n[i] < 0x21 || n[i] > 0x7e {
			return false
		}
	}
	return true
}

func parseAt(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil && !t.IsZero()
}

// parse validates o and returns it with typed fields, or a problem.
func (o Op) parse() (parsedOp, string) {
	p := parsedOp{Op: o}
	if !validNonce(o.Nonce) {
		return p, "nonce: 8 to 64 visible ASCII characters"
	}
	var ok bool
	if p.at, ok = parseAt(o.At); !ok {
		return p, "at: an RFC 3339 time"
	}
	switch o.Type {
	case OpCheckin:
		switch {
		case o.Subject == nil || (o.Subject.Kind != KindGuest && o.Subject.Kind != KindTicket):
			return p, "subject.kind: guest or ticket"
		case o.Count < 1 || o.Count > maxCount:
			return p, "count: 1 to 50"
		case o.Direction != DirIn && o.Direction != DirOut:
			return p, "direction: in or out"
		case o.Target != "" || o.Kind != "" || o.Delta != 0:
			return p, "checkin ops carry subject, count, direction and at only"
		}
		id, err := uuid.Parse(o.Subject.ID)
		if err != nil || id == uuid.Nil {
			return p, "subject.id: not an id"
		}
		p.subjectID = id
	case OpUndo:
		switch {
		case !validNonce(o.Target):
			return p, "target: the nonce of a check-in or counter op"
		case o.Target == o.Nonce:
			return p, "target: an op cannot undo itself"
		case o.Subject != nil || o.Count != 0 || o.Direction != "" || o.Kind != "" || o.Delta != 0:
			return p, "undo ops carry target and at only"
		}
	case OpCounter:
		switch {
		case o.Kind != CounterWalkup && o.Kind != CounterIn && o.Kind != CounterOut:
			return p, "kind: walkup, in or out"
		case o.Delta < 1 || o.Delta > maxCount:
			return p, "delta: 1 to 50"
		case o.Subject != nil || o.Count != 0 || o.Direction != "" || o.Target != "":
			return p, "counter ops carry kind, delta and at only"
		}
	default:
		return p, "type: checkin, undo or counter"
	}
	return p, ""
}

// tally sums the live (not undone) check-in rows of one subject, seen from
// the device that is syncing.
type tally struct {
	OtherIn int // heads let in by other devices
	OwnIn   int // heads let in by this device (earlier ops included)
	Out     int // heads let out, any device
}

// conflicts reports whether letting count more heads in through this device
// double-counts the subject: another device already let some of its heads
// in, and together they exceed the allowance (guest: 1 + plus_n, ticket:
// 1). Heads let out come off, so re-entry after an out is not a conflict.
// A device exceeding the allowance on its own is a door decision (admit
// anyway), not a conflict between devices.
func conflicts(allowance, count int, t tally) bool {
	return t.OtherIn > 0 && t.OtherIn+t.OwnIn+count-t.Out > allowance
}

// Result is the outcome of one op or add.
type Result struct {
	Nonce    string `json:"nonce"`
	ID       string `json:"id,omitempty"` // adds only: the guest id sent
	Status   string `json:"status"`
	Conflict bool   `json:"conflict"`
	Error    string `json:"error,omitempty"`
}

// Checkin is a stored check-in as devices see it.
type Checkin struct {
	Nonce     string    `json:"nonce"`
	Subject   Subject   `json:"subject"`
	Count     int       `json:"count"`
	Direction string    `json:"direction"`
	At        time.Time `json:"at"`
	DeviceID  uuid.UUID `json:"device_id"`
	Undone    bool      `json:"undone"`
	Conflict  bool      `json:"conflict"`
}

// Counters are the event's door counters (all devices, undone excluded).
type Counters struct {
	Walkups   int `json:"walkups"`
	ManualIn  int `json:"manual_in"`
	ManualOut int `json:"manual_out"`
}

// SyncInput is a batch of queued ops plus the cursor of the last sync.
type SyncInput struct {
	Since *string `json:"since"`
	Ops   []Op    `json:"ops"`
}

// SyncResult answers a sync: per-op results, the check-ins received after
// the cursor (all devices), the counters and the next cursor.
type SyncResult struct {
	Results  []Result  `json:"results"`
	Checkins []Checkin `json:"checkins"`
	Counters Counters  `json:"counters"`
	Cursor   string    `json:"cursor"`
}

// Add is one on-the-spot guest added at the door. ID is generated by the
// device, so it can queue a check-in for the guest before syncing.
type Add struct {
	ID         string `json:"id"`
	Nonce      string `json:"nonce"`
	ListID     string `json:"list_id"`
	Name       string `json:"name"`
	PlusN      int    `json:"plus_n"`
	ManagerPIN string `json:"manager_pin"`
	At         string `json:"at"`
}

// AddsInput is a batch of adds.
type AddsInput struct {
	Adds []Add `json:"adds"`
}

// AddsResult answers an adds batch.
type AddsResult struct {
	Results []Result `json:"results"`
}

type parsedAdd struct {
	Add
	id, listID uuid.UUID
}

func (a Add) parse(maxName, maxPlusN int) (parsedAdd, string) {
	p := parsedAdd{Add: a}
	p.Name = strings.Join(strings.Fields(a.Name), " ")
	var err error
	if !validNonce(a.Nonce) {
		return p, "nonce: 8 to 64 visible ASCII characters"
	}
	if p.id, err = uuid.Parse(a.ID); err != nil || p.id == uuid.Nil {
		return p, "id: a client-generated uuid"
	}
	if p.listID, err = uuid.Parse(a.ListID); err != nil {
		return p, "list_id: not an id"
	}
	switch {
	case p.Name == "" || len(p.Name) > maxName:
		return p, "name: required, at most " + strconv.Itoa(maxName) + " characters"
	case a.PlusN < 0 || a.PlusN > maxPlusN:
		return p, "plus_n: 0 to " + strconv.Itoa(maxPlusN)
	case strings.TrimSpace(a.ManagerPIN) == "" || len(a.ManagerPIN) > maxPINLen:
		return p, "manager_pin: required"
	}
	if _, ok := parseAt(a.At); !ok {
		return p, "at: an RFC 3339 time"
	}
	return p, ""
}

// Cursors are opaque to devices: base64url("v1." + unix microseconds of the
// server stamp the sync ran at).
func encodeCursor(t time.Time) string {
	return base64.RawURLEncoding.EncodeToString([]byte("v1." + strconv.FormatInt(t.UnixMicro(), 10)))
}

func decodeCursor(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil {
		return nil, invalid("since", "not a cursor from this server")
	}
	v, ok := strings.CutPrefix(string(raw), "v1.")
	if !ok {
		return nil, invalid("since", "not a cursor from this server")
	}
	us, err := strconv.ParseInt(v, 10, 64)
	if err != nil || us <= 0 {
		return nil, invalid("since", "not a cursor from this server")
	}
	t := time.UnixMicro(us).UTC()
	return &t, nil
}
