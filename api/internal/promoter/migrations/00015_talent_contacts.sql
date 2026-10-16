-- +goose Up
-- P4 bookings outreach (phase 04.10): talent contacts — the DJs a promoter
-- knows (SPEC §6.1). Name/email/phone are `personal` data, sealed with the
-- tenant DEK and blind-indexed for exact lookup and dedupe (the audience
-- pattern). City/country/genres are plaintext matching keys (SPEC §8).
-- A contact may be name-only: email is optional, and email-less contacts are
-- listed/matched with has_email=false and never sent to (OFFER-10).
--
-- This is deliberately NOT the consent-based audience_contacts table: talent
-- contacts are B2B relationships (past gig, referral, manual), a different
-- purpose and lawful basis than the marketing audience (SPEC §9, §14 Q1).

CREATE TABLE talent_contacts (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name_enc    BYTEA,
    email_enc   BYTEA,
    phone_enc   BYTEA,
    name_bidx   BYTEA,
    email_bidx  BYTEA,
    city        TEXT NOT NULL DEFAULT '',
    country     TEXT NOT NULL DEFAULT '',
    genres      TEXT[] NOT NULL DEFAULT '{}',
    source      TEXT NOT NULL CHECK (source IN ('past_gig', 'referral', 'manual')),
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- A contact needs at least one of email or name to be findable.
ALTER TABLE talent_contacts ADD CONSTRAINT talent_contacts_has_identity
    CHECK (email_enc IS NOT NULL OR name_enc IS NOT NULL);
-- Dedupe and exact lookup on the blind index; one contact per email per tenant.
CREATE UNIQUE INDEX talent_contacts_email_bidx ON talent_contacts (tenant_id, email_bidx) WHERE email_bidx IS NOT NULL;
CREATE INDEX talent_contacts_name_bidx ON talent_contacts (tenant_id, name_bidx) WHERE name_bidx IS NOT NULL;
-- Plaintext matching keys (SPEC §8): city-name equality, genre overlap.
CREATE INDEX talent_contacts_city ON talent_contacts (tenant_id, lower(city));
CREATE INDEX talent_contacts_genres ON talent_contacts USING GIN (genres);
ALTER TABLE talent_contacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE talent_contacts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON talent_contacts
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON talent_contacts TO klubhub_app;
COMMENT ON TABLE talent_contacts IS 'data_class: personal';
COMMENT ON COLUMN talent_contacts.city IS 'data_class: internal — plaintext matching key (city-name equality, SPEC §8), not identifying on its own';
COMMENT ON COLUMN talent_contacts.country IS 'data_class: internal — plaintext matching key, not identifying on its own';
COMMENT ON COLUMN talent_contacts.genres IS 'data_class: internal — plaintext matching key (genre overlap, SPEC §8)';
COMMENT ON COLUMN talent_contacts.source IS 'data_class: internal — how the relationship arose (past gig, referral, manual); provenance for the lawful-basis record';
COMMENT ON COLUMN talent_contacts.notes IS 'data_class: internal — free-text relationship note; the UI labels it as internal, not a contact detail';

-- +goose Down
DROP TABLE IF EXISTS talent_contacts;
