-- +goose Up
-- P2.2: attendees who bought tickets elsewhere (self-hosted has no checkout,
-- plan D2), imported from a platform export into the pretix-shaped
-- ticket_types → orders → order_positions model
-- (.claude/plans/promoter-p2-guests-door.plan.md). Buyer and attendee
-- names, emails and the ticket secret (the QR/barcode payload) are sealed
-- with the tenant DEK; exact lookups go through HMAC blind indexes
-- (secret_bidx is the door's QR lookup in P2.3). Re-imports are keyed by
-- (event, source, external order ref) and (order, position ref or secret),
-- so they update rows instead of duplicating them.

CREATE TABLE ticket_types (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id      UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    name          TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    name_key      TEXT NOT NULL,
    capacity      INTEGER CHECK (capacity IS NULL OR capacity > 0),
    external_ref  TEXT CHECK (external_ref IS NULL OR length(external_ref) BETWEEN 1 AND 120),
    position      SMALLINT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, event_id, name_key)
);
CREATE INDEX ticket_types_by_ref ON ticket_types (tenant_id, event_id, external_ref) WHERE external_ref IS NOT NULL;
ALTER TABLE ticket_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE ticket_types FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ticket_types
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON ticket_types TO klubhub_app;
COMMENT ON TABLE ticket_types IS 'data_class: internal';
COMMENT ON COLUMN ticket_types.name IS 'data_class: internal — a ticket tier label such as "Early bird", not a person';
COMMENT ON COLUMN ticket_types.name_key IS 'data_class: internal — the tier label folded for matching on re-import';

CREATE TABLE attendee_imports (
    id                  UUID PRIMARY KEY,
    tenant_id           UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id            UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    source              TEXT NOT NULL CHECK (source IN ('ra', 'dice', 'shotgun', 'pretix', 'luma', 'generic')),
    rows_total          INTEGER NOT NULL,
    orders_new          INTEGER NOT NULL,
    orders_updated      INTEGER NOT NULL,
    positions_new       INTEGER NOT NULL,
    positions_updated   INTEGER NOT NULL,
    positions_unchanged INTEGER NOT NULL,
    rejected            INTEGER NOT NULL,
    created_by          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX attendee_imports_by_event ON attendee_imports (tenant_id, event_id, created_at);
ALTER TABLE attendee_imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendee_imports FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON attendee_imports
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT ON attendee_imports TO klubhub_app;
COMMENT ON TABLE attendee_imports IS 'data_class: internal';

CREATE TABLE orders (
    id                UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id          UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    source            TEXT NOT NULL CHECK (source IN ('ra', 'dice', 'shotgun', 'pretix', 'luma', 'generic')),
    external_ref      TEXT NOT NULL CHECK (length(external_ref) BETWEEN 1 AND 200),
    buyer_name_enc    BYTEA,
    buyer_email_enc   BYTEA,
    buyer_email_bidx  BYTEA,
    import_id         UUID REFERENCES attendee_imports (id) ON DELETE SET NULL,
    imported_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, event_id, source, external_ref)
);
CREATE INDEX orders_buyer_email_bidx ON orders (tenant_id, event_id, buyer_email_bidx) WHERE buyer_email_bidx IS NOT NULL;
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON orders
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON orders TO klubhub_app;
COMMENT ON TABLE orders IS 'data_class: personal';
COMMENT ON COLUMN orders.source IS 'data_class: internal — the ticketing platform the order came from (ra, dice, …)';
COMMENT ON COLUMN orders.external_ref IS 'data_class: internal — the platform''s order number, the idempotent re-import key; identifies no one without the platform''s own records';

CREATE TABLE order_positions (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id             UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    order_id             UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    ticket_type_id       UUID NOT NULL REFERENCES ticket_types (id) ON DELETE RESTRICT,
    external_ref         TEXT NOT NULL CHECK (length(external_ref) BETWEEN 1 AND 200),
    attendee_name_enc    BYTEA NOT NULL,
    attendee_email_enc   BYTEA,
    secret_enc           BYTEA,
    attendee_name_bidx   BYTEA NOT NULL,
    attendee_email_bidx  BYTEA,
    secret_bidx          BYTEA,
    status               TEXT NOT NULL DEFAULT 'valid' CHECK (status IN ('valid', 'pending', 'cancelled', 'refunded')),
    imported_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, external_ref)
);
CREATE UNIQUE INDEX order_positions_secret ON order_positions (tenant_id, event_id, secret_bidx) WHERE secret_bidx IS NOT NULL;
CREATE INDEX order_positions_by_event ON order_positions (tenant_id, event_id, status);
CREATE INDEX order_positions_email_bidx ON order_positions (tenant_id, event_id, attendee_email_bidx) WHERE attendee_email_bidx IS NOT NULL;
CREATE INDEX order_positions_name_bidx ON order_positions (tenant_id, event_id, attendee_name_bidx);
ALTER TABLE order_positions ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_positions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON order_positions
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON order_positions TO klubhub_app;
COMMENT ON TABLE order_positions IS 'data_class: personal';
COMMENT ON COLUMN order_positions.external_ref IS 'data_class: internal — the platform''s ticket id within the order, or "#n" for its n-th ticket when the export has none';
COMMENT ON COLUMN order_positions.status IS 'data_class: internal — ticket state (valid, pending, cancelled, refunded)';

-- +goose Down
DROP TABLE IF EXISTS order_positions;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS attendee_imports;
DROP TABLE IF EXISTS ticket_types;
