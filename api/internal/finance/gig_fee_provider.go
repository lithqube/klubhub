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

// GetGigFeeInfo returns the gig's fee as a single invoice line, the gig
// date (default supply date) and the pre-filled customer. Soft-deleted
// gigs are treated as missing.
//
// Customer pre-fill order (docs/INVOICING.md §1): the gig's reader contact
// (gigs.gig_reader_contact_id), else a contact linked with role
// 'promoter' in gig_contacts, else the gig's free-text promoter name and
// email only.
func (p *PGGigFeeProvider) GetGigFeeInfo(ctx context.Context, gigID uuid.UUID) (*GigFeeInfo, error) {
	var (
		feeMinor                    int64
		currency, event             string
		venue, city, date           string
		promoterName, promoterEmail string
		readerContactID             *uuid.UUID
	)
	err := p.pool.QueryRow(ctx, `
		SELECT (fee_amount * 100)::BIGINT, upper(fee_currency), event_name, venue, city,
		       to_char(date, 'YYYY-MM-DD'), promoter_name, promoter_email, gig_reader_contact_id
		FROM gigs WHERE id = $1 AND deleted_at IS NULL`, gigID).
		Scan(&feeMinor, &currency, &event, &venue, &city, &date, &promoterName, &promoterEmail, &readerContactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load gig fee: %w", err)
	}
	customer, err := p.customerFor(ctx, gigID, readerContactID)
	if err != nil {
		return nil, err
	}
	if customer == nil && (promoterName != "" || promoterEmail != "") {
		customer = &Party{LegalName: promoterName, Email: promoterEmail}
	}
	return &GigFeeInfo{
		ID:              gigID,
		Currency:        currency,
		FeeMinor:        feeMinor,
		LineDescription: gigLineDescription(event, venue, city, date),
		Date:            date,
		Customer:        customer,
	}, nil
}

// customerFor loads the gig's reader contact, falling back to a promoter
// linked through gig_contacts. Returns nil when neither exists.
func (p *PGGigFeeProvider) customerFor(ctx context.Context, gigID uuid.UUID, readerContactID *uuid.UUID) (*Party, error) {
	var (
		c  Party
		id uuid.UUID
	)
	err := p.pool.QueryRow(ctx, `
		SELECT c.id, c.name, COALESCE(c.company, ''), c.email,
		       c.address_line1, c.address_line2, c.city, c.region, c.postal_code,
		       upper(c.country), c.vat_id, c.tax_id, c.is_business
		FROM contacts c
		WHERE c.deleted_at IS NULL
		  AND (c.id = $2 OR c.id IN (
		        SELECT gc.contact_id FROM gig_contacts gc
		        WHERE gc.gig_id = $1 AND lower(gc.role) = 'promoter'))
		ORDER BY COALESCE(c.id = $2, false) DESC, c.name, c.id
		LIMIT 1`, gigID, readerContactID).
		Scan(&id, &c.LegalName, &c.Company, &c.Email,
			&c.AddressLine1, &c.AddressLine2, &c.City, &c.Region, &c.PostalCode,
			&c.Country, &c.VATID, &c.TaxID, &c.IsBusiness)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load gig customer contact: %w", err)
	}
	c.ContactID = &id
	return &c, nil
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
