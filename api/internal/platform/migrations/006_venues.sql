-- +goose Up

-- Venues table: reusable venue database
CREATE TABLE IF NOT EXISTS venues (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    city                TEXT NOT NULL DEFAULT '',
    country             TEXT NOT NULL DEFAULT '',
    capacity            INT,  -- optional
    website             TEXT,  -- optional
    tech_contact_name   TEXT NOT NULL DEFAULT '',
    tech_contact_email  TEXT NOT NULL DEFAULT '',
    tech_contact_phone  TEXT NOT NULL DEFAULT '',
    notes               TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ  -- soft delete
);

CREATE INDEX IF NOT EXISTS venues_name_idx ON venues(name);
CREATE INDEX IF NOT EXISTS venues_deleted_at_idx ON venues(deleted_at) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS venues;
