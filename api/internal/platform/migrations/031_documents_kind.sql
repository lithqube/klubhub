-- +goose Up

-- A document now has a kind, and versions and "current" are kept per kind.
-- Until now an owner had one stream of versions, so an invoice's PDF, its
-- e-invoice XML and its validation report would each have replaced the last.
--
--   general            what the table held before: agreement PDFs and the like
--   invoice_pdf        the invoice or credit note PDF as issued
--   einvoice_xml       the validated EN 16931 (CII) XML of that invoice
--   validation_report  the rule-engine result for that XML (JSON)
ALTER TABLE documents
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'general'
        CHECK (kind IN ('general', 'invoice_pdf', 'einvoice_xml', 'validation_report'));

DROP INDEX IF EXISTS documents_owner_current_idx;
DROP INDEX IF EXISTS documents_owner_version_idx;
DROP INDEX IF EXISTS documents_owner_idx;

CREATE UNIQUE INDEX documents_owner_kind_current_idx
    ON documents (owner_type, owner_id, kind) WHERE is_current = true;
CREATE UNIQUE INDEX documents_owner_kind_version_idx
    ON documents (owner_type, owner_id, kind, version);
CREATE INDEX documents_owner_idx
    ON documents (owner_type, owner_id, kind, version DESC);

-- Archived documents are evidence: the only change a row may ever see is
-- losing is_current when a newer version replaces it, and rows are never
-- deleted. Enforced here so no code path, present or future, can bypass it.
-- +goose StatementBegin
CREATE FUNCTION documents_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'documents are immutable: rows cannot be deleted';
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.owner_type IS DISTINCT FROM OLD.owner_type
        OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
        OR NEW.kind IS DISTINCT FROM OLD.kind
        OR NEW.storage_key IS DISTINCT FROM OLD.storage_key
        OR NEW.filename IS DISTINCT FROM OLD.filename
        OR NEW.mime_type IS DISTINCT FROM OLD.mime_type
        OR NEW.size_bytes IS DISTINCT FROM OLD.size_bytes
        OR NEW.checksum_sha256 IS DISTINCT FROM OLD.checksum_sha256
        OR NEW.version IS DISTINCT FROM OLD.version
        OR NEW.uploaded_by IS DISTINCT FROM OLD.uploaded_by
        OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'documents are immutable: only is_current may change';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER documents_immutable
    BEFORE UPDATE OR DELETE ON documents
    FOR EACH ROW EXECUTE FUNCTION documents_immutable();

-- +goose Down
DROP TRIGGER IF EXISTS documents_immutable ON documents;
DROP FUNCTION IF EXISTS documents_immutable();
DROP INDEX IF EXISTS documents_owner_idx;
DROP INDEX IF EXISTS documents_owner_kind_version_idx;
DROP INDEX IF EXISTS documents_owner_kind_current_idx;
-- Fails if an owner already has more than one kind: archived evidence is not
-- deleted to make a rollback possible.
CREATE UNIQUE INDEX documents_owner_current_idx ON documents (owner_type, owner_id) WHERE is_current = true;
CREATE UNIQUE INDEX documents_owner_version_idx ON documents (owner_type, owner_id, version);
CREATE INDEX documents_owner_idx ON documents (owner_type, owner_id, version DESC);
ALTER TABLE documents DROP COLUMN kind;
