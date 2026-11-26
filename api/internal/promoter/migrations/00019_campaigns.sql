-- +goose Up
-- P3.2-8: campaigns.
--
-- marketing_campaigns: the org's own subject/body aimed at one audience
-- segment. Authored by the organisation, so not recipient PII; the per-contact
-- copy that is actually sent is sealed in marketing_sends.

CREATE TABLE marketing_campaigns (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    segment_id  UUID NOT NULL REFERENCES audience_segments (id) ON DELETE RESTRICT,
    subject     TEXT NOT NULL CHECK (length(subject) BETWEEN 1 AND 200),
    body        TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 200000),
    status      TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'sent')),
    sent_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE marketing_campaigns ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketing_campaigns FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON marketing_campaigns
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON marketing_campaigns TO klubhub_app;
COMMENT ON TABLE marketing_campaigns IS 'data_class: internal';

-- +goose Down
DROP TABLE IF EXISTS marketing_campaigns;
