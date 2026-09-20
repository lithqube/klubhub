package tracklist

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Tracklist represents a collection of tracks
type Tracklist struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	Title           string     `db:"title" json:"title"`
	SourceFormat    string     `db:"source_format" json:"sourceFormat"`
	RawFilePath     string     `db:"raw_file_path" json:"rawFilePath"`
	Preset          string     `db:"preset" json:"preset"`
	VisibleFields   string     `db:"visible_fields" json:"-"` // JSONB stored as string for persistence
	BgMode          string     `db:"bg_mode" json:"bgMode"`
	BgValue         string     `db:"bg_value" json:"bgValue"`
	MaxTracks       int        `db:"max_tracks" json:"maxTracks"`
	TrackRangeStart int        `db:"track_range_start" json:"trackRangeStart"`
	TrackRangeEnd   int        `db:"track_range_end" json:"trackRangeEnd"`
	CreatedAt       time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt       *time.Time `db:"deleted_at" json:"-"`
}

// MarshalJSON exposes JSONB visible fields as their wire-level string array
// while keeping the persistence representation unchanged.
func (t Tracklist) MarshalJSON() ([]byte, error) {
	type tracklistAlias Tracklist
	var visibleFields []string
	if err := json.Unmarshal([]byte(t.VisibleFields), &visibleFields); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		tracklistAlias
		VisibleFields []string `json:"visibleFields"`
	}{
		tracklistAlias: tracklistAlias(t),
		VisibleFields:  visibleFields,
	})
}

// Track represents a single track in a tracklist
type Track struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	TracklistID     uuid.UUID  `db:"tracklist_id" json:"tracklistId"`
	Position        int        `db:"position" json:"position"`
	Title           string     `db:"title" json:"title"`
	Artist          string     `db:"artist" json:"artist"`
	Album           string     `db:"album" json:"album"`
	Genre           string     `db:"genre" json:"genre"`
	BPM             float64    `db:"bpm" json:"bpm"`
	Rating          int        `db:"rating" json:"rating"`
	DurationSeconds int        `db:"duration_secs" json:"durationSecs"`
	MusicalKey      string     `db:"musical_key" json:"musicalKey"`
	DateAdded       time.Time  `db:"date_added" json:"dateAdded"`
	ArtworkStatus   string     `db:"artwork_status" json:"artworkStatus"`
	ArtworkURL      string     `db:"artwork_url" json:"artworkUrl"`
	ArtworkSource   string     `db:"artwork_source" json:"artworkSource"`
	CreatedAt       time.Time  `db:"created_at" json:"-"`
	UpdatedAt       time.Time  `db:"updated_at" json:"-"`
	DeletedAt       *time.Time `db:"deleted_at" json:"-"`
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
	MusicalKey      *string    `json:"musicalKey"`
	DurationSeconds *int       `json:"durationSecs"`
	DateAdded       *time.Time `json:"dateAdded"`
}

// ArtworkStatus constants
const (
	ArtworkPending     = "pending"
	ArtworkFetched     = "fetched"
	ArtworkPlaceholder = "placeholder"
	ArtworkManual      = "manual"
)
