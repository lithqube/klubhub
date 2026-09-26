// Package identity is the self-hosted (local) identity provider for KlubHub
// Promoter (plan D6, §13.1): one organisation per instance, argon2id
// passwords, optional-then-enforced TOTP, server-side sessions and one-time
// setup/invite links. Personal fields (email, name, password hash, TOTP
// secret) are sealed with the tenant DEK; lookups use blind indexes.
//
// SaaS uses Zitadel instead (platform/auth.Zitadel); both produce the same
// authz.Principal.
package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

var (
	ErrAlreadyBootstrapped = errors.New("identity: this instance already has an organisation")
	ErrInvalidLink         = errors.New("identity: invalid or expired link")
	ErrInvalidCredentials  = errors.New("identity: invalid credentials")
	ErrTOTPRequired        = errors.New("identity: TOTP code required")
	ErrWeakPassword        = errors.New("identity: password does not meet the policy")
	ErrInvalidRole         = errors.New("identity: unknown role")
	ErrInvalidInput        = errors.New("identity: invalid input")
)

// Roles a membership can hold (mirrors the policy and the CHECK constraint).
var Roles = []string{"owner", "admin", "booker", "finance", "marketing", "door"}

const (
	maxFailedLogins = 5
	lockoutFor      = 15 * time.Minute
	sessionIdle     = 12 * time.Hour
	sessionAbsolute = 7 * 24 * time.Hour
	bootstrapTTL    = 24 * time.Hour
	inviteTTL       = 72 * time.Hour
	touchEvery      = time.Minute
	totpIssuer      = "KlubHub Promoter"
)

// Options tunes the service (tests use cheap argon2 and a fake clock).
type Options struct {
	Argon2 auth.Argon2Params
	Now    func() time.Time
}

// Service implements the local identity provider.
type Service struct {
	db        *tenantdb.DB
	keys      *envelope.Keyring
	argon     auth.Argon2Params
	now       func() time.Time
	dummyHash string
}

// NewService builds the provider.
func NewService(db *tenantdb.DB, keys *envelope.Keyring, opt Options) *Service {
	if opt.Argon2 == (auth.Argon2Params{}) {
		opt.Argon2 = auth.DefaultArgon2
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	// Verifying against a dummy hash for unknown emails keeps response time
	// independent of whether the account exists.
	dummy, _ := auth.HashPassword("dummy password for timing", opt.Argon2)
	return &Service{db: db, keys: keys, argon: opt.Argon2, now: opt.Now, dummyHash: dummy}
}

// Organization is the tenant row exposed to the UI.
type Organization struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`
	Profile
}

// BootstrapInput creates the instance organisation and its first owner.
type BootstrapInput struct {
	OrgName, Slug, Timezone, Currency string
	OwnerEmail, OwnerName             string
}

// Bootstrap creates the organisation and an owner without a password, and
// returns a one-time setup token (24h). Operator CLI only; never exposed
// over HTTP, so there is no "first visitor becomes owner" race.
func (s *Service) Bootstrap(ctx context.Context, in BootstrapInput) (string, error) {
	if _, err := s.db.InstanceTenant(ctx); err == nil {
		return "", ErrAlreadyBootstrapped
	} else if !errors.Is(err, tenantdb.ErrNoInstanceTenant) {
		return "", err
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if in.Currency == "" {
		in.Currency = "EUR"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || strings.TrimSpace(in.OrgName) == "" ||
		!strings.Contains(in.OwnerEmail, "@") || strings.TrimSpace(in.OwnerName) == "" {
		return "", ErrInvalidInput
	}
	tenant := uuid.Must(uuid.NewV7())
	var token string
	err := s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO organizations (id, name, slug, timezone, currency) VALUES ($1, $2, $3, $4, $5)`,
			tenant, strings.TrimSpace(in.OrgName), in.Slug, in.Timezone, strings.ToUpper(in.Currency)); err != nil {
			return fmt.Errorf("create organisation: %w", err)
		}
		userID, err := s.insertUser(ctx, tx, tenant, in.OwnerEmail, in.OwnerName)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, 'owner')`, tenant, userID); err != nil {
			return err
		}
		token, err = s.insertLink(ctx, tx, tenant, &userID, in.OwnerEmail, "owner", "bootstrap", nil, bootstrapTTL)
		return err
	})
	return token, err
}

// CompleteSetup sets the password through a bootstrap/reset link.
func (s *Service) CompleteSetup(ctx context.Context, token, password string) error {
	if err := auth.CheckPasswordPolicy(password); err != nil {
		return fmt.Errorf("%w: %v", ErrWeakPassword, err)
	}
	return s.useLink(ctx, token, func(tx pgx.Tx, tenant uuid.UUID, l link) error {
		if l.userID == nil || (l.purpose != "bootstrap" && l.purpose != "reset") {
			return ErrInvalidLink
		}
		return s.setPassword(ctx, tx, tenant, *l.userID, password)
	})
}

// Invite creates a one-time invite link for email with role (72h).
func (s *Service) Invite(ctx context.Context, by authz.Principal, email, role string) (string, error) {
	if !slices.Contains(Roles, role) {
		return "", ErrInvalidRole
	}
	if role == "owner" && !slices.Contains(by.Roles, "owner") {
		return "", ErrInvalidRole
	}
	if !strings.Contains(email, "@") {
		return "", ErrInvalidInput
	}
	tenant, err := uuid.Parse(by.OrgID)
	if err != nil {
		return "", ErrInvalidInput
	}
	creator := userIDFromSub(by.Sub)
	var token string
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var e error
		var linkID uuid.UUID
		token, linkID, e = s.insertLinkWithID(ctx, tx, tenant, nil, email, role, "invite", creator, inviteTTL)
		if e != nil {
			return e
		}
		return s.emit(ctx, tx, tenant, "member", "invited", map[string]uuid.UUID{"link_id": linkID})
	})
	return token, err
}

// AcceptInvite creates the invited user with a password and the role.
func (s *Service) AcceptInvite(ctx context.Context, token, displayName, password string) error {
	if err := auth.CheckPasswordPolicy(password); err != nil {
		return fmt.Errorf("%w: %v", ErrWeakPassword, err)
	}
	if strings.TrimSpace(displayName) == "" {
		return ErrInvalidInput
	}
	return s.useLink(ctx, token, func(tx pgx.Tx, tenant uuid.UUID, l link) error {
		if l.purpose != "invite" {
			return ErrInvalidLink
		}
		userID, err := s.insertUser(ctx, tx, tenant, l.email, displayName)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, $3)`, tenant, userID, l.role); err != nil {
			return err
		}
		return s.setPassword(ctx, tx, tenant, userID, password)
	})
}

