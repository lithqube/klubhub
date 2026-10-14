package tracklist

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// ── NormalizeTitle ──

func TestNormalizeTitle(t *testing.T) {
	got, err := NormalizeTitle("  Friday Set \n")
	require.NoError(t, err)
	require.Equal(t, "Friday Set", got)

	for name, in := range map[string]string{
		"empty":      "",
		"blank":      " \t\n ",
		"too long":   strings.Repeat("a", MaxTitleChars+1),
		"NUL inside": "a\x00b",
	} {
		_, err := NormalizeTitle(in)
		require.ErrorIs(t, err, ErrInvalidTitle, name)
	}

	// The limit counts characters, not bytes.
	_, err = NormalizeTitle(strings.Repeat("夜", MaxTitleChars))
	require.NoError(t, err)
}

// ── handler ──

type renameService struct {
	contractService
	title    string
	titleErr error
	gigs     []LinkedGig
	gigsErr  error
}

func (s *renameService) UpdateTitle(_ context.Context, id uuid.UUID, title string) (*Tracklist, error) {
	if s.titleErr != nil {
		return nil, s.titleErr
	}
	s.title = title
	return &Tracklist{ID: id, Title: title, VisibleFields: `[]`}, nil
}

func (s *renameService) LinkedGigs(context.Context, uuid.UUID) ([]LinkedGig, error) {
	return s.gigs, s.gigsErr
}

func do(t *testing.T, svc tracklistServiceIface, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewHandler(svc).Routes().ServeHTTP(rec, req)
	return rec
}

const someID = "11111111-1111-1111-1111-111111111111"

func TestHandler_Rename(t *testing.T) {
	t.Run("returns the renamed tracklist", func(t *testing.T) {
		svc := &renameService{}
		rec := do(t, svc, http.MethodPut, "/"+someID, `{"title":"Warehouse night"}`)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "Warehouse night", svc.title)
		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, "Warehouse night", got["title"])
	})

	cases := map[string]struct {
		path, body string
		svcErr     error
		want       int
	}{
		"missing title":     {"/" + someID, `{}`, nil, http.StatusUnprocessableEntity},
		"invalid title":     {"/" + someID, `{"title":"x"}`, ErrInvalidTitle, http.StatusUnprocessableEntity},
		"unknown tracklist": {"/" + someID, `{"title":"x"}`, ErrNotFound, http.StatusNotFound},
		"bad json":          {"/" + someID, `{`, nil, http.StatusBadRequest},
		"bad id":            {"/not-a-uuid", `{"title":"x"}`, nil, http.StatusBadRequest},
		"internal error":    {"/" + someID, `{"title":"x"}`, errors.New("db down"), http.StatusInternalServerError},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, &renameService{titleErr: c.svcErr}, http.MethodPut, c.path, c.body)
			require.Equal(t, c.want, rec.Code)
		})
	}
}

func TestHandler_LinkedGigs(t *testing.T) {
	date := time.Date(2026, time.October, 4, 22, 0, 0, 0, time.UTC)
	gigID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	rec := do(t, &renameService{gigs: []LinkedGig{{ID: gigID, Date: date, Venue: "Tresor", City: "Berlin", EventName: "Klubnacht"}}},
		http.MethodGet, "/"+someID+"/gigs", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 1)
	require.Equal(t, gigID.String(), got[0]["id"])
	require.Equal(t, "Tresor", got[0]["venue"])
	require.Equal(t, "Klubnacht", got[0]["eventName"])
	require.Contains(t, got[0], "date")

	// No links is an empty array, never null.
	rec = do(t, &renameService{}, http.MethodGet, "/"+someID+"/gigs", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `[]`, rec.Body.String())

	require.Equal(t, http.StatusNotFound,
		do(t, &renameService{gigsErr: ErrNotFound}, http.MethodGet, "/"+someID+"/gigs", "").Code)
	require.Equal(t, http.StatusBadRequest,
		do(t, &renameService{}, http.MethodGet, "/nope/gigs", "").Code)
}

