package docstore

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/email"
)

// AttachmentSource adapts DocumentService to email.AttachmentSource so the
// email service can load attachment content without importing docstore
// (which would create an import cycle: docstore imports email for the
// interface, email must not import docstore).
type AttachmentSource struct {
	Svc *DocumentService
}

var _ email.AttachmentSource = AttachmentSource{}

// GetByID implements email.AttachmentSource.
func (a AttachmentSource) GetByID(ctx context.Context, id uuid.UUID) (*email.AttachmentDoc, error) {
	d, err := a.Svc.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toAttachmentDoc(d), nil
}

// Download implements email.AttachmentSource.
func (a AttachmentSource) Download(ctx context.Context, id uuid.UUID) (io.ReadCloser, *email.AttachmentDoc, error) {
	rc, d, err := a.Svc.Download(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return rc, toAttachmentDoc(d), nil
}

func toAttachmentDoc(d *Document) *email.AttachmentDoc {
	return &email.AttachmentDoc{
		ID:        d.ID,
		OwnerType: string(d.OwnerType),
		OwnerID:   d.OwnerID,
		Filename:  d.Filename,
		MimeType:  d.MimeType,
	}
}
