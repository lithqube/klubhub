-- +goose Up
-- Invoices: per-gig, per-currency, atomically issued.
-- Numbering: <prefix>-<zero-padded-sequence> scoped to (DJ, currency).
-- Status: draft → issued → paid / cancelled / corrected.
-- All money amounts stored as integer minor units (cents/øre/etc) in a
-- DECIMAL(19,4) column via shopspring/decimal to avoid float drift.
CREATE TABLE IF NOT EXISTS invoices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gig_id          UUID NOT NULL REFERENCES gigs(id) ON DELETE RESTRICT,
    -- Billing profile snapshot at issuance (denormalized so re-issuing
    -- never mutates history). JSONB stores the exact BillingProfile shape.
    billing_profile JSONB NOT NULL DEFAULT '{}',
    -- Numbering: invoice_number is the human-readable "INV-0001-EUR".
    -- prefix is configurable (default "INV"), sequence is per (prefix, currency).
    invoice_number  TEXT NOT NULL,
    number_prefix   TEXT NOT NULL DEFAULT 'INV',
    number_seq      BIGINT NOT NULL DEFAULT 0,
    -- Currency: 3-letter ISO 4217, uppercase. Per-currency totals are
    -- enforced; mixed-currency line items are rejected at issuance.
    currency        TEXT NOT NULL
        CHECK (char_length(currency) = 3 AND currency = upper(currency)),
    -- Amounts in minor units (cents). Line items sum to subtotal; tax
    -- is a separate percentage applied at issuance; total = subtotal + tax.
    subtotal_minor  BIGINT NOT NULL DEFAULT 0,
    tax_rate_bps    BIGINT NOT NULL DEFAULT 0,       -- basis points (10000 = 100%)
    tax_minor       BIGINT NOT NULL DEFAULT 0,
    total_minor     BIGINT NOT NULL DEFAULT 0,
    -- Status machine: draft → issued → (paid | cancelled | corrected).
    -- "corrected" means a correction invoice exists that supersedes this one.
    status          TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'issued', 'paid', 'cancelled', 'corrected')),
    -- The issued timestamp is the legal issuance moment. Immutable after
    -- first transition to 'issued'.
    issued_at       TIMESTAMPTZ,
    -- Payment tracking: expected date, actual date, reference.
    due_at          TIMESTAMPTZ,
    paid_at         TIMESTAMPTZ,
    payment_ref     TEXT NOT NULL DEFAULT '',
    -- Notes for the DJ (never sent to promoter).
    internal_notes  TEXT NOT NULL DEFAULT '',
    -- Optimistic concurrency token (updated_at). Same pattern as billing_profiles.
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Numbering uniqueness per (prefix, currency). The sequence is advanced
-- atomically inside the issuance transaction so no two invoices ever get
-- the same number. The index also supports fast "next number" queries.
CREATE UNIQUE INDEX IF NOT EXISTS invoices_number_unique_idx
    ON invoices (number_prefix, currency, number_seq);

-- Fast lookups by gig, status, currency.
CREATE INDEX IF NOT EXISTS invoices_gig_id_idx ON invoices (gig_id);
CREATE INDEX IF NOT EXISTS invoices_status_idx ON invoices (status);
CREATE INDEX IF NOT EXISTS invoices_currency_idx ON invoices (currency);

-- +goose Down
DROP TABLE IF EXISTS invoices;