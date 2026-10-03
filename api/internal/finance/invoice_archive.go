package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
			Filename: filename, MimeType: mimeType, UploadedBy: "system",
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
