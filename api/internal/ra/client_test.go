package ra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockRAHandler creates a test handler that returns mock RA responses
func mockRAHandler(t *testing.T, artistResp string, eventsResp string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var body struct {
			Query     string                   `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Check if this is an artist query
		if strings.Contains(body.Query, "GetArtist") && body.Variables["slug"] != nil {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, artistResp)
			return
		}

		// Check if this is an events query
		if strings.Contains(body.Query, "GetArtistEvents") && body.Variables["artistId"] != nil {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, eventsResp)
			return
		}

		// Default response
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data": {}, "errors": [{"message": "not implemented"}]}`)
	})
}

func TestNewRAClient(t *testing.T) {
	client := NewRAClient()
	if client == nil {
		t.Fatal("NewRAClient returned nil")
	}
	if client.cache == nil {
		t.Fatal("client.cache is nil")
	}
}

func TestRACache_GetArtist(t *testing.T) {
	cache := NewRACache()
	artist := &RAArtist{
		ID:   "123",
		Name: "Test Artist",
		Slug: "test-artist",
	}

	// Test cache miss
	_, ok := cache.GetArtist("test-artist")
	if ok {
		t.Fatal("expected cache miss, got hit")
	}

	// Set artist
	cache.SetArtist(artist)

	// Test cache hit
	got, ok := cache.GetArtist("test-artist")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if got.Name != "Test Artist" {
		t.Fatalf("expected 'Test Artist', got '%s'", got.Name)
	}
}

func TestRACache_SetAndGetEvents(t *testing.T) {
	cache := NewRACache()
	events := []RAEVENT{
		{
			ID:        "evt-1",
			Title:     "Test Event",
			Date:      "2024-06-15",
			VenueName: "Test Venue",
		},
	}

	cache.SetEvents("artist-123", events)

	got, ok := cache.GetEvents("artist-123")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Title != "Test Event" {
		t.Fatalf("expected 'Test Event', got '%s'", got[0].Title)
	}
}

func TestRACache_Clear(t *testing.T) {
	cache := NewRACache()
	cache.SetArtist(&RAArtist{Slug: "test"})
	cache.SetEvents("artist-1", []RAEVENT{})

	cache.Clear()

	_, ok := cache.GetArtist("test")
	if ok {
		t.Fatal("expected cache miss after clear")
	}

	_, ok = cache.GetEvents("artist-1")
	if ok {
		t.Fatal("expected cache miss after clear")
	}
}

func TestGetArtist_Success(t *testing.T) {
	// Mock response for artist query
	artistResp := `{
		"data": {
			"artist": {
				"id": "123",
				"name": "Test Artist",
				"slug": "test-artist",
				"url": "https://ra.co/dj/test-artist",
				"biography": {"blurb": "A test artist biography"},
				"aliases": ["TA"],
				"facebook": "https://facebook.com/test",
				"twitter": "https://twitter.com/test",
				"instagram": "https://instagram.com/test",
				"soundcloud": "https://soundcloud.com/test",
				"bandcamp": "https://test.bandcamp.com",
				"discogs": "https://discogs.com/test",
				"website": "https://test.com",
				"artistAreas": [{"areaId": "1", "areaName": "London", "countryId": "3", "countryUrl": "uk"}],
				"artistVenues": [{"venueId": "100", "venueName": "Test Club"}],
				"followerCount": 10000,
				"headerImage": "https://example.com/header.jpg",
				"profileImage": "https://example.com/profile.jpg"
			}
		}
	}`

	handler := mockRAHandler(t, artistResp, `{}`)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewRAClient()
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	artist, err := client.GetArtist(ctx, "test-artist")
	if err != nil {
		t.Fatalf("GetArtist failed: %v", err)
	}

	if artist.ID != "123" {
		t.Fatalf("expected ID '123', got '%s'", artist.ID)
	}
	if artist.Name != "Test Artist" {
		t.Fatalf("expected name 'Test Artist', got '%s'", artist.Name)
	}
	if artist.Biography.Blurb != "A test artist biography" {
		t.Fatalf("expected biography 'A test artist biography', got '%s'", artist.Biography.Blurb)
	}
	if artist.Instagram != "https://instagram.com/test" {
		t.Fatalf("expected instagram 'https://instagram.com/test', got '%s'", artist.Instagram)
	}
	if len(artist.Areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(artist.Areas))
	}
	if artist.Areas[0].AreaName != "London" {
		t.Fatalf("expected area 'London', got '%s'", artist.Areas[0].AreaName)
	}
	if len(artist.Venues) != 1 {
		t.Fatalf("expected 1 venue, got %d", len(artist.Venues))
	}
	if artist.Venues[0].VenueName != "Test Club" {
		t.Fatalf("expected venue 'Test Club', got '%s'", artist.Venues[0].VenueName)
	}
	if artist.Followers != 10000 {
		t.Fatalf("expected 10000 followers, got %d", artist.Followers)
	}
}

