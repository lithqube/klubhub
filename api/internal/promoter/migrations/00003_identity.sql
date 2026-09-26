-- +goose Up
-- Local identity for self-hosted Promoter (plan D6, §13.1). SaaS uses
-- Zitadel; these tables stay empty there except sessions for door devices.

-- The one pre-tenant lookup: the single organisation of a self-hosted
-- instance. Returns NULL unless exactly one exists. SECURITY DEFINER runs as
-- the schema owner (superuser or BYPASSRLS) and returns an id only.
-- +goose StatementBegin
CREATE FUNCTION promoter_instance_tenant() RETURNS uuid
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path = pg_catalog, public
AS $$
    SELECT CASE WHEN count(*) = 1 THEN (array_agg(id))[1] END FROM organizations
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION promoter_instance_tenant() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION promoter_instance_tenant() TO klubhub_app;

CREATE TABLE users (
    id                 UUID PRIMARY KEY,
    tenant_id          UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email_enc          BYTEA NOT NULL,
    email_bidx         BYTEA NOT NULL,
    display_name_enc   BYTEA NOT NULL,
    password_hash_enc  BYTEA,
    totp_secret_enc    BYTEA,
    totp_enabled_at    TIMESTAMPTZ,
    totp_last_step     BIGINT NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    failed_logins      INTEGER NOT NULL DEFAULT 0,
    locked_until       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email_bidx)
);
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON users
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON users TO klubhub_app;
COMMENT ON TABLE users IS 'data_class: personal';
COMMENT ON COLUMN users.totp_last_step IS 'data_class: internal — replay counter, not identifying';
COMMENT ON COLUMN users.status IS 'data_class: internal — account state';
COMMENT ON COLUMN users.failed_logins IS 'data_class: internal — lockout counter';
COMMENT ON COLUMN users.locked_until IS 'data_class: internal — lockout expiry';

CREATE TABLE memberships (
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'booker', 'finance', 'marketing', 'door')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, role)
);
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON memberships TO klubhub_app;
COMMENT ON TABLE memberships IS 'data_class: internal';

-- Server-side sessions. Only the SHA-256 of the random token is stored.
-- User sessions have user_id; door sessions have device_id + event_scope.
CREATE TABLE sessions (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    token_hash    BYTEA NOT NULL UNIQUE,
    user_id       UUID REFERENCES users (id) ON DELETE CASCADE,
    device_id     UUID,
    event_scope   UUID,
    amr           TEXT[] NOT NULL,
    auth_time     TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ,
    CHECK ((user_id IS NOT NULL) <> (device_id IS NOT NULL))
);
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sessions
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions TO klubhub_app;
COMMENT ON TABLE sessions IS 'data_class: internal';

-- One-time links: bootstrap (first owner), invite, password reset.
CREATE TABLE setup_links (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    token_hash   BYTEA NOT NULL UNIQUE,
    user_id      UUID REFERENCES users (id) ON DELETE CASCADE,
    email_enc    BYTEA NOT NULL,
    email_bidx   BYTEA NOT NULL,
    role         TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'booker', 'finance', 'marketing', 'door')),
    purpose      TEXT NOT NULL CHECK (purpose IN ('bootstrap', 'invite', 'reset')),
    created_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE setup_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE setup_links FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON setup_links
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON setup_links TO klubhub_app;
COMMENT ON TABLE setup_links IS 'data_class: personal';
COMMENT ON COLUMN setup_links.role IS 'data_class: internal — role granted by the link';
COMMENT ON COLUMN setup_links.purpose IS 'data_class: internal — link type';
COMMENT ON COLUMN setup_links.token_hash IS 'data_class: internal — SHA-256 of a 256-bit random token';
COMMENT ON COLUMN setup_links.created_by IS 'data_class: internal — user id reference';

-- +goose Down
DROP TABLE IF EXISTS setup_links;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS users;
DROP FUNCTION IF EXISTS promoter_instance_tenant();
