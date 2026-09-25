-- +goose Up
-- Email outbox: records every email sent by the system (invoices, agreements).
-- Separated from application logs so admins can audit / retry delivery.
-- Templated rendering + recipient list; SMTP credentials live outside this table.
CREATE TABLE IF NOT EXISTS email_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Purpose / category, e.g. 'invoice_issued', 'agreement_sent', 'agreement_signed'
    kind            TEXT NOT NULL,
    -- Optional reference back to the originating entity
    owner_type      TEXT,
    owner_id        UUID,
    -- Sender (FROM)
    from_email      TEXT NOT NULL,
    from_name       TEXT NOT NULL DEFAULT '',
    -- Primary recipient
    to_email        TEXT NOT NULL,
    -- Optional CC list as text[]
    cc_emails       TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    -- Optional BCC list
    bcc_emails      TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    -- Subject (rendered)
    subject         TEXT NOT NULL,
    -- Body (rendered, can be plain or HTML)
    body            TEXT NOT NULL,
    -- HTML body variant for templates that provide one
    body_html       TEXT,
    -- Attachments keyed by document id (FK to documents.id)
    attachment_ids  UUID[] NOT NULL DEFAULT ARRAY[]::UUID[],
    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'bounced')),
    -- Delivery metadata
    sent_at         TIMESTAMPTZ,
    -- Last error message if status=failed
    last_error      TEXT,
    -- Number of delivery attempts
    attempts        INT NOT NULL DEFAULT 0,
    -- When the row was enqueued
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- When the row was last updated
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS email_messages_kind_idx ON email_messages (kind);
CREATE INDEX IF NOT EXISTS email_messages_status_idx ON email_messages (status);
CREATE INDEX IF NOT EXISTS email_messages_owner_idx ON email_messages (owner_type, owner_id);

-- +goose Down
DROP TABLE IF EXISTS email_messages;