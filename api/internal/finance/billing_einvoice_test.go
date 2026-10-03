package finance

import (
	"context"
	"strings"
	"testing"
	"time"
)

func validProfileRequest() UpdateBillingProfileRequest {
	return UpdateBillingProfileRequest{
		LegalName: "Lina Vasquez", EntityKind: EntityKindIndividual, ContactEmail: "billing@example.com",
		AddressLine1: "1 Sample Street", AddressCity: "Berlin", AddressPostal: "10115",
		AddressCountry: "DE", Jurisdiction: "DE", DefaultCurrency: "EUR", UpdatedAt: time.Now(),
	}
}

func fieldsOf(errs ValidationErrors) map[string]bool {
	out := map[string]bool{}
	for _, e := range errs {
		out[e.Field] = true
	}
	return out
}

func TestValidIBAN(t *testing.T) {
	for iban, want := range map[string]bool{
		"DE02120300000000202051":         true, // DKB sample
		"GB82WEST12345698765432":         true, // ECB sample
		"FR1420041010050500013M02606":    true,
		"DE02120300000000202052":         false, // checksum off by one
		"DE0212030000000020205":          false, // too short for DE (22)
		"XX0212030000000020205":          false, // unknown country
		"":                               false,
		"DE02 1203 0000 0000 2020 51":    false, // validIBAN expects the normalised form
		"de02120300000000202051":         false, // ... upper-case
		"DE02120300000000202051XXXXXXXX": false,
	} {
		if got := validIBAN(iban); got != want {
			t.Errorf("validIBAN(%q) = %v, want %v", iban, got, want)
		}
	}
}

func TestValidate_PaymentAndTaxNumberFields(t *testing.T) {
	ok := validProfileRequest()
	ok.TaxNumber, ok.IBAN, ok.BIC = "37/123/45678", "DE02120300000000202051", "BYLADEM1001"
	if errs := Validate(&ok); len(errs) != 0 {
		t.Fatalf("valid payment fields rejected: %v", errs)
	}

	empty := validProfileRequest() // all three optional
	if errs := Validate(&empty); len(errs) != 0 {
		t.Fatalf("empty payment fields rejected: %v", errs)
	}

	bad := validProfileRequest()
	bad.TaxNumber = strings.Repeat("9", 41)
	bad.IBAN = "DE02120300000000202052"
	bad.BIC = "NOTABIC"
	got := fieldsOf(Validate(&bad))
	for _, f := range []string{"tax_number", "iban", "bic"} {
		if !got[f] {
			t.Errorf("expected a %q error, got %v", f, got)
		}
	}
}

func TestService_Update_NormalisesIBANAndBIC(t *testing.T) {
	repo := newSeededRepo()
	svc := NewService(repo)

	req := validProfileRequest()
	req.UpdatedAt = repo.current.UpdatedAt
	req.IBAN = " de02 1203 0000 0000 2020 51 "
	req.BIC = " byladem1001 "
	req.TaxNumber = "  37/123/45678 "

	p, err := svc.Update(context.Background(), &req)
	if err != nil {
		t.Fatal(err)
	}
	if p.IBAN != "DE02120300000000202051" || p.BIC != "BYLADEM1001" || p.TaxNumber != "37/123/45678" {
		t.Errorf("stored iban=%q bic=%q tax_number=%q", p.IBAN, p.BIC, p.TaxNumber)
	}
}

func TestSnapshotFromProfile_CarriesPaymentAndTaxNumber(t *testing.T) {
	p := &BillingProfile{LegalName: "Lina", TaxNumber: "37/123/45678", IBAN: "DE02120300000000202051", BIC: "BYLADEM1001"}
	s := snapshotFromProfile(p)
	if s.TaxNumber != p.TaxNumber || s.IBAN != p.IBAN || s.BIC != p.BIC {
		t.Errorf("snapshot = %+v", s)
	}
}

func TestIntegration_BillingProfileEInvoiceFieldsRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	repo := NewRepository(testPool)
	ctx := context.Background()
	seeded, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := validProfileRequest()
	req.UpdatedAt = seeded.UpdatedAt
	req.TaxNumber, req.IBAN, req.BIC = "37/123/45678", "DE02120300000000202051", "BYLADEM1001"
	if _, err := repo.Update(ctx, &req); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaxNumber != req.TaxNumber || got.IBAN != req.IBAN || got.BIC != req.BIC {
		t.Errorf("round trip: tax_number=%q iban=%q bic=%q", got.TaxNumber, got.IBAN, got.BIC)
	}
}
