package finance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeDocs struct {
	docs  map[uuid.UUID]*Document
	files map[uuid.UUID]string
	err   error
}

func (f *fakeDocs) GetByID(_ context.Context, id uuid.UUID) (*Document, error) {
	if f.err != nil {
		return nil, f.err
	}
	if d, ok := f.docs[id]; ok {
		return d, nil
	}
	return nil, ErrDocumentNotFound
}

func (f *fakeDocs) ListByOwner(_ context.Context, t DocumentOwnerType, id uuid.UUID) ([]*Document, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []*Document
	for _, d := range f.docs {
		if d.OwnerType == t && d.OwnerID == id {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeDocs) Download(ctx context.Context, id uuid.UUID) (io.ReadCloser, *Document, error) {
	d, err := f.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return io.NopCloser(strings.NewReader(f.files[id])), d, nil
}

func docHarness(t *testing.T) (*DocumentHandler, *fakeDocs, *Document) {
	t.Helper()
	d := &Document{
		ID: uuid.New(), OwnerType: DocumentOwnerInvoice, OwnerID: uuid.New(),
		StorageKey: "invoice/secret/key", Filename: "INV-1.pdf", MimeType: "application/pdf",
		SizeBytes: 9, ChecksumSHA256: "abc123", Version: 1, IsCurrent: true, CreatedAt: time.Now(),
	}
	f := &fakeDocs{docs: map[uuid.UUID]*Document{d.ID: d}, files: map[uuid.UUID]string{d.ID: "%PDF-body"}}
	return NewDocumentHandler(f), f, d
}

func getDoc(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestDocumentHandler_NeverExposesStorageKey(t *testing.T) {
	h, _, d := docHarness(t)
	for _, path := range []string{
		"/api/v1/finance/documents/" + d.ID.String(),
		"/api/v1/finance/documents?owner_type=invoice&owner_id=" + d.OwnerID.String(),
	} {
		rec := getDoc(h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "storage_key") || strings.Contains(rec.Body.String(), "secret/key") {
			t.Fatalf("%s leaked the storage key: %s", path, rec.Body)
		}
	}
}

func TestDocumentHandler_ListAndGet(t *testing.T) {
	h, _, d := docHarness(t)
	rec := getDoc(h, "/api/v1/finance/documents?owner_type=invoice&owner_id="+d.OwnerID.String())
	var list struct{ Data []documentView }
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Data) != 1 || list.Data[0].ID != d.ID || !list.Data[0].IsCurrent {
		t.Fatalf("list: %v %s", err, rec.Body)
	}
	// An owner with nothing is an empty array, not null.
	rec = getDoc(h, "/api/v1/finance/documents?owner_type=invoice&owner_id="+uuid.NewString())
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("empty list should be []: %s", rec.Body)
	}
}

func TestDocumentHandler_DownloadHeaders(t *testing.T) {
	h, _, d := docHarness(t)
	rec := getDoc(h, "/api/v1/finance/documents/"+d.ID.String()+"/download")
	if rec.Code != http.StatusOK || rec.Body.String() != "%PDF-body" {
		t.Fatalf("download: %d %q", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"Content-Type":            "application/pdf",
		"Content-Disposition":     `attachment; filename=INV-1.pdf`,
		"Content-Length":          "9",
		"Cache-Control":           "private, no-store",
		"X-Content-Type-Options":  "nosniff",
		"X-Checksum-SHA256":       "abc123",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestDocumentHandler_DownloadIsAlwaysAnAttachment(t *testing.T) {
	h, f, d := docHarness(t)
	d.MimeType, d.Filename = "text/html", "x.html"
	f.files[d.ID] = "<script>alert(1)</script>"
	rec := getDoc(h, "/api/v1/finance/documents/"+d.ID.String()+"/download?inline=1")
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition = %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestDocumentHandler_ReadOnlyAndErrors(t *testing.T) {
	h, _, d := docHarness(t)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, path := range []string{"/api/v1/finance/documents", "/api/v1/finance/documents/" + d.ID.String()} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(m, path, strings.NewReader("{}")))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d, want 405", m, path, rec.Code)
			}
		}
	}
	cases := map[string]int{
		"/api/v1/finance/documents/" + uuid.NewString():                          http.StatusNotFound,
		"/api/v1/finance/documents/" + uuid.NewString() + "/download":            http.StatusNotFound,
		"/api/v1/finance/documents/not-a-uuid":                                   http.StatusBadRequest,
		"/api/v1/finance/documents/" + d.ID.String() + "/other":                  http.StatusNotFound,
		"/api/v1/finance/documents":                                              http.StatusBadRequest,
		"/api/v1/finance/documents?owner_type=nope&owner_id=" + uuid.NewString(): http.StatusBadRequest,
		"/api/v1/finance/documents?owner_type=invoice&owner_id=not-a-uuid":       http.StatusBadRequest,
	}
	for path, want := range cases {
		if got := getDoc(h, path).Code; got != want {
			t.Errorf("GET %s = %d, want %d", path, got, want)
		}
	}
}

func TestDocumentHandler_ViaMux(t *testing.T) {
	h, _, d := docHarness(t)
	mux := NewMux(nil, nil, nil, h, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/finance/documents/"+d.ID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("mux: %d %s", rec.Code, rec.Body)
	}
}
