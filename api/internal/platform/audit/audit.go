// Package audit writes the append-only, tenant-scoped audit log (plan
// §13.7). Actor ids are pseudonymous subject ids; no IP addresses or other
// personal data are stored.
package audit

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Entry is one audited decision or action.
type Entry struct {
	ActorID  string // local:<uuid> | device:<uuid> | zitadel:<id> | system
	Action   string
	Resource string
	Allowed  bool
	Reason   string
}

// Record appends e for tenant inside the caller's tenant transaction.
func Record(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, e Entry) error {
	actorType := "system"
	switch {
	case strings.HasPrefix(e.ActorID, "device:"):
		actorType = "device"
	case strings.HasPrefix(e.ActorID, "local:"), strings.HasPrefix(e.ActorID, "zitadel:"):
		actorType = "user"
	}
	decision := "deny"
	if e.Allowed {
		decision = "allow"
	}
	var reason *string
	if e.Reason != "" {
		reason = &e.Reason
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit_log (id, tenant_id, actor_type, actor_id, action, resource, decision, reason)
	  VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		uuid.Must(uuid.NewV7()), tenant, actorType, e.ActorID, e.Action, e.Resource, decision, reason)
	return err
}
