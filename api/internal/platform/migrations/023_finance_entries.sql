-- +goose Up
-- General finance ledger. Amounts are always positive integer minor units;
-- kind supplies the sign. Voiding preserves the original amount for audit.
CREATE TABLE finance_entries (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind                     TEXT NOT NULL CHECK (kind IN ('income', 'expense')),
    amount_minor             BIGINT NOT NULL CHECK (amount_minor > 0),
    currency                 TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    category                 TEXT NOT NULL CHECK (category ~ '[^[:space:]]'),
    entry_date               DATE NOT NULL,
    description              TEXT NOT NULL DEFAULT '',
    notes                    TEXT NOT NULL DEFAULT '',
    gig_id                   UUID REFERENCES gigs(id) ON DELETE RESTRICT,
    status                   TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'voided')),
    auto_generated           BOOLEAN NOT NULL DEFAULT false,
    source_kind              TEXT NOT NULL DEFAULT 'manual'
                                 CHECK (source_kind IN ('manual', 'gig_payment', 'invoice_payment')),
    source_id                UUID,
    source_amount_minor      BIGINT CHECK (source_amount_minor IS NULL OR source_amount_minor > 0),
    source_currency          TEXT CHECK (source_currency IS NULL OR source_currency ~ '^[A-Z]{3}$'),
    source_description       TEXT NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at               TIMESTAMPTZ,
    CONSTRAINT finance_entries_manual_source_check CHECK (
        (auto_generated = false AND source_kind = 'manual' AND source_id IS NULL)
        OR
        (auto_generated = true AND source_kind <> 'manual' AND source_id IS NOT NULL)
    ),
    CONSTRAINT finance_entries_kind_text_check CHECK (
        (kind = 'income' AND description ~ '[^[:space:]]')
        OR
        (kind = 'expense' AND notes ~ '[^[:space:]]')
    )
);

CREATE INDEX finance_entries_date_idx ON finance_entries (entry_date);
CREATE INDEX finance_entries_currency_date_idx ON finance_entries (currency, entry_date);
CREATE INDEX finance_entries_kind_date_idx ON finance_entries (kind, entry_date);
CREATE INDEX finance_entries_gig_id_idx ON finance_entries (gig_id) WHERE gig_id IS NOT NULL;
CREATE INDEX finance_entries_active_idx ON finance_entries (status, entry_date) WHERE deleted_at IS NULL;
CREATE INDEX finance_entries_deleted_at_idx ON finance_entries (deleted_at) WHERE deleted_at IS NOT NULL;

-- A generated source may create at most one entry of each kind. This is the
-- concurrency/idempotency boundary used by later gig/payment hooks.
CREATE UNIQUE INDEX finance_entries_generated_source_unique_idx
    ON finance_entries (source_kind, source_id, kind)
    WHERE auto_generated = true AND source_id IS NOT NULL
      AND status = 'active' AND deleted_at IS NULL;

-- Durable user decisions created by gig/payment transition hooks. A single
-- pending row per gig is refreshed as the source changes, so clients can
-- reliably fetch the latest prompt after a page reload.
CREATE TABLE finance_entry_reconciliations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gig_id              UUID NOT NULL REFERENCES gigs(id) ON DELETE RESTRICT,
    entry_id            UUID NOT NULL REFERENCES finance_entries(id) ON DELETE CASCADE,
    reason              TEXT NOT NULL CHECK (reason IN ('fee_changed', 'currency_changed', 'payment_reversed')),
    allowed_actions     TEXT[] NOT NULL,
    gig_amount_minor    BIGINT NOT NULL CHECK (gig_amount_minor > 0),
    gig_currency        TEXT NOT NULL CHECK (gig_currency ~ '^[A-Z]{3}$'),
    gig_payment_status  payment_status NOT NULL,
    entry_updated_at    TIMESTAMPTZ NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved')),
    resolution          TEXT CHECK (resolution IS NULL OR resolution IN ('update', 'delete', 'void', 'keep')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at         TIMESTAMPTZ,
    CONSTRAINT finance_entry_reconciliations_state_check CHECK (
        (status = 'pending' AND resolution IS NULL AND resolved_at IS NULL)
        OR (status = 'resolved' AND resolution IS NOT NULL AND resolved_at IS NOT NULL)
    ),
    CONSTRAINT finance_entry_reconciliations_actions_check CHECK (
        (reason IN ('fee_changed', 'currency_changed') AND allowed_actions = ARRAY['update','keep']::TEXT[])
        OR (reason = 'payment_reversed' AND allowed_actions = ARRAY['delete','void','keep']::TEXT[])
    )
);

CREATE UNIQUE INDEX finance_entry_reconciliations_one_pending_gig_idx
    ON finance_entry_reconciliations (gig_id) WHERE status = 'pending';
CREATE INDEX finance_entry_reconciliations_entry_idx
    ON finance_entry_reconciliations (entry_id, status);

-- +goose Down
DROP TABLE IF EXISTS finance_entry_reconciliations;
DROP TABLE IF EXISTS finance_entries;
