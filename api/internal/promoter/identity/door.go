package identity

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
)

const (
	maxPINAttempts = 5
	pinLockout     = 15 * time.Minute
	maxPINWindow   = 36 * time.Hour
)

// DoorDevice is a registered door phone/tablet. Token is shown once and
// stored on the device; only its hash is kept.
type DoorDevice struct {
	ID    uuid.UUID `json:"id"`
	Label string    `json:"label"`
	Token string    `json:"token,omitempty"`
}

// RegisterDoorDevice registers a device for the caller's organisation.
func (s *Service) RegisterDoorDevice(ctx context.Context, by authz.Principal, label string) (DoorDevice, error) {
	tenant, err := uuid.Parse(by.OrgID)
	label = strings.TrimSpace(label)
	if err != nil || label == "" || len(label) > 80 {
		return DoorDevice{}, ErrInvalidInput
	}
	tok, err := auth.NewSessionToken(tenant)
	if err != nil {
		return DoorDevice{}, err
	}
	d := DoorDevice{ID: uuid.Must(uuid.NewV7()), Label: label, Token: tok.Cookie}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO door_devices (id, tenant_id, label, token_hash, created_by) VALUES ($1, $2, $3, $4, $5)`,
			d.ID, tenant, label, tok.Hash, userIDFromSub(by.Sub))
		return err
	})
	return d, err
}

// RevokeDoorDevice revokes a device; its open door sessions stop working.
func (s *Service) RevokeDoorDevice(ctx context.Context, by authz.Principal, deviceID uuid.UUID) error {
	tenant, err := uuid.Parse(by.OrgID)
	if err != nil {
		return ErrInvalidInput
	}
	return s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE door_devices SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`, deviceID, s.now())
		return err
	})
}

// SetDoorPIN generates a random 6-digit PIN for event, valid until
// validUntil (at most 36h ahead), replacing any previous PIN. Staff never
// choose PINs, so they cannot be weak or reused.
func (s *Service) SetDoorPIN(ctx context.Context, by authz.Principal, event uuid.UUID, validUntil time.Time) (string, error) {
	tenant, err := uuid.Parse(by.OrgID)
	now := s.now()
	if err != nil || event == uuid.Nil || !validUntil.After(now) || validUntil.Sub(now) > maxPINWindow {
		return "", ErrInvalidInput
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	pin := fmt.Sprintf("%06d", n.Int64())
	hash, err := auth.HashPassword(pin, s.argon)
	if err != nil {
		return "", err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		sealed, err := s.seal(ctx, tx, tenant, "door_pins", "pin_hash_enc", event, []byte(hash))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO door_pins (tenant_id, event_id, pin_hash_enc, expires_at, created_by)
		  VALUES ($1, $2, $3, $4, $5)
		  ON CONFLICT (tenant_id, event_id) DO UPDATE
		  SET pin_hash_enc = EXCLUDED.pin_hash_enc, expires_at = EXCLUDED.expires_at,
		      failed_attempts = 0, locked_until = NULL, created_by = EXCLUDED.created_by, created_at = now()`,
			tenant, event, sealed, validUntil, userIDFromSub(by.Sub))
		return err
	})
	return pin, err
}

// DoorLogin exchanges a registered device token plus the event PIN for a
// door session that ends when the PIN window ends.
func (s *Service) DoorLogin(ctx context.Context, deviceToken string, event uuid.UUID, pin string) (LoginResult, error) {
	tenant, deviceHash, err := auth.ParseSessionToken(deviceToken)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	now := s.now()
	var res LoginResult
	var outcome error
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var deviceID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM door_devices WHERE token_hash = $1 AND revoked_at IS NULL`, deviceHash).Scan(&deviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = ErrInvalidCredentials
			return nil
		}
		if err != nil {
			return err
		}
		var hashEnc []byte
		var failed int
		var locked *time.Time
		var expires time.Time
		err = tx.QueryRow(ctx, `SELECT pin_hash_enc, failed_attempts, locked_until, expires_at
		  FROM door_pins WHERE event_id = $1 FOR UPDATE`, event).Scan(&hashEnc, &failed, &locked, &expires)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = ErrInvalidCredentials
			return nil
		}
		if err != nil {
			return err
		}
		if !now.Before(expires) || (locked != nil && now.Before(*locked)) {
			outcome = ErrInvalidCredentials
			return nil
		}
		hash, err := s.open(ctx, tx, tenant, "door_pins", "pin_hash_enc", event, hashEnc)
		if err != nil {
			return err
		}
		if ok, _ := auth.VerifyPassword(strings.TrimSpace(pin), string(hash)); !ok {
			outcome = ErrInvalidCredentials
			failed++
			var lockUntil *time.Time
			if failed >= maxPINAttempts {
				t := now.Add(pinLockout)
				lockUntil, failed = &t, 0
			}
			_, err := tx.Exec(ctx, `UPDATE door_pins SET failed_attempts = $2, locked_until = $3 WHERE event_id = $1`, event, failed, lockUntil)
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE door_pins SET failed_attempts = 0, locked_until = NULL WHERE event_id = $1`, event); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE door_devices SET last_seen_at = $2 WHERE id = $1`, deviceID, now); err != nil {
			return err
		}
		tok, err := auth.NewSessionToken(tenant)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sessions (id, tenant_id, token_hash, device_id, event_scope, amr, auth_time, last_seen_at, expires_at)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8)`,
			uuid.Must(uuid.NewV7()), tenant, tok.Hash, deviceID, event, []string{"pin", "dev"}, now, expires); err != nil {
			return err
		}
		res = LoginResult{Cookie: tok.Cookie, Expires: expires}
		return nil
	})
	if err != nil {
		return LoginResult{}, err
	}
	if outcome != nil {
		return LoginResult{}, outcome
	}
	return res, nil
}
