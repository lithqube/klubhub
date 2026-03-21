package social

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestInstagramClient creates an InstagramClient wired to the given test server URL.
func newTestInstagramClient(baseURL string) *InstagramClient {
	return &InstagramClient{
		httpClient: &http.Client{},
		baseURL:    baseURL,
	}
}

// makePNGBytes generates a minimal valid 10x10 PNG image.
func makePNGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// makeJPEGBytes generates a minimal valid 10x10 JPEG image.
func makeJPEGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func TestInstagramCreateContainer_Feed(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		capturedBody = r.Form.Encode()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"container_abc123"}`))
	}))
	defer srv.Close()

	client := newTestInstagramClient(srv.URL)
	ctx := context.Background()

	containerID, err := client.CreateContainer(ctx, "ig_user_123", "test_token", "https://example.com/image.jpg", "My caption", PostTypeFeed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containerID != "container_abc123" {
		t.Errorf("expected container_abc123, got %q", containerID)
	}
	if !strings.Contains(capturedBody, "media_type=IMAGE") {
		t.Errorf("expected media_type=IMAGE in body, got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "access_token=test_token") {
		t.Errorf("expected access_token in body, got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "caption=My+caption") {
		t.Errorf("expected caption in body, got: %s", capturedBody)
	}
}

func TestInstagramCreateContainer_Story(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		capturedBody = r.Form.Encode()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"container_story456"}`))
	}))
	defer srv.Close()

	client := newTestInstagramClient(srv.URL)
	ctx := context.Background()

	containerID, err := client.CreateContainer(ctx, "ig_user_123", "test_token", "https://example.com/image.jpg", "", PostTypeStory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containerID != "container_story456" {
		t.Errorf("expected container_story456, got %q", containerID)
	}
	if !strings.Contains(capturedBody, "media_type=STORIES") {
		t.Errorf("expected media_type=STORIES in body, got: %s", capturedBody)
	}
}

func TestInstagramCreateContainer_RateLimit_DefaultDuration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := newTestInstagramClient(srv.URL)
	ctx := context.Background()

	_, err := client.CreateContainer(ctx, "ig_user_123", "test_token", "https://example.com/image.jpg", "caption", PostTypeFeed)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	rlErr, ok := err.(*RateLimitError)
	if !ok {
		t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
	}
	if rlErr.RetryAfter != 15*time.Minute {
		t.Errorf("expected 15 minutes default, got %v", rlErr.RetryAfter)
	}
}

func TestInstagramCreateContainer_RateLimit_WithRetryAfterHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120") // 120 seconds
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := newTestInstagramClient(srv.URL)
	ctx := context.Background()

	_, err := client.CreateContainer(ctx, "ig_user_123", "test_token", "https://example.com/image.jpg", "caption", PostTypeFeed)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	rlErr, ok := err.(*RateLimitError)
	if !ok {
		t.Fatalf("expected *RateLimitError, got %T", err)
	}
	if rlErr.RetryAfter != 120*time.Second {
		t.Errorf("expected 120s from header, got %v", rlErr.RetryAfter)
	}
}

func TestInstagramPublishContainer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if !strings.Contains(r.URL.Path, "media_publish") {
			t.Errorf("expected media_publish path, got %s", r.URL.Path)
		}
		creationID := r.Form.Get("creation_id")
		if creationID == "" {
			t.Error("expected creation_id in form")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"post_id_789"}`))
	}))
	defer srv.Close()

	client := newTestInstagramClient(srv.URL)
	ctx := context.Background()

	postID, err := client.PublishContainer(ctx, "ig_user_123", "test_token", "container_abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postID != "post_id_789" {
		t.Errorf("expected post_id_789, got %q", postID)
	}
}

func TestInstagramCheckContainerStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify fields param is present
		if !strings.Contains(r.URL.RawQuery, "fields=status_code") {
			t.Errorf("expected fields=status_code in query, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status_code":"FINISHED","id":"container_abc"}`))
	}))
	defer srv.Close()

	client := newTestInstagramClient(srv.URL)
	ctx := context.Background()

	status, err := client.CheckContainerStatus(ctx, "container_abc", "test_token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != ContainerStatusFinished {
		t.Errorf("expected FINISHED, got %q", status)
	}
}

func TestInstagramRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "refresh_access_token") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"new_token_xyz","token_type":"bearer","expires_in":5184000}`))
	}))
	defer srv.Close()

	client := newTestInstagramClient(srv.URL)
	ctx := context.Background()

	newToken, expiry, err := client.RefreshToken(ctx, "old_token_abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newToken != "new_token_xyz" {
		t.Errorf("expected new_token_xyz, got %q", newToken)
	}
	// expiry should be roughly now + 5184000 seconds (60 days)
	expectedExpiry := time.Now().Add(5184000 * time.Second)
	delta := expectedExpiry.Sub(expiry)
	if delta < -5*time.Second || delta > 5*time.Second {
		t.Errorf("expiry not within 5s of expected: got %v, expected ~%v", expiry, expectedExpiry)
	}
}

func TestInstagramConvertPNGToJPEG(t *testing.T) {
	client := &InstagramClient{}
	pngData := makePNGBytes()

	jpegData, err := client.convertPNGToJPEG(pngData)
	if err != nil {
		t.Fatalf("convertPNGToJPEG error: %v", err)
	}

	// Verify JPEG magic bytes (FFD8FF)
	if len(jpegData) < 3 || jpegData[0] != 0xFF || jpegData[1] != 0xD8 || jpegData[2] != 0xFF {
		t.Errorf("output does not look like JPEG: first bytes %v", jpegData[:min(3, len(jpegData))])
	}

	// Verify it can be decoded as a valid image
	_, _, err = image.Decode(bytes.NewReader(jpegData))
	if err != nil {
		t.Errorf("converted output is not a valid image: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestRateLimitError_ErrorString(t *testing.T) {
	err := &RateLimitError{RetryAfter: 15 * time.Minute}
	if err.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

func TestContainerStatusConstants(t *testing.T) {
	// Verify constants have expected string values
	cases := map[ContainerStatus]string{
		ContainerStatusInProgress: "IN_PROGRESS",
		ContainerStatusFinished:   "FINISHED",
		ContainerStatusPublished:  "PUBLISHED",
		ContainerStatusError:      "ERROR",
		ContainerStatusExpired:    "EXPIRED",
	}
	for k, v := range cases {
		if string(k) != v {
			t.Errorf("expected %q got %q", v, k)
		}
	}
}
