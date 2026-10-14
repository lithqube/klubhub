package tracklist

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// ── CreateTrackRequest.Normalize ──

func TestCreateTrackRequest_Normalize(t *testing.T) {
	bpm := 128.0
	got, err := CreateTrackRequest{Title: "  Africa ", Artist: " Toto ", MusicalKey: " 8B ", Media: "vinyl", BPM: &bpm}.Normalize()
	require.NoError(t, err)
	require.Equal(t, "Africa", got.Title)
	require.Equal(t, "Toto", got.Artist)
	require.Equal(t, "8B", got.MusicalKey)

	tooBig := 1000.0
	neg := -1.0
	for name, req := range map[string]CreateTrackRequest{
		"blank title":      {Title: "   "},
		"long title":       {Title: strings.Repeat("a", MaxTrackTextChars+1)},
		"long artist":      {Title: "x", Artist: strings.Repeat("a", MaxTrackTextChars+1)},
		"NUL":              {Title: "a\x00b"},
		"unknown media":    {Title: "x", Media: "cd"},
		"old digital type": {Title: "x", Media: "wav"},
		"bpm too high":     {Title: "x", BPM: &tooBig},
		"negative bpm":     {Title: "x", BPM: &neg},
		"long musical key": {Title: "x", MusicalKey: "12345678901"},
	} {
		_, err := req.Normalize()
		require.ErrorIs(t, err, ErrInvalidTrack, name)
	}
	for _, m := range []string{"", "vinyl", "digital"} {
		_, err := CreateTrackRequest{Title: "x", Media: m}.Normalize()
		require.NoError(t, err, m)
	}
}

// ── handler ──

type trackOpsService struct {
	contractService
	added    CreateTrackRequest
	addErr   error
	order    []uuid.UUID
	orderErr error
	updErr   error
}

func (s *trackOpsService) AddTrack(_ context.Context, id uuid.UUID, req CreateTrackRequest) (*Track, error) {
	s.added = req
	if s.addErr != nil {
		return nil, s.addErr
	}
	return &Track{ID: uuid.New(), TracklistID: id, Position: 4, Title: req.Title, Media: req.Media, HiddenGem: req.HiddenGem}, nil
}

func (s *trackOpsService) ReorderTracks(_ context.Context, id uuid.UUID, ids []uuid.UUID) ([]Track, error) {
	s.order = ids
	if s.orderErr != nil {
		return nil, s.orderErr
	}
	out := make([]Track, len(ids))
	for i, tid := range ids {
		out[i] = Track{ID: tid, TracklistID: id, Position: i + 1}
	}
	return out, nil
}

func (s *trackOpsService) UpdateTrack(context.Context, uuid.UUID, uuid.UUID, UpdateTrackRequest) (*Track, error) {
	return nil, s.updErr
}