// LoginInput is a password (+ TOTP) login.
type LoginInput struct{ Email, Password, TOTP string }

// LoginResult carries the new session cookie.
type LoginResult struct {
	Cookie       string
	Expires      time.Time
	TOTPEnrolled bool
}

// Login verifies credentials for the instance organisation. All failures
// except a missing TOTP code return ErrInvalidCredentials.
func (s *Service) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	tenant, err := s.db.InstanceTenant(ctx)
	if err != nil {
		_, _ = auth.VerifyPassword(in.Password, s.dummyHash)
		return LoginResult{}, ErrInvalidCredentials
	}
	var res LoginResult
	var outcome error
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		idx := dek.BlindIndex("users", "email", envelope.NormalizeEmail(in.Email))
		var (
			userID      uuid.UUID
			hashEnc     []byte
			totpEnc     []byte
			totpEnabled *time.Time
			lastStep    int64
			status      string
			failed      int
			lockedUntil *time.Time
		)
		err = tx.QueryRow(ctx, `SELECT id, password_hash_enc, totp_secret_enc, totp_enabled_at, totp_last_step,
		        status, failed_logins, locked_until FROM users WHERE email_bidx = $1 FOR UPDATE`, idx).
			Scan(&userID, &hashEnc, &totpEnc, &totpEnabled, &lastStep, &status, &failed, &lockedUntil)
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = auth.VerifyPassword(in.Password, s.dummyHash)
			outcome = ErrInvalidCredentials
			return nil
		}
		if err != nil {
			return err
		}
		now := s.now()
		if status != "active" || hashEnc == nil || (lockedUntil != nil && now.Before(*lockedUntil)) {
			_, _ = auth.VerifyPassword(in.Password, s.dummyHash)
			outcome = ErrInvalidCredentials
			return nil
		}
		hash, err := s.open(ctx, tx, tenant, "users", "password_hash_enc", userID, hashEnc)
		if err != nil {
			return err
		}
		ok, err := auth.VerifyPassword(in.Password, string(hash))
		if err != nil || !ok {
			outcome = ErrInvalidCredentials
			return s.recordFailure(ctx, tx, userID, failed, now)
		}
		amr := []string{"pwd"}
		if totpEnabled != nil {
			if in.TOTP == "" {
				outcome = ErrTOTPRequired
				return nil
			}
			secret, err := s.open(ctx, tx, tenant, "users", "totp_secret_enc", userID, totpEnc)
			if err != nil {
				return err
			}
			step, ok := auth.VerifyTOTP(secret, strings.TrimSpace(in.TOTP), now, lastStep)
			clear(secret)
			if !ok {
				outcome = ErrInvalidCredentials
				return s.recordFailure(ctx, tx, userID, failed, now)
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET totp_last_step = $2 WHERE id = $1`, userID, step); err != nil {
				return err
			}
			amr = append(amr, "otp")
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET failed_logins = 0, locked_until = NULL WHERE id = $1`, userID); err != nil {
			return err
		}
		cookie, expires, err := s.createSession(ctx, tx, tenant, &userID, nil, nil, amr, now)
		if err != nil {
			return err
		}
		res = LoginResult{Cookie: cookie, Expires: expires, TOTPEnrolled: totpEnabled != nil}
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

func (s *Service) recordFailure(ctx context.Context, tx pgx.Tx, userID uuid.UUID, failed int, now time.Time) error {
	failed++
	var locked *time.Time
	if failed >= maxFailedLogins {
		t := now.Add(lockoutFor)
		locked, failed = &t, 0
	}
	_, err := tx.Exec(ctx, `UPDATE users SET failed_logins = $2, locked_until = $3 WHERE id = $1`, userID, failed, locked)
	return err
}

// Authenticate implements auth.Authenticator for session cookies.
func (s *Service) Authenticate(r *http.Request) (authz.Principal, error) {
	c, err := r.Cookie(auth.SessionCookie)
	if err != nil {
		return authz.Principal{}, auth.ErrUnauthenticated
	}
	tenant, hash, err := auth.ParseSessionToken(c.Value)
	if err != nil {
		return authz.Principal{}, auth.ErrUnauthenticated
	}
	ctx := r.Context()
	now := s.now()
	var p authz.Principal
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var (
			id         uuid.UUID
			userID     *uuid.UUID
			deviceID   *uuid.UUID
			eventScope *uuid.UUID
			amr        []string
			authTime   time.Time
			lastSeen   time.Time
			expires    time.Time
		)
		err := tx.QueryRow(ctx, `SELECT s.id, s.user_id, s.device_id, s.event_scope, s.amr, s.auth_time, s.last_seen_at, s.expires_at
		  FROM sessions s
		  LEFT JOIN users u ON u.id = s.user_id
		  LEFT JOIN door_devices d ON d.id = s.device_id
		  WHERE s.token_hash = $1 AND s.revoked_at IS NULL
		    AND (s.user_id IS NULL OR u.status = 'active')
		    AND (s.device_id IS NULL OR d.revoked_at IS NULL)`, hash).
			Scan(&id, &userID, &deviceID, &eventScope, &amr, &authTime, &lastSeen, &expires)
		if err != nil {
			return auth.ErrUnauthenticated
		}
		if !now.Before(expires) || now.Sub(lastSeen) > sessionIdle {
			return auth.ErrUnauthenticated
		}
		p = authz.Principal{OrgID: tenant.String(), AMR: amr, AuthTime: authTime}
		if userID != nil {
			p.Sub = "local:" + userID.String()
			rows, err := tx.Query(ctx, `SELECT role FROM memberships WHERE user_id = $1 ORDER BY role`, *userID)
			if err != nil {
				return err
			}
			if p.Roles, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
				return err
			}
		} else {
			p.Sub = "device:" + deviceID.String()
			p.DeviceID = deviceID.String()
			p.Roles = []string{"door"}
			if eventScope != nil {
				p.EventScope = eventScope.String()
			}
		}
		if now.Sub(lastSeen) > touchEvery {
			_, err = tx.Exec(ctx, `UPDATE sessions SET last_seen_at = $2 WHERE id = $1`, id, now)
		}
		return err
	})
	if err != nil {
		return authz.Principal{}, auth.ErrUnauthenticated
	}
	return p, nil
}

