-- +goose Up

-- Per-track markers shown on the exported tracklist card.
--   hidden_gem  the card conceals this track's title and artist (a "hidden gem"
--               the DJ does not want to give away), keeping its position.
--   unreleased  the card tags the track as unreleased (dubplate, ID, promo).
--   media       what the DJ played it from: vinyl or digital.
--               '' means not set.
-- Existing rows keep false / false / '' so nothing changes for old tracklists.
-- Numbered 028: 026 and 027 are taken by the rider templates on main.

ALTER TABLE tracks
    ADD COLUMN IF NOT EXISTS hidden_gem BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS unreleased BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS media TEXT NOT NULL DEFAULT ''
        CONSTRAINT tracks_media_check CHECK (media IN ('', 'vinyl', 'digital'));

-- Drag-and-drop reordering renumbers a tracklist's positions in one
-- transaction, which a plain UNIQUE (tracklist_id, position) rejects halfway
-- through (rows swap places). Make it deferrable so the check runs at commit.
-- Soft-deleted rows keep their position and count towards the constraint.
ALTER TABLE tracks DROP CONSTRAINT IF EXISTS tracks_tracklist_id_position_key;
ALTER TABLE tracks DROP CONSTRAINT IF EXISTS tracks_tracklist_position_key;
ALTER TABLE tracks
    ADD CONSTRAINT tracks_tracklist_position_key
    UNIQUE (tracklist_id, position) DEFERRABLE INITIALLY IMMEDIATE;

-- +goose Down

ALTER TABLE tracks DROP CONSTRAINT IF EXISTS tracks_tracklist_position_key;
ALTER TABLE tracks
    ADD CONSTRAINT tracks_tracklist_id_position_key UNIQUE (tracklist_id, position);
ALTER TABLE tracks
    DROP COLUMN IF EXISTS media,
    DROP COLUMN IF EXISTS unreleased,
    DROP COLUMN IF EXISTS hidden_gem;
