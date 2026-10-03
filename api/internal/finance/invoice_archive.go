package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/einvoice"
)

// DocumentArchive is where issued documents are kept. *DocumentService
// satisfies it.
type DocumentArchive interface {
	Create(ctx context.Context, req CreateDocumentRequest, content io.Reader) (*Document, error)
	ListByOwner(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) ([]*Document, error)
}

// WithArchive makes issuing keep the documents it produces. Without it nothing
// is archived and every document is rendered on demand, as before.
func (s *InvoiceService) WithArchive(a DocumentArchive) *InvoiceService {
	s.archive = a
	return s
}

// archiveTimeout bounds the work done after a document is issued: rendering,
// the generator call and three uploads.
const archiveTimeout = 2 * time.Minute

// validationRecord is the JSON kept next to the XML: what was checked, how, and
// what the rule engine said, so the result can be shown later without
// re-running anything.
type validationRecord struct {
	Format      einvoice.Format       `json:"format"`
	Syntax      string                `json:"syntax"`
	XRechnung   bool                  `json:"xrechnung_rules_applied"`
	Passed      bool                  `json:"passed"`
	ValidatedAt time.Time             `json:"validated_at"`
	Violations  []validationViolation `json:"violations"`
	Warnings    []validationViolation `json:"warnings"`
}

type validationViolation struct {
	Rule string `json:"rule"`
	Text string `json:"text"`
}

func violationsOf(vs []einvoice.Violation) []validationViolation {
	out := make([]validationViolation, 0, len(vs))
	for _, v := range vs {
		out = append(out, validationViolation{Rule: v.Rule, Text: v.Text})
	}
	return out
}

// archiveIssued keeps the documents of a document that has just been numbered.
// The number is already committed, so a failure here must not fail the
// request: it is logged, and the PDF and e-invoice stay available on demand.
// The context is detached so a client that hangs up cannot cut the archive off.
func (s *InvoiceService) archiveIssued(ctx context.Context, id uuid.UUID) {
	if s.archive == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), archiveTimeout)
	defer cancel()
	if err := s.Archive(ctx, id); err != nil {
		slog.Error("archiving issued invoice failed", "invoice_id", id, "err", err)
	}
}

// Archive stores, once, what an issued invoice or credit note is made of: the
// PDF as issued, and, when the e-invoice can be produced and passes every
// rule, its validated XML and the validation report. It is idempotent: a kind
// that is already archived is left alone, so archived evidence is never
// replaced.
//
// An invoice that cannot be exported (missing buyer data, withholding tax, no
// generator configured) still gets its PDF archived; the rest is skipped, not
// an error.
func (s *InvoiceService) Archive(ctx context.Context, id uuid.UUID) error {
	return s.archiveAs(ctx, id, archivedBySystem)
}

// uploaded_by values that say how a document came to be archived.
const (
	archivedBySystem   = "system"   // at issue, as it was issued
	archivedByBackfill = "backfill" // rendered later for an invoice issued before archiving existed
)

