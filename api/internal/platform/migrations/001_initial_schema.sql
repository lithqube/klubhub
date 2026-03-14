-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- user_settings singleton (INFRA-07)
CREATE TABLE user_settings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dj_name         TEXT NOT NULL DEFAULT '',
    logo_path       TEXT NOT NULL DEFAULT '',
    default_colors  JSONB NOT NULL DEFAULT '{}',
    default_template TEXT NOT NULL DEFAULT '',
    visible_fields  JSONB NOT NULL DEFAULT '{}',
    social_links    JSONB NOT NULL DEFAULT '{}',
    bio_short       TEXT NOT NULL DEFAULT '',
    bio_long        TEXT NOT NULL DEFAULT '',
    contact_info    TEXT NOT NULL DEFAULT '',
    invoice_prefix  TEXT NOT NULL DEFAULT 'INV',
    user_id         UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);
CREATE INDEX ON user_settings(deleted_at) WHERE deleted_at IS NULL;

-- social_accounts stub (INFRA-09) — access_token encrypted by app layer
CREATE TABLE social_accounts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform     TEXT NOT NULL CHECK (platform IN ('instagram')),
    account_name TEXT NOT NULL DEFAULT '',
    access_token TEXT NOT NULL DEFAULT '',
    token_expiry TIMESTAMPTZ,
    status       TEXT NOT NULL DEFAULT 'disconnected'
                     CHECK (status IN ('connected','disconnected')),
    user_id      UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS social_accounts;
DROP TABLE IF EXISTS user_settings;
