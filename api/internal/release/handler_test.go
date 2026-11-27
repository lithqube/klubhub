package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// stubService answers the handler with fixed data.
type stubService struct {
	releases []*Release
	links    []*PlatformLink
	items    []*ChecklistItem
	fail     error
}

func (s *stubService) Create(context.Context, Input) (*Release, error) { return s.release(), s.fail }
func (s *stubService) Update(context.Context, uuid.UUID, Input) (*Release, error) {
	return s.release(), s.fail
}
func (s *stubService) Delete(context.Context, uuid.UUID) error { return s.fail }
func (s *stubService) Transition(context.Context, uuid.UUID, ReleaseStatus) (*Release, error) {
	return s.release(), s.fail
}
func (s *stubService) Get(context.Context, uuid.UUID) (*Release, error) { return s.release(), s.fail }
func (s *stubService) List(context.Context) ([]*Release, error)         { return s.releases, s.fail }

func (s *stubService) AddDeadline(context.Context, uuid.UUID, string, time.Time) (*Deadline, error) {
	return &Deadline{}, s.fail
}
func (s *stubService) RenameDeadline(context.Context, uuid.UUID, uuid.UUID, string) (*Deadline, error) {
	return &Deadline{}, s.fail
}
func (s *stubService) CompleteDeadline(context.Context, uuid.UUID, uuid.UUID) (*Deadline, error) {
	return &Deadline{}, s.fail
}
func (s *stubService) RemoveDeadline(context.Context, uuid.UUID, uuid.UUID) error { return s.fail }
func (s *stubService) ListDeadlines(context.Context, uuid.UUID) ([]*Deadline, error) {
	return nil, s.fail
}

func (s *stubService) SetPlatformLink(context.Context, uuid.UUID, Platform, string) (*PlatformLink, error) {
	return &PlatformLink{}, s.fail
}
func (s *stubService) RemovePlatformLink(context.Context, uuid.UUID, Platform) error { return s.fail }
func (s *stubService) ListPlatformLinks(context.Context, uuid.UUID) ([]*PlatformLink, error) {
	return s.links, s.fail
}

func (s *stubService) AddChecklistItem(context.Context, uuid.UUID, string) (*ChecklistItem, error) {
	return &ChecklistItem{}, s.fail
}
func (s *stubService) RenameChecklistItem(context.Context, uuid.UUID, uuid.UUID, string) (*ChecklistItem, error) {
	return &ChecklistItem{}, s.fail
}
func (s *stubService) ToggleChecklistItem(context.Context, uuid.UUID, uuid.UUID, bool) (*ChecklistItem, error) {
	return &ChecklistItem{}, s.fail
}
func (s *stubService) RemoveChecklistItem(context.Context, uuid.UUID, uuid.UUID) error { return s.fail }
func (s *stubService) ReorderChecklistItems(context.Context, uuid.UUID, []uuid.UUID) error {
	return s.fail
}
func (s *stubService) SaveChecklistAsTemplate(context.Context, uuid.UUID) error { return s.fail }
func (s *stubService) ListChecklistItems(context.Context, uuid.UUID) ([]*ChecklistItem, error) {
	return s.items, s.fail
}

func (s *stubService) release() *Release {
	return &Release{ID: uuid.Must(uuid.NewV7()), Title: "Night Signal", Status: StatusDemoSent}
}

func do(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandlerListReturnsTheEnvelope(t *testing.T) {
	svc := &stubService{releases: []*Release{{Title: "Night Signal"}}}
	rec := do(t, NewHandler(svc).Routes(), http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Data []Release `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].Title != "Night Signal" {
		t.Errorf("data = %+v", body.Data)
	}
}

func TestHandlerRejectsBadIDs(t *testing.T) {
	h := NewHandler(&stubService{}).Routes()
	for _, path := range []string{"/not-a-uuid", "/not-a-uuid/deadlines", "/not-a-uuid/checklist"} {
		if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", path, rec.Code)
		}
	}
}

func TestHandlerMapsServiceErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"not found":  {ErrNotFound, http.StatusNotFound},
		"invalid":    {ErrInvalid, http.StatusUnprocessableEntity},
		"transition": {ErrInvalidTransition, http.StatusConflict},
		"unexpected": {context.Canceled, http.StatusInternalServerError},
	}
	for name, c := range cases {
		h := NewHandler(&stubService{fail: c.err}).Routes()
		rec := do(t, h, http.MethodGet, "/"+uuid.Must(uuid.NewV7()).String(), nil)
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d", name, rec.Code, c.want)
		}
		if name == "unexpected" && bytes.Contains(rec.Body.Bytes(), []byte(c.err.Error())) {
			t.Errorf("unexpected error leaked to response: %s", rec.Body)
		}
	}
}

func TestHandlerListDoesNotExposeUnexpectedErrors(t *testing.T) {
	secret := errors.New("database connection details")
	rec := do(t, NewHandler(&stubService{fail: secret}).Routes(), http.MethodGet, "/", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(secret.Error())) {
		t.Fatalf("unexpected error leaked to response: %s", rec.Body)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("internal server error")) {
		t.Errorf("generic error missing from response: %s", rec.Body)
	}
}

func TestHandlerCreateValidatesTheBody(t *testing.T) {
	h := NewHandler(&stubService{}).Routes()
	if rec := do(t, h, http.MethodPost, "/", map[string]string{"title": "x"}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing date: status = %d, want 422", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/", map[string]string{"title": "x", "release_date": "2026-12-01"}); rec.Code != http.StatusCreated {
		t.Errorf("valid body: status = %d, want 201", rec.Code)
	}
}

func TestHandlerLinksAndChecklistRoutes(t *testing.T) {
	svc := &stubService{
		links: []*PlatformLink{{Platform: PlatformBeatport, URL: "https://beatport.com/x"}},
		items: []*ChecklistItem{{Label: "Artwork final"}},
	}
	h := NewHandler(svc).Routes()
	id := uuid.Must(uuid.NewV7()).String()

	rec := do(t, h, http.MethodGet, "/"+id+"/links", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("links: status = %d, body %s", rec.Code, rec.Body)
	}
	var links struct {
		Data []PlatformLink `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &links); err != nil {
		t.Fatal(err)
	}
	if len(links.Data) != 1 || links.Data[0].Platform != PlatformBeatport {
		t.Errorf("links data = %+v", links.Data)
	}

	rec = do(t, h, http.MethodGet, "/"+id+"/checklist", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("checklist: status = %d, body %s", rec.Code, rec.Body)
	}
	var items struct {
		Data []ChecklistItem `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items.Data) != 1 || items.Data[0].Label != "Artwork final" {
		t.Errorf("checklist data = %+v", items.Data)
	}
}
