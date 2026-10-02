package rider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRepo is an in-memory implementation of RepoIface for unit tests.
// Mirrors the EPK `mockRepo` pattern (Phase 03 contract tests). No DB, no
// testcontainers — Postgres-specific code paths are exercised by the real
// Repository against a live testcontainer in integration_test.go.
type fakeRepo struct {
	templates   map[uuid.UUID]*RiderTemplate
	attachments map[uuid.UUID]*RiderAttachment
	byGig       map[uuid.UUID]uuid.UUID // gigID → attachmentID (live only)

	// Error injection for negative-path tests.
	listErr       error
	getTplErr     error
	createTplErr  error
	updateTplErr  error
	deleteTplErr  error
	getAttErr     error
	getByGigErr   error
	createAttErr  error
	updateAttErr  error
	deleteAttErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		templates:   make(map[uuid.UUID]*RiderTemplate),
		attachments: make(map[uuid.UUID]*RiderAttachment),
		byGig:       make(map[uuid.UUID]uuid.UUID),
	}
}

func (m *fakeRepo) ListTemplates(_ context.Context) ([]*RiderTemplate, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	out := make([]*RiderTemplate, 0, len(m.templates))
	for _, t := range m.templates {
		if t.DeletedAt == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *fakeRepo) GetTemplate(_ context.Context, id uuid.UUID) (*RiderTemplate, error) {
	if m.getTplErr != nil {
		return nil, m.getTplErr
	}
	t, ok := m.templates[id]
	if !ok || t.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return t, nil
}

func (m *fakeRepo) CreateTemplate(_ context.Context, in CreateTemplateInput) (*RiderTemplate, error) {
	if m.createTplErr != nil {
		return nil, m.createTplErr
	}
	now := time.Now()
	t := &RiderTemplate{
		ID:          uuid.New(),
		Name:        in.Name,
		Technical:   in.Technical,
		Hospitality: in.Hospitality,
		Backline:    in.Backline,
		OtherNotes:  in.OtherNotes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.templates[t.ID] = t
	return t, nil
}

func (m *fakeRepo) UpdateTemplate(_ context.Context, id uuid.UUID, in UpdateTemplateInput) (*RiderTemplate, error) {
	if m.updateTplErr != nil {
		return nil, m.updateTplErr
	}
	t, ok := m.templates[id]
	if !ok || t.DeletedAt != nil {
		return nil, ErrNotFound
	}
	// Compare-and-set like the real UPDATE ... AND updated_at = $n.
	if !in.UpdatedAt.Equal(t.UpdatedAt) {
		return nil, ErrConflict
	}
	if in.Name != nil {
		t.Name = *in.Name
	}
	// Mirror the real Repository's COALESCE semantics per field: a non-nil
	// pointer updates that column (even to ""); a nil pointer leaves it alone.
	applyPatch(in.RiderSectionPatch, &t.Technical, &t.Hospitality, &t.Backline, &t.OtherNotes)
	t.UpdatedAt = time.Now()
	return t, nil
}

func (m *fakeRepo) SoftDeleteTemplate(_ context.Context, id uuid.UUID) error {
	if m.deleteTplErr != nil {
		return m.deleteTplErr
	}
	t, ok := m.templates[id]
	if !ok || t.DeletedAt != nil {
		return ErrNotFound
	}
	now := time.Now()
	t.DeletedAt = &now
	return nil
}

func (m *fakeRepo) GetAttachmentByGig(_ context.Context, gigID uuid.UUID) (*RiderAttachment, error) {
	if m.getByGigErr != nil {
		return nil, m.getByGigErr
	}
	id, ok := m.byGig[gigID]
	if !ok {
		return nil, ErrNotFound
	}
	a, ok := m.attachments[id]
	if !ok || a.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return a, nil
}

func (m *fakeRepo) GetAttachment(_ context.Context, id uuid.UUID) (*RiderAttachment, error) {
	if m.getAttErr != nil {
		return nil, m.getAttErr
	}
	a, ok := m.attachments[id]
	if !ok || a.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return a, nil
}

func (m *fakeRepo) CreateAttachment(_ context.Context, in CreateAttachmentInput) (*RiderAttachment, error) {
	if m.createAttErr != nil {
		return nil, m.createAttErr
	}
	// Simulate the partial unique index: a live attachment for this gig
	// already exists → ErrConflict.
	if existingID, ok := m.byGig[in.GigID]; ok {
		if existing, ok := m.attachments[existingID]; ok && existing.DeletedAt == nil {
			return nil, ErrConflict
		}
	}
	now := time.Now()
	a := &RiderAttachment{
		ID:          uuid.New(),
		GigID:       in.GigID,
		TemplateID:  in.TemplateID,
		Technical:   in.Technical,
		Hospitality: in.Hospitality,
		Backline:    in.Backline,
		OtherNotes:  in.OtherNotes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.attachments[a.ID] = a
	m.byGig[in.GigID] = a.ID
	return a, nil
}

func (m *fakeRepo) UpdateAttachment(_ context.Context, id uuid.UUID, in UpdateAttachmentInput) (*RiderAttachment, error) {
	if m.updateAttErr != nil {
		return nil, m.updateAttErr
	}
	a, ok := m.attachments[id]
	if !ok || a.DeletedAt != nil {
		return nil, ErrNotFound
	}
	if !in.UpdatedAt.Equal(a.UpdatedAt) {
		return nil, ErrConflict
	}
	applyPatch(in.RiderSectionPatch, &a.Technical, &a.Hospitality, &a.Backline, &a.OtherNotes)
	a.UpdatedAt = time.Now()
	return a, nil
}

// applyPatch sets each destination only when its patch pointer is non-nil.
func applyPatch(p RiderSectionPatch, tech, hosp, back, other *string) {
	if p.Technical != nil {
		*tech = *p.Technical
	}
	if p.Hospitality != nil {
		*hosp = *p.Hospitality
	}
	if p.Backline != nil {
		*back = *p.Backline
	}
	if p.OtherNotes != nil {
		*other = *p.OtherNotes
	}
}

func (m *fakeRepo) SoftDeleteAttachment(_ context.Context, id uuid.UUID) error {
	if m.deleteAttErr != nil {
		return m.deleteAttErr
	}
	a, ok := m.attachments[id]
	if !ok || a.DeletedAt != nil {
		return ErrNotFound
	}
	now := time.Now()
	a.DeletedAt = &now
	delete(m.byGig, a.GigID)
	return nil
}

// ─── Tests ─────────────────────────────────────────────────────────────────

// Repository interface compliance — the fake must satisfy RepoIface so
// the service layer can be wired against it in Plan 02.
var _ RepoIface = (*fakeRepo)(nil)

func TestFakeRepo_ListTemplates_ExcludesSoftDeleted(t *testing.T) {
	repo := newFakeRepo()
	a, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "A"})
	_, _ = repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "B"})
	if err := repo.SoftDeleteTemplate(context.Background(), a.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	got, err := repo.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 live template, got %d", len(got))
	}
	if got[0].Name != "B" {
		t.Fatalf("expected only B, got %q", got[0].Name)
	}
}

