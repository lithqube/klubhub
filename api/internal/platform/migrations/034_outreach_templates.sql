-- +goose Up

-- Outreach templates: the DJ's customizable booking-request copy. The
-- rendered subject/body are stored on each outreach_requests row at send
-- time, so editing or deleting a template never rewrites history.
-- version is the optimistic-concurrency token for PUT; is_default is
-- unique per direction (one default template).
CREATE TABLE outreach_templates (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL,
  direction   text NOT NULL CHECK (direction IN ('dj_to_promoter')),
  subject     text NOT NULL,
  body        text NOT NULL,
  is_default  boolean NOT NULL DEFAULT false,
  version     integer NOT NULL DEFAULT 1,
  deleted_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX outreach_templates_one_default ON outreach_templates (direction) WHERE is_default AND deleted_at IS NULL;
CREATE UNIQUE INDEX outreach_templates_name ON outreach_templates (lower(name)) WHERE deleted_at IS NULL;

-- Default template (SPEC §7): personalized opener, slot date/city/genres/
-- fee, EPK mention, opt-out footer. Variables are resolved by email.Render
-- at preview and send; unknown variables render as empty.
INSERT INTO outreach_templates (name, direction, subject, body, is_default)
VALUES (
  'Default booking request',
  'dj_to_promoter',
  'Booking request: {{dj_name}} in {{city}} on {{date}}',
  E'Hi {{promoter_name}},\n' ||
  E'\n' ||
  E'I''m {{dj_name}}, a DJ playing {{genres}}. I''m available in {{city}} on {{date}} and would love to play for you.\n' ||
  E'\n' ||
  E'Date: {{date}}\n' ||
  E'City: {{city}}\n' ||
  E'Genres: {{genres}}\n' ||
  E'Fee: {{fee}}\n' ||
  E'{{notes}}\n' ||
  E'\n' ||
  E'My press kit (EPK) is attached to this email.\n' ||
  E'{{epk_link}}\n' ||
  E'\n' ||
  E'If you''re interested, just reply to this email — it lands directly in my inbox ({{dj_email}}).\n' ||
  E'\n' ||
  E'If you''d rather not hear from us, reply "stop" and we''ll remove you.\n' ||
  E'\n' ||
  E'Best,\n' ||
  E'{{dj_name}}\n' ||
  E'{{dj_phone}}',
  true
);

-- The EPK attach path (SPEC §5.4) registers a documents pointer row with
-- kind 'epk_pdf' (owner_type 'epk' is already allowed since 015). The
-- kind CHECK was added inline in 031 (auto-named documents_kind_check);
-- extend it in place.
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_kind_check;
ALTER TABLE documents ADD CONSTRAINT documents_kind_check
  CHECK (kind IN ('general', 'invoice_pdf', 'einvoice_xml', 'validation_report', 'epk_pdf'));

-- +goose Down
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_kind_check;
ALTER TABLE documents ADD CONSTRAINT documents_kind_check
  CHECK (kind IN ('general', 'invoice_pdf', 'einvoice_xml', 'validation_report'));
DROP TABLE IF EXISTS outreach_templates;
