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
func (r *DocumentRepository) CreateWithDetails(ctx context.Context, req CreateDocumentRequest, details CreateDocumentDetails) (*Document, error) {
	const sql = `
		WITH prev AS (
			UPDATE documents
			SET is_current = false
			WHERE owner_type = $1 AND owner_id = $2 AND is_current = true
		)
		INSERT INTO documents (owner_type, owner_id, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,true)
		RETURNING id, owner_type, owner_id, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, created_at
	`
	var d Document
	err := r.db.QueryRow(ctx, sql,
		req.OwnerType, req.OwnerID, details.StorageKey, req.Filename, req.MimeType,
		details.SizeBytes, details.ChecksumSHA256, details.Version, req.UploadedBy,
	).Scan(&d.ID, &d.OwnerType, &d.OwnerID, &d.StorageKey, &d.Filename, &d.MimeType,
		&d.SizeBytes, &d.ChecksumSHA256, &d.Version, &d.UploadedBy, &d.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert document: %w", err)
	}
	return &d, nil
}

// GetCurrent returns the latest version of a document for an owner.
func (r *DocumentRepository) GetCurrent(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) (*Document, error) {
	const sql = `
		SELECT id, owner_type, owner_id, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, created_at
		FROM documents
		WHERE owner_type = $1 AND owner_id = $2 AND is_current = true
	`
	var d Document
	err := r.db.QueryRow(ctx, sql, ownerType, ownerID).Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.StorageKey, &d.Filename, &d.MimeType,
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
		SELECT id, owner_type, owner_id, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, created_at
		FROM documents
		WHERE id = $1
	`
	var d Document
	err := r.db.QueryRow(ctx, sql, id).Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.StorageKey, &d.Filename, &d.MimeType,
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
		SELECT id, owner_type, owner_id, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, created_at
		FROM documents
		WHERE owner_type = $1 AND owner_id = $2
		ORDER BY version DESC
	`
	rows, err := r.db.Query(ctx, sql, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var out []*Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.OwnerType, &d.OwnerID, &d.StorageKey, &d.Filename, &d.MimeType,
			&d.SizeBytes, &d.ChecksumSHA256, &d.Version, &d.UploadedBy, &d.IsCurrent, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}