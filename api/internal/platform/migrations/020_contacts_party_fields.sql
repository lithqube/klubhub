-- +goose Up
-- Contacts gain the address / tax fields of an invoice Party so a draft
-- invoice's customer can be pre-filled from the gig's promoter contact.
-- Empty string is "not set", matching the rest of the schema; existing rows
-- get the defaults.
ALTER TABLE contacts
    ADD COLUMN address_line1 TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_line2 TEXT NOT NULL DEFAULT '',
    ADD COLUMN city          TEXT NOT NULL DEFAULT '',
    ADD COLUMN region        TEXT NOT NULL DEFAULT '',
    ADD COLUMN postal_code   TEXT NOT NULL DEFAULT '',
    ADD COLUMN country       TEXT NOT NULL DEFAULT '',  -- ISO 3166-1 alpha-2, uppercase
    ADD COLUMN vat_id        TEXT NOT NULL DEFAULT '',  -- EU VAT ID incl. country prefix
    ADD COLUMN tax_id        TEXT NOT NULL DEFAULT '',  -- other tax id (EIN etc.), never an SSN
    ADD COLUMN is_business   BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE contacts
    DROP COLUMN IF EXISTS is_business,
    DROP COLUMN IF EXISTS tax_id,
    DROP COLUMN IF EXISTS vat_id,
    DROP COLUMN IF EXISTS country,
    DROP COLUMN IF EXISTS postal_code,
    DROP COLUMN IF EXISTS region,
    DROP COLUMN IF EXISTS city,
    DROP COLUMN IF EXISTS address_line2,
    DROP COLUMN IF EXISTS address_line1;
