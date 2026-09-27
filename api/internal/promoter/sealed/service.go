package sealed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// Service implements member keys, org key wraps, rotation, recovery and
// the ban list. Every method runs in the tenant carried by ctx.
type Service struct {
	db   *tenantdb.DB
	keys *envelope.Keyring // opens member names/emails for the recipients list only
	now  func() time.Time
}

// NewService builds the service; now defaults to time.Now.
func NewService(db *tenantdb.DB, keys *envelope.Keyring, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, keys: keys, now: now}
}

func (s *Service) clock() time.Time { return s.now().UTC() }

func tenantOf(ctx context.Context) (uuid.UUID, error) {
	id, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return uuid.Nil, tenantdb.ErrNoTenant
	}
	return id, nil
}

// userOf returns the local user id of the principal (member keys and
// member wraps are bound to it); nil for other identity providers.
func userOf(p authz.Principal) *uuid.UUID {
	raw, ok := strings.CutPrefix(p.Sub, "local:")
	if !ok {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

// lockTenant serialises every sealed-tier write of a tenant (setup,
// wraps, rotation, ban-list writes, recipient removal), so a rotation sees
// a stable ban list and a writer never races a version change.
func lockTenant(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('sealed:' || $1::text, 0))`, tenant.String())
	return err
}

// activeVersion is the active OSK version, nil before setup.
func activeVersion(ctx context.Context, tx pgx.Tx) (*int, error) {
	var v int
	err := tx.QueryRow(ctx, `SELECT version FROM org_sealed_keys WHERE status = 'active'`).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func record(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, actor, action, resource, reason string) error {
	return audit.Record(ctx, tx, tenant, audit.Entry{ActorID: actor, Action: action, Resource: resource, Allowed: true, Reason: reason})
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, aggregate, verb string, refs map[string]uuid.UUID) error {
	subject, ev, err := events.New(tenant, aggregate, verb, refs, s.clock())
	if err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, subject, ev)
}

// ---------------------------------------------------------- member key ---

// MyKey returns the caller's member key.
func (s *Service) MyKey(ctx context.Context, p authz.Principal) (MemberKey, error) {
	var out MemberKey
	user := userOf(p)
	if user == nil {
		return out, ErrLocalIdentity
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var pub, private, kdf []byte
		err := tx.QueryRow(ctx, `SELECT public_key, private_sealed, kdf FROM member_keys WHERE user_id = $1`, *user).Scan(&pub, &private, &kdf)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoMemberKey
		}
		if err != nil {
			return err
		}
		out = MemberKey{PublicKey: Encode(pub), PrivateSealed: Encode(private)}
		return json.Unmarshal(kdf, &out.KDF)
	})
	return out, err
}

// PutMyKey creates the caller's member key, or re-wraps its private key
// (same public key, e.g. a passphrase change). A different public key
// replaces the key only when replace allows it (the handler checks the
// security.manage policy: an owner recovering with the kit); the caller's
// wraps are then deleted, since they were sealed to the old key. Otherwise
// it is 409 public_key_mismatch. created reports a new key.
func (s *Service) PutMyKey(ctx context.Context, p authz.Principal, in MemberKeyInput, replace func() error) (MemberKey, bool, error) {
	var out MemberKey
	user := userOf(p)
	if user == nil {
		return out, false, ErrLocalIdentity
	}
	k, err := in.parse()
	if err != nil {
		return out, false, err
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, false, err
	}
	kdf, err := json.Marshal(k.kdf)
	if err != nil {
		return out, false, err
	}
	created := false
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		var current []byte
		err := tx.QueryRow(ctx, `SELECT public_key FROM member_keys WHERE user_id = $1`, *user).Scan(&current)
		action := "keys.member_key_rewrapped"
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			created, action = true, "keys.member_key_created"
			if _, err := tx.Exec(ctx, `INSERT INTO member_keys (tenant_id, user_id, public_key, private_sealed, kdf, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $6)`, tenant, *user, k.pub, k.private, kdf, s.clock()); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			reason := ""
			if !bytes.Equal(current, k.pub) {
				if replace == nil {
					return conflict("public_key_mismatch")
				}
				if err := replace(); err != nil {
					var denied *DeniedError
					if errors.As(err, &denied) {
						return conflict("public_key_mismatch")
					}
					return err
				}
				tag, err := tx.Exec(ctx, `DELETE FROM org_key_wraps WHERE recipient_kind = 'member' AND recipient_id = $1`, *user)
				if err != nil {
					return err
				}
				action, reason = "keys.member_key_replaced", fmt.Sprintf("%d wraps of the old key deleted", tag.RowsAffected())
			}
			if _, err := tx.Exec(ctx, `UPDATE member_keys SET public_key = $2, private_sealed = $3, kdf = $4, updated_at = $5 WHERE user_id = $1`,
				*user, k.pub, k.private, kdf, s.clock()); err != nil {
				return err
			}
			return record(ctx, tx, tenant, p.Sub, action, "member_key:"+user.String(), reason)
		}
		return record(ctx, tx, tenant, p.Sub, action, "member_key:"+user.String(), "")
	})
	if err != nil {
		return out, false, err
	}
	return MemberKey{PublicKey: Encode(k.pub), PrivateSealed: Encode(k.private), KDF: k.kdf}, created, nil
}

// ------------------------------------------------------------ org key ---

// OrgStatus reports whether the org key is set up, its active version, the
// caller's wrap for it and the recovery fingerprint.
func (s *Service) OrgStatus(ctx context.Context, p authz.Principal) (OrgStatus, error) {
	var out OrgStatus
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		out, err = orgStatus(ctx, tx, userOf(p))
		return err
	})
	return out, err
}

func orgStatus(ctx context.Context, tx pgx.Tx, user *uuid.UUID) (OrgStatus, error) {
	out := OrgStatus{Status: StatusNotSetup}
	var v int
	var fp string
	err := tx.QueryRow(ctx, `SELECT version, recovery_fingerprint FROM org_sealed_keys WHERE status = 'active'`).Scan(&v, &fp)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Status, out.Version, out.RecoveryFingerprint = StatusReady, &v, &fp
	var pending bool
	err = tx.QueryRow(ctx, `SELECT rotation_pending FROM org_key_state`).Scan(&pending)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if pending {
		out.Status = StatusRotationPending
	}
	if user != nil {
		var w []byte
		err := tx.QueryRow(ctx, `SELECT wrap_sealed FROM org_key_wraps WHERE version = $1 AND recipient_kind = 'member' AND recipient_id = $2`,
			v, *user).Scan(&w)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		out.MyWrap = encodePtr(w)
	}
	return out, nil
}

// rolePriority orders a member's roles; the recipients list shows the
// highest.
var rolePriority = []string{"owner", "admin", "booker", "finance", "marketing", "door"}

func topRole(roles []string) string {
	for _, r := range rolePriority {
		if slices.Contains(roles, r) {
			return r
		}
	}
	if len(roles) > 0 {
		return roles[0]
	}
	return ""
}

// Recipients lists members and door devices with their public keys and
// whether they hold a wrap of the active version.
func (s *Service) Recipients(ctx context.Context) (Recipients, error) {
	out := Recipients{Members: []MemberRecipient{}, Devices: []DeviceRecipient{}}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if out.Version, err = activeVersion(ctx, tx); err != nil {
			return err
		}
		version := 0
		if out.Version != nil {
			version = *out.Version
		}
		rows, err := tx.Query(ctx, `SELECT m.user_id, array_agg(m.role ORDER BY m.role), u.email_enc, u.display_name_enc, k.public_key,
		    EXISTS (SELECT 1 FROM org_key_wraps w WHERE w.version = $1 AND w.recipient_kind = 'member' AND w.recipient_id = m.user_id)
		  FROM memberships m
		  JOIN users u ON u.id = m.user_id
		  LEFT JOIN member_keys k ON k.user_id = m.user_id
		  GROUP BY m.user_id, u.email_enc, u.display_name_enc, k.public_key, u.created_at
		  ORDER BY u.created_at, m.user_id`, version)
		if err != nil {
			return err
		}
		type memberRow struct {
			MemberRecipient
			roles           []string
			emailEnc, nameE []byte
			pub             []byte
		}
		members, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (memberRow, error) {
			var m memberRow
			err := r.Scan(&m.UserID, &m.roles, &m.emailEnc, &m.nameE, &m.pub, &m.HasWrap)
			return m, err
		})
		if err != nil {
			return err
		}
		for _, m := range members {
			email, err := s.open(ctx, tx, tenant, "email_enc", m.UserID, m.emailEnc)
			if err != nil {
				return err
			}
			name, err := s.open(ctx, tx, tenant, "display_name_enc", m.UserID, m.nameE)
			if err != nil {
				return err
			}
			r := m.MemberRecipient
			r.Email, r.Name, r.Role, r.PublicKey = email, name, topRole(m.roles), encodePtr(m.pub)
			out.Members = append(out.Members, r)
		}
		rows, err = tx.Query(ctx, `SELECT d.id, d.label, d.public_key, d.revoked_at IS NOT NULL,
		    EXISTS (SELECT 1 FROM org_key_wraps w WHERE w.version = $1 AND w.recipient_kind = 'device' AND w.recipient_id = d.id)
		  FROM door_devices d ORDER BY d.revoked_at NULLS FIRST, d.created_at, d.id`, version)
		if err != nil {
			return err
		}
		devices, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (DeviceRecipient, error) {
			var d DeviceRecipient
			var pub []byte
			err := r.Scan(&d.DeviceID, &d.Label, &pub, &d.Revoked, &d.HasWrap)
			d.PublicKey = encodePtr(pub)
			return d, err
		})
		if err != nil {
			return err
		}
		out.Devices = append(out.Devices, devices...)
		return nil
	})
	return out, err
}

func (s *Service) open(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, column string, user uuid.UUID, v []byte) (string, error) {
	if v == nil {
		return "", nil
	}
	dek, err := s.keys.ForSealed(ctx, tx, tenant, v)
	if err != nil {
		return "", err
	}
	plain, err := dek.Open(tenant, envelope.Field{Table: "users", Column: column, RowID: user}, v)
	return string(plain), err
}

// checkRecipients verifies every wrap goes to a recipient that can hold
// it: a current member with a member key, or a registered, unrevoked door
// device with a public key.
func checkRecipients(ctx context.Context, tx pgx.Tx, field string, wraps []wrap) error {
	var members, devices []uuid.UUID
	for _, w := range wraps {
		if w.kind == KindMember {
			members = append(members, w.id)
		} else {
			devices = append(devices, w.id)
		}
	}
	type state struct{ member, key, revoked bool }
	known := map[string]state{}
	if len(members) > 0 {
		rows, err := tx.Query(ctx, `SELECT u.id, EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id),
		    EXISTS (SELECT 1 FROM member_keys k WHERE k.user_id = u.id)
		  FROM users u WHERE u.id = ANY($1)`, members)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id uuid.UUID
			var st state
			if err := rows.Scan(&id, &st.member, &st.key); err != nil {
				rows.Close()
				return err
			}
			known[KindMember+":"+id.String()] = st
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	if len(devices) > 0 {
		rows, err := tx.Query(ctx, `SELECT id, public_key IS NOT NULL, revoked_at IS NOT NULL FROM door_devices WHERE id = ANY($1)`, devices)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id uuid.UUID
			var st state
			if err := rows.Scan(&id, &st.key, &st.revoked); err != nil {
				rows.Close()
				return err
			}
			st.member = true
			known[KindDevice+":"+id.String()] = st
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	for i, w := range wraps {
		f := fmt.Sprintf("%s[%d].recipient_id", field, i)
		st, ok := known[w.key()]
		switch {
		case !ok || !st.member:
			return invalid(f, "unknown "+w.kind)
		case st.revoked:
			return invalid(f, "device is revoked")
		case !st.key && w.kind == KindMember:
			return invalid(f, "member has no key")
		case !st.key:
			return invalid(f, "device has no public key")
		}
	}
	return nil
}

func hasSelf(wraps []wrap, user uuid.UUID) bool {
	return slices.ContainsFunc(wraps, func(w wrap) bool { return w.kind == KindMember && w.id == user })
}

// requireOwnKey is the 409 for callers who have not created a member key.
func requireOwnKey(ctx context.Context, tx pgx.Tx, user uuid.UUID) error {
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM member_keys WHERE user_id = $1)`, user).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return conflict("no_member_key")
	}
	return nil
}

