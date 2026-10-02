-- +goose Up
-- Add the canonical image_storage_key column alongside the legacy
-- image_minio_path so the wire contract can flip in this release
-- without losing any existing object references. A one-shot backfill
-- keeps both columns populated from the legacy data, then the Go
-- repository/worker write exclusively to image_storage_key going
-- forward. The legacy column is dropped in a follow-up release once
-- v1 consumers have rolled out.
ALTER TABLE scheduled_posts
    ADD COLUMN image_storage_key TEXT NOT NULL DEFAULT '';

UPDATE scheduled_posts
SET image_storage_key = image_minio_path
WHERE image_storage_key = ''
  AND image_minio_path <> '';

-- +goose Down
ALTER TABLE scheduled_posts
    DROP COLUMN IF EXISTS image_storage_key;