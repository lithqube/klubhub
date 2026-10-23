-- +goose Up

-- outreach_requests is the sent log: one row per booking-request email,
-- linked to the email outbox row for delivery status. The row is inserted
-- in the same transaction as the daily-cap check (SPEC §9 v2.1), so failed
-- attempts count towards the cap and two concurrent sends cannot both
-- pass at 49/50. The rendered subject/body are stored here — the template
-- is never mutated by a send.
CREATE TABLE outreach_requests (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  contact_id           uuid NOT NULL REFERENCES contacts(id),
  availability_slot_id uuid REFERENCES outreach_availability_slots(id) ON DELETE SET NULL,
  template_id          uuid REFERENCES outreach_templates(id) ON DELETE SET NULL,
  epk_export_id        uuid REFERENCES epk_exports(id) ON DELETE SET NULL,
  email_message_id     uuid REFERENCES email_messages(id) ON DELETE SET NULL,
  to_email             text NOT NULL,
  subject              text NOT NULL,
  body                 text NOT NULL,
  status               text NOT NULL DEFAULT 'queued',  -- queued|sending|sent|failed|bounced
  sent_at              timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX outreach_requests_contact_id ON outreach_requests (contact_id);

-- outreach_suppressions is the do-not-contact list. The DJ side stores
-- the normalized (lower, trimmed) email in plaintext — single-user OSS,
-- consistent with contacts.email. Sending to a suppressed address is
-- refused (SPEC §9).
CREATE TABLE outreach_suppressions (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email      text NOT NULL,
  reason     text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (email)
);

-- +goose Down
DROP TABLE IF EXISTS outreach_suppressions;
DROP TABLE IF EXISTS outreach_requests;
