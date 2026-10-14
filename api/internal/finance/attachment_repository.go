package finance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AttachmentRepository persists receipt metadata in finance_entry_attachments.
type AttachmentRepository struct{ pool *pgxpool.Pool }

// NewAttachmentRepository returns an AttachmentRepository.
func NewAttachmentRepository(pool *pgxpool.Pool) *AttachmentRepository {
	return &AttachmentRepository{pool: pool}
}

const attachmentColumns = `id, entry_id, storage_key, filename, mime_type, size_bytes, checksum_sha256, created_at`

func scanAttachment(row pgx.Row) (*StoredAttachment, error) {
	var a StoredAttachment
	err := row.Scan(&a.ID, &a.EntryID, &a.StorageKey, &a.Filename, &a.MimeType, &a.SizeBytes, &a.ChecksumSHA256, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// entryStatusOf reads the status of a live (not soft-deleted) entry. When lock
// is true the row is locked for the rest of the transaction, which serialises
// uploads to one entry so the per-entry count cannot be raced past.
func entryStatusOf(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, entryID uuid.UUID, lock bool) (EntryStatus, error) {
	sql := `SELECT status FROM finance_entries WHERE id = $1 AND deleted_at IS NULL`
	if lock {
		sql += ` FOR UPDATE`
	}
	var status EntryStatus
	if err := q.QueryRow(ctx, sql, entryID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrEntryNotFound
		}
		return "", fmt.Errorf("read entry status: %w", err)
	}
	return status, nil
}

// Ensure implements AttachmentRepositoryIface.
func (r *AttachmentRepository) Ensure(ctx context.Context, entryID uuid.UUID) error {
	status, err := entryStatusOf(ctx, r.pool, entryID, false)
	if err != nil {
		return err
	}
	if status != EntryStatusActive {
		return ErrEntryInactive
	}
	return nil
}

// Create implements AttachmentRepositoryIface.
func (r *AttachmentRepository) Create(ctx context.Context, in NewAttachment) (*StoredAttachment, error) {
	var out *StoredAttachment
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		status, err := entryStatusOf(ctx, tx, in.EntryID, true)
		if err != nil {
			return err
		}
		if status != EntryStatusActive {
			return ErrEntryInactive
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM finance_entry_attachments WHERE entry_id = $1`, in.EntryID).Scan(&n); err != nil {
			return fmt.Errorf("count attachments: %w", err)
		}
		if n >= MaxAttachmentsPerEntry {
			return ErrAttachmentLimit
		}
		out, err = scanAttachment(tx.QueryRow(ctx, `
			INSERT INTO finance_entry_attachments (entry_id, storage_key, filename, mime_type, size_bytes, checksum_sha256)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING `+attachmentColumns,
			in.EntryID, in.StorageKey, in.Filename, in.MimeType, in.SizeBytes, in.ChecksumSHA256))
		if err != nil {
			return fmt.Errorf("insert attachment: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List implements AttachmentRepositoryIface. A voided entry still lists its
// evidence; a soft-deleted or unknown entry is not found.
func (r *AttachmentRepository) List(ctx context.Context, entryID uuid.UUID) ([]*EntryAttachment, error) {
	if _, err := entryStatusOf(ctx, r.pool, entryID, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+attachmentColumns+` FROM finance_entry_attachments
		WHERE entry_id = $1 ORDER BY created_at, id`, entryID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()
	out := []*EntryAttachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		out = append(out, &a.EntryAttachment)
	}
	return out, rows.Err()
}

// Get implements AttachmentRepositoryIface.
func (r *AttachmentRepository) Get(ctx context.Context, entryID, id uuid.UUID) (*StoredAttachment, error) {
	a, err := scanAttachment(r.pool.QueryRow(ctx, `
		SELECT a.id, a.entry_id, a.storage_key, a.filename, a.mime_type, a.size_bytes, a.checksum_sha256, a.created_at
		FROM finance_entry_attachments a
		JOIN finance_entries e ON e.id = a.entry_id AND e.deleted_at IS NULL
		WHERE a.id = $1 AND a.entry_id = $2`, id, entryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttachmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get attachment: %w", err)
	}
	return a, nil
}

// Delete implements AttachmentRepositoryIface.
func (r *AttachmentRepository) Delete(ctx context.Context, entryID, id uuid.UUID) (string, error) {
	var key string
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		status, err := entryStatusOf(ctx, tx, entryID, true)
		if err != nil {
			return err
		}
		if status != EntryStatusActive {
			return ErrEntryInactive
		}
		err = tx.QueryRow(ctx, `DELETE FROM finance_entry_attachments WHERE id = $1 AND entry_id = $2 RETURNING storage_key`,
			id, entryID).Scan(&key)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAttachmentNotFound
		}
		if err != nil {
			return fmt.Errorf("delete attachment: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

var _ AttachmentRepositoryIface = (*AttachmentRepository)(nil)
