package email

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestIdentityCodec_RoundTrip(t *testing.T) {
	c := IdentityCodec{}
	ref := FieldRef{Table: "email_messages", Column: "subject", RowID: uuid.New()}

	sealed, err := c.Seal(context.Background(), nil, ref, "hello world")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if string(sealed) != "hello world" {
		t.Errorf("Seal is not a passthrough: %q", sealed)
	}
	opened, err := c.Open(context.Background(), nil, ref, sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opened != "hello world" {
		t.Errorf("Open = %q", opened)
	}
}

func TestIdentityCodec_SealEmail(t *testing.T) {
	c := IdentityCodec{}
	ref := FieldRef{Table: "email_messages", Column: "to_email", RowID: uuid.New()}
	enc, bidx, err := c.SealEmail(context.Background(), nil, ref, "dj@example.com")
	if err != nil {
		t.Fatalf("SealEmail: %v", err)
	}
	if string(enc) != "dj@example.com" {
		t.Errorf("enc = %q", enc)
	}
	if bidx != nil {
		t.Errorf("identity codec must not produce a blind index, got %q", bidx)
	}
}

// recordingCodec is a fake FieldCodec that records every call, so tests can
// assert exactly which fields the service seals on enqueue and unseals on
// the delivery path.
type recordingCodec struct {
	mu       sync.Mutex
	sealed   []string // "column=value"
	sealErr  error
	opened   []string
	openErr  error
	sealMail []string
}

func (c *recordingCodec) Seal(_ context.Context, _ pgx.Tx, f FieldRef, plaintext string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealErr != nil {
		return nil, c.sealErr
	}
	c.sealed = append(c.sealed, f.Column+"="+plaintext)
	return []byte("enc(" + plaintext + ")"), nil
}

func (c *recordingCodec) SealEmail(_ context.Context, _ pgx.Tx, f FieldRef, email string) ([]byte, []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealErr != nil {
		return nil, nil, c.sealErr
	}
	c.sealMail = append(c.sealMail, f.Column+"="+email)
	return []byte("enc(" + email + ")"), []byte("bidx(" + email + ")"), nil
}

func (c *recordingCodec) Open(_ context.Context, _ pgx.Tx, f FieldRef, sealed []byte) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openErr != nil {
		return "", c.openErr
	}
	c.opened = append(c.opened, f.Column+"="+string(sealed))
	// Reverse the fake sealing: enc(x) -> x
	s := string(sealed)
	if len(s) > 4 && s[:4] == "enc(" && s[len(s)-1] == ')' {
		return s[4 : len(s)-1], nil
	}
	return s, nil
}

// sealingFakeRepo emulates the tenant-scoped repository's contract: before
// the insert it invokes the FieldSealer (the service) so the outbox PII
// columns are sealed inside the transaction scope. The stored row keeps the
// sealed bytes; reads return them as stored (this fake does not unseal, so
// tests can assert exactly what would land in the sealed columns).
type sealingFakeRepo struct {
	*fakeEmailRepo
	sealer FieldSealer
}

func (r *sealingFakeRepo) Create(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	if r.sealer != nil {
		if _, err := r.sealer.SealMessageFields(ctx, nil, msg); err != nil {
			return nil, err
		}
	}
	return r.fakeEmailRepo.Create(ctx, msg)
}

// newSealingService builds a service + repo pair wired the way the promoter
// binary wires them: the service is the repository's FieldSealer and carries
// the envelope-backed codec.
func newSealingService(codec FieldCodec) (*EmailService, *sealingFakeRepo) {
	repo := &sealingFakeRepo{fakeEmailRepo: newFakeEmailRepo()}
	svc := NewEmailService(repo, newFakeSMTPSender()).WithFieldCodec(codec)
	repo.sealer = svc
	return svc, repo
}

