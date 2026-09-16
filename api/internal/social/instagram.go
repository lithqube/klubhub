package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ContainerStatus is the status of an Instagram media container.
type ContainerStatus string

const (
	ContainerStatusInProgress ContainerStatus = "IN_PROGRESS"
	ContainerStatusFinished   ContainerStatus = "FINISHED"
	ContainerStatusPublished  ContainerStatus = "PUBLISHED"
	ContainerStatusError      ContainerStatus = "ERROR"
	ContainerStatusExpired    ContainerStatus = "EXPIRED"
)

// RateLimitError is returned when Instagram responds with HTTP 429.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("instagram rate limited: retry after %s", e.RetryAfter)
}

// InstagramClient is an HTTP client for the Instagram Graph API.
type InstagramClient struct {
	httpClient *http.Client
	baseURL    string
	cryptoKey  []byte
}

// NewInstagramClient creates an InstagramClient with the default Instagram Graph API base URL.
func NewInstagramClient(cryptoKey []byte) *InstagramClient {
	return &InstagramClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://graph.instagram.com/v22.0",
		cryptoKey:  cryptoKey,
	}
}

// igMediaResponse is used to parse a container or publish response.
type igMediaResponse struct {
	ID string `json:"id"`
}

// igContainerStatusResponse is used to parse the container status response.
type igContainerStatusResponse struct {
	StatusCode string `json:"status_code"`
	ID         string `json:"id"`
}

// igRefreshTokenResponse is used to parse the refresh token response.
type igRefreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// CreateContainer creates an Instagram media container for the given post.
// For feed posts, media_type=IMAGE; for stories, media_type=STORIES.
// On HTTP 429, returns a *RateLimitError with RetryAfter duration.
//
// All non-rate-limit error returns wrap the detail with the sanitized
// status + body so that transport errors do not leak the access_token
// the API client embedded in its request URL.
func (c *InstagramClient) CreateContainer(ctx context.Context, igUserID, accessToken, imageURL, caption string, postType PostType) (string, error) {
	mediaType := "IMAGE"
	if postType == PostTypeStory {
		mediaType = "STORIES"
	}

	params := url.Values{}
	params.Set("image_url", imageURL)
	params.Set("media_type", mediaType)
	params.Set("access_token", accessToken)
	if caption != "" {
		params.Set("caption", caption)
	}

	endpoint := fmt.Sprintf("%s/%s/media", c.baseURL, igUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(params.Encode()))
	if err != nil {
		return "", fmt.Errorf("create container request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 15 * time.Minute
		if header := resp.Header.Get("Retry-After"); header != "" {
			if seconds, err := strconv.ParseInt(header, 10, 64); err == nil {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		return "", &RateLimitError{RetryAfter: retryAfter}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create container: status %d: %s",
			resp.StatusCode, SanitizeTransportError(string(body)))
	}

	var result igMediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode create container response: %w", err)
	}
	return result.ID, nil
}

// PublishContainer publishes a previously created Instagram media container.
func (c *InstagramClient) PublishContainer(ctx context.Context, igUserID, accessToken, containerID string) (string, error) {
	params := url.Values{}
	params.Set("creation_id", containerID)
	params.Set("access_token", accessToken)

	endpoint := fmt.Sprintf("%s/%s/media_publish", c.baseURL, igUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(params.Encode()))
	if err != nil {
		return "", fmt.Errorf("publish container request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("publish container: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("publish container: status %d: %s",
			resp.StatusCode, SanitizeTransportError(string(body)))
	}

	var result igMediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode publish container response: %w", err)
	}
	return result.ID, nil
}

// CheckContainerStatus checks the status of an Instagram media container.
func (c *InstagramClient) CheckContainerStatus(ctx context.Context, containerID, accessToken string) (ContainerStatus, error) {
	endpoint := fmt.Sprintf("%s/%s", c.baseURL, containerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("check container status request: %w", err)
	}

	q := req.URL.Query()
	q.Set("fields", "status_code")
	q.Set("access_token", accessToken)
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("check container status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("check container status: status %d: %s",
			resp.StatusCode, SanitizeTransportError(string(body)))
	}

	var result igContainerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode container status response: %w", err)
	}
	return ContainerStatus(result.StatusCode), nil
}

// RefreshToken refreshes an Instagram long-lived access token.
// Returns the new token and its expiry time.
//
// Error strings from this function intentionally do NOT include the
// full request URL — the URL carries access_token=<value> in the
// query, and we sanitize the body but the URL string itself is the
// caller's, not the body's. We log the unredacted URL only via the
// caller (worker.go) which goes to a private log sink.
func (c *InstagramClient) RefreshToken(ctx context.Context, currentToken string) (string, time.Time, error) {
	endpoint := fmt.Sprintf("%s/refresh_access_token", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("refresh token request: %w", err)
	}

	q := req.URL.Query()
	q.Set("grant_type", "ig_refresh_token")
	q.Set("access_token", currentToken)
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("refresh token: status %d: %s",
			resp.StatusCode, SanitizeTransportError(string(body)))
	}

	var result igRefreshTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decode refresh token response: %w", err)
	}

	expiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return result.AccessToken, expiry, nil
}

// convertPNGToJPEG decodes PNG image data and re-encodes it as JPEG with quality 95.
func (c *InstagramClient) convertPNGToJPEG(pngData []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("decode PNG: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}
	return buf.Bytes(), nil
}
