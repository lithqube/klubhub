package release

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewReleaseStartsWithTheDefaultChecklist(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListChecklistItems(context.Background(), rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("checklist has %d items, want the shipped default of 6", len(items))
	}
	for i, item := range items {
		if item.Position != i {
			t.Errorf("item %d has position %d", i, item.Position)
		}
		if item.Label != DefaultChecklistLabels()[i] {
			t.Errorf("item %d = %q, want %q", i, item.Label, DefaultChecklistLabels()[i])
		}
		if item.Done {
			t.Errorf("item %d starts done", i)
		}
	}
}

func TestSavedTemplateSeedsTheNextRelease(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	// Trim the checklist to five steps and keep it as the new default.
	items, err := svc.ListChecklistItems(context.Background(), rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveChecklistItem(context.Background(), rel.ID, items[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveChecklistAsTemplate(context.Background(), rel.ID); err != nil {
		t.Fatal(err)
	}

	next, err := svc.Create(context.Background(), Input{Title: "Blue Hour", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := svc.ListChecklistItems(context.Background(), next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) != 5 {
		t.Fatalf("seeded checklist has %d items, want the saved template's 5", len(seeded))
	}
	if seeded[0].Label != "Single sent to promoters" {
		t.Errorf("first seeded item = %q", seeded[0].Label)
	}
}

func TestChecklistItemOperations(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	added, err := svc.AddChecklistItem(context.Background(), rel.ID, "  Send the press kit  ")
	if err != nil {
		t.Fatal(err)
	}
	if added.Label != "Send the press kit" {
		t.Errorf("label = %q, want trimmed", added.Label)
	}
	if _, err := svc.AddChecklistItem(context.Background(), rel.ID, "   "); !errors.Is(err, ErrInvalid) {
		t.Errorf("blank label: want ErrInvalid, got %v", err)
	}
	if _, err := svc.RenameChecklistItem(context.Background(), rel.ID, added.ID, "Press kit sent"); err != nil {
		t.Fatal(err)
	}
	toggled, err := svc.ToggleChecklistItem(context.Background(), rel.ID, added.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !toggled.Done {
		t.Error("item not marked done")
	}
	// Toggling to the same state is a no-op, not an error.
	if _, err := svc.ToggleChecklistItem(context.Background(), rel.ID, added.ID, true); err != nil {
		t.Fatalf("second toggle: %v", err)
	}
	if err := svc.RemoveChecklistItem(context.Background(), rel.ID, added.ID); err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListChecklistItems(context.Background(), rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("checklist has %d items after remove, want 6", len(items))
	}
}

func TestReorderChecklistItems(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListChecklistItems(context.Background(), rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	reversed := make([]uuid.UUID, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		reversed = append(reversed, items[i].ID)
	}
	if err := svc.ReorderChecklistItems(context.Background(), rel.ID, reversed); err != nil {
		t.Fatal(err)
	}
	after, err := svc.ListChecklistItems(context.Background(), rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].ID != reversed[0] {
		t.Errorf("first item = %s, want %s", after[0].ID, reversed[0])
	}
	if err := svc.ReorderChecklistItems(context.Background(), rel.ID, reversed[:2]); !errors.Is(err, ErrInvalid) {
		t.Errorf("short order: want ErrInvalid, got %v", err)
	}
	if err := svc.ReorderChecklistItems(context.Background(), rel.ID, append(reversed[:2:2], uuid.Must(uuid.NewV7()))); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown step: want ErrInvalid, got %v", err)
	}
}

func TestChecklistItemsBelongToOneRelease(t *testing.T) {
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
	items, err := svc.ListChecklistItems(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleChecklistItem(context.Background(), second.ID, items[0].ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for another release's step, got %v", err)
	}
}
