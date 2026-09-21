package ra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// RA GraphQL endpoint
	graphQLURL = "https://ra.co/graphql"

	// RA's edge rejects Go's anonymous default User-Agent with a 403 block
	// page. Identify the app honestly instead of impersonating a browser.
	userAgent = "KlubHubDJ/1.0 (self-hosted DJ tool; +https://github.com/lithqube/klubhub-dj)"

	// RA serves site-relative links ("/dj/figuds"); this makes them absolute.
	siteURL = "https://ra.co"

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
	mu            sync.RWMutex
	artists       map[string]*cachedArtist // slug -> artist
	events        map[string][]RAEVENT    // artistID -> events
	areas         map[string][]RAAreaResult // query -> areas
	eventExpires  map[string]time.Time    // artistID -> expiration time
	clearedAt     time.Time
}

type cachedArtist struct {
	artist   *RAArtist
	expiresAt time.Time
}

// NewRACache creates a new RA cache
func NewRACache() *RACache {
	return &RACache{
		artists:      make(map[string]*cachedArtist),
		events:       make(map[string][]RAEVENT),
		areas:        make(map[string][]RAAreaResult),
		eventExpires: make(map[string]time.Time),
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

// GetEvents retrieves events from cache with per-entry expiration
func (c *RACache) GetEvents(artistID string) ([]RAEVENT, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	events, ok := c.events[artistID]
	if !ok || events == nil {
		return nil, false
	}

	// Check per-entry expiration
	if expiresAt, hasExpiry := c.eventExpires[artistID]; hasExpiry && time.Now().After(expiresAt) {
		delete(c.events, artistID)
		delete(c.eventExpires, artistID)
		return nil, false
	}

	return events, true
}

// SetEvents stores events in cache with per-entry expiration
func (c *RACache) SetEvents(artistID string, events []RAEVENT) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events[artistID] = events
	// Set per-entry expiration instead of shared clearedAt
	c.eventExpires[artistID] = time.Now().Add(cacheTTL)
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
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Don't echo the body: on a block it is a full HTML page (which
		// also carries the caller's IP) and this error reaches API clients.
		return nil, fmt.Errorf("unexpected status %d from RA", resp.StatusCode)
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
				urlSafeName
				contentUrl
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
				area {
					id
					name
					country {
						id
						urlCode
					}
				}
				venuesMostPlayed {
					id
					name
				}
				followerCount
				coverImage
				image
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
			Artist *wireArtist `json:"artist"`
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
		return nil, fmt.Errorf("%w: %s", ErrArtistNotFound, slug)
	}

	artist := result.Data.Artist.toArtist()
	c.cache.SetArtist(artist)
	return artist, nil
}

// ErrArtistNotFound is returned when RA has no artist for the given slug, so
// callers can tell "no such artist" apart from transport or schema failures.
var ErrArtistNotFound = errors.New("artist not found")

// The wire* types mirror RA's actual GraphQL schema. They are decoded first
// and then mapped onto the exported RA* models, whose JSON tags double as this
// API's response contract and therefore must not follow RA's field names.
type wireArtist struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	URLSafeName string      `json:"urlSafeName"`
	ContentURL  string      `json:"contentUrl"`
	Biography   RABiography `json:"biography"`
	Aliases     string      `json:"aliases"` // comma-separated string, not a list
	Facebook    string      `json:"facebook"`
	Twitter     string      `json:"twitter"`
	Instagram   string      `json:"instagram"`
	Soundcloud  string      `json:"soundcloud"`
	Bandcamp    string      `json:"bandcamp"`
	Discogs     string      `json:"discogs"`
	Website     string      `json:"website"`
	Area        *wireArea   `json:"area"`
	Venues      []wireVenue `json:"venuesMostPlayed"`
	Followers   int         `json:"followerCount"`
	CoverImage  string      `json:"coverImage"`
	Image       string      `json:"image"`
}

type wireArea struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country *struct {
		ID      string `json:"id"`
		URLCode string `json:"urlCode"`
	} `json:"country"`
}

type wireVenue struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ContentURL string    `json:"contentUrl"`
	Area       *wireArea `json:"area"`
}

