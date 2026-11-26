-- +goose Up

-- Releases: a DJ's planned or shipped releases (Phase 6, REL-01/REL-02).
--
-- The status is the workflow REL-02 describes: demo_sent -> accepted ->
-- mastering -> artwork -> promo -> released. It is an enum so an illegal
-- status can never be stored; the allowed moves are enforced in the service.
CREATE TYPE release_status AS ENUM ('demo_sent', 'accepted', 'mastering', 'artwork', 'promo', 'released');
CREATE TYPE release_format AS ENUM ('digital', 'vinyl', 'both');

CREATE TABLE IF NOT EXISTS releases (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title               TEXT NOT NULL,
    artists             TEXT NOT NULL DEFAULT '',   -- comma-separated, as entered
    label               TEXT NOT NULL DEFAULT '',
    distributor         TEXT NOT NULL DEFAULT '',
    release_date        DATE NOT NULL,
    format              release_format NOT NULL DEFAULT 'digital',
    notes               TEXT NOT NULL DEFAULT '',
    status              release_status NOT NULL DEFAULT 'demo_sent',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ                 -- soft delete
);

CREATE INDEX IF NOT EXISTS releases_release_date_idx ON releases(release_date);
CREATE INDEX IF NOT EXISTS releases_status_idx ON releases(status);
CREATE INDEX IF NOT EXISTS releases_deleted_at_idx ON releases(deleted_at) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS releases;
DROP TYPE IF EXISTS release_format;
DROP TYPE IF EXISTS release_status;
