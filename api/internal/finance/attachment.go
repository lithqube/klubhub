package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Receipt limits. A phone photo is typically 2-8 MB and a scanned PDF a few
// more, so 15 MB leaves headroom without letting one request fill the disk.
const (
	MaxAttachmentBytes     = 15 << 20
	MaxAttachmentsPerEntry = 10
	maxAttachmentFilename  = 200
)

// attachmentTypes is the closed set of stored file types. The type is read
// from the file's own bytes (http.DetectContentType); the browser's claim and
// the file name are never trusted. SVG, HTML and executables are not in it.
var attachmentTypes = map[string]struct{}{
	"image/jpeg": {}, "image/png": {}, "image/webp": {}, "application/pdf": {},
}

// EntryAttachment is what clients see. The storage key stays server-side.
type EntryAttachment struct {
	ID             uuid.UUID `json:"id"`
	EntryID        uuid.UUID `json:"entry_id"`
	Filename       string    `json:"filename"`
	MimeType       string    `json:"mime_type"`
	SizeBytes      int64     `json:"size_bytes"`
	ChecksumSHA256 string    `json:"checksum_sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

// StoredAttachment adds the object key for the service and repository.
type StoredAttachment struct {
	EntryAttachment
	StorageKey string `json:"-"`
}

// NewAttachment is a verified, already-uploaded file ready to be recorded.
type NewAttachment struct {
	EntryID        uuid.UUID
	StorageKey     string
	Filename       string
	MimeType       string
	SizeBytes      int64
	ChecksumSHA256 string
}

var (
	ErrAttachmentNotFound   = errors.New("attachment not found")
	ErrAttachmentType       = errors.New("unsupported attachment type")
	ErrAttachmentTooLarge   = errors.New("attachment too large")
	ErrAttachmentLimit      = errors.New("attachment limit reached")
	ErrAttachmentValidation = errors.New("invalid attachment")
)

// AttachmentRepositoryIface is what the service needs from persistence.
type AttachmentRepositoryIface interface {
	// Ensure reports whether the entry exists (ErrEntryNotFound) and is still
	// active (ErrEntryInactive).
	Ensure(ctx context.Context, entryID uuid.UUID) error
	// Create records an uploaded file under a lock on the entry so concurrent
	// uploads cannot exceed MaxAttachmentsPerEntry (ErrAttachmentLimit).
	Create(ctx context.Context, in NewAttachment) (*StoredAttachment, error)
	List(ctx context.Context, entryID uuid.UUID) ([]*EntryAttachment, error)
	// Get is scoped to the entry: another entry's attachment id is not found.
	Get(ctx context.Context, entryID, id uuid.UUID) (*StoredAttachment, error)
	// Delete removes the record and returns the storage key to delete. A
	// voided entry's evidence cannot be removed (ErrEntryInactive).
	Delete(ctx context.Context, entryID, id uuid.UUID) (string, error)
}

// AttachmentService stores receipt files in the object store and records them.
type AttachmentService struct {
	repo   AttachmentRepositoryIface
	store  ObjectStore
	bucket string
}

// NewAttachmentService returns an AttachmentService.
func NewAttachmentService(repo AttachmentRepositoryIface, store ObjectStore, bucket string) *AttachmentService {
	return &AttachmentService{repo: repo, store: store, bucket: bucket}
}

// Add verifies and stores one file. Nothing is written for a file that fails a
// check, and the object is removed again if recording it fails.
func (s *AttachmentService) Add(ctx context.Context, entryID uuid.UUID, filename string, content io.ReadSeeker) (*EntryAttachment, error) {
	if err := s.repo.Ensure(ctx, entryID); err != nil {
		return nil, err
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(content, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read attachment: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("%w: the file is empty", ErrAttachmentValidation)
	}
	mimeType, _, _ := strings.Cut(http.DetectContentType(head[:n]), ";")
	if _, ok := attachmentTypes[mimeType]; !ok {
		return nil, fmt.Errorf("%w: use a JPEG, PNG, WebP or PDF file", ErrAttachmentType)
	}

	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind attachment: %w", err)
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, io.LimitReader(content, MaxAttachmentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("hash attachment: %w", err)
	}
	if size > MaxAttachmentBytes {
		return nil, fmt.Errorf("%w: the limit is %d MB per file", ErrAttachmentTooLarge, MaxAttachmentBytes>>20)
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind attachment: %w", err)
	}

	key := fmt.Sprintf("finance/entries/%s/%s", entryID, uuid.NewString())
	if err := s.store.PutObject(ctx, s.bucket, key, io.LimitReader(content, size), size, mimeType); err != nil {
		return nil, fmt.Errorf("store attachment: %w", err)
	}
	rec, err := s.repo.Create(ctx, NewAttachment{
		EntryID: entryID, StorageKey: key, Filename: sanitizeAttachmentFilename(filename),
		MimeType: mimeType, SizeBytes: size, ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
	})
	if err != nil {
		_ = s.store.DeleteObject(ctx, s.bucket, key) // do not leave an orphan behind
		return nil, err
	}
	return &rec.EntryAttachment, nil
}

// List returns an entry's attachments, oldest first.
func (s *AttachmentService) List(ctx context.Context, entryID uuid.UUID) ([]*EntryAttachment, error) {
	return s.repo.List(ctx, entryID)
}

// Open returns the stored bytes and metadata of one attachment.
func (s *AttachmentService) Open(ctx context.Context, entryID, id uuid.UUID) (io.ReadCloser, *EntryAttachment, error) {
	rec, err := s.repo.Get(ctx, entryID, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.store.GetObject(ctx, s.bucket, rec.StorageKey)
	if err != nil {
		return nil, nil, fmt.Errorf("read attachment: %w", err)
	}
	return rc, &rec.EntryAttachment, nil
}

// Remove deletes an attachment of an active entry: the record first, then the
// object. A leftover object (storage error) is harmless; a record pointing at
// a missing object is not.
func (s *AttachmentService) Remove(ctx context.Context, entryID, id uuid.UUID) error {
	key, err := s.repo.Delete(ctx, entryID, id)
	if err != nil {
		return err
	}
	_ = s.store.DeleteObject(ctx, s.bucket, key)
	return nil
}

// sanitizeAttachmentFilename keeps only a display name: no directories, no
// control characters, at most maxAttachmentFilename characters. The name is
// never used as a storage path.
func sanitizeAttachmentFilename(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || name == "/" {
		return "receipt"
	}
	if utf8.RuneCountInString(name) > maxAttachmentFilename {
		runes := []rune(name)
		name = string(runes[:maxAttachmentFilename])
	}
	return name
}