// upsertWraps stores wraps of version (a recipient's wrap is replaced).
func upsertWraps(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, version int, wraps []wrap, by *uuid.UUID, now time.Time) error {
	for _, w := range wraps {
		var id *uuid.UUID
		if w.kind != KindRecovery {
			id = &w.id
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_key_wraps (id, tenant_id, version, recipient_kind, recipient_id, wrap_sealed, created_by, created_at)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		  ON CONFLICT (tenant_id, version, recipient_kind, COALESCE(recipient_id, '00000000-0000-0000-0000-000000000000'::uuid))
		  DO UPDATE SET wrap_sealed = EXCLUDED.wrap_sealed, created_by = EXCLUDED.created_by, created_at = EXCLUDED.created_at`,
			uuid.Must(uuid.NewV7()), tenant, version, w.kind, id, w.blob, by, now); err != nil {
			return err
		}
	}
	return nil
}

// createdBy is the users FK of an actor (nil for non-local principals).
func createdBy(p authz.Principal) *uuid.UUID { return userOf(p) }

// Setup creates OSK version 1 with the recovery wrap and the given wraps,
// which must include the caller's own. 409 already_setup when any version
// exists.
func (s *Service) Setup(ctx context.Context, p authz.Principal, in SetupInput) (OrgStatus, error) {
	var out OrgStatus
	user := userOf(p)
	if user == nil {
		return out, ErrLocalIdentity
	}
	if in.Version != 1 {
		return out, invalid("version", "must be 1")
	}
	rec, err := in.Recovery.parse("recovery")
	if err != nil {
		return out, err
	}
	wraps, err := parseWraps("wraps", in.Wraps)
	if err != nil {
		return out, err
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_sealed_keys)`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return conflict("already_setup")
		}
		if err := requireOwnKey(ctx, tx, *user); err != nil {
			return err
		}
		if !hasSelf(wraps, *user) {
			return invalid("wraps", "must include your own wrap")
		}
		if err := checkRecipients(ctx, tx, "wraps", wraps); err != nil {
			return err
		}
		now := s.clock()
		if _, err := tx.Exec(ctx, `INSERT INTO org_sealed_keys (tenant_id, version, status, recovery_public_key, recovery_fingerprint, created_by, created_at)
		  VALUES ($1, 1, 'active', $2, $3, $4, $5)`, tenant, rec.pub, rec.fingerprint, createdBy(p), now); err != nil {
			return err
		}
		all := append([]wrap{{kind: KindRecovery, blob: rec.wrap}}, wraps...)
		if err := upsertWraps(ctx, tx, tenant, 1, all, createdBy(p), now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_key_state (tenant_id) VALUES ($1) ON CONFLICT (tenant_id) DO UPDATE
		  SET rotation_pending = false, rotation_reason = NULL, pending_since = NULL`, tenant); err != nil {
			return err
		}
		if err := record(ctx, tx, tenant, p.Sub, "keys.org_setup", "org_key:1",
			fmt.Sprintf("version 1, recovery wrap, %d recipient wraps", len(wraps))); err != nil {
			return err
		}
		out, err = orgStatus(ctx, tx, user)
		return err
	})
	return out, err
}

// AddWraps grants the active OSK version to members and devices (a
// recipient's existing wrap is replaced).
func (s *Service) AddWraps(ctx context.Context, p authz.Principal, in WrapsInput) (WrapsResult, error) {
	var out WrapsResult
	wraps, err := parseWraps("wraps", in.Wraps)
	if err != nil {
		return out, err
	}
	if len(wraps) == 0 {
		return out, invalid("wraps", "at least one wrap")
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		v, err := activeVersion(ctx, tx)
		if err != nil {
			return err
		}
		if v == nil {
			return conflict("not_setup")
		}
		if in.Version != nil && *in.Version != *v {
			return conflict("version_conflict")
		}
		if err := checkRecipients(ctx, tx, "wraps", wraps); err != nil {
			return err
		}
		if err := upsertWraps(ctx, tx, tenant, *v, wraps, createdBy(p), s.clock()); err != nil {
			return err
		}
		out = WrapsResult{Version: *v, Added: len(wraps)}
		return record(ctx, tx, tenant, p.Sub, "keys.wraps_added", fmt.Sprintf("org_key:%d", *v), fmt.Sprintf("%d wraps", len(wraps)))
	})
	return out, err
}

// PutDeviceWrap stores the active OSK version's wrap for a door device.
func (s *Service) PutDeviceWrap(ctx context.Context, p authz.Principal, device uuid.UUID, in DeviceWrapInput) (DeviceWrapResult, error) {
	var out DeviceWrapResult
	blob, err := DecodeWrap("wrap", in.Wrap)
	if err != nil {
		return out, err
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		var hasKey, revoked bool
		err := tx.QueryRow(ctx, `SELECT public_key IS NOT NULL, revoked_at IS NOT NULL FROM door_devices WHERE id = $1`, device).Scan(&hasKey, &revoked)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		v, err := activeVersion(ctx, tx)
		if err != nil {
			return err
		}
		switch {
		case v == nil:
			return conflict("not_setup")
		case in.Version != nil && *in.Version != *v:
			return conflict("version_conflict")
		case revoked:
			return invalid("device", "device is revoked")
		case !hasKey:
			return invalid("device", "device has no public key")
		}
		if err := upsertWraps(ctx, tx, tenant, *v, []wrap{{kind: KindDevice, id: device, blob: blob}}, createdBy(p), s.clock()); err != nil {
			return err
		}
		out = DeviceWrapResult{DeviceID: device, Version: *v}
		return record(ctx, tx, tenant, p.Sub, "keys.device_wrap_set", "door_device:"+device.String(), fmt.Sprintf("org key version %d", *v))
	})
	return out, err
}

// Recovery returns the active version's recovery wrap and public key (for
// recovering with the kit). Reading it is audited.
func (s *Service) Recovery(ctx context.Context, p authz.Principal) (Recovery, error) {
	var out Recovery
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var pub, w []byte
		err := tx.QueryRow(ctx, `SELECT k.version, k.recovery_public_key, k.recovery_fingerprint, w.wrap_sealed
		  FROM org_sealed_keys k JOIN org_key_wraps w ON w.version = k.version AND w.recipient_kind = 'recovery'
		  WHERE k.status = 'active'`).Scan(&out.Version, &pub, &out.Fingerprint, &w)
		if errors.Is(err, pgx.ErrNoRows) {
			return conflict("not_setup")
		}
		if err != nil {
			return err
		}
		out.PublicKey, out.Wrap = Encode(pub), Encode(w)
		return record(ctx, tx, tenant, p.Sub, "keys.recovery_read", fmt.Sprintf("org_key:%d", out.Version), "")
	})
	return out, err
}

// Rotate replaces the active OSK version atomically: the old version is
// retired, the new one activated with its recovery wrap and wraps (the
// caller's included), every unexpired ban entry is replaced by its
// re-encryption (the set of ids must match exactly), expired leftovers and
// the old version's wraps are deleted and rotation_pending is cleared.
// 409 version_conflict when from_version is not the active version.
func (s *Service) Rotate(ctx context.Context, p authz.Principal, in RotateInput) (OrgStatus, error) {
	var out OrgStatus
	user := userOf(p)
	if user == nil {
		return out, ErrLocalIdentity
	}
	if in.FromVersion < 1 {
		return out, invalid("from_version", "required")
	}
	if in.ToVersion != in.FromVersion+1 {
		return out, invalid("to_version", "must be from_version + 1")
	}
	rec, err := in.Recovery.parse("recovery")
	if err != nil {
		return out, err
	}
	wraps, err := parseWraps("wraps", in.Wraps)
	if err != nil {
		return out, err
	}
	if len(in.BanEntries) > MaxBanEntries {
		return out, invalid("ban_entries", fmt.Sprintf("at most %d entries", MaxBanEntries))
	}
	entries := make(map[uuid.UUID][]byte, len(in.BanEntries))
	for i, e := range in.BanEntries {
		f := fmt.Sprintf("ban_entries[%d]", i)
		if e.ID == uuid.Nil {
			return out, invalid(f+".id", "required")
		}
		if _, dup := entries[e.ID]; dup {
			return out, invalid(f+".id", "duplicate entry")
		}
		blob, err := DecodeEntrySealed(f+".entry_sealed", e.EntrySealed)
		if err != nil {
			return out, err
		}
		entries[e.ID] = blob
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		v, err := activeVersion(ctx, tx)
		if err != nil {
			return err
		}
		if v == nil {
			return conflict("not_setup")
		}
		if *v != in.FromVersion {
			return conflict("version_conflict")
		}
		if err := requireOwnKey(ctx, tx, *user); err != nil {
			return err
		}
		if !hasSelf(wraps, *user) {
			return invalid("wraps", "must include your own wrap")
		}
		if err := checkRecipients(ctx, tx, "wraps", wraps); err != nil {
			return err
		}
		now := s.clock()
		rows, err := tx.Query(ctx, `SELECT id FROM ban_entries WHERE expires_at > $1`, now)
		if err != nil {
			return err
		}
		live, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		missing := 0
		for _, id := range live {
			if _, ok := entries[id]; !ok {
				missing++
			}
		}
		if extra := len(entries) - (len(live) - missing); missing > 0 || extra > 0 {
			return invalid("ban_entries", fmt.Sprintf("must re-encrypt every ban entry exactly (%d missing, %d unknown)", missing, extra))
		}
		if _, err := tx.Exec(ctx, `UPDATE org_sealed_keys SET status = 'retired', retired_at = $2 WHERE version = $1`, *v, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_sealed_keys (tenant_id, version, status, recovery_public_key, recovery_fingerprint, created_by, created_at)
		  VALUES ($1, $2, 'active', $3, $4, $5, $6)`, tenant, in.ToVersion, rec.pub, rec.fingerprint, createdBy(p), now); err != nil {
			return err
		}
		all := append([]wrap{{kind: KindRecovery, blob: rec.wrap}}, wraps...)
		if err := upsertWraps(ctx, tx, tenant, in.ToVersion, all, createdBy(p), now); err != nil {
			return err
		}
		// Expired entries are still sealed under the old key; the retention
		// job would delete them anyway.
		expired, err := tx.Exec(ctx, `DELETE FROM ban_entries WHERE expires_at <= $1`, now)
		if err != nil {
			return err
		}
		for id, blob := range entries {
			if _, err := tx.Exec(ctx, `UPDATE ban_entries SET key_version = $2, entry_sealed = $3, updated_at = $4 WHERE id = $1`,
				id, in.ToVersion, blob, now); err != nil {
				return err
			}
		}
		oldWraps, err := tx.Exec(ctx, `DELETE FROM org_key_wraps WHERE version <> $1`, in.ToVersion)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_key_state (tenant_id) VALUES ($1) ON CONFLICT (tenant_id) DO UPDATE
		  SET rotation_pending = false, rotation_reason = NULL, pending_since = NULL`, tenant); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, tenant, "keys", "rotated", nil); err != nil {
			return err
		}
		if err := record(ctx, tx, tenant, p.Sub, "keys.rotated", fmt.Sprintf("org_key:%d", in.ToVersion),
			fmt.Sprintf("version %d → %d, %d recipient wraps, %d ban entries re-encrypted, %d expired deleted, %d old wraps deleted",
				*v, in.ToVersion, len(wraps), len(entries), expired.RowsAffected(), oldWraps.RowsAffected())); err != nil {
			return err
		}
		out, err = orgStatus(ctx, tx, user)
		return err
	})
	return out, err
}

// ------------------------------------------------- recipients removed ---

// ForgetMember deletes a removed member's key and wraps inside the
// caller's tenant transaction. If the member held a wrap (so had access to
// the OSK), rotation_pending is set with reason member_removed.
func ForgetMember(ctx context.Context, tx pgx.Tx, tenant, user uuid.UUID, actor string, now time.Time) (bool, error) {
	if err := lockTenant(ctx, tx, tenant); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM member_keys WHERE user_id = $1`, user); err != nil {
		return false, err
	}
	return forget(ctx, tx, tenant, KindMember, user, ReasonMemberRemoved, actor, now)
}

