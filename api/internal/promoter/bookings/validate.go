package bookings

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Validation limits (SPEC §6.1). Every validator normalizes in place (trim,
// default) and reports the FIRST invalid field as an *InvalidError, which the
// handler maps to 422.

const (
	maxNameLen     = 120
	maxEmailLen    = 254
	maxPhoneLen    = 40
	maxCityLen     = 80
	maxGenreCount  = 12
	maxGenreLen    = 40
	maxNotesLen    = 2000
	maxRoleLen     = 80
	maxTplNameLen  = 80
	maxSubjectLen  = 200
	maxBodyLen     = 20000
	maxReasonLen   = 200
)

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

func validSource(s string) bool {
	switch s {
	case SourcePastGig, SourceReferral, SourceManual:
		return true
	}
	return false
}

// validateGenres normalizes and validates a genre list in place.
func validateGenres(genres *[]string) error {
	if *genres == nil {
		*genres = []string{}
	}
	if len(*genres) > maxGenreCount {
		return invalid("genres", fmt.Sprintf("at most %d genres", maxGenreCount))
	}
	for i, g := range *genres {
		(*genres)[i] = strings.TrimSpace(g)
		if (*genres)[i] == "" {
			return invalid("genres", "genres must not be empty")
		}
		if utf8.RuneCountInString((*genres)[i]) > maxGenreLen {
			return invalid("genres", fmt.Sprintf("each genre exceeds %d characters", maxGenreLen))
		}
	}
	return nil
}

// validateTalentContact normalizes and validates a talent contact write. A
// contact needs at least a name or an email (the table's has_identity CHECK);
// the email is optional — name-only contacts are listed/matched with
// has_email=false and never sent to (OFFER-10).
func validateTalentContact(in *TalentContactInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(in.Name) > maxNameLen {
		return invalid("name", fmt.Sprintf("exceeds %d characters", maxNameLen))
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" {
		in.Email = ""
	} else {
		if utf8.RuneCountInString(in.Email) > maxEmailLen {
			return invalid("email", fmt.Sprintf("exceeds %d characters", maxEmailLen))
		}
		if !validEmail(in.Email) {
			return invalid("email", "must be a valid email address")
		}
	}
	if in.Name == "" && in.Email == "" {
		return invalid("name", "a name or an email is required")
	}
	in.Phone = strings.TrimSpace(in.Phone)
	if utf8.RuneCountInString(in.Phone) > maxPhoneLen {
		return invalid("phone", fmt.Sprintf("exceeds %d characters", maxPhoneLen))
	}
	in.City = strings.TrimSpace(in.City)
	if utf8.RuneCountInString(in.City) > maxCityLen {
		return invalid("city", fmt.Sprintf("exceeds %d characters", maxCityLen))
	}
	in.Country = strings.TrimSpace(in.Country)
	if utf8.RuneCountInString(in.Country) > maxCityLen {
		return invalid("country", fmt.Sprintf("exceeds %d characters", maxCityLen))
	}
	if err := validateGenres(&in.Genres); err != nil {
		return err
	}
	in.Source = strings.TrimSpace(in.Source)
	if in.Source == "" {
		in.Source = SourceManual // a contact added by hand
	}
	if !validSource(in.Source) {
		return invalid("source", "must be one of past_gig, referral, manual")
	}
	in.Notes = strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(in.Notes) > maxNotesLen {
		return invalid("notes", fmt.Sprintf("exceeds %d characters", maxNotesLen))
	}
	return nil
}

// validateOpenSlot normalizes and validates a new open slot. The role may be
// empty (a general "play this night" slot); UNIQUE (event_id, role) keeps one
// slot per role per event.
func validateOpenSlot(in *OpenSlotInput) error {
	in.Role = strings.TrimSpace(in.Role)
	if utf8.RuneCountInString(in.Role) > maxRoleLen {
		return invalid("role", fmt.Sprintf("exceeds %d characters", maxRoleLen))
	}
	if err := validateGenres(&in.Genres); err != nil {
		return err
	}
	if in.FeeMin != nil && *in.FeeMin < 0 {
		return invalid("fee_min", "must not be negative")
	}
	if in.FeeMax != nil && *in.FeeMax < 0 {
		return invalid("fee_max", "must not be negative")
	}
	if in.FeeMin != nil && in.FeeMax != nil && *in.FeeMin > *in.FeeMax {
		return invalid("fee_min", "must not exceed fee_max")
	}
	if in.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*in.Currency))
		in.Currency = &c
		if c != "" && (len(c) != 3 || !isAlpha3(c)) {
			return invalid("currency", "must be a 3-letter ISO 4217 code")
		}
	}
	in.Notes = strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(in.Notes) > maxNotesLen {
		return invalid("notes", fmt.Sprintf("exceeds %d characters", maxNotesLen))
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

// validateTemplateCreate normalizes and validates a new template.
func validateTemplateCreate(in *TemplateCreate) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name", "required")
	}
	if utf8.RuneCountInString(in.Name) > maxTplNameLen {
		return invalid("name", fmt.Sprintf("exceeds %d characters", maxTplNameLen))
	}
	in.Subject = strings.TrimSpace(in.Subject)
	if in.Subject == "" {
		return invalid("subject", "required")
	}
	if utf8.RuneCountInString(in.Subject) > maxSubjectLen {
		return invalid("subject", fmt.Sprintf("exceeds %d characters", maxSubjectLen))
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		return invalid("body", "required")
	}
	if utf8.RuneCountInString(in.Body) > maxBodyLen {
		return invalid("body", fmt.Sprintf("exceeds %d characters", maxBodyLen))
	}
	return nil
}

