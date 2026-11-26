package release

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Deadline is one named date a release is due (REL-03).
type Deadline struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ReleaseID uuid.UUID `json:"release_id" db:"release_id"`
	Name      string    `json:"name" db:"name"`
	DueDate   time.Time `json:"due_date" db:"due_date"`
	Completed bool      `json:"completed" db:"completed"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsOverdue reports whether the deadline has passed while it is still open.
// Overdue is computed, never stored.
func (d *Deadline) IsOverdue(now time.Time) bool {
	if d.Completed {
		return false
	}
	return dayOf(d.DueDate).Before(dayOf(now))
}

// AddDeadline adds a named deadline to a release.
func (s *Service) AddDeadline(ctx context.Context, releaseID uuid.UUID, name string, dueDate time.Time) (*Deadline, error) {
	if _, err := s.repo.GetByID(ctx, releaseID); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.Join(ErrInvalid, errors.New("deadline name is required"))
	}
	if dueDate.IsZero() {
		return nil, errors.Join(ErrInvalid, errors.New("due_date is required"))
	}
	d := &Deadline{
		ReleaseID: releaseID,
		Name:      trimmed,
		DueDate:   dayOf(dueDate),
	}
	if err := s.repo.InsertDeadline(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// RenameDeadline renames a deadline of a release.
func (s *Service) RenameDeadline(ctx context.Context, releaseID, id uuid.UUID, name string) (*Deadline, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.Join(ErrInvalid, errors.New("deadline name is required"))
	}
	d, err := s.deadlineOf(ctx, releaseID, id)
	if err != nil {
		return nil, err
	}
	d.Name = trimmed
	if err := s.repo.UpdateDeadline(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// CompleteDeadline marks a deadline done. Completing a completed deadline is
// a no-op, so a retried tap does not fail.
func (s *Service) CompleteDeadline(ctx context.Context, releaseID, id uuid.UUID) (*Deadline, error) {
	d, err := s.deadlineOf(ctx, releaseID, id)
	if err != nil {
		return nil, err
	}
	if d.Completed {
		return d, nil
	}
	d.Completed = true
	if err := s.repo.UpdateDeadline(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// RemoveDeadline removes a deadline of a release.
func (s *Service) RemoveDeadline(ctx context.Context, releaseID, id uuid.UUID) error {
	d, err := s.deadlineOf(ctx, releaseID, id)
	if err != nil {
		return err
	}
	return s.repo.DeleteDeadline(ctx, d.ID)
}

// GetDeadline returns one deadline by ID.
func (s *Service) GetDeadline(ctx context.Context, id uuid.UUID) (*Deadline, error) {
	return s.repo.GetDeadline(ctx, id)
}

// ListDeadlines returns the deadlines of a release, soonest first.
func (s *Service) ListDeadlines(ctx context.Context, releaseID uuid.UUID) ([]*Deadline, error) {
	if _, err := s.repo.GetByID(ctx, releaseID); err != nil {
		return nil, err
	}
	return s.repo.ListDeadlines(ctx, releaseID)
}

// deadlineOf loads a deadline and checks it belongs to the release.
func (s *Service) deadlineOf(ctx context.Context, releaseID, id uuid.UUID) (*Deadline, error) {
	d, err := s.repo.GetDeadline(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.ReleaseID != releaseID {
		return nil, ErrNotFound
	}
	return d, nil
}

// dayOf strips the time part so dates compare on the day, not the instant.
func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