// ForgetDevice deletes a revoked door device's wraps inside the caller's
// tenant transaction and sets rotation_pending (device_revoked) if it held
// one.
func ForgetDevice(ctx context.Context, tx pgx.Tx, tenant, device uuid.UUID, actor string, now time.Time) (bool, error) {
	if err := lockTenant(ctx, tx, tenant); err != nil {
		return false, err
	}
	return forget(ctx, tx, tenant, KindDevice, device, ReasonDeviceRevoked, actor, now)
}

func forget(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, kind string, id uuid.UUID, reason, actor string, now time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM org_key_wraps WHERE recipient_kind = $1 AND recipient_id = $2`, kind, id)
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO org_key_state (tenant_id, rotation_pending, rotation_reason, pending_since) VALUES ($1, true, $2, $3)
	  ON CONFLICT (tenant_id) DO UPDATE SET rotation_pending = true, rotation_reason = EXCLUDED.rotation_reason,
	    pending_since = COALESCE(org_key_state.pending_since, EXCLUDED.pending_since)`, tenant, reason, now); err != nil {
		return false, err
	}
	return true, record(ctx, tx, tenant, actor, "keys.recipient_removed", kind+":"+id.String(),
		fmt.Sprintf("%d wraps deleted; rotation pending (%s)", tag.RowsAffected(), reason))
}

