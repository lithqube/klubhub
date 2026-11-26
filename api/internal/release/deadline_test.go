package release

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAddDeadlineValidatesInput(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddDeadline(context.Background(), rel.ID, "   ", time.Now()); !errors.Is(err, ErrInvalid) {
		t.Errorf("blank name: want ErrInvalid, got %v", err)
	}
	if _, err := svc.AddDeadline(context.Background(), rel.ID, "Artwork", time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("no due date: want ErrInvalid, got %v", err)
	}
	if _, err := svc.AddDeadline(context.Background(), uuid.Must(uuid.NewV7()), "Artwork", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown release: want ErrNotFound, got %v", err)
	}
	d, err := svc.AddDeadline(context.Background(), rel.ID, "  Artwork final  ", time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Artwork final" {
		t.Errorf("name = %q, want trimmed", d.Name)
	}
	if d.DueDate.Hour() != 0 {
		t.Errorf("due date = %s, want the day, not the instant", d.DueDate)
	}
}

func TestDeadlineOverdueIsComputed(t *testing.T) {
	today := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		due       time.Time
		completed bool
		want      bool
	}{
		"past and open":      {time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), false, true},
		"past and completed":  {time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), true, false},
		"due today":          {time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC), false, false},
		"future":             {time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC), false, false},
	}
	for name, c := range cases {
		d := Deadline{DueDate: c.due, Completed: c.completed}
		if got := d.IsOverdue(today); got != c.want {
			t.Errorf("%s: IsOverdue = %v, want %v", name, got, c.want)
		}
	}
}

func TestCompleteDeadlineIsIdempotent(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.AddDeadline(context.Background(), rel.ID, "Artwork", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	done, err := svc.CompleteDeadline(context.Background(), rel.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Completed {
		t.Fatal("deadline not marked completed")
	}
	again, err := svc.CompleteDeadline(context.Background(), rel.ID, d.ID)
	if err != nil {
		t.Fatalf("completing twice: %v", err)
	}
	if !again.Completed {
		t.Fatal("second completion lost the flag")
	}
}

func TestDeadlinesBelongToOneRelease(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	first, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(context.Background(), Input{Title: "Blue Hour", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.AddDeadline(context.Background(), first.ID, "Artwork", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteDeadline(context.Background(), second.ID, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for another release's deadline, got %v", err)
	}
	if _, err := svc.RenameDeadline(context.Background(), second.ID, d.ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for another release's deadline, got %v", err)
	}
	list, err := svc.ListDeadlines(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("first release has %d deadlines, want 1", len(list))
	}
	if err := svc.RemoveDeadline(context.Background(), first.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	list, err = svc.ListDeadlines(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("after remove %d deadlines, want 0", len(list))
	}
}
