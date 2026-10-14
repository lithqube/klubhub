-- +goose Up

-- Two live templates cannot share a name (case-insensitive): otherwise the
-- picker offers indistinguishable entries. A soft-deleted template frees its
-- name. The repository maps a violation of this index to a 422.
--
-- Databases that applied 026 may already hold duplicates, which would make the
-- index creation (and so API startup) fail. Keep the oldest template of each
-- name untouched and tag the later ones with a short id suffix, so nothing is
-- deleted and every name stays within the 200-character limit.

UPDATE rider_templates t
   SET name = left(t.name, 188) || ' [' || left(t.id::text, 8) || ']'
  FROM (
        SELECT id,
               row_number() OVER (PARTITION BY lower(name)
                                  ORDER BY created_at, id) AS rn
          FROM rider_templates
         WHERE deleted_at IS NULL
       ) d
 WHERE t.id = d.id AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS rider_templates_name_unique
    ON rider_templates (lower(name)) WHERE deleted_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS rider_templates_name_unique;
