package ra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// RA GraphQL endpoint
	graphQLURL = "https://ra.co/graphql"

	// Default timeouts
	defaultTimeout = 30 * time.Second

	// Cache TTL
	cacheTTL = 1 * time.Hour
)

// RAArtist represents a Resident Advisor artist
type RAArtist struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	URL         string     `json:"url"`
	Biography   RABiography `json:"biography"` // blurb text
	Aliases     []string   `json:"aliases"`
	Facebook    string     `json:"facebook"`
	Twitter     string     `json:"twitter"`
	Instagram   string     `json:"instagram"`
	Soundcloud  string     `json:"soundcloud"`
	Bandcamp    string     `json:"bandcamp"`
	Discogs     string     `json:"discogs"`
	Website     string     `json:"website"`
	Areas       []RAArea   `json:"artistAreas"`
	Venues      []RAVenue  `json:"artistVenues"`
	Followers   int        `json:"followerCount"`
	HeaderImage string     `json:"headerImage"`
	ProfileImage string    `json:"profileImage"`
}

// RABiography represents the nested biography structure from RA API
type RABiography struct {
	Blurb string `json:"blurb"`
}

// RAArea represents a geographic area (city/country)
type RAArea struct {
	AreaID   string `json:"areaId"`
	AreaName string `json:"areaName"`
	CountryID string `json:"countryId"`
	Country  string `json:"countryUrl"`
}

// RAVenue represents a venue associated with an artist
type RAVenue struct {
	VenueID   string `json:"venueId"`
	VenueName string `json:"venueName"`
}

