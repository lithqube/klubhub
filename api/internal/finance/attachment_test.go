package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- fixtures ---------------------------------------------------------------

var (
	jpegBytes = append([]byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), bytes.Repeat([]byte{1}, 64)...)
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{2}, 64)...)
	webpBytes = append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{3}, 64)...)
	pdfBytes  = append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 64)...)
)

// attStore is an in-memory ObjectStore.
type attStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	putErr  error
}

func newAttStore() *attStore { return &attStore{objects: map[string][]byte{}} }

func (s *attStore) PutObject(_ context.Context, _, key string, r io.Reader, _ int64, _ string) error {
	if s.putErr != nil {
		return s.putErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.objects[key] = b
	s.mu.Unlock()
	return nil
}

func (s *attStore) GetObject(_ context.Context, _, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.objects[key]
	if !ok {
		return nil, errors.New("no such key")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (s *attStore) DeleteObject(_ context.Context, _, key string) error {
	s.mu.Lock()
	delete(s.objects, key)
	s.mu.Unlock()
	return nil
}

func (s *attStore) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.objects) }

// attRepo is an in-memory AttachmentRepositoryIface. entries maps an entry id
// to its status; a missing id is not found.
type attRepo struct {
	mu      sync.Mutex
	entries map[uuid.UUID]EntryStatus
	rows    []*StoredAttachment
	failAdd error
}

func newAttRepo() *attRepo { return &attRepo{entries: map[uuid.UUID]EntryStatus{}} }

func (r *attRepo) addEntry(status EntryStatus) uuid.UUID {
	id := uuid.New()
	r.entries[id] = status
	return id
}

func (r *attRepo) Ensure(_ context.Context, entryID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.entries[entryID]
	if !ok {
		return ErrEntryNotFound
	}
	if st == EntryStatusVoided {
		return ErrEntryInactive
	}
	return nil
}

func (r *attRepo) Create(ctx context.Context, in NewAttachment) (*StoredAttachment, error) {
	if err := r.Ensure(ctx, in.EntryID); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failAdd != nil {
		return nil, r.failAdd
	}
	n := 0
	for _, a := range r.rows {
		if a.EntryID == in.EntryID {
			n++
		}
	}
	if n >= MaxAttachmentsPerEntry {
		return nil, ErrAttachmentLimit
	}
	a := &StoredAttachment{
		EntryAttachment: EntryAttachment{ID: uuid.New(), EntryID: in.EntryID, Filename: in.Filename, MimeType: in.MimeType,
			SizeBytes: in.SizeBytes, ChecksumSHA256: in.ChecksumSHA256, CreatedAt: time.Now().UTC()},
		StorageKey: in.StorageKey,
	}
	r.rows = append(r.rows, a)
	return a, nil
}

func (r *attRepo) List(_ context.Context, entryID uuid.UUID) ([]*EntryAttachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[entryID]; !ok {
		return nil, ErrEntryNotFound
	}
	out := []*EntryAttachment{}
	for _, a := range r.rows {
		if a.EntryID == entryID {
			cp := a.EntryAttachment
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *attRepo) Get(_ context.Context, entryID, id uuid.UUID) (*StoredAttachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.rows {
		if a.ID == id && a.EntryID == entryID {
			cp := *a
			return &cp, nil
		}
	}
	return nil, ErrAttachmentNotFound
}

func (r *attRepo) Delete(ctx context.Context, entryID, id uuid.UUID) (string, error) {
	if err := r.Ensure(ctx, entryID); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, a := range r.rows {
		if a.ID == id && a.EntryID == entryID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return a.StorageKey, nil
		}
	}
	return "", ErrAttachmentNotFound
}

type attHarness struct {
	t     *testing.T
	repo  *attRepo
	store *attStore
	svc   *AttachmentService
	h     http.Handler
	entry uuid.UUID
}

func newAttHarness(t *testing.T) *attHarness {
	t.Helper()
	repo, store := newAttRepo(), newAttStore()
	svc := NewAttachmentService(repo, store, "bucket")
	return &attHarness{t: t, repo: repo, store: store, svc: svc, h: NewAttachmentHandler(svc), entry: repo.addEntry(EntryStatusActive)}
}

func (hs *attHarness) path(entry uuid.UUID, rest ...string) string {
	p := "/api/v1/finance/entries/" + entry.String() + "/attachments"
	for _, r := range rest {
		p += "/" + r
	}
	return p
}

// upload posts a multipart "file" part.
func (hs *attHarness) upload(entry uuid.UUID, filename string, content []byte) *httptest.ResponseRecorder {
	hs.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		hs.t.Fatal(err)
	}
	_, _ = fw.Write(content)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, hs.path(entry), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec
}

func (hs *attHarness) do(method, path string) *httptest.ResponseRecorder {
	hs.t.Helper()
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func decodeAttachment(t *testing.T, rec *httptest.ResponseRecorder) EntryAttachment {
	t.Helper()
	var out struct {
		Data EntryAttachment `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return out.Data
}

// --- service rules ------------------------------------------------------------

func TestAttachmentService_AcceptsReceiptFormatsByContent(t *testing.T) {
	for name, content := range map[string][]byte{
		"photo.jpg": jpegBytes, "scan.png": pngBytes, "shot.webp": webpBytes, "bill.pdf": pdfBytes,
	} {
		hs := newAttHarness(t)
		a, err := hs.svc.Add(context.Background(), hs.entry, name, bytes.NewReader(content))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if a.SizeBytes != int64(len(content)) || a.ChecksumSHA256 == "" || a.Filename != name {
			t.Errorf("%s: stored %+v", name, a)
		}
	}
}

func TestAttachmentService_DetectsTypeFromBytesNotFilename(t *testing.T) {
	hs := newAttHarness(t)
	// A PDF named .jpg is stored as the PDF it is.
	a, err := hs.svc.Add(context.Background(), hs.entry, "receipt.jpg", bytes.NewReader(pdfBytes))
	if err != nil || a.MimeType != "application/pdf" {
		t.Fatalf("got %+v, %v", a, err)
	}
	// HTML, SVG and executables renamed to .jpg/.pdf are refused outright.
	for name, content := range map[string][]byte{
		"x.jpg": []byte("<html><script>alert(1)</script></html>"),
		"y.png": []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`),
		"z.pdf": []byte("MZ\x90\x00\x03\x00\x00\x00 executable"),
		"t.pdf": []byte("just some text, not a pdf at all"),
	} {
		if _, err := hs.svc.Add(context.Background(), hs.entry, name, bytes.NewReader(content)); !errors.Is(err, ErrAttachmentType) {
			t.Errorf("%s: err = %v, want ErrAttachmentType", name, err)
		}
	}
	if hs.store.count() != 1 {
		t.Errorf("rejected files reached storage: %d objects", hs.store.count())
	}
}

func TestAttachmentService_RejectsEmptyAndOversize(t *testing.T) {
	hs := newAttHarness(t)
	if _, err := hs.svc.Add(context.Background(), hs.entry, "e.pdf", bytes.NewReader(nil)); !errors.Is(err, ErrAttachmentValidation) {
		t.Errorf("empty: %v", err)
	}
	big := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), MaxAttachmentBytes)...) // header + cap bytes > cap
	if _, err := hs.svc.Add(context.Background(), hs.entry, "big.pdf", bytes.NewReader(big)); !errors.Is(err, ErrAttachmentTooLarge) {
		t.Errorf("oversize: %v", err)
	}
	exact := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), MaxAttachmentBytes-9)...)
	if _, err := hs.svc.Add(context.Background(), hs.entry, "ok.pdf", bytes.NewReader(exact)); err != nil {
		t.Errorf("a file exactly at the cap must be accepted: %v", err)
	}
}

