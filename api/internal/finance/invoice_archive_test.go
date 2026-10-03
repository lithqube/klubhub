package finance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/einvoice"
)

type archiveHarness struct {
	*einvoiceHarness
	docs  *DocumentService
	repo  *fakeDocumentRepo
	store *fakeObjectStore
}

func newArchiveHarness(t *testing.T) *archiveHarness {
	t.Helper()
	hs := newEInvoiceHarness(t)
	repo, store := newFakeDocumentRepo(), newFakeObjectStore()
	docs := NewDocumentService(repo, store, "b")
	hs.svc.WithArchive(docs)
	return &archiveHarness{einvoiceHarness: hs, docs: docs, repo: repo, store: store}
}

func (a *archiveHarness) kinds(id uuid.UUID) map[DocumentKind]*Document {
	list, err := a.docs.ListByOwner(context.Background(), DocumentOwnerInvoice, id)
	if err != nil {
		a.t.Fatal(err)
	}
	out := map[DocumentKind]*Document{}
	for _, d := range list {
		out[d.Kind] = d
	}
	return out
}

func (a *archiveHarness) content(d *Document) string {
	rc, err := a.store.GetObject(context.Background(), "b", d.StorageKey)
	if err != nil {
		a.t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	return string(b)
}

// issueViaService issues through the service, the path that archives.
func (a *archiveHarness) issueViaService(over map[string]any) *Invoice {
	a.t.Helper()
	draft := a.createDraft("")
	cust := *completeCustomer()
	cust.Email = "buchhaltung@club-hamburg.example"
	body := map[string]any{"customer": cust, "buyer_reference": "04011000-12345-34"}
	for k, v := range over {
		body[k] = v
	}
	if code, out := a.putDraft(draft, body); code != 200 {
		a.t.Fatalf("put: %d %v", code, out)
	}
	cur := a.getInvoice(draft.ID)
	inv, err := a.svc.Issue(context.Background(), cur.ID, IssueInvoiceRequest{UpdatedAt: cur.UpdatedAt})
	if err != nil {
		a.t.Fatal(err)
	}
	return inv
}

func TestArchive_IssueKeepsPDFXMLAndReport(t *testing.T) {
	a := newArchiveHarness(t)
	inv := a.issueViaService(nil)

	got := a.kinds(inv.ID)
	if len(got) != 3 {
		t.Fatalf("archived kinds = %v, want invoice_pdf, einvoice_xml, validation_report", got)
	}
	if pdf := got[DocumentKindInvoicePDF]; pdf.MimeType != "application/pdf" || !strings.HasPrefix(a.content(pdf), "%PDF") || pdf.UploadedBy != "system" {
		t.Errorf("pdf = %+v", pdf)
	}
	xml := got[DocumentKindEInvoiceXML]
	if xml.MimeType != "application/xml" || xml.Filename != inv.Number()+"-einvoice.xml" || !strings.Contains(a.content(xml), "<") {
		t.Errorf("xml = %+v", xml)
	}
	var rec validationRecord
	rep := got[DocumentKindValidationReport]
	if err := json.Unmarshal([]byte(a.content(rep)), &rec); err != nil {
		t.Fatal(err)
	}
	if !rec.Passed || rec.Format != einvoice.FacturX || rec.Syntax != "CII" || rec.Violations == nil || rec.ValidatedAt.IsZero() {
		t.Errorf("report = %+v", rec)
	}
	for _, d := range got {
		if d.Version != 1 || !d.IsCurrent || d.ChecksumSHA256 == "" {
			t.Errorf("%s = %+v", d.Kind, d)
		}
	}
}

func TestArchive_IsIdempotent(t *testing.T) {
	a := newArchiveHarness(t)
	inv := a.issueViaService(nil)
	before := len(a.store.objects)
	if err := a.svc.Archive(context.Background(), inv.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.repo.documents) != 3 || len(a.store.objects) != before {
		t.Errorf("archiving again changed things: %d docs, %d objects (was %d)", len(a.repo.documents), len(a.store.objects), before)
	}
}

func TestArchive_UnexportableInvoiceStillKeepsItsPDF(t *testing.T) {
	a := newArchiveHarness(t)
	// Withholding tax has no EN 16931 field, so this invoice cannot be exported.
	inv := a.issueViaService(map[string]any{"withholding_rate_bps": 1500})
	got := a.kinds(inv.ID)
	if len(got) != 1 || got[DocumentKindInvoicePDF] == nil {
		t.Fatalf("kinds = %v, want only invoice_pdf", got)
	}
}

func TestArchive_GeneratorOutageStillKeepsPDFAndDoesNotFailIssue(t *testing.T) {
	a := newArchiveHarness(t)
	a.gen.err = einvoice.ErrUnavailable
	inv := a.issueViaService(nil) // must not fail
	got := a.kinds(inv.ID)
	if len(got) != 1 || got[DocumentKindInvoicePDF] == nil {
		t.Fatalf("kinds = %v", got)
	}
	// Once the generator is back, archiving fills in what is missing and
	// leaves the PDF alone.
	a.gen.err = nil
	pdfID := got[DocumentKindInvoicePDF].ID
	if err := a.svc.Archive(context.Background(), inv.ID); err != nil {
		t.Fatal(err)
	}
	got = a.kinds(inv.ID)
	if len(got) != 3 || got[DocumentKindInvoicePDF].ID != pdfID {
		t.Fatalf("kinds after retry = %v", got)
	}
}

func TestArchive_StorageFailureDoesNotFailIssue(t *testing.T) {
	a := newArchiveHarness(t)
	a.svc.WithArchive(failingArchive{})
	inv := a.issueViaService(nil)
	if inv.Number() == "" || inv.Status != InvoiceStatusIssued {
		t.Fatalf("invoice = %+v", inv)
	}
}

func TestArchive_DraftsAreNotArchived(t *testing.T) {
	a := newArchiveHarness(t)
	draft := a.createDraft("")
	if err := a.svc.Archive(context.Background(), draft.ID); !errors.Is(err, ErrInvoiceBadState) {
		t.Fatalf("err = %v, want ErrInvoiceBadState", err)
	}
	if len(a.repo.documents) != 0 {
		t.Fatal("a draft was archived")
	}
}

func TestArchive_WithoutAnArchiveNothingHappens(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(nil)
	if err := hs.svc.Archive(context.Background(), inv.ID); err != nil {
		t.Fatal(err)
	}
}

func TestArchive_CreditNotesAreArchivedToo(t *testing.T) {
	a := newArchiveHarness(t)
	inv := a.issueViaService(nil)
	cur := a.getInvoice(inv.ID)
	res, err := a.svc.CreditNote(context.Background(), cur.ID, CreditNoteRequest{Reason: "wrong amount", UpdatedAt: cur.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	got := a.kinds(res.CreditNote.ID)
	if got[DocumentKindInvoicePDF] == nil || got[DocumentKindEInvoiceXML] == nil {
		t.Fatalf("credit note kinds = %v", got)
	}
	// The original keeps exactly what it had.
	if len(a.kinds(inv.ID)) != 3 {
		t.Errorf("original changed: %v", a.kinds(inv.ID))
	}
}

type failingArchive struct{}

func (failingArchive) Create(context.Context, CreateDocumentRequest, io.Reader) (*Document, error) {
	return nil, errors.New("storage down")
}

func (failingArchive) ListByOwner(context.Context, DocumentOwnerType, uuid.UUID) ([]*Document, error) {
	return nil, nil
}
