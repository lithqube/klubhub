package social_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/storage"
	"github.com/klubhub/dj/api/internal/social"
	"github.com/stretchr/testify/require"
)

func TestSocialHandler_GetPostImage_SendsPrivateNoCacheHeader(t *testing.T) {
	const imageBody = "stored image bytes"
	const objectKey = "social/post/image.png"
	// Exercise the concrete S3 client against a local HTTP fixture, including
	// bucket location, object metadata and the object body. No live storage needed.
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("location") {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
			return
		}
		require.Equal(t, "/images/"+objectKey, r.URL.Path)
		require.Contains(t, []string{http.MethodHead, http.MethodGet}, r.Method)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprint(len(imageBody)))
		w.Header().Set("Last-Modified", "Thu, 01 Oct 2026 12:00:00 GMT")
		w.Header().Set("ETag", `"fixture"`)
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, imageBody)
		}
	}))
	defer s3.Close()
	client, err := storage.New(&config.Config{
		S3Endpoint: strings.TrimPrefix(s3.URL, "http://"), S3Bucket: "images",
		S3AccessKey: "test-access", S3SecretKey: "test-secret",
	})
	require.NoError(t, err)
	id := uuid.New()
	svc := newHandlerMock()
	svc.posts[id] = &social.ScheduledPost{ID: id, ImageStorageKey: objectKey}
	handler := social.NewHandler(svc, client)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/posts/"+id.String()+"/image", nil))
	response := rec.Result()
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode, rec.Body.String())
	require.Equal(t, "private, no-cache", response.Header.Get("Cache-Control"))
	require.Equal(t, "image/png", response.Header.Get("Content-Type"))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, imageBody, string(body))
}
