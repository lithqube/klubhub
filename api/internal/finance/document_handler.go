package finance

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// DocumentReader is the read side of DocumentService.
type DocumentReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Document, error)
	ListByOwner(ctx context.Context, ownerType DocumentOwnerType, ownerID uuid.UUID) ([]*Document, error)
	Download(ctx context.Context, id uuid.UUID) (io.ReadCloser, *Document, error)
}

// DocumentHandler serves stored documents read-only:
//
//	GET /api/v1/finance/documents?owner_type=&owner_id=   all versions, newest first
//	GET /api/v1/finance/documents/{id}                    metadata
//	GET /api/v1/finance/documents/{id}/download           the file, always as an attachment
//
// There is deliberately no upload route: documents are written by the server
// when it archives something it produced (an issued invoice, an agreement),
// never from arbitrary client input, and they are never changed or deleted
// through HTTP.
type DocumentHandler struct {
	svc DocumentReader
}

// NewDocumentHandler wires a DocumentHandler.
func NewDocumentHandler(svc DocumentReader) *DocumentHandler {
	return &DocumentHandler{svc: svc}
}

// documentView is a document as the API shows it. The storage key is internal
// and never leaves the server.
type documentView struct {
	ID             uuid.UUID         `json:"id"`
	OwnerType      DocumentOwnerType `json:"owner_type"`
	OwnerID        uuid.UUID         `json:"owner_id"`
	Filename       string            `json:"filename"`
	MimeType       string            `json:"mime_type"`
	SizeBytes      int64             `json:"size_bytes"`
	ChecksumSHA256 string            `json:"checksum_sha256"`
	Version        int               `json:"version"`
	IsCurrent      bool              `json:"is_current"`
	CreatedAt      time.Time         `json:"created_at"`
}

func viewOf(d *Document) documentView {
	return documentView{
		ID: d.ID, OwnerType: d.OwnerType, OwnerID: d.OwnerID, Filename: d.Filename,
		MimeType: d.MimeType, SizeBytes: d.SizeBytes, ChecksumSHA256: d.ChecksumSHA256,
		Version: d.Version, IsCurrent: d.IsCurrent, CreatedAt: d.CreatedAt,
	}
}

func (h *DocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/finance/"
	if len(r.URL.Path) < len(prefix) || r.URL.Path[:len(prefix)] != prefix {
		writeError(w, http.StatusNotFound, "not_found", "invalid document path")
		return
	}
	parts := splitPath(r.URL.Path[len(prefix):])
	if len(parts) < 1 || len(parts) > 3 || parts[0] != "documents" {
		writeError(w, http.StatusNotFound, "not_found", "invalid document path")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "documents are read-only; only GET is supported")
		return
	}

	if len(parts) == 1 {
		h.handleList(w, r)
		return
	}
	id, err := uuid.Parse(parts[1])
	if err != nil || id == uuid.Nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid document id")
		return
	}
	switch {
	case len(parts) == 2:
		h.handleGet(w, r, id)
	case parts[2] == "download":
		h.handleDownload(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "not_found", "invalid document path")
	}
}

func (h *DocumentHandler) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ownerType := DocumentOwnerType(q.Get("owner_type"))
	ownerID, err := uuid.Parse(q.Get("owner_id"))
	if !ownerType.IsValid() || err != nil || ownerID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "owner_type and owner_id are required")
		return
	}
	docs, err := h.svc.ListByOwner(r.Context(), ownerType, ownerID)
	if err != nil {
		writeDocumentError(w, r, err)
		return
	}
	out := make([]documentView, 0, len(docs))
	for _, d := range docs {
		out = append(out, viewOf(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *DocumentHandler) handleGet(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	d, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeDocumentError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": viewOf(d)})
}

func (h *DocumentHandler) handleDownload(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	rc, d, err := h.svc.Download(r.Context(), id)
	if err != nil {
		writeDocumentError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()

	hdr := w.Header()
	hdr.Set("Content-Type", d.MimeType)
	hdr.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.Filename}))
	hdr.Set("Content-Length", strconv.FormatInt(d.SizeBytes, 10))
	hdr.Set("X-Checksum-SHA256", d.ChecksumSHA256)
	// Financial documents: never cached, never sniffed, never given a script origin.
	hdr.Set("Cache-Control", "private, no-store")
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func writeDocumentError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrDocumentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	writeInternalError(w, r, err)
}
