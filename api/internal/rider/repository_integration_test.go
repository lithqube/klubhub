package rider

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Real-database coverage for repository behaviour the fake-repo tests cannot
// see. The service and handler map ErrNotFound to 404; a raw pgx.ErrNoRows
// would surface as a 500.
func TestIntegration_GetTemplate_MissingIsErrNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	repo := NewRepository(testPool)

	if _, err := repo.GetTemplate(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTemplate(unknown id) err = %v, want ErrNotFound", err)
	}
}

func TestIntegration_GetTemplate_SoftDeletedIsErrNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	ctx := context.Background()
	repo := NewRepository(testPool)

	created, err := repo.CreateTemplate(ctx, CreateTemplateInput{
		Name:               "to-delete",
		RiderSectionValues: RiderSectionValues{Technical: "CDJ-3000 x4"},
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if got, err := repo.GetTemplate(ctx, created.ID); err != nil || got.Technical != "CDJ-3000 x4" {
		t.Fatalf("GetTemplate(live) = %+v, %v", got, err)
	}
	if err := repo.SoftDeleteTemplate(ctx, created.ID); err != nil {
		t.Fatalf("SoftDeleteTemplate: %v", err)
	}
	if _, err := repo.GetTemplate(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTemplate(soft-deleted) err = %v, want ErrNotFound", err)
	}
}
