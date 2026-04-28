-- +goose Up

-- Join table: gig can have multiple contacts (promoter, agent, etc.)
CREATE TABLE IF NOT EXISTS gig_contacts (
    gig_id      UUID NOT NULL REFERENCES gigs(id) ON DELETE CASCADE,
    contact_id  UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'promoter',  -- 'promoter', 'agent', etc.
    PRIMARY KEY (gig_id, contact_id)
);

CREATE INDEX IF NOT EXISTS gig_contacts_gig_id_idx ON gig_contacts(gig_id);
CREATE INDEX IF NOT EXISTS gig_contacts_contact_id_idx ON gig_contacts(contact_id);

-- +goose Down
DROP TABLE IF EXISTS gig_contacts;
