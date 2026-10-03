package einvoice

import (
	"context"
	"fmt"
	"strings"
)

// File is a finished, validated e-invoice.
type File struct {
	Data []byte
	// XML is the validated EN 16931 XML behind the file: Data itself for
	// XRechnung, the plain CII twin of the embedded XML for Factur-X.
	XML       []byte
	Format    Format
	MimeType  string
	Extension string
	// Report is the validation of the XML inside the file. The BMF recommends
	// keeping it with the invoice.
	Report *Report
}

// NotExportableError means the invoice cannot be exported yet; Problems say
// what to fix.
type NotExportableError struct{ Problems []Problem }

func (e *NotExportableError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Field + ": " + p.Message
	}
	return "invoice is not ready to be exported: " + strings.Join(parts, "; ")
}

// Exporter produces validated e-invoices through a Generator.
type Exporter struct{ Gen Generator }

// Check reports what stops d from being exported as f. It stops at the plain
// missing-field list; only a document that passes that is generated and run
// through the rule engine, so a nearly empty invoice never reaches the
// sidecar. An empty result means Export will succeed (barring an outage).
func (e *Exporter) Check(ctx context.Context, d Document, f Format) ([]Problem, error) {
	if !f.Valid() {
		return nil, fmt.Errorf("unknown e-invoice format %q", f)
	}
	if ps := Prepare(d, f); len(ps) > 0 {
		return ps, nil
	}
	_, rep, err := e.generateXML(ctx, d, f)
	if err != nil {
		return nil, err
	}
	return violationProblems(rep), nil
}

// Export generates the e-invoice. pdf is the visual invoice PDF; it is
// embedded for FacturX and ignored otherwise.
//
// The XML is generated and validated first; nothing invalid is ever returned.
// For XRechnung the validated XML is the file. For Factur-X the same business
// content is generated as plain EN 16931 CII, validated, and only then is the
// PDF wrapped, so the PDF's embedded XML comes from validated input.
func (e *Exporter) Export(ctx context.Context, d Document, f Format, pdf []byte) (*File, error) {
	if !f.Valid() {
		return nil, fmt.Errorf("unknown e-invoice format %q", f)
	}
	if f == FacturX && len(pdf) == 0 {
		return nil, fmt.Errorf("a Factur-X export needs the invoice PDF to embed")
	}
	if ps := Prepare(d, f); len(ps) > 0 {
		return nil, &NotExportableError{Problems: ps}
	}
	xml, rep, err := e.generateXML(ctx, d, f)
	if err != nil {
		return nil, err
	}
	if ps := violationProblems(rep); len(ps) > 0 {
		return nil, &NotExportableError{Problems: ps}
	}

	data := xml
	if f == FacturX {
		raw, err := d.SidecarJSON()
		if err != nil {
			return nil, err
		}
		if data, err = e.Gen.Generate(ctx, f.SidecarName(), raw, pdf); err != nil {
			return nil, err
		}
	}
	return &File{Data: data, XML: xml, Format: f, MimeType: f.MimeType(), Extension: f.Extension(), Report: rep}, nil
}

// generateXML produces and validates the XML for f (the file itself for
// XRechnung, the plain CII twin for Factur-X).
func (e *Exporter) generateXML(ctx context.Context, d Document, f Format) ([]byte, *Report, error) {
	if e == nil || e.Gen == nil {
		return nil, nil, ErrUnavailable
	}
	raw, err := d.SidecarJSON()
	if err != nil {
		return nil, nil, err
	}
	xml, err := e.Gen.Generate(ctx, f.ValidationName(), raw, nil)
	if err != nil {
		return nil, nil, err
	}
	rep, err := Validate(xml)
	if err != nil {
		return nil, nil, err
	}
	return xml, rep, nil
}

func violationProblems(r *Report) []Problem {
	var ps []Problem
	for _, v := range r.Violations {
		ps = append(ps, ProblemFor(v))
	}
	return ps
}
