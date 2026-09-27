package identity

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/promoter/sealed"
)

// Offline manager-PIN verifier (P2.3 contract): PBKDF2-SHA256 with a random
// 16-byte salt and 210 000 iterations, 32-byte output. Browsers compute the
// same with WebCrypto's deriveBits, so the door can check the PIN without a
// network. A 6-digit PIN is guessable offline from this verifier by anyone
// holding an unlocked door device; that is accepted because the device
// already holds the whole list, and the server re-verifies every add against
// the Argon2id hash with lockout.
const (
	PINCheckIterations = 210_000
	pinCheckSaltLen    = 16
	pinCheckKeyLen     = 32
)

// PINCheck is the offline verifier of a manager PIN. JSON encodes the byte
// slices as standard base64.
type PINCheck struct {
	Salt       []byte `json:"salt"`
	Iterations int    `json:"iterations"`
	Hash       []byte `json:"hash"`
}

// ManagerPINVerifier derives the offline verifier hash of pin.
func ManagerPINVerifier(pin string, salt []byte, iterations int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, pin, salt, iterations, pinCheckKeyLen)
}

// NewPINCheck builds a verifier for pin with a fresh salt.
func NewPINCheck(pin string) (PINCheck, error) {
	salt := make([]byte, pinCheckSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return PINCheck{}, err
	}
	hash, err := ManagerPINVerifier(pin, salt, PINCheckIterations)
	if err != nil {
		return PINCheck{}, err
	}
	return PINCheck{Salt: salt, Iterations: PINCheckIterations, Hash: hash}, nil
}

// VerifyManagerPIN checks pin against the event's manager PIN inside the
// caller's tenant transaction (the door adds batch). Failures count towards
// the manager row's lockout (5 attempts, then 15 minutes), separate from
// the staff PIN's. A missing, expired or locked PIN never verifies.
func (s *Service) VerifyManagerPIN(ctx context.Context, tx pgx.Tx, tenant, event uuid.UUID, pin string) (bool, error) {
	var hashEnc []byte
	var failed int
	var locked *time.Time
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT pin_hash_enc, failed_attempts, locked_until, expires_at
	  FROM door_pins WHERE event_id = $1 AND manager FOR UPDATE`, event).Scan(&hashEnc, &failed, &locked, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := s.now()
	if !now.Before(expires) || (locked != nil && now.Before(*locked)) {
		return false, nil
	}
	hash, err := s.open(ctx, tx, tenant, "door_pins", pinColumn(true), event, hashEnc)
	if err != nil {
		return false, err
	}
	if ok, _ := auth.VerifyPassword(strings.TrimSpace(pin), string(hash)); ok {
		_, err := tx.Exec(ctx, `UPDATE door_pins SET failed_attempts = 0, locked_until = NULL WHERE event_id = $1 AND manager`, event)
		return err == nil, err
	}
	failed++
	var lockUntil *time.Time
	if failed >= maxPINAttempts {
		t := now.Add(pinLockout)
		lockUntil, failed = &t, 0
	}
	_, err = tx.Exec(ctx, `UPDATE door_pins SET failed_attempts = $2, locked_until = $3 WHERE event_id = $1 AND manager`, event, failed, lockUntil)
	return false, err
}

// ManagerPINCheck returns the offline verifier of the event's manager PIN,
// or nil when there is no unexpired manager PIN.
func (s *Service) ManagerPINCheck(ctx context.Context, tx pgx.Tx, tenant, event uuid.UUID) (*PINCheck, error) {
	var checkEnc []byte
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT check_enc, expires_at FROM door_pins WHERE event_id = $1 AND manager`, event).Scan(&checkEnc, &expires)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (checkEnc == nil || !s.now().Before(expires))) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	raw, err := s.open(ctx, tx, tenant, "door_pins", "check_enc", event, checkEnc)
	if err != nil {
		return nil, err
	}
	var c PINCheck
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// PINWindow is when a door PIN stops working, and until when it refuses
// logins after too many wrong tries (null when not locked).
type PINWindow struct {
	ValidUntil  time.Time  `json:"valid_until"`
	LockedUntil *time.Time `json:"locked_until"`
}

// PINStatus says which door PINs of an event are live; never the PINs.
type PINStatus struct {
	Staff   *PINWindow `json:"staff"`
	Manager *PINWindow `json:"manager"`
}

// DoorPINStatus reports the event's unexpired staff and manager PINs and
// any running lockout.
func (s *Service) DoorPINStatus(ctx context.Context, by authz.Principal, event uuid.UUID) (PINStatus, error) {
	tenant, err := uuid.Parse(by.OrgID)
	if err != nil {
		return PINStatus{}, ErrInvalidInput
	}
	var out PINStatus
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		now := s.now()
		rows, err := tx.Query(ctx, `SELECT manager, expires_at, CASE WHEN locked_until > $2 THEN locked_until END
		  FROM door_pins WHERE event_id = $1 AND expires_at > $2`, event, now)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var manager bool
			var w PINWindow
			if err := rows.Scan(&manager, &w.ValidUntil, &w.LockedUntil); err != nil {
				return err
			}
			if manager {
				out.Manager = &w
			} else {
				out.Staff = &w
			}
		}
		return rows.Err()
	})
	return out, err
}

// DoorDeviceInfo is a registered door device as the device list shows it.
type DoorDeviceInfo struct {
	ID         uuid.UUID  `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	// PublicKey (base64url X25519, null when the device sent none) and
	// HasWrap (holds the active org key version) drive the Door tab's
	// sealed status (P2.6; additive to the P2.3 shape).
	PublicKey *string `json:"public_key"`
	HasWrap   bool    `json:"has_wrap"`
}

// DoorDevices lists the organisation's door devices, active first.
func (s *Service) DoorDevices(ctx context.Context, by authz.Principal) ([]DoorDeviceInfo, error) {
	tenant, err := uuid.Parse(by.OrgID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	out := []DoorDeviceInfo{}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT d.id, d.label, d.created_at, d.last_seen_at, d.revoked_at, d.public_key,
		    EXISTS (SELECT 1 FROM org_key_wraps w JOIN org_sealed_keys k ON k.version = w.version AND k.status = 'active'
		            WHERE w.recipient_kind = 'device' AND w.recipient_id = d.id)
		  FROM door_devices d ORDER BY d.revoked_at NULLS FIRST, d.created_at`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (DoorDeviceInfo, error) {
			var d DoorDeviceInfo
			var pub []byte
			err := r.Scan(&d.ID, &d.Label, &d.CreatedAt, &d.LastSeenAt, &d.RevokedAt, &pub, &d.HasWrap)
			if pub != nil {
				k := sealed.Encode(pub)
				d.PublicKey = &k
			}
			return d, err
		})
		return err
	})
	if out == nil {
		out = []DoorDeviceInfo{}
	}
	return out, err
}
