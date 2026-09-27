-- +goose Up
-- P3.1: audience CRM (.claude/plans/promoter-p3-audience-promotion.plan.md).
-- Contacts are `personal` data (name/email/phone), sealed with the tenant
-- DEK, blind-indexed for exact lookup and CSV-import dedupe. A consent
-- record travels with each contact per plan §Data model and security:
-- timestamp, IP (also sealed — an IP is personal data), the exact form
-- text shown at the time, lawful basis, and double-opt-in confirmation.
-- Segments are internal (a saved filter definition, not a person).

CREATE TABLE audience_contacts (
    id                          UUID PRIMARY KEY,
    tenant_id                   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name_enc                    BYTEA,
    email_enc                   BYTEA,
    phone_enc                   BYTEA,
    name_bidx                   BYTEA,
    email_bidx                  BYTEA,
    status                      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'unsubscribed', 'bounced', 'complained')),
    source                      TEXT NOT NULL CHECK (source IN ('rsvp', 'follow', 'notify_me', 'csv', 'door')),
    consent_basis               TEXT NOT NULL CHECK (consent_basis IN ('consent', 'soft_opt_in')),
    consent_recorded_at         TIMESTAMPTZ NOT NULL,
    consent_ip_enc              BYTEA,
    consent_form_text           TEXT NOT NULL DEFAULT '' CHECK (length(consent_form_text) <= 2000),
    double_opt_in_confirmed_at  TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- A contact needs at least one of email or name to be findable/mergeable.
ALTER TABLE audience_contacts ADD CONSTRAINT audience_contacts_has_identity
    CHECK (email_enc IS NOT NULL OR name_enc IS NOT NULL);
CREATE INDEX audience_contacts_by_status ON audience_contacts (tenant_id, status, created_at);
CREATE INDEX audience_contacts_by_source ON audience_contacts (tenant_id, source);
-- Dedupe on CSV import and exact lookup; a tenant may have only one active
-- (non-purged) contact per email, mirroring the guests table's per-event
-- dedupe intent but scoped to the whole org, since audience is org-wide.
CREATE UNIQUE INDEX audience_contacts_email_bidx ON audience_contacts (tenant_id, email_bidx) WHERE email_bidx IS NOT NULL;
CREATE INDEX audience_contacts_name_bidx ON audience_contacts (tenant_id, name_bidx) WHERE name_bidx IS NOT NULL;
ALTER TABLE audience_contacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE audience_contacts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audience_contacts
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON audience_contacts TO klubhub_app;
COMMENT ON TABLE audience_contacts IS 'data_class: personal';
COMMENT ON COLUMN audience_contacts.status IS 'data_class: internal — subscription workflow state';
COMMENT ON COLUMN audience_contacts.source IS 'data_class: internal — how the contact was acquired';
COMMENT ON COLUMN audience_contacts.consent_basis IS 'data_class: internal — the lawful basis claimed, not identifying on its own';
COMMENT ON COLUMN audience_contacts.consent_recorded_at IS 'data_class: internal — a timestamp, not identifying on its own';
COMMENT ON COLUMN audience_contacts.consent_form_text IS 'data_class: internal — KlubHub''s own copy shown at consent time, identical for every contact on that form';
COMMENT ON COLUMN audience_contacts.double_opt_in_confirmed_at IS 'data_class: internal — a timestamp, not identifying on its own';

CREATE TABLE audience_segments (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    filter      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX audience_segments_by_name ON audience_segments (tenant_id, name);
ALTER TABLE audience_segments ENABLE ROW LEVEL SECURITY;
ALTER TABLE audience_segments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audience_segments
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON audience_segments TO klubhub_app;
COMMENT ON TABLE audience_segments IS 'data_class: internal';
COMMENT ON COLUMN audience_segments.name IS 'data_class: internal — a segment label such as "Berlin, techno", not a person';
COMMENT ON COLUMN audience_segments.filter IS 'data_class: internal — a saved query definition (status/source/date-range/text), never contact PII itself';

-- +goose Down
DROP TABLE IF EXISTS audience_segments;
DROP TABLE IF EXISTS audience_contacts;
