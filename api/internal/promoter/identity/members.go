package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/promoter/sealed"
)

var (
	// ErrMemberNotFound: the user is not a member of the organisation.
	ErrMemberNotFound = errors.New("identity: member not found")
	// ErrCannotRemoveSelf: members leave through another member.
	ErrCannotRemoveSelf = errors.New("identity: cannot remove yourself")
	// ErrLastOwner: the organisation always keeps an owner.
	ErrLastOwner = errors.New("identity: cannot remove the last owner")
	// ErrOwnerRequired: only an owner can remove an owner.
	ErrOwnerRequired = errors.New("identity: only an owner can remove an owner")
)

// RemoveMember removes user from the caller's organisation (P2.6): its
// memberships go, its sessions are revoked, its unused setup/reset links
// are voided, and its member key and org key wraps are deleted. If it
// held a wrap of the org sealed key, the key is marked for rotation
// (member_removed). The user row stays (audit and created_by references).
// The caller cannot remove itself; only owners remove owners; the last
// owner cannot be removed.
func (s *Service) RemoveMember(ctx context.Context, by authz.Principal, user uuid.UUID) error {
	tenant, err := uuid.Parse(by.OrgID)
	if err != nil || user == uuid.Nil {
		return ErrInvalidInput
	}
	if self := userIDFromSub(by.Sub); self != nil && *self == user {
		return ErrCannotRemoveSelf
	}
	now := s.now()
	return s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		// Serialise membership removals so two owners cannot remove each
		// other at once and leave none.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('memberships:' || $1::text, 0))`, tenant.String()); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT role FROM memberships WHERE user_id = $1`, user)
		if err != nil {
			return err
		}
		roles, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if len(roles) == 0 {
			return ErrMemberNotFound
		}
		if slices.Contains(roles, "owner") {
			if !slices.Contains(by.Roles, "owner") {
				return ErrOwnerRequired
			}
			var others int
			if err := tx.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM memberships WHERE role = 'owner' AND user_id <> $1`, user).Scan(&others); err != nil {
				return err
			}
			if others == 0 {
				return ErrLastOwner
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM memberships WHERE user_id = $1`, user); err != nil {
			return err
		}
		sessions, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, user, now)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE setup_links SET used_at = $2 WHERE user_id = $1 AND used_at IS NULL`, user, now); err != nil {
			return err
		}
		pending, err := sealed.ForgetMember(ctx, tx, tenant, user, by.Sub, now)
		if err != nil {
			return err
		}
		if err := s.emit(ctx, tx, tenant, "member", "removed", map[string]uuid.UUID{"user_id": user}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, tenant, audit.Entry{
			ActorID: by.Sub, Action: "member.removed", Resource: "user:" + user.String(), Allowed: true,
			Reason: fmt.Sprintf("roles %v, %d sessions revoked, org key rotation pending: %t", roles, sessions.RowsAffected(), pending),
		})
	})
}