func TestAttachmentService_EnforcesPerEntryLimit(t *testing.T) {
	hs := newAttHarness(t)
	for i := 0; i < MaxAttachmentsPerEntry; i++ {
		if _, err := hs.svc.Add(context.Background(), hs.entry, fmt.Sprintf("r%d.pdf", i), bytes.NewReader(pdfBytes)); err != nil {
			t.Fatalf("file %d: %v", i, err)
		}
	}
	before := hs.store.count()
	if _, err := hs.svc.Add(context.Background(), hs.entry, "extra.pdf", bytes.NewReader(pdfBytes)); !errors.Is(err, ErrAttachmentLimit) {
		t.Fatalf("err = %v, want ErrAttachmentLimit", err)
	}
	if hs.store.count() != before {
		t.Errorf("the refused upload left an orphan object in storage")
	}
}

func TestAttachmentService_UnknownAndVoidedEntries(t *testing.T) {
	hs := newAttHarness(t)
	if _, err := hs.svc.Add(context.Background(), uuid.New(), "a.pdf", bytes.NewReader(pdfBytes)); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("unknown entry: %v", err)
	}
	voided := hs.repo.addEntry(EntryStatusVoided)
	if _, err := hs.svc.Add(context.Background(), voided, "a.pdf", bytes.NewReader(pdfBytes)); !errors.Is(err, ErrEntryInactive) {
		t.Errorf("voided entry: %v", err)
	}
	if hs.store.count() != 0 {
		t.Errorf("nothing may be stored for a refused entry, got %d objects", hs.store.count())
	}
}

