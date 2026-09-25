package finance

import (
	"strings"
	"testing"
	"time"
)

// TestValidate_AcceptsMinimal confirms a minimal-valid profile passes.
func TestValidate_AcceptsMinimal(t *testing.T) {
	r := UpdateBillingProfileRequest{
		LegalName:       "Lina Vasquez",
		EntityKind:      EntityKindIndividual,
		ContactEmail:    "billing@example.com",
		AddressLine1:    "1 Sample Street",
		AddressCity:     "Berlin",
		AddressPostal:   "10115",
		Jurisdiction:    "DE",
		DefaultCurrency: "EUR",
		UpdatedAt:       time.Now(),
	}
	if errs := Validate(&r); len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
}

// TestValidate_RejectsMissingFields aggregates the required-field
// violations so we never silently allow partial profiles through.
func TestValidate_RejectsMissingFields(t *testing.T) {
	r := UpdateBillingProfileRequest{
		EntityKind:      EntityKindIndividual,
		DefaultCurrency: "EUR",
		UpdatedAt:       time.Now(),
	}
	errs := Validate(&r)
	if len(errs) == 0 {
		t.Fatalf("expected validation errors")
	}
	want := map[string]bool{
		"legal_name":       true,
		"contact_email":    true,
		"address_line1":    true,
		"address_city":     true,
		"address_postal":   true,
		"jurisdiction":     true,
		"default_currency": false, // present
	}
	for _, e := range errs {
		_, expected := want[e.Field]
		if !expected {
			t.Errorf("unexpected error field %q", e.Field)
		}
	}
	for field, required := range want {
		if !required {
			continue
		}
		found := false
		for _, e := range errs {
			if e.Field == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected validation error for %q", field)
		}
	}
}

// TestValidate_RejectsBadEmail protects the contact-email field from
// arbitrary strings. The error message must be safe to surface to the UI.
func TestValidate_RejectsBadEmail(t *testing.T) {
	r := UpdateBillingProfileRequest{
		LegalName:       "Lina Vasquez",
		EntityKind:      EntityKindIndividual,
		ContactEmail:    "no-at-symbol",
		AddressLine1:    "1 Sample Street",
		AddressCity:     "Berlin",
		AddressPostal:   "10115",
		Jurisdiction:    "DE",
		DefaultCurrency: "EUR",
		UpdatedAt:       time.Now(),
	}
	errs := Validate(&r)
	if len(errs) == 0 {
		t.Fatalf("expected email validation error")
	}
	if !strings.Contains(errs.Error(), "contact_email") {
		t.Errorf("expected contact_email in error message, got %q", errs.Error())
	}
}

// TestValidate_RejectsLongFields guards against storage abuse / DoS via
// huge fields. Lengths must be rejected explicitly, not truncated.
func TestValidate_RejectsLongFields(t *testing.T) {
	r := UpdateBillingProfileRequest{
		LegalName:           strings.Repeat("a", 201),
		EntityKind:          EntityKindIndividual,
		ContactEmail:        "x@y.com",
		AddressLine1:        "1 Sample Street",
		AddressCity:         "Berlin",
		AddressPostal:       "10115",
		Jurisdiction:        "DE",
		DefaultCurrency:     "EUR",
		PaymentInstructions: strings.Repeat("p", 2001),
		UpdatedAt:           time.Now(),
	}
	errs := Validate(&r)
	if len(errs) < 2 {
		t.Fatalf("expected at least two validation errors, got %d", len(errs))
	}
}

// TestMissingIssueFields checks the readiness gate lists every empty
// required field exactly once.
func TestMissingIssueFields(t *testing.T) {
	p := &BillingProfile{EntityKind: EntityKindIndividual, TaxIDKind: TaxIDKindEmpty}
	missing := MissingIssueFields(p)
	want := map[string]bool{
		"legal_name":       true,
		"contact_email":    true,
		"address_line1":    true,
		"address_city":     true,
		"address_postal":   true,
		"jurisdiction":     true,
		"default_currency": true,
	}
	if len(missing) != len(want) {
		t.Fatalf("missing=%v, want count %d", missing, len(want))
	}
	for _, f := range missing {
		if !want[f] {
			t.Errorf("unexpected missing field %q", f)
		}
	}
}
