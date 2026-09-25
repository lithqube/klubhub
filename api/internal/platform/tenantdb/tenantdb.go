// Package tenantdb is the only way Promoter code gets a database transaction.
//
// Every Promoter table is protected by Postgres row level security keyed on
// the transaction-local setting app.tenant_id (plan §13.3). WithTenant opens a
// transaction, sets that key with set_config(..., is_local => true) so it can
// never outlive the transaction on a pooled connection, runs fn, and commits
// or rolls back. There is deliberately no "default tenant": a query that does
// not go through this package fails in the database instead of returning
// another tenant's rows.
package tenantdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNoTenant is returned when no tenant is supplied; fn is not run.
	ErrNoTenant = errors.New("tenantdb: no tenant in scope")
	// ErrPrivilegedRole is returned by AssertRuntimeRole when the pool's role
	// could bypass row level security.
	ErrPrivilegedRole = errors.New("tenantdb: runtime database role can bypass row level security")
)

type ctxKey struct{}

// ContextWithTenant returns a context carrying the tenant for Run. The auth
// middleware sets it from the verified principal; it must never come from
// client input.
func ContextWithTenant(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, tenantID)
}

// TenantFromContext returns the tenant set by ContextWithTenant.
func TenantFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKey{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

// Run is WithTenant with the tenant taken from the context.
func Run(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	id, ok := TenantFromContext(ctx)
	if !ok {
		return ErrNoTenant
	}
	return WithTenant(ctx, pool, id, fn)
}

// WithTenant runs fn in a transaction scoped to tenantID.
func WithTenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(pgx.Tx) error) error {
	if tenantID == uuid.Nil {
		return ErrNoTenant
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID.String()); err != nil {
			return fmt.Errorf("tenantdb: set tenant: %w", err)
		}
		return fn(tx)
	})
}

// AssertRuntimeRole refuses a pool whose role is a superuser or has
// BYPASSRLS: row level security does not apply to those roles, even with
// FORCE. The Promoter binary calls this at boot and exits on error.
func AssertRuntimeRole(ctx context.Context, pool *pgxpool.Pool) error {
	var role string
	var super, bypass bool
	err := pool.QueryRow(ctx,
		`SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`,
	).Scan(&role, &super, &bypass)
	if err != nil {
		return fmt.Errorf("tenantdb: inspect runtime role: %w", err)
	}
	if super || bypass {
		return fmt.Errorf("%w (role %q, superuser=%t, bypassrls=%t)", ErrPrivilegedRole, role, super, bypass)
	}
	return nil
}