func TestAttachmentService_CleansUpOrphanObjectWhenRecordFails(t *testing.T) {
	hs := newAttHarness(t)
	hs.repo.failAdd = errors.New("db down")
	if _, err := hs.svc.Add(context.Background(), hs.entry, "a.pdf", bytes.NewReader(pdfBytes)); err == nil {
		t.Fatal("expected the repo error")
	}
	if hs.store.count() != 0 {
		t.Errorf("object left behind after a failed record insert")
	}
}

func TestAttachmentService_StorageFailureRecordsNothing(t *testing.T) {
	hs := newAttHarness(t)
	hs.store.putErr = errors.New("garage down")
	if _, err := hs.svc.Add(context.Background(), hs.entry, "a.pdf", bytes.NewReader(pdfBytes)); err == nil {
		t.Fatal("expected the storage error")
	}
	if rows, _ := hs.repo.List(context.Background(), hs.entry); len(rows) != 0 {
		t.Errorf("a record exists for a file that was never stored: %+v", rows)
	}
}

func TestSanitizeAttachmentFilename(t *testing.T) {
	long := strings.Repeat("a", 300) + ".pdf"
	for in, want := range map[string]string{
		"receipt.pdf":                    "receipt.pdf",
		`C:\Users\me\Documents\bill.pdf`: "bill.pdf",
		"../../etc/passwd":               "passwd",
		"  spaced name.jpg ":             "spaced name.jpg",
		"tab\tand\nnewline.png":          "tabandnewline.png",
		"":                               "receipt",
		"   ":                            "receipt",
		"..":                             "receipt",
		"Rechnung Größe – Café.pdf":      "Rechnung Größe – Café.pdf",
	} {
		if got := sanitizeAttachmentFilename(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sanitizeAttachmentFilename(long); len([]rune(got)) > 200 {
		t.Errorf("long name not truncated: %d runes", len([]rune(got)))
	}
}

// --- HTTP contract ----------------------------------------------------------------

func TestAttachmentHandler_UploadReturnsMetadataWithoutStorageKey(t *testing.T) {
	hs := newAttHarness(t)
	rec := hs.upload(hs.entry, "Rewe 2026-10-02.jpg", jpegBytes)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	a := decodeAttachment(t, rec)
	if a.ID == uuid.Nil || a.EntryID != hs.entry || a.MimeType != "image/jpeg" || a.Filename != "Rewe 2026-10-02.jpg" {
		t.Errorf("attachment = %+v", a)
	}
	if strings.Contains(rec.Body.String(), "storage_key") || strings.Contains(rec.Body.String(), "finance/entries") {
		t.Errorf("internal storage location leaked: %s", rec.Body.String())
	}
}

func TestAttachmentHandler_UploadErrors(t *testing.T) {
	hs := newAttHarness(t)
	cases := []struct {
		name string
		rec  *httptest.ResponseRecorder
		code int
		err  string
	}{
		{"unsupported type", hs.upload(hs.entry, "x.jpg", []byte("<html></html>")), http.StatusUnsupportedMediaType, "unsupported_media_type"},
		{"empty file", hs.upload(hs.entry, "e.pdf", nil), http.StatusBadRequest, "validation_failed"},
		{"unknown entry", hs.upload(uuid.New(), "a.pdf", pdfBytes), http.StatusNotFound, "not_found"},
		{"voided entry", hs.upload(hs.repo.addEntry(EntryStatusVoided), "a.pdf", pdfBytes), http.StatusConflict, "inactive"},
	}
	for _, tc := range cases {
		var out map[string]any
		_ = json.Unmarshal(tc.rec.Body.Bytes(), &out)
		if tc.rec.Code != tc.code || out["error"] != tc.err {
			t.Errorf("%s: %d %v, want %d %s", tc.name, tc.rec.Code, out, tc.code, tc.err)
		}
	}
}

func TestAttachmentHandler_RejectsOversizeBodyEarly(t *testing.T) {
	hs := newAttHarness(t)
	big := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), MaxAttachmentBytes+1024)...)
	rec := hs.upload(hs.entry, "big.pdf", big)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusRequestEntityTooLarge || out["error"] != "too_large" {
		t.Errorf("%d %v, want 413 too_large", rec.Code, out)
	}
	if hs.store.count() != 0 {
		t.Errorf("oversize upload reached storage")
	}
}

