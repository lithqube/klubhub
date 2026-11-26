-- +goose Up

-- Promo checklist per release (Phase 6, REL-05/REL-06): the steps every
-- release repeats, plus the template a user keeps for the next one.
CREATE TABLE IF NOT EXISTS release_checklist_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id          UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    position            INT NOT NULL DEFAULT 0,
    label               TEXT NOT NULL,
    done                BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS release_checklist_items_release_idx ON release_checklist_items(release_id, position);

-- The user's own default template: the checklist they saved for future
-- releases. One row per instance (the OSS app has a single operator).
CREATE TABLE IF NOT EXISTS release_checklist_templates (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label               TEXT NOT NULL,
    items               JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS release_checklist_templates_singleton_idx ON release_checklist_templates ((1));

-- +goose Down
DROP TABLE IF EXISTS release_checklist_templates;
DROP TABLE IF EXISTS release_checklist_items;
