-- +goose Up

-- EN 16931 data model (docs: .claude/plans/e-invoicing.plan.md §2). All columns
-- are additive with empty / neutral defaults, so existing rows and clients keep
-- working; nothing here blocks issuing (export readiness is checked separately).
--
-- Seller (billing profile, snapshotted onto the invoice at issue):
--   tax_number  BT-32 national tax number (DE Steuernummer), distinct from the
--               VAT ID kept in tax_id when tax_id_kind = 'vat'.
--   iban / bic  BG-17 payment account (BT-84 / BT-86). Stored normalised
--               (upper-case, no spaces); the IBAN checksum is validated in Go.
ALTER TABLE billing_profiles
    ADD COLUMN tax_number TEXT NOT NULL DEFAULT '',
    ADD COLUMN iban       TEXT NOT NULL DEFAULT '',
    ADD COLUMN bic        TEXT NOT NULL DEFAULT '';

-- Invoice level references and terms:
--   buyer_reference     BT-10 (mandatory in XRechnung: the Leitweg-ID for public clients)
--   purchase_order_ref  BT-13
--   contract_ref        BT-12
--   payment_terms       BT-20 free-text terms, e.g. "Payable within 14 days"
ALTER TABLE invoices
    ADD COLUMN buyer_reference    TEXT NOT NULL DEFAULT '',
    ADD COLUMN purchase_order_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN contract_ref       TEXT NOT NULL DEFAULT '',
    ADD COLUMN payment_terms      TEXT NOT NULL DEFAULT '';

-- BT-130 unit of measure (UN/ECE Recommendation 20 code). C62 = "one" / piece.
-- The allowed set is curated in Go (invoiceUnitCodes); the DB only guards shape.
ALTER TABLE invoice_lines
    ADD COLUMN unit_code TEXT NOT NULL DEFAULT 'C62'
        CONSTRAINT invoice_lines_unit_code_check CHECK (unit_code ~ '^[A-Z0-9]{2,3}$');

-- +goose Down
ALTER TABLE invoice_lines
    DROP COLUMN IF EXISTS unit_code;
ALTER TABLE invoices
    DROP COLUMN IF EXISTS payment_terms,
    DROP COLUMN IF EXISTS contract_ref,
    DROP COLUMN IF EXISTS purchase_order_ref,
    DROP COLUMN IF EXISTS buyer_reference;
ALTER TABLE billing_profiles
    DROP COLUMN IF EXISTS bic,
    DROP COLUMN IF EXISTS iban,
    DROP COLUMN IF EXISTS tax_number;
