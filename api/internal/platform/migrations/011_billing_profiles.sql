-- +goose Up
-- Billing identity for the DJ's invoicing workflow. Separate from
-- user_settings (public artist biography) so that legal entity, billing
-- address, tax status and payment instructions can be required for
-- issuing invoices without leaking into the EPK export.
CREATE TABLE IF NOT EXISTS billing_profiles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Legal entity information
    legal_name      TEXT NOT NULL,
    trading_name    TEXT NOT NULL DEFAULT '',
    entity_kind     TEXT NOT NULL DEFAULT 'individual'
        CHECK (entity_kind IN ('individual', 'sole_trader', 'partnership', 'llc', 'corp', 'other')),
    tax_id          TEXT NOT NULL DEFAULT '',
    tax_id_kind     TEXT NOT NULL DEFAULT ''
        CHECK (tax_id_kind IN ('', 'vat', 'ein', 'gst', 'abn', 'other')),
    -- Contact for billing matters (may differ from promoter contact)
    contact_email   TEXT NOT NULL,
    contact_phone   TEXT NOT NULL DEFAULT '',
    -- Postal billing address
    address_line1   TEXT NOT NULL,
    address_line2   TEXT NOT NULL DEFAULT '',
    address_city    TEXT NOT NULL,
    address_region  TEXT NOT NULL DEFAULT '',
    address_postal  TEXT NOT NULL,
    address_country  TEXT NOT NULL DEFAULT '',  -- ISO 3166-1 alpha-2, empty allowed
    -- Free-form legal/jurisdiction text (jurisdiction is user-supplied; never
    -- inferred from locale). Required for issuing invoices.
    jurisdiction    TEXT NOT NULL DEFAULT '',
    -- Payment instructions surfaced on issued invoices (free-text; the UI
    -- surfaces a "Review before Issue" gate so the user can fix typos).
    payment_instructions TEXT NOT NULL DEFAULT '',
    -- Currency the DJ quotes by default. Per-currency is enforced per invoice;
    -- this is just the most common value, not a sum target.
    default_currency TEXT NOT NULL DEFAULT 'EUR'
        CHECK (char_length(default_currency) = 3),
    -- Optimistic concurrency token (same shape as user_settings).
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Only one row: this is a singleton profile per deployment.
CREATE UNIQUE INDEX IF NOT EXISTS billing_profiles_singleton_idx
    ON billing_profiles ((TRUE));

-- +goose Down
DROP TABLE IF EXISTS billing_profiles;
