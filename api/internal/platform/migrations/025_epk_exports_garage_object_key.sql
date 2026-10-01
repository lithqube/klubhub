-- +goose Up
-- Add the canonical garage_object_key column alongside the legacy
-- minio_path so the wire contract can flip in this release without
-- losing any existing object references. A one-shot backfill keeps
-- both columns populated from the legacy data, then the Go
-- repository/handler write exclusively to garage_object_key going
-- forward. The legacy column is dropped in a follow-up release once
-- v1 consumers have rolled out.
--
-- Mirrors the pattern used in migration 024 for
-- scheduled_posts.image_storage_key.
ALTER TABLE epk_exports
    ADD COLUMN garage_object_key TEXT NOT NULL DEFAULT '';

UPDATE epk_exports
SET garage_object_key = minio_path
WHERE garage_object_key = ''
  AND minio_path <> '';

-- +goose Down
ALTER TABLE epk_exports
    DROP COLUMN IF EXISTS garage_object_key;
