-- +goose Up
-- P2.5: privacy and retention (.claude/plans/promoter-p2-guests-door.plan.md,
-- "P2.5 contract"). After an event ends plus the organisation's retention
-- period, the retention job anonymises the event in place: every personal
-- column and blind index of its guests, orders and ticket positions becomes
-- NULL and the row gets purged_at. Rows are kept, so the post-event report
-- (counts, curve, by list / submitter) keeps working without names.

-- Retention period per organisation. A missing row means 30 days.
CREATE TABLE org_privacy (
    tenant_id       UUID NOT NULL PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    retention_days  SMALLINT NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 365),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      TEXT NOT NULL DEFAULT ''
);
ALTER TABLE org_privacy ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_privacy FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON org_privacy
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE ON org_privacy TO klubhub_app;
COMMENT ON TABLE org_privacy IS 'data_class: internal';
COMMENT ON COLUMN org_privacy.updated_by IS 'data_class: internal — pseudonymous staff subject id (local:<uuid>), as in audit_log.actor_id';

-- One row per event: when it is due (ends_at + retention, recomputed while
-- not purged) and, once purged, when, how and how many rows per table.
CREATE TABLE event_purges (
    tenant_id    UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id     UUID NOT NULL PRIMARY KEY REFERENCES events (id) ON DELETE CASCADE,
    purge_after  TIMESTAMPTZ NOT NULL,
    purged_at    TIMESTAMPTZ,
    counts       JSONB,
    trigger      TEXT CHECK (trigger IN ('schedule', 'manual')),
    CHECK ((purged_at IS NULL) = (trigger IS NULL)),
    CHECK ((purged_at IS NULL) = (counts IS NULL))
);
CREATE INDEX event_purges_due ON event_purges (tenant_id, purge_after) WHERE purged_at IS NULL;
CREATE INDEX event_purges_recent ON event_purges (tenant_id, purged_at DESC) WHERE purged_at IS NOT NULL;
ALTER TABLE event_purges ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_purges FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON event_purges
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE ON event_purges TO klubhub_app;
COMMENT ON TABLE event_purges IS 'data_class: internal';
COMMENT ON COLUMN event_purges.counts IS 'data_class: internal — rows anonymised per table, counts only';
COMMENT ON COLUMN event_purges.trigger IS 'data_class: internal — schedule (retention job) or manual (erase now)';

-- Purged rows: personal columns may be NULL only once purged_at is set, and
-- a purged row can never hold personal data again.
ALTER TABLE guests ADD COLUMN purged_at TIMESTAMPTZ;
ALTER TABLE guests ALTER COLUMN name_enc DROP NOT NULL;
ALTER TABLE guests ALTER COLUMN name_bidx DROP NOT NULL;
ALTER TABLE guests ADD CONSTRAINT guests_personal_until_purged
    CHECK (purged_at IS NOT NULL OR (name_enc IS NOT NULL AND name_bidx IS NOT NULL));
ALTER TABLE guests ADD CONSTRAINT guests_purged_is_anonymous
    CHECK (purged_at IS NULL OR (name_enc IS NULL AND email_enc IS NULL AND phone_enc IS NULL AND note_enc IS NULL
                                 AND name_bidx IS NULL AND email_bidx IS NULL));

ALTER TABLE orders ADD COLUMN purged_at TIMESTAMPTZ;
ALTER TABLE orders ADD CONSTRAINT orders_purged_is_anonymous
    CHECK (purged_at IS NULL OR (buyer_name_enc IS NULL AND buyer_email_enc IS NULL AND buyer_email_bidx IS NULL));

ALTER TABLE order_positions ADD COLUMN purged_at TIMESTAMPTZ;
ALTER TABLE order_positions ALTER COLUMN attendee_name_enc DROP NOT NULL;
ALTER TABLE order_positions ALTER COLUMN attendee_name_bidx DROP NOT NULL;
ALTER TABLE order_positions ADD CONSTRAINT order_positions_personal_until_purged
    CHECK (purged_at IS NOT NULL OR (attendee_name_enc IS NOT NULL AND attendee_name_bidx IS NOT NULL));
ALTER TABLE order_positions ADD CONSTRAINT order_positions_purged_is_anonymous
    CHECK (purged_at IS NULL OR (attendee_name_enc IS NULL AND attendee_email_enc IS NULL AND secret_enc IS NULL
                                 AND attendee_name_bidx IS NULL AND attendee_email_bidx IS NULL AND secret_bidx IS NULL));

-- The retention job runs across tenants without bypassing row level
-- security: it asks for tenant ids only, through this narrow SECURITY
-- DEFINER function (as promoter_instance_tenant() does), and then works on
-- each tenant inside tenantdb.WithTenant.
-- +goose StatementBegin
CREATE FUNCTION promoter_tenant_ids() RETURNS SETOF uuid
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path = pg_catalog, public
AS $$
    SELECT id FROM organizations ORDER BY id
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION promoter_tenant_ids() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION promoter_tenant_ids() TO klubhub_app;

-- +goose Down
-- Restoring NOT NULL fails while purged rows exist: purged data cannot come
-- back, and deleting the rows would silently change past reports.
DROP FUNCTION IF EXISTS promoter_tenant_ids();
ALTER TABLE order_positions DROP CONSTRAINT order_positions_purged_is_anonymous;
ALTER TABLE order_positions DROP CONSTRAINT order_positions_personal_until_purged;
ALTER TABLE order_positions ALTER COLUMN attendee_name_bidx SET NOT NULL;
ALTER TABLE order_positions ALTER COLUMN attendee_name_enc SET NOT NULL;
ALTER TABLE order_positions DROP COLUMN purged_at;
ALTER TABLE orders DROP CONSTRAINT orders_purged_is_anonymous;
ALTER TABLE orders DROP COLUMN purged_at;
ALTER TABLE guests DROP CONSTRAINT guests_purged_is_anonymous;
ALTER TABLE guests DROP CONSTRAINT guests_personal_until_purged;
ALTER TABLE guests ALTER COLUMN name_bidx SET NOT NULL;
ALTER TABLE guests ALTER COLUMN name_enc SET NOT NULL;
ALTER TABLE guests DROP COLUMN purged_at;
DROP TABLE IF EXISTS event_purges;
DROP TABLE IF EXISTS org_privacy;
