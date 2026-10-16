package bookings

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func wantInvalid(t *testing.T, err error, field string) {
	t.Helper()
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("want *InvalidError, got %v", err)
	}
	if inv.Field != field {
		t.Fatalf("want field %q, got %q (%v)", field, inv.Field, inv)
	}
}

func TestValidateTalentContact_NormalizesAndDefaults(t *testing.T) {
	in := TalentContactInput{
		Name: "  Nadia Voss  ", Email: "  Nadia@Example.com ", Phone: " +49 30 1 ",
		City: " Berlin ", Country: " DE ", Genres: []string{" techno ", "house"}, Notes: " met at Berghain ",
	}
	if err := validateTalentContact(&in); err != nil {
		t.Fatal(err)
	}
	if in.Name != "Nadia Voss" || in.Email != "Nadia@Example.com" || in.Phone != "+49 30 1" {
		t.Fatalf("not trimmed: %+v", in)
	}
	if in.City != "Berlin" || in.Country != "DE" || in.Notes != "met at Berghain" {
		t.Fatalf("not trimmed: %+v", in)
	}
	if in.Source != SourceManual {
		t.Fatalf("empty source must default to manual, got %q", in.Source)
	}
	if len(in.Genres) != 2 || in.Genres[0] != "techno" {
		t.Fatalf("genres not normalized: %+v", in.Genres)
	}
}

func TestValidateTalentContact_NameOnlyIsAllowed(t *testing.T) {
	in := TalentContactInput{Name: "Resident DJ"}
	if err := validateTalentContact(&in); err != nil {
		t.Fatal(err)
	}
	if in.Email != "" {
		t.Fatalf("email must stay empty, got %q", in.Email)
	}
}

func TestValidateTalentContact_RequiresNameOrEmail(t *testing.T) {
	in := TalentContactInput{}
	wantInvalid(t, validateTalentContact(&in), "name")
}

func TestValidateTalentContact_RejectsBadEmail(t *testing.T) {
	in := TalentContactInput{Name: "X", Email: "not-an-email"}
	wantInvalid(t, validateTalentContact(&in), "email")
	in.Email = "two@@example.com"
	wantInvalid(t, validateTalentContact(&in), "email")
	in.Email = strings.Repeat("a", 250) + "@example.com"
	wantInvalid(t, validateTalentContact(&in), "email")
}

func TestValidateTalentContact_LengthCaps(t *testing.T) {
	long := strings.Repeat("x", maxNameLen+1)
	in := TalentContactInput{Name: long}
	wantInvalid(t, validateTalentContact(&in), "name")

	in = TalentContactInput{Name: "X", Phone: strings.Repeat("x", maxPhoneLen+1)}
	wantInvalid(t, validateTalentContact(&in), "phone")

	in = TalentContactInput{Name: "X", City: strings.Repeat("x", maxCityLen+1)}
	wantInvalid(t, validateTalentContact(&in), "city")

	in = TalentContactInput{Name: "X", Country: strings.Repeat("x", maxCityLen+1)}
	wantInvalid(t, validateTalentContact(&in), "country")

	in = TalentContactInput{Name: "X", Notes: strings.Repeat("x", maxNotesLen+1)}
	wantInvalid(t, validateTalentContact(&in), "notes")
}

func TestValidateTalentContact_Genres(t *testing.T) {
	in := TalentContactInput{Name: "X"}
	if err := validateTalentContact(&in); err != nil {
		t.Fatal(err)
	}
	if in.Genres == nil || len(in.Genres) != 0 {
		t.Fatalf("nil genres must normalize to an empty slice, got %#v", in.Genres)
	}

	in = TalentContactInput{Name: "X", Genres: []string{""}}
	wantInvalid(t, validateTalentContact(&in), "genres")

	in = TalentContactInput{Name: "X", Genres: []string{strings.Repeat("g", maxGenreLen+1)}}
	wantInvalid(t, validateTalentContact(&in), "genres")

	tooMany := make([]string, maxGenreCount+1)
	for i := range tooMany {
		tooMany[i] = "techno"
	}
	in = TalentContactInput{Name: "X", Genres: tooMany}
	wantInvalid(t, validateTalentContact(&in), "genres")
}

