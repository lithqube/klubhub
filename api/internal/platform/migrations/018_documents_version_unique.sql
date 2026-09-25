-- +goose Up
-- Guard against a race where two concurrent uploads for the same owner both
-- read the same "current max version" and insert the same version number.
-- The version is now computed inside the insert statement (see
-- DocumentRepository.CreateWithDetails), and this unique index turns any
-- remaining race into a Postgres unique-violation (23505) that the
-- application maps to a conflict error instead of silently duplicating rows.
CREATE UNIQUE INDEX IF NOT EXISTS documents_owner_version_idx
    ON documents (owner_type, owner_id, version);

-- +goose Down
DROP INDEX IF EXISTS documents_owner_version_idx;
