package event

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrNotFound        = errors.New("event: not found")
	ErrVersionConflict = errors.New("event: changed by someone else")
	ErrVenueInUse      = errors.New("event: venue is used by upcoming events")
	ErrBadTransition   = errors.New("event: status change not allowed")
)

// InvalidError reports a field that failed validation.
type InvalidError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *InvalidError) Error() string { return "event: invalid " + e.Field + ": " + e.Problem }

func invalid(field, problem string) error { return &InvalidError{Field: field, Problem: problem} }

// CutsSetsError: an event edit would leave scheduled sets outside the new
// window (UX decision 5: reject and list them).
type CutsSetsError struct{ Entries []uuid.UUID }

func (e *CutsSetsError) Error() string { return "event: edit would cut scheduled sets" }

// BlockedError: scheduling, publishing or exporting is blocked (UX decision 3).
type BlockedError struct {
	Reason string  `json:"reason"`
	Issues []Issue `json:"issues,omitempty"`
}

func (e *BlockedError) Error() string { return "event: blocked: " + e.Reason }
