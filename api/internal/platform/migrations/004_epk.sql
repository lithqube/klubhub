-- +goose Up

-- EPK content singleton — one row per installation
CREATE TABLE IF NOT EXISTS epk_content (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bio_short          TEXT NOT NULL DEFAULT '',
    bio_long           TEXT NOT NULL DEFAULT '',
    tech_rider         TEXT NOT NULL DEFAULT '',
    stage_plot_path    TEXT NOT NULL DEFAULT '',
    gig_highlights     JSONB NOT NULL DEFAULT '[]',
    press_quotes       JSONB NOT NULL DEFAULT '[]',
    photo_paths        JSONB NOT NULL DEFAULT '[]',
    section_visibility JSONB NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enforce singleton: only one row allowed via unique index on constant expression
CREATE UNIQUE INDEX IF NOT EXISTS epk_content_singleton ON epk_content ((true));

-- EPK export history — hard delete allowed (no deleted_at)
CREATE TABLE IF NOT EXISTS epk_exports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    minio_path  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS epk_exports;
DROP INDEX IF EXISTS epk_content_singleton;
DROP TABLE IF EXISTS epk_content;
