package einvoice

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- SidecarClient against a fake e-invoice-eu ------------------------------------

type recorded struct {
	path, method string
	invoice      []byte
	invoiceType  string
	pdf          []byte
	hasPDF       bool
	embedPDF     string
}

func fakeSidecar(t *testing.T, status int, body string, ct string) (*SidecarClient, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.path, rec.method = r.URL.Path, r.Method
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err == nil && strings.HasPrefix(mt, "multipart/") {
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				data, _ := io.ReadAll(p)
				switch p.FormName() {
				case "invoice":
					rec.invoice, rec.invoiceType = data, p.Header.Get("Content-Type")
					if p.FileName() == "" {
						t.Errorf("the invoice must be a FILE part (a text field gets a 400)")
					}
				case "pdf":
					rec.pdf, rec.hasPDF = data, true
				case "embedPDF":
					rec.embedPDF = string(data)
				}
			}
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewSidecarClient(srv.URL, srv.Client()), rec
}

func TestSidecarClient_XMLFormat(t *testing.T) {
	c, rec := fakeSidecar(t, 201, "<xml/>", "application/xml")
	out, err := c.Generate(context.Background(), "XRECHNUNG-CII", []byte(`{"a":1}`), nil)
	if err != nil || string(out) != "<xml/>" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if rec.method != http.MethodPost || rec.path != "/api/invoice/create/XRECHNUNG-CII" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if string(rec.invoice) != `{"a":1}` || rec.invoiceType != "application/json" {
		t.Errorf("invoice part = %q (%s)", rec.invoice, rec.invoiceType)
	}
	if rec.hasPDF || rec.embedPDF != "" {
		t.Errorf("no PDF expected for an XML format")
	}
}

func TestSidecarClient_FacturXSendsThePDFToEmbed(t *testing.T) {
	c, rec := fakeSidecar(t, 201, "%PDF-1.7 hybrid", "application/pdf")
	out, err := c.Generate(context.Background(), "Factur-X-EN16931", []byte(`{}`), []byte("%PDF-1.3 plain"))
	if err != nil || string(out) != "%PDF-1.7 hybrid" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if string(rec.pdf) != "%PDF-1.3 plain" || rec.embedPDF != "true" {
		t.Errorf("pdf=%q embedPDF=%q", rec.pdf, rec.embedPDF)
	}
}

func TestSidecarClient_InputRejectedIsAGenerationError(t *testing.T) {
	body := `{"message":"Transformation failed.","details":{"errors":[{"instancePath":"/ubl:Invoice/cac:PaymentMeans","message":"must be array"}]}}`
	c, _ := fakeSidecar(t, 400, body, "application/json")
	_, err := c.Generate(context.Background(), "CII", []byte(`{}`), nil)
	var ge *GenerationError
	if !errors.As(err, &ge) {
		t.Fatalf("err = %v, want *GenerationError", err)
	}
	if ge.Status != 400 || !strings.Contains(ge.Error(), "Transformation failed") || !strings.Contains(ge.Error(), "cac:PaymentMeans") {
		t.Errorf("error = %q", ge.Error())
	}
	if errors.Is(err, ErrUnavailable) {
		t.Error("a rejected document is our bug or bad data, not an outage")
	}
}

func TestSidecarClient_OutagesAreUnavailable(t *testing.T) {
	c, _ := fakeSidecar(t, 503, "down", "text/plain")
	if _, err := c.Generate(context.Background(), "CII", []byte(`{}`), nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("503: %v", err)
	}
	c, _ = fakeSidecar(t, 500, "boom", "text/plain")
	if _, err := c.Generate(context.Background(), "CII", []byte(`{}`), nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("500: %v", err)
	}
	dead := NewSidecarClient("http://127.0.0.1:1", &http.Client{Timeout: 2 * time.Second})
	if _, err := dead.Generate(context.Background(), "CII", []byte(`{}`), nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("connection refused: %v", err)
	}
}

