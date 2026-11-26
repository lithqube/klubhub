-- +goose Up
-- P3.2-8b: campaign click tracking.
--
-- marketing_campaign_links: the http(s) links of a sent campaign, numbered in
-- body order. Authored by the organisation, so not recipient PII. The click
-- redirect only ever targets a row here, never a URL from the request.
--
-- marketing_clicks: one row per click on a wrapped link. send_id names the
-- recipient, so the table is personal; deleting the contact deletes its
-- sends and, through them, its clicks (erasure).

CREATE TABLE marketing_campaign_links (
    tenant_id    UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    campaign_id  UUID NOT NULL REFERENCES marketing_campaigns (id) ON DELETE CASCADE,
    position     INT NOT NULL CHECK (position >= 0),
    url          TEXT NOT NULL,
    PRIMARY KEY (campaign_id, position)
);
ALTER TABLE marketing_campaign_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketing_campaign_links FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON marketing_campaign_links
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON marketing_campaign_links TO klubhub_app;
COMMENT ON TABLE marketing_campaign_links IS 'data_class: internal';

CREATE TABLE marketing_clicks (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    campaign_id  UUID NOT NULL REFERENCES marketing_campaigns (id) ON DELETE CASCADE,
    position     INT NOT NULL,
    send_id      UUID NOT NULL REFERENCES marketing_sends (id) ON DELETE CASCADE,
    clicked_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX marketing_clicks_campaign_idx ON marketing_clicks (tenant_id, campaign_id);
CREATE INDEX marketing_clicks_send_idx ON marketing_clicks (send_id);
ALTER TABLE marketing_clicks ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketing_clicks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON marketing_clicks
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON marketing_clicks TO klubhub_app;
COMMENT ON TABLE marketing_clicks IS 'data_class: personal';

-- +goose Down
DROP TABLE IF EXISTS marketing_clicks;
DROP TABLE IF EXISTS marketing_campaign_links;
