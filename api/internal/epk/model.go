package epk

import (
	"time"

	"github.com/google/uuid"
)

// EPKContent is the singleton row from epk_content table.
type EPKContent struct {
	ID                uuid.UUID
	BioShort          string
	BioLong           string
	TechRider         string
	StagePlotPath     string
	GigHighlights     []string
	PressQuotes       []PressQuote
	PhotoPaths        []string
	SectionVisibility map[string]bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PressQuote holds a single press quote with source attribution.
type PressQuote struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// EPKExport records a generated PDF export.
type EPKExport struct {
	ID uuid.UUID
	// GarageObjectKey is the Garage (S3-compatible) object key for the
	// exported PDF. Renamed from the legacy minio_path column in
	// migration 025. The legacy column is kept in the table for one
	// release cycle to allow zero-downtime backfill and rollback; the
	// Go repository/handler now write and read exclusively against
	// garage_object_key.
	GarageObjectKey string
	CreatedAt       time.Time
}

// UpsertEPKContentRequest carries partial update fields. Nil pointer = no change.
type UpsertEPKContentRequest struct {
	BioShort          *string
	BioLong           *string
	TechRider         *string
	StagePlotPath     *string
	GigHighlights     []string
	PressQuotes       []PressQuote
	PhotoPaths        []string
	SectionVisibility map[string]bool
}
