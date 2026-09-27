package retention

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/events"
)

// BanPurge is one tenant's expired ban entries deleted by a run.
type BanPurge struct {
	TenantID uuid.UUID
	Deleted  int
}

// PurgeExpiredBans deletes, tenant by tenant, the sealed ban entries whose
// expires_at has passed (P2.6: only the expiry is plaintext, so this needs
// no key). Each deleted entry emits banlist.updated (its id only); each
// tenant with deletions gets one audit entry with the count. A failing
// tenant does not stop the others.
func (s *Service) PurgeExpiredBans(ctx context.Context) ([]BanPurge, error) {
	tenants, err := s.db.Tenants(ctx)
	if err != nil {
		return nil, err
	}
	var out []BanPurge
	var errs []error
	for _, tenant := range tenants {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		var n int
		err := s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			now := s.clock()
			rows, err := tx.Query(ctx, `DELETE FROM ban_entries WHERE expires_at <= $1 RETURNING id`, now)
			if err != nil {
				return err
			}
			ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			if err != nil || len(ids) == 0 {
				return err
			}
			for _, id := range ids {
				subject, ev, err := events.New(tenant, "banlist", "updated", map[string]uuid.UUID{"entry_id": id}, now)
				if err != nil {
					return err
				}
				if err := events.Enqueue(ctx, tx, subject, ev); err != nil {
					return err
				}
			}
			n = len(ids)
			return audit.Record(ctx, tx, tenant, audit.Entry{
				ActorID: SystemActor, Action: "banlist.expired_purged", Resource: "org:" + tenant.String(), Allowed: true,
				Reason: fmt.Sprintf("%d expired ban entries deleted", n),
			})
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenant, err))
			continue
		}
		if n > 0 {
			out = append(out, BanPurge{TenantID: tenant, Deleted: n})
		}
	}
	return out, errors.Join(errs...)
}

// ExpiredBans counts expired ban entries across tenants (dry run).
func (s *Service) ExpiredBans(ctx context.Context) (int, error) {
	tenants, err := s.db.Tenants(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, tenant := range tenants {
		err := s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			var n int
			err := tx.QueryRow(ctx, `SELECT count(*) FROM ban_entries WHERE expires_at <= $1`, s.clock()).Scan(&n)
			total += n
			return err
		})
		if err != nil {
			return total, fmt.Errorf("tenant %s: %w", tenant, err)
		}
	}
	return total, nil
}