type wireEvent struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Date       string       `json:"date"`
	StartTime  string       `json:"startTime"`
	EndTime    string       `json:"endTime"`
	ContentURL string       `json:"contentUrl"`
	Attending  int          `json:"attending"`
	Venue      *wireVenue   `json:"venue"`
	Artists    []wireArtist `json:"artists"`
	Promoters  []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		ContentURL string `json:"contentUrl"`
	} `json:"promoters"`
	Pick *struct {
		ID string `json:"id"`
	} `json:"pick"`
}

// absoluteURL turns RA's site-relative links into absolute ones.
func absoluteURL(path string) string {
	if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return siteURL + path
}

func (w *wireArtist) toArtist() *RAArtist {
	a := &RAArtist{
		ID:           w.ID,
		Name:         w.Name,
		Slug:         w.URLSafeName,
		URL:          absoluteURL(w.ContentURL),
		Biography:    w.Biography,
		Aliases:      []string{},
		Facebook:     w.Facebook,
		Twitter:      w.Twitter,
		Instagram:    w.Instagram,
		Soundcloud:   w.Soundcloud,
		Bandcamp:     w.Bandcamp,
		Discogs:      w.Discogs,
		Website:      w.Website,
		Areas:        []RAArea{},
		Venues:       []RAVenue{},
		Followers:    w.Followers,
		HeaderImage:  w.CoverImage,
		ProfileImage: w.Image,
	}
	for _, alias := range strings.Split(w.Aliases, ",") {
		if alias = strings.TrimSpace(alias); alias != "" {
			a.Aliases = append(a.Aliases, alias)
		}
	}
	if w.Area != nil {
		area := RAArea{AreaID: w.Area.ID, AreaName: w.Area.Name}
		if w.Area.Country != nil {
			area.CountryID = w.Area.Country.ID
			area.Country = strings.ToLower(w.Area.Country.URLCode)
		}
		a.Areas = append(a.Areas, area)
	}
	for _, v := range w.Venues {
		a.Venues = append(a.Venues, RAVenue{VenueID: v.ID, VenueName: v.Name})
	}
	return a
}

// RA sends LocalDateTime values ("2026-10-17T21:00:00.000"), while RAEVENT
// documents Date as YYYY-MM-DD and consumers parse it that way; without this
// every imported event was skipped as having an invalid date.
func datePart(localDateTime string) string {
	if len(localDateTime) >= 10 {
		return localDateTime[:10]
	}
	return localDateTime
}

// timePart extracts HH:MM from a LocalDateTime, or returns the input as-is.
func timePart(localDateTime string) string {
	if len(localDateTime) >= 16 && localDateTime[10] == 'T' {
		return localDateTime[11:16]
	}
	return localDateTime
}

func (w *wireEvent) toEvent() RAEVENT {
	e := RAEVENT{
		ID:         w.ID,
		Title:      w.Title,
		Date:       datePart(w.Date),
		StartTime:  timePart(w.StartTime),
		EndTime:    timePart(w.EndTime),
		Artists:    []RAArtistRef{},
		Hosts:      []RAArtistRef{},
		Attending:  w.Attending,
		ContentURL: absoluteURL(w.ContentURL),
		IsPick:     w.Pick != nil,
	}
	if w.Venue != nil {
		e.VenueID = w.Venue.ID
		e.VenueName = w.Venue.Name
		e.VenueURL = absoluteURL(w.Venue.ContentURL)
	}
	for _, a := range w.Artists {
		e.Artists = append(e.Artists, RAArtistRef{
			ArtistID: a.ID,
			Name:     a.Name,
			URL:      absoluteURL(a.ContentURL),
			Slug:     a.URLSafeName,
		})
	}
	// RA has no "hosts"; the promoters running the night are what Hosts means.
	for _, p := range w.Promoters {
		e.Hosts = append(e.Hosts, RAArtistRef{
			ArtistID: p.ID,
			Name:     p.Name,
			URL:      absoluteURL(p.ContentURL),
		})
	}
	return e
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
				events(limit: $limit, type: LATEST) {
					id
					title
					date
					startTime
					endTime
					contentUrl
					attending
					venue {
						id
						name
						contentUrl
					}
					artists {
						id
						name
						urlSafeName
						contentUrl
					}
					promoters {
						id
						name
						contentUrl
					}
					pick {
						id
					}
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
			Artist *struct {
				Events []wireEvent `json:"events"`
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

	events := []RAEVENT{}
	if result.Data.Artist != nil {
		for i := range result.Data.Artist.Events {
			events = append(events, result.Data.Artist.Events[i].toEvent())
		}
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