func TestGetArtist_NotFound(t *testing.T) {
	// Mock response for artist not found
	artistResp := `{
		"data": {
			"artist": null
		}
	}`

	handler := mockRAHandler(t, artistResp, `{}`)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewRAClient()
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.GetArtist(ctx, "non-existent-artist")
	if err == nil {
		t.Fatal("expected error for non-existent artist")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got '%s'", err.Error())
	}
}

func TestGetArtistEvents_Success(t *testing.T) {
	eventsResp := `{
		"data": {
			"artist": {
				"events": [
					{
						"id": "evt-1",
						"title": "Test Event 1",
						"date": "2024-06-15",
						"startTime": "21:00",
						"endTime": "03:00",
						"venueId": "100",
						"venueName": "Test Venue",
						"venueUrl": "https://ra.co/clubs/test-venue",
						"artists": [{"artistId": "123", "name": "Test Artist", "url": "/dj/test-artist", "slug": "test-artist"}],
						"hosts": [],
						"attending": 150,
						"contentUrl": "https://ra.co/events/evt-1/tickets",
						"isPick": true,
						"isSoldOut": false,
						"promo": "A great night of music"
					},
					{
						"id": "evt-2",
						"title": "Test Event 2",
						"date": "2024-07-20",
						"startTime": "22:00",
						"endTime": "04:00",
						"venueId": "101",
						"venueName": "Another Venue",
						"venueUrl": "https://ra.co/clubs/another-venue",
						"artists": [{"artistId": "123", "name": "Test Artist", "url": "/dj/test-artist", "slug": "test-artist"}],
						"hosts": [{"artistId": "456", "name": "Support Act", "url": "/dj/support-act", "slug": "support-act"}],
						"attending": 200,
						"contentUrl": "https://ra.co/events/evt-2/tickets",
						"isPick": false,
						"isSoldOut": false,
						"promo": ""
					}
				]
			}
		}
	}`

	handler := mockRAHandler(t, `{}`, eventsResp)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewRAClient()
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := client.GetArtistEvents(ctx, "123", 10)
	if err != nil {
		t.Fatalf("GetArtistEvents failed: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].ID != "evt-1" {
		t.Fatalf("expected event ID 'evt-1', got '%s'", events[0].ID)
	}
	if events[0].Title != "Test Event 1" {
		t.Fatalf("expected title 'Test Event 1', got '%s'", events[0].Title)
	}
	if events[0].Date != "2024-06-15" {
		t.Fatalf("expected date '2024-06-15', got '%s'", events[0].Date)
	}
	if events[0].VenueName != "Test Venue" {
		t.Fatalf("expected venue 'Test Venue', got '%s'", events[0].VenueName)
	}
	if events[0].Attending != 150 {
		t.Fatalf("expected attending 150, got %d", events[0].Attending)
	}
	if !events[0].IsPick {
		t.Fatal("expected isPick to be true")
	}
	if len(events[0].Artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(events[0].Artists))
	}
	if events[0].Artists[0].Name != "Test Artist" {
		t.Fatalf("expected artist 'Test Artist', got '%s'", events[0].Artists[0].Name)
	}

	// Check second event
	if events[1].Hosts[0].Name != "Support Act" {
		t.Fatalf("expected host 'Support Act', got '%s'", events[1].Hosts[0].Name)
	}
}

func TestSearchAreas_Success(t *testing.T) {
	areasResp := `{
		"data": {
			"areas": [
				{
					"areaId": "13",
					"areaName": "London",
					"countryId": "3",
					"countryUrl": "uk",
					"parentAreaId": null,
					"parentAreaName": null
				},
				{
					"areaId": "14",
					"areaName": "Manchester",
					"countryId": "3",
					"countryUrl": "uk",
					"parentAreaId": null,
					"parentAreaName": null
				}
			]
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string                   `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		if strings.Contains(body.Query, "SearchAreas") || strings.Contains(body.Query, "areas") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, areasResp)
			return
		}

		fmt.Fprint(w, `{"data": {}, "errors": [{"message": "not implemented"}]}`)
	}))
	defer server.Close()

	client := NewRAClient()
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	areas, err := client.SearchAreas(ctx, "London")
	if err != nil {
		t.Fatalf("SearchAreas failed: %v", err)
	}

	if len(areas) != 2 {
		t.Fatalf("expected 2 areas, got %d", len(areas))
	}

	if areas[0].AreaName != "London" {
		t.Fatalf("expected area 'London', got '%s'", areas[0].AreaName)
	}
	if areas[1].AreaName != "Manchester" {
		t.Fatalf("expected area 'Manchester', got '%s'", areas[1].AreaName)
	}
}

func TestCacheExpiration(t *testing.T) {
	cache := NewRACache()

	// Set up cache with very short TTL for testing
	// We'll manually manipulate the expiration
	artist := &RAArtist{Slug: "test"}
	cache.mu.Lock()
	cache.artists["test"] = &cachedArtist{
		artist:    artist,
		expiresAt: time.Now().Add(-time.Hour), // Already expired
	}
	cache.mu.Unlock()

	_, ok := cache.GetArtist("test")
	if ok {
		t.Fatal("expected expired cache to return miss")
	}
}
