package finance

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// mailHarness is the archive harness with a real PlunkSender pointed at a
// recording server, so the exact request Plunk would receive can be checked.
type mailHarness struct {
	*archiveHarness
	emails *EmailService
	repo   *fakeEmailRepo
	mailer *InvoiceMailer
	server *httptest.Server
	sent   []plunkRequest
	status int
}

func newMailHarness(t *testing.T) *mailHarness {
	t.Helper()
	m := &mailHarness{archiveHarness: newArchiveHarness(t), repo: newFakeEmailRepo(), status: http.StatusOK}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req plunkRequest
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req)
		if m.status != http.StatusOK {
			w.WriteHeader(m.status)
			return
		}
		m.sent = append(m.sent, req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.ReplaceAll(runtimePlunkAck, "to@example.com", req.To[0])))
	}))
	t.Cleanup(m.server.Close)

	sender := NewPlunkSender(PlunkConfig{BaseURL: m.server.URL, APIKey: "k"})
	m.emails = NewEmailService(m.repo, sender).WithAttachments(m.docs)
	m.mailer = NewInvoiceMailer(m.svc, m.emails, "billing@klubhub.example", "KlubHub")
	m.h = NewInvoiceHandler(m.svc).WithMailer(m.mailer)
	return m
}

func (m *mailHarness) post(id uuid.UUID, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	m.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+id.String()+"/email", strings.NewReader(body)))
	return rec
}

func attachmentNames(r plunkRequest) []string {
	var out []string
	for _, a := range r.Attachments {
		out = append(out, a.Filename)
	}
	return out
}

func TestInvoiceEmail_SendsPDFAndEInvoiceXML(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(nil)

	rec := m.post(inv.ID, `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out struct{ Data EmailMessage }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Data.Status != EmailStatusSent {
		t.Fatalf("message = %+v, %v", out.Data, err)
	}
	if len(m.sent) != 1 {
		t.Fatalf("provider calls = %d", len(m.sent))
	}
	req := m.sent[0]

	// What Plunk receives: base64 content in its own schema.
	if len(req.Attachments) != 2 {
		t.Fatalf("attachments = %v", attachmentNames(req))
	}
	pdf, xml := req.Attachments[0], req.Attachments[1]
	if pdf.ContentType != "application/pdf" || xml.ContentType != "application/xml" || pdf.Disposition != "attachment" {
		t.Errorf("types = %s %s %s", pdf.ContentType, xml.ContentType, pdf.Disposition)
	}
	if xml.Filename != inv.Number()+"-einvoice.xml" {
		t.Errorf("xml filename = %q", xml.Filename)
	}
	rawPDF, err := base64.StdEncoding.DecodeString(pdf.Content)
	if err != nil || !bytes.HasPrefix(rawPDF, []byte("%PDF")) {
		t.Errorf("pdf content is not base64 PDF bytes: %v", err)
	}
	// ...and they are byte-for-byte the archived documents.
	docs := m.kinds(inv.ID)
	if string(rawPDF) != m.content(docs[DocumentKindInvoicePDF]) {
		t.Error("attached PDF differs from the archived one")
	}
	rawXML, _ := base64.StdEncoding.DecodeString(xml.Content)
	if string(rawXML) != m.content(docs[DocumentKindEInvoiceXML]) {
		t.Error("attached XML differs from the archived one")
	}

	// Default recipient, subject and text.
	if req.To[0] != "buchhaltung@club-hamburg.example" || !strings.Contains(req.Subject, inv.Number()) {
		t.Errorf("to=%v subject=%q", req.To, req.Subject)
	}
	for _, want := range []string{"E-Rechnung", "e-invoice", "Guten Tag"} {
		if !strings.Contains(req.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, req.Body)
		}
	}
	// The row keeps ids, never content.
	stored, _ := m.repo.GetByID(context.Background(), out.Data.ID)
	if len(stored.AttachmentIDs) != 2 || stored.Attachments != nil {
		t.Errorf("stored = %+v", stored)
	}
}

func TestInvoiceEmail_PDFOnlyWhenAsked(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(nil)
	if rec := m.post(inv.ID, `{"include_einvoice":false,"to_email":"other@client.example"}`); rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	req := m.sent[0]
	if len(req.Attachments) != 1 || req.To[0] != "other@client.example" || strings.Contains(req.Body, "E-Rechnung") {
		t.Errorf("to=%v attachments=%v body=%q", req.To, attachmentNames(req), req.Body)
	}
}

func TestInvoiceEmail_UnexportableInvoiceSendsPDFAlone(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(map[string]any{"withholding_rate_bps": 1500}) // no e-invoice possible
	if rec := m.post(inv.ID, `{}`); rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := attachmentNames(m.sent[0]); len(got) != 1 || !strings.HasSuffix(got[0], ".pdf") {
		t.Errorf("attachments = %v", got)
	}
	// Insisting on the e-invoice is refused rather than silently dropped.
	before := len(m.sent)
	rec := m.post(inv.ID, `{"include_einvoice":true}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "not_exportable") || len(m.sent) != before {
		t.Errorf("status %d, sent %d: %s", rec.Code, len(m.sent)-before, rec.Body)
	}
}