func TestValidateTalentContact_SourceEnum(t *testing.T) {
	for _, s := range Sources {
		in := TalentContactInput{Name: "X", Source: s}
		if err := validateTalentContact(&in); err != nil {
			t.Fatalf("source %q must be valid: %v", s, err)
		}
	}
	in := TalentContactInput{Name: "X", Source: "scraped"}
	wantInvalid(t, validateTalentContact(&in), "source")
}

func TestValidateOpenSlot(t *testing.T) {
	min, max := 500.0, 800.0
	cur := "eur"
	in := OpenSlotInput{Role: " Headliner ", Genres: []string{"techno"}, FeeMin: &min, FeeMax: &max, Currency: &cur, Notes: " PA included "}
	if err := validateOpenSlot(&in); err != nil {
		t.Fatal(err)
	}
	if in.Role != "Headliner" || in.Notes != "PA included" {
		t.Fatalf("not trimmed: %+v", in)
	}
	if in.Currency == nil || *in.Currency != "EUR" {
		t.Fatalf("currency must normalize to upper case, got %v", in.Currency)
	}

	// Empty role is fine (a general "play this night" slot).
	in = OpenSlotInput{}
	if err := validateOpenSlot(&in); err != nil {
		t.Fatal(err)
	}

	neg := -1.0
	in = OpenSlotInput{FeeMin: &neg}
	wantInvalid(t, validateOpenSlot(&in), "fee_min")
	in = OpenSlotInput{FeeMax: &neg}
	wantInvalid(t, validateOpenSlot(&in), "fee_max")

	lo, hi := 900.0, 100.0
	in = OpenSlotInput{FeeMin: &lo, FeeMax: &hi}
	wantInvalid(t, validateOpenSlot(&in), "fee_min")

	bad := "EURO"
	in = OpenSlotInput{Currency: &bad}
	wantInvalid(t, validateOpenSlot(&in), "currency")
	short := "EU"
	in = OpenSlotInput{Currency: &short}
	wantInvalid(t, validateOpenSlot(&in), "currency")
	digits := "E1R"
	in = OpenSlotInput{Currency: &digits}
	wantInvalid(t, validateOpenSlot(&in), "currency")

	in = OpenSlotInput{Role: strings.Repeat("r", maxRoleLen+1)}
	wantInvalid(t, validateOpenSlot(&in), "role")
	in = OpenSlotInput{Notes: strings.Repeat("n", maxNotesLen+1)}
	wantInvalid(t, validateOpenSlot(&in), "notes")
	in = OpenSlotInput{Genres: []string{""}}
	wantInvalid(t, validateOpenSlot(&in), "genres")
}

func TestValidateTemplateCreate(t *testing.T) {
	in := TemplateCreate{Name: "  Offer A ", Subject: " Hi ", Body: " Body "}
	if err := validateTemplateCreate(&in); err != nil {
		t.Fatal(err)
	}
	if in.Name != "Offer A" || in.Subject != "Hi" || in.Body != "Body" {
		t.Fatalf("not trimmed: %+v", in)
	}

	wantInvalid(t, validateTemplateCreate(&TemplateCreate{Subject: "s", Body: "b"}), "name")
	wantInvalid(t, validateTemplateCreate(&TemplateCreate{Name: "n", Body: "b"}), "subject")
	wantInvalid(t, validateTemplateCreate(&TemplateCreate{Name: "n", Subject: "s"}), "body")
	wantInvalid(t, validateTemplateCreate(&TemplateCreate{Name: strings.Repeat("n", maxTplNameLen+1), Subject: "s", Body: "b"}), "name")
	wantInvalid(t, validateTemplateCreate(&TemplateCreate{Name: "n", Subject: strings.Repeat("s", maxSubjectLen+1), Body: "b"}), "subject")
	wantInvalid(t, validateTemplateCreate(&TemplateCreate{Name: "n", Subject: "s", Body: strings.Repeat("b", maxBodyLen+1)}), "body")
}

