-- +goose Up
-- KlubHub Promoter foundation: runtime role and the tenant (organization) row.
--
-- Tenant isolation (plan §13.3, pattern from bridge ADR-0006, reimplemented):
--   * the app connects as klubhub_app: not the owner, no BYPASSRLS, never
--     a superuser (the binary refuses to start otherwise);
--   * every table has ENABLE + FORCE row level security and one policy on
--     current_setting('app.tenant_id')::uuid. Without a tenant the setting is
--     missing or '' and the cast raises, so queries fail closed;
--   * the tenant is set per transaction by tenantdb.WithTenant only.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'klubhub_app') THEN
        -- LOGIN and the password are granted by the setup script from a secret,
        -- never from a migration.
        CREATE ROLE klubhub_app NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO klubhub_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO klubhub_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO klubhub_app;

-- One row per collective / organisation. Its id IS the tenant id: the same
-- UUID is the Zitadel organization (SaaS), the RLS key and the NATS subject
-- token (tenant.{id}.*).
CREATE TABLE organizations (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    slug        TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    timezone    TEXT NOT NULL DEFAULT 'UTC',
    currency    CHAR(3) NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON organizations
    USING (id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (id = current_setting('app.tenant_id')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON organizations TO klubhub_app;

COMMENT ON TABLE organizations IS 'data_class: internal';
COMMENT ON COLUMN organizations.name IS 'data_class: public — the collective''s public name, shown on its pages';

-- +goose Down
DROP TABLE IF EXISTS organizations;
