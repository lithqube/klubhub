-- +goose Up
-- Collective profile (P1.5): what the export pack says about the organiser.
-- Everything here is meant to be public; contacts stay out of it.
ALTER TABLE organizations
    ADD COLUMN bio            TEXT NOT NULL DEFAULT '' CHECK (length(bio) <= 2000),
    ADD COLUMN website_url    TEXT CHECK (website_url ~ '^https://' AND length(website_url) <= 300),
    ADD COLUMN instagram_url  TEXT CHECK (instagram_url ~ '^https://' AND length(instagram_url) <= 300),
    ADD COLUMN soundcloud_url TEXT CHECK (soundcloud_url ~ '^https://' AND length(soundcloud_url) <= 300),
    ADD COLUMN ra_url         TEXT CHECK (ra_url ~ '^https://' AND length(ra_url) <= 300),
    ADD COLUMN accent_color   TEXT CHECK (accent_color ~ '^#[0-9a-f]{6}$');

COMMENT ON COLUMN organizations.bio IS 'data_class: public — the collective''s public description';
COMMENT ON COLUMN organizations.website_url IS 'data_class: public — public link';
COMMENT ON COLUMN organizations.instagram_url IS 'data_class: public — public link';
COMMENT ON COLUMN organizations.soundcloud_url IS 'data_class: public — public link';
COMMENT ON COLUMN organizations.ra_url IS 'data_class: public — public link';
COMMENT ON COLUMN organizations.accent_color IS 'data_class: public — brand colour for exports';

-- +goose Down
ALTER TABLE organizations
    DROP COLUMN accent_color,
    DROP COLUMN ra_url,
    DROP COLUMN soundcloud_url,
    DROP COLUMN instagram_url,
    DROP COLUMN website_url,
    DROP COLUMN bio;
