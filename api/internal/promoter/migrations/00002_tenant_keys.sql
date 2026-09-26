-- +goose Up
-- Wrapped per-tenant data-encryption keys (plan §13.4, envelope tier).
-- wrapped_dek is AES-256-GCM ciphertext under the deployment KEK named by
-- kek_id; the KEK itself never enters the database. Deleting a tenant's rows
-- crypto-shreds every `personal` / `financial` value sealed with them.
CREATE TABLE tenant_keys (
    tenant_id    UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    key_version  INTEGER NOT NULL CHECK (key_version > 0),
    kek_id       TEXT NOT NULL,
    wrapped_dek  BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at   TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, key_version)
);

-- At most one active (unretired) key per tenant.
CREATE UNIQUE INDEX tenant_keys_one_active ON tenant_keys (tenant_id) WHERE retired_at IS NULL;

ALTER TABLE tenant_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_keys
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_keys TO klubhub_app;

COMMENT ON TABLE tenant_keys IS 'data_class: internal';

-- +goose Down
DROP TABLE IF EXISTS tenant_keys;
