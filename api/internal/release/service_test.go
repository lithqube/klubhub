package release

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeStore is an in-memory Store for the service tests.
type fakeStore struct {
	releases  map[uuid.UUID]*Release
	deadlines map[uuid.UUID]*Deadline
	updates   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{releases: map[uuid.UUID]*Release{}, deadlines: map[uuid.UUID]*Deadline{}}
}

func (f *fakeStore) Create(_ context.Context, rel *Release) error {
	if rel.ID == uuid.Nil {
		rel.ID = uuid.Must(uuid.NewV7())
	}
	rel.CreatedAt = time.Now()
	rel.UpdatedAt = rel.CreatedAt
	f.releases[rel.ID] = rel
	return nil
}

func (f *fakeStore) Update(_ context.Context, rel *Release) error {
	if _, ok := f.releases[rel.ID]; !ok {
		return ErrNotFound
	}
	f.updates++
	f.releases[rel.ID] = rel
	return nil
}

func (f *fakeStore) SoftDelete(_ context.Context, id uuid.UUID) error {
	delete(f.releases, id)
	return nil
}

func (f *fakeStore) GetByID(_ context.Context, id uuid.UUID) (*Release, error) {
	rel, ok := f.releases[id]
	if !ok {
		return nil, ErrNotFound
	}
	return rel, nil
}

func (f *fakeStore) List(_ context.Context) ([]*Release, error) {
	out := make([]*Release, 0, len(f.releases))
	for _, rel := range f.releases {
		out = append(out, rel)
	}
	return out, nil
}

func (f *fakeStore) InsertDeadline(_ context.Context, d *Deadline) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.Must(uuid.NewV7())
	}
	d.CreatedAt = time.Now()
	d.UpdatedAt = d.CreatedAt
	f.deadlines[d.ID] = d
	return nil
}

func (f *fakeStore) UpdateDeadline(_ context.Context, d *Deadline) error {
	if _, ok := f.deadlines[d.ID]; !ok {
		return ErrNotFound
	}
	f.deadlines[d.ID] = d
	return nil
}

func (f *fakeStore) DeleteDeadline(_ context.Context, id uuid.UUID) error {
	delete(f.deadlines, id)
	return nil
}

func (f *fakeStore) GetDeadline(_ context.Context, id uuid.UUID) (*Deadline, error) {
	d, ok := f.deadlines[id]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func (f *fakeStore) ListDeadlines(_ context.Context, releaseID uuid.UUID) ([]*Deadline, error) {
	out := make([]*Deadline, 0, len(f.deadlines))
	for _, d := range f.deadlines {
		if d.ReleaseID == releaseID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteDeadlinesForRelease(_ context.Context, releaseID uuid.UUID) error {
	for id, d := range f.deadlines {
		if d.ReleaseID == releaseID {
			delete(f.deadlines, id)
		}
	}
	return nil
}

func TestCanTransitionOnlyNeighbouringSteps(t *testing.T) {
	all := []ReleaseStatus{StatusDemoSent, StatusAccepted, StatusMastering, StatusArtwork, StatusPromo, StatusReleased}
	for _, from := range all {
		for _, to := range all {
			want := false
			switch {
			case from == to:
				want = false
			case from == StatusReleased:
				want = false
			default:
				fi := indexOf(t, from)
				ti := indexOf(t, to)
				want = ti-fi == 1 || ti-fi == -1
			}
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestTransitionRejectsIllegalJump(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Transition(context.Background(), rel.ID, StatusPromo); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
	if _, err := svc.Transition(context.Background(), rel.ID, StatusReleased); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
	if store.updates != 0 {
		t.Fatalf("illegal transitions wrote %d updates, want 0", store.updates)
	}
}

func TestTransitionWalksTheWholeWorkflow(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []ReleaseStatus{StatusAccepted, StatusMastering, StatusArtwork, StatusPromo, StatusReleased} {
		rel, err = svc.Transition(context.Background(), rel.ID, to)
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
		if rel.Status != to {
			t.Fatalf("status = %s, want %s", rel.Status, to)
		}
	}
	if _, err := svc.Transition(context.Background(), rel.ID, StatusPromo); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition out of released, got %v", err)
	}
	if _, err := svc.Transition(context.Background(), rel.ID, StatusReleased); err != nil {
		t.Fatalf("same-status transition: %v", err)
	}
}

func TestCreateValidatesInput(t *testing.T) {
	svc := NewService(newFakeStore())
	day := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	for name, in := range map[string]Input{
		"no title":       {ReleaseDate: day},
		"blank title":    {Title: "   ", ReleaseDate: day},
		"no date":        {Title: "Night Signal"},
		"unknown format": {Title: "Night Signal", ReleaseDate: day, Format: ReleaseFormat("cassette")},
	} {
		if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	rel, err := svc.Create(context.Background(), Input{Title: "  Night Signal  ", ReleaseDate: day})
	if err != nil {
		t.Fatal(err)
	}
	if rel.Title != "Night Signal" {
		t.Errorf("title = %q, want trimmed", rel.Title)
	}
	if rel.Format != FormatDigital {
		t.Errorf("format = %q, want the digital default", rel.Format)
	}
	if rel.Status != StatusDemoSent {
		t.Errorf("status = %q, want demo_sent", rel.Status)
	}
}

func TestIsOverdueIsComputed(t *testing.T) {	today := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	past := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC)

	cases := map[string]struct {
		date   time.Time
		status ReleaseStatus
		want   bool
	}{
		"past and unreleased":  {past, StatusPromo, true},
		"past and released":    {past, StatusReleased, false},
		"future":               {future, StatusPromo, false},
		"due today is not yet": {today, StatusPromo, false},
	}
	for name, c := range cases {
		rel := Release{ReleaseDate: c.date, Status: c.status}
		if got := rel.IsOverdue(today); got != c.want {
			t.Errorf("%s: IsOverdue = %v, want %v", name, got, c.want)
		}
	}
}

// indexOf returns the position of a status in the workflow order.
func indexOf(t *testing.T, s ReleaseStatus) int {
	t.Helper()
	for i, v := range statusOrder {
		if v == s {
			return i
		}
	}
	t.Fatalf("unknown status %s", s)
	return -1
}
