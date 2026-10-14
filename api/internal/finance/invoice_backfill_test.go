package finance

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
)

// backfillHarness issues invoices with no archive, the state of an
// installation that predates archiving; enableArchive then turns it on.
type backfillHarness struct {
	*archiveHarness
}

func newBackfillHarness(t *testing.T) *backfillHarness {
	t.Helper()
	a := newArchiveHarness(t)
	a.svc.archive = nil // issue as the old release did: nothing is archived
	return &backfillHarness{a}
}

func (b *backfillHarness) enableArchive() { b.svc.WithArchive(b.docs) }

func (b *backfillHarness) kindUploader(id uuid.UUID, kind DocumentKind) string {
	if d := b.kinds(id)[kind]; d != nil {
		return d.UploadedBy
	}
	return ""
}

func TestBackfill_ArchivesOlderInvoicesAndMarksThem(t *testing.T) {
	b := newBackfillHarness(t)
	exportable := b.issueViaService(nil)
	unexportable := b.issueViaService(map[string]any{"withholding_rate_bps": 1500})
	cur := b.getInvoice(exportable.ID)
	cn, err := b.svc.CreditNote(context.Background(), cur.ID, CreditNoteRequest{Reason: "wrong amount", UpdatedAt: cur.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.repo.documents) != 0 {
		t.Fatalf("setup archived %d documents", len(b.repo.documents))
	}
	b.enableArchive()

	var lines []string
	rep, err := b.svc.BackfillArchive(context.Background(), BackfillOptions{
		Progress: func(done, total int, number, outcome string) { lines = append(lines, number+": "+outcome) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 3 || rep.Archived != 3 || rep.Complete != 0 || len(rep.Failed) != 0 || len(lines) != 3 {
		t.Fatalf("report = %+v, progress = %v", rep, lines)
	}
	// original + credit note export fully; the withholding invoice keeps only its PDF
	if rep.Created[DocumentKindInvoicePDF] != 3 || rep.Created[DocumentKindEInvoiceXML] != 2 || rep.Created[DocumentKindValidationReport] != 2 {
		t.Errorf("created = %v", rep.Created)
	}
	if rep.WithoutEInvoice != 1 {
		t.Errorf("without e-invoice = %d, want 1", rep.WithoutEInvoice)
	}
	if got := b.kinds(unexportable.ID); len(got) != 1 || got[DocumentKindInvoicePDF] == nil {
		t.Errorf("unexportable kinds = %v", got)
	}
	// Honest provenance: rendered now, not as issued.
	for _, id := range []uuid.UUID{exportable.ID, unexportable.ID, cn.CreditNote.ID} {
		if by := b.kindUploader(id, DocumentKindInvoicePDF); by != "backfill" {
			t.Errorf("pdf of %s uploaded_by = %q, want backfill", id, by)
		}
	}
}

func TestBackfill_IsIdempotentAndLeavesIssueTimeArchivesAlone(t *testing.T) {
	b := newBackfillHarness(t)
	b.issueViaService(nil)
	b.enableArchive()
	fresh := b.issueViaService(nil) // archived at issue, by "system"
	freshPDF := b.kinds(fresh.ID)[DocumentKindInvoicePDF].ID

	rep, err := b.svc.BackfillArchive(context.Background(), BackfillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 2 || rep.Complete != 1 || rep.Archived != 1 {
		t.Fatalf("first run = %+v", rep)
	}
	if b.kinds(fresh.ID)[DocumentKindInvoicePDF].ID != freshPDF || b.kindUploader(fresh.ID, DocumentKindInvoicePDF) != "system" {
		t.Error("the backfill touched an invoice that was archived at issue")
	}

	docs, objects := len(b.repo.documents), len(b.store.objects)
	rep, err = b.svc.BackfillArchive(context.Background(), BackfillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Complete != 2 || rep.Archived != 0 || len(rep.Created) != 0 || len(b.repo.documents) != docs || len(b.store.objects) != objects {
		t.Errorf("second run changed things: %+v (docs %d→%d)", rep, docs, len(b.repo.documents))
	}
}

func TestBackfill_DryRunWritesNothing(t *testing.T) {
	b := newBackfillHarness(t)
	b.issueViaService(nil)
	b.issueViaService(nil)
	b.enableArchive()
	calls := len(b.gen.calls)

	rep, err := b.svc.BackfillArchive(context.Background(), BackfillOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || rep.Total != 2 || rep.Archived != 2 || len(rep.Created) != 0 {
		t.Errorf("report = %+v", rep)
	}
	if len(b.repo.documents) != 0 || len(b.store.objects) != 0 || len(b.gen.calls) != calls {
		t.Errorf("dry run wrote %d documents, %d objects and called the generator %d times",
			len(b.repo.documents), len(b.store.objects), len(b.gen.calls)-calls)
	}
}

func TestBackfill_GeneratorOutageThenRerunFillsInTheXML(t *testing.T) {
	b := newBackfillHarness(t)
	inv := b.issueViaService(nil)
	b.enableArchive()
	b.gen.err = errors.New("generator down")

	rep, err := b.svc.BackfillArchive(context.Background(), BackfillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := b.kinds(inv.ID); len(got) != 1 || rep.WithoutEInvoice != 1 || len(rep.Failed) != 1 {
		t.Fatalf("kinds = %v, report = %+v", got, rep)
	}

	b.gen.err = nil
	pdfID := b.kinds(inv.ID)[DocumentKindInvoicePDF].ID
	rep, err = b.svc.BackfillArchive(context.Background(), BackfillOptions{})
	if err != nil || len(rep.Failed) != 0 || rep.WithoutEInvoice != 0 {
		t.Fatalf("rerun: %+v, %v", rep, err)
	}
	got := b.kinds(inv.ID)
	if len(got) != 3 || got[DocumentKindInvoicePDF].ID != pdfID {
		t.Errorf("kinds after rerun = %v (the PDF must not be replaced)", got)
	}
}

// flakyArchive refuses to store anything for one invoice.
type flakyArchive struct {
	DocumentArchive
	bad uuid.UUID
}

func (f flakyArchive) Create(ctx context.Context, req CreateDocumentRequest, r io.Reader) (*Document, error) {
	if req.OwnerID == f.bad {
		return nil, errors.New("storage refused")
	}
	return f.DocumentArchive.Create(ctx, req, r)
}

func TestBackfill_OneFailureDoesNotStopTheRest(t *testing.T) {
	b := newBackfillHarness(t)
	first := b.issueViaService(nil)
	second := b.issueViaService(nil)
	b.svc.WithArchive(flakyArchive{DocumentArchive: b.docs, bad: first.ID})

	rep, err := b.svc.BackfillArchive(context.Background(), BackfillOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 1 || rep.Failed[0].ID != first.ID || rep.Failed[0].Number != first.Number() {
		t.Fatalf("failed = %+v", rep.Failed)
	}
	if len(b.kinds(second.ID)) != 3 {
		t.Errorf("the other invoice was not archived: %v", b.kinds(second.ID))
	}
}

func TestBackfill_SkipsDrafts(t *testing.T) {
	b := newBackfillHarness(t)
	b.createDraft("")
	b.enableArchive()
	rep, err := b.svc.BackfillArchive(context.Background(), BackfillOptions{})
	if err != nil || rep.Total != 0 || len(b.repo.documents) != 0 {
		t.Errorf("report = %+v, err = %v", rep, err)
	}
}

func TestBackfill_NeedsAnArchive(t *testing.T) {
	b := newBackfillHarness(t) // archive not enabled
	if _, err := b.svc.BackfillArchive(context.Background(), BackfillOptions{}); !errors.Is(err, ErrEmailUnavailable) {
		t.Errorf("err = %v", err)
	}
}

func TestIntegration_NumberedIDsQueryRuns(t *testing.T) {
	requireEntryPG(t)
	if _, err := NewInvoiceRepository(testPool).NumberedIDs(context.Background()); err != nil {
		t.Fatalf("NumberedIDs: %v", err)
	}
}
