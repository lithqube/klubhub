// Package audience is the P3.1 audience CRM: contacts with a consent
// record, a subscription status, and saved-filter segments.
// Plan: .claude/plans/promoter-p3-audience-promotion.plan.md (P3.1).
//
// Contact names, emails and phones are `personal` data (plan §Data model
// and security): sealed with the tenant DEK, with blind indexes for exact
// lookup and CSV-import dedupe. Per D2 (self-hosted has no public surface),
// nothing here runs a public follow/notify-me form — "follow" and
// "notify_me" are recorded sources for contacts added through the admin
// UI or a CSV import, mirroring what a SaaS-hosted public page would have
// captured.
package audience

import (
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Statuses a contact can hold.
const (
	StatusActive       = "active"
	StatusUnsubscribed = "unsubscribed"
	StatusBounced      = "bounced"
	StatusComplained   = "complained"
)

// Statuses in tab order.
var Statuses = []string{StatusActive, StatusUnsubscribed, StatusBounced, StatusComplained}

// Sources a contact can come from.
const (
	SourceRSVP     = "rsvp"
	SourceFollow   = "follow"
	SourceNotifyMe = "notify_me"
	SourceCSV      = "csv"
	SourceDoor     = "door"
)

// Sources in display order.
var Sources = []string{SourceRSVP, SourceFollow, SourceNotifyMe, SourceCSV, SourceDoor}

// Lawful bases for the consent record (plan §Data model and security).
const (
	BasisConsent   = "consent"
	BasisSoftOptIn = "soft_opt_in"
)

var bases = []string{BasisConsent, BasisSoftOptIn}

// Limits.
const (
	maxNameLen     = 120
	maxEmailLen    = 254
	maxPhoneLen    = 40
	maxFormTextLen = 2000
	maxSegmentName = 80
	MaxBatch       = 2000
	MaxPerOrg      = 100000
)

var phoneRe = regexp.MustCompile(`^\+?[0-9][0-9 ().-]{4,}$`)

// ConsentInput is the consent record captured when a contact is added.
// IP is optional (a CSV import of past attendees may have none); when
// present it is sealed like any other personal field.
type ConsentInput struct {
	Basis         string     `json:"basis"`
	RecordedAt    time.Time  `json:"recorded_at"`
	IP            string     `json:"ip"`
	FormText      string     `json:"form_text"`
	DoubleOptInAt *time.Time `json:"double_opt_in_confirmed_at,omitempty"`
}

func (c *ConsentInput) validate() error {
	c.FormText = strings.TrimSpace(c.FormText)
	c.IP = strings.TrimSpace(c.IP)
	switch {
	case !slices.Contains(bases, c.Basis):
		return invalid("consent.basis", strings.Join(bases, ", "))
	case c.RecordedAt.IsZero():
		return invalid("consent.recorded_at", "required")
	case len(c.FormText) > maxFormTextLen:
		return invalid("consent.form_text", fmt.Sprintf("at most %d characters", maxFormTextLen))
	}
	return nil
}

// ContactInput creates or updates a contact.
type ContactInput struct {
	Name    string       `json:"name"`
	Email   string       `json:"email"`
	Phone   string       `json:"phone"`
	Source  string       `json:"source"`
	Status  string       `json:"status,omitempty"`
	Consent ConsentInput `json:"consent"`
}

func (in *ContactInput) validate(requireConsent bool) error {
	in.Name = strings.Join(strings.Fields(in.Name), " ")
	in.Email = strings.TrimSpace(in.Email)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.Status == "" {
		in.Status = StatusActive
	}
	switch {
	case in.Name == "" && in.Email == "":
		return invalid("name", "a contact needs a name or an email")
	case len(in.Name) > maxNameLen:
		return invalid("name", fmt.Sprintf("at most %d characters", maxNameLen))
	case len(in.Email) > maxEmailLen:
		return invalid("email", "too long")
	case len(in.Phone) > maxPhoneLen || (in.Phone != "" && !phoneRe.MatchString(in.Phone)):
		return invalid("phone", "digits, spaces and an optional leading +")
	case !slices.Contains(Sources, in.Source):
		return invalid("source", strings.Join(Sources, ", "))
	case !slices.Contains(Statuses, in.Status):
		return invalid("status", strings.Join(Statuses, ", "))
	}
	if in.Email != "" {
		a, err := mail.ParseAddress(in.Email)
		if err != nil || a.Address != in.Email || !strings.Contains(in.Email[strings.LastIndex(in.Email, "@"):], ".") {
			return invalid("email", "not an email address")
		}
	}
	if requireConsent {
		return in.Consent.validate()
	}
	return nil
}

// Contact as the audience table shows it (decrypted for org members).
type Contact struct {
	ID                     uuid.UUID  `json:"id"`
	Name                   string     `json:"name"`
	Email                  string     `json:"email"`
	Phone                  string     `json:"phone"`
	Status                 string     `json:"status"`
	Source                 string     `json:"source"`
	ConsentBasis           string     `json:"consent_basis"`
	ConsentRecordedAt      time.Time  `json:"consent_recorded_at"`
	ConsentIP              string     `json:"consent_ip,omitempty"`
	ConsentFormText        string     `json:"consent_form_text"`
	DoubleOptInConfirmedAt *time.Time `json:"double_opt_in_confirmed_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// Counts are the status-tab counts.
type Counts struct {
	All          int `json:"all"`
	Active       int `json:"active"`
	Unsubscribed int `json:"unsubscribed"`
	Bounced      int `json:"bounced"`
	Complained   int `json:"complained"`
}

// ContactFilter narrows the contact list.
type ContactFilter struct {
	Status string
	Source string
	Q      string // exact lookup through the blind indexes (an email or full name)
}

// Page is the audience contact table: matching contacts plus tab counts.
type Page struct {
	Contacts []Contact `json:"contacts"`
	Counts   Counts    `json:"counts"`
}

// StatusInput changes one contact's subscription status (e.g. an inbound
// bounce or complaint webhook from the email provider, or a manual
// unsubscribe by staff).
type StatusInput struct {
	Status string `json:"status"`
}

func (in *StatusInput) validate() error {
	if !slices.Contains(Statuses, in.Status) {
		return invalid("status", strings.Join(Statuses, ", "))
	}
	return nil
}

// SegmentFilter is a saved query over contacts. Only predicates backed by
// data that actually exists today are implemented (plan §Open questions —
// predicates referencing campaign engagement or event attendance are
// deferred to when that data exists, in P3.2/P3.3).
type SegmentFilter struct {
	Status    string `json:"status,omitempty"`
	Source    string `json:"source,omitempty"`
	SinceDays *int   `json:"since_days,omitempty"` // created in the last N days
}

func (f *SegmentFilter) validate() error {
	if f.Status != "" && !slices.Contains(Statuses, f.Status) {
		return invalid("filter.status", strings.Join(Statuses, ", "))
	}
	if f.Source != "" && !slices.Contains(Sources, f.Source) {
		return invalid("filter.source", strings.Join(Sources, ", "))
	}
	if f.SinceDays != nil && (*f.SinceDays < 1 || *f.SinceDays > 3650) {
		return invalid("filter.since_days", "1 to 3650")
	}
	return nil
}

// SegmentInput creates or replaces a segment.
type SegmentInput struct {
	Name   string        `json:"name"`
	Filter SegmentFilter `json:"filter"`
}

func (in *SegmentInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxSegmentName {
		return invalid("name", fmt.Sprintf("required, at most %d characters", maxSegmentName))
	}
	return in.Filter.validate()
}

// Segment is a saved filter with its current matching count.
type Segment struct {
	ID        uuid.UUID     `json:"id"`
	Name      string        `json:"name"`
	Filter    SegmentFilter `json:"filter"`
	Matching  int           `json:"matching"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ImportRow is one row of a CSV import.
type ImportRow struct {
	Name  string
	Email string
	Phone string
}

// ImportResult reports what an import did.
type ImportResult struct {
	Added      int `json:"added"`
	Updated    int `json:"updated"`
	Duplicates int `json:"duplicates"`
	Invalid    int `json:"invalid"`
}
