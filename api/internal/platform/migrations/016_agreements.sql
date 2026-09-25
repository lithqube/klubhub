-- +goose Up
-- Agreements: versioned templates + per-gig instances with signing workflow.
-- Each agreement has a template (shared) and zero or more instances (per gig).
-- Template versioning allows DJs to evolve their standard terms.
-- Instance lifecycle: draft → sent → signed → completed | expired | cancelled.
-- Signing: when both parties have signed, instance status becomes "completed".
CREATE TABLE IF NOT EXISTS agreement_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    description     TEXT,
    content_md      TEXT NOT NULL,      -- Markdown template with {{placeholders}}
    version         INT NOT NULL DEFAULT 1,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_by      TEXT NOT NULL DEFAULT 'system',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS agreement_templates_name_version_idx
    ON agreement_templates (name, version);

CREATE INDEX IF NOT EXISTS agreement_templates_active_idx
    ON agreement_templates (is_active) WHERE is_active = true;

-- Per-gig agreement instances
CREATE TABLE IF NOT EXISTS agreement_instances (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id         UUID NOT NULL REFERENCES agreement_templates(id),
    template_version    INT NOT NULL,
    gig_id              UUID NOT NULL,
    -- Rendered content at time of creation (Markdown with placeholders filled)
    content_md          TEXT NOT NULL,
    -- Rendered PDF (stored in Garage S3 via documents table)
    document_id         UUID REFERENCES documents(id),
    -- Signing workflow
    status              TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'sent', 'signed', 'completed', 'expired', 'cancelled')),
    -- Who needs to sign (dj, client, both)
    required_signers    TEXT[] NOT NULL DEFAULT ARRAY['dj', 'client'],
    dj_signed_at        TIMESTAMPTZ,
    dj_signed_by        TEXT,
    client_signed_at    TIMESTAMPTZ,
    client_signed_by    TEXT,
    -- Expiry
    expires_at          TIMESTAMPTZ,
    -- Metadata
    internal_notes      TEXT,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS agreement_instances_gig_idx ON agreement_instances (gig_id);
CREATE INDEX IF NOT EXISTS agreement_instances_status_idx ON agreement_instances (status);

-- +goose Down
DROP TABLE IF EXISTS agreement_instances;
DROP TABLE IF EXISTS agreement_templates;