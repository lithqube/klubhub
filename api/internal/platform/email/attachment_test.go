package email

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// fakeAttachmentSource is an in-memory AttachmentSource (the shape
// docstore.AttachmentSource provides in production).
type fakeAttachmentSource struct {
	docs    map[uuid.UUID]*AttachmentDoc
	content map[uuid.UUID][]byte
}

func newFakeAttachmentSource() *fakeAttachmentSource {
	return &fakeAttachmentSource{docs: map[uuid.UUID]*AttachmentDoc{}, content: map[uuid.UUID][]byte{}}
}

func (f *fakeAttachmentSource) add(doc AttachmentDoc, content []byte) {
	cp := doc
	f.docs[doc.ID] = &cp
	f.content[doc.ID] = content
}

func (f *fakeAttachmentSource) GetByID(_ context.Context, id uuid.UUID) (*AttachmentDoc, error) {
	d, ok := f.docs[id]
	if !ok {
		return nil, ErrEmailNotFound
	}
	cp := *d
	return &cp, nil
}

func (f *fakeAttachmentSource) Download(_ context.Context, id uuid.UUID) (io.ReadCloser, *AttachmentDoc, error) {
	d, ok := f.docs[id]
	if !ok {
		return nil, nil, ErrEmailNotFound
	}
	content, ok := f.content[id]
	if !ok {
		return nil, nil, ErrEmailNotFound
	}
	cp := *d
	return io.NopCloser(bytes.NewReader(content)), &cp, nil
}

func ownedRequest(ownerType string, ownerID uuid.UUID, attachmentIDs ...uuid.UUID) CreateEmailRequest {
	return CreateEmailRequest{
		Kind:          testKindInvoiceIssued,
		OwnerType:     ownerType,
		OwnerID:       &ownerID,
		FromEmail:     "from@example.com",
		ToEmail:       "to@example.com",
		Subject:       "subj",
		Body:          "body",
		AttachmentIDs: attachmentIDs,
	}
}

func TestEmailStatus_IsValid(t *testing.T) {
	for _, s := range []EmailStatus{EmailStatusQueued, EmailStatusSending, EmailStatusSent, EmailStatusFailed, EmailStatusBounced} {
		if !s.IsValid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if EmailStatus("nonsense").IsValid() {
		t.Error("unknown status accepted")
	}
	if EmailStatus("").IsValid() {
		t.Error("empty status accepted")
	}
}

func TestPlunkFilename_Sanitizes(t *testing.T) {
	cases := map[string]string{
		`invoice.pdf`:            "invoice.pdf",
		"a\"b\rc\nd/e\\f.pdf":    "a_b_c_d_e_f.pdf", // real CR/LF and one backslash
		"":                       "attachment",
		strings.Repeat("x", 300): strings.Repeat("x", 255),
	}
	for in, want := range cases {
		if got := plunkFilename(in); got != want {
			t.Errorf("plunkFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSendOwned_RequiresAttachmentSource verifies a message that carries
// attachments is refused when the service has no document source wired.
func TestSendOwned_RequiresAttachmentSource(t *testing.T) {
	repo := newFakeEmailRepo()
	svc := NewEmailService(repo, newFakeSMTPSender()) // no WithAttachments
	ownerID := uuid.New()
	_, err := svc.SendOwned(context.Background(), ownedRequest("invoice", ownerID, uuid.New()))
	if !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("err = %v, want ErrEmailValidation", err)
	}
	if len(repo.messages) != 0 {
		t.Fatal("a refused message was stored")
	}
}

// TestSendOwned_RequiresOwner verifies attachments without an owner are
// refused: there is nothing to check ownership against.
func TestSendOwned_RequiresOwner(t *testing.T) {
	src := newFakeAttachmentSource()
	doc := AttachmentDoc{ID: uuid.New(), OwnerType: "invoice", OwnerID: uuid.New(), Filename: "a.pdf", MimeType: "application/pdf"}
	src.add(doc, []byte("pdf"))
	svc := NewEmailService(newFakeEmailRepo(), newFakeSMTPSender()).WithAttachments(src)

	req := ownedRequest("", uuid.Nil, doc.ID)
	req.OwnerType, req.OwnerID = "", nil
	if _, err := svc.SendOwned(context.Background(), req); !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("ownerless attachments accepted: %v", err)
	}
}

// TestSendOwned_RejectsForeignAttachment verifies the ownership guard: a
// message about entity A can never carry entity B's document.
func TestSendOwned_RejectsForeignAttachment(t *testing.T) {
	src := newFakeAttachmentSource()
	mine, theirs := uuid.New(), uuid.New()
	foreign := AttachmentDoc{ID: uuid.New(), OwnerType: "invoice", OwnerID: theirs, Filename: "theirs.pdf", MimeType: "application/pdf"}
	src.add(foreign, []byte("theirs"))

	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender).WithAttachments(src)

	_, err := svc.SendOwned(context.Background(), ownedRequest("invoice", mine, foreign.ID))
	if !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("foreign attachment accepted: %v", err)
	}
	if len(repo.messages) != 0 || sender.sentCount() != 0 {
		t.Fatal("a refused message was stored or sent")
	}
}

// TestSendOwned_RejectsUnknownAttachment verifies a missing document is a
// validation error before anything is stored or sent.
func TestSendOwned_RejectsUnknownAttachment(t *testing.T) {
	src := newFakeAttachmentSource()
	svc := NewEmailService(newFakeEmailRepo(), newFakeSMTPSender()).WithAttachments(src)
	_, err := svc.SendOwned(context.Background(), ownedRequest("invoice", uuid.New(), uuid.New()))
	if !errors.Is(err, ErrEmailValidation) {
		t.Fatalf("unknown attachment accepted: %v", err)
	}
}

// TestSendOwned_LoadsAttachmentsAtDelivery is the happy path: the message is
// stored with the attachment ids, and the content is loaded (and ownership
// re-checked) at delivery time, so a failed send retries with the same files.
func TestSendOwned_LoadsAttachmentsAtDelivery(t *testing.T) {
	src := newFakeAttachmentSource()
	ownerID := uuid.New()
	doc := AttachmentDoc{ID: uuid.New(), OwnerType: "invoice", OwnerID: ownerID, Filename: "invoice.pdf", MimeType: "application/pdf"}
	src.add(doc, []byte("%PDF-1.4 fake"))

	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender).WithAttachments(src)

	msg, err := svc.SendOwned(context.Background(), ownedRequest("invoice", ownerID, doc.ID))
	if err != nil {
		t.Fatalf("SendOwned: %v", err)
	}
	if msg.Status != EmailStatusSent {
		t.Fatalf("status = %q", msg.Status)
	}
	if sender.sentCount() != 1 {
		t.Fatalf("sent %d", sender.sentCount())
	}
	got := sender.sent[0]
	if len(got.Attachments) != 1 {
		t.Fatalf("attachments = %+v", got.Attachments)
	}
	att := got.Attachments[0]
	if att.Filename != "invoice.pdf" || att.MimeType != "application/pdf" || string(att.Content) != "%PDF-1.4 fake" {
		t.Errorf("attachment = %+v", att)
	}
	// The row keeps only the ids, never the content.
	stored, err := repo.GetByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.AttachmentIDs) != 1 || stored.AttachmentIDs[0] != doc.ID {
		t.Errorf("stored attachment ids = %v", stored.AttachmentIDs)
	}
	if len(stored.Attachments) != 0 {
		t.Errorf("content persisted on the row: %+v", stored.Attachments)
	}
}

