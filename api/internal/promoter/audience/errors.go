package audience

import "errors"

var (
	ErrNotFound      = errors.New("audience: not found")
	ErrDuplicate     = errors.New("audience: a contact with this email already exists")
	ErrSegmentExists = errors.New("audience: a segment with this name already exists")
)

// InvalidError reports a field that failed validation.
type InvalidError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *InvalidError) Error() string { return "audience: invalid " + e.Field + ": " + e.Problem }

func invalid(field, problem string) error { return &InvalidError{Field: field, Problem: problem} }
