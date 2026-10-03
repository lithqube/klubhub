package einvoice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Golden tests: the real e-invoice-eu server end to end.
//
//	EINVOICE_URL=http://127.0.0.1:3101  go test ./internal/einvoice -run Golden -v
//	VERAPDF_URL=http://127.0.0.1:8081   (optional) also checks PDF/A-3b
//
// They are skipped without EINVOICE_URL. scripts/einvoice-golden.sh starts the
// pinned containers and also runs the KoSIT validator (the official XRechnung
// schematron) over the XML, through EINVOICE_OUT_DIR (see keepGolden).

// keepGolden writes what the generator produced to $EINVOICE_OUT_DIR, if set, so
// scripts/einvoice-golden.sh can hand the very same files to the KoSIT
// validator. A Factur-X export keeps the PDF and the XML twin that was
// validated in-process; the XML inside the PDF is extracted from the PDF itself
// by the script.
func keepGolden(t *testing.T, name string, f Format, file *File) {
	t.Helper()
	dir := os.Getenv("EINVOICE_OUT_DIR")
	if dir == "" {
		return
	}
	base := dir + "/" + strings.NewReplacer("/", "_", " ", "_").Replace(name) + "__" + string(f)
	write := func(path string, data []byte) {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("keep golden file: %v", err)
		}
	}
	write(base+file.Extension, file.Data)
	if f == FacturX {
		write(base+".twin.xml", file.XML)
	}
}

func goldenExporter(t *testing.T) *Exporter {
	t.Helper()
	url := os.Getenv("EINVOICE_URL")
	if url == "" {
		t.Skip("EINVOICE_URL not set")
	}
	return &Exporter{Gen: NewSidecarClient(url, &http.Client{Timeout: 60 * time.Second})}
}

func zero(code string, withPercent bool, vatex, reason string) Category {
	return Category{Code: code, Percent: "0", HasPercent: withPercent, ExemptionCode: vatex, ExemptionReason: reason}
}

// retax rewrites every line and the breakdown to category c with no VAT.
func retax(d *Document, c Category) {
	for i := range d.Lines {
		d.Lines[i].Category = c
	}
	d.Taxes = []TaxLine{{Category: c, Taxable: "1200.00", Tax: "0.00"}}
	d.Totals.Tax, d.Totals.TaxInclusive, d.Totals.Payable = "0.00", "1200.00", "1200.00"
}

func goldenScenarios() map[string]Document {
	withRefs := sampleDoc()
	withRefs.PurchaseOrderRef, withRefs.ContractRef = "PO-7781", "C-2026-09"

	reverse := sampleDoc()
	reverse.Number = "INV-0043-EUR"
	reverse.Buyer.Country, reverse.Buyer.VATID, reverse.Buyer.Name = "FR", "FR12345678901", "Paris Club SAS"
	retax(&reverse, zero("AE", true, "VATEX-EU-AE", "Steuerschuldnerschaft des Leistungsempfängers (Reverse Charge) / Reverse charge"))

	kleinunternehmer := sampleDoc()
	kleinunternehmer.Number = "INV-0044-EUR"
	kleinunternehmer.Seller.VATID, kleinunternehmer.Seller.TaxNumber = "", "37/123/45678"
	retax(&kleinunternehmer, zero("E", true, "", "Gemäß § 19 UStG wird keine Umsatzsteuer berechnet. / VAT is not charged under § 19 UStG."))

	outside := sampleDoc()
	outside.Number = "INV-0045-EUR"
	outside.Seller.TaxNumber = "37/123/45678" // BR-O-02: the VAT IDs drop out, the Steuernummer identifies the seller
	outside.Buyer.Country, outside.Buyer.VATID, outside.Buyer.Name = "CH", "CHE-123.456.789", "Zürich Events AG"
	retax(&outside, zero("O", false, "VATEX-EU-O", "Nicht steuerbare sonstige Leistung (§ 3a Abs. 2 UStG) / Not subject to German VAT"))

	credit := sampleDoc()
	credit.Number, credit.TypeCode = "CN-0001-EUR", TypeCreditNote
	credit.PrecedingNumber, credit.PrecedingIssueDate = "INV-0042-EUR", "2026-10-05"

	return map[string]Document{
		"domestic": sampleDoc(), "references": withRefs, "reverse-charge": reverse,
		"kleinunternehmer": kleinunternehmer, "outside-scope": outside, "credit-note": credit,
	}
}

