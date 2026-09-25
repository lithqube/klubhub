package finance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists BillingProfile rows. There is at most one row
// (enforced by the singleton unique index).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository backed by the given pgxpool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Pool exposes the underlying connection pool for integration tests that
// need raw SQL. Application code should not call this.
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// Get returns the singleton billing profile, creating an empty one on
// first call so the rest of the app always has something to PUT against.
// The returned profile's UpdatedAt is the concurrency token callers must
// echo back on the next PUT.
func (r *Repository) Get(ctx context.Context) (*BillingProfile, error) {
	var p BillingProfile
	err := r.pool.QueryRow(ctx, `
		SELECT id, legal_name, trading_name, entity_kind, tax_id, tax_id_kind,
		       contact_email, contact_phone,
		       address_line1, address_line2, address_city, address_region,
		       address_postal, address_country,
		       jurisdiction, payment_instructions, default_currency,
		       vat_exempt_small_business, default_vat_rate_bps,
		       updated_at, created_at
		FROM billing_profiles
		ORDER BY created_at ASC
		LIMIT 1`).Scan(
		&p.ID, &p.LegalName, &p.TradingName, &p.EntityKind, &p.TaxID, &p.TaxIDKind,
		&p.ContactEmail, &p.ContactPhone,
		&p.AddressLine1, &p.AddressLine2, &p.AddressCity, &p.AddressRegion,
		&p.AddressPostal, &p.AddressCountry,
		&p.Jurisdiction, &p.PaymentInstructions, &p.DefaultCurrency,
		&p.VATExemptSmallBusiness, &p.DefaultVATRateBps,
		&p.UpdatedAt, &p.CreatedAt,
	)
	if err == nil {
		return &p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get billing profile: %w", err)
	}
	// No row yet — seed an empty one and return it.
	if err := r.seedEmpty(ctx); err != nil {
		return nil, fmt.Errorf("seed empty billing profile: %w", err)
	}
	return r.Get(ctx)
}

// Update replaces the singleton profile if and only if the caller's
// UpdatedAt token matches the row's current UpdatedAt. Returns ErrConflict
// on token mismatch, never overwrites silently. The DB enforces the
// concurrency check via WHERE updated_at = $N; we also refresh
// updated_at = now() on success so subsequent PUTs use the new token.
func (r *Repository) Update(ctx context.Context, req *UpdateBillingProfileRequest) (*BillingProfile, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE billing_profiles SET
			legal_name           = $2,
			trading_name         = $3,
			entity_kind          = $4,
			tax_id               = $5,
			tax_id_kind          = $6,
			contact_email        = $7,
			contact_phone        = $8,
			address_line1        = $9,
			address_line2        = $10,
			address_city         = $11,
			address_region       = $12,
			address_postal       = $13,
			address_country      = $14,
			jurisdiction         = $15,
			payment_instructions = $16,
			default_currency     = $17,
			vat_exempt_small_business = $18,
			default_vat_rate_bps = $19,
			updated_at           = now()
		WHERE updated_at = $1`,
		req.UpdatedAt,
		req.LegalName, req.TradingName, string(req.EntityKind), req.TaxID, string(req.TaxIDKind),
		req.ContactEmail, req.ContactPhone,
		req.AddressLine1, req.AddressLine2, req.AddressCity, req.AddressRegion,
		req.AddressPostal, req.AddressCountry,
		req.Jurisdiction, req.PaymentInstructions, req.DefaultCurrency,
		req.VATExemptSmallBusiness, req.DefaultVATRateBps,
	)
	if err != nil {
		return nil, fmt.Errorf("update billing profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either no row (shouldn't happen because Get seeds) or stale token.
		// We treat both as ErrConflict so the caller can decide.
		return nil, ErrConflict
	}
	return r.Get(ctx)
}

// seedEmpty inserts an empty profile so future Get() calls always find a
// row to lock against.
func (r *Repository) seedEmpty(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO billing_profiles (
			legal_name, contact_email, address_line1, address_city,
			address_postal, jurisdiction, default_currency
		) VALUES ('', '', '', '', '', '', 'EUR')
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return fmt.Errorf("insert empty billing profile: %w", err)
	}
	return nil
}

// Sentinel imports keep pgx referenced (so go.mod does not strip it) —
// the import is needed for the ErrNoRows comparison above.
var _ = pgx.ErrNoRows
var _ = uuid.Nil
