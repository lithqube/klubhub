-- +goose Up
-- P2.1: guest lists, standing lists, allocations and guests
-- (.claude/plans/promoter-p2-guests-door.plan.md). Guest names, contacts and
-- notes are sealed with the tenant DEK; exact lookups (paste-by-email,
-- dedupe) go through HMAC blind indexes. Lists are name-only by default:
-- email and phone are stored only when the list collects contact details.

CREATE TABLE standing_lists (
    id               UUID PRIMARY KEY,
    tenant_id        UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name             TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    type             TEXT NOT NULL CHECK (type IN ('artist', 'promoter', 'comp', 'industry', 'vip', 'reduced', 'crew')),
    entry_terms      JSONB NOT NULL DEFAULT '{}'::jsonb,
    collect_contact  BOOLEAN NOT NULL DEFAULT false,
    position         SMALLINT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE standing_lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE standing_lists FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON standing_lists
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON standing_lists TO klubhub_app;
COMMENT ON TABLE standing_lists IS 'data_class: internal';
COMMENT ON COLUMN standing_lists.name IS 'data_class: internal — a list label such as "Resident DJs", not a person';

CREATE TABLE guest_lists (
    id                    UUID PRIMARY KEY,
    tenant_id             UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id              UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    name                  TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    type                  TEXT NOT NULL CHECK (type IN ('artist', 'promoter', 'comp', 'industry', 'vip', 'reduced', 'crew')),
    entry_terms           JSONB NOT NULL DEFAULT '{}'::jsonb,
    collect_contact       BOOLEAN NOT NULL DEFAULT false,
    standing_template_id  UUID REFERENCES standing_lists (id) ON DELETE SET NULL,
    position              SMALLINT NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX guest_lists_by_event ON guest_lists (tenant_id, event_id, position);
ALTER TABLE guest_lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_lists FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON guest_lists
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON guest_lists TO klubhub_app;
COMMENT ON TABLE guest_lists IS 'data_class: internal';
COMMENT ON COLUMN guest_lists.name IS 'data_class: internal — a list label such as "Artist guests", not a person';

CREATE TABLE guest_allocations (
    id                     UUID PRIMARY KEY,
    tenant_id              UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id               UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    list_id                UUID NOT NULL REFERENCES guest_lists (id) ON DELETE CASCADE,
    label                  TEXT NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
    submitter_contact_enc  BYTEA,
    quota                  INTEGER NOT NULL CHECK (quota BETWEEN 1 AND 1000),
    plus_n_max             SMALLINT NOT NULL DEFAULT 0 CHECK (plus_n_max BETWEEN 0 AND 10),
    deadline               TIMESTAMPTZ,
    requires_approval      BOOLEAN NOT NULL DEFAULT false,
    revoked_at             TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX guest_allocations_by_list ON guest_allocations (tenant_id, list_id);
ALTER TABLE guest_allocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE guest_allocations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON guest_allocations
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON guest_allocations TO klubhub_app;
COMMENT ON TABLE guest_allocations IS 'data_class: internal';
COMMENT ON COLUMN guest_allocations.label IS 'data_class: public — the submitter as billed (artist or stage name), the reviewed exception as in lineup_entries.display_name; private contacts go in submitter_contact_enc';

CREATE TABLE guests (
    id             UUID PRIMARY KEY,
    tenant_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id       UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    list_id        UUID NOT NULL REFERENCES guest_lists (id) ON DELETE CASCADE,
    allocation_id  UUID REFERENCES guest_allocations (id) ON DELETE SET NULL,
    name_enc       BYTEA NOT NULL,
    email_enc      BYTEA,
    phone_enc      BYTEA,
    note_enc       BYTEA,
    name_bidx      BYTEA NOT NULL,
    email_bidx     BYTEA,
    plus_n         SMALLINT NOT NULL DEFAULT 0 CHECK (plus_n BETWEEN 0 AND 10),
    status         TEXT NOT NULL DEFAULT 'going' CHECK (status IN ('going', 'pending', 'waitlist', 'invited', 'declined')),
    source         TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'paste', 'import', 'door')),
    created_by     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX guests_by_event ON guests (tenant_id, event_id, status);
CREATE INDEX guests_by_list ON guests (tenant_id, list_id);
CREATE INDEX guests_by_allocation ON guests (tenant_id, allocation_id) WHERE allocation_id IS NOT NULL;
CREATE INDEX guests_email_bidx ON guests (tenant_id, event_id, email_bidx) WHERE email_bidx IS NOT NULL;
CREATE INDEX guests_name_bidx ON guests (tenant_id, event_id, name_bidx);
ALTER TABLE guests ENABLE ROW LEVEL SECURITY;
ALTER TABLE guests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON guests
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON guests TO klubhub_app;
COMMENT ON TABLE guests IS 'data_class: personal';
COMMENT ON COLUMN guests.plus_n IS 'data_class: internal — a head count, not identifying';
COMMENT ON COLUMN guests.status IS 'data_class: internal — workflow state (going, pending, …)';
COMMENT ON COLUMN guests.source IS 'data_class: internal — how the row was added (manual, paste, import, door)';
COMMENT ON COLUMN guests.created_by IS 'data_class: internal — pseudonymous staff subject id (local:<uuid>), as in audit_log.actor_id';

-- +goose Down
DROP TABLE IF EXISTS guests;
DROP TABLE IF EXISTS guest_allocations;
DROP TABLE IF EXISTS guest_lists;
DROP TABLE IF EXISTS standing_lists;
