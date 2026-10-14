-- +goose Up

-- Receipts, supplier bills and scans attached to a ledger entry (photos taken
-- on a phone, PDFs, scanned pages). One entry can carry several files, so this
-- is deliberately NOT the `documents` table: that one keeps a single current
-- version per owner (documents_owner_current_idx).
--
--   storage_key   object key in the S3 bucket (never sent to clients)
--   mime_type     detected from the file's own bytes, never from the client
--   checksum      sha256 of the stored bytes, so an archived original can be
--                 shown to be unaltered
--
-- ON DELETE RESTRICT: ledger entries are soft-deleted, and evidence must
-- never disappear with them.
CREATE TABLE finance_entry_attachments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_id        UUID NOT NULL REFERENCES finance_entries(id) ON DELETE RESTRICT,
    storage_key     TEXT NOT NULL,
    filename        TEXT NOT NULL CHECK (filename ~ '[^[:space:]]'),
    mime_type       TEXT NOT NULL
        CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp', 'application/pdf')),
    size_bytes      BIGINT NOT NULL CHECK (size_bytes > 0),
    checksum_sha256 TEXT NOT NULL CHECK (checksum_sha256 ~ '^[0-9a-f]{64}$'),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX finance_entry_attachments_entry_idx
    ON finance_entry_attachments (entry_id, created_at, id);

-- +goose Down
DROP TABLE IF EXISTS finance_entry_attachments;
