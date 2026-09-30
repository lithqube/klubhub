package rider

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors returned by the rider package.
var (
	// ErrNotFound is returned when a template, attachment, or referenced gig
	// does not exist (including when it has been soft-deleted).
	ErrNotFound = errors.New("rider record not found")

	// ErrConflict is returned when an attachment already exists for the same
	// gig. The partial unique index rider_attachments_one_per_gig enforces
	// "at most one live attachment per gig"; concurrent inserts race on the
	// index and the loser receives a 23505 SQLSTATE that the repository
	// translates into this error.
	ErrConflict = errors.New("rider attachment already exists for this gig")

	// ErrInvalidInput is returned when a required field is missing or
	// otherwise invalid (e.g. blank template name).
	ErrInvalidInput = errors.New("invalid rider input")
)

// RiderTemplate is a reusable four-section rider owned by the user. Soft-
// deleted templates remain in the table for forensics but are filtered from
// every read path.
type RiderTemplate struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Technical   string    `json:"technical"`
	Hospitality string    `json:"hospitality"`
	Backline    string    `json:"backline"`
	OtherNotes  string    `json:"otherNotes"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
}

// RiderAttachment is a per-gig copy of a rider. The TemplateID field
// records which template the copy was sourced from at insert time, but the
// four section fields are independent of the source template — updating the
// template does not change the attachment (RIDER-02 "per-gig copy, not a
// live link").
type RiderAttachment struct {
	ID          uuid.UUID  `json:"id"`
	GigID       uuid.UUID  `json:"gigId"`
	TemplateID  *uuid.UUID `json:"templateId"`
	Technical   string     `json:"technical"`
	Hospitality string     `json:"hospitality"`
	Backline    string     `json:"backline"`
	OtherNotes  string     `json:"otherNotes"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
}

// RiderSectionValues is the editable field set shared between templates and
// attachments. A non-nil pointer in Update*Input signals "user explicitly
// provided a value, including possibly empty"; a nil pointer signals "leave
// the column alone". This matches the EPK UpsertEPKContentRequest pattern.
type RiderSectionValues struct {
	Technical   string `json:"technical"`
	Hospitality string `json:"hospitality"`
	Backline    string `json:"backline"`
	OtherNotes  string `json:"otherNotes"`
}

// CreateTemplateInput is the request shape for creating a template. Name is
// required; the four section strings default to empty when absent.
type CreateTemplateInput struct {
	Name string `json:"name"`
	RiderSectionValues
}

// UpdateTemplateInput supports partial updates. Every field is a pointer so
// the caller can distinguish "set to empty string" from "leave alone".
// RiderSectionValues is non-nil when the user is updating any section; nil
// means "only the Name field is being updated".
type UpdateTemplateInput struct {
	Name *string `json:"name,omitempty"`
	*RiderSectionValues
}

// CreateAttachmentInput is the request shape for creating a per-gig copy.
// TemplateID is optional: when set, the service copies the template's
// current four-section values into the new row at insert time; when nil,
// the RiderSectionValues from this input are used as-is.
type CreateAttachmentInput struct {
	GigID      uuid.UUID `json:"gigId"`
	TemplateID *uuid.UUID `json:"templateId,omitempty"`
	RiderSectionValues
}

// UpdateAttachmentInput supports per-section overrides. nil pointers leave
// columns alone; the inline RiderSectionValues is nil-safe as a whole
// because each section is independently optional.
type UpdateAttachmentInput struct {
	*RiderSectionValues
}

// RepoIface is the contract the service layer consumes. All read paths
// filter WHERE deleted_at IS NULL; soft-delete sets deleted_at to now().
// Idempotent semantics: soft-delete returns ErrNotFound on the second call.
type RepoIface interface {
	// Templates
	ListTemplates(ctx context.Context) ([]*RiderTemplate, error)
	GetTemplate(ctx context.Context, id uuid.UUID) (*RiderTemplate, error)
	CreateTemplate(ctx context.Context, in CreateTemplateInput) (*RiderTemplate, error)
	UpdateTemplate(ctx context.Context, id uuid.UUID, in UpdateTemplateInput) (*RiderTemplate, error)
	SoftDeleteTemplate(ctx context.Context, id uuid.UUID) error

	// Attachments
	GetAttachmentByGig(ctx context.Context, gigID uuid.UUID) (*RiderAttachment, error)
	GetAttachment(ctx context.Context, id uuid.UUID) (*RiderAttachment, error)
	CreateAttachment(ctx context.Context, in CreateAttachmentInput) (*RiderAttachment, error)
	UpdateAttachment(ctx context.Context, id uuid.UUID, in UpdateAttachmentInput) (*RiderAttachment, error)
	SoftDeleteAttachment(ctx context.Context, id uuid.UUID) error
}