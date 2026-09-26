-- +goose Up
-- Door access (plan §13.1): registered devices plus a per-event PIN yield a
-- short-lived, event-scoped `door` session. An unregistered device cannot
-- try PINs at all; PIN attempts lock out per event.
CREATE TABLE door_devices (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    label         TEXT NOT NULL CHECK (length(label) BETWEEN 1 AND 80),
    token_hash    BYTEA NOT NULL UNIQUE,
    created_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ
);
ALTER TABLE door_devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE door_devices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON door_devices
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON door_devices TO klubhub_app;
COMMENT ON TABLE door_devices IS 'data_class: internal';

-- event_id gets its foreign key when the events table lands (P1).
CREATE TABLE door_pins (
    tenant_id        UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id         UUID NOT NULL,
    pin_hash_enc     BYTEA NOT NULL,
    failed_attempts  INTEGER NOT NULL DEFAULT 0,
    locked_until     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, event_id)
);
ALTER TABLE door_pins ENABLE ROW LEVEL SECURITY;
ALTER TABLE door_pins FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON door_pins
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON door_pins TO klubhub_app;
COMMENT ON TABLE door_pins IS 'data_class: internal';

-- +goose Down
DROP TABLE IF EXISTS door_pins;
DROP TABLE IF EXISTS door_devices;
