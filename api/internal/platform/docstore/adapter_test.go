package docstore

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestAttachmentSourceAdapter verifies the docstore → email adapter: the
// email service loads attachment metadata and content through
// AttachmentSource{Svc: ...} without docstore importing email's callers
// (email imports nothing from docstore; the adapter lives here).
func TestAttachmentSourceAdapter(t *testing.T) {
	repo, store := newFakeDocumentRepo(), newFakeObjectStore()
	svc := NewDocumentService(repo, store, "bucket")
	src := AttachmentSource{Svc: svc}

	ownerID := uuid.New()
	doc, err := svc.Create(context.Background(), CreateDocumentRequest{
		OwnerType: DocumentOwnerInvoice,
		OwnerID:   ownerID,
		Kind:      DocumentKindInvoicePDF,
		Filename:  "invoice.pdf",
		MimeType:  "application/pdf",
	}, strings.NewReader("%PDF-1.4 fake"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// GetByID maps Document → email.AttachmentDoc.
	got, err := src.GetByID(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != doc.ID || got.OwnerType != "invoice" || got.OwnerID != ownerID ||
		got.Filename != "invoice.pdf" || got.MimeType != "application/pdf" {
		t.Errorf("AttachmentDoc = %+v", got)
	}

	// Download returns the content plus the same metadata.
	rc, att, err := src.Download(context.Background(), doc.ID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	content, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "%PDF-1.4 fake" {
		t.Errorf("content = %q", content)
	}
	if att.ID != doc.ID || att.OwnerType != "invoice" || att.OwnerID != ownerID {
		t.Errorf("download AttachmentDoc = %+v", att)
	}

	// Unknown ids surface an error (the email service refuses the message).
	if _, err := src.GetByID(context.Background(), uuid.New()); err == nil {
		t.Error("GetByID unknown id: no error")
	}
	if _, _, err := src.Download(context.Background(), uuid.New()); err == nil {
		t.Error("Download unknown id: no error")
	}
}
