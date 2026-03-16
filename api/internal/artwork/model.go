package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ArtworkResult represents the result of an artwork fetch attempt
type ArtworkResult struct {
	URL    string
	Source string // "spotify", "discogs", "musicbrainz", "cache", "placeholder"
	Err    error
}

// ErrNoResult indicates no artwork was found
var ErrNoResult = errors.New("no result")

// SpotifySearcher interface for Spotify client
type SpotifySearcher interface {
	Search(ctx context.Context, title, artist string) (string, error)
}

// DiscogsSearcher interface for Discogs client
type DiscogsSearcher interface {
	Search(ctx context.Context, title, artist string) (string, error)
}

// MusicBrainzSearcher interface for MusicBrainz client
type MusicBrainzSearcher interface {
	Search(ctx context.Context, title, artist string) (string, error)
}

// ArtworkCacher interface for MinIO cache
type ArtworkCacher interface {
	Get(ctx context.Context, title, artist string) string
	Put(ctx context.Context, title, artist string, imageURL string) error
}

// SpotifyClient wraps Spotify API calls with token management
type SpotifyClient struct {
	clientID     string
	clientSecret string
	token        string
	tokenExpiry  time.Time
	httpClient   *http.Client
	mu           sync.Mutex
}

// NewSpotifyClient creates a new Spotify client
func NewSpotifyClient(clientID, clientSecret string) *SpotifyClient {
	return &SpotifyClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// getToken obtains a Spotify API token using Client Credentials flow
func (s *SpotifyClient) getToken(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we have a valid token
	if s.token != "" && time.Now().Add(50*time.Second).Before(s.tokenExpiry) {
		return nil
	}

	// Request new token
	req, err := http.NewRequestWithContext(ctx, "POST", "https://accounts.spotify.com/api/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return err
	}

	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("spotify token request failed")
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	s.token = tokenResp.AccessToken
	s.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return nil
}

// Search queries Spotify for artwork
func (s *SpotifyClient) Search(ctx context.Context, title, artist string) (string, error) {
	if s.clientID == "" || s.clientSecret == "" {
		return "", errors.New("spotify not configured")
	}

	if err := s.getToken(ctx); err != nil {
		return "", err
	}

	url := "https://api.spotify.com/v1/search?q=" + encodeQueryParam(title+"+"+artist) + "&type=track&limit=1"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("spotify search failed")
	}

	var result struct {
		Tracks struct {
			Items []struct {
				Album struct {
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"items"`
		} `json:"tracks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Tracks.Items) == 0 {
		return "", ErrNoResult
	}

	if len(result.Tracks.Items[0].Album.Images) == 0 {
		return "", ErrNoResult
	}

	return result.Tracks.Items[0].Album.Images[0].URL, nil
}

// DiscogsClient wraps Discogs API calls
type DiscogsClient struct {
	token      string
	httpClient *http.Client
}

// NewDiscogsClient creates a new Discogs client
func NewDiscogsClient(token string) *DiscogsClient {
	if token == "" {
		return nil
	}
	return &DiscogsClient{
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Search queries Discogs for artwork
func (d *DiscogsClient) Search(ctx context.Context, title, artist string) (string, error) {
	if d == nil || d.token == "" {
		return "", errors.New("discogs not configured")
	}

	url := "https://api.discogs.com/database/search?artist=" + encodeQueryParam(artist) + "&track=" + encodeQueryParam(title) + "&token=" + d.token
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Discogs token="+d.token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("discogs search failed")
	}

	var result struct {
		Results []struct {
			CoverImage string `json:"cover_image"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Results) == 0 {
		return "", ErrNoResult
	}

	return result.Results[0].CoverImage, nil
}

// MusicBrainzClient wraps MusicBrainz API calls with rate limiting
type MusicBrainzClient struct {
	userAgent  string
	httpClient *http.Client
	ticker     *time.Ticker
	mu         sync.Mutex
}

// NewMusicBrainzClient creates a new MusicBrainz client
func NewMusicBrainzClient(userAgent string) *MusicBrainzClient {
	return &MusicBrainzClient{
		userAgent:  userAgent,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		ticker:     time.NewTicker(1 * time.Second), // 1 req/s rate limit
	}
}

// Search queries MusicBrainz for artwork via Cover Art Archive
func (m *MusicBrainzClient) Search(ctx context.Context, title, artist string) (string, error) {
	// Wait for rate limiter
	<-m.ticker.C

	// First, search for the recording
	searchURL := "https://musicbrainz.org/ws/2/recording?query=artist:" + encodeQueryParam(artist) + "+AND+recording:" + encodeQueryParam(title) + "&fmt=json&limit=1&inc=releases"
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", m.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("musicbrainz search failed")
	}

	var result struct {
		Recordings []struct {
			Releases []struct {
				ID string `json:"id"`
			} `json:"releases"`
		} `json:"recordings"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Recordings) == 0 || len(result.Recordings[0].Releases) == 0 {
		return "", ErrNoResult
	}

	mbid := result.Recordings[0].Releases[0].ID

	// Now fetch from Cover Art Archive
	caaURL := "https://coverartarchive.org/release/" + mbid + "/front"
	caaReq, err := http.NewRequestWithContext(ctx, "GET", caaURL, nil)
	if err != nil {
		return "", err
	}
	caaReq.Header.Set("User-Agent", m.userAgent)

	// Follow redirect manually - we check for 307/302 status codes
	caaResp, err := m.httpClient.Do(caaReq)
	if err != nil {
		return "", err
	}
	defer caaResp.Body.Close()

	// Check for redirect status codes (307 Temporary Redirect, 302 Found)
	status := caaResp.StatusCode
	if status == 307 || status == 302 {
		// Follow redirect
		redirectURL := caaResp.Header.Get("Location")
		if redirectURL == "" {
			return "", errors.New("caa redirect without location")
		}
		return redirectURL, nil
	}

	if caaResp.StatusCode != http.StatusOK {
		return "", errors.New("caa fetch failed")
	}

	// Return the CAA URL - the actual image URL after redirect handling
	return caaURL, nil
}

// FetchChain orchestrates the artwork fetch pipeline
type FetchChain struct {
	spotify        SpotifySearcher
	discogs        DiscogsSearcher
	musicbrainz    MusicBrainzSearcher
	cache          ArtworkCacher
	placeholderURL string
}

// NewFetchChain creates a new FetchChain
func NewFetchChain(spotify SpotifySearcher, discogs DiscogsSearcher, musicbrainz MusicBrainzSearcher, cache ArtworkCacher, placeholderURL string) *FetchChain {
	return &FetchChain{
		spotify:        spotify,
		discogs:        discogs,
		musicbrainz:    musicbrainz,
		cache:          cache,
		placeholderURL: placeholderURL,
	}
}

// Fetch attempts to fetch artwork from the chain: cache -> Spotify -> Discogs -> MusicBrainz -> placeholder
func (f *FetchChain) Fetch(ctx context.Context, title, artist string) ArtworkResult {
	// 1. Check cache first
	if f.cache != nil {
		if cachedURL := f.cache.Get(ctx, title, artist); cachedURL != "" {
			return ArtworkResult{URL: cachedURL, Source: "cache"}
		}
	}

	// 2. Try Spotify
	if f.spotify != nil {
		if url, err := f.spotify.Search(ctx, title, artist); err == nil && url != "" {
			// Store in cache
			if f.cache != nil {
				_ = f.cache.Put(ctx, title, artist, url)
			}
			return ArtworkResult{URL: url, Source: "spotify"}
		}
	}

	// 3. Try Discogs
	if f.discogs != nil {
		if url, err := f.discogs.Search(ctx, title, artist); err == nil && url != "" {
			// Store in cache
			if f.cache != nil {
				_ = f.cache.Put(ctx, title, artist, url)
			}
			return ArtworkResult{URL: url, Source: "discogs"}
		}
	}

	// 4. Try MusicBrainz (always available - no credentials needed)
	if f.musicbrainz != nil {
		if url, err := f.musicbrainz.Search(ctx, title, artist); err == nil && url != "" {
			// Store in cache
			if f.cache != nil {
				_ = f.cache.Put(ctx, title, artist, url)
			}
			return ArtworkResult{URL: url, Source: "musicbrainz"}
		}
	}

	// 5. Return placeholder
	return ArtworkResult{URL: f.placeholderURL, Source: "placeholder"}
}

// Helper functions

func encodeQueryParam(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), "\"", "")
}

// Unused but needed for interface compatibility
var _ io.Reader
