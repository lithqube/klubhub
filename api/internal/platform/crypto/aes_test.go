package crypto_test

import (
	"crypto/rand"
	"testing"

	"github.com/klubhub/dj/api/internal/platform/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := "hello world test string"
	ciphertext, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Errorf("round-trip failed: got %q, want %q", got, plaintext)
	}
}

func TestDecryptWrongKeyReturnsError(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := "secret data"
	ciphertext, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	wrongKey := make([]byte, 32)
	if _, err := rand.Read(wrongKey); err != nil {
		t.Fatalf("failed to generate wrong key: %v", err)
	}

	_, err = crypto.Decrypt(wrongKey, ciphertext)
	if err == nil {
		t.Error("expected error when decrypting with wrong key, got nil")
	}
}