func TestFakeRepo_GetTemplate_NotFoundForMissingID(t *testing.T) {
	repo := newFakeRepo()
	_, err := repo.GetTemplate(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFakeRepo_GetTemplate_NotFoundForSoftDeleted(t *testing.T) {
	repo := newFakeRepo()
	a, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "A"})
	_ = repo.SoftDeleteTemplate(context.Background(), a.ID)
	_, err := repo.GetTemplate(context.Background(), a.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFakeRepo_CreateTemplate_RoundTripsAllFields(t *testing.T) {
	repo := newFakeRepo()
	in := CreateTemplateInput{
		Name: "Standard club",
		RiderSectionValues: RiderSectionValues{
			Technical:   "2× CDJ-3000",
			Hospitality: "4× bottled water",
			Backline:    "DJM-A9",
			OtherNotes:  "Arrival 2h before doors",
		},
	}
	created, err := repo.CreateTemplate(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("expected non-zero ID")
	}
	if created.Name != in.Name || created.Technical != in.Technical ||
		created.Hospitality != in.Hospitality || created.Backline != in.Backline ||
		created.OtherNotes != in.OtherNotes {
		t.Fatalf("field round-trip failed: %+v", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("expected non-zero timestamps")
	}
}

func TestFakeRepo_UpdateTemplate_PartialOnlyUpdatesNamedFields(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{
		Name: "Original",
		RiderSectionValues: RiderSectionValues{
			Technical:   "tech-A",
			Hospitality: "hosp-A",
			Backline:    "back-A",
			OtherNotes:  "other-A",
		},
	})
	// Name + one section: the other three sections are not mentioned, so
	// they must be left alone.
	updated, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{
		Name:              ptr("Renamed"),
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("tech-B")},
		UpdatedAt:         tpl.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Fatalf("name update failed: %q", updated.Name)
	}
	if updated.Technical != "tech-B" {
		t.Fatalf("technical update failed: %q", updated.Technical)
	}
	if updated.Hospitality != "hosp-A" || updated.Backline != "back-A" || updated.OtherNotes != "other-A" {
		t.Fatalf("untouched fields mutated: %+v", updated)
	}
}

func TestFakeRepo_UpdateTemplate_NilSectionValues_LeavesAllSectionsAlone(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{
		Name: "Original",
		RiderSectionValues: RiderSectionValues{
			Technical: "tech-A", Hospitality: "hosp-A", Backline: "back-A", OtherNotes: "other-A",
		},
	})
	updated, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{Name: ptr("Renamed"), UpdatedAt: tpl.UpdatedAt})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Fatalf("name update failed: %q", updated.Name)
	}
	for _, c := range []struct {
		label, got, want string
	}{{"technical", updated.Technical, "tech-A"}, {"hospitality", updated.Hospitality, "hosp-A"}, {"backline", updated.Backline, "back-A"}, {"otherNotes", updated.OtherNotes, "other-A"}} {
		if c.got != c.want {
			t.Fatalf("%s mutated: got %q want %q", c.label, c.got, c.want)
		}
	}
}

