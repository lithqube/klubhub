package finance

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

// RequiredFieldsForIssue returns the field names that must be non-empty
// before any document (invoice or agreement) can be issued. The list is
// deliberately separate from Validate so that callers can distinguish
// "the profile is malformed" (reject at PUT) from "the profile is not
// ready to issue yet" (allow at PUT, block at issue).
func RequiredFieldsForIssue() []string {
	return []string{
		"legal_name",
		"contact_email",
		"address_line1",
		"address_city",
		"address_postal",
		"jurisdiction",
		"default_currency",
	}
}

// Validate performs a strict shape check on a profile (request form).
// Empty / malformed fields are reported as FieldErrors wrapped in
// ValidationErrors. Returning an empty slice means the request is valid.
func Validate(r *UpdateBillingProfileRequest) ValidationErrors {
	var errs ValidationErrors
	// Trim once for length / equality checks below.
	legalName := strings.TrimSpace(r.LegalName)
	contactEmail := strings.TrimSpace(r.ContactEmail)
	addressLine1 := strings.TrimSpace(r.AddressLine1)
	addressCity := strings.TrimSpace(r.AddressCity)
	addressPostal := strings.TrimSpace(r.AddressPostal)
	jurisdiction := strings.TrimSpace(r.Jurisdiction)
	defaultCurrency := strings.TrimSpace(r.DefaultCurrency)

	if legalName == "" {
		errs = append(errs, FieldError{"legal_name", "required"})
	} else if utf8.RuneCountInString(legalName) > 200 {
		errs = append(errs, FieldError{"legal_name", "exceeds 200 characters"})
	}

	if contactEmail == "" {
		errs = append(errs, FieldError{"contact_email", "required"})
	} else if _, err := mail.ParseAddress(contactEmail); err != nil {
		errs = append(errs, FieldError{"contact_email", fmt.Sprintf("not a valid email: %v", err)})
	}

	if addressLine1 == "" {
		errs = append(errs, FieldError{"address_line1", "required"})
	}
	if addressCity == "" {
		errs = append(errs, FieldError{"address_city", "required"})
	}
	if addressPostal == "" {
		errs = append(errs, FieldError{"address_postal", "required"})
	}
	if jurisdiction == "" {
		errs = append(errs, FieldError{"jurisdiction", "required"})
	}

	if defaultCurrency == "" {
		errs = append(errs, FieldError{"default_currency", "required"})
	} else if len(defaultCurrency) != 3 {
		errs = append(errs, FieldError{"default_currency", "must be a 3-letter ISO 4217 code"})
	} else {
		for _, r := range defaultCurrency {
			if r < 'A' || r > 'Z' {
				errs = append(errs, FieldError{"default_currency", "must be uppercase A-Z letters"})
				break
			}
		}
	}

	if _, ok := ValidEntityKinds[r.EntityKind]; !ok {
		errs = append(errs, FieldError{"entity_kind", "must be one of individual, sole_trader, partnership, llc, corp, other"})
	}
	if _, ok := ValidTaxIDKinds[r.TaxIDKind]; !ok {
		errs = append(errs, FieldError{"tax_id_kind", "must be one of '', vat, ein, gst, abn, other"})
	}

	if len(r.AddressCountry) != 0 && len(r.AddressCountry) != 2 {
		errs = append(errs, FieldError{"address_country", "must be a 2-letter ISO 3166-1 alpha-2 code"})
	}
	for _, r := range r.AddressCountry {
		if r < 'A' || r > 'Z' {
			errs = append(errs, FieldError{"address_country", "must be uppercase A-Z letters"})
			break
		}
	}

	if r.DefaultVATRateBps < 0 || r.DefaultVATRateBps > 10000 {
		errs = append(errs, FieldError{"default_vat_rate_bps", "must be between 0 and 10000 basis points"})
	}

	if utf8.RuneCountInString(r.TradingName) > 200 {
		errs = append(errs, FieldError{"trading_name", "exceeds 200 characters"})
	}
	if utf8.RuneCountInString(r.TaxID) > 64 {
		errs = append(errs, FieldError{"tax_id", "exceeds 64 characters"})
	}
	if utf8.RuneCountInString(r.PaymentInstructions) > 2000 {
		errs = append(errs, FieldError{"payment_instructions", "exceeds 2000 characters"})
	}
	if utf8.RuneCountInString(r.AddressLine2) > 200 {
		errs = append(errs, FieldError{"address_line2", "exceeds 200 characters"})
	}
	if utf8.RuneCountInString(r.AddressRegion) > 100 {
		errs = append(errs, FieldError{"address_region", "exceeds 100 characters"})
	}
	if utf8.RuneCountInString(r.ContactPhone) > 50 {
		errs = append(errs, FieldError{"contact_phone", "exceeds 50 characters"})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// MissingIssueFields returns the names of any field that is required for
// issuing documents but currently empty/whitespace in p.
func MissingIssueFields(p *BillingProfile) []string {
	missing := []string{}
	if strings.TrimSpace(p.LegalName) == "" {
		missing = append(missing, "legal_name")
	}
	if strings.TrimSpace(p.ContactEmail) == "" {
		missing = append(missing, "contact_email")
	}
	if strings.TrimSpace(p.AddressLine1) == "" {
		missing = append(missing, "address_line1")
	}
	if strings.TrimSpace(p.AddressCity) == "" {
		missing = append(missing, "address_city")
	}
	if strings.TrimSpace(p.AddressPostal) == "" {
		missing = append(missing, "address_postal")
	}
	if strings.TrimSpace(p.Jurisdiction) == "" {
		missing = append(missing, "jurisdiction")
	}
	if strings.TrimSpace(p.DefaultCurrency) == "" {
		missing = append(missing, "default_currency")
	}
	return missing
}
