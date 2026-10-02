-- +goose Up

-- Rider templates: reusable named sets of four rider sections (technical,
-- hospitality, backline, other notes). Soft-deletion via deleted_at
-- (RIDER-05). Ordering is by name for stable list rendering.
--
-- Size limits (name <= 200 characters, each section <= 20000) mirror
-- rider.MaxNameChars / rider.MaxSectionChars; the service rejects with a
-- readable 422 first, these CHECKs are the backstop. Keep them in sync.

CREATE TABLE IF NOT EXISTS rider_templates (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    technical    TEXT NOT NULL DEFAULT '',
    hospitality  TEXT NOT NULL DEFAULT '',
    backline     TEXT NOT NULL DEFAULT '',
    other_notes  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    CONSTRAINT rider_templates_name_check
        CHECK (char_length(name) BETWEEN 1 AND 200 AND name ~ '[^[:space:]]'),
    CONSTRAINT rider_templates_sections_check
        CHECK (char_length(technical) <= 20000 AND char_length(hospitality) <= 20000
           AND char_length(backline) <= 20000 AND char_length(other_notes) <= 20000)
);

CREATE INDEX IF NOT EXISTS rider_templates_alive_idx
    ON rider_templates (name) WHERE deleted_at IS NULL;

-- Two live templates cannot share a name (case-insensitive): otherwise the
-- picker offers indistinguishable entries. A soft-deleted template frees its
-- name. The repository maps a violation of this index to a 422.
CREATE UNIQUE INDEX IF NOT EXISTS rider_templates_name_unique
    ON rider_templates (lower(name)) WHERE deleted_at IS NULL;

-- Rider attachments: per-gig copies of a rider template (or manually entered).
-- The copy is a snapshot, not a live link to the source template
-- (RIDER-02): updating the source template never changes the per-gig copy.
-- template_id is nullable so a gig may have an attachment with no source
-- template (manual entry / "Start from blank").
--
-- Gigs and templates are only ever soft-deleted by the application, so the
-- FK actions below are a backstop for hard deletes (RESTRICT protects gig
-- history; SET NULL keeps the per-gig copy but drops the source reference).
-- Soft-deleting a template does the same explicitly (SoftDeleteTemplate sets
-- template_id to NULL on its attachments), so an attachment never points at a
-- template that no longer exists.
-- Attaching to a soft-deleted gig is refused in the repository, not here.

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
    CONSTRAINT rider_attachments_sections_check
        CHECK (char_length(technical) <= 20000 AND char_length(hospitality) <= 20000
           AND char_length(backline) <= 20000 AND char_length(other_notes) <= 20000),
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