// RAArtistRef represents a reference to an artist (in events, etc.)
type RAArtistRef struct {
	ArtistID string `json:"artistId"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Slug     string `json:"slug"`
}

// RAEVENT represents a Resident Advisor event
type RAEVENT struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Date        string        `json:"date"` // ISO format YYYY-MM-DD
	StartTime   string        `json:"startTime"`
	EndTime     string        `json:"endTime"`
	VenueID     string        `json:"venueId"`
	VenueName   string        `json:"venueName"`
	VenueURL    string        `json:"venueUrl"`
	Artists     []RAArtistRef `json:"artists"`
	Hosts       []RAArtistRef `json:"hosts"`
	Attending   int           `json:"attending"`
	ContentURL  string        `json:"contentUrl"` // ticket link
	IsPick      bool          `json:"isPick"`
	IsSoldOut   bool          `json:"isSoldOut"`
	Promo       string        `json:"promo"` // short description
}

// RAAreaResult represents a search result for areas
type RAAreaResult struct {
	AreaID   string `json:"areaId"`
	AreaName string `json:"areaName"`
	CountryID string `json:"countryId"`
	Country  string `json:"countryUrl"`
	ParentAreaID string `json:"parentAreaId"`
	ParentAreaName string `json:"parentAreaName"`
}

// RACache provides in-memory caching for RA data
type RACache struct {
	mu       sync.RWMutex
	artists  map[string]*cachedArtist // slug -> artist
	events   map[string][]RAEVENT     // artistID -> events
	areas    map[string][]RAAreaResult // query -> areas
	clearedAt time.Time
}

type cachedArtist struct {
	artist   *RAArtist
	expiresAt time.Time
}

// NewRACache creates a new RA cache
func NewRACache() *RACache {
	return &RACache{
		artists: make(map[string]*cachedArtist),
		events:  make(map[string][]RAEVENT),
		areas:   make(map[string][]RAAreaResult),
	}
}

// GetArtist retrieves an artist from cache
func (c *RACache) GetArtist(slug string) (*RAArtist, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.artists[slug]
	if !ok {
		return nil, false
	}
	if time.Now().After(cached.expiresAt) {
		return nil, false // expired
	}
	return cached.artist, true
}

// SetArtist stores an artist in cache
func (c *RACache) SetArtist(artist *RAArtist) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.artists[artist.Slug] = &cachedArtist{
		artist:    artist,
		expiresAt: time.Now().Add(cacheTTL),
	}
	// Also index by ID for lookups
	// (in a real implementation, we'd have a separate ID index)
}

// GetEvents retrieves events from cache
func (c *RACache) GetEvents(artistID string) ([]RAEVENT, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	events, ok := c.events[artistID]
	if !ok {
		return nil, false
	}
	// Check if cache is stale (more than cacheTTL old)
	// For simplicity, we'll clear events cache periodically
	if time.Since(c.clearedAt) > cacheTTL {
		return nil, false
	}
	return events, true
}

// SetEvents stores events in cache
func (c *RACache) SetEvents(artistID string, events []RAEVENT) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events[artistID] = events
	c.clearedAt = time.Now()
}

// GetAreas retrieves areas from cache
func (c *RACache) GetAreas(query string) ([]RAAreaResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	areas, ok := c.areas[query]
	if !ok {
		return nil, false
	}
	return areas, true
}

// SetAreas stores areas in cache
func (c *RACache) SetAreas(query string, areas []RAAreaResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.areas[query] = areas
}

// Clear clears all cached data
func (c *RACache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.artists = make(map[string]*cachedArtist)
	c.events = make(map[string][]RAEVENT)
	c.areas = make(map[string][]RAAreaResult)
	c.clearedAt = time.Time{}
}

// RAClient is a GraphQL client for Resident Advisor
type RAClient struct {
	httpClient *http.Client
	cache      *RACache
	baseURL    string
}

// NewRAClient creates a new RA client
func NewRAClient() *RAClient {
	return &RAClient{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cache:   NewRACache(),
		baseURL: graphQLURL,
	}
}

// SetBaseURL overrides the default GraphQL endpoint (for testing)
func (c *RAClient) SetBaseURL(url string) {
	c.baseURL = url
}

// executeQuery sends a GraphQL query and returns the raw response
func (c *RAClient) executeQuery(ctx context.Context, query string, variables map[string]interface{}) ([]byte, error) {
	reqBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, io.NopCloser(&bytesBuffer{Body: body}))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return respBody, nil
}

// GetArtist fetches an artist by slug
func (c *RAClient) GetArtist(ctx context.Context, slug string) (*RAArtist, error) {
	// Check cache first
	if artist, ok := c.cache.GetArtist(slug); ok {
		return artist, nil
	}

	query := `
		query GetArtist($slug: String!) {
			artist(slug: $slug) {
				id
				name
				slug
				url
				biography {
					blurb
				}
				aliases
				facebook
				twitter
				instagram
				soundcloud
				bandcamp
				discogs
				website
				artistAreas {
					areaId
					areaName
					countryId
					countryUrl
				}
				artistVenues {
					venueId
					venueName
				}
				followerCount
				headerImage
				profileImage
			}
		}
	`

	variables := map[string]interface{}{
		"slug": slug,
	}

	respBody, err := c.executeQuery(ctx, query, variables)
	if err != nil {
		return nil, fmt.Errorf("get artist: %w", err)
	}

	var result struct {
		Data struct {
			Artist *RAArtist `json:"artist"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", result.Errors[0].Message)
	}

	if result.Data.Artist == nil {
		return nil, fmt.Errorf("artist not found: %s", slug)
	}

	c.cache.SetArtist(result.Data.Artist)
	return result.Data.Artist, nil
}