func TestGolden_EveryScenarioInEveryFormat(t *testing.T) {
	ex := goldenExporter(t)
	pdf := readFixture(t, "plain-invoice.pdf")
	for name, doc := range goldenScenarios() {
		for _, f := range Formats {
			t.Run(name+"/"+string(f), func(t *testing.T) {
				file, err := ex.Export(context.Background(), doc, f, pdf)
				if err != nil {
					t.Fatalf("export: %v", err)
				}
				if !file.Report.OK() {
					t.Fatalf("violations: %v", file.Report.Violations)
				}
				keepGolden(t, "scenario-"+name, f, file)
				switch f {
				case FacturX:
					if !bytes.HasPrefix(file.Data, []byte("%PDF-")) {
						t.Fatalf("not a PDF: %.20q", file.Data)
					}
					assertPDFA3(t, file.Data)
				default:
					if !bytes.Contains(file.Data[:200], []byte("<?xml")) {
						t.Fatalf("not XML: %.60q", file.Data)
					}
					if !file.Report.XRechnung {
						t.Errorf("not recognised as XRechnung")
					}
				}
			})
		}
	}
}

// What the user sees when something is missing, through the real server.
func TestGolden_CheckReportsMissingFields(t *testing.T) {
	ex := goldenExporter(t)
	d := sampleDoc()
	d.BuyerReference, d.Seller.Phone = "", ""
	ps, err := ex.Check(context.Background(), d, XRechnungCII)
	if err != nil || !hasProblem(ps, "buyer_reference") || !hasProblem(ps, "seller.phone") {
		t.Fatalf("problems = %v, err = %v", ps, err)
	}
	// Factur-X does not need either.
	if ps, err := ex.Check(context.Background(), d, FacturX); err != nil || len(ps) != 0 {
		t.Errorf("factur-x problems = %v, err = %v", ps, err)
	}
}

// The sidecar does not calculate: totals that do not add up come back as a
// finding from OUR validator, never as a file.
func TestGolden_WrongTotalsAreCaughtNotShipped(t *testing.T) {
	ex := goldenExporter(t)
	d := sampleDoc()
	d.Totals.Payable = "1500.00"
	_, err := ex.Export(context.Background(), d, XRechnungCII, nil)
	ne, ok := err.(*NotExportableError)
	if !ok {
		t.Fatalf("err = %v, want *NotExportableError", err)
	}
	if !strings.Contains(ne.Error(), "BR-CO-16") {
		t.Errorf("error = %v", ne)
	}
}

// assertPDFA3 asks veraPDF whether the hybrid PDF is PDF/A-3b compliant, when
// VERAPDF_URL points at a veraPDF REST server.
func assertPDFA3(t *testing.T, pdf []byte) {
	t.Helper()
	url := os.Getenv("VERAPDF_URL")
	if url == "" {
		return
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "invoice.pdf")
	_, _ = fw.Write(pdf)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, url+"/api/validate/auto", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json") // veraPDF answers HTML by default
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("veraPDF: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Report struct {
			Jobs []struct {
				ValidationResult []struct {
					ProfileName string `json:"profileName"`
					Compliant   bool   `json:"compliant"`
					Details     struct {
						FailedRules int `json:"failedRules"`
					} `json:"details"`
				} `json:"validationResult"`
			} `json:"jobs"`
		} `json:"report"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Report.Jobs) == 0 || len(out.Report.Jobs[0].ValidationResult) == 0 {
		t.Fatalf("veraPDF answer: %v\n%.300s", err, raw)
	}
	r := out.Report.Jobs[0].ValidationResult[0]
	if !r.Compliant || !strings.Contains(r.ProfileName, "PDF/A-3") {
		t.Errorf("veraPDF: profile=%q compliant=%v failedRules=%d", r.ProfileName, r.Compliant, r.Details.FailedRules)
	}
}
