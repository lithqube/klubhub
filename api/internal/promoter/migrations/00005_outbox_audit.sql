-- +goose Up
-- Transactional outbox and append-only audit log (plan §13.6, §13.7).
--
-- Domain code writes an outbox row in the same transaction as its change;
-- the relay publishes it to NATS JetStream with Nats-Msg-Id = id. Payloads
-- carry identifiers only (events.Event), never personal data.
--
-- The relay reads across tenants, so it connects as klubhub_relay: BYPASSRLS
-- but granted nothing except SELECT/UPDATE on outbox. The app role cannot
-- publish across tenants and the relay cannot read anything else.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'klubhub_relay') THEN
        CREATE ROLE klubhub_relay NOLOGIN NOSUPERUSER BYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$$;
-- +goose StatementEnd
GRANT USAGE ON SCHEMA public TO klubhub_relay;

CREATE TABLE outbox (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    subject       TEXT NOT NULL CHECK (subject ~ '^(tenant\.[0-9a-f-]{36}|audit)\.[a-z_]+(\.[a-z_]+)+$'),
    payload       JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ,
    attempts      INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT
);
CREATE INDEX outbox_pending ON outbox (created_at) WHERE published_at IS NULL;
ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON outbox
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
-- The app only appends; it never edits or deletes outbox rows.
GRANT INSERT ON outbox TO klubhub_app;
REVOKE SELECT, UPDATE, DELETE ON outbox FROM klubhub_app;
GRANT SELECT, UPDATE (published_at, attempts, last_error) ON outbox TO klubhub_relay;
COMMENT ON TABLE outbox IS 'data_class: internal';

-- +goose StatementBegin
CREATE FUNCTION outbox_notify() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('promoter_outbox', '');
    RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER outbox_notify AFTER INSERT ON outbox
    FOR EACH STATEMENT EXECUTE FUNCTION outbox_notify();

-- Append-only audit log. The app role can insert and read its own tenant's
-- entries but never change or delete them.
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_type  TEXT NOT NULL CHECK (actor_type IN ('user', 'device', 'system')),
    actor_id    TEXT NOT NULL,
    action      TEXT NOT NULL,
    resource    TEXT NOT NULL,
    decision    TEXT NOT NULL CHECK (decision IN ('allow', 'deny')),
    reason      TEXT
);
CREATE INDEX audit_log_recent ON audit_log (tenant_id, occurred_at DESC);
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_log
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT ON audit_log TO klubhub_app;
REVOKE UPDATE, DELETE ON audit_log FROM klubhub_app;
COMMENT ON TABLE audit_log IS 'data_class: internal';
COMMENT ON COLUMN audit_log.actor_id IS 'data_class: internal — pseudonymous subject id (local:<uuid>, device:<uuid>, zitadel:<id>)';

-- +goose Down
DROP TABLE IF EXISTS audit_log;
DROP TRIGGER IF EXISTS outbox_notify ON outbox;
DROP FUNCTION IF EXISTS outbox_notify();
DROP TABLE IF EXISTS outbox;
