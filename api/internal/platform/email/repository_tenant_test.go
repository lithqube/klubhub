package email

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// TestTenantScopedRepository_RequiresTenantInContext verifies that every
// tenant-scoped repository method refuses to run without a tenant in the
// context, BEFORE touching the database. The repository is constructed with
// a nil *tenantdb.DB: any database access would panic, so a clean
// tenantdb.ErrNoTenant return proves the guard fires first.
func TestTenantScopedRepository_RequiresTenantInContext(t *testing.T) {
	repo := NewEmailRepository(nil, true)
	ctx := context.Background() // no tenant
	id := uuid.New()
	ownerID := uuid.New()
	now := time.Now().UTC()

	t.Run("Create", func(t *testing.T) {
		_, err := repo.Create(ctx, &EmailMessage{ID: id, ToEmail: "to@example.com"})
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("Create err = %v, want ErrNoTenant", err)
		}
	})
	t.Run("GetByID", func(t *testing.T) {
		_, err := repo.GetByID(ctx, id)
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("GetByID err = %v, want ErrNoTenant", err)
		}
	})
	t.Run("ListByOwner", func(t *testing.T) {
		_, err := repo.ListByOwner(ctx, "invoice", ownerID)
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("ListByOwner err = %v, want ErrNoTenant", err)
		}
	})
	t.Run("UpdateStatus", func(t *testing.T) {
		_, err := repo.UpdateStatus(ctx, id, EmailStatusSent, "", &now)
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("UpdateStatus err = %v, want ErrNoTenant", err)
		}
	})
	t.Run("FinishAttempt", func(t *testing.T) {
		_, err := repo.FinishAttempt(ctx, &EmailMessage{ID: id, Attempts: 1, UpdatedAt: now}, EmailStatusSent, "", &now)
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("FinishAttempt err = %v, want ErrNoTenant", err)
		}
	})
	t.Run("ListByStatus", func(t *testing.T) {
		_, err := repo.ListByStatus(ctx, EmailStatusQueued, 10)
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("ListByStatus err = %v, want ErrNoTenant", err)
		}
	})
	t.Run("ClaimFailedForRetry", func(t *testing.T) {
		_, err := repo.ClaimFailedForRetry(ctx, 10, maxEmailAttempts, emailRetryBackoffBaseSeconds)
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("ClaimFailedForRetry err = %v, want ErrNoTenant", err)
		}
	})
	t.Run("RequeueStale", func(t *testing.T) {
		_, err := repo.RequeueStale(ctx, emailStaleSendingTimeout)
		if !errors.Is(err, tenantdb.ErrNoTenant) {
			t.Errorf("RequeueStale err = %v, want ErrNoTenant", err)
		}
	})
}

// TestTenantScopedRepository_WithSealerStillRequiresTenant verifies the
// tenant guard fires even when a FieldSealer is wired: the sealer must never
// be invoked outside a tenant scope (it would resolve the wrong DEK).
func TestTenantScopedRepository_WithSealerStillRequiresTenant(t *testing.T) {
	sealer := &recordingSealer{}
	repo := NewEmailRepository(nil, true).WithFieldSealer(sealer)

	_, err := repo.Create(context.Background(), &EmailMessage{ID: uuid.New(), ToEmail: "to@example.com"})
	if !errors.Is(err, tenantdb.ErrNoTenant) {
		t.Fatalf("Create err = %v, want ErrNoTenant", err)
	}
	if sealer.sealCalls != 0 || sealer.unsealCalls != 0 {
		t.Errorf("sealer invoked without tenant: seal=%d unseal=%d", sealer.sealCalls, sealer.unsealCalls)
	}
}

// recordingSealer counts FieldSealer invocations.
type recordingSealer struct {
	sealCalls   int
	unsealCalls int
}

func (s *recordingSealer) SealMessageFields(_ context.Context, _ pgx.Tx, _ *EmailMessage) ([]byte, error) {
	s.sealCalls++
	return nil, nil
}

func (s *recordingSealer) UnsealMessageFields(_ context.Context, _ pgx.Tx, _ *EmailMessage) error {
	s.unsealCalls++
	return nil
}
