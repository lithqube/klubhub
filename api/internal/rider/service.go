package rider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/settings"
)

// StorageIface is the thin Garage S3 interface the rider service needs.
// Same shape as epk.storageIface; duplicated locally to keep the package
// boundary clean.
type StorageIface interface {
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	PresignedGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
	DeleteObject(ctx context.Context, bucket, key string) error
}

// SettingsIface is the thin settings interface the rider service needs.
// Same shape as epk.settingsServiceIface; duplicated locally.
type SettingsIface interface {
	GetSettings(ctx context.Context) (*settings.UserSettings, error)
}

// Service is the business-logic layer over RepoIface. It is the seam the
// HTTP handler (Plan 03) depends on; Plan 03 declares its own
// package-internal ServiceIface that this struct satisfies by method set.
type Service struct {
	repo     RepoIface
	storage  StorageIface
	settings SettingsIface
}

// NewService constructs a Service with the given dependencies.
func NewService(repo RepoIface, storage StorageIface, settings SettingsIface) *Service {
	return &Service{repo: repo, storage: storage, settings: settings}
}

// StorageBucket is the Garage bucket rider exports are stored under.
// Same constant name as epk.storageBucket for symmetry; kept local because
// the rider package does not depend on epk.
const StorageBucket = "klubhub"

// PDFPresignedExpiry is how long a generated rider PDF's download URL
// remains valid. 15 minutes matches the EPK export pattern.
const PDFDownloadExpiry = 15 * time.Minute