func TestFakeRepo_SoftDeleteTemplate_NotFoundOnSecondCall(t *testing.T) {
	repo := newFakeRepo()
	a, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "A"})
	if err := repo.SoftDeleteTemplate(context.Background(), a.ID); err != nil {
		t.Fatalf("first soft delete: %v", err)
	}
	if err := repo.SoftDeleteTemplate(context.Background(), a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second soft delete: expected ErrNotFound, got %v", err)
	}
}

func TestFakeRepo_CreateAttachment_WithTemplateID_RecordsSnapshot(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{
		Name: "Standard club",
		RiderSectionValues: RiderSectionValues{
			Technical: "tech-tpl", Hospitality: "hosp-tpl", Backline: "back-tpl", OtherNotes: "other-tpl",
		},
	})
	gigID := uuid.New()
	// Caller-provided empty section values are *ignored* when a template is
	// supplied (service layer is responsible for copying template values
	// before the insert); the fake records whatever the caller passes.
	att, err := repo.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID:      gigID,
		TemplateID: &tpl.ID,
	})
	if err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	if att.TemplateID == nil || *att.TemplateID != tpl.ID {
		t.Fatalf("template_id not recorded: %+v", att.TemplateID)
	}
	// Subsequent template update must NOT mutate the attachment (snapshot
	// semantics). Verify by directly reading the attachment after the
	// update.
	newName := "Renamed"
	if _, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{Name: &newName, UpdatedAt: tpl.UpdatedAt}); err != nil {
		t.Fatalf("update template: %v", err)
	}
	again, err := repo.GetAttachment(context.Background(), att.ID)
	if err != nil {
		t.Fatalf("get attachment: %v", err)
	}
	// The fake's UpdateTemplate doesn't propagate changes to attachments
	// (the real repository neither), so this is a defence-in-depth check.
	if again.TemplateID == nil || *again.TemplateID != tpl.ID {
		t.Fatal("attachment template_id drifted from the source template")
	}
}

func TestFakeRepo_UpdateAttachment_DoesNotChangeTemplateID(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "tpl"})
	att, _ := repo.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID:      uuid.New(),
		TemplateID: &tpl.ID,
	})
	updated, err := repo.UpdateAttachment(context.Background(), att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("new")},
		UpdatedAt:         att.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.TemplateID == nil || *updated.TemplateID != tpl.ID {
		t.Fatalf("template_id changed on attachment update: %+v", updated.TemplateID)
	}
}

func TestFakeRepo_CreateAttachment_DuplicateGig_ReturnsConflict(t *testing.T) {
	repo := newFakeRepo()
	gigID := uuid.New()
	if _, err := repo.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: gigID}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: gigID})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict on duplicate gig, got %v", err)
	}
}

func TestFakeRepo_GetAttachmentByGig_OnlyReturnsLive(t *testing.T) {
	repo := newFakeRepo()
	first, _ := repo.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: uuid.New()})
	if err := repo.SoftDeleteAttachment(context.Background(), first.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	_, err := repo.GetAttachmentByGig(context.Background(), first.GigID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after soft delete, got %v", err)
	}
}

func TestFakeRepo_UpdateAttachment_ThenGetReturnsNewValues(t *testing.T) {
	repo := newFakeRepo()
	att, _ := repo.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID: uuid.New(),
		RiderSectionValues: RiderSectionValues{
			Technical: "old-tech", Hospitality: "old-hosp", Backline: "old-back", OtherNotes: "old-other",
		},
	})
	updated, err := repo.UpdateAttachment(context.Background(), att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("new-tech")},
		UpdatedAt:         att.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Technical != "new-tech" {
		t.Fatalf("technical update failed: %q", updated.Technical)
	}
	if updated.Hospitality != "old-hosp" || updated.Backline != "old-back" || updated.OtherNotes != "old-other" {
		t.Fatalf("untouched fields mutated: %+v", updated)
	}
	again, err := repo.GetAttachment(context.Background(), att.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if again.Technical != "new-tech" {
		t.Fatalf("GetAttachment returned stale technical: %q", again.Technical)
	}
}

func TestFakeRepo_GetAttachment_NotFoundForMissingID(t *testing.T) {
	repo := newFakeRepo()
	_, err := repo.GetAttachment(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFakeRepo_SoftDeleteAttachment_NotFoundOnSecondCall(t *testing.T) {
	repo := newFakeRepo()
	att, _ := repo.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: uuid.New()})
	if err := repo.SoftDeleteAttachment(context.Background(), att.ID); err != nil {
		t.Fatalf("first soft delete: %v", err)
	}
	if err := repo.SoftDeleteAttachment(context.Background(), att.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second soft delete: expected ErrNotFound, got %v", err)
	}
}

// TestRepoIfaceCompliance_Failing exists for symmetry and to ensure the
// Repository concrete type satisfies the same interface as the fake.
// Compile-time assertion only.
var _ RepoIface = (*Repository)(nil)

// ptr is a tiny helper for tests that need *string literals inline.
func ptr[T any](v T) *T { return &v }