// Logout revokes the session behind cookie.
func (s *Service) Logout(ctx context.Context, cookie string) error {
	tenant, hash, err := auth.ParseSessionToken(cookie)
	if err != nil {
		return nil
	}
	return s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL`, hash, s.now())
		return err
	})
}

// EnrollTOTP stores a new (not yet active) TOTP secret for the caller and
// returns it with its otpauth:// URI for the QR code.
func (s *Service) EnrollTOTP(ctx context.Context, p authz.Principal) ([]byte, string, error) {
	userID := userIDFromSub(p.Sub)
	tenant, err := uuid.Parse(p.OrgID)
	if userID == nil || err != nil {
		return nil, "", ErrInvalidInput
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		return nil, "", err
	}
	var email string
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var emailEnc []byte
		var enabled *time.Time
		if err := tx.QueryRow(ctx, `SELECT email_enc, totp_enabled_at FROM users WHERE id = $1`, *userID).Scan(&emailEnc, &enabled); err != nil {
			return err
		}
		if enabled != nil {
			return ErrInvalidInput // disabling and re-enrolling is a separate, step-up flow
		}
		plain, err := s.open(ctx, tx, tenant, "users", "email_enc", *userID, emailEnc)
		if err != nil {
			return err
		}
		email = string(plain)
		sealed, err := s.seal(ctx, tx, tenant, "users", "totp_secret_enc", *userID, secret)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE users SET totp_secret_enc = $2, updated_at = now() WHERE id = $1`, *userID, sealed)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return secret, auth.TOTPProvisioningURI(secret, totpIssuer, email), nil
}