// TestSendOwned_AttachmentSizeCap verifies the per-message size limit is
// enforced at delivery: nothing is sent without its attachments, and the row
// stays retryable (failed) rather than silently dropping the files.
func TestSendOwned_AttachmentSizeCap(t *testing.T) {
	src := newFakeAttachmentSource()
	ownerID := uuid.New()
	doc := AttachmentDoc{ID: uuid.New(), OwnerType: "invoice", OwnerID: ownerID, Filename: "big.pdf", MimeType: "application/pdf"}
	src.add(doc, make([]byte, maxEmailAttachmentBytes+1))

	repo := newFakeEmailRepo()
	sender := newFakeSMTPSender()
	svc := NewEmailService(repo, sender).WithAttachments(src)

	// Delivery failure is recorded on the row, not returned as an error
	// (same semantics as a provider failure): the message comes back failed
	// and nothing is sent.
	msg, err := svc.SendOwned(context.Background(), ownedRequest("invoice", ownerID, doc.ID))
	if err != nil {
		t.Fatalf("SendOwned: %v", err)
	}
	if sender.sentCount() != 0 {
		t.Fatal("oversized attachment was sent")
	}
	if msg.Status != EmailStatusFailed {
		t.Fatalf("status = %q, want failed (retryable)", msg.Status)
	}
	if !strings.Contains(msg.LastError, "exceed") {
		t.Errorf("last_error = %q, want the size-cap message", msg.LastError)
	}
	stored, err := repo.GetByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != EmailStatusFailed {
		t.Errorf("stored status = %q, want failed (retryable)", stored.Status)
	}
}

// TestEmailService_GetByIDAndListByOwner covers the read passthroughs.
func TestEmailService_GetByIDAndListByOwner(t *testing.T) {
	repo := newFakeEmailRepo()
	svc := NewEmailService(repo, newFakeSMTPSender())
	ownerID := uuid.New()

	msg, err := svc.Enqueue(context.Background(), CreateEmailRequest{
		Kind: testKindInvoiceIssued, OwnerType: "invoice", OwnerID: &ownerID,
		FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "s", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetByID(context.Background(), msg.ID)
	if err != nil || got.ID != msg.ID {
		t.Fatalf("GetByID: %+v, %v", got, err)
	}
	if _, err := svc.GetByID(context.Background(), uuid.New()); !errors.Is(err, ErrEmailNotFound) {
		t.Fatalf("GetByID missing: %v", err)
	}
	list, err := svc.ListByOwner(context.Background(), "invoice", ownerID)
	if err != nil || len(list) != 1 || list[0].ID != msg.ID {
		t.Fatalf("ListByOwner: %+v, %v", list, err)
	}
	if list, err := svc.ListByOwner(context.Background(), "invoice", uuid.New()); err != nil || len(list) != 0 {
		t.Fatalf("ListByOwner other: %+v, %v", list, err)
	}
}
