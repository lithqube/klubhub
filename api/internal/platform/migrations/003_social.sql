-- +goose Up
ALTER TABLE social_accounts
    ADD COLUMN IF NOT EXISTS ig_user_id  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS account_name_ig TEXT,
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ;

-- Add unique constraint on platform for ON CONFLICT upsert support.
ALTER TABLE social_accounts
    ADD CONSTRAINT social_accounts_platform_unique UNIQUE (platform);

CREATE TABLE scheduled_posts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id       UUID NOT NULL REFERENCES social_accounts(id),
    status           TEXT NOT NULL DEFAULT 'draft'
                         CHECK (status IN ('draft','scheduled','publishing','published','failed','permanently_failed')),
    post_type        TEXT NOT NULL CHECK (post_type IN ('feed','story')),
    caption          TEXT NOT NULL DEFAULT '',
    image_minio_path TEXT NOT NULL DEFAULT '',
    scheduled_at_utc TIMESTAMPTZ NOT NULL,
    timezone_name    TEXT NOT NULL DEFAULT 'UTC',
    retry_count      INT NOT NULL DEFAULT 0,
    next_retry_at    TIMESTAMPTZ,
    last_error       TEXT,
    container_id     TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX ON scheduled_posts (status, scheduled_at_utc)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS scheduled_posts;
ALTER TABLE social_accounts
    DROP CONSTRAINT IF EXISTS social_accounts_platform_unique,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS account_name_ig,
    DROP COLUMN IF EXISTS ig_user_id;
