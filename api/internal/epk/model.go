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
	ID        uuid.UUID
	MinioPath string
	CreatedAt time.Time
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
