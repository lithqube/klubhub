package finance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DocumentRepository persists Document rows.
type DocumentRepository struct {
	db *pgxpool.Pool
}

// NewDocumentRepository creates a DocumentRepository.
func NewDocumentRepository(db *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{db: db}
}

var _ DocumentRepositoryIface = (*DocumentRepository)(nil)

// CreateWithDetails inserts a new document row and marks it as current.
// The previous current document for the same owner is marked as not current.
//
// The version number is computed inside the insert (COALESCE(MAX(version),0)+1
// scoped to the owner and kind) rather than being read-then-written by the caller, closing
// a race where two concurrent uploads for the same owner could both observe the
// same "next version" and attempt to insert it. A UNIQUE (owner_type, owner_id,
// kind, version) index (see migration 031) turns any surviving race into a Postgres
// unique violation, which callers should map via isUniqueViolation.
func (r *DocumentRepository) CreateWithDetails(ctx context.Context, req CreateDocumentRequest, details CreateDocumentDetails) (*Document, error) {
	// Two statements in one transaction, not one statement with CTEs: Postgres
	// does not define the order a data-modifying CTE and the main query run
	// in, so a single statement could insert the new current row before the
	// old one had been demoted and trip the one-current-per-kind index.
	const demote = `
		UPDATE documents SET is_current = false
		WHERE owner_type = $1 AND owner_id = $2 AND kind = $3 AND is_current = true
	`
	const insert = `
		INSERT INTO documents (owner_type, owner_id, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, kind)
		SELECT $1, $2, $3, $4, $5, $6, $7, COALESCE(MAX(version), 0) + 1, $8, true, $9
		FROM documents
		WHERE owner_type = $1 AND owner_id = $2 AND kind = $9
		RETURNING id, owner_type, owner_id, kind, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, created_at
	`
	kind := req.kindOrGeneral()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, demote, req.OwnerType, req.OwnerID, kind); err != nil {
		return nil, fmt.Errorf("demote previous document: %w", err)
	}
	var d Document
	err = tx.QueryRow(ctx, insert,
		req.OwnerType, req.OwnerID, details.StorageKey, req.Filename, req.MimeType,
		details.SizeBytes, details.ChecksumSHA256, req.UploadedBy, kind,
	).Scan(&d.ID, &d.OwnerType, &d.OwnerID, &d.Kind, &d.StorageKey, &d.Filename, &d.MimeType,
		&d.SizeBytes, &d.ChecksumSHA256, &d.Version, &d.UploadedBy, &d.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDocumentConflict
		}
		return nil, fmt.Errorf("insert document: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDocumentConflict
		}
		return nil, fmt.Errorf("commit document: %w", err)
	}
	d.IsCurrent = true
	return &d, nil
}

// GetCurrent returns the latest version of a document for an owner.
func (r *DocumentRepository) GetCurrent(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) (*Document, error) {
	const sql = `
		SELECT id, owner_type, owner_id, kind, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, created_at
		FROM documents
		WHERE owner_type = $1 AND owner_id = $2 AND kind = 'general' AND is_current = true
	`
	var d Document
	err := r.db.QueryRow(ctx, sql, ownerType, ownerID).Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.Kind, &d.StorageKey, &d.Filename, &d.MimeType,
		&d.SizeBytes, &d.ChecksumSHA256, &d.Version, &d.UploadedBy, &d.IsCurrent, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, fmt.Errorf("get current document: %w", err)
	}
	return &d, nil
}

// GetByID returns a specific document version.
func (r *DocumentRepository) GetByID(ctx context.Context, id uuid.UUID) (*Document, error) {
	const sql = `
		SELECT id, owner_type, owner_id, kind, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, created_at
		FROM documents
		WHERE id = $1
	`
	var d Document
	err := r.db.QueryRow(ctx, sql, id).Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.Kind, &d.StorageKey, &d.Filename, &d.MimeType,
		&d.SizeBytes, &d.ChecksumSHA256, &d.Version, &d.UploadedBy, &d.IsCurrent, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, fmt.Errorf("get document by id: %w", err)
	}
	return &d, nil
}

// ListByOwner returns all versions for an owner, newest first.
func (r *DocumentRepository) ListByOwner(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) ([]*Document, error) {
	const sql = `
		SELECT id, owner_type, owner_id, kind, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, created_at
		FROM documents
		WHERE owner_type = $1 AND owner_id = $2
		ORDER BY created_at DESC, kind, version DESC
	`
	rows, err := r.db.Query(ctx, sql, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var out []*Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.OwnerType, &d.OwnerID, &d.Kind, &d.StorageKey, &d.Filename, &d.MimeType,
			&d.SizeBytes, &d.ChecksumSHA256, &d.Version, &d.UploadedBy, &d.IsCurrent, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}
