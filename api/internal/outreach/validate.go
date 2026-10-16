package outreach

import (
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ValidationError lists the invalid fields of an outreach write. It
// satisfies errors.Is(err, ErrValidation).
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range sortedKeys(e.Fields) {
		parts = append(parts, f+": "+e.Fields[f])
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Is reports whether target is ErrValidation.
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// NormalizeEmail lower-cases and trims an email address — the form stored
// in outreach_suppressions and compared against contact emails.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeCurrency trims and upper-cases an ISO 4217 currency code.
func NormalizeCurrency(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// ValidateSlotCreate normalizes in place and validates a new slot. The date
// must not be in the past (OUTR-09): the comparison is calendar-day based —
// the slot's UTC day against today's day in the process local timezone, so
// "today" is accepted in every timezone.
func ValidateSlotCreate(in *SlotCreate, now time.Time) error {
	errs := map[string]string{}
	in.City = strings.TrimSpace(in.City)
	if in.City == "" {
		errs["city"] = "required"
	} else if utf8.RuneCountInString(in.City) > 80 {
		errs["city"] = "exceeds 80 characters"
	}
	in.Country = strings.TrimSpace(in.Country)
	if utf8.RuneCountInString(in.Country) > 80 {
		errs["country"] = "exceeds 80 characters"
	}
	if time.Time(in.Date).IsZero() {
		errs["date"] = "required"
	} else {
		local := now.In(time.Local)
		todayUTC := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		if time.Time(in.Date).UTC().Before(todayUTC) {
			errs["date"] = "must not be in the past"
		}
	}
	if in.Genres == nil {
		in.Genres = []string{}
	}
	if len(in.Genres) > 12 {
		errs["genres"] = "at most 12 genres"
	}
	for i, g := range in.Genres {
		in.Genres[i] = strings.TrimSpace(g)
		if in.Genres[i] == "" {
			errs["genres"] = "genres must not be empty"
			break
		}
		if utf8.RuneCountInString(in.Genres[i]) > 40 {
			errs["genres"] = "each genre exceeds 40 characters"
			break
		}
	}
	if in.FeeMin != nil && *in.FeeMin < 0 {
		errs["fee_min"] = "must not be negative"
	}
	if in.FeeMax != nil && *in.FeeMax < 0 {
		errs["fee_max"] = "must not be negative"
	}
	if in.FeeMin != nil && in.FeeMax != nil && *in.FeeMin > *in.FeeMax {
		errs["fee_min"] = "must not exceed fee_max"
	}
	if in.Currency != nil {
		c := NormalizeCurrency(*in.Currency)
		in.Currency = &c
		if c != "" && (len(c) != 3 || !isAlpha3(c)) {
			errs["currency"] = "must be a 3-letter ISO 4217 code"
		}
	}
	in.Notes = strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(in.Notes) > 2000 {
		errs["notes"] = "exceeds 2000 characters"
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

func isAlpha3(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// ValidateTemplateCreate normalizes in place and validates a new template.
func ValidateTemplateCreate(in *TemplateCreate) error {
	errs := map[string]string{}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		errs["name"] = "required"
	} else if utf8.RuneCountInString(in.Name) > 120 {
		errs["name"] = "exceeds 120 characters"
	}
	in.Subject = strings.TrimSpace(in.Subject)
	if in.Subject == "" {
		errs["subject"] = "required"
	} else if utf8.RuneCountInString(in.Subject) > 200 {
		errs["subject"] = "exceeds 200 characters"
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		errs["body"] = "required"
	} else if utf8.RuneCountInString(in.Body) > 20000 {
		errs["body"] = "exceeds 20000 characters"
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// ValidateTemplateUpdate validates the non-nil fields of an update. Version
// is the optimistic-concurrency token and must be present (>= 1).
func ValidateTemplateUpdate(in *TemplateUpdate) error {
	errs := map[string]string{}
	if in.Version < 1 {
		errs["version"] = "required (optimistic concurrency token)"
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
		if n == "" {
			errs["name"] = "required"
		} else if utf8.RuneCountInString(n) > 120 {
			errs["name"] = "exceeds 120 characters"
		}
	}
	if in.Subject != nil {
		s := strings.TrimSpace(*in.Subject)
		in.Subject = &s
		if s == "" {
			errs["subject"] = "required"
		} else if utf8.RuneCountInString(s) > 200 {
			errs["subject"] = "exceeds 200 characters"
		}
	}
	if in.Body != nil {
		b := strings.TrimSpace(*in.Body)
		in.Body = &b
		if b == "" {
			errs["body"] = "required"
		} else if utf8.RuneCountInString(b) > 20000 {
			errs["body"] = "exceeds 20000 characters"
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// ValidateSendRequest validates the per-request overrides. ContactID must
// be set; subject/body overrides, when present, obey the same caps as
// templates.
func ValidateSendRequest(in *SendRequestInput) error {
	errs := map[string]string{}
	if in.ContactID == uuid.Nil {
		errs["contact_id"] = "required"
	}
	if in.Subject != nil && utf8.RuneCountInString(*in.Subject) > 200 {
		errs["subject"] = "exceeds 200 characters"
	}
	if in.Body != nil && utf8.RuneCountInString(*in.Body) > 20000 {
		errs["body"] = "exceeds 20000 characters"
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// ValidateSuppression normalizes in place and validates a suppression.
func ValidateSuppression(in *SuppressionCreate) error {
	errs := map[string]string{}
	in.Email = NormalizeEmail(in.Email)
	if in.Email == "" {
		errs["email"] = "required"
	} else if !validEmail(in.Email) {
		errs["email"] = "must be a valid email address"
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > 500 {
		errs["reason"] = "exceeds 500 characters"
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// formatFee renders the slot fee range for the {{fee}} variable.
func formatFee(s *Slot) string {
	cur := ""
	if s.Currency != nil && *s.Currency != "" {
		cur = " " + *s.Currency
	}
	switch {
	case s.FeeMin != nil && s.FeeMax != nil && *s.FeeMin == *s.FeeMax:
		return formatMoney(*s.FeeMin) + cur
	case s.FeeMin != nil && s.FeeMax != nil:
		return formatMoney(*s.FeeMin) + "–" + formatMoney(*s.FeeMax) + cur
	case s.FeeMin != nil:
		return "from " + formatMoney(*s.FeeMin) + cur
	case s.FeeMax != nil:
		return "up to " + formatMoney(*s.FeeMax) + cur
	}
	return ""
}

func formatMoney(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
