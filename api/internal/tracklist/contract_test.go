package tracklist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type contractService struct {
	tracklists []Tracklist
	tracklist  *Tracklist
	tracks     []Track
}

func (s *contractService) Upload(context.Context, string, []byte, int64) (*Tracklist, []Track, []ParseWarning, error) {
	return nil, nil, nil, nil
}

func (s *contractService) GetWithTracks(context.Context, uuid.UUID) (*Tracklist, []Track, error) {
	return s.tracklist, s.tracks, nil
}

func (s *contractService) List(context.Context) ([]Tracklist, error) {
	return s.tracklists, nil
}

func (s *contractService) UpdateTrack(context.Context, uuid.UUID, uuid.UUID, UpdateTrackRequest) (*Track, error) {
	return nil, nil
}

func (s *contractService) SoftDelete(context.Context, uuid.UUID) error { return nil }

func (s *contractService) SaveManualArtwork(context.Context, uuid.UUID, uuid.UUID, []byte, string) error {
	return nil
}

func (s *contractService) GenerateImage(context.Context, uuid.UUID, string) (map[string]string, error) {
	return nil, nil
}

func TestListContractUsesCamelCaseRawArray(t *testing.T) {
	createdAt := time.Date(2026, time.September, 19, 12, 0, 0, 0, time.UTC)
	tracklistID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := NewHandler(&contractService{tracklists: []Tracklist{{
		ID:              tracklistID,
		Title:           "Friday Set",
		SourceFormat:    "rekordbox",
		RawFilePath:     "/tmp/set.txt",
		Preset:          "story",
		VisibleFields:   `["title","artist"]`,
		BgMode:          "solid",
		BgValue:         "#000000",
		MaxTracks:       20,
		TrackRangeStart: 1,
		TrackRangeEnd:   10,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}}})

	recorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	// Response should be wrapped in {data: [...]}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a valid JSON object: %v\nbody: %s", err, recorder.Body.String())
	}
	require.Contains(t, body, "data")
	var tracklists []map[string]any
	if err := json.Unmarshal(body["data"], &tracklists); err != nil {
		t.Fatalf("data is not a JSON array: %v\nbody: %s", err, recorder.Body.String())
	}
	if len(tracklists) != 1 {
		t.Fatalf("len(tracklists) = %d, want 1", len(tracklists))
	}
	wantKeys := []string{
		"id", "title", "sourceFormat", "rawFilePath", "preset", "visibleFields",
		"bgMode", "bgValue", "maxTracks", "trackRangeStart", "trackRangeEnd", "createdAt", "updatedAt",
	}
	if len(tracklists[0]) != len(wantKeys) {
		t.Fatalf("keys = %v, want exactly %v", mapKeys(tracklists[0]), wantKeys)
	}
	for _, key := range wantKeys {
		if _, ok := tracklists[0][key]; !ok {
			t.Errorf("missing camelCase key %q; keys = %v", key, mapKeys(tracklists[0]))
		}
	}
	fields, ok := tracklists[0]["visibleFields"].([]any)
	if !ok {
		t.Fatalf("visibleFields type = %T, want JSON array", tracklists[0]["visibleFields"])
	}
	if len(fields) != 2 || fields[0] != "title" || fields[1] != "artist" {
		t.Fatalf("visibleFields = %#v", fields)
	}
}

func TestDetailContractWrapsCamelCaseTracklistAndTracks(t *testing.T) {
	tracklistID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	trackID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	now := time.Date(2026, time.September, 19, 12, 0, 0, 0, time.UTC)
	tracklist := &Tracklist{ID: tracklistID, Title: "Friday Set", VisibleFields: `[]`, CreatedAt: now, UpdatedAt: now}
	h := NewHandler(&contractService{
		tracklist: tracklist,
		tracks: []Track{{
			ID: trackID, TracklistID: tracklistID, Position: 1, Title: "One",
			Artist: "Artist", Album: "Album", Genre: "House", BPM: 124.5,
			Rating: 4, DurationSeconds: 180, MusicalKey: "8A", DateAdded: now,
			ArtworkStatus: ArtworkFetched, ArtworkURL: "/art.jpg", ArtworkSource: "spotify",
		}},
	})

	recorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/"+tracklistID.String(), nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 || body["tracklist"] == nil || body["tracks"] == nil {
		t.Fatalf("top-level keys = %v, want exactly [tracklist tracks]", mapKeys(body))
	}
	var tracks []map[string]any
	if err := json.Unmarshal(body["tracks"], &tracks); err != nil {
		t.Fatal(err)
	}
	wantTrackKeys := []string{
		"id", "tracklistId", "position", "title", "artist", "album", "genre", "bpm", "rating",
		"durationSecs", "musicalKey", "dateAdded", "artworkStatus", "artworkUrl", "artworkSource",
	}
	if len(tracks) != 1 || len(tracks[0]) != len(wantTrackKeys) {
		t.Fatalf("track keys = %v, want exactly %v", mapKeys(tracks[0]), wantTrackKeys)
	}
	for _, key := range wantTrackKeys {
		if _, ok := tracks[0][key]; !ok {
			t.Errorf("missing camelCase track key %q; keys = %v", key, mapKeys(tracks[0]))
		}
	}
	if _, ok := tracks[0]["durationSecs"].(float64); !ok {
		t.Errorf("durationSecs type = %T, want JSON number", tracks[0]["durationSecs"])
	}
	if _, ok := tracks[0]["dateAdded"].(string); !ok {
		t.Errorf("dateAdded type = %T, want JSON string", tracks[0]["dateAdded"])
	}
}

func mapKeys[T any](value map[string]T) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}
