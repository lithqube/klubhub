package einvoice

import (
	"bytes"
	"errors"
	"fmt"

	sd "github.com/speedata/einvoice"
)

// ErrUnreadable means the bytes are not an e-invoice at all (not XML, or a
// different document), as opposed to an e-invoice that breaks a rule.
var ErrUnreadable = errors.New("not a readable e-invoice")

// Violation is one broken business rule.
type Violation struct {
	Rule string // e.g. "BR-CO-16", "BR-DE-15"
	Text string
}

// Report is what the validator found. Violations make the file invalid;
// warnings are recommendations.
type Report struct {
	Syntax     string // "CII" or "UBL"
	XRechnung  bool
	Violations []Violation
	Warnings   []Violation
}

// OK reports whether the file broke no rule.
func (r *Report) OK() bool { return r != nil && len(r.Violations) == 0 }

// Validate checks an e-invoice XML against the EN 16931 business rules, the
// VAT category rules and, for XRechnung, the German BR-DE rules, all in
// process. The BMF recommends keeping such a report with the invoice.
func Validate(xml []byte) (*Report, error) {
	inv, err := sd.ParseReader(bytes.NewReader(xml))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	rep := &Report{XRechnung: inv.IsXRechnung()}
	switch inv.SchemaType {
	case sd.CII:
		rep.Syntax = "CII"
	case sd.UBL:
		rep.Syntax = "UBL"
	}

	if verr := inv.Validate(); verr != nil {
		var ve *sd.ValidationError
		if !errors.As(verr, &ve) {
			return nil, fmt.Errorf("%w: %v", ErrUnreadable, verr)
		}
		for _, v := range ve.Violations() {
			rep.Violations = append(rep.Violations, Violation{Rule: v.Rule.Code, Text: v.Text})
		}
		for _, w := range ve.Warnings() {
			rep.Warnings = append(rep.Warnings, Violation{Rule: w.Rule.Code, Text: w.Text})
		}
	}
	return rep, nil
}
