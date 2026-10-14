package finance

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func putDoc(t *testing.T, svc *DocumentService, owner uuid.UUID, kind DocumentKind, body string) *Document {
	t.Helper()
	d, err := svc.Create(context.Background(), CreateDocumentRequest{
		OwnerType: DocumentOwnerInvoice, OwnerID: owner, Kind: kind, Filename: "f", MimeType: "application/pdf",
	}, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("create %s: %v", kind, err)
	}
	return d
}

func TestIntegration_Documents_KindsKeepSeparateVersions(t *testing.T) {
	requireEntryPG(t)
	svc := NewDocumentService(NewDocumentRepository(testPool), newFakeObjectStore(), "b")
	owner := uuid.New()

	pdf := putDoc(t, svc, owner, DocumentKindInvoicePDF, "pdf")
	xml := putDoc(t, svc, owner, DocumentKindEInvoiceXML, "xml")
	if pdf.Version != 1 || xml.Version != 1 || pdf.Kind != DocumentKindInvoicePDF || xml.Kind != DocumentKindEInvoiceXML {
		t.Fatalf("each kind starts at version 1: pdf=%d/%s xml=%d/%s", pdf.Version, pdf.Kind, xml.Version, xml.Kind)
	}
	pdf2 := putDoc(t, svc, owner, DocumentKindInvoicePDF, "pdf2")
	if pdf2.Version != 2 {
		t.Fatalf("second pdf = v%d", pdf2.Version)
	}

	list, err := svc.ListByOwner(context.Background(), DocumentOwnerInvoice, owner)
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	current := map[DocumentKind]int{}
	for _, d := range list {
		if d.IsCurrent {
			current[d.Kind]++
		}
	}
	if current[DocumentKindInvoicePDF] != 1 || current[DocumentKindEInvoiceXML] != 1 {
		t.Fatalf("one current per kind, got %v", current)
	}
}

func TestIntegration_Documents_GeneralKeepsItsOldBehaviour(t *testing.T) {
	requireEntryPG(t)
	svc := NewDocumentService(NewDocumentRepository(testPool), newFakeObjectStore(), "b")
	owner := uuid.New()
	putDoc(t, svc, owner, "", "a")
	putDoc(t, svc, owner, DocumentKindInvoicePDF, "x")
	b := putDoc(t, svc, owner, DocumentKindGeneral, "b")
	cur, err := svc.GetCurrent(context.Background(), DocumentOwnerInvoice, owner)
	if err != nil || cur.ID != b.ID || cur.Version != 2 {
		t.Fatalf("GetCurrent = %+v, %v; want the second general document", cur, err)
	}
}

func TestIntegration_Documents_AreImmutable(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	svc := NewDocumentService(NewDocumentRepository(testPool), newFakeObjectStore(), "b")
	d := putDoc(t, svc, uuid.New(), DocumentKindInvoicePDF, "x")

	for name, sql := range map[string]string{
		"delete":        `DELETE FROM documents WHERE id = $1`,
		"rename":        `UPDATE documents SET filename = 'evil' WHERE id = $1`,
		"swap content":  `UPDATE documents SET checksum_sha256 = 'x' WHERE id = $1`,
		"change kind":   `UPDATE documents SET kind = 'general' WHERE id = $1`,
		"repoint bytes": `UPDATE documents SET storage_key = 'other' WHERE id = $1`,
	} {
		_, err := testPool.Exec(ctx, sql, d.ID)
		if err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Errorf("%s: err = %v, want an immutability error", name, err)
		}
	}
	// Losing is_current to a newer version is the one allowed change.
	putDoc(t, svc, d.OwnerID, DocumentKindInvoicePDF, "y")
	old, err := svc.GetByID(ctx, d.ID)
	if err != nil || old.IsCurrent {
		t.Fatalf("old version = %+v, %v; should have lost is_current", old, err)
	}
}

func TestIntegration_Documents_UnknownKindRejected(t *testing.T) {
	requireEntryPG(t)
	svc := NewDocumentService(NewDocumentRepository(testPool), newFakeObjectStore(), "b")
	_, err := svc.Create(context.Background(), CreateDocumentRequest{
		OwnerType: DocumentOwnerInvoice, OwnerID: uuid.New(), Kind: "nonsense", Filename: "f", MimeType: "x",
	}, strings.NewReader("x"))
	if err == nil {
		t.Fatal("unknown kind accepted")
	}
}
