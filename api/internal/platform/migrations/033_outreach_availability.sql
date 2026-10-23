-- +goose Up

-- Availability slots: dates/cities the DJ is available for booking. The
-- outreach matcher (04.10) pairs each slot with the DJ's own promoter
-- contacts in the same city. Country is part of the unique key so two
-- same-named cities in different countries do not collide (OUTR-09).
-- The date-not-in-the-past rule is enforced at validation time, not by
-- the DB (a slot created today for tomorrow must stay valid).
CREATE TABLE outreach_availability_slots (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  date        date NOT NULL,
  city        text NOT NULL,
  country     text NOT NULL DEFAULT '',
  genres      text[] NOT NULL DEFAULT '{}',
  fee_min     numeric(12,2),
  fee_max     numeric(12,2),
  currency    char(3),
  notes       text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (date, city, country)
);

-- +goose Down
DROP TABLE IF EXISTS outreach_availability_slots;