func TestHandler_AddTrack(t *testing.T) {
	svc := &trackOpsService{}
	rec := do(t, svc, http.MethodPost, "/"+someID+"/tracks", `{"title":"New ID","artist":"Me","media":"vinyl","hiddenGem":true,"unreleased":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, CreateTrackRequest{Title: "New ID", Artist: "Me", Media: "vinyl", HiddenGem: true, Unreleased: true}, svc.added)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "vinyl", got["media"])
	require.Equal(t, true, got["hiddenGem"])
	require.Contains(t, got, "unreleased")

	for name, c := range map[string]struct {
		path, body string
		err        error
		want       int
	}{
		"invalid track": {"/" + someID + "/tracks", `{"title":""}`, ErrInvalidTrack, http.StatusUnprocessableEntity},
		"no tracklist":  {"/" + someID + "/tracks", `{"title":"x"}`, ErrNotFound, http.StatusNotFound},
		"bad json":      {"/" + someID + "/tracks", `{`, nil, http.StatusBadRequest},
		"bad id":        {"/nope/tracks", `{"title":"x"}`, nil, http.StatusBadRequest},
		"server error":  {"/" + someID + "/tracks", `{"title":"x"}`, errors.New("db down"), http.StatusInternalServerError},
	} {
		require.Equal(t, c.want, do(t, &trackOpsService{addErr: c.err}, http.MethodPost, c.path, c.body).Code, name)
	}
}

func TestHandler_ReorderTracks(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	svc := &trackOpsService{}
	rec := do(t, svc, http.MethodPut, "/"+someID+"/tracks/order", `{"trackIds":["`+b.String()+`","`+a.String()+`"]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []uuid.UUID{b, a}, svc.order)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2)
	require.Equal(t, b.String(), got[0]["id"])

	require.Equal(t, http.StatusUnprocessableEntity,
		do(t, &trackOpsService{orderErr: ErrInvalidTrack}, http.MethodPut, "/"+someID+"/tracks/order", `{"trackIds":[]}`).Code)
	require.Equal(t, http.StatusNotFound,
		do(t, &trackOpsService{orderErr: ErrNotFound}, http.MethodPut, "/"+someID+"/tracks/order", `{"trackIds":[]}`).Code)
	require.Equal(t, http.StatusBadRequest,
		do(t, &trackOpsService{}, http.MethodPut, "/"+someID+"/tracks/order", `{"trackIds":["not-a-uuid"]}`).Code)
	require.Equal(t, http.StatusBadRequest,
		do(t, &trackOpsService{}, http.MethodPut, "/nope/tracks/order", `{}`).Code)
}

func TestHandler_UpdateTrack_InvalidMediaIs422(t *testing.T) {
	rec := do(t, &trackOpsService{updErr: ErrInvalidTrack}, http.MethodPut, "/"+someID+"/tracks/"+someID, `{"media":"cd"}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

// ── repository (real Postgres) ──

func positions(t *testing.T, repo *Repository, id uuid.UUID) []string {
	t.Helper()
	_, tracks, err := repo.Get(context.Background(), id)
	require.NoError(t, err)
	out := make([]string, len(tracks))
	for i, tr := range tracks {
		out[i] = tr.Title
	}
	return out
}

func TestIntegration_AddTrack(t *testing.T) {
	if testing.Short() || testPool == nil {
		t.Skip("integration test")
	}
	ctx := context.Background()
	repo := NewRepository(testPool)
	svc := &Service{repo: repo}
	id := newTestTracklist(t, repo, "Set")

	bpm := 124.5
	a, err := svc.AddTrack(ctx, id, CreateTrackRequest{Title: " First ", Artist: "A", BPM: &bpm, MusicalKey: "8A", Media: "digital", Unreleased: true})
	require.NoError(t, err)
	require.Equal(t, 1, a.Position)
	require.Equal(t, "First", a.Title)
	b, err := svc.AddTrack(ctx, id, CreateTrackRequest{Title: "Second", HiddenGem: true, Media: "vinyl"})
	require.NoError(t, err)
	require.Equal(t, 2, b.Position)

	_, tracks, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Len(t, tracks, 2)
	require.Equal(t, 124.5, tracks[0].BPM)
	require.Equal(t, "8A", tracks[0].MusicalKey)
	require.Equal(t, "digital", tracks[0].Media)
	require.True(t, tracks[0].Unreleased)
	require.False(t, tracks[0].HiddenGem)
	require.True(t, tracks[1].HiddenGem)
	require.Equal(t, ArtworkPlaceholder, tracks[0].ArtworkStatus, "hand-typed tracks skip the artwork lookup")

	// A soft-deleted track keeps its position, so the next one goes after it.
	_, err = testPool.Exec(ctx, `UPDATE tracks SET deleted_at = now() WHERE id = $1`, b.ID)
	require.NoError(t, err)
	c, err := svc.AddTrack(ctx, id, CreateTrackRequest{Title: "Third"})
	require.NoError(t, err)
	require.Equal(t, 3, c.Position)

	_, err = svc.AddTrack(ctx, id, CreateTrackRequest{Title: " "})
	require.ErrorIs(t, err, ErrInvalidTrack)
	_, err = svc.AddTrack(ctx, uuid.New(), CreateTrackRequest{Title: "x"})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestIntegration_ReorderTracks(t *testing.T) {
	if testing.Short() || testPool == nil {
		t.Skip("integration test")
	}
	ctx := context.Background()
	repo := NewRepository(testPool)
	svc := &Service{repo: repo}
	id := newTestTracklist(t, repo, "Set")

	ids := map[string]uuid.UUID{}
	for _, title := range []string{"A", "B", "C", "D"} {
		tr, err := svc.AddTrack(ctx, id, CreateTrackRequest{Title: title})
		require.NoError(t, err)
		ids[title] = tr.ID
	}
	// B is deleted: it still occupies position 2.
	_, err := testPool.Exec(ctx, `UPDATE tracks SET deleted_at = now() WHERE id = $1`, ids["B"])
	require.NoError(t, err)

	got, err := svc.ReorderTracks(ctx, id, []uuid.UUID{ids["D"], ids["A"], ids["C"]})
	require.NoError(t, err)
	require.Equal(t, []string{"D", "A", "C"}, []string{got[0].Title, got[1].Title, got[2].Title})
	for i, tr := range got {
		require.Equal(t, i+1, tr.Position, "live tracks are numbered 1..n")
	}

	// Again, the other way, to prove the deleted row never collides.
	_, err = svc.ReorderTracks(ctx, id, []uuid.UUID{ids["C"], ids["D"], ids["A"]})
	require.NoError(t, err)
	require.Equal(t, []string{"C", "D", "A"}, positions(t, repo, id))

	// Adding after a reorder lands at the end.
	_, err = svc.AddTrack(ctx, id, CreateTrackRequest{Title: "E"})
	require.NoError(t, err)
	require.Equal(t, []string{"C", "D", "A", "E"}, positions(t, repo, id))

	// The list must be every live track exactly once, and only this tracklist's.
	other := newTestTracklist(t, repo, "Other")
	foreign, err := svc.AddTrack(ctx, other, CreateTrackRequest{Title: "Foreign"})
	require.NoError(t, err)
	var e uuid.UUID
	for _, tr := range func() []Track { _, tt, _ := repo.Get(ctx, id); return tt }() {
		if tr.Title == "E" {
			e = tr.ID
		}
	}
	for name, bad := range map[string][]uuid.UUID{
		"missing one":   {ids["C"], ids["D"], ids["A"]},
		"duplicate":     {ids["C"], ids["C"], ids["A"], e},
		"deleted track": {ids["C"], ids["D"], ids["A"], ids["B"]},
		"foreign track": {ids["C"], ids["D"], ids["A"], foreign.ID},
		"empty":         {},
	} {
		_, err := svc.ReorderTracks(ctx, id, bad)
		require.ErrorIs(t, err, ErrInvalidTrack, name)
	}
	require.Equal(t, []string{"C", "D", "A", "E"}, positions(t, repo, id), "a refused reorder changes nothing")

	_, err = svc.ReorderTracks(ctx, uuid.New(), []uuid.UUID{})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestIntegration_UpdateTrackMarkers(t *testing.T) {
	if testing.Short() || testPool == nil {
		t.Skip("integration test")
	}
	ctx := context.Background()
	repo := NewRepository(testPool)
	svc := &Service{repo: repo}
	id := newTestTracklist(t, repo, "Set")
	tr, err := svc.AddTrack(ctx, id, CreateTrackRequest{Title: "T", Media: "digital"})
	require.NoError(t, err)

	yes, media := true, "vinyl"
	got, err := svc.UpdateTrack(ctx, id, tr.ID, UpdateTrackRequest{HiddenGem: &yes, Unreleased: &yes, Media: &media})
	require.NoError(t, err)
	require.True(t, got.HiddenGem)
	require.True(t, got.Unreleased)
	require.Equal(t, "vinyl", got.Media)

	// A partial update leaves the markers alone.
	title := "T2"
	got, err = svc.UpdateTrack(ctx, id, tr.ID, UpdateTrackRequest{Title: &title})
	require.NoError(t, err)
	require.True(t, got.HiddenGem)
	require.Equal(t, "vinyl", got.Media)

	// Clearing media is allowed; an unknown one is refused in the service and by the database.
	empty, no := "", false
	got, err = svc.UpdateTrack(ctx, id, tr.ID, UpdateTrackRequest{Media: &empty, HiddenGem: &no})
	require.NoError(t, err)
	require.Equal(t, "", got.Media)
	require.False(t, got.HiddenGem)

	bad := "cd"
	_, err = svc.UpdateTrack(ctx, id, tr.ID, UpdateTrackRequest{Media: &bad})
	require.ErrorIs(t, err, ErrInvalidTrack)
	require.Error(t, repo.UpdateTrack(ctx, id, tr.ID, UpdateTrackRequest{Media: &bad}), "the CHECK constraint backs the service")
}
