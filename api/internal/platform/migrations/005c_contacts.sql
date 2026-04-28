-- +goose Up

-- Contact type enum
CREATE TYPE contact_type AS ENUM ('promoter', 'agent', 'label', 'other');

-- Contacts table: reusable contact database
CREATE TABLE IF NOT EXISTS contacts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    company             TEXT,  -- optional
    email               TEXT NOT NULL DEFAULT '',
    phone               TEXT NOT NULL DEFAULT '',
    type                contact_type NOT NULL DEFAULT 'other',
    notes               TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ  -- soft delete
);

CREATE INDEX IF NOT EXISTS contacts_name_idx ON contacts(name);
CREATE INDEX IF NOT EXISTS contacts_type_idx ON contacts(type);
CREATE INDEX IF NOT EXISTS contacts_deleted_at_idx ON contacts(deleted_at) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS contacts;
DROP TYPE IF EXISTS contact_type;