// ConfirmTOTP activates the pending secret with a valid code and upgrades
// the current session to amr [pwd otp] (the user just proved possession).
func (s *Service) ConfirmTOTP(ctx context.Context, p authz.Principal, cookie, code string) error {
	userID := userIDFromSub(p.Sub)
	tenant, err := uuid.Parse(p.OrgID)
	if userID == nil || err != nil {
		return ErrInvalidInput
	}
	_, hash, err := auth.ParseSessionToken(cookie)
	if err != nil {
		return ErrInvalidInput
	}
	now := s.now()
	return s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var enc []byte
		var enabled *time.Time
		if err := tx.QueryRow(ctx, `SELECT totp_secret_enc, totp_enabled_at FROM users WHERE id = $1 FOR UPDATE`, *userID).Scan(&enc, &enabled); err != nil {
			return err
		}
		if enc == nil || enabled != nil {
			return ErrInvalidInput
		}
		secret, err := s.open(ctx, tx, tenant, "users", "totp_secret_enc", *userID, enc)
		if err != nil {
			return err
		}
		step, ok := auth.VerifyTOTP(secret, strings.TrimSpace(code), now, 0)
		clear(secret)
		if !ok {
			return ErrInvalidCredentials
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET totp_enabled_at = $2, totp_last_step = $3, updated_at = now() WHERE id = $1`, *userID, now, step); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE sessions SET amr = ARRAY['pwd','otp'], auth_time = $3 WHERE token_hash = $1 AND user_id = $2`, hash, *userID, now)
		return err
	})
}

