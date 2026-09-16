-- +goose Up

-- Join table: gig can have multiple venues (e.g., multiple rooms at a festival)
CREATE TABLE IF NOT EXISTS gig_venues (
    gig_id      UUID NOT NULL REFERENCES gigs(id) ON DELETE CASCADE,
    venue_id    UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    is_primary  BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (gig_id, venue_id)
);

CREATE INDEX IF NOT EXISTS gig_venues_gig_id_idx ON gig_venues(gig_id);
CREATE INDEX IF NOT EXISTS gig_venues_venue_id_idx ON gig_venues(venue_id);

-- +goose Down
DROP TABLE IF EXISTS gig_venues;