// validateTemplateUpdate validates the non-nil fields of an update. Version
// is the optimistic-concurrency token and must be present (>= 1).
func validateTemplateUpdate(in *TemplateUpdate) error {
	if in.Version < 1 {
		return invalid("version", "required (optimistic concurrency token)")
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
		if n == "" {
			return invalid("name", "required")
		}
		if utf8.RuneCountInString(n) > maxTplNameLen {
			return invalid("name", fmt.Sprintf("exceeds %d characters", maxTplNameLen))
		}
	}
	if in.Subject != nil {
		s := strings.TrimSpace(*in.Subject)
		in.Subject = &s
		if s == "" {
			return invalid("subject", "required")
		}
		if utf8.RuneCountInString(s) > maxSubjectLen {
			return invalid("subject", fmt.Sprintf("exceeds %d characters", maxSubjectLen))
		}
	}
	if in.Body != nil {
		b := strings.TrimSpace(*in.Body)
		in.Body = &b
		if b == "" {
			return invalid("body", "required")
		}
		if utf8.RuneCountInString(b) > maxBodyLen {
			return invalid("body", fmt.Sprintf("exceeds %d characters", maxBodyLen))
		}
	}
	return nil
}

// validateSendOffer validates the per-request overrides. The contact must be
// set; subject/body overrides, when present, obey the same caps as templates.
func validateSendOffer(in *SendOfferInput) error {
	if in.TalentContactID == uuid.Nil {
		return invalid("talent_contact_id", "required")
	}
	if in.Subject != nil && utf8.RuneCountInString(*in.Subject) > maxSubjectLen {
		return invalid("subject", fmt.Sprintf("exceeds %d characters", maxSubjectLen))
	}
	if in.Body != nil && utf8.RuneCountInString(*in.Body) > maxBodyLen {
		return invalid("body", fmt.Sprintf("exceeds %d characters", maxBodyLen))
	}
	return nil
}

// validateSuppression normalizes and validates a suppression. The email is
// required (the blind index is the only thing stored — SPEC §9).
func validateSuppression(in *SuppressionCreate) error {
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" {
		return invalid("email", "required")
	}
	if utf8.RuneCountInString(in.Email) > maxEmailLen {
		return invalid("email", fmt.Sprintf("exceeds %d characters", maxEmailLen))
	}
	if !validEmail(in.Email) {
		return invalid("email", "must be a valid email address")
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > maxReasonLen {
		return invalid("reason", fmt.Sprintf("exceeds %d characters", maxReasonLen))
	}
	return nil
}

// formatFee renders the slot fee range for the {{fee}} variable.
func formatFee(s *OpenSlot) string {
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

// localMidnightIn returns local midnight of t's day in the named IANA
// timezone — the start of the daily-cap window (SPEC §9 v2.1: the window is
// the ORG's timezone, organizations.timezone). An unknown timezone falls back
// to UTC rather than failing the send.
func localMidnightIn(t time.Time, tzName string) time.Time {
	loc, err := time.LoadLocation(tzName)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}
