package docstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestIsUniqueViolation covers the SQLSTATE classifier directly.
func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(&pgconn.PgError{Code: pgUniqueViolation}) {
		t.Error("23505 not detected")
	}
	if isUniqueViolation(&pgconn.PgError{Code: "23503"}) {
		t.Error("foreign-key violation misclassified")
	}
	if isUniqueViolation(errors.New("boom")) {
		t.Error("generic error misclassified")
	}
	if isUniqueViolation(nil) {
		t.Error("nil misclassified")
	}
}

// TestIntegration_DocumentRepository_NotFoundPaths covers the read paths
// that return typed errors.
func TestIntegration_DocumentRepository_NotFoundPaths(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewDocumentRepository(testPool)
	ownerID := uuid.New()

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("GetByID missing = %v, want ErrDocumentNotFound", err)
	}
	if _, err := repo.GetCurrent(ctx, DocumentOwnerInvoice, ownerID); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("GetCurrent missing = %v, want ErrDocumentNotFound", err)
	}
	list, err := repo.ListByOwner(ctx, DocumentOwnerInvoice, ownerID)
	if err != nil || len(list) != 0 {
		t.Errorf("ListByOwner empty = %+v, %v", list, err)
	}
}

// TestIntegration_DocumentRepository_VersionSequencing covers the
// repository-level versioning contract: versions increment per owner+kind,
// only the newest is current, and reads see the demotion.
func TestIntegration_DocumentRepository_VersionSequencing(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewDocumentRepository(testPool)
	ownerID := uuid.New()
	mkReq := func(name string) CreateDocumentRequest {
		return CreateDocumentRequest{
			OwnerType: DocumentOwnerInvoice, OwnerID: ownerID,
			Kind: DocumentKindInvoicePDF, Filename: name, MimeType: "application/pdf",
		}
	}
	mkDetails := func(key string) CreateDocumentDetails {
		return CreateDocumentDetails{StorageKey: key, SizeBytes: 3, ChecksumSHA256: "abc"}
	}

	v1, err := repo.CreateWithDetails(ctx, mkReq("v1.pdf"), mkDetails("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || !v1.IsCurrent {
		t.Fatalf("v1 = %+v", v1)
	}
	v2, err := repo.CreateWithDetails(ctx, mkReq("v2.pdf"), mkDetails("k2"))
	if err != nil {
		t.Fatal(err)
	}
	if v2.Version != 2 || !v2.IsCurrent {
		t.Fatalf("v2 = %+v", v2)
	}

	// The previous version is demoted but retained.
	old, err := repo.GetByID(ctx, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if old.IsCurrent {
		t.Error("v1 still current after v2")
	}
	// GetCurrent reads the 'general' kind stream only; this owner holds just
	// invoice_pdf documents, so it must report not found.
	if _, err := repo.GetCurrent(ctx, DocumentOwnerInvoice, ownerID); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("GetCurrent invoice-only owner = %v, want ErrDocumentNotFound", err)
	}
	list, err := repo.ListByOwner(ctx, DocumentOwnerInvoice, ownerID)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListByOwner = %+v, %v", list, err)
	}
	if list[0].Version != 2 || list[1].Version != 1 {
		t.Errorf("ListByOwner order = v%d, v%d", list[0].Version, list[1].Version)
	}
	if !list[0].IsCurrent || list[1].IsCurrent {
		t.Errorf("ListByOwner currency = %v, %v, want true, false", list[0].IsCurrent, list[1].IsCurrent)
	}
}

// TestIntegration_DocumentRepository_UniqueViolationMapsToConflict forces
// the version race deterministically: a second transaction inserts the
// version the repository is about to compute and stays uncommitted, so the
// repository's insert blocks on the unique index and, once the competitor
// commits, surfaces 23505 — which must map to ErrDocumentConflict.
func TestIntegration_DocumentRepository_UniqueViolationMapsToConflict(t *testing.T) {
	requirePG(t)
	ctx := context.Background()
	repo := NewDocumentRepository(testPool)
	ownerID := uuid.New()

	// Seed version 1 through the repository.
	if _, err := repo.CreateWithDetails(ctx, CreateDocumentRequest{
		OwnerType: DocumentOwnerInvoice, OwnerID: ownerID,
		Filename: "a.pdf", MimeType: "application/pdf",
	}, CreateDocumentDetails{StorageKey: "k1", SizeBytes: 1, ChecksumSHA256: "x"}); err != nil {
		t.Fatal(err)
	}

	// Competitor: insert version 2 (the version the repository will compute)
	// and hold the transaction open.
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO documents (owner_type, owner_id, storage_key, filename, mime_type,
			size_bytes, checksum_sha256, version, uploaded_by, is_current, kind)
		VALUES ($1, $2, 'k2', 'b.pdf', 'application/pdf', 1, 'y', 2, 'system', false, 'general')
	`, DocumentOwnerInvoice, ownerID); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := repo.CreateWithDetails(ctx, CreateDocumentRequest{
			OwnerType: DocumentOwnerInvoice, OwnerID: ownerID,
			Filename: "c.pdf", MimeType: "application/pdf",
		}, CreateDocumentDetails{StorageKey: "k3", SizeBytes: 1, ChecksumSHA256: "z"})
		errCh <- err
	}()

	// Wait until the repository's insert is blocked on the unique index. A
	// unique-conflict wait is a wait on the competitor's transaction, not a
	// relation lock, so pg_locks cannot see it; poll pg_stat_activity for the
	// in-flight INSERT instead. Then commit the competitor so the waiter
	// wakes with a violation.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var blocked int
		if err := testPool.QueryRow(ctx, `
			SELECT count(*) FROM pg_stat_activity
			WHERE state = 'active'
			  AND query LIKE '%INSERT INTO documents%'
			  AND pid <> pg_backend_pid()
		`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("repository insert never blocked on the unique index")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrDocumentConflict) {
			t.Fatalf("err = %v, want ErrDocumentConflict", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CreateWithDetails did not return after the competing commit")
	}
}
