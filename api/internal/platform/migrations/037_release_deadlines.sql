-- +goose Up

-- Release deadlines (Phase 6, REL-03): named dates a release is due —
-- mastering done, artwork handed over, promo live. Overdue is computed, so
-- nothing about it is stored.
CREATE TABLE IF NOT EXISTS release_deadlines (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id          UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    due_date            DATE NOT NULL,
    completed           BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS release_deadlines_release_idx ON release_deadlines(release_id);
CREATE INDEX IF NOT EXISTS release_deadlines_due_date_idx ON release_deadlines(due_date);

-- +goose Down
DROP TABLE IF EXISTS release_deadlines;
