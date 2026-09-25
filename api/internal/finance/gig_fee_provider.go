package finance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGGigFeeProvider reads the fee for an invoice straight from the gigs
// table (005_gigs.sql). The gig module stores fees as DECIMAL(12,2) major
// units; Postgres does the ×100 so no float ever touches the amount.
type PGGigFeeProvider struct {
	pool *pgxpool.Pool
}

// NewPGGigFeeProvider returns a GigFeeProvider backed by pool.
func NewPGGigFeeProvider(pool *pgxpool.Pool) *PGGigFeeProvider {
	return &PGGigFeeProvider{pool: pool}
}

// GetGigFeeInfo returns the gig's fee as a single invoice line. Tax is
// zero until a per-profile tax rate exists; the DJ adjusts the draft.
// Soft-deleted gigs are treated as missing.
func (p *PGGigFeeProvider) GetGigFeeInfo(ctx context.Context, gigID uuid.UUID) (*GigFeeInfo, error) {
	var (
		feeMinor          int64
		currency, event   string
		venue, city, date string
	)
	err := p.pool.QueryRow(ctx, `
		SELECT (fee_amount * 100)::BIGINT, upper(fee_currency), event_name, venue, city,
		       to_char(date, 'YYYY-MM-DD')
		FROM gigs WHERE id = $1 AND deleted_at IS NULL`, gigID).
		Scan(&feeMinor, &currency, &event, &venue, &city, &date)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load gig fee: %w", err)
	}
	return &GigFeeInfo{
		ID:              gigID,
		Currency:        currency,
		FeeMinor:        feeMinor,
		TotalMinor:      feeMinor,
		LineDescription: gigLineDescription(event, venue, city, date),
	}, nil
}

// gigLineDescription builds "DJ performance – Event @ Venue, City (date)",
// skipping empty parts.
func gigLineDescription(event, venue, city, date string) string {
	desc := "DJ performance"
	if event != "" {
		desc += " – " + event
	}
	var where []string
	if venue != "" {
		where = append(where, venue)
	}
	if city != "" {
		where = append(where, city)
	}
	if len(where) > 0 {
		desc += " @ " + strings.Join(where, ", ")
	}
	if date != "" {
		desc += " (" + date + ")"
	}
	return desc
}
