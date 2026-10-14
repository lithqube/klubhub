package artwork_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/artwork"
	"github.com/klubhub/dj/api/internal/tracklist"
)

// stubUpdater records UpdateArtworkStatus calls.
type stubUpdater struct {
	mu    sync.Mutex
	calls []statusCall
	err   error
}

type statusCall struct {
	trackID uuid.UUID
	status  string
	url     string
	source  string
}

func (s *stubUpdater) UpdateArtworkStatus(ctx context.Context, trackID uuid.UUID, status, url, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, statusCall{trackID, status, url, source})
	return s.err
}

// stubChain is a scriptable artwork.FetchChain replacement. FetchChain is a
// concrete struct, so the worker test builds a real one over stub searchers
// and a stub cache instead.
type stubCacheForWorker struct{ hit string }

func (c *stubCacheForWorker) Get(ctx context.Context, title, artist string) string { return c.hit }
func (c *stubCacheForWorker) Put(ctx context.Context, title, artist, imageURL string) error {
	return nil
}

func track(id uuid.UUID, title, artist, artworkSource string) tracklist.Track {
	return tracklist.Track{ID: id, Title: title, Artist: artist, ArtworkSource: artworkSource}
}

func TestFetchAll_SkipsManualArtwork(t *testing.T) {
	updater := &stubUpdater{}
	chain := artwork.NewFetchChain(&stubSearcher{url: "https://spotify.example.com/a.jpg"}, nil, nil, &stubCacheForWorker{}, "placeholder://default")
	svc := artwork.NewService(chain, updater, "placeholder://default")

	manual := track(uuid.New(), "Manual Song", "Artist", tracklist.ArtworkManual)
	svc.FetchAll(context.Background(), []tracklist.Track{manual})

	if len(updater.calls) != 0 {
		t.Errorf("manual-artwork track was re-fetched: %+v — manual overrides must never be replaced", updater.calls)
	}
}

func TestFetchAll_MapsFetchedSourceToFetchedStatus(t *testing.T) {
	updater := &stubUpdater{}
	chain := artwork.NewFetchChain(&stubSearcher{url: "https://spotify.example.com/a.jpg"}, nil, nil, &stubCacheForWorker{}, "placeholder://default")
	svc := artwork.NewService(chain, updater, "placeholder://default")

	id := uuid.New()
	svc.FetchAll(context.Background(), []tracklist.Track{track(id, "Song", "Artist", "")})

	if len(updater.calls) != 1 {
		t.Fatalf("UpdateArtworkStatus calls = %d, want 1", len(updater.calls))
	}
	call := updater.calls[0]
	if call.trackID != id {
		t.Errorf("trackID = %v, want %v", call.trackID, id)
	}
	if call.status != tracklist.ArtworkFetched {
		t.Errorf("status = %q, want %q", call.status, tracklist.ArtworkFetched)
	}
	if call.url != "https://spotify.example.com/a.jpg" || call.source != "spotify" {
		t.Errorf("url/source = %q/%q, want the spotify hit", call.url, call.source)
	}
}

func TestFetchAll_MapsPlaceholderSourceToPlaceholderStatus(t *testing.T) {
	updater := &stubUpdater{}
	// No providers configured -> placeholder.
	chain := artwork.NewFetchChain(nil, nil, nil, &stubCacheForWorker{}, "placeholder://default")
	svc := artwork.NewService(chain, updater, "placeholder://default")

	id := uuid.New()
	svc.FetchAll(context.Background(), []tracklist.Track{track(id, "Song", "Artist", "")})

	if len(updater.calls) != 1 {
		t.Fatalf("UpdateArtworkStatus calls = %d, want 1", len(updater.calls))
	}
	call := updater.calls[0]
	if call.status != tracklist.ArtworkPlaceholder {
		t.Errorf("status = %q, want %q", call.status, tracklist.ArtworkPlaceholder)
	}
	if call.url != "placeholder://default" || call.source != "placeholder" {
		t.Errorf("url/source = %q/%q, want placeholder://default/placeholder", call.url, call.source)
	}
}

func TestFetchAll_RepoErrorDoesNotStopOtherTracks(t *testing.T) {
	updater := &stubUpdater{err: errors.New("db write failed")}
	chain := artwork.NewFetchChain(&stubSearcher{url: "https://spotify.example.com/a.jpg"}, nil, nil, &stubCacheForWorker{}, "placeholder://default")
	svc := artwork.NewService(chain, updater, "placeholder://default")

	// FetchAll must not panic or hang when the repo rejects every write;
	// it logs and continues.
	svc.FetchAll(context.Background(), []tracklist.Track{
		track(uuid.New(), "One", "Artist", ""),
		track(uuid.New(), "Two", "Artist", ""),
	})

	if len(updater.calls) != 2 {
		t.Errorf("UpdateArtworkStatus calls = %d, want 2 (errors tolerated, all tracks attempted)", len(updater.calls))
	}
}

func TestFetchAll_EmptyTrackListIsNoOp(t *testing.T) {
	updater := &stubUpdater{}
	chain := artwork.NewFetchChain(&stubSearcher{url: "https://spotify.example.com/a.jpg"}, nil, nil, &stubCacheForWorker{}, "placeholder://default")
	svc := artwork.NewService(chain, updater, "placeholder://default")

	svc.FetchAll(context.Background(), nil)

	if len(updater.calls) != 0 {
		t.Errorf("UpdateArtworkStatus calls = %d, want 0 for empty input", len(updater.calls))
	}
}
