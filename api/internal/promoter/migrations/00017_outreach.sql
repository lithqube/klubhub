-- +goose Up
-- P4 bookings outreach (phase 04.10): promoter→DJ outreach (SPEC §6.1).
--
-- outreach_templates: customizable offer templates, one default per tenant
-- (mirrors the DJ side's 034, scoped to the tenant). The default
-- promoter_to_dj template is seeded for every organisation that exists when
-- this migration runs; organisations created later fall back to the same
-- built-in default in the bookings service until they save their own.
--
-- outreach_requests: the sent log. The recipient AND the rendered
-- subject/body carry talent PII, so all three are sealed with the tenant DEK
-- (SPEC §6.1 v2.1); the service unseals when rendering the log. The log
-- survives talent deletion (talent_contact_id ON DELETE SET NULL).
--
-- outreach_suppressions: do-not-contact list keyed by the email blind index
-- ONLY — the email itself is never stored (SPEC §9).

CREATE TABLE outreach_templates (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    direction   TEXT NOT NULL CHECK (direction IN ('promoter_to_dj')),
    name        TEXT NOT NULL,
    subject     TEXT NOT NULL,
    body        TEXT NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT false,
    version     INTEGER NOT NULL DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX outreach_templates_one_default ON outreach_templates (tenant_id, direction) WHERE is_default AND deleted_at IS NULL;
CREATE UNIQUE INDEX outreach_templates_name ON outreach_templates (tenant_id, lower(name)) WHERE deleted_at IS NULL;
ALTER TABLE outreach_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE outreach_templates FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON outreach_templates
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON outreach_templates TO klubhub_app;
COMMENT ON TABLE outreach_templates IS 'data_class: internal';
COMMENT ON COLUMN outreach_templates.name IS 'data_class: internal — a template label, not a person';

CREATE TABLE outreach_requests (
    id                UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id          UUID REFERENCES events (id) ON DELETE SET NULL,
    open_slot_id      UUID REFERENCES event_open_slots (id) ON DELETE SET NULL,
    talent_contact_id UUID REFERENCES talent_contacts (id) ON DELETE SET NULL,
    template_id       UUID REFERENCES outreach_templates (id) ON DELETE SET NULL,
    email_message_id  UUID REFERENCES email_messages (id) ON DELETE SET NULL,
    to_email_enc      BYTEA NOT NULL,
    to_email_bidx     BYTEA NOT NULL,
    subject_enc       BYTEA NOT NULL,
    body_enc          BYTEA NOT NULL,
    status            TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'bounced')),
    sent_at           TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX outreach_requests_contact_idx ON outreach_requests (tenant_id, talent_contact_id);
CREATE INDEX outreach_requests_created_idx ON outreach_requests (tenant_id, created_at);
ALTER TABLE outreach_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE outreach_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON outreach_requests
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON outreach_requests TO klubhub_app;
COMMENT ON TABLE outreach_requests IS 'data_class: personal';
COMMENT ON COLUMN outreach_requests.status IS 'data_class: internal — delivery workflow state';

CREATE TABLE outreach_suppressions (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email_bidx  BYTEA NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email_bidx)
);
ALTER TABLE outreach_suppressions ENABLE ROW LEVEL SECURITY;
ALTER TABLE outreach_suppressions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON outreach_suppressions
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON outreach_suppressions TO klubhub_app;
COMMENT ON TABLE outreach_suppressions IS 'data_class: personal';
COMMENT ON COLUMN outreach_suppressions.reason IS 'data_class: internal — why the address was suppressed; the email itself is never stored (blind index only)';

-- Seed the default promoter_to_dj template for every existing organisation
-- (SPEC §7: event name/date/city/venue, slot role/genres/fee, org identity,
-- opt-out footer; {{venue}} renders empty for withheld locations, OFFER-09).
INSERT INTO outreach_templates (id, tenant_id, direction, name, subject, body, is_default)
SELECT gen_random_uuid(), o.id, 'promoter_to_dj', 'Default booking offer',
  'Booking offer: {{event_name}} on {{date}}',
  E'Hi {{dj_name}},\n' ||
  E'\n' ||
  E'we''re {{org_name}} and we''d love to have you play {{event_name}}.\n' ||
  E'\n' ||
  E'Event: {{event_name}}\n' ||
  E'Date: {{date}}\n' ||
  E'City: {{city}}\n' ||
  E'Venue: {{venue}}\n' ||
  E'Slot: {{slot_role}}\n' ||
  E'Genres: {{genres}}\n' ||
  E'Fee: {{fee}}\n' ||
  E'{{notes}}\n' ||
  E'\n' ||
  E'If you''re interested, just reply to this email — it lands directly in {{promoter_name}}''s inbox ({{promoter_email}}).\n' ||
  E'\n' ||
  E'If you''d rather not hear from us, reply "stop" and we''ll remove you.\n' ||
  E'\n' ||
  E'{{org_name}}',
  true
FROM organizations o;

-- +goose Down
DROP TABLE IF EXISTS outreach_suppressions;
DROP TABLE IF EXISTS outreach_requests;
DROP TABLE IF EXISTS outreach_templates;
