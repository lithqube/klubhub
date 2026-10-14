package finance

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// putDraft sends PUT /invoices/{id} with the draft's current editable fields
// plus the given overrides (lines, references, ...).
func (hs *invoiceHarness) putDraft(inv Invoice, overrides map[string]any) (int, map[string]any) {
	hs.t.Helper()
	body := map[string]any{
		"customer": inv.Customer, "vat_treatment": "domestic", "tax_rate_bps": 1900, "tax_note": inv.TaxNote,
		"withholding_rate_bps": 0, "supply_date": inv.SupplyDate, "number_prefix": inv.NumberPrefix,
		"internal_notes": "", "updated_at": inv.UpdatedAt,
	}
	for k, v := range overrides {
		body[k] = v
	}
	return hs.do(http.MethodPut, "/"+inv.ID.String(), body)
}

func (hs *invoiceHarness) getLines(id uuid.UUID) []InvoiceLine {
	hs.t.Helper()
	code, out := hs.do(http.MethodGet, "/"+id.String(), nil)
	if code != http.StatusOK {
		hs.t.Fatalf("get: %d %v", code, out)
	}
	var lines []InvoiceLine
	decode(hs.t, out["lines"], &lines)
	return lines
}

func (hs *invoiceHarness) getInvoice(id uuid.UUID) Invoice {
	hs.t.Helper()
	code, out := hs.do(http.MethodGet, "/"+id.String(), nil)
	if code != http.StatusOK {
		hs.t.Fatalf("get: %d %v", code, out)
	}
	var inv Invoice
	decode(hs.t, out["data"], &inv)
	return inv
}

func twoLines() []map[string]any {
	return []map[string]any{
		{"description": "DJ performance", "quantity": 1, "unit_minor": 100000, "unit_code": "LS"},
		{"description": "Travel (km)", "quantity": 200, "unit_minor": 50, "unit_code": "KMT"},
	}
}

func TestInvoiceHandler_PutReplacesLinesAndRecomputesTotals(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")

	code, out := hs.putDraft(draft, map[string]any{"lines": twoLines()})
	if code != http.StatusOK {
		t.Fatalf("put: %d %v", code, out)
	}
	var inv Invoice
	decode(t, out["data"], &inv)
	// 1000.00 + 200 × 0.50 = 1100.00 net; 19% = 209.00.
	if inv.SubtotalMinor != 110000 || inv.TaxMinor != 20900 || inv.TotalMinor != 130900 {
		t.Errorf("totals = %d / %d / %d", inv.SubtotalMinor, inv.TaxMinor, inv.TotalMinor)
	}
	lines := hs.getLines(draft.ID)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if lines[0].Description != "DJ performance" || lines[0].UnitCode != "LS" || lines[0].SortOrder != 0 {
		t.Errorf("line 0 = %+v", lines[0])
	}
	if lines[1].UnitCode != "KMT" || lines[1].Quantity != 200 || lines[1].LineTotalMinor != 10000 || lines[1].TaxBps != 1900 {
		t.Errorf("line 1 = %+v", lines[1])
	}
}

func TestInvoiceHandler_PutWithoutLinesKeepsExistingLines(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")
	before := hs.getLines(draft.ID)

	if code, out := hs.putDraft(draft, nil); code != http.StatusOK {
		t.Fatalf("put: %d %v", code, out)
	}
	after := hs.getLines(draft.ID)
	if len(after) != len(before) || after[0].ID != before[0].ID {
		t.Errorf("lines changed without a lines field: before=%v after=%v", before, after)
	}
}

func TestInvoiceHandler_GigDraftLineDefaultsToPieceUnit(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")
	if lines := hs.getLines(draft.ID); len(lines) != 1 || lines[0].UnitCode != "C62" {
		t.Errorf("lines = %+v, want one line with unit_code C62", lines)
	}
}