// GetArtistEvents fetches an artist's events
func (c *RAClient) GetArtistEvents(ctx context.Context, artistID string, limit int) ([]RAEVENT, error) {
	// Check cache first
	if events, ok := c.cache.GetEvents(artistID); ok {
		if len(events) > 0 {
			return events, nil
		}
	}

	query := `
		query GetArtistEvents($artistId: ID!, $limit: Int) {
			artist(id: $artistId) {
				events(first: $limit) {
					id
					title
					date
					startTime
					endTime
					venueId
					venueName
					venueUrl
					artists {
						artistId
						name
						url
						slug
					}
					hosts {
						artistId
						name
						url
						slug
					}
					attending
					contentUrl
					isPick
					isSoldOut
					promo
				}
			}
		}
	`

	variables := map[string]interface{}{
		"artistId": artistID,
		"limit":    limit,
	}

	respBody, err := c.executeQuery(ctx, query, variables)
	if err != nil {
		return nil, fmt.Errorf("get artist events: %w", err)
	}

	var result struct {
		Data struct {
			Artist struct {
				Events []RAEVENT `json:"events"`
			} `json:"artist"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", result.Errors[0].Message)
	}

	events := result.Data.Artist.Events
	if events == nil {
		events = []RAEVENT{}
	}

	c.cache.SetEvents(artistID, events)
	return events, nil
}

// SearchAreas searches for areas by name
func (c *RAClient) SearchAreas(ctx context.Context, query string) ([]RAAreaResult, error) {
	// Check cache first
	if areas, ok := c.cache.GetAreas(query); ok {
		return areas, nil
	}

	// URL encode the query
	encodedQuery := url.QueryEscape(query)

	queryStr := fmt.Sprintf(`
		query SearchAreas {
			areas(searchTerm: "%s") {
				areaId
				areaName
				countryId
				countryUrl
				parentAreaId
				parentAreaName
			}
		}
	`, encodedQuery)

	variables := map[string]interface{}{}

	respBody, err := c.executeQuery(ctx, queryStr, variables)
	if err != nil {
		return nil, fmt.Errorf("search areas: %w", err)
	}

	var result struct {
		Data struct {
			Areas []RAAreaResult `json:"areas"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", result.Errors[0].Message)
	}

	areas := result.Data.Areas
	if areas == nil {
		areas = []RAAreaResult{}
	}

	c.cache.SetAreas(query, areas)
	return areas, nil
}

// GetVenue fetches a venue by ID
func (c *RAClient) GetVenue(ctx context.Context, venueID string) (*RAVenue, error) {
	query := `
		query GetVenue($venueId: ID!) {
			venue(id: $venueId) {
				venueId: id
				venueName: name
			}
		}
	`

	variables := map[string]interface{}{
		"venueId": venueID,
	}

	respBody, err := c.executeQuery(ctx, query, variables)
	if err != nil {
		return nil, fmt.Errorf("get venue: %w", err)
	}

	var result struct {
		Data struct {
			Venue *RAVenue `json:"venue"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", result.Errors[0].Message)
	}

	if result.Data.Venue == nil {
		return nil, fmt.Errorf("venue not found: %s", venueID)
	}

	return result.Data.Venue, nil
}

// GetEvent fetches a single event by ID
func (c *RAClient) GetEvent(ctx context.Context, eventID string) (*RAEVENT, error) {
	query := `
		query GetEvent($eventId: ID!) {
			event(id: $eventId) {
				id
				title
				date
				startTime
				endTime
				venueId
				venueName
				venueUrl
				artists {
					artistId
					name
					url
					slug
				}
				hosts {
					artistId
					name
					url
					slug
				}
				attending
				contentUrl
				isPick
				isSoldOut
				promo
			}
		}
	`

	variables := map[string]interface{}{
		"eventId": eventID,
	}

	respBody, err := c.executeQuery(ctx, query, variables)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}

	var result struct {
		Data struct {
			Event *RAEVENT `json:"event"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", result.Errors[0].Message)
	}

	if result.Data.Event == nil {
		return nil, fmt.Errorf("event not found: %s", eventID)
	}

	return result.Data.Event, nil
}

// bytesBuffer is a simple buffer that implements io.Reader and io.Writer
type bytesBuffer struct {
	Body []byte
	pos  int
}

func (b *bytesBuffer) Read(p []byte) (n int, err error) {
	if b.pos >= len(b.Body) {
		return 0, io.EOF
	}
	n = copy(p, b.Body[b.pos:])
	b.pos += n
	return n, nil
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	b.Body = append(b.Body, p...)
	return len(p), nil
}
