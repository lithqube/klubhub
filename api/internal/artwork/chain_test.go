package artwork_test

import (
	"context"
	"errors"
	"testing"

	"github.com/klubhub/dj/api/internal/artwork"
)

// stubSearcher is a scriptable SpotifySearcher/DiscogsSearcher/MusicBrainzSearcher.
type stubSearcher struct {
	url string
	err error
	// calls records how often Search was invoked.
	calls int
}

func (s *stubSearcher) Search(ctx context.Context, title, artist string) (string, error) {
	s.calls++
	return s.url, s.err
}

// recordingCache records Put calls and serves scripted Get hits.
type recordingCache struct {
	hit    string
	puts   []putCall
	putErr error
}

type putCall struct {
	title, artist, imageURL string
}

func (c *recordingCache) Get(ctx context.Context, title, artist string) string { return c.hit }
func (c *recordingCache) Put(ctx context.Context, title, artist, imageURL string) error {
	c.puts = append(c.puts, putCall{title, artist, imageURL})
	return c.putErr
}

func TestFetchChain_CacheHitShortCircuitsProviders(t *testing.T) {
	cache := &recordingCache{hit: "https://cdn.example.com/cached.jpg"}
	spotify := &stubSearcher{url: "https://spotify.example.com/a.jpg"}
	discogs := &stubSearcher{url: "https://discogs.example.com/a.jpg"}
	mb := &stubSearcher{url: "https://mb.example.com/a.jpg"}

	chain := artwork.NewFetchChain(spotify, discogs, mb, cache, "placeholder://default")
	result := chain.Fetch(context.Background(), "Song", "Artist")

	if result.Source != "cache" {
		t.Errorf("source = %q, want cache", result.Source)
	}
	if result.URL != "https://cdn.example.com/cached.jpg" {
		t.Errorf("url = %q, want the cached URL", result.URL)
	}
	if spotify.calls != 0 || discogs.calls != 0 || mb.calls != 0 {
		t.Errorf("providers called on cache hit: spotify=%d discogs=%d mb=%d, want all 0",
			spotify.calls, discogs.calls, mb.calls)
	}
	if len(cache.puts) != 0 {
		t.Errorf("cache.Put called on cache hit: %+v", cache.puts)
	}
}

func TestFetchChain_ProviderHitPopulatesCache(t *testing.T) {
	cache := &recordingCache{}
	spotify := &stubSearcher{url: "https://spotify.example.com/a.jpg"}

	chain := artwork.NewFetchChain(spotify, nil, nil, cache, "placeholder://default")
	result := chain.Fetch(context.Background(), "Song", "Artist")

	if result.Source != "spotify" || result.URL != "https://spotify.example.com/a.jpg" {
		t.Errorf("result = %+v, want spotify hit", result)
	}
	if len(cache.puts) != 1 {
		t.Fatalf("cache.Put calls = %d, want 1", len(cache.puts))
	}
	got := cache.puts[0]
	if got.title != "Song" || got.artist != "Artist" || got.imageURL != "https://spotify.example.com/a.jpg" {
		t.Errorf("cache.Put args = %+v", got)
	}
}

func TestFetchChain_ProviderErrorFallsThrough(t *testing.T) {
	cache := &recordingCache{}
	spotify := &stubSearcher{err: errors.New("spotify 500")}
	discogs := &stubSearcher{url: "https://discogs.example.com/a.jpg"}

	chain := artwork.NewFetchChain(spotify, discogs, nil, cache, "placeholder://default")
	result := chain.Fetch(context.Background(), "Song", "Artist")

	if result.Source != "discogs" {
		t.Errorf("source = %q, want discogs (spotify errored)", result.Source)
	}
	if spotify.calls != 1 || discogs.calls != 1 {
		t.Errorf("calls: spotify=%d discogs=%d, want 1/1", spotify.calls, discogs.calls)
	}
}

func TestFetchChain_EmptyURLFallsThrough(t *testing.T) {
	cache := &recordingCache{}
	spotify := &stubSearcher{url: ""} // no error, but no artwork either
	mb := &stubSearcher{url: "https://mb.example.com/a.jpg"}

	chain := artwork.NewFetchChain(spotify, nil, mb, cache, "placeholder://default")
	result := chain.Fetch(context.Background(), "Song", "Artist")

	if result.Source != "musicbrainz" {
		t.Errorf("source = %q, want musicbrainz (spotify returned empty URL)", result.Source)
	}
}

func TestFetchChain_AllMissReturnsPlaceholder(t *testing.T) {
	cache := &recordingCache{}
	spotify := &stubSearcher{err: errors.New("down")}
	discogs := &stubSearcher{url: ""}
	mb := &stubSearcher{err: errors.New("no match")}

	chain := artwork.NewFetchChain(spotify, discogs, mb, cache, "placeholder://default")
	result := chain.Fetch(context.Background(), "Song", "Artist")

	if result.Source != "placeholder" || result.URL != "placeholder://default" {
		t.Errorf("result = %+v, want placeholder://default", result)
	}
	if len(cache.puts) != 0 {
		t.Errorf("cache.Put called on total miss: %+v", cache.puts)
	}
}

func TestFetchChain_NilCacheDoesNotPanic(t *testing.T) {
	spotify := &stubSearcher{url: "https://spotify.example.com/a.jpg"}
	chain := artwork.NewFetchChain(spotify, nil, nil, nil, "placeholder://default")

	result := chain.Fetch(context.Background(), "Song", "Artist")
	if result.Source != "spotify" {
		t.Errorf("source = %q, want spotify with nil cache", result.Source)
	}

	chain = artwork.NewFetchChain(nil, nil, nil, nil, "placeholder://default")
	result = chain.Fetch(context.Background(), "Song", "Artist")
	if result.Source != "placeholder" {
		t.Errorf("source = %q, want placeholder with nil cache and no providers", result.Source)
	}
}

func TestFetchChain_CachePutErrorStillReturnsArtwork(t *testing.T) {
	// A failing cache write must not lose an otherwise successful fetch:
	// Fetch ignores Put errors by design.
	cache := &recordingCache{putErr: errors.New("storage down")}
	spotify := &stubSearcher{url: "https://spotify.example.com/a.jpg"}

	chain := artwork.NewFetchChain(spotify, nil, nil, cache, "placeholder://default")
	result := chain.Fetch(context.Background(), "Song", "Artist")

	if result.Source != "spotify" || result.URL != "https://spotify.example.com/a.jpg" {
		t.Errorf("result = %+v, want the spotify URL despite cache.Put failure", result)
	}
}
