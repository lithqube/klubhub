-- +goose Up

-- Platform links per release (Phase 6, REL-04): where the release is live.
-- Manual entry — the app does not talk to the stores, the DJ types the URL.
CREATE TABLE IF NOT EXISTS release_platform_links (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id          UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    platform            TEXT NOT NULL CHECK (platform IN ('beatport', 'bandcamp', 'spotify', 'soundcloud')),
    url                 TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (release_id, platform)
);

CREATE INDEX IF NOT EXISTS release_platform_links_release_idx ON release_platform_links(release_id);

-- +goose Down
DROP TABLE IF EXISTS release_platform_links;