// TestService_SealsOutboxFieldsOnEnqueue verifies that with an envelope-backed
// codec wired, the service seals the five outbox PII fields (to_email via
// SealEmail, subject, body, body_html, reply_to) before the repository insert.
func TestService_SealsOutboxFieldsOnEnqueue(t *testing.T) {
	codec := &recordingCodec{}
	svc, repo := newSealingService(codec)

	ownerID := uuid.New()
	msg, err := svc.Enqueue(context.Background(), CreateEmailRequest{
		Kind:      testKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "from@example.com",
		ToEmail:   "to@example.com",
		Subject:   "subj",
		Body:      "body",
		BodyHTML:  "<p>body</p>",
		ReplyTo:   "reply@example.com",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	wantSealed := map[string]string{
		"to_email":  "to@example.com",
		"subject":   "subj",
		"body":      "body",
		"body_html": "<p>body</p>",
		"reply_to":  "reply@example.com",
	}
	got := map[string]string{}
	for _, s := range codec.sealed {
		for col := range wantSealed {
			if len(s) > len(col) && s[:len(col)+1] == col+"=" {
				got[col] = s[len(col)+1:]
			}
		}
	}
	for col, v := range wantSealed {
		if col == "to_email" {
			continue // sealed via SealEmail, checked below
		}
		if got[col] != v {
			t.Errorf("sealed %s = %q, want %q (all: %v)", col, got[col], v, codec.sealed)
		}
	}
	if len(codec.sealMail) != 1 || codec.sealMail[0] != "to_email=to@example.com" {
		t.Errorf("SealEmail calls = %v", codec.sealMail)
	}

	// The stored row carries the sealed bytes, not the plaintext.
	stored, err := repo.GetByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ToEmail != "enc(to@example.com)" {
		t.Errorf("stored to_email = %q, want sealed", stored.ToEmail)
	}
	if stored.Subject != "enc(subj)" {
		t.Errorf("stored subject = %q, want sealed", stored.Subject)
	}
}

// TestService_IdentityCodecLeavesBytesUnchanged verifies the DJ-side default:
// with no codec wired (IdentityCodec), the message bytes are a plaintext
// passthrough — byte-identical to the pre-extraction behaviour.
func TestService_IdentityCodecLeavesBytesUnchanged(t *testing.T) {
	repo := newFakeEmailRepo()
	svc := NewEmailService(repo, newFakeSMTPSender()) // no WithFieldCodec

	msg, err := svc.Enqueue(context.Background(), CreateEmailRequest{
		Kind:      testKindInvoiceIssued,
		FromEmail: "from@example.com",
		ToEmail:   "to@example.com",
		Subject:   "subj",
		Body:      "body",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	stored, err := repo.GetByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ToEmail != "to@example.com" || stored.Subject != "subj" || stored.Body != "body" {
		t.Errorf("identity codec changed bytes: %+v", stored)
	}
}

// TestService_SealFailureAbortsEnqueue verifies a codec error surfaces as an
// enqueue error and nothing is persisted.
func TestService_SealFailureAbortsEnqueue(t *testing.T) {
	codec := &recordingCodec{sealErr: errors.New("dek unavailable")}
	svc, repo := newSealingService(codec)

	_, err := svc.Enqueue(context.Background(), CreateEmailRequest{
		Kind:      testKindInvoiceIssued,
		FromEmail: "from@example.com",
		ToEmail:   "to@example.com",
		Subject:   "subj",
		Body:      "body",
	})
	if err == nil {
		t.Fatal("expected seal error")
	}
	if len(repo.messages) != 0 {
		t.Errorf("a failed seal still persisted %d rows", len(repo.messages))
	}
}

// TestService_UnsealMessageFieldsRoundTrip verifies SealMessageFields and
// UnsealMessageFields are inverses through a fake codec.
func TestService_UnsealMessageFieldsRoundTrip(t *testing.T) {
	codec := &recordingCodec{}
	svc := NewEmailService(newFakeEmailRepo(), newFakeSMTPSender()).WithFieldCodec(codec)

	msg := &EmailMessage{
		ID:       uuid.New(),
		ToEmail:  "to@example.com",
		Subject:  "subj",
		Body:     "body",
		BodyHTML: "<p>b</p>",
		ReplyTo:  "reply@example.com",
	}
	if _, err := svc.SealMessageFields(context.Background(), nil, msg); err != nil {
		t.Fatalf("SealMessageFields: %v", err)
	}
	if msg.ToEmail == "to@example.com" || msg.Subject == "subj" {
		t.Fatalf("fields not sealed: %+v", msg)
	}
	if err := svc.UnsealMessageFields(context.Background(), nil, msg); err != nil {
		t.Fatalf("UnsealMessageFields: %v", err)
	}
	if msg.ToEmail != "to@example.com" || msg.Subject != "subj" || msg.Body != "body" ||
		msg.BodyHTML != "<p>b</p>" || msg.ReplyTo != "reply@example.com" {
		t.Errorf("round trip mismatch: %+v", msg)
	}
}

// TestService_IdentityCodecSealUnsealIsNoOp verifies the identity path of the
// service-level seal/unseal helpers is a passthrough.
func TestService_IdentityCodecSealUnsealIsNoOp(t *testing.T) {
	svc := NewEmailService(newFakeEmailRepo(), newFakeSMTPSender())
	msg := &EmailMessage{ID: uuid.New(), ToEmail: "to@example.com", Subject: "s", Body: "b"}
	if _, err := svc.SealMessageFields(context.Background(), nil, msg); err != nil {
		t.Fatal(err)
	}
	if msg.ToEmail != "to@example.com" || msg.Subject != "s" || msg.Body != "b" {
		t.Errorf("identity seal changed fields: %+v", msg)
	}
	if err := svc.UnsealMessageFields(context.Background(), nil, msg); err != nil {
		t.Fatal(err)
	}
	if msg.ToEmail != "to@example.com" {
		t.Errorf("identity unseal changed fields: %+v", msg)
	}
}
