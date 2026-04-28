-- +goose Up

-- Join table: tracklist can be linked to multiple gigs
CREATE TABLE IF NOT EXISTS tracklist_gigs (
    tracklist_id    UUID NOT NULL REFERENCES tracklists(id) ON DELETE CASCADE,
    gig_id          UUID NOT NULL REFERENCES gigs(id) ON DELETE CASCADE,
    PRIMARY KEY (tracklist_id, gig_id)
);

CREATE INDEX IF NOT EXISTS tracklist_gigs_tracklist_id_idx ON tracklist_gigs(tracklist_id);
CREATE INDEX IF NOT EXISTS tracklist_gigs_gig_id_idx ON tracklist_gigs(gig_id);

-- +goose Down
DROP TABLE IF EXISTS tracklist_gigs;