func (s *InvoiceService) archiveAs(ctx context.Context, id uuid.UUID, by string) error {
	if s.archive == nil {
		return nil
	}
	data, err := s.einvoiceSource(ctx, id) // numbered documents only
	if err != nil {
		return err
	}
	existing, err := s.archive.ListByOwner(ctx, DocumentOwnerInvoice, id)
	if err != nil {
		return err
	}
	have := map[DocumentKind]bool{}
	for _, d := range existing {
		have[d.Kind] = true
	}
	if have[DocumentKindInvoicePDF] && have[DocumentKindEInvoiceXML] && have[DocumentKindValidationReport] {
		return nil
	}

	pdf, name, _, err := NewPDFRenderer().RenderInvoice(ctx, data)
	if err != nil {
		return err
	}
	number := strings.ReplaceAll(data.Invoice.Number(), "/", "-")
	put := func(kind DocumentKind, filename, mimeType string, content []byte) error {
		if have[kind] {
			return nil
		}
		_, err := s.archive.Create(ctx, CreateDocumentRequest{
			OwnerType: DocumentOwnerInvoice, OwnerID: id, Kind: kind,
			Filename: filename, MimeType: mimeType, UploadedBy: by,
		}, bytes.NewReader(content))
		if errors.Is(err, ErrDocumentConflict) {
			return nil // a concurrent archive got there first
		}
		return err
	}

	errs := []error{put(DocumentKindInvoicePDF, name, "application/pdf", pdf)}
	if (!have[DocumentKindEInvoiceXML] || !have[DocumentKindValidationReport]) && s.exporter != nil {
		doc, problems := einvoiceDocument(data)
		if len(problems) == 0 {
			file, err := s.exporter.Export(ctx, doc, einvoice.FacturX, pdf)
			var notExportable *einvoice.NotExportableError
			switch {
			case err == nil:
				rep := file.Report
				record, merr := json.MarshalIndent(validationRecord{
					Format: file.Format, Syntax: rep.Syntax, XRechnung: rep.XRechnung, Passed: rep.OK(),
					ValidatedAt: time.Now().UTC(),
					Violations:  violationsOf(rep.Violations), Warnings: violationsOf(rep.Warnings),
				}, "", "  ")
				errs = append(errs, merr,
					put(DocumentKindEInvoiceXML, number+"-einvoice.xml", "application/xml", file.XML),
					put(DocumentKindValidationReport, number+"-validation.json", "application/json", record))
			case errors.As(err, &notExportable):
				slog.Info("e-invoice not archived: not exportable", "invoice_id", id, "problems", len(notExportable.Problems))
			case errors.Is(err, einvoice.ErrUnavailable):
				slog.Info("e-invoice not archived: generator unavailable", "invoice_id", id)
			default:
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// NumberedInvoiceLister lists every numbered invoice and credit note, oldest
// first. *InvoiceRepository satisfies it.
type NumberedInvoiceLister interface {
	NumberedIDs(ctx context.Context) ([]uuid.UUID, error)
}

// NumberedIDs lists the ids of every document that has a number: issued, paid,
// credited and corrected invoices and credit notes, oldest first.
func (r *InvoiceRepository) NumberedIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM invoices
		WHERE status IN ('issued', 'paid', 'credited', 'corrected')
		ORDER BY issued_at NULLS LAST, created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list numbered invoices: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan invoice id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// BackfillOptions controls BackfillArchive.
type BackfillOptions struct {
	// DryRun reports what would be archived and writes nothing.
	DryRun bool
	// Progress, if set, is called after each invoice with a one-line outcome.
	Progress func(done, total int, number, outcome string)
}

// BackfillFailure is one invoice that could not be archived.
type BackfillFailure struct {
	ID     uuid.UUID `json:"id"`
	Number string    `json:"number"`
	Error  string    `json:"error"`
}

// BackfillReport is the outcome of BackfillArchive.
type BackfillReport struct {
	DryRun bool `json:"dry_run"`
	Total  int  `json:"total"`
	// Complete had every kind already; Archived got at least one new document
	// (or, in a dry run, would).
	Complete int `json:"complete"`
	Archived int `json:"archived"`
	// Created counts new documents per kind (always zero in a dry run).
	Created map[DocumentKind]int `json:"created"`
	// WithoutEInvoice is how many are left with no e-invoice XML afterwards:
	// invoices that cannot be exported (older ones often lack the buyer and
	// line data EN 16931 needs) or a generator that was not reachable. A later
	// run tries them again.
	WithoutEInvoice int               `json:"without_einvoice"`
	Failed          []BackfillFailure `json:"failed"`
}

// BackfillArchive archives the invoices and credit notes that were issued
// before archiving existed, with the same rules as at issue: PDF always, the
// validated XML and report when the invoice can be exported.
//
// What it stores is rendered now from the invoice as stored (including the
// supplier snapshot taken when it was issued). It is not the file that was
// produced or sent back then, and the layout may have changed since, so these
// documents are marked uploaded_by "backfill" rather than "system".
//
// It is idempotent and safe to run beside a live server: a kind that exists is
// never replaced, and one invoice failing does not stop the rest.
func (s *InvoiceService) BackfillArchive(ctx context.Context, opts BackfillOptions) (*BackfillReport, error) {
	if s.archive == nil {
		return nil, ErrEmailUnavailable
	}
	lister, ok := s.repo.(NumberedInvoiceLister)
	if !ok {
		return nil, errors.New("this invoice repository cannot list numbered invoices")
	}
	ids, err := lister.NumberedIDs(ctx)
	if err != nil {
		return nil, err
	}
	rep := &BackfillReport{DryRun: opts.DryRun, Total: len(ids), Created: map[DocumentKind]int{}, Failed: []BackfillFailure{}}

	kindsOf := func(id uuid.UUID) (map[DocumentKind]bool, error) {
		docs, err := s.archive.ListByOwner(ctx, DocumentOwnerInvoice, id)
		if err != nil {
			return nil, err
		}
		out := map[DocumentKind]bool{}
		for _, d := range docs {
			out[d.Kind] = true
		}
		return out, nil
	}
	note := func(i int, number, outcome string) {
		if opts.Progress != nil {
			opts.Progress(i+1, len(ids), number, outcome)
		}
	}

	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		inv, _, err := s.repo.GetByID(ctx, id)
		number := id.String()
		if err == nil {
			number = inv.Number()
		}
		fail := func(err error) {
			rep.Failed = append(rep.Failed, BackfillFailure{ID: id, Number: number, Error: err.Error()})
			note(i, number, "FAILED: "+err.Error())
		}
		if err != nil {
			fail(err)
			continue
		}
		before, err := kindsOf(id)
		if err != nil {
			fail(err)
			continue
		}
		if before[DocumentKindInvoicePDF] && before[DocumentKindEInvoiceXML] && before[DocumentKindValidationReport] {
			rep.Complete++
			note(i, number, "already complete")
			continue
		}
		if opts.DryRun {
			rep.Archived++
			note(i, number, "would archive")
			continue
		}

		archiveCtx, cancel := context.WithTimeout(ctx, archiveTimeout)
		err = s.archiveAs(archiveCtx, id, archivedByBackfill)
		cancel()
		after, lerr := kindsOf(id)
		if lerr != nil {
			fail(lerr)
			continue
		}
		var added []string
		for kind := range after {
			if !before[kind] {
				rep.Created[kind]++
				added = append(added, string(kind))
			}
		}
		if !after[DocumentKindEInvoiceXML] {
			rep.WithoutEInvoice++
		}
		switch {
		case err != nil:
			fail(err)
		case len(added) > 0:
			rep.Archived++
			note(i, number, "archived "+strings.Join(added, ", "))
		default:
			note(i, number, "nothing to add")
		}
	}
	return rep, nil
}
