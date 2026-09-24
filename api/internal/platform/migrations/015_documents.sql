-- +goose Up
-- Documents: immutable, versioned, stored in Garage S3.
-- Each document belongs to an owner entity (invoice, agreement, etc.).
-- Multiple versions are kept; only the latest is "current".
-- The storage backend is Garage S3 (S3-compatible).
CREATE TABLE IF NOT EXISTS documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type      TEXT NOT NULL
        CHECK (owner_type IN ('invoice', 'agreement', 'epk', 'gig', 'other')),
    owner_id        UUID NOT NULL,
    -- Garage S3 object key (prefix + UUID). The actual bytes live in Garage.
    storage_key     TEXT NOT NULL,
    -- Original filename for Content-Disposition.
    filename        TEXT NOT NULL,
    -- MIME type (application/pdf for invoices/agreements).
    mime_type       TEXT NOT NULL,
    -- Size in bytes.
    size_bytes      BIGINT NOT NULL,
    -- SHA-256 of the content for integrity verification.
    checksum_sha256 TEXT NOT NULL,
    -- Version number (1, 2, 3...). Only the max version per (owner_type, owner_id)
    -- is considered "current". Older versions are retained for history.
    version         INT NOT NULL DEFAULT 1,
    -- Uploaded by (user id or system).
    uploaded_by     TEXT NOT NULL DEFAULT 'system',
    -- Created timestamp.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Is this the current (latest) version? Application-maintained.
    is_current      BOOLEAN NOT NULL DEFAULT true
);

-- Unique constraint: one current version per owner.
CREATE UNIQUE INDEX IF NOT EXISTS documents_owner_current_idx
    ON documents (owner_type, owner_id)
    WHERE is_current = true;

-- Fast lookups by owner.
CREATE INDEX IF NOT EXISTS documents_owner_idx ON documents (owner_type, owner_id, version DESC);

-- +goose Down
DROP TABLE IF EXISTS documents;