// Package envelope implements the server-side tier of the Promoter data
// protection model (plan §13.4, decision D5):
//
//	KEK (deployment key, never stored in the database)
//	  └── wraps one DEK per tenant and key version (tenant_keys table)
//	        ├── seals `personal` / `financial` fields (AES-256-GCM, AAD binds
//	        │   tenant ‖ table ‖ column ‖ row, so ciphertext cannot be moved)
//	        └── derives blind-index keys (HKDF) for exact-match lookups
//
// Destroying a tenant's DEK rows crypto-shreds all of its sealed data,
// including copies in backups. The `sealed` tier (end-to-end, browser-held
// keys) is not handled here: the server never sees those keys.
package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	keyLen = 32
	// formatV1 is the first byte of every sealed field: version ‖ key version
	// (uint32 BE) ‖ nonce (12) ‖ ciphertext+tag.
	formatV1  byte = 0x01
	headerLen      = 1 + 4
)

var (
	ErrKeyLength    = errors.New("envelope: key must be 32 bytes")
	ErrDecrypt      = errors.New("envelope: decryption failed")
	ErrKeyDestroyed = errors.New("envelope: key material destroyed")
)

// KEK is the deployment key-encryption key. It only wraps and unwraps DEKs.
type KEK struct {
	id   string
	aead cipher.AEAD
}

// NewKEK builds a KEK from 32 raw bytes. The id is recorded next to every
// wrapped DEK so KEKs can be rotated.
func NewKEK(id string, raw []byte) (*KEK, error) {
	if len(raw) != keyLen {
		return nil, ErrKeyLength
	}
	if id == "" || strings.ContainsAny(id, "|\x00") {
		return nil, errors.New("envelope: KEK id must be non-empty and contain no separators")
	}
	aead, err := newAEAD(raw)
	if err != nil {
		return nil, err
	}
	return &KEK{id: id, aead: aead}, nil
}

// ID identifies the KEK version.
func (k *KEK) ID() string { return k.id }

func (k *KEK) dekAAD(tenant uuid.UUID, version uint32) []byte {
	return fmt.Appendf(nil, "klubhub/dek/v1|%s|%d|%s", tenant, version, k.id)
}

// GenerateDEK creates a fresh DEK for tenant/version and returns it together
// with its wrapped form for storage.
func GenerateDEK(k *KEK, tenant uuid.UUID, version uint32) (*DEK, []byte, error) {
	raw := make([]byte, keyLen)
	if _, err := rand.Read(raw); err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	wrapped := k.aead.Seal(nonce, nonce, raw, k.dekAAD(tenant, version))
	dek, err := newDEK(raw, version)
	clear(raw)
	return dek, wrapped, err
}

// UnwrapDEK recovers a DEK wrapped by GenerateDEK for the same tenant/version.
func (k *KEK) UnwrapDEK(tenant uuid.UUID, version uint32, wrapped []byte) (*DEK, error) {
	ns := k.aead.NonceSize()
	if len(wrapped) < ns+k.aead.Overhead() {
		return nil, ErrDecrypt
	}
	raw, err := k.aead.Open(nil, wrapped[:ns], wrapped[ns:], k.dekAAD(tenant, version))
	if err != nil {
		return nil, ErrDecrypt
	}
	defer clear(raw)
	return newDEK(raw, version)
}

// DEK is a tenant data-encryption key (one version).
type DEK struct {
	mu        sync.RWMutex
	version   uint32
	aead      cipher.AEAD
	indexRoot []byte
	destroyed bool
}

func newDEK(raw []byte, version uint32) (*DEK, error) {
	aead, err := newAEAD(raw)
	if err != nil {
		return nil, err
	}
	// Blind-index keys are derived, never the encryption key itself.
	root, err := hkdf.Key(sha256.New, raw, nil, "klubhub/blind-index/v1", keyLen)
	if err != nil {
		return nil, err
	}
	return &DEK{version: version, aead: aead, indexRoot: root}, nil
}

// Version is the tenant key version this DEK belongs to.
func (d *DEK) Version() uint32 { return d.version }

// Field names the storage location a ciphertext is bound to.
type Field struct {
	Table  string
	Column string
	RowID  uuid.UUID
}

func fieldAAD(tenant uuid.UUID, f Field, version uint32) []byte {
	return fmt.Appendf(nil, "klubhub/field/v1|%s|%s|%s|%s|%d", tenant, f.Table, f.Column, f.RowID, version)
}

// Seal encrypts plaintext for tenant at the given field.
func (d *DEK) Seal(tenant uuid.UUID, f Field, plaintext []byte) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.destroyed {
		return nil, ErrKeyDestroyed
	}
	out := make([]byte, headerLen, headerLen+d.aead.NonceSize()+len(plaintext)+d.aead.Overhead())
	out[0] = formatV1
	binary.BigEndian.PutUint32(out[1:headerLen], d.version)
	nonce := make([]byte, d.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out = append(out, nonce...)
	return d.aead.Seal(out, nonce, plaintext, fieldAAD(tenant, f, d.version)), nil
}

// Open decrypts a value produced by Seal for the same tenant and field.
func (d *DEK) Open(tenant uuid.UUID, f Field, sealed []byte) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.destroyed {
		return nil, ErrKeyDestroyed
	}
	ns := d.aead.NonceSize()
	if len(sealed) < headerLen+ns+d.aead.Overhead() || sealed[0] != formatV1 ||
		binary.BigEndian.Uint32(sealed[1:headerLen]) != d.version {
		return nil, ErrDecrypt
	}
	body := sealed[headerLen:]
	pt, err := d.aead.Open(nil, body[:ns], body[ns:], fieldAAD(tenant, f, d.version))
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// BlindIndex returns a keyed, deterministic 32-byte digest of an already
// normalised value, scoped to table and column. It supports equality lookups
// on sealed fields without storing plaintext; it does not hide equality.
func (d *DEK) BlindIndex(table, column, normalised string) []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()
	mac := hmac.New(sha256.New, d.indexRoot)
	fmt.Fprintf(mac, "%s|%s|", table, column)
	mac.Write([]byte(normalised))
	return mac.Sum(nil)
}

// Destroy wipes what can be wiped and makes the DEK unusable.
func (d *DEK) Destroy() {
	d.mu.Lock()
	defer d.mu.Unlock()
	clear(d.indexRoot)
	d.aead = nil
	d.destroyed = true
}

// KeyVersionOf reads the tenant key version from a sealed value's header.
func KeyVersionOf(sealed []byte) (uint32, error) {
	if len(sealed) < headerLen || sealed[0] != formatV1 {
		return 0, ErrDecrypt
	}
	return binary.BigEndian.Uint32(sealed[1:headerLen]), nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// NormalizeEmail lowercases and trims an email for blind indexing.
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

var foldLetters = strings.NewReplacer("ł", "l", "đ", "d", "ø", "o", "æ", "ae", "œ", "oe", "ß", "ss", "þ", "th", "ı", "i")

// NormalizeName folds case, diacritics and whitespace so "José  Müller" and
// "jose muller" share a blind index (door search, dedupe).
func NormalizeName(s string) string {
	t := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(t, strings.ToLower(s))
	if err != nil {
		folded = strings.ToLower(s)
	}
	return strings.Join(strings.Fields(foldLetters.Replace(folded)), " ")
}
