-- +goose Up
-- Payments: tracks actual money movements against invoices.
-- Supports deposits, partial payments, full payments, refunds.
-- Each payment references one invoice and one currency.
-- Status: pending → completed | failed | refunded.
-- The sum of completed payments for an invoice must not exceed invoice total.
CREATE TABLE IF NOT EXISTS payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id      UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
    currency        TEXT NOT NULL
        CHECK (char_length(currency) = 3 AND currency = upper(currency)),
    amount_minor    BIGINT NOT NULL
        CHECK (amount_minor > 0),
    kind            TEXT NOT NULL DEFAULT 'payment'
        CHECK (kind IN ('deposit', 'payment', 'refund')),
    status          TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'completed', 'failed', 'refunded')),
    method          TEXT NOT NULL DEFAULT '',
    reference       TEXT NOT NULL DEFAULT '',
    received_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS payments_invoice_id_idx ON payments (invoice_id);
CREATE INDEX IF NOT EXISTS payments_status_idx ON payments (status);

-- +goose Down
DROP TABLE IF EXISTS payments;