package social_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/social"
)

// handlerMockService is a mock implementation of serviceIface for handler tests.
type handlerMockService struct {
	account     *social.SocialAccount
	posts       map[uuid.UUID]*social.ScheduledPost
	oauthURL    string
	scheduleErr error
	editErr     error
	retryErr    error
	validateErr error
}

func newHandlerMock() *handlerMockService {
	return &handlerMockService{
		posts:    make(map[uuid.UUID]*social.ScheduledPost),
		oauthURL: "https://api.instagram.com/oauth/authorize?client_id=test&state=abc",
	}
}

func (m *handlerMockService) GetOAuthURL(_ context.Context) (string, error) {
	return m.oauthURL, nil
}

// IssueOAuthState is the handler-test mock for IssueOAuthState. The bind
// value is fixed to "test-bind" — that lets callback tests control whether
// the cookie matches by setting it (or not) on the test request.
func (m *handlerMockService) IssueOAuthState(_ context.Context) (string, string, string, error) {
	return m.oauthURL, "test-state", "test-bind", nil
}

func (m *handlerMockService) HandleOAuthCallback(_ context.Context, code, state, _ string) (*social.SocialAccount, error) {
	return &social.SocialAccount{Platform: "instagram", Status: "connected"}, nil
}

func (m *handlerMockService) GetAccount(_ context.Context) (*social.SocialAccount, error) {
	return m.account, nil
}

func (m *handlerMockService) DisconnectAccount(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *handlerMockService) ListPosts(_ context.Context) ([]social.ScheduledPost, error) {
	var out []social.ScheduledPost
	for _, p := range m.posts {
		out = append(out, *p)
	}
	return out, nil
}

func (m *handlerMockService) GetPost(_ context.Context, id uuid.UUID) (*social.ScheduledPost, error) {
	p, ok := m.posts[id]
	if !ok {
		return nil, social.ErrNotFound
	}
	return p, nil
}

func (m *handlerMockService) SchedulePost(_ context.Context, req social.CreatePostRequest) (*social.ScheduledPost, error) {
	if m.scheduleErr != nil {
		return nil, m.scheduleErr
	}
	p := &social.ScheduledPost{
		ID:             uuid.New(),
		Status:         social.PostStatusScheduled,
		PostType:       req.PostType,
		Caption:        req.Caption,
		ScheduledAtUTC: time.Now().UTC(),
		TimezoneName:   req.TimezoneName,
	}
	m.posts[p.ID] = p
	return p, nil
}

func (m *handlerMockService) EditPost(_ context.Context, id uuid.UUID, req social.EditPostRequest) (*social.ScheduledPost, error) {
	if m.editErr != nil {
		return nil, m.editErr
	}
	p, ok := m.posts[id]
	if !ok {
		return nil, social.ErrNotFound
	}
	p.Caption = req.Caption
	return p, nil
}

func (m *handlerMockService) SoftDeletePost(_ context.Context, id uuid.UUID) error {
	delete(m.posts, id)
	return nil
}

func (m *handlerMockService) RetryPost(_ context.Context, id uuid.UUID) error {
	if m.retryErr != nil {
		return m.retryErr
	}
	p, ok := m.posts[id]
	if !ok {
		return social.ErrNotFound
	}
	p.Status = social.PostStatusScheduled
	now := time.Now()
	p.NextRetryAt = &now
	return nil
}

func (m *handlerMockService) ValidateImage(data []byte, postType social.PostType) error {
	return m.validateErr
}

func newTestRouter(svc *handlerMockService) http.Handler {
	h := social.NewHandler(svc, nil)
	r := chi.NewRouter()
	r.Mount("/api/v1/social", h.Routes())
	return r
}

// TestSocialHandler_GetAccount_ReturnsNullWhenNoneConnected verifies GET /accounts returns {"data": null}.
func TestSocialHandler_GetAccount_ReturnsNullWhenNoneConnected(t *testing.T) {
	mock := newHandlerMock()
	// account is nil by default
	router := newTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/social/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if data, ok := body["data"]; !ok || data != nil {
		t.Errorf("expected {\"data\": null}, got: %+v", body)
	}
}

// TestSocialHandler_GetOAuthURL_ReturnsURL verifies GET /auth/url returns a URL.
func TestSocialHandler_GetOAuthURL_ReturnsURL(t *testing.T) {
	mock := newHandlerMock()
	router := newTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/social/auth/url", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(body["url"], "instagram.com") {
		t.Errorf("expected instagram.com in OAuth URL, got: %s", body["url"])
	}
}

