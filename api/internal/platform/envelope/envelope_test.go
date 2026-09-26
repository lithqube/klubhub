package envelope

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func testKEK(t *testing.T, id string) *KEK {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	k, err := NewKEK(id, raw)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestNewKEKRejectsWrongLengthsAndIDs(t *testing.T) {
	if _, err := NewKEK("k1", make([]byte, 16)); !errors.Is(err, ErrKeyLength) {
		t.Fatalf("16-byte KEK must be rejected, got %v", err)
	}
	if _, err := NewKEK("", make([]byte, 32)); err == nil {
		t.Fatal("empty KEK id must be rejected")
	}
	if _, err := NewKEK("has|pipe", make([]byte, 32)); err == nil {
		t.Fatal("KEK id with a separator must be rejected")
	}
}

func TestWrapUnwrapDEKIsBoundToTenantAndKEK(t *testing.T) {
	kek := testKEK(t, "kek-2026-09")
	tenant := uuid.New()
	dek, wrapped, err := GenerateDEK(kek, tenant, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := kek.UnwrapDEK(tenant, 1, wrapped)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	f := Field{Table: "t", Column: "c", RowID: uuid.New()}
	ct, _ := dek.Seal(tenant, f, []byte("secret"))
	if pt, err := got.Open(tenant, f, ct); err != nil || string(pt) != "secret" {
		t.Fatalf("unwrapped DEK must open what the original sealed: %q %v", pt, err)
	}
	if _, err := kek.UnwrapDEK(uuid.New(), 1, wrapped); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("a DEK must not unwrap for another tenant, got %v", err)
	}
	if _, err := kek.UnwrapDEK(tenant, 2, wrapped); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("a DEK must not unwrap as another key version, got %v", err)
	}
	if _, err := testKEK(t, "kek-2026-09").UnwrapDEK(tenant, 1, wrapped); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("a DEK must not unwrap with another KEK, got %v", err)
	}
}

func TestFieldRoundTripAndBinding(t *testing.T) {
	kek := testKEK(t, "k")
	tenant := uuid.New()
	dek, _, _ := GenerateDEK(kek, tenant, 3)
	aad := Field{Table: "guests", Column: "name_enc", RowID: uuid.New()}

	ct, err := dek.Seal(tenant, aad, []byte("Anna Kowalska"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("Anna")) {
		t.Fatal("ciphertext leaks plaintext")
	}
	if v, err := KeyVersionOf(ct); err != nil || v != 3 {
		t.Fatalf("key version must be readable from the ciphertext header, got %d %v", v, err)
	}
	pt, err := dek.Open(tenant, aad, ct)
	if err != nil || string(pt) != "Anna Kowalska" {
		t.Fatalf("round trip failed: %q %v", pt, err)
	}

	// Moving ciphertext to another row, column, table or tenant must fail.
	for name, other := range map[string]Field{
		"row":    {Table: aad.Table, Column: aad.Column, RowID: uuid.New()},
		"column": {Table: aad.Table, Column: "email_enc", RowID: aad.RowID},
		"table":  {Table: "contacts", Column: aad.Column, RowID: aad.RowID},
	} {
		if _, err := dek.Open(tenant, other, ct); !errors.Is(err, ErrDecrypt) {
			t.Errorf("ciphertext moved to another %s must not decrypt, got %v", name, err)
		}
	}
	if _, err := dek.Open(uuid.New(), aad, ct); !errors.Is(err, ErrDecrypt) {
		t.Error("ciphertext must not decrypt for another tenant")
	}

	// Tampering is detected.
	bad := append([]byte(nil), ct...)
	bad[len(bad)-1] ^= 0x01
	if _, err := dek.Open(tenant, aad, bad); !errors.Is(err, ErrDecrypt) {
		t.Errorf("tampered ciphertext must fail, got %v", err)
	}
	if _, err := dek.Open(tenant, aad, ct[:5]); !errors.Is(err, ErrDecrypt) {
		t.Errorf("truncated ciphertext must fail, got %v", err)
	}
}

func TestSealIsRandomised(t *testing.T) {
	kek := testKEK(t, "k")
	tenant := uuid.New()
	dek, _, _ := GenerateDEK(kek, tenant, 1)
	f := Field{Table: "t", Column: "c", RowID: uuid.New()}
	a, _ := dek.Seal(tenant, f, []byte("same"))
	b, _ := dek.Seal(tenant, f, []byte("same"))
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of the same value must differ (random nonce)")
	}
}

func TestBlindIndexIsDeterministicScopedAndNormalised(t *testing.T) {
	kek := testKEK(t, "k")
	tenant := uuid.New()
	dek, _, _ := GenerateDEK(kek, tenant, 1)

	a := dek.BlindIndex("audience_contacts", "email", NormalizeEmail("  Anna@Example.COM "))
	b := dek.BlindIndex("audience_contacts", "email", NormalizeEmail("anna@example.com"))
	if !bytes.Equal(a, b) {
		t.Fatal("blind index must be stable for equivalent emails")
	}
	if bytes.Equal(a, dek.BlindIndex("guests", "email", NormalizeEmail("anna@example.com"))) {
		t.Fatal("blind index must differ per table/column")
	}
	other, _, _ := GenerateDEK(kek, uuid.New(), 1)
	if bytes.Equal(a, other.BlindIndex("audience_contacts", "email", NormalizeEmail("anna@example.com"))) {
		t.Fatal("blind index must differ per tenant key")
	}
	if len(a) != 32 {
		t.Fatalf("blind index length = %d, want 32", len(a))
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"  José   Müller ": "jose muller",
		"ANNA-LENA":        "anna-lena",
		"Łukasz  Żółw":     "lukasz zolw",
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDestroyWipesKeyMaterial(t *testing.T) {
	kek := testKEK(t, "k")
	tenant := uuid.New()
	dek, _, _ := GenerateDEK(kek, tenant, 1)
	f := Field{Table: "t", Column: "c", RowID: uuid.New()}
	ct, _ := dek.Seal(tenant, f, []byte("x"))
	dek.Destroy()
	if _, err := dek.Open(tenant, f, ct); !errors.Is(err, ErrKeyDestroyed) {
		t.Fatalf("destroyed DEK must refuse to open, got %v", err)
	}
	if _, err := dek.Seal(tenant, f, []byte("x")); !errors.Is(err, ErrKeyDestroyed) {
		t.Fatalf("destroyed DEK must refuse to seal, got %v", err)
	}
}
