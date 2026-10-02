package tracklist

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

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
	ID              uuid.UUID `db:"id" json:"id"`
	TracklistID     uuid.UUID `db:"tracklist_id" json:"tracklistId"`
	Position        int       `db:"position" json:"position"`
	Title           string    `db:"title" json:"title"`
	Artist          string    `db:"artist" json:"artist"`
	Album           string    `db:"album" json:"album"`
	Genre           string    `db:"genre" json:"genre"`
	BPM             float64   `db:"bpm" json:"bpm"`
	Rating          int       `db:"rating" json:"rating"`
	DurationSeconds int       `db:"duration_secs" json:"durationSecs"`
	MusicalKey      string    `db:"musical_key" json:"musicalKey"`
	DateAdded       time.Time `db:"date_added" json:"dateAdded"`
	ArtworkStatus   string    `db:"artwork_status" json:"artworkStatus"`
	ArtworkURL      string    `db:"artwork_url" json:"artworkUrl"`
	ArtworkSource   string    `db:"artwork_source" json:"artworkSource"`
	// Markers shown on the exported card: HiddenGem conceals the title and
	// artist, Unreleased tags the track, Media is vinyl or digital.
	HiddenGem  bool       `db:"hidden_gem" json:"hiddenGem"`
	Unreleased bool       `db:"unreleased" json:"unreleased"`
	Media      string     `db:"media" json:"media"`
	CreatedAt  time.Time  `db:"created_at" json:"-"`
	UpdatedAt  time.Time  `db:"updated_at" json:"-"`
	DeletedAt  *time.Time `db:"deleted_at" json:"-"`
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
	ErrFileTooLarge  = errors.New("file_too_large")
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrInvalidTitle  = errors.New("invalid_title")
)

// MaxTitleChars is the longest tracklist title the API accepts (in characters).
const MaxTitleChars = 200

// UpdateTracklistRequest is the body of PUT /{id}: today only the title can be renamed.
type UpdateTracklistRequest struct {
	Title *string `json:"title"`
}

// NormalizeTitle trims a tracklist title and rejects an empty, over-long or
// NUL-containing one (Postgres refuses NUL in text).
func NormalizeTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > MaxTitleChars || strings.ContainsRune(title, 0) {
		return "", ErrInvalidTitle
	}
	return title, nil
}

// LinkedGig is the slice of a gig shown next to a tracklist it is linked to.
type LinkedGig struct {
	ID        uuid.UUID `json:"id"`
	Date      time.Time `json:"date"`
	Venue     string    `json:"venue"`
	City      string    `json:"city"`
	EventName string    `json:"eventName"`
}

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
	HiddenGem       *bool      `json:"hiddenGem"`
	Unreleased      *bool      `json:"unreleased"`
	Media           *string    `json:"media"`
}

// Media values: what a track was played from. "" means not set.
const (
	MediaVinyl   = "vinyl"
	MediaDigital = "digital"
)

// ValidMedia reports whether m is "" (not set), vinyl or digital.
func ValidMedia(m string) bool {
	switch m {
	case "", MediaVinyl, MediaDigital:
		return true
	}
	return false
}

// ErrInvalidTrack is returned for a track that fails validation (blank title,
// over-long text, unknown media, a bad reorder list).
var ErrInvalidTrack = errors.New("invalid_track")

// MaxTrackTextChars bounds the title and artist of a manually added track.
const MaxTrackTextChars = 200

// CreateTrackRequest is the body of POST /{id}/tracks: a track added by hand.
// It is appended after the last track; reorder afterwards if it belongs elsewhere.
type CreateTrackRequest struct {
	Title      string   `json:"title"`
	Artist     string   `json:"artist"`
	BPM        *float64 `json:"bpm"`
	MusicalKey string   `json:"musicalKey"`
	Media      string   `json:"media"`
	HiddenGem  bool     `json:"hiddenGem"`
	Unreleased bool     `json:"unreleased"`
}

// Normalize trims the text fields and validates the request.
func (r CreateTrackRequest) Normalize() (CreateTrackRequest, error) {
	r.Title = strings.TrimSpace(r.Title)
	r.Artist = strings.TrimSpace(r.Artist)
	r.MusicalKey = strings.TrimSpace(r.MusicalKey)
	bad := r.Title == "" ||
		utf8.RuneCountInString(r.Title) > MaxTrackTextChars ||
		utf8.RuneCountInString(r.Artist) > MaxTrackTextChars ||
		utf8.RuneCountInString(r.MusicalKey) > 10 ||
		strings.ContainsRune(r.Title+r.Artist+r.MusicalKey, 0) ||
		!ValidMedia(r.Media) ||
		(r.BPM != nil && (*r.BPM < 0 || *r.BPM > 999))
	if bad {
		return r, ErrInvalidTrack
	}
	return r, nil
}

// ReorderTracksRequest is the body of PUT /{id}/tracks/order: every live track
// id of the tracklist, in the new order.
type ReorderTracksRequest struct {
	TrackIDs []uuid.UUID `json:"trackIds"`
}

// ArtworkStatus constants
const (
	ArtworkPending     = "pending"
	ArtworkFetched     = "fetched"
	ArtworkPlaceholder = "placeholder"
	ArtworkManual      = "manual"
)
