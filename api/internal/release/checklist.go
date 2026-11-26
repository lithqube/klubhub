package release

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ChecklistItem is one promo step of a release (REL-05).
type ChecklistItem struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ReleaseID uuid.UUID `json:"release_id" db:"release_id"`
	Position  int       `json:"position" db:"position"`
	Label     string    `json:"label" db:"label"`
	Done      bool      `json:"done" db:"done"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// DefaultChecklistLabels is the promo checklist every release starts with
// when the operator has not saved a template of their own (REL-05).
func DefaultChecklistLabels() []string {
	return []string{
		"Single sent to promoters",
		"Artwork final",
		"Promo copies delivered",
		"Playlists pitched",
		"Social posts scheduled",
		"Release-day post",
	}
}

// ListChecklistItems returns the promo steps of a release, in order.
func (s *Service) ListChecklistItems(ctx context.Context, releaseID uuid.UUID) ([]*ChecklistItem, error) {
	if _, err := s.repo.GetByID(ctx, releaseID); err != nil {
		return nil, err
	}
	return s.repo.ListChecklistItems(ctx, releaseID)
}

// AddChecklistItem appends a promo step to a release.
func (s *Service) AddChecklistItem(ctx context.Context, releaseID uuid.UUID, label string) (*ChecklistItem, error) {
	if _, err := s.repo.GetByID(ctx, releaseID); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return nil, errors.Join(ErrInvalid, errors.New("checklist label is required"))
	}
	items, err := s.repo.ListChecklistItems(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	item := &ChecklistItem{ReleaseID: releaseID, Position: len(items), Label: trimmed}
	if err := s.repo.InsertChecklistItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

// RenameChecklistItem renames a promo step.
func (s *Service) RenameChecklistItem(ctx context.Context, releaseID, id uuid.UUID, label string) (*ChecklistItem, error) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return nil, errors.Join(ErrInvalid, errors.New("checklist label is required"))
	}
	item, err := s.checklistItemOf(ctx, releaseID, id)
	if err != nil {
		return nil, err
	}
	item.Label = trimmed
	if err := s.repo.UpdateChecklistItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

// ToggleChecklistItem marks a promo step done or not done.
func (s *Service) ToggleChecklistItem(ctx context.Context, releaseID, id uuid.UUID, done bool) (*ChecklistItem, error) {
	item, err := s.checklistItemOf(ctx, releaseID, id)
	if err != nil {
		return nil, err
	}
	if item.Done == done {
		return item, nil
	}
	item.Done = done
	if err := s.repo.UpdateChecklistItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

// RemoveChecklistItem removes a promo step.
func (s *Service) RemoveChecklistItem(ctx context.Context, releaseID, id uuid.UUID) error {
	item, err := s.checklistItemOf(ctx, releaseID, id)
	if err != nil {
		return err
	}
	return s.repo.DeleteChecklistItem(ctx, item.ID)
}

// ReorderChecklistItems puts the promo steps in the given order.
func (s *Service) ReorderChecklistItems(ctx context.Context, releaseID uuid.UUID, order []uuid.UUID) error {
	items, err := s.repo.ListChecklistItems(ctx, releaseID)
	if err != nil {
		return err
	}
	known := make(map[uuid.UUID]*ChecklistItem, len(items))
	for _, item := range items {
		known[item.ID] = item
	}
	if len(order) != len(items) {
		return errors.Join(ErrInvalid, errors.New("the new order must list every step"))
	}
	seen := make(map[uuid.UUID]bool, len(order))
	for position, id := range order {
		item, ok := known[id]
		if !ok || seen[id] {
			return errors.Join(ErrInvalid, errors.New("unknown step in the new order"))
		}
		seen[id] = true
		item.Position = position
		if err := s.repo.UpdateChecklistItem(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// SaveChecklistAsTemplate stores the release's promo steps as the template
// for future releases (REL-06).
func (s *Service) SaveChecklistAsTemplate(ctx context.Context, releaseID uuid.UUID) error {
	items, err := s.repo.ListChecklistItems(ctx, releaseID)
	if err != nil {
		return err
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return s.repo.SaveChecklistTemplate(ctx, labels)
}

// DefaultTemplateLabels returns the template new releases are seeded from:
// the saved one when there is one, the shipped default otherwise.
func (s *Service) DefaultTemplateLabels(ctx context.Context) ([]string, error) {
	saved, err := s.repo.GetChecklistTemplate(ctx)
	if err != nil {
		return nil, err
	}
	if len(saved) > 0 {
		return saved, nil
	}
	return DefaultChecklistLabels(), nil
}

// seedChecklist gives a new release its promo steps.
func (s *Service) seedChecklist(ctx context.Context, releaseID uuid.UUID) error {
	labels, err := s.DefaultTemplateLabels(ctx)
	if err != nil {
		return err
	}
	for position, label := range labels {
		item := &ChecklistItem{ReleaseID: releaseID, Position: position, Label: label}
		if err := s.repo.InsertChecklistItem(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// checklistItemOf loads a checklist item and checks it belongs to the release.
func (s *Service) checklistItemOf(ctx context.Context, releaseID, id uuid.UUID) (*ChecklistItem, error) {
	item, err := s.repo.GetChecklistItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.ReleaseID != releaseID {
		return nil, ErrNotFound
	}
	return item, nil
}