func TestInvoiceHandler_PutLineWithoutUnitCodeDefaultsToPiece(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")
	lines := []map[string]any{{"description": "Set", "quantity": 1, "unit_minor": 5000}}
	if code, out := hs.putDraft(draft, map[string]any{"lines": lines}); code != http.StatusOK {
		t.Fatalf("put: %d %v", code, out)
	}
	if got := hs.getLines(draft.ID); got[0].UnitCode != "C62" {
		t.Errorf("unit_code = %q", got[0].UnitCode)
	}
}

func TestInvoiceHandler_PutRejectsInvalidLines(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")
	line := func(m map[string]any) []map[string]any {
		base := map[string]any{"description": "Set", "quantity": 1, "unit_minor": 100}
		for k, v := range m {
			base[k] = v
		}
		return []map[string]any{base}
	}
	tooMany := make([]map[string]any, 101)
	for i := range tooMany {
		tooMany[i] = map[string]any{"description": "x", "quantity": 1, "unit_minor": 1}
	}
	cases := []struct {
		name  string
		lines any
		field string
	}{
		{"empty list", []map[string]any{}, "lines"},
		{"too many lines", tooMany, "lines"},
		{"blank description", line(map[string]any{"description": "  "}), "lines[0].description"},
		{"description too long", line(map[string]any{"description": strings.Repeat("x", 501)}), "lines[0].description"},
		{"zero quantity", line(map[string]any{"quantity": 0}), "lines[0].quantity"},
		{"quantity too large", line(map[string]any{"quantity": 1_000_001}), "lines[0].quantity"},
		{"negative price", line(map[string]any{"unit_minor": -1}), "lines[0].unit_minor"},
		{"unknown unit code", line(map[string]any{"unit_code": "XYZ"}), "lines[0].unit_code"},
		{"line total overflows the cap", line(map[string]any{"quantity": 1_000_000, "unit_minor": 1_000_000_000_000}), "lines[0].unit_minor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := hs.putDraft(draft, map[string]any{"lines": tc.lines})
			if code != http.StatusBadRequest || out["error"] != "validation_failed" {
				t.Fatalf("status %d %v, want 400 validation_failed", code, out)
			}
			if msg, _ := out["message"].(string); !strings.Contains(msg, tc.field+":") {
				t.Errorf("message %q does not name %q", msg, tc.field)
			}
		})
	}
	// None of the rejected writes may have touched the draft.
	if lines := hs.getLines(draft.ID); len(lines) != 1 {
		t.Errorf("rejected PUTs changed the lines: %+v", lines)
	}
}

func TestInvoiceHandler_PutAndGetCarryReferencesAndTerms(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")

	code, out := hs.putDraft(draft, map[string]any{
		"buyer_reference": " 04011000-12345-34 ", "purchase_order_ref": "PO-7781",
		"contract_ref": "C-2026-09", "payment_terms": "Payable within 14 days without deduction.",
	})
	if code != http.StatusOK {
		t.Fatalf("put: %d %v", code, out)
	}
	var inv Invoice
	decode(t, out["data"], &inv)
	if inv.BuyerReference != "04011000-12345-34" || inv.PurchaseOrderRef != "PO-7781" ||
		inv.ContractRef != "C-2026-09" || inv.PaymentTerms != "Payable within 14 days without deduction." {
		t.Errorf("references = %+v", inv)
	}
	_, got := hs.do(http.MethodGet, "/"+draft.ID.String(), nil)
	if d := got["data"].(map[string]any); d["buyer_reference"] != "04011000-12345-34" {
		t.Errorf("GET buyer_reference = %v", d["buyer_reference"])
	}
}

func TestInvoiceHandler_PutRejectsOverlongReferences(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")
	for field, n := range map[string]int{"buyer_reference": 101, "purchase_order_ref": 101, "contract_ref": 101, "payment_terms": 501} {
		code, out := hs.putDraft(draft, map[string]any{field: strings.Repeat("x", n)})
		if code != http.StatusBadRequest || !strings.Contains(out["message"].(string), field+":") {
			t.Errorf("%s: status %d %v", field, code, out)
		}
	}
}

