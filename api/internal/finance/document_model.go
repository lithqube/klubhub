package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

// DocumentOwnerType enumerates the entity types that can own documents.
type DocumentOwnerType string

const (
	DocumentOwnerInvoice   DocumentOwnerType = "invoice"
	DocumentOwnerAgreement DocumentOwnerType = "agreement"
	DocumentOwnerEPK       DocumentOwnerType = "epk"
	DocumentOwnerGig       DocumentOwnerType = "gig"
	DocumentOwnerOther     DocumentOwnerType = "other"
)

var validDocumentOwnerTypes = map[DocumentOwnerType]struct{}{
	DocumentOwnerInvoice:   {},
	DocumentOwnerAgreement: {},
	DocumentOwnerEPK:       {},
	DocumentOwnerGig:       {},
	DocumentOwnerOther:     {},
}

func (t DocumentOwnerType) IsValid() bool {
	_, ok := validDocumentOwnerTypes[t]
	return ok
}

// Document represents a stored file (PDF, image, etc.) in Garage S3.
type Document struct {
	ID             uuid.UUID          `json:"id"                db:"id"`
	OwnerType      DocumentOwnerType  `json:"owner_type"        db:"owner_type"`
	OwnerID        uuid.UUID          `json:"owner_id"          db:"owner_id"`
	StorageKey     string             `json:"storage_key"       db:"storage_key"`
	Filename       string             `json:"filename"          db:"filename"`
	MimeType       string             `json:"mime_type"         db:"mime_type"`
	SizeBytes      int64              `json:"size_bytes"        db:"size_bytes"`
	ChecksumSHA256 string             `json:"checksum_sha256"   db:"checksum_sha256"`
	Version        int                `json:"version"           db:"version"`
	UploadedBy     string             `json:"uploaded_by"       db:"uploaded_by"`
	IsCurrent      bool               `json:"is_current"        db:"is_current"`
	CreatedAt      time.Time          `json:"created_at"        db:"created_at"`
}

var (
	ErrDocumentNotFound = errors.New("document not found")
	ErrDocumentConflict = errors.New("document updated by another writer")
	ErrDocumentValidation = errors.New("invalid document")
)

// CreateDocumentRequest is the body of POST /api/v1/finance/documents.
type CreateDocumentRequest struct {
	OwnerType  DocumentOwnerType `json:"owner_type"`
	OwnerID    uuid.UUID         `json:"owner_id"`
	Filename   string            `json:"filename"`
	MimeType   string            `json:"mime_type"`
	UploadedBy string            `json:"uploaded_by,omitempty"`
}

// CreateDocumentDetails carries computed fields from the service to the repo.
type CreateDocumentDetails struct {
	StorageKey     string
	SizeBytes      int64
	ChecksumSHA256 string
	Version        int
}

// DocumentRepositoryIface is the subset the service uses.
type DocumentRepositoryIface interface {
	CreateWithDetails(ctx context.Context, req CreateDocumentRequest, details CreateDocumentDetails) (*Document, error)
	GetCurrent(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) (*Document, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Document, error)
	ListByOwner(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) ([]*Document, error)
}

// DocumentService coordinates document storage via Garage S3.
type DocumentService struct {
	repo   DocumentRepositoryIface
	store  ObjectStore // Garage S3 client
	bucket string
}

// ObjectStore is the interface for S3-compatible storage (Garage).
type ObjectStore interface {
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, bucket, key string) error
}

// NewDocumentService returns a DocumentService.
func NewDocumentService(repo DocumentRepositoryIface, store ObjectStore, bucket string) *DocumentService {
	return &DocumentService{repo: repo, store: store, bucket: bucket}
}

// Create uploads content to Garage S3 and records the document.
// The version is auto-incremented per owner.
func (s *DocumentService) Create(ctx context.Context, req CreateDocumentRequest, content io.Reader) (*Document, error) {
	if err := ValidateCreateDocumentRequest(req); err != nil {
		return nil, err
	}

	// Compute checksum and size from content
	hasher := sha256.New()
	size, err := io.Copy(hasher, content)
	if err != nil {
		return nil, fmt.Errorf("compute checksum: %w", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	_ = checksum // used in repo.Create

	// Rewind content for upload
	_, err = content.(io.Seeker).Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("seek content: %w", err)
	}

	// Determine next version
	existing, _ := s.repo.GetCurrent(ctx, req.OwnerType, req.OwnerID)
	version := 1
	if existing != nil {
		version = existing.Version + 1
	}

	// Storage key: owner_type/owner_id/version-uuid.ext
	key := fmt.Sprintf("%s/%s/v%d-%s", req.OwnerType, req.OwnerID, version, uuid.New().String())

	// Upload to Garage S3
	if err := s.store.PutObject(ctx, s.bucket, key, content, size, req.MimeType); err != nil {
		return nil, fmt.Errorf("put object: %w", err)
	}

	// Record in DB
	details := CreateDocumentDetails{
		StorageKey:     key,
		SizeBytes:      size,
		ChecksumSHA256: checksum,
		Version:        version,
	}
	return s.repo.CreateWithDetails(ctx, req, details)
}

// GetCurrent returns the latest version of a document for an owner.
func (s *DocumentService) GetCurrent(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) (*Document, error) {
	return s.repo.GetCurrent(ctx, ownerType, ownerID)
}

// GetByID returns a specific document version.
func (s *DocumentService) GetByID(ctx context.Context, id uuid.UUID) (*Document, error) {
	return s.repo.GetByID(ctx, id)
}

// ListByOwner returns all versions for an owner, newest first.
func (s *DocumentService) ListByOwner(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) ([]*Document, error) {
	return s.repo.ListByOwner(ctx, ownerType, ownerID)
}

// Download streams a document from Garage S3.
func (s *DocumentService) Download(ctx context.Context, id uuid.UUID) (io.ReadCloser, *Document, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.store.GetObject(ctx, s.bucket, doc.StorageKey)
	if err != nil {
		return nil, nil, fmt.Errorf("get object: %w", err)
	}
	return rc, doc, nil
}

// ValidateCreateDocumentRequest validates a CreateDocumentRequest.
func ValidateCreateDocumentRequest(req CreateDocumentRequest) error {
	if !req.OwnerType.IsValid() {
		return fmt.Errorf("%w: owner_type must be one of invoice, agreement, epk, gig, other", ErrDocumentValidation)
	}
	if req.OwnerID == uuid.Nil {
		return fmt.Errorf("%w: owner_id is required", ErrDocumentValidation)
	}
	if req.Filename == "" {
		return fmt.Errorf("%w: filename is required", ErrDocumentValidation)
	}
	if req.MimeType == "" {
		return fmt.Errorf("%w: mime_type is required", ErrDocumentValidation)
	}
	return nil
}

// DownloadDocumentRequest is the query for GET /api/v1/finance/documents/{id}/download.
type DownloadDocumentRequest struct{}

var _ = uuid.Nil