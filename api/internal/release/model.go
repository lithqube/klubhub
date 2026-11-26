// Package release models a DJ's music releases: the entry itself and the
// status workflow it moves through. Deadlines, platform links and the promo
// checklist arrive in the later cards.
package release

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ReleaseStatus is where a release sits in the workflow (REL-02).
type ReleaseStatus string

const (
	StatusDemoSent  ReleaseStatus = "demo_sent"
	StatusAccepted  ReleaseStatus = "accepted"
	StatusMastering ReleaseStatus = "mastering"
	StatusArtwork   ReleaseStatus = "artwork"
	StatusPromo     ReleaseStatus = "promo"
	StatusReleased  ReleaseStatus = "released"
)

// statusOrder is the workflow order. A release moves one step forward or one
// step back (artwork can slip back to mastering); released is terminal.
var statusOrder = []ReleaseStatus{
	StatusDemoSent,
	StatusAccepted,
	StatusMastering,
	StatusArtwork,
	StatusPromo,
	StatusReleased,
}

// ReleaseFormat is what the release ships as (REL-01).
type ReleaseFormat string

const (
	FormatDigital ReleaseFormat = "digital"
	FormatVinyl   ReleaseFormat = "vinyl"
	FormatBoth    ReleaseFormat = "both"
)

var (
	// ErrNotFound means no live release has that ID.
	ErrNotFound = errors.New("release: not found")
	// ErrInvalid means the input cannot be stored.
	ErrInvalid = errors.New("release: invalid input")
	// ErrInvalidTransition means the requested status move is not allowed.
	ErrInvalidTransition = errors.New("release: invalid status transition")
)

// Release is one planned or shipped release.
type Release struct {
	ID          uuid.UUID     `json:"id" db:"id"`
	Title       string        `json:"title" db:"title"`
	Artists     string        `json:"artists" db:"artists"` // comma-separated, as entered
	Label       string        `json:"label" db:"label"`
	Distributor string        `json:"distributor" db:"distributor"`
	ReleaseDate time.Time     `json:"release_date" db:"release_date"`
	Format      ReleaseFormat `json:"format" db:"format"`
	Notes       string        `json:"notes" db:"notes"`
	Status      ReleaseStatus `json:"status" db:"status"`
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time    `json:"deleted_at,omitempty" db:"deleted_at"`
}

// CanTransition reports whether a release may move from one status to
// another. Only the neighbouring steps are allowed and released is terminal.
// Moving to the same status is not a transition (the service treats it as a
// no-op instead).
func CanTransition(from, to ReleaseStatus) bool {
	if from == to {
		return false
	}
	fi, okFrom := statusIndex(from)
	ti, okTo := statusIndex(to)
	if !okFrom || !okTo {
		return false
	}
	if from == StatusReleased {
		return false
	}
	step := ti - fi
	return step == 1 || step == -1
}

// IsOverdue reports whether the release date has passed while the release is
// not out yet. Overdue is computed, never stored.
func (r *Release) IsOverdue(now time.Time) bool {
	if r.Status == StatusReleased {
		return false
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	releaseDay := time.Date(r.ReleaseDate.Year(), r.ReleaseDate.Month(), r.ReleaseDate.Day(), 0, 0, 0, 0, time.UTC)
	return releaseDay.Before(day)
}

// ValidFormat reports whether f is one of the three known formats.
func ValidFormat(f ReleaseFormat) bool {
	switch f {
	case FormatDigital, FormatVinyl, FormatBoth:
		return true
	}
	return false
}

func statusIndex(s ReleaseStatus) (int, bool) {
	for i, v := range statusOrder {
		if v == s {
			return i, true
		}
	}
	return 0, false
}