// Org returns the caller's organisation.
func (s *Service) Org(ctx context.Context, p authz.Principal) (Organization, error) {
	tenant, err := uuid.Parse(p.OrgID)
	if err != nil {
		return Organization{}, ErrInvalidInput
	}
	var o Organization
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text, name, slug, timezone, currency, `+profileCols+` FROM organizations WHERE id = $1`, tenant).
			Scan(append([]any{&o.ID, &o.Name, &o.Slug, &o.Timezone, &o.Currency}, o.Profile.scanDest()...)...)
	})
	return o, err
}

// --- helpers ---------------------------------------------------------------

func userIDFromSub(sub string) *uuid.UUID {
	raw, ok := strings.CutPrefix(sub, "local:")
	if !ok {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

func (s *Service) seal(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, table, column string, row uuid.UUID, v []byte) ([]byte, error) {
	dek, err := s.keys.Current(ctx, tx, tenant)
	if err != nil {
		return nil, err
	}
	return dek.Seal(tenant, envelope.Field{Table: table, Column: column, RowID: row}, v)
}

func (s *Service) open(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, table, column string, row uuid.UUID, v []byte) ([]byte, error) {
	dek, err := s.keys.ForSealed(ctx, tx, tenant, v)
	if err != nil {
		return nil, err
	}
	return dek.Open(tenant, envelope.Field{Table: table, Column: column, RowID: row}, v)
}

func (s *Service) insertUser(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, email, name string) (uuid.UUID, error) {
	id := uuid.Must(uuid.NewV7())
	dek, err := s.keys.Current(ctx, tx, tenant)
	if err != nil {
		return uuid.Nil, err
	}
	norm := envelope.NormalizeEmail(email)
	emailEnc, err := dek.Seal(tenant, envelope.Field{Table: "users", Column: "email_enc", RowID: id}, []byte(norm))
	if err != nil {
		return uuid.Nil, err
	}
	nameEnc, err := dek.Seal(tenant, envelope.Field{Table: "users", Column: "display_name_enc", RowID: id}, []byte(strings.TrimSpace(name)))
	if err != nil {
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO users (id, tenant_id, email_enc, email_bidx, display_name_enc) VALUES ($1, $2, $3, $4, $5)`,
		id, tenant, emailEnc, dek.BlindIndex("users", "email", norm), nameEnc)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (s *Service) setPassword(ctx context.Context, tx pgx.Tx, tenant, userID uuid.UUID, password string) error {
	hash, err := auth.HashPassword(password, s.argon)
	if err != nil {
		return err
	}
	sealed, err := s.seal(ctx, tx, tenant, "users", "password_hash_enc", userID, []byte(hash))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET password_hash_enc = $2, failed_logins = 0, locked_until = NULL, updated_at = now() WHERE id = $1`, userID, sealed)
	return err
}

type link struct {
	id      uuid.UUID
	userID  *uuid.UUID
	email   string
	role    string
	purpose string
}

func (s *Service) insertLink(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, userID *uuid.UUID, email, role, purpose string, by *uuid.UUID, ttl time.Duration) (string, error) {
	tok, _, err := s.insertLinkWithID(ctx, tx, tenant, userID, email, role, purpose, by, ttl)
	return tok, err
}

func (s *Service) insertLinkWithID(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, userID *uuid.UUID, email, role, purpose string, by *uuid.UUID, ttl time.Duration) (string, uuid.UUID, error) {
	tok, err := auth.NewSessionToken(tenant) // same opaque format: v1.<tenant>.<random>
	if err != nil {
		return "", uuid.Nil, err
	}
	id := uuid.Must(uuid.NewV7())
	dek, err := s.keys.Current(ctx, tx, tenant)
	if err != nil {
		return "", uuid.Nil, err
	}
	norm := envelope.NormalizeEmail(email)
	emailEnc, err := dek.Seal(tenant, envelope.Field{Table: "setup_links", Column: "email_enc", RowID: id}, []byte(norm))
	if err != nil {
		return "", uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO setup_links (id, tenant_id, token_hash, user_id, email_enc, email_bidx, role, purpose, created_by, expires_at)
	  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, tenant, tok.Hash, userID, emailEnc, dek.BlindIndex("setup_links", "email", norm), role, purpose, by, s.now().Add(ttl))
	if err != nil {
		return "", uuid.Nil, err
	}
	return tok.Cookie, id, nil
}

// emit appends an identifiers-only event to the outbox in tx.
func (s *Service) emit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, aggregate, verb string, refs map[string]uuid.UUID) error {
	subject, ev, err := events.New(tenant, aggregate, verb, refs, s.now())
	if err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, subject, ev)
}

// useLink validates a one-time link, runs fn, and marks the link used in the
// same transaction.
func (s *Service) useLink(ctx context.Context, token string, fn func(pgx.Tx, uuid.UUID, link) error) error {
	tenant, hash, err := auth.ParseSessionToken(token)
	if err != nil {
		return ErrInvalidLink
	}
	return s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var l link
		var emailEnc []byte
		var expires time.Time
		var used *time.Time
		err := tx.QueryRow(ctx, `SELECT id, user_id, email_enc, role, purpose, expires_at, used_at
		  FROM setup_links WHERE token_hash = $1 FOR UPDATE`, hash).
			Scan(&l.id, &l.userID, &emailEnc, &l.role, &l.purpose, &expires, &used)
		if err != nil || used != nil || !s.now().Before(expires) {
			return ErrInvalidLink
		}
		email, err := s.open(ctx, tx, tenant, "setup_links", "email_enc", l.id, emailEnc)
		if err != nil {
			return err
		}
		l.email = string(email)
		if err := fn(tx, tenant, l); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE setup_links SET used_at = $2 WHERE id = $1`, l.id, s.now())
		return err
	})
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, userID, deviceID, eventScope *uuid.UUID, amr []string, now time.Time) (string, time.Time, error) {
	tok, err := auth.NewSessionToken(tenant)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := now.Add(sessionAbsolute)
	_, err = tx.Exec(ctx, `INSERT INTO sessions (id, tenant_id, token_hash, user_id, device_id, event_scope, amr, auth_time, last_seen_at, expires_at)
	  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9)`,
		uuid.Must(uuid.NewV7()), tenant, tok.Hash, userID, deviceID, eventScope, amr, now, expires)
	return tok.Cookie, expires, err
}
