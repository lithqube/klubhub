-- +goose Up
CREATE TABLE tracklists (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title         TEXT NOT NULL,
    source_format TEXT NOT NULL DEFAULT 'rekordbox',
    raw_file_path TEXT,
    preset        TEXT NOT NULL DEFAULT 'default',
    visible_fields JSONB NOT NULL DEFAULT '["title","artist","bpm","key","time_played"]',
    bg_mode       TEXT NOT NULL DEFAULT 'solid',
    bg_value      TEXT,
    max_tracks    INT NOT NULL DEFAULT 30,
    track_range_start INT,
    track_range_end   INT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE TABLE tracks (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tracklist_id   UUID NOT NULL REFERENCES tracklists(id),
    position       INT NOT NULL,
    title          TEXT NOT NULL,
    artist         TEXT,
    album          TEXT,
    genre          TEXT,
    bpm            NUMERIC(6,2),
    rating         INT,
    duration_secs  INT,
    musical_key    TEXT,
    date_added     DATE,
    artwork_status TEXT NOT NULL DEFAULT 'pending',
    artwork_url    TEXT,
    artwork_source TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ,
    UNIQUE (tracklist_id, position)
);

-- TODO (Phase 4): When the gigs table is added, the SoftDelete method in
-- repository.go must also run:
--   UPDATE gigs SET tracklist_id = NULL WHERE tracklist_id = $1
-- to satisfy TRKL-27 (gig references set to NULL on tracklist delete).
-- This cannot be done in Phase 1 because the gigs table does not yet exist.

-- +goose Down
DROP TABLE IF EXISTS tracks;
DROP TABLE IF EXISTS tracklists;