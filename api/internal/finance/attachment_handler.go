package finance

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// AttachmentServiceIface is what the handler needs from AttachmentService.
type AttachmentServiceIface interface {
	Add(ctx context.Context, entryID uuid.UUID, filename string, content io.ReadSeeker) (*EntryAttachment, error)
	List(ctx context.Context, entryID uuid.UUID) ([]*EntryAttachment, error)
	Open(ctx context.Context, entryID, id uuid.UUID) (io.ReadCloser, *EntryAttachment, error)
	Remove(ctx context.Context, entryID, id uuid.UUID) error
}

// AttachmentHandler serves the receipts of a ledger entry:
//
//	POST   /entries/{id}/attachments            multipart, one "file" part → 201 {data}
//	GET    /entries/{id}/attachments            → {data: []}
//	GET    /entries/{id}/attachments/{aid}      the file (?inline=1 for images only)
//	DELETE /entries/{id}/attachments/{aid}      → 204
type AttachmentHandler struct{ svc AttachmentServiceIface }

// NewAttachmentHandler wires an AttachmentHandler.
func NewAttachmentHandler(svc AttachmentServiceIface) *AttachmentHandler {
	return &AttachmentHandler{svc: svc}
}

// multipartOverhead is the headroom above the file cap for the multipart
// framing; the file itself is checked against MaxAttachmentBytes exactly.
const multipartOverhead = 1 << 20

func (h *AttachmentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/finance/"
	if len(r.URL.Path) < len(prefix) || r.URL.Path[:len(prefix)] != prefix {
		writeError(w, http.StatusNotFound, "not_found", "invalid attachment path")
		return
	}
	parts := splitPath(r.URL.Path[len(prefix):])
	if len(parts) < 3 || len(parts) > 4 || parts[0] != "entries" || parts[2] != "attachments" {
		writeError(w, http.StatusNotFound, "not_found", "invalid attachment path")
		return
	}
	entryID, err := uuid.Parse(parts[1])
	if err != nil || entryID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid entry id")
		return
	}

	if len(parts) == 3 {
		switch r.Method {
		case http.MethodPost:
			h.handleUpload(w, r, entryID)
		case http.MethodGet:
			h.handleList(w, r, entryID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are supported")
		}
		return
	}

	id, err := uuid.Parse(parts[3])
	if err != nil || id == uuid.Nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid attachment id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.handleDownload(w, r, entryID, id)
	case http.MethodDelete:
		h.handleDelete(w, r, entryID, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and DELETE are supported")
	}
}

func (h *AttachmentHandler) handleUpload(w http.ResponseWriter, r *http.Request, entryID uuid.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentBytes+multipartOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "the file is larger than the limit")
		default:
			writeError(w, http.StatusBadRequest, "bad_request", "send the file as multipart/form-data in a \"file\" part")
		}
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "missing \"file\" part")
		return
	}
	defer func() { _ = file.Close() }()

	a, err := h.svc.Add(r.Context(), entryID, header.Filename, file)
	if err != nil {
		writeAttachmentError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": a})
}

func (h *AttachmentHandler) handleList(w http.ResponseWriter, r *http.Request, entryID uuid.UUID) {
	list, err := h.svc.List(r.Context(), entryID)
	if err != nil {
		writeAttachmentError(w, r, err)
		return
	}
	if list == nil {
		list = []*EntryAttachment{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *AttachmentHandler) handleDownload(w http.ResponseWriter, r *http.Request, entryID, id uuid.UUID) {
	rc, a, err := h.svc.Open(r.Context(), entryID, id)
	if err != nil {
		writeAttachmentError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()

	// Only images may be shown inline (thumbnails); a PDF is always a download.
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && a.MimeType != "application/pdf" {
		disposition = "inline"
	}
	hdr := w.Header()
	hdr.Set("Content-Type", a.MimeType)
	hdr.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Filename}))
	hdr.Set("Content-Length", strconv.FormatInt(a.SizeBytes, 10))
	// Financial documents: never cached, never sniffed, never given a script origin.
	hdr.Set("Cache-Control", "private, no-store")
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (h *AttachmentHandler) handleDelete(w http.ResponseWriter, r *http.Request, entryID, id uuid.UUID) {
	if err := h.svc.Remove(r.Context(), entryID, id); err != nil {
		writeAttachmentError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeAttachmentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrAttachmentType):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", err.Error())
	case errors.Is(err, ErrAttachmentTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", err.Error())
	case errors.Is(err, ErrAttachmentLimit):
		writeError(w, http.StatusConflict, "limit_reached", "an entry can hold at most "+strconv.Itoa(MaxAttachmentsPerEntry)+" files")
	case errors.Is(err, ErrAttachmentValidation):
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
	case errors.Is(err, ErrAttachmentNotFound):
		writeError(w, http.StatusNotFound, "not_found", "attachment not found")
	case errors.Is(err, ErrEntryNotFound):
		writeError(w, http.StatusNotFound, "not_found", "finance entry not found")
	case errors.Is(err, ErrEntryInactive):
		writeError(w, http.StatusConflict, "inactive", "voided finance entries cannot change their attachments")
	default:
		writeInternalError(w, r, err)
	}
}