func TestSidecarClient_HonoursContextAndCapsTheResponse(t *testing.T) {
	c, _ := fakeSidecar(t, 201, "x", "application/xml")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Generate(ctx, "CII", []byte(`{}`), nil); err == nil {
		t.Error("a cancelled context must fail")
	}

	big, _ := fakeSidecar(t, 201, strings.Repeat("x", maxResponseBytes+10), "application/xml")
	if _, err := big.Generate(context.Background(), "CII", []byte(`{}`), nil); err == nil || errors.Is(err, ErrUnavailable) {
		t.Errorf("an oversized response must be refused, got %v", err)
	}
}

// --- Exporter with a fake generator -------------------------------------------------

type fakeGen struct {
	calls []string
	pdfs  [][]byte
	out   map[string][]byte
	err   error
}

func (g *fakeGen) Generate(_ context.Context, name string, _ []byte, pdf []byte) ([]byte, error) {
	g.calls = append(g.calls, name)
	g.pdfs = append(g.pdfs, pdf)
	if g.err != nil {
		return nil, g.err
	}
	return g.out[name], nil
}

func goodGen(t *testing.T) *fakeGen {
	t.Helper()
	return &fakeGen{out: map[string][]byte{
		"CII":              readFixture(t, "xrechnung-cii.xml"),
		"XRECHNUNG-CII":    readFixture(t, "xrechnung-cii.xml"),
		"XRECHNUNG-UBL":    readFixture(t, "xrechnung-ubl.xml"),
		"Factur-X-EN16931": []byte("%PDF-1.7 hybrid"),
	}}
}

func TestExporter_CheckStopsAtMissingFieldsWithoutCallingTheSidecar(t *testing.T) {
	g := goodGen(t)
	d := sampleDoc()
	d.BuyerReference = ""
	ps, err := (&Exporter{Gen: g}).Check(context.Background(), d, XRechnungCII)
	if err != nil || !hasProblem(ps, "buyer_reference") {
		t.Fatalf("problems = %v, err = %v", ps, err)
	}
	if len(g.calls) != 0 {
		t.Errorf("the generator was called for an incomplete document: %v", g.calls)
	}
}

func TestExporter_CheckOfACompleteDocumentGeneratesAndValidates(t *testing.T) {
	g := goodGen(t)
	ps, err := (&Exporter{Gen: g}).Check(context.Background(), sampleDoc(), XRechnungCII)
	if err != nil || len(ps) != 0 {
		t.Fatalf("problems = %v, err = %v", ps, err)
	}
	if strings.Join(g.calls, ",") != "XRECHNUNG-CII" {
		t.Errorf("calls = %v", g.calls)
	}
}

func TestExporter_ValidatorFindingsBecomeProblems(t *testing.T) {
	g := goodGen(t)
	g.out["XRECHNUNG-CII"] = readFixture(t, "bad-totals-cii.xml")
	ps, err := (&Exporter{Gen: g}).Check(context.Background(), sampleDoc(), XRechnungCII)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) == 0 || !strings.Contains(ps[0].Message, "BR-CO-16") {
		t.Errorf("problems = %v", ps)
	}
}

func TestExporter_XRechnungExportGeneratesOnce(t *testing.T) {
	g := goodGen(t)
	f, err := (&Exporter{Gen: g}).Export(context.Background(), sampleDoc(), XRechnungUBL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.Data, readFixture(t, "xrechnung-ubl.xml")) || f.MimeType != "application/xml" || f.Extension != ".xml" || f.Format != XRechnungUBL {
		t.Errorf("file = %s %s %s", f.Format, f.MimeType, f.Extension)
	}
	if f.Report == nil || !f.Report.OK() || !f.Report.XRechnung {
		t.Errorf("report = %+v", f.Report)
	}
	if strings.Join(g.calls, ",") != "XRECHNUNG-UBL" {
		t.Errorf("calls = %v; the validated XML is the shipped file, so one call", g.calls)
	}
}