// ForDevice is the door bundle's sealed block for device: its wrap of the
// active version and the unexpired ban entries, or nil when the org is not
// set up or the device has no wrap for the active version.
func ForDevice(ctx context.Context, tx pgx.Tx, device uuid.UUID, now time.Time) (*DoorSealed, error) {
	var out DoorSealed
	var w []byte
	err := tx.QueryRow(ctx, `SELECT k.version, w.wrap_sealed FROM org_sealed_keys k
	  JOIN org_key_wraps w ON w.version = k.version AND w.recipient_kind = 'device' AND w.recipient_id = $1
	  WHERE k.status = 'active'`, device).Scan(&out.KeyVersion, &w)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out.Wrap = Encode(w)
	rows, err := tx.Query(ctx, `SELECT id, entry_sealed, expires_at FROM ban_entries
	  WHERE key_version = $1 AND expires_at > $2 ORDER BY created_at, id`, out.KeyVersion, now)
	if err != nil {
		return nil, err
	}
	out.BanEntries, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (DoorBanEntry, error) {
		var e DoorBanEntry
		var blob []byte
		err := r.Scan(&e.ID, &blob, &e.ExpiresAt)
		e.EntrySealed, e.ExpiresAt = Encode(blob), e.ExpiresAt.UTC()
		return e, err
	})
	if err != nil {
		return nil, err
	}
	if out.BanEntries == nil {
		out.BanEntries = []DoorBanEntry{}
	}
	return &out, nil
}
