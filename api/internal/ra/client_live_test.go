package ra

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive_RAContract runs the real queries against ra.co so schema drift
// (renamed or removed fields) and edge blocks are caught. It only runs when
// RA_LIVE_SLUG is set, e.g.:
//
//	RA_LIVE_SLUG=some-artist go test ./internal/ra/ -run TestLive -v
func TestLive_RAContract(t *testing.T) {
	slug := os.Getenv("RA_LIVE_SLUG")
	if slug == "" {
		t.Skip("set RA_LIVE_SLUG to run against the live RA API")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewRAClient()

	artist, err := client.GetArtist(ctx, slug)
	if err != nil {
		t.Fatalf("GetArtist(%q): %v", slug, err)
	}
	if artist.ID == "" || artist.Name == "" || artist.Slug != slug {
		t.Fatalf("incomplete artist mapping: %+v", artist)
	}
	t.Logf("artist: id=%s name=%q slug=%s url=%s followers=%d areas=%v venues=%d aliases=%v",
		artist.ID, artist.Name, artist.Slug, artist.URL, artist.Followers, artist.Areas, len(artist.Venues), artist.Aliases)

	events, err := client.GetArtistEvents(ctx, artist.ID, 10)
	if err != nil {
		t.Fatalf("GetArtistEvents(%s): %v", artist.ID, err)
	}
	t.Logf("upcoming events: %d", len(events))
	for _, e := range events {
		if len(e.Date) != 10 {
			t.Errorf("event %s: Date %q is not YYYY-MM-DD", e.ID, e.Date)
		}
		t.Logf("  %s %s | %s | %s", e.Date, e.StartTime, e.Title, e.VenueName)
	}
}
