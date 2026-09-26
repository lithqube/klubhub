-- +goose Up
-- P1: venues, events, stages and lineup (.claude/plans/promoter-p1-events.plan.md).
-- Times are timestamptz (UTC) plus an IANA timezone for display. Venue
-- addresses, coordinates and contacts are sealed with the tenant DEK: a
-- warehouse or open-air location can be the most sensitive fact of a night.

CREATE TABLE venues (
    id                  UUID PRIMARY KEY,
    tenant_id           UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    city                TEXT NOT NULL DEFAULT '' CHECK (length(city) <= 120),
    country             CHAR(2) CHECK (country ~ '^[A-Z]{2}$'),
    timezone            TEXT NOT NULL DEFAULT 'UTC',
    capacity            INTEGER CHECK (capacity > 0),
    curfew_local        TIME,
    tech_notes          TEXT NOT NULL DEFAULT '' CHECK (length(tech_notes) <= 5000),
    address_enc         BYTEA,
    geo_enc             BYTEA,
    contact_name_enc    BYTEA,
    contact_email_enc   BYTEA,
    contact_phone_enc   BYTEA,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at         TIMESTAMPTZ
);
CREATE INDEX venues_active ON venues (tenant_id, name) WHERE archived_at IS NULL;
ALTER TABLE venues ENABLE ROW LEVEL SECURITY;
ALTER TABLE venues FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON venues
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON venues TO klubhub_app;
COMMENT ON TABLE venues IS 'data_class: internal';
COMMENT ON COLUMN venues.name IS 'data_class: public — the venue''s public name';
COMMENT ON COLUMN venues.tech_notes IS 'data_class: internal — equipment and production specs; contacts have sealed fields and the UI says so';

CREATE TABLE venue_rooms (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    venue_id    UUID NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    capacity    INTEGER CHECK (capacity > 0),
    position    SMALLINT NOT NULL DEFAULT 0
);
ALTER TABLE venue_rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE venue_rooms FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON venue_rooms
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON venue_rooms TO klubhub_app;
COMMENT ON TABLE venue_rooms IS 'data_class: internal';
COMMENT ON COLUMN venue_rooms.name IS 'data_class: public — room / stage name';

CREATE TABLE events (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    title                TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    slug                 TEXT NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,80}$'),
    status               TEXT NOT NULL DEFAULT 'draft'
                         CHECK (status IN ('draft', 'scheduled', 'published', 'cancelled', 'postponed')),
    visibility           TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'unlisted', 'private')),
    publish_at           TIMESTAMPTZ,
    starts_at            TIMESTAMPTZ NOT NULL,
    ends_at              TIMESTAMPTZ NOT NULL,
    doors_at             TIMESTAMPTZ,
    timezone             TEXT NOT NULL,
    venue_id             UUID REFERENCES venues (id) ON DELETE SET NULL,
    city                 TEXT NOT NULL DEFAULT '' CHECK (length(city) <= 120),
    location_mode        TEXT NOT NULL DEFAULT 'venue' CHECK (location_mode IN ('venue', 'city_only', 'secret')),
    location_reveal_at   TIMESTAMPTZ,
    min_age              SMALLINT CHECK (min_age BETWEEN 0 AND 30),
    genres               TEXT[] NOT NULL DEFAULT '{}',
    description_md       TEXT NOT NULL DEFAULT '' CHECK (length(description_md) <= 20000),
    cost_text            TEXT NOT NULL DEFAULT '' CHECK (length(cost_text) <= 200),
    external_ticket_url  TEXT CHECK (external_ticket_url ~ '^https://'),
    capacity             INTEGER CHECK (capacity > 0),
    version              INTEGER NOT NULL DEFAULT 1,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (doors_at IS NULL OR doors_at <= starts_at),
    UNIQUE (tenant_id, slug)
);
CREATE INDEX events_upcoming ON events (tenant_id, starts_at);
ALTER TABLE events ENABLE ROW LEVEL SECURITY;
ALTER TABLE events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON events
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON events TO klubhub_app;
COMMENT ON TABLE events IS 'data_class: public';

CREATE TABLE event_stages (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id             UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    name                 TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    position             SMALLINT NOT NULL DEFAULT 0,
    curfew_at            TIMESTAMPTZ,
    changeover_minutes   SMALLINT NOT NULL DEFAULT 0 CHECK (changeover_minutes BETWEEN 0 AND 120)
);
ALTER TABLE event_stages ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_stages FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON event_stages
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON event_stages TO klubhub_app;
COMMENT ON TABLE event_stages IS 'data_class: public';
COMMENT ON COLUMN event_stages.name IS 'data_class: public — stage name';

CREATE TABLE lineup_entries (
    id             UUID PRIMARY KEY,
    tenant_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id       UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    stage_id       UUID REFERENCES event_stages (id) ON DELETE SET NULL,
    display_name   TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 120),
    profile_url    TEXT CHECK (profile_url ~ '^https://'),
    billing_order  SMALLINT NOT NULL DEFAULT 0,
    b2b_group      SMALLINT,
    set_start      TIMESTAMPTZ,
    set_end        TIMESTAMPTZ,
    CHECK ((set_start IS NULL) = (set_end IS NULL)),
    CHECK (set_end IS NULL OR set_end > set_start)
);
CREATE INDEX lineup_by_event ON lineup_entries (tenant_id, event_id, billing_order);
ALTER TABLE lineup_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE lineup_entries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON lineup_entries
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON lineup_entries TO klubhub_app;
COMMENT ON TABLE lineup_entries IS 'data_class: public';
COMMENT ON COLUMN lineup_entries.display_name IS 'data_class: public — artist stage name as billed';

-- Door PINs now belong to real events.
ALTER TABLE door_pins ADD CONSTRAINT door_pins_event_fk
    FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE door_pins DROP CONSTRAINT IF EXISTS door_pins_event_fk;
DROP TABLE IF EXISTS lineup_entries;
DROP TABLE IF EXISTS event_stages;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS venue_rooms;
DROP TABLE IF EXISTS venues;
