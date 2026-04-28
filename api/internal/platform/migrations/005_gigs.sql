-- +goose Up

-- Gig status and payment status enums
CREATE TYPE gig_status AS ENUM ('inquiry', 'confirmed', 'advanced', 'played', 'cancelled');
CREATE TYPE payment_status AS ENUM ('unpaid', 'deposit_paid', 'paid', 'overdue', 'waived');

-- Gigs table
CREATE TABLE IF NOT EXISTS gigs (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    date                        TIMESTAMPTZ NOT NULL,
    venue                       TEXT NOT NULL DEFAULT '',
    city                        TEXT NOT NULL DEFAULT '',
    country                     TEXT NOT NULL DEFAULT '',
    event_name                  TEXT NOT NULL DEFAULT '',
    promoter_name               TEXT NOT NULL DEFAULT '',
    promoter_email              TEXT NOT NULL DEFAULT '',
    promoter_phone              TEXT NOT NULL DEFAULT '',
    fee_amount                  DECIMAL(12,2) NOT NULL DEFAULT 0,
    fee_currency                TEXT NOT NULL DEFAULT 'USD',
    set_length_minutes          INT NOT NULL DEFAULT 0,
    notes                       TEXT NOT NULL DEFAULT '',
    status                      gig_status NOT NULL DEFAULT 'inquiry',
    payment_status              payment_status NOT NULL DEFAULT 'unpaid',
    gig_reader_venue_id         UUID,  -- FK nullable: links to reusable venue record
    gig_reader_contact_id       UUID,  -- FK nullable: links to reusable contact record
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at                  TIMESTAMPTZ  -- soft delete
);

CREATE INDEX IF NOT EXISTS gigs_status_idx ON gigs(status);
CREATE INDEX IF NOT EXISTS gigs_payment_status_idx ON gigs(payment_status);
CREATE INDEX IF NOT EXISTS gigs_date_idx ON gigs(date);
CREATE INDEX IF NOT EXISTS gigs_deleted_at_idx ON gigs(deleted_at) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS gigs;
DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS gig_status;
