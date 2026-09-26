package guest

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrNotFound          = errors.New("guest: not found")
	ErrAllocationRevoked = errors.New("guest: allocation revoked")
	ErrAllocationClosed  = errors.New("guest: allocation deadline passed")
)

// InvalidError reports a field that failed validation.
type InvalidError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *InvalidError) Error() string { return "guest: invalid " + e.Field + ": " + e.Problem }

func invalid(field, problem string) error { return &InvalidError{Field: field, Problem: problem} }

// QuotaError: the change would put more heads on an allocation than its
// quota allows. Heads are a guest plus their +N.
type QuotaError struct {
	AllocationID uuid.UUID `json:"allocation_id"`
	Label        string    `json:"label"`
	Quota        int       `json:"quota"`
	Used         int       `json:"used"`
	Requested    int       `json:"requested"`
}

func (e *QuotaError) Error() string { return "guest: allocation quota exceeded" }

// NotEmptyError: a list still has guests; deleting it needs force.
type NotEmptyError struct{ Guests int }

func (e *NotEmptyError) Error() string { return "guest: list is not empty" }
