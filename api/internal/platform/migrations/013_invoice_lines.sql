-- +goose Up
-- Invoice line items. Belongs to an invoice. Quantity * unit_minor = line_total_minor.
-- Description is free-text (title of the performance/service).
-- tax_bps allows per-line tax rate override (default = invoice tax_rate_bps).
-- The sum of line_total_minor must equal invoice.subtotal_minor at issuance.
CREATE TABLE IF NOT EXISTS invoice_lines (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id      UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    sort_order      INT NOT NULL DEFAULT 0,
    description     TEXT NOT NULL,
    quantity        INT NOT NULL DEFAULT 1
        CHECK (quantity > 0),
    unit_minor      BIGINT NOT NULL
        CHECK (unit_minor >= 0),
    tax_bps         BIGINT NOT NULL DEFAULT 0,
    line_total_minor BIGINT GENERATED ALWAYS AS (quantity * unit_minor) STORED,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS invoice_lines_invoice_id_idx ON invoice_lines (invoice_id, sort_order);

-- +goose Down
DROP TABLE IF EXISTS invoice_lines;