// ExportResult mirrors epk.ExportResult shape: a frontend page that
// already renders an EPK export button can use the same component for
// rider PDF exports without a second downloadUrl consumer.
type ExportResult struct {
	ID          uuid.UUID `json:"id"`
	DownloadURL string    `json:"downloadUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ─── Templates ─────────────────────────────────────────────────────────────

// ListTemplates returns every non-deleted template, sorted by name.
func (s *Service) ListTemplates(ctx context.Context) ([]*RiderTemplate, error) {
	return s.repo.ListTemplates(ctx)
}

// GetTemplate returns a single non-deleted template by ID.
func (s *Service) GetTemplate(ctx context.Context, id uuid.UUID) (*RiderTemplate, error) {
	return s.repo.GetTemplate(ctx, id)
}

// CreateTemplate validates the name, persists a new template, and returns
// the populated record. The four section values default to empty when
// absent; that's the responsibility of the *Repository.CreateTemplate
// implementation, not duplicated here.
func (s *Service) CreateTemplate(ctx context.Context, in CreateTemplateInput) (*RiderTemplate, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	in.Name = strings.TrimSpace(in.Name)
	return s.repo.CreateTemplate(ctx, in)
}

// UpdateTemplate passes through to the repository. The repository handles
// nil-pointer partial updates; the service layer is intentionally thin
// for templates because validation lives in the input shape (pointer
// granularity).
func (s *Service) UpdateTemplate(ctx context.Context, id uuid.UUID, in UpdateTemplateInput) (*RiderTemplate, error) {
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: name cannot be blank", ErrInvalidInput)
		}
		in.Name = &trimmed
	}
	return s.repo.UpdateTemplate(ctx, id, in)
}

// DeleteTemplate soft-deletes a template.
func (s *Service) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDeleteTemplate(ctx, id)
}

// ─── Attachments ───────────────────────────────────────────────────────────

// GetAttachmentByGig returns the live attachment for a gig, or
// ErrNotFound when none exists.
func (s *Service) GetAttachmentByGig(ctx context.Context, gigID uuid.UUID) (*RiderAttachment, error) {
	return s.repo.GetAttachmentByGig(ctx, gigID)
}

// GetAttachment returns a single non-deleted attachment by ID.
func (s *Service) GetAttachment(ctx context.Context, id uuid.UUID) (*RiderAttachment, error) {
	return s.repo.GetAttachment(ctx, id)
}

// CreateAttachment implements the RIDER-02 "per-gig copy, not a live link"
// semantics: when TemplateID is set, the source template's current
// field values are copied into the new attachment row at insert time.
// After CreateAttachment returns, updating the source template never
// mutates the per-gig copy.
//
// When TemplateID is nil, the RiderSectionValues from in are used as-is
// (manual entry / "Start from blank").
func (s *Service) CreateAttachment(ctx context.Context, in CreateAttachmentInput) (*RiderAttachment, error) {
	if in.TemplateID != nil {
		tpl, err := s.repo.GetTemplate(ctx, *in.TemplateID)
		if err != nil {
			// ErrNotFound propagates — the client asked for a template
			// that doesn't exist (or is soft-deleted).
			return nil, err
		}
		// Copy template fields into the new row. This is the snapshot
		// step: subsequent changes to tpl do not affect the new
		// attachment because the values are now inlined into in.
		in.Technical = tpl.Technical
		in.Hospitality = tpl.Hospitality
		in.Backline = tpl.Backline
		in.OtherNotes = tpl.OtherNotes
		// in.TemplateID stays set — it's a record of where the copy
		// came from. RIDER-03 (override) does not change it.
	}
	return s.repo.CreateAttachment(ctx, in)
}

// UpdateAttachment passes through to the repository. Per-string partial
// update granularity is the repository's responsibility; the service
// layer is intentionally thin here.
func (s *Service) UpdateAttachment(ctx context.Context, id uuid.UUID, in UpdateAttachmentInput) (*RiderAttachment, error) {
	return s.repo.UpdateAttachment(ctx, id, in)
}

// DeleteAttachment soft-deletes a per-gig attachment.
func (s *Service) DeleteAttachment(ctx context.Context, id uuid.UUID) error {
	return s.repo.SoftDeleteAttachment(ctx, id)
}

// ─── PDF export ────────────────────────────────────────────────────────────

// GeneratePDF renders a per-gig rider as a one-page PDF and uploads it to
// Garage. Returns an ExportResult with a 15-minute presigned download URL.
//
// The PDF is overwritten on subsequent calls — there is no per-attachment
// export history in v1 (no rider_exports table; see Plan 02's trade-off
// note in 04.5-02-PLAN.md).
func (s *Service) GeneratePDF(ctx context.Context, attachmentID uuid.UUID) (*ExportResult, error) {
	att, err := s.repo.GetAttachment(ctx, attachmentID)
	if err != nil {
		return nil, err
	}

	// Load venue + date via gig.GigReader (interface). We don't import
	// the gig package directly to keep the boundary one-way; the
	// concrete reader is wired in Plan 03 (or Plan 04 at main.go).
	//
	// For now, fall back to safe defaults if no reader is configured:
	// "Unknown Venue" and time.Now(). This keeps the package compileable
	// before Plan 03 wires the reader.
	venueName, venueCity, venueCountry, gigDate := s.lookupGigContext(ctx, att.GigID)

	userSettings, err := s.settings.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get settings for PDF: %w", err)
	}

	var buf bytes.Buffer
	if err := renderRiderPDF(att, venueName, venueCity, venueCountry, gigDate, userSettings, &buf); err != nil {
		return nil, fmt.Errorf("render PDF: %w", err)
	}

	key := fmt.Sprintf("rider/exports/%s.pdf", attachmentID.String())
	pdfBytes := buf.Bytes()
	if err := s.storage.PutObject(ctx, StorageBucket, key, bytes.NewReader(pdfBytes), int64(len(pdfBytes)), "application/pdf"); err != nil {
		return nil, fmt.Errorf("store PDF: %w", err)
	}

	downloadURL, err := s.storage.PresignedGetObject(ctx, StorageBucket, key, PDFDownloadExpiry)
	if err != nil {
		return nil, fmt.Errorf("presign PDF URL: %w", err)
	}

	return &ExportResult{
		ID:          attachmentID,
		DownloadURL: downloadURL,
		CreatedAt:   time.Now(),
	}, nil
}

// lookupGigContext returns (venueName, venueCity, venueCountry, gigDate)
// for the gig this attachment belongs to. The default Service
// implementation has no gig reader wired — it returns placeholder values
// so the renderer compiles and the package is testable. Plan 03 replaces
// this with a method on a GigReader-aware service variant.
func (s *Service) lookupGigContext(_ context.Context, _ uuid.UUID) (name, city, country string, date time.Time) {
	return "Unknown Venue", "", "", time.Now()
}

// isBlank is a small helper exported for the PDF renderer.
func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}