func TestValidateTemplateUpdate(t *testing.T) {
	// Version is the optimistic-concurrency token and must be present.
	wantInvalid(t, validateTemplateUpdate(&TemplateUpdate{}), "version")

	// All-nil fields with a version is a valid no-op patch.
	if err := validateTemplateUpdate(&TemplateUpdate{Version: 3}); err != nil {
		t.Fatal(err)
	}

	name := "  New name "
	upd := TemplateUpdate{Version: 1, Name: &name}
	if err := validateTemplateUpdate(&upd); err != nil {
		t.Fatal(err)
	}
	if *upd.Name != "New name" {
		t.Fatalf("name not trimmed: %q", *upd.Name)
	}

	empty := ""
	wantInvalid(t, validateTemplateUpdate(&TemplateUpdate{Version: 1, Name: &empty}), "name")
	wantInvalid(t, validateTemplateUpdate(&TemplateUpdate{Version: 1, Subject: &empty}), "subject")
	wantInvalid(t, validateTemplateUpdate(&TemplateUpdate{Version: 1, Body: &empty}), "body")
	long := strings.Repeat("x", maxSubjectLen+1)
	wantInvalid(t, validateTemplateUpdate(&TemplateUpdate{Version: 1, Subject: &long}), "subject")
}

func TestValidateSendOffer(t *testing.T) {
	wantInvalid(t, validateSendOffer(&SendOfferInput{}), "talent_contact_id")

	in := SendOfferInput{TalentContactID: uuid.New()}
	if err := validateSendOffer(&in); err != nil {
		t.Fatal(err)
	}

	subj := strings.Repeat("s", maxSubjectLen+1)
	in.Subject = &subj
	wantInvalid(t, validateSendOffer(&in), "subject")
	in.Subject = nil
	body := strings.Repeat("b", maxBodyLen+1)
	in.Body = &body
	wantInvalid(t, validateSendOffer(&in), "body")
}

func TestValidateSuppression(t *testing.T) {
	in := SuppressionCreate{Email: "  DJ@Example.com ", Reason: " asked to stop "}
	if err := validateSuppression(&in); err != nil {
		t.Fatal(err)
	}
	if in.Email != "DJ@Example.com" || in.Reason != "asked to stop" {
		t.Fatalf("not trimmed: %+v", in)
	}

	wantInvalid(t, validateSuppression(&SuppressionCreate{}), "email")
	wantInvalid(t, validateSuppression(&SuppressionCreate{Email: "nope"}), "email")
	wantInvalid(t, validateSuppression(&SuppressionCreate{Email: strings.Repeat("a", 250) + "@x.co"}), "email")
	wantInvalid(t, validateSuppression(&SuppressionCreate{Email: "a@b.co", Reason: strings.Repeat("r", maxReasonLen+1)}), "reason")
}

func TestFormatFee(t *testing.T) {
	eur := "EUR"
	cases := []struct {
		name string
		slot OpenSlot
		want string
	}{
		{"equal min and max", OpenSlot{FeeMin: ptr(500.0), FeeMax: ptr(500.0), Currency: &eur}, "500 EUR"},
		{"range", OpenSlot{FeeMin: ptr(500.0), FeeMax: ptr(800.0), Currency: &eur}, "500–800 EUR"},
		{"min only", OpenSlot{FeeMin: ptr(500.0), Currency: &eur}, "from 500 EUR"},
		{"max only", OpenSlot{FeeMax: ptr(800.0), Currency: &eur}, "up to 800 EUR"},
		{"no fee", OpenSlot{Currency: &eur}, ""},
		{"no currency", OpenSlot{FeeMin: ptr(500.0), FeeMax: ptr(500.0)}, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatFee(&tc.slot); got != tc.want {
				t.Fatalf("formatFee = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[float64]string{500: "500", 500.5: "500.5", 500.25: "500.25", 0: "0", 1234.99: "1234.99"}
	for in, want := range cases {
		if got := formatMoney(in); got != want {
			t.Fatalf("formatMoney(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalMidnightIn(t *testing.T) {
	// 2026-10-15 12:00 UTC is 14:00 in Berlin (CEST, UTC+2): local midnight is
	// 2026-10-14 22:00 UTC.
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	mid := localMidnightIn(at, "Europe/Berlin")
	want := time.Date(2026, 10, 14, 22, 0, 0, 0, time.UTC)
	if !mid.Equal(want) {
		t.Fatalf("localMidnightIn = %s, want %s", mid, want)
	}
	if mid.Location().String() != "Europe/Berlin" {
		t.Fatalf("midnight must carry the org location, got %s", mid.Location())
	}

	// An unknown timezone falls back to UTC rather than failing the send.
	mid = localMidnightIn(at, "Mars/Olympus")
	if !mid.Equal(time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unknown tz must fall back to UTC midnight, got %s", mid)
	}
}

func ptr(v float64) *float64 { return &v }

func sptr(v string) *string { return &v }
