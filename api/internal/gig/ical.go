package gig

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"time"
)

// CalendarConfig holds configuration for calendar generation.
type CalendarConfig struct {
	Secret string
}

// GenerateCalendar generates an RFC 5545 compliant iCal VCALENDAR string
// containing all non-cancelled gigs from the gig service.
func GenerateCalendar(ctx context.Context, gigSvc *Service, config CalendarConfig) (string, error) {
	gigs, err := gigSvc.ListGigs(ctx, GigFilter{})
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("BEGIN:VCALENDAR\r\n")
	sb.WriteString("VERSION:2.0\r\n")
	sb.WriteString("PRODID:-//KlubHub//Gig Tracker//EN\r\n")
	sb.WriteString("CALSCALE:GREGORIAN\r\n")
	sb.WriteString("METHOD:PUBLISH\r\n")

	for _, gig := range gigs {
		if gig.Status == GigStatusCancelled {
			continue
		}

		// Determine summary: event name if set, otherwise venue
		summary := gig.Venue
		if gig.EventName != "" {
			summary = gig.EventName
		}

		// Location: venue, city, country
		location := gig.Venue
		if gig.City != "" {
			location += ", " + gig.City
		}
		if gig.Country != "" {
			location += ", " + gig.Country
		}

		// Description: fee + currency, promoter email if present
		description := fmt.Sprintf("Fee: %s %s", gig.FeeAmount.String(), gig.FeeCurrency)
		if gig.PromoterEmail != "" {
			description += fmt.Sprintf("\\nPromoter: %s", gig.PromoterEmail)
		}

		// Format DTSTART as YYYYMMDDTHHmmss in Europe/Berlin timezone
		dtStart := gig.Date.In(time.UTC).Format("20060102T150405")

		sb.WriteString("BEGIN:VEVENT\r\n")
		sb.WriteString(fmt.Sprintf("UID:%s@klubhub\r\n", gig.ID.String()))
		sb.WriteString(fmt.Sprintf("DTSTART;TZID=Europe/Berlin:%s\r\n", dtStart))
		sb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICalText(summary)))
		sb.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICalText(location)))
		sb.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICalText(description)))
		sb.WriteString("END:VEVENT\r\n")
	}

	sb.WriteString("END:VCALENDAR\r\n")
	return sb.String(), nil
}

// ValidateSecret checks if the provided secret matches the expected secret
// using constant-time comparison to prevent timing attacks.
func ValidateSecret(provided, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// escapeICalText escapes special characters in iCal text fields per RFC 5545.
func escapeICalText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