func TestInvoiceEmail_FillsInAMissingEInvoiceBeforeSending(t *testing.T) {
	m := newMailHarness(t)
	m.gen.err = errors.New("generator down")
	inv := m.issueViaService(nil) // PDF only archived
	if len(m.kinds(inv.ID)) != 1 {
		t.Fatalf("kinds = %v", m.kinds(inv.ID))
	}
	m.gen.err = nil
	if rec := m.post(inv.ID, `{}`); rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(m.sent[0].Attachments) != 2 {
		t.Errorf("attachments = %v", attachmentNames(m.sent[0]))
	}
}

func TestInvoiceEmail_CreditNote(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(nil)
	cur := m.getInvoice(inv.ID)
	res, err := m.svc.CreditNote(context.Background(), cur.ID, CreditNoteRequest{Reason: "wrong amount", UpdatedAt: cur.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if rec := m.post(res.CreditNote.ID, `{}`); rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	req := m.sent[0]
	if !strings.Contains(req.Subject, "Credit note") || !strings.Contains(req.Subject, "Gutschrift") || strings.Contains(req.Body, "due ") {
		t.Errorf("subject=%q body=%q", req.Subject, req.Body)
	}
}

func TestInvoiceEmail_Refusals(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(nil)
	draft := m.createDraft("")

	cases := []struct {
		name string
		id   uuid.UUID
		body string
		want int
	}{
		{"draft", draft.ID, `{}`, http.StatusConflict},
		{"unknown invoice", uuid.New(), `{}`, http.StatusNotFound},
		{"bad recipient", inv.ID, `{"to_email":"not an address"}`, http.StatusBadRequest},
		{"header injection in recipient", inv.ID, "{\"to_email\":\"a@b.example\\r\\nBcc: evil@x.example\"}", http.StatusBadRequest},
		{"header injection in subject", inv.ID, "{\"subject\":\"hi\\r\\nBcc: evil@x.example\"}", http.StatusBadRequest},
		{"unknown field", inv.ID, `{"attachment_ids":["` + uuid.NewString() + `"]}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := m.post(c.id, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
	}
	if len(m.sent) != 0 {
		t.Errorf("a refused request still sent %d email(s)", len(m.sent))
	}
}

func TestInvoiceEmail_CustomerWithoutEmailNeedsARecipient(t *testing.T) {
	m := newMailHarness(t)
	draft := m.createDraft("")
	if code, out := m.putDraft(draft, map[string]any{"customer": completeCustomer()}); code != 200 {
		t.Fatalf("put: %d %v", code, out)
	}
	cur := m.getInvoice(draft.ID)
	inv, err := m.svc.Issue(context.Background(), cur.ID, IssueInvoiceRequest{UpdatedAt: cur.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Customer.Email != "" {
		t.Skip("fixture customer has an email")
	}
	rec := m.post(inv.ID, `{}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "to_email") {
		t.Errorf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestInvoiceEmail_FailedDeliveryIsStoredAndRetriedWithItsAttachments(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(nil)

	m.status = http.StatusInternalServerError
	rec := m.post(inv.ID, `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out struct{ Data EmailMessage }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Data.Status != EmailStatusFailed || len(m.sent) != 0 {
		t.Fatalf("message = %+v", out.Data)
	}

	// The provider recovers; the retry loads the same two files again.
	m.status = http.StatusOK
	msg, _ := m.repo.GetByID(context.Background(), out.Data.ID)
	msg.Status, msg.Attempts = EmailStatusSending, 2
	got, err := m.emails.finalizeDelivery(context.Background(), msg)
	if err != nil || got.Status != EmailStatusSent {
		t.Fatalf("retry: %+v, %v", got, err)
	}
	if len(m.sent) != 1 || len(m.sent[0].Attachments) != 2 {
		t.Fatalf("retry sent %d email(s)", len(m.sent))
	}
}

func TestInvoiceEmail_UnavailableWithoutMailOrStorage(t *testing.T) {
	hs := newEInvoiceHarness(t)
	inv := hs.issued(nil)

	// No mailer at all.
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/finance/invoices/"+inv.ID.String()+"/email", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no mailer: %d", rec.Code)
	}
	// A mailer but no archive to attach from.
	mailer := NewInvoiceMailer(hs.svc, NewEmailService(newFakeEmailRepo(), newFakeSMTPSender()), "a@b.example", "")
	if _, err := mailer.SendInvoiceEmail(context.Background(), inv.ID, EmailInvoiceRequest{}); !errors.Is(err, ErrEmailUnavailable) {
		t.Errorf("no archive: %v", err)
	}
	// No sender address.
	a := newArchiveHarness(t)
	mailer = NewInvoiceMailer(a.svc, NewEmailService(newFakeEmailRepo(), newFakeSMTPSender()), "", "")
	if _, err := mailer.SendInvoiceEmail(context.Background(), uuid.New(), EmailInvoiceRequest{}); !errors.Is(err, ErrEmailUnavailable) {
		t.Errorf("no sender: %v", err)
	}
}

func TestInvoiceEmail_OnlyPOST(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(nil)
	rec := httptest.NewRecorder()
	m.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/finance/invoices/"+inv.ID.String()+"/email", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", rec.Code)
	}
}

// --- the email service: who may attach what ----------------------------------

func TestSendOwned_AttachmentsMustBelongToTheOwner(t *testing.T) {
	m := newMailHarness(t)
	mine, theirs := m.issueViaService(nil), m.issueViaService(nil)
	others := m.kinds(theirs.ID)[DocumentKindInvoicePDF]

	ownerID := mine.ID
	req := CreateEmailRequest{
		Kind: EmailKindInvoiceIssued, OwnerType: "invoice", OwnerID: &ownerID,
		FromEmail: "a@b.example", ToEmail: "c@d.example", Subject: "s", Body: "b",
		AttachmentIDs: []uuid.UUID{others.ID},
	}
	if _, err := m.emails.SendOwned(context.Background(), req); !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("another invoice's document was accepted: %v", err)
	}
	if len(m.repo.messages) != 0 || len(m.sent) != 0 {
		t.Fatal("a refused message was stored or sent")
	}

	// Without an owner there is nothing to check against: refused too.
	req.OwnerType, req.OwnerID = "", nil
	req.AttachmentIDs = []uuid.UUID{m.kinds(mine.ID)[DocumentKindInvoicePDF].ID}
	if _, err := m.emails.SendOwned(context.Background(), req); !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("ownerless attachment accepted: %v", err)
	}
}

func TestSendOwned_ClientRequestsStillCannotCarryAttachments(t *testing.T) {
	m := newMailHarness(t)
	inv := m.issueViaService(nil)
	req := runtimeEmailRequest()
	req.AttachmentIDs = []uuid.UUID{m.kinds(inv.ID)[DocumentKindInvoicePDF].ID}
	if _, err := m.emails.Send(context.Background(), req); !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("generic send accepted attachment_ids: %v", err)
	}
	if _, err := m.emails.Enqueue(context.Background(), req); !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("generic enqueue accepted attachment_ids: %v", err)
	}
}

// --- the Plunk sender ---------------------------------------------------------

func TestPlunkSender_AttachmentLimitsAndNames(t *testing.T) {
	var got plunkRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(runtimePlunkAck))
	}))
	defer server.Close()
	sender := NewPlunkSender(PlunkConfig{BaseURL: server.URL, APIKey: "k"})
	base := func() *EmailMessage {
		return &EmailMessage{ID: uuid.New(), FromEmail: "a@b.example", ToEmail: "to@example.com", Subject: "s", Body: "b"}
	}
	attach := func(m *EmailMessage, name string, size int) {
		m.AttachmentIDs = append(m.AttachmentIDs, uuid.New())
		m.Attachments = append(m.Attachments, EmailAttachment{Filename: name, MimeType: "application/pdf", Content: bytes.Repeat([]byte{1}, size)})
	}

	// Filenames Plunk would reject are cleaned, not sent through.
	m := base()
	attach(m, "a\"b\r\nc/d.pdf", 10)
	if err := sender.Send(m); err != nil {
		t.Fatal(err)
	}
	if got.Attachments[0].Filename != "a_b__c_d.pdf" {
		t.Errorf("filename = %q", got.Attachments[0].Filename)
	}

	// More than 10 files, or more than 10 MB of base64: refused before the provider.
	got = plunkRequest{}
	m = base()
	for i := 0; i < 11; i++ {
		attach(m, "f.pdf", 1)
	}
	if err := sender.Send(m); !errors.Is(err, ErrEmailValidation) || got.Subject != "" {
		t.Errorf("11 attachments: %v (provider saw %q)", err, got.Subject)
	}
	m = base()
	attach(m, "big.pdf", 8<<20)
	if err := sender.Send(m); !errors.Is(err, ErrEmailValidation) || got.Subject != "" {
		t.Errorf("oversize: %v (provider saw %q)", err, got.Subject)
	}
}

// --- reply-to ----------------------------------------------------------------

func TestInvoiceEmail_RepliesGoToTheSupplier(t *testing.T) {
	m := newMailHarness(t)
	contact := m.billing.profile.ContactEmail
	inv := m.issueViaService(nil)

	rec := m.post(inv.ID, `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out struct{ Data EmailMessage }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)

	if contact == "" || m.sent[0].Reply != contact {
		t.Fatalf("Plunk reply = %q, want the supplier contact %q", m.sent[0].Reply, contact)
	}
	if m.sent[0].From.Email != "billing@klubhub.example" {
		t.Errorf("from = %q: the sender must stay the verified platform address", m.sent[0].From.Email)
	}
	if out.Data.ReplyTo != contact {
		t.Errorf("response reply_to = %q", out.Data.ReplyTo)
	}
	stored, _ := m.repo.GetByID(context.Background(), out.Data.ID)
	if stored.ReplyTo != contact {
		t.Errorf("stored reply_to = %q", stored.ReplyTo)
	}
}

func TestInvoiceEmail_ReplyToIsOmittedNotAnError(t *testing.T) {
	for name, contact := range map[string]string{
		"missing":   "",
		"malformed": "not an address",
		"is sender": "billing@klubhub.example",
	} {
		t.Run(name, func(t *testing.T) {
			m := newMailHarness(t)
			m.billing.profile.ContactEmail = contact
			inv := m.issueViaService(nil)
			if rec := m.post(inv.ID, `{}`); rec.Code != http.StatusCreated {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if m.sent[0].Reply != "" {
				t.Errorf("reply = %q, want none", m.sent[0].Reply)
			}
		})
	}
}

func TestInvoiceEmail_RetryKeepsTheReplyTo(t *testing.T) {
	m := newMailHarness(t)
	contact := m.billing.profile.ContactEmail
	inv := m.issueViaService(nil)

	m.status = http.StatusInternalServerError
	var out struct{ Data EmailMessage }
	_ = json.Unmarshal(m.post(inv.ID, `{}`).Body.Bytes(), &out)

	m.status = http.StatusOK
	msg, _ := m.repo.GetByID(context.Background(), out.Data.ID)
	msg.Status, msg.Attempts = EmailStatusSending, 2
	if _, err := m.emails.finalizeDelivery(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 1 || m.sent[0].Reply != contact {
		t.Errorf("retry reply = %v, want %q", m.sent, contact)
	}
}

func TestPlunkSender_ReplyIsABareValidAddress(t *testing.T) {
	var got plunkRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(runtimePlunkAck))
	}))
	defer server.Close()
	sender := NewPlunkSender(PlunkConfig{BaseURL: server.URL, APIKey: "k"})
	msg := func(reply string) *EmailMessage {
		return &EmailMessage{ID: uuid.New(), FromEmail: "a@b.example", ToEmail: "to@example.com", Subject: "s", Body: "b", ReplyTo: reply}
	}

	// Plunk's field takes an address, not "Name <address>".
	if err := sender.Send(msg("Lina Vasquez <lina@dj.example>")); err != nil {
		t.Fatal(err)
	}
	if got.Reply != "lina@dj.example" {
		t.Errorf("reply = %q", got.Reply)
	}
	got = plunkRequest{}
	if err := sender.Send(msg("")); err != nil || got.Reply != "" {
		t.Errorf("no reply-to: err=%v reply=%q", err, got.Reply)
	}
	got = plunkRequest{}
	if err := sender.Send(msg("nonsense")); !errors.Is(err, ErrEmailValidation) || got.Subject != "" {
		t.Errorf("invalid reply-to: %v (provider saw %q)", err, got.Subject)
	}
}

func TestEmailRequest_ReplyToIsValidated(t *testing.T) {
	req := runtimeEmailRequest()
	for name, bad := range map[string]string{"not an address": "nope", "header injection": "a@b.example\r\nBcc: evil@x.example"} {
		req.ReplyTo = bad
		if err := ValidateCreateEmailRequest(req); !errors.Is(err, ErrEmailValidation) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	req.ReplyTo = "ok@dj.example"
	if err := ValidateCreateEmailRequest(req); err != nil {
		t.Errorf("valid reply-to rejected: %v", err)
	}
}

// Every query that lists the email columns also carries reply_to.
func TestIntegration_EmailRepo_ReplyToRoundTrips(t *testing.T) {
	requireEntryPG(t)
	ctx := context.Background()
	repo := NewEmailRepository(testPool)
	owner := uuid.New()
	created, err := repo.Create(ctx, &EmailMessage{
		Kind: EmailKindInvoiceIssued, OwnerType: "invoice", OwnerID: &owner,
		FromEmail: "a@b.example", ToEmail: "c@d.example", Subject: "s", Body: "b",
		ReplyTo: "lina@dj.example", Status: EmailStatusFailed, Attempts: 1,
	})
	if err != nil || created.ReplyTo != "lina@dj.example" {
		t.Fatalf("create: %+v, %v", created, err)
	}
	if got, err := repo.GetByID(ctx, created.ID); err != nil || got.ReplyTo != "lina@dj.example" {
		t.Errorf("get: %+v, %v", got, err)
	}
	if list, err := repo.ListByOwner(ctx, "invoice", owner); err != nil || len(list) != 1 || list[0].ReplyTo != "lina@dj.example" {
		t.Errorf("list by owner: %+v, %v", list, err)
	}
	if list, err := repo.ListByStatus(ctx, EmailStatusFailed, 1000); err != nil {
		t.Errorf("list by status: %v", err)
	} else {
		found := false
		for _, m := range list {
			found = found || m.ID == created.ID && m.ReplyTo == "lina@dj.example"
		}
		if !found {
			t.Error("list by status lost the reply-to")
		}
	}
	if upd, err := repo.UpdateStatus(ctx, created.ID, EmailStatusSent, "", nil); err != nil || upd.ReplyTo != "lina@dj.example" {
		t.Errorf("update: %+v, %v", upd, err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE email_messages SET status = 'failed', attempts = 1, updated_at = now() - interval '1 hour' WHERE id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimFailedForRetry(ctx, 1000, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range claimed {
		found = found || m.ID == created.ID && m.ReplyTo == "lina@dj.example"
	}
	if !found {
		t.Error("claimed retry lost the reply-to")
	}
}
