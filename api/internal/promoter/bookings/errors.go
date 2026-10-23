package bookings

import "errors"

// Sentinel errors. The handler maps them to HTTP statuses:
// invalid / no contact email / no member reply-to → 422, not found → 404,
// forbidden (slot outside the caller's tenant) → 403, duplicate / template
// version conflict / suppressed → 409, rate limited → 429, email
// unavailable → 503.
var (
	ErrNotFound         = errors.New("bookings: not found")
	ErrForbidden        = errors.New("bookings: forbidden")
	ErrDuplicate        = errors.New("bookings: a row with this key already exists")
	ErrConflict         = errors.New("bookings: updated by another writer")
	ErrNoContactEmail   = errors.New("bookings: contact has no email address")
	ErrNoReplyTo        = errors.New("bookings: no member email to reply to")
	ErrSuppressed       = errors.New("bookings: address is suppressed")
	ErrRateLimited      = errors.New("bookings: daily outreach limit reached")
	ErrEmailUnavailable = errors.New("bookings: email is not available")

	// ErrOutreachNotPermitted is returned when the configured email provider
	// has not been attested as permitting unsolicited B2B outreach. Booking
	// offers are cold outreach by construction — the talent contact never
	// opted in — and Plunk's ToS §5 ("NO COLD EMAILING POLICY") prohibits
	// exactly that, including via the transactional endpoint. The operator
	// must attest (OUTREACH_PROVIDER_PERMITS_OUTREACH=true) that their
	// provider permits it. Fail-closed. See platform/email.OutreachGate.
	ErrOutreachNotPermitted = errors.New(
		"bookings: outreach sending is disabled: your email provider's terms " +
			"prohibit unsolicited B2B outreach (see docs/OUTREACH-PROVIDER.md)")
)

// InvalidError reports a field that failed validation.
type InvalidError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *InvalidError) Error() string { return "bookings: invalid " + e.Field + ": " + e.Problem }

func invalid(field, problem string) error { return &InvalidError{Field: field, Problem: problem} }