// A credit note copies the lines, unit codes included, so the reversal prints
// exactly what was invoiced.
func TestInvoiceHandler_CreditNoteKeepsLineUnitCodes(t *testing.T) {
	hs := newInvoiceHarness(t)
	draft := hs.createDraft("")
	if code, out := hs.putDraft(draft, map[string]any{"lines": twoLines()}); code != http.StatusOK {
		t.Fatalf("put: %d %v", code, out)
	}
	issued := hs.issue(hs.getInvoice(draft.ID))

	_, out := hs.do(http.MethodPost, "/"+issued.ID.String()+"/credit-note",
		map[string]any{"reason": "cancelled", "updated_at": issued.UpdatedAt})
	var res CreditNoteResult
	decode(t, out["data"], &res)
	lines := hs.getLines(res.CreditNote.ID)
	if len(lines) != 2 || lines[0].UnitCode != "LS" || lines[1].UnitCode != "KMT" {
		t.Errorf("credit note lines = %+v", lines)
	}
}

// --- real Postgres -----------------------------------------------------------

func TestIntegration_UpdateDraftReplacesLinesAtomically(t *testing.T) {
	requirePG(t)
	setBillingProfile(t, nil)
	svc := newTestInvoiceService()
	ctx := context.Background()
	gigID := insertTestGig(t, "EUR")

	draft, err := svc.CreateDraft(ctx, gigID, CreateInvoiceRequest{Customer: completeCustomer()})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	req := UpdateInvoiceRequest{
		Customer: draft.Customer, VATTreatment: draft.VATTreatment, TaxRateBps: 1900,
		SupplyDate: draft.SupplyDate, NumberPrefix: draft.NumberPrefix, UpdatedAt: draft.UpdatedAt,
		BuyerReference: "04011000-12345-34", PaymentTerms: "14 days net",
		Lines: []InvoiceLineInput{
			{Description: "DJ performance", Quantity: 1, UnitMinor: 100000, UnitCode: "LS"},
			{Description: "Travel", Quantity: 200, UnitMinor: 50, UnitCode: "KMT"},
		},
	}
	updated, err := svc.UpdateDraft(ctx, draft.ID, req)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.SubtotalMinor != 110000 || updated.TaxMinor != 20900 || updated.BuyerReference != "04011000-12345-34" {
		t.Errorf("updated = subtotal %d tax %d buyer_ref %q", updated.SubtotalMinor, updated.TaxMinor, updated.BuyerReference)
	}
	_, lines, err := svc.GetByID(ctx, draft.ID)
	if err != nil || len(lines) != 2 || lines[0].UnitCode != "LS" || lines[1].UnitCode != "KMT" || lines[1].TaxBps != 1900 {
		t.Fatalf("lines = %+v err=%v", lines, err)
	}

	// Issue, then credit: the credit note copies the lines with unit codes.
	issued, err := svc.Issue(ctx, draft.ID, IssueInvoiceRequest{UpdatedAt: updated.UpdatedAt})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	res, err := svc.CreditNote(ctx, issued.ID, CreditNoteRequest{Reason: "cancelled", UpdatedAt: issued.UpdatedAt})
	if err != nil {
		t.Fatalf("credit note: %v", err)
	}
	_, cnLines, _ := svc.GetByID(ctx, res.CreditNote.ID)
	if len(cnLines) != 2 || cnLines[1].UnitCode != "KMT" {
		t.Errorf("credit note lines = %+v", cnLines)
	}

	// An issued invoice is immutable: its lines can no longer be replaced.
	req.UpdatedAt = issued.UpdatedAt
	if _, err := svc.UpdateDraft(ctx, issued.ID, req); err == nil {
		t.Error("lines of an issued invoice were editable")
	}
}
