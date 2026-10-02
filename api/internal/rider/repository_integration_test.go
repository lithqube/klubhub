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

// The data-loss regression: a partial update must change only the sections it
// names. Fake-repo tests could not catch this because the double had the same
// all-or-nothing behaviour as the bug; this runs the real COALESCE SQL.
func seedTemplate(t *testing.T, repo *Repository, name string) *RiderTemplate {
	t.Helper()
	tpl, err := repo.CreateTemplate(context.Background(), CreateTemplateInput{
		Name: name,
		RiderSectionValues: RiderSectionValues{
			Technical: "tech-A", Hospitality: "hosp-A", Backline: "back-A", OtherNotes: "other-A",
		},
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	return tpl
}

func requireIntegration(t *testing.T) *Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	return NewRepository(testPool)
}

func sections(tech, hosp, back, other string) [4]string { return [4]string{tech, hosp, back, other} }

func templateSections(t *RiderTemplate) [4]string {
	return sections(t.Technical, t.Hospitality, t.Backline, t.OtherNotes)
}

func TestIntegration_UpdateTemplate_SingleSection_LeavesOthersAlone(t *testing.T) {
	repo := requireIntegration(t)
	tpl := seedTemplate(t, repo, "single-section")

	got, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("tech-B")},
	})
	if err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if want := sections("tech-B", "hosp-A", "back-A", "other-A"); templateSections(got) != want {
		t.Fatalf("sections = %v, want %v (other sections must be untouched)", templateSections(got), want)
	}
	if got.Name != "single-section" {
		t.Fatalf("name changed: %q", got.Name)
	}
}

func TestIntegration_UpdateTemplate_EmptyString_ClearsOnlyThatSection(t *testing.T) {
	repo := requireIntegration(t)
	tpl := seedTemplate(t, repo, "clear-one")

	got, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Hospitality: ptr("")},
	})
	if err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if want := sections("tech-A", "", "back-A", "other-A"); templateSections(got) != want {
		t.Fatalf("sections = %v, want %v", templateSections(got), want)
	}
}

func TestIntegration_UpdateTemplate_NameOnly_LeavesSectionsAlone(t *testing.T) {
	repo := requireIntegration(t)
	tpl := seedTemplate(t, repo, "name-only")

	got, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{Name: ptr("renamed")})
	if err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if got.Name != "renamed" || templateSections(got) != sections("tech-A", "hosp-A", "back-A", "other-A") {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestIntegration_UpdateAttachment_SingleSection_LeavesOthersAlone(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	tpl := seedTemplate(t, repo, "att-source")
	gigID := seedGig(t)
	att, err := repo.CreateAttachment(ctx, CreateAttachmentInput{
		GigID: gigID, TemplateID: &tpl.ID,
		RiderSectionValues: RiderSectionValues{
			Technical: "tech-A", Hospitality: "hosp-A", Backline: "back-A", OtherNotes: "other-A",
		},
	})
	if err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}

	got, err := repo.UpdateAttachment(ctx, att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{Backline: ptr("back-B")},
	})
	if err != nil {
		t.Fatalf("UpdateAttachment: %v", err)
	}
	if want := sections("tech-A", "hosp-A", "back-B", "other-A"); sections(got.Technical, got.Hospitality, got.Backline, got.OtherNotes) != want {
		t.Fatalf("sections = %v, want %v", sections(got.Technical, got.Hospitality, got.Backline, got.OtherNotes), want)
	}

	cleared, err := repo.UpdateAttachment(ctx, att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{OtherNotes: ptr("")},
	})
	if err != nil {
		t.Fatalf("UpdateAttachment(clear): %v", err)
	}
	if want := sections("tech-A", "hosp-A", "back-B", ""); sections(cleared.Technical, cleared.Hospitality, cleared.Backline, cleared.OtherNotes) != want {
		t.Fatalf("after clear = %v, want %v", sections(cleared.Technical, cleared.Hospitality, cleared.Backline, cleared.OtherNotes), want)
	}
}


// seedGig inserts the minimal live gig a rider attachment can reference.
func seedGig(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO gigs (date, venue) VALUES (now(), 'Rider test venue') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("seed gig: %v", err)
	}
	return id
}
