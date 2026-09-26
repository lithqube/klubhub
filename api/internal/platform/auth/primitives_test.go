package auth

import (
	"encoding/base32"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var fastParams = Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

func TestPasswordHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple", fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("want a PHC argon2id string, got %q", h)
	}
	ok, err := VerifyPassword("correct horse battery staple", h)
	if err != nil || !ok {
		t.Fatalf("verify failed: %v", err)
	}
	if ok, _ := VerifyPassword("wrong", h); ok {
		t.Fatal("wrong password verified")
	}
	h2, _ := HashPassword("correct horse battery staple", fastParams)
	if h == h2 {
		t.Fatal("hashes must be salted")
	}
	if _, err := VerifyPassword("x", "$argon2i$v=19$m=1,t=1,p=1$AA$AA"); err == nil {
		t.Fatal("non-argon2id hashes must be rejected")
	}
}

func TestPasswordPolicy(t *testing.T) {
	if err := CheckPasswordPolicy("short"); err == nil {
		t.Fatal("short passwords must be rejected")
	}
	if err := CheckPasswordPolicy(strings.Repeat("a", 12)); err == nil {
		t.Fatal("a single repeated character must be rejected")
	}
	if err := CheckPasswordPolicy("four random words here"); err != nil {
		t.Fatalf("a long passphrase must pass: %v", err)
	}
}

// RFC 6238 appendix B, SHA-1, 8 digits, secret "12345678901234567890".
func TestTOTPRFCVectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	vectors := map[int64]string{59: "94287082", 1111111109: "07081804", 1234567890: "89005924", 2000000000: "69279037"}
	for ts, want := range vectors {
		if got := totpCode(secret, time.Unix(ts, 0), 8); got != want {
			t.Errorf("t=%d: got %s want %s", ts, got, want)
		}
	}
}

func TestTOTPVerifyWindowAndReplay(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	code := totpCode(secret, now, 6)
	step, ok := VerifyTOTP(secret, code, now, 0)
	if !ok {
		t.Fatal("current code must verify")
	}
	if _, ok := VerifyTOTP(secret, code, now, step); ok {
		t.Fatal("a code must not be accepted twice (replay)")
	}
	prev := totpCode(secret, now.Add(-30*time.Second), 6)
	if _, ok := VerifyTOTP(secret, prev, now, 0); !ok {
		t.Fatal("one step of clock skew must be tolerated")
	}
	old := totpCode(secret, now.Add(-90*time.Second), 6)
	if _, ok := VerifyTOTP(secret, old, now, 0); ok {
		t.Fatal("codes older than one step must be rejected")
	}
	uri := TOTPProvisioningURI(secret, "KlubHub Promoter", "anna@example.com")
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, "secret="+base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)) {
		t.Fatalf("bad provisioning uri %q", uri)
	}
}

func TestSessionTokenRoundTrip(t *testing.T) {
	tenant := uuid.New()
	tok, err := NewSessionToken(tenant)
	if err != nil {
		t.Fatal(err)
	}
	gotTenant, hash, err := ParseSessionToken(tok.Cookie)
	if err != nil || gotTenant != tenant || string(hash) != string(tok.Hash) {
		t.Fatalf("parse mismatch: %v", err)
	}
	if len(tok.Hash) != 32 {
		t.Fatalf("hash length %d", len(tok.Hash))
	}
	for _, bad := range []string{"", "v1." + tenant.String(), "v2." + tenant.String() + ".abc", "v1.not-a-uuid.abc", "v1." + tenant.String() + ".%%%"} {
		if _, _, err := ParseSessionToken(bad); err == nil {
			t.Errorf("malformed cookie %q accepted", bad)
		}
	}
}

func TestCSRFGuard(t *testing.T) {
	h := CSRF([]string{"https://promoter.example"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	do := func(method, origin, header string) int {
		r := httptest.NewRequest(method, "/api/v1/events", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if header != "" {
			r.Header.Set(CSRFHeader, header)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := do(http.MethodGet, "", ""); c != http.StatusNoContent {
		t.Fatalf("safe methods pass, got %d", c)
	}
	if c := do(http.MethodPost, "https://promoter.example", "1"); c != http.StatusNoContent {
		t.Fatalf("same-origin POST with header passes, got %d", c)
	}
	if c := do(http.MethodPost, "https://evil.example", "1"); c != http.StatusForbidden {
		t.Fatalf("foreign origin must be refused, got %d", c)
	}
	if c := do(http.MethodPost, "https://promoter.example", ""); c != http.StatusForbidden {
		t.Fatalf("missing CSRF header must be refused, got %d", c)
	}
	if c := do(http.MethodDelete, "", "1"); c != http.StatusForbidden {
		t.Fatalf("unsafe request without Origin must be refused, got %d", c)
	}
}
