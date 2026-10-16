-- +goose Up
-- P4 bookings outreach (phase 04.10): the tenant-scoped email outbox.
--
-- NOT a column-for-column copy of platform 017_email.sql (SPEC §6.1 v2.1):
-- on the promoter side to_email is a talent contact's email and the rendered
-- subject/body carry their name, so the PII columns are sealed with the
-- tenant DEK (BYTEA, Field{Table:"email_messages",…}) with a blind index for
-- exact lookup, and there is no cc/bcc (outreach never uses them). The
-- shared platform/email repository serves both sides: the tenant-scoped
-- variant seals on write and unseals for delivery/log through a FieldCodec
-- (identity on the DJ side, envelope-backed here).

CREATE TABLE email_messages (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    owner_type      TEXT NOT NULL DEFAULT '',
    owner_id        UUID,
    from_email      TEXT NOT NULL,
    from_name       TEXT NOT NULL DEFAULT '',
    to_email_enc    BYTEA NOT NULL,
    to_email_bidx   BYTEA,
    subject_enc     BYTEA NOT NULL,
    body_enc        BYTEA NOT NULL,
    body_html_enc   BYTEA,
    attachment_ids  UUID[] NOT NULL DEFAULT ARRAY[]::UUID[],
    reply_to_enc    BYTEA,
    status          TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'bounced')),
    sent_at         TIMESTAMPTZ,
    last_error      TEXT NOT NULL DEFAULT '',
    attempts        INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_messages_kind_idx ON email_messages (kind);
CREATE INDEX email_messages_status_idx ON email_messages (status);
CREATE INDEX email_messages_owner_idx ON email_messages (owner_type, owner_id);
CREATE INDEX email_messages_tenant_idx ON email_messages (tenant_id);
ALTER TABLE email_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE email_messages FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email_messages
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON email_messages TO klubhub_app;
COMMENT ON TABLE email_messages IS 'data_class: personal';
COMMENT ON COLUMN email_messages.kind IS 'data_class: internal — the message category (booking_offer), not identifying';
COMMENT ON COLUMN email_messages.owner_type IS 'data_class: internal — the entity the message belongs to (booking_offer), not identifying';
COMMENT ON COLUMN email_messages.from_email IS 'data_class: internal — the instance sender address (PLUNK_FROM_EMAIL), not recipient PII';
COMMENT ON COLUMN email_messages.from_name IS 'data_class: internal — the instance sender display name, not recipient PII';
COMMENT ON COLUMN email_messages.attachment_ids IS 'data_class: internal — document ids only; outreach v1 sends no attachments';
COMMENT ON COLUMN email_messages.status IS 'data_class: internal — delivery workflow state';
COMMENT ON COLUMN email_messages.last_error IS 'data_class: internal — delivery error text, no recipient data';
COMMENT ON COLUMN email_messages.attempts IS 'data_class: internal — delivery attempt counter';

-- +goose Down
DROP TABLE IF EXISTS email_messages;