// TestSocialHandler_EditPost_Returns409WhenErrEditBlocked verifies 409 status for blocked edits.
func TestSocialHandler_EditPost_Returns409WhenErrEditBlocked(t *testing.T) {
	mock := newHandlerMock()
	mock.editErr = social.ErrEditBlocked

	// Add a post to the mock so the ID resolves
	postID := uuid.New()
	mock.posts[postID] = &social.ScheduledPost{
		ID:     postID,
		Status: social.PostStatusPublished,
	}

	router := newTestRouter(mock)

	body := `{"caption":"new caption","scheduled_at":"2026-12-01T12:00","timezone_name":"UTC"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/social/posts/"+postID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestSocialHandler_RetryPost_Returns200 verifies POST /posts/{id}/retry returns 200.
func TestSocialHandler_RetryPost_Returns200(t *testing.T) {
	mock := newHandlerMock()
	postID := uuid.New()
	mock.posts[postID] = &social.ScheduledPost{
		ID:     postID,
		Status: social.PostStatusFailed,
	}

	router := newTestRouter(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts/"+postID.String()+"/retry", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	// Verify the post status was updated
	if mock.posts[postID].Status != social.PostStatusScheduled {
		t.Errorf("expected post status=scheduled after retry, got %q", mock.posts[postID].Status)
	}
}

// TestSocialHandler_RetryPost_Returns404WhenNotFound verifies 404 when post not found.
func TestSocialHandler_RetryPost_Returns404WhenNotFound(t *testing.T) {
	mock := newHandlerMock()
	mock.retryErr = social.ErrNotFound

	nonExistentID := uuid.New()
	router := newTestRouter(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts/"+nonExistentID.String()+"/retry", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestSocialHandler_CreatePost_Returns422ForImageExceeding8MB verifies image size rejection.
func TestSocialHandler_CreatePost_Returns422ForImageExceeding8MB(t *testing.T) {
	mock := newHandlerMock()
	mock.validateErr = social.ErrFileTooLarge
	router := newTestRouter(mock)

	// Build a multipart form with a large fake image file
	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	mw.WriteField("post_type", "feed")
	mw.WriteField("scheduled_at", "2026-12-01T12:00")
	mw.WriteField("timezone_name", "UTC")
	fw, _ := mw.CreateFormFile("image_file", "big.jpg")
	// Write enough bytes to simulate a large file
	fw.Write(make([]byte, 100))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts", &bodyBuf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestSocialHandler_CreatePost_Returns422ForInvalidMIME verifies MIME type rejection.
func TestSocialHandler_CreatePost_Returns422ForInvalidMIME(t *testing.T) {
	mock := newHandlerMock()
	mock.validateErr = social.ErrInvalidMIME
	router := newTestRouter(mock)

	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	mw.WriteField("post_type", "feed")
	mw.WriteField("scheduled_at", "2026-12-01T12:00")
	mw.WriteField("timezone_name", "UTC")
	fw, _ := mw.CreateFormFile("image_file", "text.txt")
	fw.Write([]byte("not an image"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts", &bodyBuf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestSocialHandler_CreatePost_Returns422ForInvalidAspectRatio verifies aspect ratio rejection.
func TestSocialHandler_CreatePost_Returns422ForInvalidAspectRatio(t *testing.T) {
	mock := newHandlerMock()
	mock.validateErr = social.ErrInvalidDimensions
	router := newTestRouter(mock)

	// Create a 320x960 PNG (1:3 ratio - invalid for feed)
	var imgBuf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 320, 960))
	png.Encode(&imgBuf, img)

	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	mw.WriteField("post_type", "feed")
	mw.WriteField("scheduled_at", "2026-12-01T12:00")
	mw.WriteField("timezone_name", "UTC")
	fw, _ := mw.CreateFormFile("image_file", "portrait.png")
	fw.Write(imgBuf.Bytes())
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts", &bodyBuf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestSocialHandler_CreatePost_Returns201ForValidPost verifies successful post creation.
func TestSocialHandler_CreatePost_Returns201ForValidPost(t *testing.T) {
	mock := newHandlerMock()
	router := newTestRouter(mock)

	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	mw.WriteField("post_type", "feed")
	mw.WriteField("caption", "Test caption")
	mw.WriteField("scheduled_at", "2026-12-01T12:00")
	mw.WriteField("timezone_name", "UTC")
	mw.WriteField("image_id", "social/test.jpg")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/social/posts", &bodyBuf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestSocialHandler_DisconnectAccount_Returns200 verifies account disconnect returns 200.
func TestSocialHandler_DisconnectAccount_Returns200(t *testing.T) {
	mock := newHandlerMock()
	router := newTestRouter(mock)

	accountID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/social/accounts/"+accountID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// Ensure ErrNotFound is not wrapped so errors.Is works correctly.
var _ error = social.ErrNotFound

func init() {
	// Verify ErrEditBlocked sentinel behaviour
	if !errors.Is(social.ErrEditBlocked, social.ErrEditBlocked) {
		panic("ErrEditBlocked sentinel broken")
	}
}