func TestAttachmentHandler_MissingFilePart(t *testing.T) {
	hs := newAttHarness(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("note", "no file here")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, hs.path(hs.entry), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
	// And a non-multipart body.
	req = httptest.NewRequest(http.MethodPost, hs.path(hs.entry), strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("json body: status %d, want 400", rec.Code)
	}
}

func TestAttachmentHandler_ListAndLimit(t *testing.T) {
	hs := newAttHarness(t)
	for i := 0; i < MaxAttachmentsPerEntry; i++ {
		if rec := hs.upload(hs.entry, fmt.Sprintf("r%d.pdf", i), pdfBytes); rec.Code != http.StatusCreated {
			t.Fatalf("upload %d: %d", i, rec.Code)
		}
	}
	rec := hs.upload(hs.entry, "extra.pdf", pdfBytes)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusConflict || out["error"] != "limit_reached" {
		t.Errorf("11th upload: %d %v, want 409 limit_reached", rec.Code, out)
	}

	list := hs.do(http.MethodGet, hs.path(hs.entry))
	var got struct {
		Data []EntryAttachment `json:"data"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &got)
	if list.Code != http.StatusOK || len(got.Data) != MaxAttachmentsPerEntry {
		t.Errorf("list: %d, %d items", list.Code, len(got.Data))
	}
	if rec := hs.do(http.MethodGet, hs.path(uuid.New())); rec.Code != http.StatusNotFound {
		t.Errorf("list for unknown entry: %d", rec.Code)
	}
}

func TestAttachmentHandler_DownloadIsAnAttachmentAndNeverSniffed(t *testing.T) {
	hs := newAttHarness(t)
	a := decodeAttachment(t, hs.upload(hs.entry, "Rechnung März.pdf", pdfBytes))

	rec := hs.do(http.MethodGet, hs.path(hs.entry, a.ID.String()))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), pdfBytes) {
		t.Fatalf("status %d, body %d bytes", rec.Code, rec.Body.Len())
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/pdf" {
		t.Errorf("Content-Type = %q", h.Get("Content-Type"))
	}
	if cd := h.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, "Rechnung") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "private, no-store" {
		t.Errorf("missing hardening headers: %v", h)
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("Content-Security-Policy = %q, want a sandbox policy", h.Get("Content-Security-Policy"))
	}
	// The PDF never renders inline, whatever the query says.
	rec = hs.do(http.MethodGet, hs.path(hs.entry, a.ID.String())+"?inline=1")
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("a PDF was offered inline: %q", cd)
	}
}

func TestAttachmentHandler_ImagesCanBeShownInline(t *testing.T) {
	hs := newAttHarness(t)
	a := decodeAttachment(t, hs.upload(hs.entry, "photo.jpg", jpegBytes))
	rec := hs.do(http.MethodGet, hs.path(hs.entry, a.ID.String())+"?inline=1")
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "inline;") {
		t.Errorf("Content-Disposition = %q, want inline for thumbnails", cd)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestAttachmentHandler_DownloadIsScopedToItsEntry(t *testing.T) {
	hs := newAttHarness(t)
	a := decodeAttachment(t, hs.upload(hs.entry, "a.pdf", pdfBytes))
	other := hs.repo.addEntry(EntryStatusActive)
	if rec := hs.do(http.MethodGet, hs.path(other, a.ID.String())); rec.Code != http.StatusNotFound {
		t.Errorf("another entry's attachment id resolved: %d", rec.Code)
	}
	if rec := hs.do(http.MethodGet, hs.path(hs.entry, uuid.NewString())); rec.Code != http.StatusNotFound {
		t.Errorf("unknown attachment: %d", rec.Code)
	}
}

func TestAttachmentHandler_DeleteRemovesRecordAndObject(t *testing.T) {
	hs := newAttHarness(t)
	a := decodeAttachment(t, hs.upload(hs.entry, "a.pdf", pdfBytes))
	if rec := hs.do(http.MethodDelete, hs.path(hs.entry, a.ID.String())); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if hs.store.count() != 0 {
		t.Errorf("object not removed from storage")
	}
	if rec := hs.do(http.MethodGet, hs.path(hs.entry, a.ID.String())); rec.Code != http.StatusNotFound {
		t.Errorf("deleted attachment still downloads: %d", rec.Code)
	}
	if rec := hs.do(http.MethodDelete, hs.path(hs.entry, a.ID.String())); rec.Code != http.StatusNotFound {
		t.Errorf("second delete: %d, want 404", rec.Code)
	}
}

func TestAttachmentHandler_VoidedEntryAttachmentsAreKept(t *testing.T) {
	hs := newAttHarness(t)
	a := decodeAttachment(t, hs.upload(hs.entry, "a.pdf", pdfBytes))
	hs.repo.entries[hs.entry] = EntryStatusVoided // the entry is voided later

	// Evidence stays readable and downloadable, but cannot be removed.
	if rec := hs.do(http.MethodGet, hs.path(hs.entry, a.ID.String())); rec.Code != http.StatusOK {
		t.Errorf("download after void: %d", rec.Code)
	}
	if rec := hs.do(http.MethodDelete, hs.path(hs.entry, a.ID.String())); rec.Code != http.StatusConflict {
		t.Errorf("delete after void: %d, want 409", rec.Code)
	}
	if hs.store.count() != 1 {
		t.Errorf("a voided entry's receipt was deleted from storage")
	}
}

// The mux sends /entries/{id}/attachments to the attachment handler and every
// other entries route to the entry handler; with no attachment handler it
// answers 503 for the former only.
func TestMux_RoutesAttachments(t *testing.T) {
	mark := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("X-Handler", name) })
	}
	id := uuid.NewString()
	serve := func(m *Mux, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	m := NewMux(nil, nil, nil, nil, nil, nil, nil, mark("entries")).WithAttachments(mark("attachments"))
	for path, want := range map[string]string{
		"/api/v1/finance/entries/" + id + "/attachments":                     "attachments",
		"/api/v1/finance/entries/" + id + "/attachments/" + uuid.NewString(): "attachments",
		"/api/v1/finance/entries/" + id:                                      "entries",
		"/api/v1/finance/entries/" + id + "/void":                            "entries",
		"/api/v1/finance/entries":                                            "entries",
	} {
		if got := serve(m, path).Header().Get("X-Handler"); got != want {
			t.Errorf("%s routed to %q, want %q", path, got, want)
		}
	}

	bare := NewMux(nil, nil, nil, nil, nil, nil, nil, mark("entries"))
	if rec := serve(bare, "/api/v1/finance/entries/"+id+"/attachments"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("without storage: %d, want 503", rec.Code)
	}
	if rec := serve(bare, "/api/v1/finance/entries/"+id); rec.Header().Get("X-Handler") != "entries" {
		t.Errorf("entries must keep working without attachments")
	}
}

func TestAttachmentHandler_Routing(t *testing.T) {
	hs := newAttHarness(t)
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodPut, hs.path(hs.entry), http.StatusMethodNotAllowed},
		{http.MethodPost, hs.path(hs.entry, uuid.NewString()), http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/finance/entries/not-a-uuid/attachments", http.StatusBadRequest},
		{http.MethodGet, hs.path(hs.entry, "not-a-uuid"), http.StatusBadRequest},
		{http.MethodGet, hs.path(hs.entry, uuid.NewString(), "extra"), http.StatusNotFound},
		{http.MethodGet, "/api/v1/finance/entries/" + hs.entry.String(), http.StatusNotFound},
	} {
		if rec := hs.do(tc.method, tc.path); rec.Code != tc.code {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.path, rec.Code, tc.code)
		}
	}
}
