package finance

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// The fakes below used to live in the moved test files (email_contract_test.go,
// document_contract_test.go, email_runtime_test.go, plunk_sender.go). They are
// finance-local copies so the staying finance tests keep compiling unchanged.
// All types they touch resolve through the platform aliases
// (platform_aliases.go), so they satisfy the platform interfaces.

// fakeSMTPSender is an in-memory SMTPSender for tests. Safe for concurrent
// use (RetryFailed's concurrency tests deliver from multiple goroutines).
type fakeSMTPSender struct {
	mu       sync.Mutex
	sent     []*EmailMessage
	sentIDs  map[uuid.UUID]int
	failNext bool
	err      error
}

func newFakeSMTPSender() *fakeSMTPSender {
	return &fakeSMTPSender{sent: make([]*EmailMessage, 0), sentIDs: make(map[uuid.UUID]int)}
}

func (f *fakeSMTPSender) Send(msg *EmailMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		return f.err
	}
	f.sent = append(f.sent, msg)
	f.sentIDs[msg.ID]++
	return nil
}

// Reset clears the failNext flag (call before tests that expect success).
func (f *fakeSMTPSender) Reset(failNext bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = failNext
	f.err = err
}

func (f *fakeSMTPSender) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// duplicateSends returns the message IDs that were delivered more than
// once, which must never happen for a correctly-claimed outbox.
func (f *fakeSMTPSender) duplicateSends() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	var dupes []uuid.UUID
	for id, n := range f.sentIDs {
		if n > 1 {
			dupes = append(dupes, id)
		}
	}
	return dupes
}

// fakeEmailRepo is an in-memory EmailRepository. It emulates the atomic
// claim semantics (SELECT ... FOR UPDATE SKIP LOCKED) that
// EmailRepository.ClaimFailedForRetry implements against Postgres by
// guarding the whole map with a mutex, so concurrent callers can never
// observe or mutate the same row at once.
type fakeEmailRepo struct {
	mu       sync.Mutex
	messages map[uuid.UUID]*EmailMessage
}

func newFakeEmailRepo() *fakeEmailRepo {
	return &fakeEmailRepo{messages: make(map[uuid.UUID]*EmailMessage)}
}

func (f *fakeEmailRepo) Create(ctx context.Context, msg *EmailMessage) (*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *msg
	f.messages[msg.ID] = &cp
	out := cp
	return &out, nil
}