// ── repository (real Postgres) ──

func newTestTracklist(t *testing.T, repo *Repository, title string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	tl := &Tracklist{
		ID: uuid.New(), Title: title, SourceFormat: "rekordbox", Preset: "default",
		VisibleFields: `["title","artist"]`, BgMode: "solid", MaxTracks: 50,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(context.Background(), tl, nil))
	return tl.ID
}

func newTestGig(t *testing.T, venue string, date time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, testPool.QueryRow(context.Background(),
		`INSERT INTO gigs (date, venue, city, event_name) VALUES ($1, $2, 'Berlin', 'Night') RETURNING id`,
		date, venue).Scan(&id))
	return id
}

func TestIntegration_UpdateTitle(t *testing.T) {
	if testing.Short() || testPool == nil {
		t.Skip("integration test")
	}
	ctx := context.Background()
	repo := NewRepository(testPool)
	svc := &Service{repo: repo}

	id := newTestTracklist(t, repo, "rekordbox_sample.txt")
	before, _, err := repo.Get(ctx, id)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)
	got, err := svc.UpdateTitle(ctx, id, "  Friday at Tresor  ")
	require.NoError(t, err)
	require.Equal(t, "Friday at Tresor", got.Title)
	require.True(t, got.UpdatedAt.After(before.UpdatedAt), "updated_at must move on rename")

	// An invalid title is rejected before it reaches the database.
	_, err = svc.UpdateTitle(ctx, id, "   ")
	require.ErrorIs(t, err, ErrInvalidTitle)
	after, _, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Friday at Tresor", after.Title)

	// Unknown and soft-deleted tracklists are not found.
	_, err = svc.UpdateTitle(ctx, uuid.New(), "x")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, repo.SoftDelete(ctx, id))
	_, err = svc.UpdateTitle(ctx, id, "after delete")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestIntegration_LinkedGigs(t *testing.T) {
	if testing.Short() || testPool == nil {
		t.Skip("integration test")
	}
	ctx := context.Background()
	repo := NewRepository(testPool)

	id := newTestTracklist(t, repo, "Set")
	other := newTestTracklist(t, repo, "Other set")
	older := newTestGig(t, "Older club", time.Date(2026, 9, 1, 22, 0, 0, 0, time.UTC))
	newer := newTestGig(t, "Newer club", time.Date(2026, 10, 1, 22, 0, 0, 0, time.UTC))
	deleted := newTestGig(t, "Deleted club", time.Date(2026, 10, 2, 22, 0, 0, 0, time.UTC))
	unrelated := newTestGig(t, "Unrelated club", time.Date(2026, 10, 3, 22, 0, 0, 0, time.UTC))

	for _, gigID := range []uuid.UUID{older, newer, deleted} {
		_, err := testPool.Exec(ctx, `INSERT INTO tracklist_gigs (tracklist_id, gig_id) VALUES ($1, $2)`, id, gigID)
		require.NoError(t, err)
	}
	_, err := testPool.Exec(ctx, `INSERT INTO tracklist_gigs (tracklist_id, gig_id) VALUES ($1, $2)`, other, unrelated)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE gigs SET deleted_at = now() WHERE id = $1`, deleted)
	require.NoError(t, err)

	got, err := repo.LinkedGigs(ctx, id)
	require.NoError(t, err)
	require.Len(t, got, 2, "soft-deleted and other tracklists' gigs are excluded")
	require.Equal(t, []string{"Newer club", "Older club"}, []string{got[0].Venue, got[1].Venue}, "newest gig first")
	require.Equal(t, newer, got[0].ID)
	require.Equal(t, "Berlin", got[0].City)

	// A tracklist with no links is an empty slice, not an error.
	empty := newTestTracklist(t, repo, "Unlinked")
	none, err := repo.LinkedGigs(ctx, empty)
	require.NoError(t, err)
	require.NotNil(t, none)
	require.Empty(t, none)

	_, err = repo.LinkedGigs(ctx, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}
