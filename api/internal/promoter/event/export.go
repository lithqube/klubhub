package event

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PublicVenue is what the export may say about a venue once allowed.
type PublicVenue struct {
	Name    string
	City    string
	Country string
	Address string // decrypted by the caller; only used when revealed
}

// PublicLocation is the location as the public may see it at a moment.
type PublicLocation struct {
	Name     string `json:"name"`
	City     string `json:"city"`
	Country  string `json:"country,omitempty"`
	Address  string `json:"address,omitempty"`
	Withheld bool   `json:"withheld"`
	// RevealAt is set when a secret location unlocks later.
	RevealAt *time.Time `json:"reveal_at,omitempty"`
}

// LocationAt applies the location mode (UX decision 2) at time now.
func LocationAt(ev Event, venue *PublicVenue, now time.Time) PublicLocation {
	city := ev.City
	if city == "" && venue != nil {
		city = venue.City
	}
	tba := PublicLocation{Name: city + " — location TBA", City: city, Withheld: true}
	if venue != nil {
		tba.Country = venue.Country
	}
	switch ev.LocationMode {
	case LocationCityOnly:
		tba.Name = city
		return tba
	case LocationSecret:
		if ev.LocationRevealAt == nil || now.Before(*ev.LocationRevealAt) {
			tba.RevealAt = ev.LocationRevealAt
			return tba
		}
	}
	if venue == nil {
		return PublicLocation{Name: city, City: city}
	}
	return PublicLocation{Name: venue.Name, City: city, Country: venue.Country, Address: venue.Address}
}

// Organizer is the collective as it appears in exports (P1.5 profile).
type Organizer struct {
	Name        string   `json:"name"`
	URL         string   `json:"url,omitempty"`
	SameAs      []string `json:"same_as,omitempty"`
	Bio         string   `json:"bio,omitempty"`
	AccentColor string   `json:"accent_color,omitempty"`
}

var schemaStatus = map[string]string{
	StatusCancelled: "https://schema.org/EventCancelled",
	StatusPostponed: "https://schema.org/EventPostponed",
}

// JSONLD builds a schema.org MusicEvent (Google event rich result fields).
func JSONLD(ev Event, loc PublicLocation, org Organizer, lineup []LineupEntry) map[string]any {
	tz, err := time.LoadLocation(ev.Timezone)
	if err != nil {
		tz = time.UTC
	}
	status := schemaStatus[ev.Status]
	if status == "" {
		status = "https://schema.org/EventScheduled"
	}
	address := map[string]any{"@type": "PostalAddress", "addressLocality": loc.City}
	if loc.Country != "" {
		address["addressCountry"] = loc.Country
	}
	if loc.Address != "" && !loc.Withheld {
		address["streetAddress"] = loc.Address
	}
	doc := map[string]any{
		"@context":            "https://schema.org",
		"@type":               "MusicEvent",
		"name":                ev.Title,
		"startDate":           ev.StartsAt.In(tz).Format(time.RFC3339),
		"endDate":             ev.EndsAt.In(tz).Format(time.RFC3339),
		"eventStatus":         status,
		"eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
		"location":            map[string]any{"@type": "Place", "name": loc.Name, "address": address},
	}
	if ev.DoorsAt != nil {
		doc["doorTime"] = ev.DoorsAt.In(tz).Format(time.RFC3339)
	}
	if d := PlainText(ev.DescriptionMD); d != "" {
		doc["description"] = d
	}
	if org.Name != "" {
		o := map[string]any{"@type": "Organization", "name": org.Name}
		if org.URL != "" {
			o["url"] = org.URL
		}
		if len(org.SameAs) > 0 {
			o["sameAs"] = org.SameAs
		}
		doc["organizer"] = o
	}
	var performers []map[string]any
	for _, e := range SortByBilling(lineup) {
		p := map[string]any{"@type": "Person", "name": e.DisplayName}
		if e.ProfileURL != nil {
			p["sameAs"] = *e.ProfileURL
		}
		performers = append(performers, p)
	}
	if len(performers) > 0 {
		doc["performer"] = performers
	}
	if ev.ExternalTicketURL != nil {
		offer := map[string]any{"@type": "Offer", "url": *ev.ExternalTicketURL, "availability": "https://schema.org/InStock"}
		if ev.PublishAt != nil {
			offer["validFrom"] = ev.PublishAt.In(tz).Format(time.RFC3339)
		}
		doc["offers"] = offer
	}
	if ev.MinAge != nil && *ev.MinAge > 0 {
		doc["typicalAgeRange"] = fmt.Sprintf("%d-", *ev.MinAge)
	}
	return doc
}

// SortByBilling returns the lineup in billing order (stable).
func SortByBilling(lineup []LineupEntry) []LineupEntry {
	out := append([]LineupEntry(nil), lineup...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].BillingOrder < out[j-1].BillingOrder; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var mdNoise = regexp.MustCompile("[*_`#>]+|\\[([^\\]]*)\\]\\([^)]*\\)")

// PlainText strips light Markdown for plain-text exports.
func PlainText(md string) string {
	s := mdNoise.ReplaceAllStringFunc(md, func(m string) string {
		if strings.HasPrefix(m, "[") {
			return mdNoise.FindStringSubmatch(m)[1]
		}
		return ""
	})
	return strings.TrimSpace(s)
}

// ICS builds an RFC 5545 calendar with one event. UID is stable per event
// and SEQUENCE follows the event version, so calendar apps update in place.
func ICS(ev Event, loc PublicLocation, now time.Time) string {
	stamp := func(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
	status := "CONFIRMED"
	switch ev.Status {
	case StatusCancelled:
		status = "CANCELLED"
	case StatusPostponed, StatusDraft:
		status = "TENTATIVE"
	}
	location := loc.Name
	if loc.Address != "" && !loc.Withheld {
		location += ", " + loc.Address
	}
	lines := []string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//KlubHub//Promoter//EN", "CALSCALE:GREGORIAN", "METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + ev.ID.String() + "@promoter.klubhub",
		fmt.Sprintf("SEQUENCE:%d", max(ev.Version-1, 0)),
		"DTSTAMP:" + stamp(now),
		"DTSTART:" + stamp(ev.StartsAt),
		"DTEND:" + stamp(ev.EndsAt),
		"SUMMARY:" + icsEscape(ev.Title),
		"LOCATION:" + icsEscape(location),
		"STATUS:" + status,
	}
	if d := PlainText(ev.DescriptionMD); d != "" {
		lines = append(lines, "DESCRIPTION:"+icsEscape(d))
	}
	if ev.ExternalTicketURL != nil {
		lines = append(lines, "URL:"+*ev.ExternalTicketURL)
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(fold(l))
		b.WriteString("\r\n")
	}
	return b.String()
}

var icsEscaper = strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\r\n", "\\n", "\n", "\\n")

func icsEscape(s string) string { return icsEscaper.Replace(s) }

// fold splits content lines longer than 75 octets (RFC 5545 §3.1) without
// breaking UTF-8 sequences.
func fold(line string) string {
	if len(line) <= 75 {
		return line
	}
	var b strings.Builder
	n := 0
	for _, r := range line {
		size := len(string(r))
		if n+size > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	return b.String()
}
