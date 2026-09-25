-- +goose Up
-- The Go models scan these columns into plain strings, and pgx refuses to
-- scan NULL into *string: creating an agreement instance (dj_signed_by /
-- client_signed_by start NULL) or an email (last_error starts NULL) failed
-- on every call. Empty string is the "not set" value throughout the app,
-- so make the columns NOT NULL DEFAULT '' rather than widening every model.
UPDATE agreement_templates SET description = '' WHERE description IS NULL;
ALTER TABLE agreement_templates
    ALTER COLUMN description SET DEFAULT '',
    ALTER COLUMN description SET NOT NULL;

UPDATE agreement_instances SET dj_signed_by = '' WHERE dj_signed_by IS NULL;
UPDATE agreement_instances SET client_signed_by = '' WHERE client_signed_by IS NULL;
UPDATE agreement_instances SET internal_notes = '' WHERE internal_notes IS NULL;
ALTER TABLE agreement_instances
    ALTER COLUMN dj_signed_by SET DEFAULT '',
    ALTER COLUMN dj_signed_by SET NOT NULL,
    ALTER COLUMN client_signed_by SET DEFAULT '',
    ALTER COLUMN client_signed_by SET NOT NULL,
    ALTER COLUMN internal_notes SET DEFAULT '',
    ALTER COLUMN internal_notes SET NOT NULL;

UPDATE email_messages SET owner_type = '' WHERE owner_type IS NULL;
UPDATE email_messages SET body_html = '' WHERE body_html IS NULL;
UPDATE email_messages SET last_error = '' WHERE last_error IS NULL;
ALTER TABLE email_messages
    ALTER COLUMN owner_type SET DEFAULT '',
    ALTER COLUMN owner_type SET NOT NULL,
    ALTER COLUMN body_html SET DEFAULT '',
    ALTER COLUMN body_html SET NOT NULL,
    ALTER COLUMN last_error SET DEFAULT '',
    ALTER COLUMN last_error SET NOT NULL;

-- +goose Down
ALTER TABLE email_messages
    ALTER COLUMN owner_type DROP NOT NULL, ALTER COLUMN owner_type DROP DEFAULT,
    ALTER COLUMN body_html DROP NOT NULL, ALTER COLUMN body_html DROP DEFAULT,
    ALTER COLUMN last_error DROP NOT NULL, ALTER COLUMN last_error DROP DEFAULT;
ALTER TABLE agreement_instances
    ALTER COLUMN dj_signed_by DROP NOT NULL, ALTER COLUMN dj_signed_by DROP DEFAULT,
    ALTER COLUMN client_signed_by DROP NOT NULL, ALTER COLUMN client_signed_by DROP DEFAULT,
    ALTER COLUMN internal_notes DROP NOT NULL, ALTER COLUMN internal_notes DROP DEFAULT;
ALTER TABLE agreement_templates
    ALTER COLUMN description DROP NOT NULL, ALTER COLUMN description DROP DEFAULT;
