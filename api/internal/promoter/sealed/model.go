// Package sealed is the server side of the sealed tier (ADR 0001, P2.6
// contract): member keys, the organisation's sealed key (OSK) wraps, the
// recovery kit's public half and the ban list.
//
// Everything sealed is encrypted in the browser. The server stores X25519
// public keys and opaque ciphertext and has no decryption path: it checks
// structure only (base64url, lengths, the version byte, KDF bounds), who may
// receive a wrap, and the key-version bookkeeping. Ciphertext never reaches
// logs, audit entries or outbox events.
package sealed

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Structural limits. Blobs are `0x01 ‖ …` as produced by the browser:
//
//   - wrap (seal to a public key of a 32-byte OSK):
//     0x01 ‖ eph_pub(32) ‖ nonce(12) ‖ ct(32) ‖ tag(16) = 93 bytes; up to
//     256 bytes are accepted so the format can carry a little metadata.
//   - private_sealed (AES-GCM of the X25519 private key under the
//     passphrase key): 0x01 ‖ nonce(12) ‖ ct(≥32) ‖ tag(16) ≥ 61 bytes; up
//     to 512 (a PKCS#8 or JWK encoding fits).
//   - entry_sealed (AES-GCM of the ban entry JSON under the OSK):
//     0x01 ‖ nonce(12) ‖ ct(≥1) ‖ tag(16) ≥ 30 bytes; up to 4096, which
//     leaves room for name, email, reason and a short note.
//
// The same bounds are CHECK constraints in migration 00012.
const (
	BlobVersion = 0x01

	PublicKeyLen = 32

	MinWrap = 1 + 32 + 12 + 32 + 16
	MaxWrap = 256

	MinPrivateSealed = 1 + 12 + 32 + 16
	MaxPrivateSealed = 512

	MinEntrySealed = 1 + 12 + 1 + 16
	MaxEntrySealed = 4096

	// Argon2id bounds for the member key's passphrase KDF (m in KiB).
	MinKDFMemoryKiB = 19 * 1024
	MaxKDFMemoryKiB = 256 * 1024
	MinKDFTime      = 1
	MaxKDFTime      = 10
	MinKDFThreads   = 1
	MaxKDFThreads   = 4
	KDFSaltLen      = 16

	// MaxWraps caps wraps per request (setup, add, rotate).
	MaxWraps = 500
	// MaxBanEntries caps the ban list per organisation; a rotation carries
	// every entry, so this also bounds the rotate body.
	MaxBanEntries = 2000

	// Ban entry expiry window, with a little grace for the client's clock
	// and request latency (a "1 day" preset computed in the browser is a
	// few seconds short of 24 h when it arrives).
	MinBanLifetime = 24 * time.Hour
	MaxBanYears    = 3
	ExpiryGrace    = 5 * time.Minute
)

// Recipient kinds of a wrap.
const (
	KindMember   = "member"
	KindDevice   = "device"
	KindRecovery = "recovery"
)

// Rotation reasons.
const (
	ReasonMemberRemoved = "member_removed"
	ReasonDeviceRevoked = "device_revoked"
)

// Org key states reported by GET /keys/org.
const (
	StatusNotSetup        = "not_setup"
	StatusReady           = "ready"
	StatusRotationPending = "rotation_pending"
)

var (
	// ErrNotFound: unknown ban entry or device.
	ErrNotFound = errors.New("sealed: not found")
	// ErrNoMemberKey: the caller has not created a member key.
	ErrNoMemberKey = errors.New("sealed: no member key")
	// ErrLocalIdentity: member keys are bound to local user ids.
	ErrLocalIdentity = errors.New("sealed: local identity required")
)

// InvalidError names the request field that failed structural validation
// (422 {"error":"invalid","field","problem"}).
type InvalidError struct {
	Field   string
	Problem string
}

func (e *InvalidError) Error() string { return "sealed: invalid " + e.Field + ": " + e.Problem }

func invalid(field, problem string) error { return &InvalidError{Field: field, Problem: problem} }

// ConflictError is a 409 with a machine-readable code (already_setup,
// not_setup, version_conflict, key_version_stale, public_key_mismatch,
// no_member_key, ban_entry_exists, ban_list_full).
type ConflictError struct{ Code string }

func (e *ConflictError) Error() string { return "sealed: conflict: " + e.Code }

func conflict(code string) error { return &ConflictError{Code: code} }

// DeniedError is a policy refusal found inside a handler (403).
type DeniedError struct{ Reason string }

