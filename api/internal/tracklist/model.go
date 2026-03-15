package tracklist

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Tracklist represents a collection of tracks
type Tracklist struct {
	ID              uuid.UUID  `db:"id"`
	Title           string     `db:"title"`
	SourceFormat    string     `db:"source_format"`
	RawFilePath     string     `db:"raw_file_path"`
	Preset          string     `db:"preset"`
	VisibleFields   string     `db:"visible_fields"` // JSONB stored as string for simplicity
	BgMode          string     `db:"bg_mode"`
	BgValue         string     `db:"bg_value"`
	MaxTracks       int        `db:"max_tracks"`
	TrackRangeStart int        `db:"track_range_start"`
	TrackRangeEnd   int        `db:"track_range_end"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	DeletedAt       *time.Time `db:"deleted_at"`
}

// Track represents a single track in a tracklist
type Track struct {
	ID              uuid.UUID  `db:"id"`
	TracklistID     uuid.UUID  `db:"tracklist_id"`
	Position        int        `db:"position"`
	Title           string     `db:"title"`
	Artist          string     `db:"artist"`
	Album           string     `db:"album"`
	Genre           string     `db:"genre"`
	BPM             float64    `db:"bpm"`
	Rating          int        `db:"rating"`
	DurationSeconds int        `db:"duration_secs"`
	MusicalKey      string     `db:"musical_key"`
	DateAdded       time.Time  `db:"date_added"`
	ArtworkStatus   string     `db:"artwork_status"`
	ArtworkURL      string     `db:"artwork_url"`
	ArtworkSource   string     `db:"artwork_source"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	DeletedAt       *time.Time `db:"deleted_at"`
}

// ParseResult contains the parsed tracks and any warnings
type ParseResult struct {
	Tracks   []Track
	Warnings []ParseWarning
}

// ParseWarning represents a warning encountered during parsing
type ParseWarning struct {
	Row     int
	Field   string
	Message string
}

// Sentinel errors
var (
	ErrInvalidFormat = errors.New("invalid_format")
	ErrZeroTracks    = errors.New("zero_tracks")
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
)

// UpdateTrackRequest contains fields that can be updated via PUT /{id}/tracks/{track_id}
type UpdateTrackRequest struct {
	Title           *string    `json:"title"`
	Artist          *string    `json:"artist"`
	Album           *string    `json:"album"`
	Genre           *string    `json:"genre"`
	BPM             *float64   `json:"bpm"`
	Rating          *int       `json:"rating"`
	MusicalKey      *string    `json:"key"`
	DurationSeconds *int       `json:"duration_secs"`
	DateAdded       *time.Time `json:"date_added"`
}

// ArtworkStatus constants
const (
	ArtworkPending     = "pending"
	ArtworkFetched     = "fetched"
	ArtworkPlaceholder = "placeholder"
	ArtworkManual      = "manual"
)
