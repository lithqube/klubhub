package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/klubhub/dj/api/internal/epk"
	"github.com/klubhub/dj/api/internal/gig"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/ra"
)

// fakeRAClient satisfies raClient without any network access.
type fakeRAClient struct{}

func (fakeRAClient) GetArtist(_ context.Context, slug string) (*ra.RAArtist, error) {
	return &ra.RAArtist{ID: "1", Name: "Test Artist", Slug: slug}, nil
}

func (fakeRAClient) GetArtistEvents(_ context.Context, _ string, _ int) ([]ra.RAEVENT, error) {
	return nil, nil
}

// raTestRouter wires the EPK and gig routers exactly like run() does, with
// the RA integration gated by features. calls counts client constructions.
func raTestRouter(t *testing.T, features config.Features) (http.Handler, *int) {
	t.Helper()
	calls := 0
	newClient := func() raClient {
		calls++
		return fakeRAClient{}
	}
	gigHandler := gig.NewHandler(nil, "test-secret")
	loader := wireRAImport(features, newClient, gigHandler, nil, nil, nil)
	epkHandler := epk.NewHandler(nil, loader)

	r := chi.NewRouter()
	r.Mount("/api/v1/epk", epkHandler.Routes())
	r.Mount("/api/v1/gigs", gigHandler.Routes())
	return r, &calls
}

func doRequest(h http.Handler, method, path, body string) int {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code
}

func TestRAImport_DisabledByDefault_RoutesNotMounted(t *testing.T) {
	router, calls := raTestRouter(t, config.Features{})

	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/epk/import-ra", `{"artist_slug":"test-artist"}`},
		{http.MethodPost, "/api/v1/gigs/import-ra", `{"artist_slug":"test-artist"}`},
		{http.MethodGet, "/api/v1/gigs/info/test-artist", ""},
	}
	for _, c := range cases {
		if got := doRequest(router, c.method, c.path, c.body); got != http.StatusNotFound {
			t.Errorf("%s %s: expected 404 with RA disabled, got %d", c.method, c.path, got)
		}
	}
	if *calls != 0 {
		t.Errorf("expected no RA client to be constructed when disabled, got %d", *calls)
	}
}

func TestRAImport_Enabled_RoutesServeWithInjectedClient(t *testing.T) {
	router, calls := raTestRouter(t, config.Features{RAImport: true})

	if got := doRequest(router, http.MethodPost, "/api/v1/epk/import-ra", `{"artist_slug":"test-artist"}`); got != http.StatusOK {
		t.Errorf("POST /epk/import-ra: expected 200, got %d", got)
	}
	if got := doRequest(router, http.MethodGet, "/api/v1/gigs/info/test-artist", ""); got != http.StatusOK {
		t.Errorf("GET /gigs/info/{slug}: expected 200, got %d", got)
	}
	// Mounted: validation runs instead of a 404.
	if got := doRequest(router, http.MethodPost, "/api/v1/gigs/import-ra", `{}`); got != http.StatusBadRequest {
		t.Errorf("POST /gigs/import-ra with empty body: expected 400, got %d", got)
	}
	if *calls != 1 {
		t.Errorf("expected exactly one shared RA client, got %d constructions", *calls)
	}
}