func (e *DeniedError) Error() string { return "sealed: denied: " + e.Reason }

// Encode is the wire form of bytes: base64url without padding.
func Encode(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func encodePtr(b []byte) *string {
	if b == nil {
		return nil
	}
	s := Encode(b)
	return &s
}

// Decode accepts base64url with or without padding; standard base64
// ('+', '/') and whitespace are refused.
func Decode(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	if s == "" {
		return nil, errors.New("empty")
	}
	return base64.RawURLEncoding.Strict().DecodeString(s)
}

// DecodePublicKey decodes an X25519 public key (exactly 32 bytes; the
// all-zero key is refused).
func DecodePublicKey(field, s string) ([]byte, error) {
	b, err := Decode(s)
	if err != nil {
		return nil, invalid(field, "base64url")
	}
	if len(b) != PublicKeyLen {
		return nil, invalid(field, fmt.Sprintf("must be %d bytes", PublicKeyLen))
	}
	zero := true
	for _, c := range b {
		zero = zero && c == 0
	}
	if zero {
		return nil, invalid(field, "all-zero key")
	}
	return b, nil
}

// decodeBlob decodes a sealed blob: version byte 0x01, length in [lo, hi].
func decodeBlob(field, s string, lo, hi int) ([]byte, error) {
	b, err := Decode(s)
	if err != nil {
		return nil, invalid(field, "base64url")
	}
	if len(b) < lo || len(b) > hi {
		return nil, invalid(field, fmt.Sprintf("must be %d to %d bytes", lo, hi))
	}
	if b[0] != BlobVersion {
		return nil, invalid(field, "unknown version")
	}
	return b, nil
}

// DecodeWrap decodes a wrap of the OSK.
func DecodeWrap(field, s string) ([]byte, error) { return decodeBlob(field, s, MinWrap, MaxWrap) }

// DecodePrivateSealed decodes a member's sealed private key.
func DecodePrivateSealed(field, s string) ([]byte, error) {
	return decodeBlob(field, s, MinPrivateSealed, MaxPrivateSealed)
}

// DecodeEntrySealed decodes a sealed ban entry.
func DecodeEntrySealed(field, s string) ([]byte, error) {
	return decodeBlob(field, s, MinEntrySealed, MaxEntrySealed)
}

// Fingerprint is the recovery key fingerprint: the first 8 bytes of
// SHA-256(public key), lowercase hex.
func Fingerprint(pub []byte) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// KDF are the public Argon2id parameters of a member key.
type KDF struct {
	Alg  string `json:"alg"`
	M    int    `json:"m"`
	T    int    `json:"t"`
	P    int    `json:"p"`
	Salt string `json:"salt"`
}

// Validate checks the parameters are Argon2id within bounds and returns
// the canonical form (salt re-encoded without padding).
func (k KDF) Validate(field string) (KDF, error) {
	switch {
	case k.Alg != "argon2id":
		return k, invalid(field+".alg", "must be argon2id")
	case k.M < MinKDFMemoryKiB || k.M > MaxKDFMemoryKiB:
		return k, invalid(field+".m", fmt.Sprintf("memory must be %d to %d KiB", MinKDFMemoryKiB, MaxKDFMemoryKiB))
	case k.T < MinKDFTime || k.T > MaxKDFTime:
		return k, invalid(field+".t", fmt.Sprintf("iterations must be %d to %d", MinKDFTime, MaxKDFTime))
	case k.P < MinKDFThreads || k.P > MaxKDFThreads:
		return k, invalid(field+".p", fmt.Sprintf("parallelism must be %d to %d", MinKDFThreads, MaxKDFThreads))
	}
	salt, err := Decode(k.Salt)
	if err != nil || len(salt) != KDFSaltLen {
		return k, invalid(field+".salt", fmt.Sprintf("must be %d bytes, base64url", KDFSaltLen))
	}
	k.Salt = Encode(salt)
	return k, nil
}

// ---------------------------------------------------------------- wire ---

// MemberKeyInput is PUT /keys/me.
type MemberKeyInput struct {
	PublicKey     string `json:"public_key"`
	PrivateSealed string `json:"private_sealed"`
	KDF           KDF    `json:"kdf"`
}

// MemberKey is the caller's key as GET /keys/me returns it.
type MemberKey struct {
	PublicKey     string `json:"public_key"`
	PrivateSealed string `json:"private_sealed"`
	KDF           KDF    `json:"kdf"`
}

type memberKey struct {
	pub, private []byte
	kdf          KDF
}

func (in MemberKeyInput) parse() (memberKey, error) {
	var k memberKey
	var err error
	if k.pub, err = DecodePublicKey("public_key", in.PublicKey); err != nil {
		return k, err
	}
	if k.private, err = DecodePrivateSealed("private_sealed", in.PrivateSealed); err != nil {
		return k, err
	}
	k.kdf, err = in.KDF.Validate("kdf")
	return k, err
}

// OrgStatus is GET /keys/org.
type OrgStatus struct {
	Status              string  `json:"status"`
	Version             *int    `json:"version"`
	MyWrap              *string `json:"my_wrap"`
	RecoveryFingerprint *string `json:"recovery_fingerprint"`
}

// WrapInput is one wrap in setup, add and rotate. The canonical shape is
// {"recipient_kind","recipient_id","wrap"}; "kind" and "wrap_sealed" are
// accepted as aliases.
type WrapInput struct {
	RecipientKind string     `json:"recipient_kind"`
	RecipientID   *uuid.UUID `json:"recipient_id"`
	Wrap          string     `json:"wrap"`
}

// UnmarshalJSON accepts the aliases and refuses unknown fields.
func (w *WrapInput) UnmarshalJSON(b []byte) error {
	var raw struct {
		RecipientKind string     `json:"recipient_kind"`
		Kind          string     `json:"kind"`
		RecipientID   *uuid.UUID `json:"recipient_id"`
		Wrap          string     `json:"wrap"`
		WrapSealed    string     `json:"wrap_sealed"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	w.RecipientKind, w.RecipientID, w.Wrap = raw.RecipientKind, raw.RecipientID, raw.Wrap
	if w.RecipientKind == "" {
		w.RecipientKind = raw.Kind
	}
	if w.Wrap == "" {
		w.Wrap = raw.WrapSealed
	}
	return nil
}

type wrap struct {
	kind string
	id   uuid.UUID
	blob []byte
}

func (w wrap) key() string { return w.kind + ":" + w.id.String() }

// parseWraps checks the structure of member/device wraps (no recovery
// kind, a recipient id, a valid blob, no recipient twice).
func parseWraps(field string, in []WrapInput) ([]wrap, error) {
	if len(in) > MaxWraps {
		return nil, invalid(field, fmt.Sprintf("at most %d wraps", MaxWraps))
	}
	out := make([]wrap, 0, len(in))
	seen := map[string]bool{}
	for i, w := range in {
		f := fmt.Sprintf("%s[%d]", field, i)
		if w.RecipientKind != KindMember && w.RecipientKind != KindDevice {
			return nil, invalid(f+".recipient_kind", "member or device")
		}
		if w.RecipientID == nil || *w.RecipientID == uuid.Nil {
			return nil, invalid(f+".recipient_id", "required")
		}
		blob, err := DecodeWrap(f+".wrap", w.Wrap)
		if err != nil {
			return nil, err
		}
		p := wrap{kind: w.RecipientKind, id: *w.RecipientID, blob: blob}
		if seen[p.key()] {
			return nil, invalid(f+".recipient_id", "duplicate recipient")
		}
		seen[p.key()] = true
		out = append(out, p)
	}
	return out, nil
}

// RecoveryInput is the recovery kit's public half and its wrap.
type RecoveryInput struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	Wrap        string `json:"wrap"`
}

type recovery struct {
	pub         []byte
	fingerprint string
	wrap        []byte
}

func (in RecoveryInput) parse(field string) (recovery, error) {
	var r recovery
	var err error
	if r.pub, err = DecodePublicKey(field+".public_key", in.PublicKey); err != nil {
		return r, err
	}
	r.fingerprint = strings.ToLower(strings.TrimSpace(in.Fingerprint))
	if r.fingerprint != Fingerprint(r.pub) {
		return r, invalid(field+".fingerprint", "must be the first 8 bytes of SHA-256(public_key), hex")
	}
	r.wrap, err = DecodeWrap(field+".wrap", in.Wrap)
	return r, err
}

// SetupInput is POST /keys/org/setup.
type SetupInput struct {
	Version  int           `json:"version"`
	Recovery RecoveryInput `json:"recovery"`
	Wraps    []WrapInput   `json:"wraps"`
}

// WrapsInput is POST /keys/org/wraps. Version is optional; when present it
// must be the active version (a client holding a retired OSK gets 409
// version_conflict instead of wrapping a stale key).
type WrapsInput struct {
	Version *int        `json:"version"`
	Wraps   []WrapInput `json:"wraps"`
}

// WrapsResult answers POST /keys/org/wraps.
type WrapsResult struct {
	Version int `json:"version"`
	Added   int `json:"added"`
}

// DeviceWrapInput is PUT /keys/devices/{deviceID}/wrap.
type DeviceWrapInput struct {
	Wrap    string `json:"wrap"`
	Version *int   `json:"version"`
}

// DeviceWrapResult answers PUT /keys/devices/{deviceID}/wrap.
type DeviceWrapResult struct {
	DeviceID uuid.UUID `json:"device_id"`
	Version  int       `json:"version"`
}

// RotateEntry is one re-encrypted ban entry.
type RotateEntry struct {
	ID          uuid.UUID `json:"id"`
	EntrySealed string    `json:"entry_sealed"`
}

// RotateInput is POST /keys/org/rotate.
type RotateInput struct {
	FromVersion int           `json:"from_version"`
	ToVersion   int           `json:"to_version"`
	Recovery    RecoveryInput `json:"recovery"`
	Wraps       []WrapInput   `json:"wraps"`
	BanEntries  []RotateEntry `json:"ban_entries"`
}

// MemberRecipient is a member in GET /keys/org/recipients.
type MemberRecipient struct {
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	PublicKey *string   `json:"public_key"`
	HasWrap   bool      `json:"has_wrap"`
}

// DeviceRecipient is a door device in GET /keys/org/recipients.
type DeviceRecipient struct {
	DeviceID  uuid.UUID `json:"device_id"`
	Label     string    `json:"label"`
	PublicKey *string   `json:"public_key"`
	HasWrap   bool      `json:"has_wrap"`
	Revoked   bool      `json:"revoked"`
}

// Recipients is GET /keys/org/recipients (wraps of the active version).
type Recipients struct {
	Version *int              `json:"version"`
	Members []MemberRecipient `json:"members"`
	Devices []DeviceRecipient `json:"devices"`
}

// Recovery is GET /keys/org/recovery.
type Recovery struct {
	Version     int    `json:"version"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	Wrap        string `json:"wrap"`
}

// BanEntry is one ban-list entry (ciphertext only).
type BanEntry struct {
	ID          uuid.UUID `json:"id"`
	KeyVersion  int       `json:"key_version"`
	EntrySealed string    `json:"entry_sealed"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BanList is GET /ban-list: the active key version and the unexpired
// entries.
type BanList struct {
	KeyVersion *int       `json:"key_version"`
	Entries    []BanEntry `json:"entries"`
}

// BanInput is POST /ban-list (ID required) and PUT /ban-list/{id} (ID
// optional; must match the URL when given).
type BanInput struct {
	ID          *uuid.UUID `json:"id"`
	KeyVersion  int        `json:"key_version"`
	EntrySealed string     `json:"entry_sealed"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

type banEntry struct {
	keyVersion int
	blob       []byte
	expiresAt  time.Time
}

// ExpiryWindow returns the accepted [lo, hi] for a new expiry at now.
func ExpiryWindow(now time.Time) (time.Time, time.Time) {
	return now.Add(MinBanLifetime - ExpiryGrace), now.AddDate(MaxBanYears, 0, 0).Add(ExpiryGrace)
}

func (in BanInput) parse(now time.Time) (banEntry, error) {
	var e banEntry
	if in.KeyVersion < 1 {
		return e, invalid("key_version", "required")
	}
	blob, err := DecodeEntrySealed("entry_sealed", in.EntrySealed)
	if err != nil {
		return e, err
	}
	lo, hi := ExpiryWindow(now)
	if in.ExpiresAt.IsZero() || in.ExpiresAt.Before(lo) || in.ExpiresAt.After(hi) {
		return e, invalid("expires_at", "between 1 day and 3 years from now")
	}
	return banEntry{keyVersion: in.KeyVersion, blob: blob, expiresAt: in.ExpiresAt.UTC()}, nil
}

// DoorSealed is the `sealed` block of the door bundle: this device's wrap
// of the active OSK and the unexpired ban entries.
type DoorSealed struct {
	KeyVersion int            `json:"key_version"`
	Wrap       string         `json:"wrap"`
	BanEntries []DoorBanEntry `json:"ban_entries"`
}

// DoorBanEntry is a ban entry as the door gets it.
type DoorBanEntry struct {
	ID          uuid.UUID `json:"id"`
	EntrySealed string    `json:"entry_sealed"`
	ExpiresAt   time.Time `json:"expires_at"`
}
