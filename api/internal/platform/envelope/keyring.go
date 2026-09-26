package envelope

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrNoKey means the tenant has no key for the requested version (never
	// created, or crypto-shredded).
	ErrNoKey = errors.New("envelope: no tenant key")
	// ErrUnknownKEK means a DEK was wrapped by a KEK this process does not
	// hold (configure it as a previous KEK during rotation).
	ErrUnknownKEK = errors.New("envelope: DEK wrapped by an unknown KEK")
)

// Keyring loads, creates, rotates and shreds tenant DEKs stored in the
// tenant_keys table. Every method takes a tenant-scoped transaction from
// platform/tenantdb, so row level security applies to the key rows too.
//
// Unwrapped DEKs are cached per process; Shred wipes cached copies.
type Keyring struct {
	current *KEK
	byID    map[string]*KEK
	mu      sync.Mutex
	cache   map[cacheKey]*DEK
}

type cacheKey struct {
	tenant  uuid.UUID
	version uint32
}

// NewKeyring uses current to wrap new DEKs; previous KEKs can still unwrap
// DEKs created before a KEK rotation.
func NewKeyring(current *KEK, previous ...*KEK) *Keyring {
	byID := map[string]*KEK{current.ID(): current}
	for _, k := range previous {
		byID[k.ID()] = k
	}
	return &Keyring{current: current, byID: byID, cache: map[cacheKey]*DEK{}}
}

// Current returns the tenant's active DEK, creating version 1 on first use.
func (r *Keyring) Current(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (*DEK, error) {
	var version uint32
	err := tx.QueryRow(ctx,
		`SELECT key_version FROM tenant_keys WHERE tenant_id = $1 AND retired_at IS NULL`, tenant,
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return r.create(ctx, tx, tenant, 1)
	}
	if err != nil {
		return nil, fmt.Errorf("envelope: load active key: %w", err)
	}
	return r.Version(ctx, tx, tenant, version)
}

// ForSealed returns the DEK version a sealed value was written with.
func (r *Keyring) ForSealed(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, sealed []byte) (*DEK, error) {
	v, err := KeyVersionOf(sealed)
	if err != nil {
		return nil, err
	}
	return r.Version(ctx, tx, tenant, v)
}

// Version returns a specific DEK version (active or retired).
func (r *Keyring) Version(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, version uint32) (*DEK, error) {
	if dek := r.cached(tenant, version); dek != nil {
		return dek, nil
	}
	var kekID string
	var wrapped []byte
	err := tx.QueryRow(ctx,
		`SELECT kek_id, wrapped_dek FROM tenant_keys WHERE tenant_id = $1 AND key_version = $2`,
		tenant, version,
	).Scan(&kekID, &wrapped)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, fmt.Errorf("envelope: load key: %w", err)
	}
	kek, ok := r.byID[kekID]
	if !ok {
		return nil, fmt.Errorf("%w (%s)", ErrUnknownKEK, kekID)
	}
	dek, err := kek.UnwrapDEK(tenant, version, wrapped)
	if err != nil {
		return nil, err
	}
	return r.store(tenant, dek), nil
}

// Rotate retires the active DEK and creates the next version. Existing
// ciphertext stays readable through its recorded version; re-encryption is
// a separate background job.
func (r *Keyring) Rotate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (*DEK, error) {
	var next uint32
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(key_version), 0) + 1 FROM tenant_keys WHERE tenant_id = $1`, tenant,
	).Scan(&next); err != nil {
		return nil, fmt.Errorf("envelope: next key version: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE tenant_keys SET retired_at = now() WHERE tenant_id = $1 AND retired_at IS NULL`, tenant,
	); err != nil {
		return nil, fmt.Errorf("envelope: retire key: %w", err)
	}
	return r.create(ctx, tx, tenant, next)
}

// Shred removes every key row of the tenant and wipes cached copies. All
// data sealed for the tenant becomes permanently unreadable (GDPR Art. 17),
// including in backups taken before the shred.
func (r *Keyring) Shred(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	if _, err := tx.Exec(ctx, shredSQL, tenant); err != nil {
		return fmt.Errorf("envelope: shred keys: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, dek := range r.cache {
		if k.tenant == tenant {
			dek.Destroy()
			delete(r.cache, k)
		}
	}
	return nil
}

const shredSQL = "DELETE " + "FROM tenant_keys WHERE tenant_id = $1"

func (r *Keyring) create(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, version uint32) (*DEK, error) {
	dek, wrapped, err := GenerateDEK(r.current, tenant, version)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenant_keys (tenant_id, key_version, kek_id, wrapped_dek) VALUES ($1, $2, $3, $4)`,
		tenant, version, r.current.ID(), wrapped,
	); err != nil {
		dek.Destroy()
		return nil, fmt.Errorf("envelope: store key: %w", err)
	}
	return r.store(tenant, dek), nil
}

func (r *Keyring) cached(tenant uuid.UUID, version uint32) *DEK {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cache[cacheKey{tenant, version}]
}

func (r *Keyring) store(tenant uuid.UUID, dek *DEK) *DEK {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := cacheKey{tenant, dek.Version()}
	if existing, ok := r.cache[k]; ok {
		dek.Destroy()
		return existing
	}
	r.cache[k] = dek
	return dek
}
