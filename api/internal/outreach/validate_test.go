package outreach

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  DJ@Example.COM "); got != "dj@example.com" {
		t.Fatalf("NormalizeEmail = %q", got)
	}
}

func TestNormalizeCurrency(t *testing.T) {
	if got := NormalizeCurrency(" eur "); got != "EUR" {
		t.Fatalf("NormalizeCurrency = %q", got)
	}
}

func TestValidateSlotCreate(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	future := Date(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	today := Date(time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
	past := Date(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC))

	t.Run("valid normalizes in place", func(t *testing.T) {
		cur := "eur"
		in := &SlotCreate{
			Date:     future,
			City:     "  Berlin ",
			Country:  " Germany ",
			Genres:   []string{" techno ", "house"},
			FeeMin:   ptrFloat(100),
			FeeMax:   ptrFloat(200),
			Currency: &cur,
			Notes:    "  open deck  ",
		}
		if err := ValidateSlotCreate(in, now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if in.City != "Berlin" || in.Country != "Germany" || in.Notes != "open deck" {
			t.Fatalf("not normalized: %+v", in)
		}
		if *in.Currency != "EUR" {
			t.Fatalf("currency not normalized: %q", *in.Currency)
		}
		if in.Genres[0] != "techno" {
			t.Fatalf("genre not trimmed: %q", in.Genres[0])
		}
	})

	t.Run("today is accepted", func(t *testing.T) {
		in := &SlotCreate{Date: today, City: "Berlin"}
		if err := ValidateSlotCreate(in, now); err != nil {
			t.Fatalf("today must be accepted: %v", err)
		}
	})

	t.Run("past date rejected", func(t *testing.T) {
		in := &SlotCreate{Date: past, City: "Berlin"}
		err := ValidateSlotCreate(in, now)
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("missing city and date", func(t *testing.T) {
		in := &SlotCreate{}
		err := ValidateSlotCreate(in, now)
		ve, ok := err.(*ValidationError)
		if !ok {
			t.Fatalf("expected *ValidationError, got %T", err)
		}
		if ve.Fields["city"] == "" || ve.Fields["date"] == "" {
			t.Fatalf("expected city+date errors, got %v", ve.Fields)
		}
	})

	t.Run("fee range inverted", func(t *testing.T) {
		in := &SlotCreate{Date: future, City: "Berlin", FeeMin: ptrFloat(300), FeeMax: ptrFloat(200)}
		err := ValidateSlotCreate(in, now)
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("negative fee rejected", func(t *testing.T) {
		in := &SlotCreate{Date: future, City: "Berlin", FeeMin: ptrFloat(-1)}
		if err := ValidateSlotCreate(in, now); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("bad currency", func(t *testing.T) {
		cur := "eu"
		in := &SlotCreate{Date: future, City: "Berlin", Currency: &cur}
		if err := ValidateSlotCreate(in, now); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("too many genres", func(t *testing.T) {
		genres := make([]string, 13)
		for i := range genres {
			genres[i] = "g"
		}
		in := &SlotCreate{Date: future, City: "Berlin", Genres: genres}
		if err := ValidateSlotCreate(in, now); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("empty genre rejected", func(t *testing.T) {
		in := &SlotCreate{Date: future, City: "Berlin", Genres: []string{"  "}}
		if err := ValidateSlotCreate(in, now); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("nil genres become empty slice", func(t *testing.T) {
		in := &SlotCreate{Date: future, City: "Berlin"}
		if err := ValidateSlotCreate(in, now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if in.Genres == nil || len(in.Genres) != 0 {
			t.Fatalf("genres should be an empty slice, got %v", in.Genres)
		}
	})
}

func TestValidateTemplateCreate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		in := &TemplateCreate{Name: "  Default ", Subject: " Hi ", Body: " Body "}
		if err := ValidateTemplateCreate(in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if in.Name != "Default" || in.Subject != "Hi" || in.Body != "Body" {
			t.Fatalf("not normalized: %+v", in)
		}
	})

	t.Run("all required", func(t *testing.T) {
		err := ValidateTemplateCreate(&TemplateCreate{})
		ve, ok := err.(*ValidationError)
		if !ok {
			t.Fatalf("expected *ValidationError, got %T", err)
		}
		for _, f := range []string{"name", "subject", "body"} {
			if ve.Fields[f] == "" {
				t.Fatalf("expected %s error, got %v", f, ve.Fields)
			}
		}
	})

	t.Run("oversized body", func(t *testing.T) {
		in := &TemplateCreate{Name: "n", Subject: "s", Body: strings.Repeat("x", 20001)}
		if err := ValidateTemplateCreate(in); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})
}

func TestValidateTemplateUpdate(t *testing.T) {
	t.Run("version required", func(t *testing.T) {
		err := ValidateTemplateUpdate(&TemplateUpdate{})
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("nil fields pass", func(t *testing.T) {
		if err := ValidateTemplateUpdate(&TemplateUpdate{Version: 3}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		name := "   "
		err := ValidateTemplateUpdate(&TemplateUpdate{Version: 1, Name: &name})
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("pointer patch normalizes", func(t *testing.T) {
		subject := "  New subject "
		in := &TemplateUpdate{Version: 2, Subject: &subject}
		if err := ValidateTemplateUpdate(in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *in.Subject != "New subject" {
			t.Fatalf("subject not trimmed: %q", *in.Subject)
		}
	})
}

func TestValidateSendRequest(t *testing.T) {
	t.Run("contact required", func(t *testing.T) {
		err := ValidateSendRequest(&SendRequestInput{})
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("valid with overrides", func(t *testing.T) {
		subject := "Custom"
		in := &SendRequestInput{ContactID: uuid.New(), Subject: &subject}
		if err := ValidateSendRequest(in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("oversized override", func(t *testing.T) {
		body := strings.Repeat("x", 20001)
		in := &SendRequestInput{ContactID: uuid.New(), Body: &body}
		if err := ValidateSendRequest(in); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})
}

func TestValidateSuppression(t *testing.T) {
	t.Run("valid normalizes", func(t *testing.T) {
		in := &SuppressionCreate{Email: "  DJ@Example.COM ", Reason: " asked "}
		if err := ValidateSuppression(in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if in.Email != "dj@example.com" || in.Reason != "asked" {
			t.Fatalf("not normalized: %+v", in)
		}
	})

	t.Run("invalid email", func(t *testing.T) {
		in := &SuppressionCreate{Email: "not-an-email"}
		if err := ValidateSuppression(in); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("empty email", func(t *testing.T) {
		in := &SuppressionCreate{Email: "   "}
		if err := ValidateSuppression(in); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})
}

func TestFormatFee(t *testing.T) {
	cur := "EUR"
	cases := []struct {
		name string
		slot *Slot
		want string
	}{
		{"none", &Slot{}, ""},
		{"equal", &Slot{FeeMin: ptrFloat(150), FeeMax: ptrFloat(150), Currency: &cur}, "150 EUR"},
		{"range", &Slot{FeeMin: ptrFloat(100), FeeMax: ptrFloat(250.5), Currency: &cur}, "100–250.5 EUR"},
		{"min only", &Slot{FeeMin: ptrFloat(80)}, "from 80"},
		{"max only", &Slot{FeeMax: ptrFloat(300)}, "up to 300"},
		{"no currency", &Slot{FeeMin: ptrFloat(100), FeeMax: ptrFloat(200)}, "100–200"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatFee(tc.slot); got != tc.want {
				t.Fatalf("formatFee = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	err := &ValidationError{Fields: map[string]string{"b": "two", "a": "one"}}
	want := "validation failed: a: one; b: two"
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatal("ValidationError must satisfy errors.Is(err, ErrValidation)")
	}
}

func ptrFloat(v float64) *float64 { return &v }
