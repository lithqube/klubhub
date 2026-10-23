package bookings

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/email"
	"github.com/klubhub/dj/api/internal/promoter/event"
)

func TestOfferVenueName(t *testing.T) {
	reveal := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

	// 'venue': the venue is public.
	if got := offerVenueName(event.LocationVenue, nil, "Tresor", now); got != "Tresor" {
		t.Fatalf("venue mode must show the venue, got %q", got)
	}
	// 'city_only': the venue is never disclosed (OFFER-09).
	if got := offerVenueName(event.LocationCityOnly, nil, "Tresor", now); got != "" {
		t.Fatalf("city_only must hide the venue, got %q", got)
	}
	// 'secret' before the reveal: hidden; at/after the reveal: shown.
	if got := offerVenueName(event.LocationSecret, &reveal, "Tresor", now); got != "" {
		t.Fatalf("secret before reveal must hide the venue, got %q", got)
	}
	if got := offerVenueName(event.LocationSecret, &reveal, "Tresor", reveal); got != "Tresor" {
		t.Fatalf("secret at reveal must show the venue, got %q", got)
	}
	if got := offerVenueName(event.LocationSecret, &reveal, "Tresor", reveal.Add(time.Hour)); got != "Tresor" {
		t.Fatalf("secret after reveal must show the venue, got %q", got)
	}
	// 'secret' without a reveal time: never disclosed.
	if got := offerVenueName(event.LocationSecret, nil, "Tresor", now); got != "" {
		t.Fatalf("secret without reveal must hide the venue, got %q", got)
	}
}

func TestOfferVariables_WithEventAndSlot(t *testing.T) {
	starts := time.Date(2026, 12, 24, 23, 0, 0, 0, time.UTC)
	reveal := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	ev := &eventOffer{
		title: "Klubnacht", startsAt: starts, timezone: "Europe/Berlin",
		city: "Berlin", locationMode: event.LocationSecret, locationRevealAt: &reveal,
		genres: []string{"techno"}, venueName: "Tresor",
	}
	fee := 600.0
	slot := &OpenSlot{ID: uuid.New(), Role: "Headliner", Genres: []string{"techno", "house"}, FeeMin: &fee, FeeMax: &fee, Currency: sptr("EUR"), Notes: "PA included"}
	now := time.Date(2026, 12, 10, 12, 0, 0, 0, time.UTC) // after the reveal

	vars := offerVariables("Nachtwerk", "Mara", "mara@nachtwerk.example", "Nadia", ev, slot, now)

	want := map[string]string{
		"org_name":       "Nachtwerk",
		"promoter_name":  "Mara",
		"promoter_email": "mara@nachtwerk.example",
		"promoter_phone": "", // users has no phone column (00003)
		"dj_name":        "Nadia",
		"event_name":     "Klubnacht",
		"date":           "2026-12-25 00:00 CET", // 23:00 UTC in Berlin (CET, UTC+1)
		"city":           "Berlin",
		"venue":          "Tresor", // revealed
		"genres":         "techno, house",
		"fee":            "600 EUR",
		"slot_role":      "Headliner",
		"notes":          "PA included",
	}
	for k, v := range want {
		if vars[k] != v {
			t.Fatalf("var %s = %q, want %q", k, vars[k], v)
		}
	}
}

func TestOfferVariables_WithoutEventOrSlot(t *testing.T) {
	vars := offerVariables("Nachtwerk", "Mara", "mara@nachtwerk.example", "Nadia", nil, nil, time.Now())
	for _, k := range []string{"org_name", "promoter_name", "promoter_email", "dj_name"} {
		if vars[k] == "" {
			t.Fatalf("var %s must be set even without an event", k)
		}
	}
	// Event/slot variables are absent (Render turns unknown keys into "").
	for _, k := range []string{"event_name", "date", "city", "venue", "genres", "fee", "slot_role", "notes"} {
		if _, ok := vars[k]; ok {
			t.Fatalf("var %s must be absent without an event/slot, got %q", k, vars[k])
		}
	}
}

func TestOfferVariables_VenueWithheld(t *testing.T) {
	reveal := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) // before the reveal
	ev := &eventOffer{
		title: "Klubnacht", startsAt: now, timezone: "UTC",
		city: "Berlin", locationMode: event.LocationSecret, locationRevealAt: &reveal,
		venueName: "Tresor",
	}
	vars := offerVariables("Org", "Mara", "m@o.example", "Nadia", ev, nil, now)
	if vars["venue"] != "" {
		t.Fatalf("venue must render empty before the reveal, got %q", vars["venue"])
	}
	if vars["city"] != "Berlin" {
		t.Fatalf("city must still render, got %q", vars["city"])
	}
}

func TestFormatEventDate(t *testing.T) {
	starts := time.Date(2026, 12, 24, 23, 0, 0, 0, time.UTC)
	if got := formatEventDate(starts, "Europe/Berlin"); got != "2026-12-25 00:00 CET" {
		t.Fatalf("formatEventDate = %q", got)
	}
	// Unknown timezone falls back to UTC.
	if got := formatEventDate(starts, "not/a-zone"); got != "2026-12-24 23:00 UTC" {
		t.Fatalf("formatEventDate fallback = %q", got)
	}
}

// The built-in default template must render with every SPEC §7 variable
// covered — no {{...}} may survive a render with a full variable map.
func TestDefaultTemplate_RendersAllVariables(t *testing.T) {
	vars := map[string]string{
		"org_name": "Nachtwerk", "promoter_name": "Mara", "promoter_email": "mara@nachtwerk.example",
		"promoter_phone": "", "dj_name": "Nadia", "event_name": "Klubnacht", "date": "2026-12-25 00:00 CET",
		"city": "Berlin", "venue": "Tresor", "genres": "techno", "fee": "600 EUR",
		"slot_role": "Headliner", "notes": "PA included",
	}
	subject := email.Render(DefaultTemplateSubject, vars)
	if strings.Contains(subject, "{{") {
		t.Fatalf("subject still has a variable: %q", subject)
	}
	if !strings.Contains(subject, "Klubnacht") || !strings.Contains(subject, "2026-12-25") {
		t.Fatalf("subject must carry the event name and date: %q", subject)
	}
	body := email.Render(DefaultTemplateBody, vars)
	if strings.Contains(body, "{{") {
		t.Fatalf("body still has a variable: %q", body)
	}
	for _, want := range []string{"Nadia", "Nachtwerk", "Klubnacht", "Berlin", "Tresor", "Headliner", "techno", "600 EUR", "mara@nachtwerk.example", "Mara"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body must contain %q:\n%s", want, body)
		}
	}
	// The opt-out footer is part of the compliance contract (SPEC §9).
	if !strings.Contains(body, "stop") {
		t.Fatalf("default template must carry the opt-out footer:\n%s", body)
	}
}

// Render strips CR/LF from substituted values (header-injection defense):
// a contact name with a newline must not survive into the subject.
func TestDefaultTemplate_RenderStripsHeaderInjection(t *testing.T) {
	vars := map[string]string{"dj_name": "Nadia\r\nBcc: victim@example.com", "event_name": "X", "date": "D"}
	subject := email.Render(DefaultTemplateSubject, vars)
	if strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("rendered subject must not contain CR/LF: %q", subject)
	}
}
