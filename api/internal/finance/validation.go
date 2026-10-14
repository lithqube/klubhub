package finance

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

var bicPattern = regexp.MustCompile(`^[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}([A-Z0-9]{3})?$`)

// ibanLengths maps an ISO 3166 country to its IBAN length (SEPA area plus the
// other common IBAN countries). A country not listed is rejected.
var ibanLengths = map[string]int{
	"AD": 24, "AE": 23, "AL": 28, "AT": 20, "AZ": 28, "BA": 20, "BE": 16, "BG": 22, "BH": 22, "BR": 29,
	"CH": 21, "CY": 28, "CZ": 24, "DE": 22, "DK": 18, "EE": 20, "ES": 24, "FI": 18, "FR": 27, "GB": 22,
	"GE": 22, "GI": 23, "GR": 27, "HR": 21, "HU": 28, "IE": 22, "IL": 23, "IS": 26, "IT": 27, "JO": 30,
	"KW": 30, "LB": 28, "LI": 21, "LT": 20, "LU": 20, "LV": 21, "MC": 27, "MD": 24, "ME": 22, "MK": 19,
	"MT": 31, "NL": 18, "NO": 15, "PL": 28, "PT": 25, "QA": 29, "RO": 24, "RS": 22, "SA": 24, "SE": 24,
	"SI": 19, "SK": 24, "SM": 27, "TR": 26, "UA": 29, "VA": 22, "XK": 20,
}

// normalizeBillingRequest trims free text and puts the IBAN and BIC in their
// canonical form (upper-case, no spaces) so storage and comparison are stable.
func normalizeBillingRequest(r *UpdateBillingProfileRequest) {
	r.TaxNumber = strings.TrimSpace(r.TaxNumber)
	compact := strings.NewReplacer(" ", "", "\t", "")
	r.IBAN = strings.ToUpper(compact.Replace(strings.TrimSpace(r.IBAN)))
	r.BIC = strings.ToUpper(compact.Replace(strings.TrimSpace(r.BIC)))
}

// validIBAN reports whether s, already normalised, is a well-formed IBAN: a
// known country with the right length and a valid ISO 7064 mod-97 checksum.
func validIBAN(s string) bool {
	if len(s) < 5 {
		return false
	}
	if n, ok := ibanLengths[s[:2]]; !ok || len(s) != n {
		return false
	}
	rem := 0
	for _, r := range s[4:] + s[:4] {
		switch {
		case r >= '0' && r <= '9':
			rem = (rem*10 + int(r-'0')) % 97
		case r >= 'A' && r <= 'Z':
			rem = (rem*100 + int(r-'A') + 10) % 97
		default:
			return false // lower-case, spaces and punctuation are not normalised input
		}
	}
	return rem == 1
}

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

	if utf8.RuneCountInString(r.TaxNumber) > 40 {
		errs = append(errs, FieldError{"tax_number", "exceeds 40 characters"})
	}
	if r.IBAN != "" && !validIBAN(r.IBAN) {
		errs = append(errs, FieldError{"iban", "not a valid IBAN (check the country, length and check digits)"})
	}
	if r.BIC != "" && !bicPattern.MatchString(r.BIC) {
		errs = append(errs, FieldError{"bic", "must be an 8 or 11 character BIC/SWIFT code"})
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
