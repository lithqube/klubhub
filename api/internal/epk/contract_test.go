package epk_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/epk"
)

var contentContractKeys = []string{
	"bioLong", "bioShort", "createdAt", "gigHighlights", "id", "photoPaths",
	"photoUrls", // signed display URL per photoPaths entry (paths are storage keys)
	"pressQuotes", "sectionVisibility", "stagePlotPath", "techRider", "updatedAt",
}

var legacyContentKeys = []string{
	"bio_long", "bio_short", "created_at", "gig_highlights", "photo_paths",
	"press_quotes", "section_visibility", "stage_plot_path", "tech_rider", "updated_at",
}

func contractContent() *epk.EPKContent {
	return &epk.EPKContent{
		ID:                uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		BioShort:          "Short bio",
		BioLong:           "Long bio",
		TechRider:         "Two decks",
		StagePlotPath:     "epk/stage.png",
		GigHighlights:     []string{"Tresor"},
		PressQuotes:       []epk.PressQuote{{Text: "Excellent", Source: "DJ Mag"}},
		PhotoPaths:        []string{"epk/photo.jpg"},
		SectionVisibility: map[string]bool{"bio": true},
		CreatedAt:         time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:         time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC),
	}
}

func assertContentContract(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &got))
	// Response should be wrapped in {data: ...}
	require.Contains(t, got, "data")
	data, ok := got["data"].(map[string]interface{})
	require.True(t, ok)
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assert.Equal(t, contentContractKeys, keys)
	for _, key := range legacyContentKeys {
		assert.NotContains(t, data, key)
	}
	return data
}

func TestHandler_GetContent_UsesCamelCaseContract(t *testing.T) {
	router := newHandlerRoutes(&mockService{getContentResult: contractContent()})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/content", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	got := assertContentContract(t, rr.Body.Bytes())
	assert.Equal(t, "Short bio", got["bioShort"])
}

func TestHandler_PutContent_UsesCamelCaseContract(t *testing.T) {
	svc := &mockService{upsertContentResult: contractContent()}
	body := []byte(`{"bioShort":"Updated","bioLong":"Biography","techRider":"Mixer","stagePlotPath":"epk/new.png","gigHighlights":["Berghain"],"pressQuotes":[{"text":"Great","source":"Mixmag"}],"photoPaths":["epk/new.jpg"],"sectionVisibility":{"bio":false}}`)
	req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	newHandlerRoutes(svc).ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	assertContentContract(t, rr.Body.Bytes())
	require.NotNil(t, svc.upsertContentRequest.BioShort)
	assert.Equal(t, "Updated", *svc.upsertContentRequest.BioShort)
	require.NotNil(t, svc.upsertContentRequest.StagePlotPath)
	assert.Equal(t, "epk/new.png", *svc.upsertContentRequest.StagePlotPath)
	assert.Equal(t, []string{"Berghain"}, svc.upsertContentRequest.GigHighlights)
	assert.Equal(t, []epk.PressQuote{{Text: "Great", Source: "Mixmag"}}, svc.upsertContentRequest.PressQuotes)
	assert.Equal(t, []string{"epk/new.jpg"}, svc.upsertContentRequest.PhotoPaths)
	assert.Equal(t, map[string]bool{"bio": false}, svc.upsertContentRequest.SectionVisibility)
}