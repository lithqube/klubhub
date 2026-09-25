package epk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/epk"
	"github.com/klubhub/dj/api/internal/ra"
)

// fakeRAArtistLoader stands in for the RA client; it never touches the network.
type fakeRAArtistLoader struct {
	calls int
}

func (f *fakeRAArtistLoader) GetArtist(_ context.Context, slug string) (*ra.RAArtist, error) {
	f.calls++
	return &ra.RAArtist{Name: "Test Artist " + slug, Instagram: "https://instagram.example/test"}, nil
}

// RA import is a licensed edition feature: with no client injected the
// route is not mounted at all.
func TestHandler_ImportRA_DisabledReturns404(t *testing.T) {
	h := epk.NewHandler(&mockService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/import-ra", strings.NewReader(`{"artist_slug":"test-artist"}`))
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_ImportRA_EnabledUsesInjectedClient(t *testing.T) {
	fake := &fakeRAArtistLoader{}
	h := epk.NewHandler(&mockService{}, fake)

	req := httptest.NewRequest(http.MethodPost, "/import-ra", strings.NewReader(`{"artist_slug":"test-artist"}`))
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, 1, fake.calls)

	var body struct {
		Data epk.RAImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "Test Artist test-artist", body.Data.ArtistName)
}
