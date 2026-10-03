package finance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// getPDF issues GET /invoices/{id}/pdf against the harness handler.
func (hs *invoiceHarness) getPDF(id string) *httptest.ResponseRecorder {
	hs.t.Helper()
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices/"+id+"/pdf", nil))
	return rec
}

func TestInvoiceHandler_PDF_IssuedInvoice(t *testing.T) {
	hs := newInvoiceHarness(t)
	issued := hs.issue(hs.createDraft(""))

	rec := hs.getPDF(issued.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	wantName := "invoice-" + issued.Number() + ".pdf"
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, wantName) {
		t.Errorf("Content-Disposition = %q, want attachment with %q", cd, wantName)
	}
	// Financial documents must not be cached by shared caches or sniffed.
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing X-Content-Type-Options: nosniff")
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Errorf("body is not a PDF: %.20q", rec.Body.String())
	}
}

func TestInvoiceHandler_PDF_DraftRendersWithoutNumber(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")

	rec := hs.getPDF(draft.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "invoice-draft-"+draft.ID.String()[:8]+".pdf") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestInvoiceHandler_PDF_CreditNote(t *testing.T) {
	hs := newInvoiceHarness(t)
	issued := hs.issue(hs.createDraft(""))
	code, out := hs.do(http.MethodPost, "/"+issued.ID.String()+"/credit-note",
		map[string]any{"reason": "gig cancelled", "updated_at": issued.UpdatedAt})
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("credit note: %d %v", code, out)
	}
	var res CreditNoteResult
	decode(t, out["data"], &res)

	rec := hs.getPDF(res.CreditNote.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "credit-note-") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestInvoiceHandler_PDF_Errors(t *testing.T) {
	hs := newInvoiceHarness(t)

	if rec := hs.getPDF(uuid.NewString()); rec.Code != http.StatusNotFound {
		t.Errorf("unknown invoice: status %d, want 404", rec.Code)
	}
	if rec := hs.getPDF("not-a-uuid"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: status %d, want 400", rec.Code)
	}
	draft := hs.createDraft("")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+draft.ID.String()+"/pdf", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d, want 405", rec.Code)
	}
}

// A credit note's PDF must name the invoice it reverses, not its UUID.
func TestInvoiceService_PDFData_CreditNoteReferencesOriginalNumber(t *testing.T) {
	hs := newInvoiceHarness(t)
	issued := hs.issue(hs.createDraft(""))
	_, out := hs.do(http.MethodPost, "/"+issued.ID.String()+"/credit-note",
		map[string]any{"reason": "gig cancelled", "updated_at": issued.UpdatedAt})
	var res CreditNoteResult
	decode(t, out["data"], &res)

	svc := NewInvoiceService(hs.repo, hs.billing, hs.gigs)
	data, err := svc.pdfData(context.Background(), res.CreditNote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if data.CreditedInvoiceNumber != issued.Number() {
		t.Errorf("CreditedInvoiceNumber = %q, want %q", data.CreditedInvoiceNumber, issued.Number())
	}
	if data.BillingProfile != nil {
		t.Error("issued documents print their own snapshot; the live profile must not be loaded")
	}
}

// Drafts have no supplier snapshot yet, so the live billing profile fills
// the FROM block.
func TestInvoiceService_PDFData_DraftUsesLiveBillingProfile(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")

	svc := NewInvoiceService(hs.repo, hs.billing, hs.gigs)
	data, err := svc.pdfData(context.Background(), draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if data.BillingProfile == nil || data.BillingProfile.LegalName != "Test DJ" {
		t.Errorf("BillingProfile = %+v", data.BillingProfile)
	}
}
