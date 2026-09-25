-- +goose Up
-- VAT settings for the supplier (the DJ). vat_exempt_small_business drives
-- the "exempt" suggestion (e.g. DE §19 UStG, FR franchise en base);
-- default_vat_rate_bps is the domestic rate suggested for same-country
-- customers (basis points, 1900 = 19%).
ALTER TABLE billing_profiles
    ADD COLUMN vat_exempt_small_business BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN default_vat_rate_bps BIGINT NOT NULL DEFAULT 0
        CONSTRAINT billing_profiles_default_vat_rate_check
        CHECK (default_vat_rate_bps BETWEEN 0 AND 10000);

-- +goose Down
ALTER TABLE billing_profiles
    DROP COLUMN IF EXISTS default_vat_rate_bps,
    DROP COLUMN IF EXISTS vat_exempt_small_business;
