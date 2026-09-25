package contact

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// ValidationError lists the invalid fields of a contact write. It satisfies
// errors.Is(err, ErrValidation).
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range sortedKeys(e.Fields) {
		parts = append(parts, f+": "+e.Fields[f])
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Is reports whether target is ErrValidation.
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// NormalizeCountry trims and upper-cases an ISO 3166-1 alpha-2 code.
func NormalizeCountry(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// NormalizeTaxNumber trims, upper-cases and removes inner spaces/dots from a
// VAT or tax id, the form tax authorities validate.
func NormalizeTaxNumber(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	return strings.NewReplacer(" ", "", ".", "").Replace(v)
}

// validatePartyValue records a problem for one party field (already
// normalized). Shared by create and update.
func validatePartyValue(errs map[string]string, field, value string) {
	limits := map[string]int{
		"address_line1": 200, "address_line2": 200, "city": 100, "region": 100,
		"postal_code": 20, "vat_id": 20, "tax_id": 40,
	}
	if field == "country" {
		if value != "" && !isAlpha2(value) {
			errs[field] = "must be a 2-letter ISO 3166-1 alpha-2 code"
		}
		return
	}
	if max, ok := limits[field]; ok && utf8.RuneCountInString(value) > max {
		errs[field] = fmt.Sprintf("exceeds %d characters", max)
	}
}

func isAlpha2(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// NormalizeCreate normalizes the party fields of c in place and validates
// the request.
func NormalizeCreate(c *ContactCreate) error {
	errs := map[string]string{}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		errs["name"] = "required"
	} else if utf8.RuneCountInString(c.Name) > 200 {
		errs["name"] = "exceeds 200 characters"
	}
	if c.Type == "" {
		c.Type = ContactTypeOther
	}
	if !c.Type.IsValid() {
		errs["type"] = "must be one of promoter, agent, label, other"
	}
	p := &c.PartyFields
	p.Country = NormalizeCountry(p.Country)
	p.VATID = NormalizeTaxNumber(p.VATID)
	p.TaxID = strings.TrimSpace(p.TaxID)
	for field, v := range map[string]string{
		"address_line1": p.AddressLine1, "address_line2": p.AddressLine2, "city": p.City,
		"region": p.Region, "postal_code": p.PostalCode, "country": p.Country,
		"vat_id": p.VATID, "tax_id": p.TaxID,
	} {
		validatePartyValue(errs, field, v)
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// NormalizeUpdate normalizes the non-nil party fields of u in place and
// validates the request.
func NormalizeUpdate(u *ContactUpdate) error {
	errs := map[string]string{}
	if u.Name != nil {
		n := strings.TrimSpace(*u.Name)
		u.Name = &n
		if n == "" {
			errs["name"] = "required"
		}
	}
	if u.Type != nil && !u.Type.IsValid() {
		errs["type"] = "must be one of promoter, agent, label, other"
	}
	if u.Country != nil {
		c := NormalizeCountry(*u.Country)
		u.Country = &c
	}
	if u.VATID != nil {
		v := NormalizeTaxNumber(*u.VATID)
		u.VATID = &v
	}
	for field, v := range map[string]*string{
		"address_line1": u.AddressLine1, "address_line2": u.AddressLine2, "city": u.City,
		"region": u.Region, "postal_code": u.PostalCode, "country": u.Country,
		"vat_id": u.VATID, "tax_id": u.TaxID,
	} {
		if v != nil {
			validatePartyValue(errs, field, *v)
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// IsValid reports whether t is a known contact type.
func (t ContactType) IsValid() bool {
	switch t {
	case ContactTypePromoter, ContactTypeAgent, ContactTypeLabel, ContactTypeOther:
		return true
	}
	return false
}
