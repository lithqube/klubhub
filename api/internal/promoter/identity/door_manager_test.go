package identity_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/klubhub/dj/api/internal/promoter/identity"
)

// The offline verifier must match WebCrypto's PBKDF2-SHA256 on the device,
// so it is pinned to the RFC 7914 §11 test vector.
func TestManagerPINVerifierMatchesPBKDF2SHA256(t *testing.T) {
	cases := []struct {
		pin, salt string
		iter      int
		want      string
	}{
		{"passwd", "salt", 1, "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc"},
	}
	for _, c := range cases {
		got, err := identity.ManagerPINVerifier(c.pin, []byte(c.salt), c.iter)
		if err != nil || hex.EncodeToString(got) != c.want {
			t.Fatalf("PBKDF2(%q, %q, %d) = %x %v, want %s", c.pin, c.salt, c.iter, got, err, c.want)
		}
	}
}

func TestNewPINCheck(t *testing.T) {
	c, err := identity.NewPINCheck("042917")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Salt) != 16 || c.Iterations != 210_000 || len(c.Hash) != 32 {
		t.Fatalf("verifier params: salt %d, iterations %d, hash %d", len(c.Salt), c.Iterations, len(c.Hash))
	}
	again, _ := identity.ManagerPINVerifier("042917", c.Salt, c.Iterations)
	wrong, _ := identity.ManagerPINVerifier("042918", c.Salt, c.Iterations)
	if !bytes.Equal(again, c.Hash) || bytes.Equal(wrong, c.Hash) {
		t.Fatal("the verifier must accept the PIN and only the PIN")
	}
	other, _ := identity.NewPINCheck("042917")
	if bytes.Equal(other.Salt, c.Salt) {
		t.Fatal("each verifier needs a fresh salt")
	}
	raw, _ := json.Marshal(c)
	var shape map[string]any
	_ = json.Unmarshal(raw, &shape)
	salt, _ := base64.StdEncoding.DecodeString(shape["salt"].(string))
	if len(shape) != 3 || !bytes.Equal(salt, c.Salt) || shape["iterations"].(float64) != 210_000 {
		t.Fatalf("JSON shape: %s", raw)
	}
}
