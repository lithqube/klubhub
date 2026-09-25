package event

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func sampleEvent() Event {
	url := "https://tickets.example/klubnacht"
	age := 18
	return Event{
		ID: uuid.MustParse("0190f1d2-7c1a-7a00-9f00-0000000000e1"), Title: "Klubnacht 03", Status: StatusPublished,
		StartsAt: *at(3, 23, 0), EndsAt: *at(4, 7, 0), DoorsAt: at(3, 22, 30), Timezone: "Europe/Berlin",
		City: "Berlin", LocationMode: LocationVenue, DescriptionMD: "**Eight hours**, two rooms; see [RA](https://ra.co).",
		ExternalTicketURL: &url, MinAge: &age, Version: 3,
	}
}

var venue = &PublicVenue{Name: "Tresor.West", City: "Berlin", Country: "DE", Address: "Unterstraße 3"}

func TestLocationModes(t *testing.T) {
	ev := sampleEvent()
	now := *at(1, 12, 0)
	if l := LocationAt(ev, venue, now); l.Withheld || l.Name != "Tresor.West" || l.Address == "" {
		t.Fatalf("venue mode: %+v", l)
	}
	ev.LocationMode = LocationCityOnly
	if l := LocationAt(ev, venue, now); !l.Withheld || l.Address != "" || strings.Contains(l.Name, "Tresor") {
		t.Fatalf("city_only must never expose the venue: %+v", l)
	}
	ev.LocationMode = LocationSecret
	ev.LocationRevealAt = at(3, 12, 0)
	if l := LocationAt(ev, venue, now); !l.Withheld || strings.Contains(l.Name, "Tresor") || l.RevealAt == nil {
		t.Fatalf("secret before reveal: %+v", l)
	}
	if l := LocationAt(ev, venue, *at(3, 12, 1)); l.Withheld || l.Name != "Tresor.West" {
		t.Fatalf("secret after reveal: %+v", l)
	}
	ev.LocationRevealAt = nil
	if l := LocationAt(ev, venue, *at(9, 0, 0)); !l.Withheld {
		t.Fatal("secret without a reveal time stays hidden")
	}
}

func TestJSONLDHasGoogleFieldsAndNoWithheldData(t *testing.T) {
	ev := sampleEvent()
	lineup := []LineupEntry{{DisplayName: "Dasha Rush", BillingOrder: 2}, {DisplayName: "Ben Klock", BillingOrder: 1, ProfileURL: ptrS("https://ra.co/dj/benklock")}}
	doc := JSONLD(ev, LocationAt(ev, venue, *at(1, 0, 0)), Organizer{Name: "Nachtwerk"}, lineup)
	for _, k := range []string{"@context", "@type", "name", "startDate", "location", "endDate", "eventStatus", "doorTime", "performer", "offers", "organizer", "description"} {
		if _, ok := doc[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if doc["startDate"] != "2026-10-03T23:00:00+02:00" {
		t.Errorf("startDate must carry the event offset: %v", doc["startDate"])
	}
	b, _ := json.Marshal(doc)
	if !strings.Contains(string(b), `"name":"Ben Klock","sameAs":"https://ra.co/dj/benklock"`) && !strings.Contains(string(b), `"sameAs":"https://ra.co/dj/benklock"`) {
		t.Errorf("performer sameAs missing: %s", b)
	}
	if strings.Index(string(b), "Ben Klock") > strings.Index(string(b), "Dasha Rush") {
		t.Error("performers must follow billing order")
	}
	ev.LocationMode = LocationSecret
	ev.LocationRevealAt = at(3, 12, 0)
	b, _ = json.Marshal(JSONLD(ev, LocationAt(ev, venue, *at(1, 0, 0)), Organizer{}, nil))
	if strings.Contains(string(b), "Tresor") || strings.Contains(string(b), "Unterstraße") {
		t.Fatalf("withheld venue leaked into JSON-LD: %s", b)
	}
	ev.Status = StatusCancelled
	if JSONLD(ev, PublicLocation{}, Organizer{}, nil)["eventStatus"] != "https://schema.org/EventCancelled" {
		t.Error("cancelled status")
	}
}

func TestICS(t *testing.T) {
	ev := sampleEvent()
	ev.Title = "Klubnacht 03; closing, part 1"
	ics := ICS(ev, LocationAt(ev, venue, *at(1, 0, 0)), time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"UID:0190f1d2-7c1a-7a00-9f00-0000000000e1@promoter.klubhub", "SEQUENCE:2",
		"DTSTART:20261003T210000Z", "DTEND:20261004T050000Z", `SUMMARY:Klubnacht 03\; closing\, part 1`, "STATUS:CONFIRMED",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("ICS missing %q\n%s", want, ics)
		}
	}
	for _, line := range strings.Split(ics, "\r\n") {
		if len(line) > 75 {
			t.Errorf("unfolded line (%d octets): %q", len(line), line)
		}
	}
	ev.LocationMode = LocationCityOnly
	if strings.Contains(ICS(ev, LocationAt(ev, venue, *at(1, 0, 0)), time.Now()), "Tresor") {
		t.Error("city_only venue leaked into ICS")
	}
}

func TestPlainText(t *testing.T) {
	if got := PlainText("**Bold** and [a link](https://x.y) #tag"); got != "Bold and a link tag" {
		t.Fatalf("got %q", got)
	}
}

func ptrS(s string) *string { return &s }

func TestJSONLDOrganizerCarriesProfileLinks(t *testing.T) {
	ev := sampleEvent()
	org := Organizer{Name: "Nachtwerk", URL: "https://nachtwerk.example", SameAs: []string{"https://instagram.com/nachtwerk", "https://ra.co/promoters/1"}}
	o, _ := JSONLD(ev, LocationAt(ev, venue, *at(1, 0, 0)), org, nil)["organizer"].(map[string]any)
	if o["url"] != "https://nachtwerk.example" {
		t.Errorf("organizer url: %v", o)
	}
	if links, _ := o["sameAs"].([]string); len(links) != 2 {
		t.Errorf("organizer sameAs: %v", o)
	}
}
