package finance

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeObjectStore is an in-memory S3 replacement for tests.
type fakeObjectStore struct {
	objects map[string][]byte
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string][]byte)}
}

func (f *fakeObjectStore) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *fakeObjectStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeObjectStore) DeleteObject(ctx context.Context, bucket, key string) error {
	delete(f.objects, key)
	return nil
}

// fakeDocumentRepo is an in-memory replacement for DocumentRepository.
type fakeDocumentRepo struct {
	documents map[uuid.UUID]*Document
}

func newFakeDocumentRepo() *fakeDocumentRepo {
	return &fakeDocumentRepo{documents: make(map[uuid.UUID]*Document)}
}

func (f *fakeDocumentRepo) CreateWithDetails(ctx context.Context, req CreateDocumentRequest, details CreateDocumentDetails) (*Document, error) {
	d := &Document{
		ID:             uuid.New(),
		OwnerType:      req.OwnerType,
		OwnerID:        req.OwnerID,
		StorageKey:     details.StorageKey,
		Filename:       req.Filename,
		MimeType:       req.MimeType,
		SizeBytes:      details.SizeBytes,
		ChecksumSHA256: details.ChecksumSHA256,
		Version:        details.Version,
		UploadedBy:     req.UploadedBy,
		CreatedAt:      time.Now().UTC(),
	}
	f.documents[d.ID] = d
	return d, nil
}

func (f *fakeDocumentRepo) GetCurrent(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) (*Document, error) {
	var latest *Document
	for _, d := range f.documents {
		if d.OwnerType == ownerType && d.OwnerID == ownerID {
			if latest == nil || d.Version > latest.Version {
				latest = d
			}
		}
	}
	if latest == nil {
		return nil, ErrDocumentNotFound
	}
	cp := *latest
	return &cp, nil
}

func (f *fakeDocumentRepo) GetByID(ctx context.Context, id uuid.UUID) (*Document, error) {
	d, ok := f.documents[id]
	if !ok {
		return nil, ErrDocumentNotFound
	}
	cp := *d
	return &cp, nil
}

func (f *fakeDocumentRepo) ListByOwner(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) ([]*Document, error) {
	var out []*Document
	for _, d := range f.documents {
		if d.OwnerType == ownerType && d.OwnerID == ownerID {
			cp := *d
			out = append(out, &cp)
		}
	}
	// Sort by version descending (newest first)
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[i].Version < out[j].Version {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func TestDocumentService_CreateAndGetCurrent(t *testing.T) {
	repo := newFakeDocumentRepo()
	store := newFakeObjectStore()
	svc := NewDocumentService(repo, store, "test-bucket")

	content := []byte("test pdf content")
	req := CreateDocumentRequest{
		OwnerType:  DocumentOwnerInvoice,
		OwnerID:    uuid.New(),
		Filename:   "invoice.pdf",
		MimeType:   "application/pdf",
		UploadedBy: "test-user",
	}

	doc, err := svc.Create(context.Background(), req, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if doc.ID == uuid.Nil {
		t.Fatal("missing id")
	}
	if doc.Version != 1 {
		t.Errorf("version: %d", doc.Version)
	}
	if doc.ChecksumSHA256 == "" {
		t.Error("missing checksum")
	}

	// Get current
	current, err := svc.GetCurrent(context.Background(), req.OwnerType, req.OwnerID)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if current.ID != doc.ID {
		t.Errorf("current id mismatch")
	}

	// Create second version
	content2 := []byte("updated content")
	doc2, err := svc.Create(context.Background(), req, bytes.NewReader(content2))
	if err != nil {
		t.Fatalf("Create v2: %v", err)
	}
	if doc2.Version != 2 {
		t.Errorf("version: %d", doc2.Version)
	}

	// Current should now be v2
	current, err = svc.GetCurrent(context.Background(), req.OwnerType, req.OwnerID)
	if err != nil {
		t.Fatalf("GetCurrent v2: %v", err)
	}
	if current.Version != 2 {
		t.Errorf("current version: %d", current.Version)
	}
}

func TestDocumentService_Download(t *testing.T) {
	repo := newFakeDocumentRepo()
	store := newFakeObjectStore()
	svc := NewDocumentService(repo, store, "test-bucket")

	content := []byte("download test content")
	req := CreateDocumentRequest{
		OwnerType: DocumentOwnerInvoice,
		OwnerID:   uuid.New(),
		Filename:  "test.pdf",
		MimeType:  "application/pdf",
	}

	doc, err := svc.Create(context.Background(), req, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rc, dlDoc, err := svc.Download(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "download test content" {
		t.Errorf("content mismatch: %s", string(data))
	}
	if dlDoc.ID != doc.ID {
		t.Errorf("doc id mismatch")
	}
}

func TestDocumentService_ListByOwner(t *testing.T) {
	repo := newFakeDocumentRepo()
	store := newFakeObjectStore()
	svc := NewDocumentService(repo, store, "test-bucket")

	ownerID := uuid.New()
	for i := 1; i <= 3; i++ {
		req := CreateDocumentRequest{
			OwnerType: DocumentOwnerInvoice,
			OwnerID:   ownerID,
			Filename:  fmt.Sprintf("v%d.pdf", i),
			MimeType:  "application/pdf",
		}
		_, err := svc.Create(context.Background(), req, bytes.NewReader([]byte(fmt.Sprintf("content %d", i))))
		if err != nil {
			t.Fatalf("Create v%d: %v", i, err)
		}
	}

	docs, err := svc.ListByOwner(context.Background(), DocumentOwnerInvoice, ownerID)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("expected 3 docs, got %d", len(docs))
	}
	// Should be newest first
	if docs[0].Version != 3 || docs[1].Version != 2 || docs[2].Version != 1 {
		t.Errorf("versions not in desc order: %d, %d, %d", docs[0].Version, docs[1].Version, docs[2].Version)
	}
}

func TestDocumentValidation_RejectsBadInput(t *testing.T) {
	req := CreateDocumentRequest{
		OwnerType:  DocumentOwnerType("alien"),
		OwnerID:    uuid.Nil,
		Filename:   "",
		MimeType:   "",
		UploadedBy: "u",
	}
	err := ValidateCreateDocumentRequest(req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}