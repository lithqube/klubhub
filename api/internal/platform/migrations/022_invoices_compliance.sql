-- +goose Up
-- EU/US-compliant invoicing (docs/INVOICING.md):
--   * customer party snapshot + source contact
--   * VAT treatment, legal note, per-rate tax breakdown
--   * withholding and net payable (balances use net payable)
--   * numbers are allocated at issue, never on drafts (gap-free series)
--   * credit notes (kind = 'credit_note') reverse an issued invoice;
--     statuses 'credited' and 'corrected'.
ALTER TABLE invoices
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'invoice'
        CONSTRAINT invoices_kind_check CHECK (kind IN ('invoice', 'credit_note')),
    ADD COLUMN credits_invoice_id UUID REFERENCES invoices(id) ON DELETE RESTRICT,
    ADD COLUMN replaced_by_invoice_id UUID REFERENCES invoices(id) ON DELETE RESTRICT,
    ADD COLUMN customer JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN customer_contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    ADD COLUMN supply_date DATE,
    ADD COLUMN vat_treatment TEXT NOT NULL DEFAULT 'none'
        CONSTRAINT invoices_vat_treatment_check CHECK (vat_treatment IN
            ('domestic', 'reverse_charge', 'exempt', 'outside_scope', 'us_sales_tax', 'none')),
    ADD COLUMN tax_note TEXT NOT NULL DEFAULT '',
    ADD COLUMN withholding_rate_bps BIGINT NOT NULL DEFAULT 0
        CONSTRAINT invoices_withholding_rate_check CHECK (withholding_rate_bps BETWEEN 0 AND 10000),
    ADD COLUMN withholding_minor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN net_payable_minor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN tax_breakdown JSONB NOT NULL DEFAULT '[]';

-- Backfill money columns for existing rows: no withholding existed, so
-- net payable is the total; the breakdown is the single invoice-level rate.
UPDATE invoices SET net_payable_minor = total_minor;
UPDATE invoices
SET tax_breakdown = jsonb_build_array(jsonb_build_object(
        'rate_bps', tax_rate_bps, 'taxable_minor', subtotal_minor, 'tax_minor', tax_minor))
WHERE subtotal_minor <> 0 OR tax_minor <> 0;

-- Drafts default their supply date to the gig date (issued documents are
-- immutable and keep NULL).
UPDATE invoices i
SET supply_date = to_char(g.date, 'YYYY-MM-DD')::date
FROM gigs g
WHERE g.id = i.gig_id AND i.status = 'draft';

-- Numbers are now allocated at issue. Rows that were never issued (drafts,
-- and cancelled drafts) give their provisional number back.
ALTER TABLE invoices
    ALTER COLUMN invoice_number DROP NOT NULL,
    ALTER COLUMN number_seq DROP NOT NULL,
    ALTER COLUMN number_seq DROP DEFAULT;
UPDATE invoices SET invoice_number = NULL, number_seq = NULL
WHERE status = 'draft' OR (status = 'cancelled' AND issued_at IS NULL);

ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_status_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_status_check CHECK (status IN
    ('draft', 'issued', 'paid', 'cancelled', 'credited', 'corrected'));

ALTER TABLE invoices
    ADD CONSTRAINT invoices_number_pair_check
        CHECK ((invoice_number IS NULL) = (number_seq IS NULL)),
    ADD CONSTRAINT invoices_draft_unnumbered_check
        CHECK (status <> 'draft' OR invoice_number IS NULL),
    ADD CONSTRAINT invoices_issued_numbered_check
        CHECK (status NOT IN ('issued', 'paid', 'credited', 'corrected') OR invoice_number IS NOT NULL),
    ADD CONSTRAINT invoices_credit_note_ref_check
        CHECK ((kind = 'credit_note') = (credits_invoice_id IS NOT NULL));

-- An invoice is reversed at most once.
CREATE UNIQUE INDEX IF NOT EXISTS invoices_one_credit_note_idx
    ON invoices (credits_invoice_id) WHERE credits_invoice_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS invoices_kind_idx ON invoices (kind);

-- +goose Down
-- Lossy by necessity: credit notes become plain invoices and 'credited'
-- maps to 'corrected' (both mean "superseded").
DROP INDEX IF EXISTS invoices_kind_idx;
DROP INDEX IF EXISTS invoices_one_credit_note_idx;
ALTER TABLE invoices
    DROP CONSTRAINT IF EXISTS invoices_credit_note_ref_check,
    DROP CONSTRAINT IF EXISTS invoices_issued_numbered_check,
    DROP CONSTRAINT IF EXISTS invoices_draft_unnumbered_check,
    DROP CONSTRAINT IF EXISTS invoices_number_pair_check;

UPDATE invoices SET status = 'corrected' WHERE status = 'credited';
ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_status_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_status_check CHECK (status IN
    ('draft', 'issued', 'paid', 'cancelled', 'corrected'));

-- The old schema requires a number on every row: give unnumbered rows the
-- next free numbers in their series.
WITH series_max AS (
    SELECT number_prefix, currency, COALESCE(MAX(number_seq), 0) AS max_seq
    FROM invoices GROUP BY number_prefix, currency
), ranked AS (
    SELECT i.id, i.number_prefix, i.currency,
           m.max_seq + row_number() OVER (PARTITION BY i.number_prefix, i.currency ORDER BY i.created_at, i.id) AS seq
    FROM invoices i
    JOIN series_max m ON m.number_prefix = i.number_prefix AND m.currency = i.currency
    WHERE i.number_seq IS NULL
)
UPDATE invoices
SET number_seq = ranked.seq,
    invoice_number = upper(ranked.number_prefix) || '-' ||
        CASE WHEN length(ranked.seq::text) >= 4 THEN ranked.seq::text ELSE lpad(ranked.seq::text, 4, '0') END
        || '-' || ranked.currency
FROM ranked
WHERE invoices.id = ranked.id;

ALTER TABLE invoices
    ALTER COLUMN number_seq SET DEFAULT 0,
    ALTER COLUMN number_seq SET NOT NULL,
    ALTER COLUMN invoice_number SET NOT NULL;

ALTER TABLE invoices
    DROP COLUMN IF EXISTS tax_breakdown,
    DROP COLUMN IF EXISTS net_payable_minor,
    DROP COLUMN IF EXISTS withholding_minor,
    DROP COLUMN IF EXISTS withholding_rate_bps,
    DROP COLUMN IF EXISTS tax_note,
    DROP COLUMN IF EXISTS vat_treatment,
    DROP COLUMN IF EXISTS supply_date,
    DROP COLUMN IF EXISTS customer_contact_id,
    DROP COLUMN IF EXISTS customer,
    DROP COLUMN IF EXISTS replaced_by_invoice_id,
    DROP COLUMN IF EXISTS credits_invoice_id,
    DROP COLUMN IF EXISTS kind;
