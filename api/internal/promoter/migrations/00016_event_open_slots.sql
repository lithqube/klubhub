-- +goose Up
-- P4 bookings outreach (phase 04.10): open slots on events (SPEC §6.1) —
-- unfilled positions a promoter wants to fill (role, genres, fee range).
-- Separate from lineup_entries, which are *billed* acts. Scoped to the
-- tenant's events; UNIQUE (event_id, role) keeps one slot per role per event.

CREATE TABLE event_open_slots (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id    UUID NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT '',
    genres      TEXT[] NOT NULL DEFAULT '{}',
    fee_min     NUMERIC(12,2),
    fee_max     NUMERIC(12,2),
    currency    CHAR(3),
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, role)
);
CREATE INDEX event_open_slots_event_idx ON event_open_slots (tenant_id, event_id);
ALTER TABLE event_open_slots ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_open_slots FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON event_open_slots
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON event_open_slots TO klubhub_app;
COMMENT ON TABLE event_open_slots IS 'data_class: internal';
COMMENT ON COLUMN event_open_slots.role IS 'data_class: internal — the position to fill (headliner, warm-up, …), not a person';
COMMENT ON COLUMN event_open_slots.genres IS 'data_class: internal — plaintext matching key (genre overlap, SPEC §8)';
COMMENT ON COLUMN event_open_slots.fee_min IS 'data_class: internal — the offered fee range, the promoter''s own figure';
COMMENT ON COLUMN event_open_slots.fee_max IS 'data_class: internal — the offered fee range, the promoter''s own figure';
COMMENT ON COLUMN event_open_slots.currency IS 'data_class: internal — ISO 4217 currency of the fee range';
COMMENT ON COLUMN event_open_slots.notes IS 'data_class: internal — production note for the slot (e.g. "PA + booth"), not a person';

-- +goose Down
DROP TABLE IF EXISTS event_open_slots;
