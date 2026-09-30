-- +goose Up

-- Rider templates: reusable named sets of four rider sections (technical,
-- hospitality, backline, other notes). Soft-deletion via deleted_at
-- (RIDER-05). Ordering is by name for stable list rendering.

CREATE TABLE IF NOT EXISTS rider_templates (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    technical    TEXT NOT NULL DEFAULT '',
    hospitality  TEXT NOT NULL DEFAULT '',
    backline     TEXT NOT NULL DEFAULT '',
    other_notes  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS rider_templates_alive_idx
    ON rider_templates (name) WHERE deleted_at IS NULL;

-- Rider attachments: per-gig copies of a rider template (or manually entered).
-- The copy is a snapshot, not a live link to the source template
-- (RIDER-02): updating the source template never changes the per-gig copy.
-- template_id is nullable so a gig may have an attachment with no source
-- template (manual entry / "Start from blank").
--
-- On gig delete the FK RESTRICT prevents destroying gig history that a
-- per-gig copy references; on template delete the FK SET NULL preserves
-- the per-gig copy but loses the source reference (intentional — soft-delete
-- on templates hides them but keeps rows for forensics).

CREATE TABLE IF NOT EXISTS rider_attachments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gig_id       UUID NOT NULL,
    template_id  UUID,
    technical    TEXT NOT NULL DEFAULT '',
    hospitality  TEXT NOT NULL DEFAULT '',
    backline     TEXT NOT NULL DEFAULT '',
    other_notes  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    CONSTRAINT rider_attachments_gig_id_fkey FOREIGN KEY (gig_id)
        REFERENCES gigs (id) ON DELETE RESTRICT,
    CONSTRAINT rider_attachments_template_id_fkey FOREIGN KEY (template_id)
        REFERENCES rider_templates (id) ON DELETE SET NULL
);

-- One live attachment per gig (RIDER-02). Uses the boolean-column partial
-- unique index pattern (not a subquery predicate) because Postgres rejects
-- subqueries in index WHERE clauses (SQLSTATE 0A000). See
-- references/finance-module-pattern.md.

CREATE UNIQUE INDEX IF NOT EXISTS rider_attachments_one_per_gig
    ON rider_attachments (gig_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS rider_attachments_gig_idx
    ON rider_attachments (gig_id);

-- +goose Down
DROP TABLE IF EXISTS rider_attachments;
DROP TABLE IF EXISTS rider_templates;