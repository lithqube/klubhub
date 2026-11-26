-- +goose Up
-- P3.2 email platform: the marketing stream (campaign mail to audience
-- contacts), kept apart from the transactional email_messages outbox.
--
-- marketing_provider_configs: one delivery backend per tenant. Credentials
-- and webhook secret live sealed in settings_enc (tenant DEK).
--
-- marketing_sends: the throttled send queue. Recipient and rendered content
-- carry contact PII, so they are sealed with a blind index for lookup.
-- Deleting a contact deletes its queued and sent mail (erasure).
--
-- marketing_suppressions: no-send list keyed by the email blind index only;
-- the address itself is never stored (as outreach_suppressions).

CREATE TABLE marketing_provider_configs (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL UNIQUE REFERENCES organizations (id) ON DELETE CASCADE,
    provider      TEXT NOT NULL CHECK (provider IN ('smtp', 'plunk')),
    from_email    TEXT NOT NULL,
    from_name     TEXT NOT NULL DEFAULT '',
    settings_enc  BYTEA NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE marketing_provider_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketing_provider_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON marketing_provider_configs
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON marketing_provider_configs TO klubhub_app;
COMMENT ON TABLE marketing_provider_configs IS 'data_class: internal';
COMMENT ON COLUMN marketing_provider_configs.from_email IS 'data_class: internal — the organisation sender address, not recipient PII';
COMMENT ON COLUMN marketing_provider_configs.from_name IS 'data_class: internal — the organisation sender display name, not recipient PII';

CREATE TABLE marketing_sends (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    contact_id           UUID REFERENCES audience_contacts (id) ON DELETE CASCADE,
    to_email_enc         BYTEA NOT NULL,
    to_email_bidx        BYTEA NOT NULL,
    subject_enc          BYTEA NOT NULL,
    body_enc             BYTEA NOT NULL,
    body_html_enc        BYTEA,
    status               TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'bounced', 'complained')),
    attempts             INT NOT NULL DEFAULT 0,
    last_error           TEXT NOT NULL DEFAULT '',
    provider_message_id  TEXT NOT NULL DEFAULT '',
    scheduled_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at              TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX marketing_sends_queue_idx ON marketing_sends (status, scheduled_at);
CREATE INDEX marketing_sends_tenant_idx ON marketing_sends (tenant_id, created_at);
CREATE INDEX marketing_sends_contact_idx ON marketing_sends (contact_id);
ALTER TABLE marketing_sends ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketing_sends FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON marketing_sends
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON marketing_sends TO klubhub_app;
COMMENT ON TABLE marketing_sends IS 'data_class: personal';
COMMENT ON COLUMN marketing_sends.status IS 'data_class: internal — delivery workflow state';
COMMENT ON COLUMN marketing_sends.attempts IS 'data_class: internal — delivery attempt counter';
COMMENT ON COLUMN marketing_sends.last_error IS 'data_class: internal — delivery error text, no recipient data';
COMMENT ON COLUMN marketing_sends.provider_message_id IS 'data_class: internal — the provider''s opaque message id';

CREATE TABLE marketing_suppressions (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email_bidx  BYTEA NOT NULL,
    reason      TEXT NOT NULL CHECK (reason IN ('bounce', 'complaint', 'unsubscribe', 'manual')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email_bidx)
);
ALTER TABLE marketing_suppressions ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketing_suppressions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON marketing_suppressions
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON marketing_suppressions TO klubhub_app;
COMMENT ON TABLE marketing_suppressions IS 'data_class: personal';
COMMENT ON COLUMN marketing_suppressions.reason IS 'data_class: internal — why the address is suppressed; the email itself is never stored (blind index only)';

-- +goose Down
DROP TABLE IF EXISTS marketing_suppressions;
DROP TABLE IF EXISTS marketing_sends;
DROP TABLE IF EXISTS marketing_provider_configs;
