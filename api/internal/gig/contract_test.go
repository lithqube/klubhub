package gig

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestGigJSONContractUsesExactSnakeCaseKeys(t *testing.T) {
	g := Gig{
		ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Date: time.Date(2026, 9, 19, 20, 0, 0, 0, time.UTC),
		Venue: "Legacy venue text",
		City: "Berlin",
		Country: "DE",
		EventName: "Night Shift",
		PromoterName: "Legacy promoter text",
		PromoterEmail: "promoter@example.com",
		PromoterPhone: "+49123",
		FeeAmount: decimal.NewFromInt(500),
		FeeCurrency: "EUR",
		SetLengthMinutes: 90,
		Notes: "notes",
		Status: GigStatusConfirmed,
		PaymentStatus: PaymentStatusDepositPaid,
		CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	want := []string{
		"id", "date", "venue", "city", "country", "event_name",
		"promoter_name", "promoter_email", "promoter_phone", "fee_amount",
		"fee_currency", "set_length_minutes", "notes", "status", "payment_status",
		"gig_reader_venue_id", "gig_reader_contact_id", "created_at", "updated_at", "deleted_at",
	}
	if len(got) != len(want) {
		t.Fatalf("JSON keys = %v; want exactly %v", mapKeys(got), want)
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("missing JSON key %q; got %v", key, mapKeys(got))
		}
	}
}

func TestGigDetailJSONIncludesRelationshipsAndPreservesLegacyFields(t *testing.T) {
	venueID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	contactID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	tracklistID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	detail := GigDetailResponse{
		Gig: Gig{Venue: "Legacy venue text", PromoterName: "Legacy promoter text"},
		Venues: []LinkedVenueResponse{{Venue: VenueResponse{ID: venueID, Name: "Linked Venue"}, IsPrimary: true}},
		Contacts: []LinkedContactResponse{{Contact: ContactResponse{ID: contactID, Name: "Linked Contact"}, Role: "promoter"}},
		Tracklists: []TracklistResponse{{ID: tracklistID, Title: "Closing Set"}},
	}

	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{"venue", "promoter_name", "venues", "contacts", "tracklists"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing top-level key %q: %s", key, data)
		}
	}
	if string(got["venue"]) != `"Legacy venue text"` || string(got["promoter_name"]) != `"Legacy promoter text"` {
		t.Fatalf("legacy free-text fields changed: %s", data)
	}

	var venues []map[string]json.RawMessage
	if err := json.Unmarshal(got["venues"], &venues); err != nil {
		t.Fatal(err)
	}
	if _, ok := venues[0]["is_primary"]; !ok {
		t.Fatalf("linked venue is missing is_primary: %s", got["venues"])
	}
	var contacts []map[string]json.RawMessage
	if err := json.Unmarshal(got["contacts"], &contacts); err != nil {
		t.Fatal(err)
	}
	if _, ok := contacts[0]["role"]; !ok {
		t.Fatalf("linked contact is missing role: %s", got["contacts"])
	}
}

func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
