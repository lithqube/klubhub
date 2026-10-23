package email

import (
	"context"
	"io"

	"github.com/google/uuid"
)

// AttachmentDoc is the minimal document shape the email service needs to
// guard attachment ownership. docstore.Document converts to it.
type AttachmentDoc struct {
	ID        uuid.UUID
	OwnerType string
	OwnerID   uuid.UUID
	Filename  string
	MimeType  string
}

// AttachmentSource is where attachment content comes from.
// docstore.AttachmentSource satisfies it.
type AttachmentSource interface {
	GetByID(ctx context.Context, id uuid.UUID) (*AttachmentDoc, error)
	Download(ctx context.Context, id uuid.UUID) (io.ReadCloser, *AttachmentDoc, error)
}
