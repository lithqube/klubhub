package outreach

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/email"
)

func TestDateJSONRoundTrip(t *testing.T) {
	in := Date(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"2026-11-01"` {
		t.Fatalf("Marshal = %s", data)
	}
	var out Date
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if time.Time(out).UTC().Format(DateFormat) != "2026-11-01" {
		t.Fatalf("round-trip = %v", time.Time(out))
	}
}

func TestDateUnmarshalRejectsBadInput(t *testing.T) {
	var d Date
	if err := json.Unmarshal([]byte(`"01/11/2026"`), &d); err == nil {
		t.Fatal("expected an error for a non-ISO date")
	}
	if err := json.Unmarshal([]byte(`42`), &d); err == nil {
		t.Fatal("expected an error for a non-string date")
	}
}

func TestSlotJSONShape(t *testing.T) {
	// The wire shape the frontend consumes: snake_case, date as YYYY-MM-DD,
	// nullable fee/currency omitted when unset.
	slot := Slot{
		ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Date:      Date(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)),
		City:      "Berlin",
		Country:   "DE",
		Genres:    []string{"techno"},
		Notes:     "open deck",
		CreatedAt: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(slot)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m["date"] != "2026-11-01" {
		t.Fatalf("date = %v", m["date"])
	}
	if _, ok := m["fee_min"]; ok {
		t.Fatal("fee_min must be omitted when unset")
	}
	if _, ok := m["currency"]; ok {
		t.Fatal("currency must be omitted when unset")
	}
	if m["city"] != "Berlin" || m["country"] != "DE" {
		t.Fatalf("city/country = %v/%v", m["city"], m["country"])
	}
}

func TestMatchedContactHasEmailFlag(t *testing.T) {
	// OUTR-05 v2.1: contacts without an email are matchable but flagged.
	c := MatchedContact{ID: uuid.New(), Name: "No Email", Email: "", HasEmail: false, City: "Berlin"}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m["has_email"] != false {
		t.Fatalf("has_email = %v", m["has_email"])
	}
}

func TestBookingRequestKindRegistered(t *testing.T) {
	if !EmailKindBookingRequest.IsValid() {
		t.Fatal("booking_request kind must be registered")
	}
	if EmailKindBookingRequest != email.EmailKind("booking_request") {
		t.Fatalf("kind = %q", EmailKindBookingRequest)
	}
	// Unknown kinds are still rejected by the shared validator.
	if email.EmailKind("not_a_kind").IsValid() {
		t.Fatal("unknown kind must not validate")
	}
}