func TestExporter_FacturXValidatesTheXMLTwinThenWrapsThePDF(t *testing.T) {
	g := goodGen(t)
	pdf := []byte("%PDF-1.3 our invoice")
	f, err := (&Exporter{Gen: g}).Export(context.Background(), sampleDoc(), FacturX, pdf)
	if err != nil {
		t.Fatal(err)
	}
	if string(f.Data) != "%PDF-1.7 hybrid" || f.MimeType != "application/pdf" || f.Extension != ".pdf" {
		t.Errorf("file = %q %s %s", f.Data, f.MimeType, f.Extension)
	}
	if strings.Join(g.calls, ",") != "CII,Factur-X-EN16931" {
		t.Errorf("calls = %v", g.calls)
	}
	if g.pdfs[0] != nil || !bytes.Equal(g.pdfs[1], pdf) {
		t.Errorf("the PDF goes to the wrap call only: %q", g.pdfs)
	}
}

func TestExporter_FacturXNeedsThePDF(t *testing.T) {
	if _, err := (&Exporter{Gen: goodGen(t)}).Export(context.Background(), sampleDoc(), FacturX, nil); err == nil {
		t.Error("Factur-X without a PDF to embed must fail")
	}
}

func TestExporter_NothingInvalidIsEverShipped(t *testing.T) {
	g := goodGen(t)
	g.out["CII"] = readFixture(t, "bad-totals-cii.xml")
	_, err := (&Exporter{Gen: g}).Export(context.Background(), sampleDoc(), FacturX, []byte("%PDF"))
	var ne *NotExportableError
	if !errors.As(err, &ne) || len(ne.Problems) == 0 {
		t.Fatalf("err = %v, want *NotExportableError", err)
	}
	for _, c := range g.calls {
		if c == "Factur-X-EN16931" {
			t.Error("the PDF was wrapped although its XML failed validation")
		}
	}

	d := sampleDoc()
	d.Seller.Phone = ""
	g2 := goodGen(t)
	_, err = (&Exporter{Gen: g2}).Export(context.Background(), d, XRechnungCII, nil)
	if !errors.As(err, &ne) || !hasProblem(ne.Problems, "seller.phone") || len(g2.calls) != 0 {
		t.Errorf("err = %v calls = %v", err, g2.calls)
	}
}

func TestExporter_Failures(t *testing.T) {
	if _, err := (&Exporter{}).Export(context.Background(), sampleDoc(), XRechnungCII, nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("no generator: %v", err)
	}
	if _, err := (&Exporter{}).Check(context.Background(), sampleDoc(), XRechnungCII); !errors.Is(err, ErrUnavailable) {
		t.Errorf("no generator (check): %v", err)
	}
	g := goodGen(t)
	g.err = ErrUnavailable
	if _, err := (&Exporter{Gen: g}).Export(context.Background(), sampleDoc(), XRechnungCII, nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("outage: %v", err)
	}
	g.err = &GenerationError{Status: 400, Message: "bad"}
	var ge *GenerationError
	if _, err := (&Exporter{Gen: g}).Export(context.Background(), sampleDoc(), XRechnungCII, nil); !errors.As(err, &ge) {
		t.Errorf("rejected input: %v", err)
	}
	g2 := goodGen(t)
	g2.out["XRECHNUNG-CII"] = []byte("not xml at all")
	if _, err := (&Exporter{Gen: g2}).Export(context.Background(), sampleDoc(), XRechnungCII, nil); !errors.Is(err, ErrUnreadable) {
		t.Errorf("unreadable output: %v", err)
	}
	if _, err := (&Exporter{Gen: goodGen(t)}).Export(context.Background(), sampleDoc(), Format("bogus"), nil); err == nil {
		t.Error("an unknown format must be refused")
	}
}