func (f *fakeEmailRepo) GetByID(ctx context.Context, id uuid.UUID) (*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok {
		return nil, ErrEmailNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeEmailRepo) ListByOwner(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*EmailMessage
	for _, m := range f.messages {
		if m.OwnerType == ownerType && m.OwnerID != nil && *m.OwnerID == ownerID {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeEmailRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status EmailStatus, lastError string, sentAt *time.Time) (*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	if !ok {
		return nil, ErrEmailNotFound
	}
	m.Status = status
	m.LastError = lastError
	m.SentAt = sentAt
	m.UpdatedAt = time.Now().UTC()
	cp := *m
	return &cp, nil
}

func (f *fakeEmailRepo) ListByStatus(ctx context.Context, status EmailStatus, limit int) ([]*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*EmailMessage
	for _, m := range f.messages {
		if m.Status == status {
			cp := *m
			out = append(out, &cp)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ClaimFailedForRetry emulates the real repository's atomic
// UPDATE ... WHERE id IN (SELECT ... FOR UPDATE SKIP LOCKED) claim: while
// holding the map mutex it selects eligible 'failed' rows, flips them to
// 'sending' and increments attempts, then returns copies. Because the
// selection and mutation happen under the same lock, two goroutines
// calling this concurrently can never both claim the same row.
func (f *fakeEmailRepo) ClaimFailedForRetry(ctx context.Context, limit, maxAttempts, backoffBaseSeconds int) ([]*EmailMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var ids []uuid.UUID
	for id, m := range f.messages {
		if m.Status != EmailStatusFailed {
			continue
		}
		if m.Attempts >= maxAttempts {
			continue
		}
		backoff := time.Duration(m.Attempts*backoffBaseSeconds) * time.Second
		if time.Since(m.UpdatedAt) < backoff {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return f.messages[ids[i]].CreatedAt.Before(f.messages[ids[j]].CreatedAt)
	})
	if limit >= 0 && len(ids) > limit {
		ids = ids[:limit]
	}

	claimed := make([]*EmailMessage, 0, len(ids))
	for _, id := range ids {
		m := f.messages[id]
		m.Status = EmailStatusSending
		m.Attempts++
		m.UpdatedAt = time.Now().UTC()
		cp := *m
		claimed = append(claimed, &cp)
	}
	return claimed, nil
}

// RequeueStale emulates the real repository's reaper: rows stuck in
// 'sending' past olderThan are moved back to 'failed'. updated_at is
// deliberately left untouched so the row is immediately eligible for
// ClaimFailedForRetry's backoff check (it already waited longer than any
// backoff window).
func (f *fakeEmailRepo) RequeueStale(ctx context.Context, olderThan time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().UTC().Add(-olderThan)
	n := 0
	for _, m := range f.messages {
		if m.Status == EmailStatusSending && m.UpdatedAt.Before(cutoff) {
			m.Status = EmailStatusFailed
			m.LastError = "requeued: stuck in sending past timeout"
			n++
		}
	}
	return n, nil
}

// fakeObjectStore is an in-memory S3 replacement for tests.
type fakeObjectStore struct {
	objects map[string][]byte
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string][]byte)}
}

func (f *fakeObjectStore) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *fakeObjectStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeObjectStore) DeleteObject(ctx context.Context, bucket, key string) error {
	delete(f.objects, key)
	return nil
}

// fakeDocumentRepo is an in-memory replacement for DocumentRepository.
type fakeDocumentRepo struct {
	documents map[uuid.UUID]*Document
}

func newFakeDocumentRepo() *fakeDocumentRepo {
	return &fakeDocumentRepo{documents: make(map[uuid.UUID]*Document)}
}

func (f *fakeDocumentRepo) CreateWithDetails(ctx context.Context, req CreateDocumentRequest, details CreateDocumentDetails) (*Document, error) {
	// Mirror the real repository: compute the next version atomically
	// (from the fake's point of view) rather than trusting a caller-supplied
	// value, and mark any previous current document as no longer current.
	kind := req.Kind
	if kind == "" {
		kind = DocumentKindGeneral
	}
	nextVersion := 1
	for _, d := range f.documents {
		if d.OwnerType == req.OwnerType && d.OwnerID == req.OwnerID && d.Kind == kind {
			if d.Version >= nextVersion {
				nextVersion = d.Version + 1
			}
			if d.IsCurrent {
				d.IsCurrent = false
			}
		}
	}
	d := &Document{
		ID:             uuid.New(),
		OwnerType:      req.OwnerType,
		OwnerID:        req.OwnerID,
		Kind:           kind,
		StorageKey:     details.StorageKey,
		Filename:       req.Filename,
		MimeType:       req.MimeType,
		SizeBytes:      details.SizeBytes,
		ChecksumSHA256: details.ChecksumSHA256,
		Version:        nextVersion,
		UploadedBy:     req.UploadedBy,
		IsCurrent:      true,
		CreatedAt:      time.Now().UTC(),
	}
	f.documents[d.ID] = d
	return d, nil
}

func (f *fakeDocumentRepo) GetCurrent(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) (*Document, error) {
	var latest *Document
	for _, d := range f.documents {
		if d.OwnerType == ownerType && d.OwnerID == ownerID && d.Kind == DocumentKindGeneral {
			if latest == nil || d.Version > latest.Version {
				latest = d
			}
		}
	}
	if latest == nil {
		return nil, ErrDocumentNotFound
	}
	cp := *latest
	return &cp, nil
}

func (f *fakeDocumentRepo) GetByID(ctx context.Context, id uuid.UUID) (*Document, error) {
	d, ok := f.documents[id]
	if !ok {
		return nil, ErrDocumentNotFound
	}
	cp := *d
	return &cp, nil
}

func (f *fakeDocumentRepo) ListByOwner(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) ([]*Document, error) {
	var out []*Document
	for _, d := range f.documents {
		if d.OwnerType == ownerType && d.OwnerID == ownerID {
			cp := *d
			out = append(out, &cp)
		}
	}
	// Sort by version descending (newest first)
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[i].Version < out[j].Version {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// plunkRequest mirrors the JSON payload the Plunk sender posts, so tests can
// decode and assert on exactly what would hit the wire.
type plunkRequest struct {
	From    plunkContact `json:"from"`
	To      []string     `json:"to"`
	CC      []string     `json:"cc,omitempty"`
	BCC     []string     `json:"bcc,omitempty"`
	Subject string       `json:"subject"`
	// Reply is a single plain address; Plunk sets it as the Reply-To header.
	Reply    string `json:"reply,omitempty"`
	Body     string `json:"body,omitempty"`
	BodyHTML string `json:"bodyHtml,omitempty"`
	// Attachments follow Plunk's send schema: base64 content, at most 10 files
	// and 10 MB of base64 by default.
	Attachments []plunkAttachment `json:"attachments,omitempty"`
}

type plunkAttachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"contentType"`
	Disposition string `json:"disposition"`
}

type plunkContact struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

const runtimePlunkAck = `{"success":true,"data":{"emails":[{"contact":{"email":"to@example.com"},"email":"ac32f08e-c6b9-45d3-9824-a73dff1e3bbf"}],"timestamp":"2025-01-15T10:30:00.000Z"}}`

func runtimeEmailRequest() CreateEmailRequest {
	return CreateEmailRequest{Kind: EmailKindInvoiceIssued, FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "test", Body: "test"}
}
