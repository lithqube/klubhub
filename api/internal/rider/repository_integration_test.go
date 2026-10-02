package rider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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
		UpdatedAt:         tpl.UpdatedAt,
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
		UpdatedAt:         tpl.UpdatedAt,
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

	got, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{Name: ptr("renamed"), UpdatedAt: tpl.UpdatedAt})
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
		UpdatedAt:         att.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("UpdateAttachment: %v", err)
	}
	if want := sections("tech-A", "hosp-A", "back-B", "other-A"); sections(got.Technical, got.Hospitality, got.Backline, got.OtherNotes) != want {
		t.Fatalf("sections = %v, want %v", sections(got.Technical, got.Hospitality, got.Backline, got.OtherNotes), want)
	}

	cleared, err := repo.UpdateAttachment(ctx, att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{OtherNotes: ptr("")},
		UpdatedAt:         got.UpdatedAt, // the token returned by the previous save
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

// ─── Optimistic concurrency (updatedAt token) ───────────────────────────────

func TestIntegration_UpdateTemplate_StaleToken_IsErrStaleUpdate_AndChangesNothing(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	tpl := seedTemplate(t, repo, "stale-template")

	// Another writer saves first.
	other, err := repo.UpdateTemplate(ctx, tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("tech-other-writer")},
		UpdatedAt:         tpl.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("first writer: %v", err)
	}

	// We still hold the original token: must not overwrite.
	_, err = repo.UpdateTemplate(ctx, tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("tech-stale-writer")},
		UpdatedAt:         tpl.UpdatedAt,
	})
	if !errors.Is(err, ErrStaleUpdate) {
		t.Fatalf("stale update err = %v, want ErrStaleUpdate", err)
	}
	got, err := repo.GetTemplate(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got.Technical != "tech-other-writer" || !got.UpdatedAt.Equal(other.UpdatedAt) {
		t.Fatalf("stale write changed the row: %+v", got)
	}
}

func TestIntegration_UpdateTemplate_ChainedSavesWithReturnedTokens_Succeed(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	tpl := seedTemplate(t, repo, "chained-template")

	first, err := repo.UpdateTemplate(ctx, tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("one")}, UpdatedAt: tpl.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !first.UpdatedAt.After(tpl.UpdatedAt) {
		t.Fatalf("updatedAt did not advance: %v -> %v", tpl.UpdatedAt, first.UpdatedAt)
	}
	second, err := repo.UpdateTemplate(ctx, tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("two")}, UpdatedAt: first.UpdatedAt,
	})
	if err != nil || second.Technical != "two" {
		t.Fatalf("second save with the returned token: %+v, %v", second, err)
	}
	// The token survives a JSON round trip (the client sends it back as text).
	roundTripped, perr := time.Parse(time.RFC3339Nano, second.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	if _, err := repo.UpdateTemplate(ctx, tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("three")}, UpdatedAt: roundTripped,
	}); err != nil {
		t.Fatalf("token lost precision across a JSON round trip: %v", err)
	}
}

func TestIntegration_UpdateTemplate_MissingOrDeleted_IsErrNotFound_NotConflict(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()

	if _, err := repo.UpdateTemplate(ctx, uuid.New(), UpdateTemplateInput{UpdatedAt: time.Now()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id err = %v, want ErrNotFound", err)
	}
	tpl := seedTemplate(t, repo, "deleted-template")
	if err := repo.SoftDeleteTemplate(ctx, tpl.ID); err != nil {
		t.Fatalf("SoftDeleteTemplate: %v", err)
	}
	if _, err := repo.UpdateTemplate(ctx, tpl.ID, UpdateTemplateInput{UpdatedAt: tpl.UpdatedAt}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("soft-deleted err = %v, want ErrNotFound (gone, not stale)", err)
	}
}

func TestIntegration_UpdateAttachment_StaleToken_IsErrStaleUpdate_AndNotFoundStaysNotFound(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	att, err := repo.CreateAttachment(ctx, CreateAttachmentInput{
		GigID:              seedGig(t),
		RiderSectionValues: RiderSectionValues{Technical: "tech-A"},
	})
	if err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}

	saved, err := repo.UpdateAttachment(ctx, att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("tech-B")}, UpdatedAt: att.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("first writer: %v", err)
	}
	_, err = repo.UpdateAttachment(ctx, att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("tech-stale")}, UpdatedAt: att.UpdatedAt,
	})
	if !errors.Is(err, ErrStaleUpdate) {
		t.Fatalf("stale update err = %v, want ErrStaleUpdate", err)
	}
	got, _ := repo.GetAttachment(ctx, att.ID)
	if got.Technical != "tech-B" || !got.UpdatedAt.Equal(saved.UpdatedAt) {
		t.Fatalf("stale write changed the row: %+v", got)
	}

	if _, err := repo.UpdateAttachment(ctx, uuid.New(), UpdateAttachmentInput{UpdatedAt: time.Now()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id err = %v, want ErrNotFound", err)
	}
	if err := repo.SoftDeleteAttachment(ctx, att.ID); err != nil {
		t.Fatalf("SoftDeleteAttachment: %v", err)
	}
	if _, err := repo.UpdateAttachment(ctx, att.ID, UpdateAttachmentInput{UpdatedAt: saved.UpdatedAt}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("soft-deleted err = %v, want ErrNotFound", err)
	}
}

// ─── Attach only to a live gig ──────────────────────────────────────────────

func TestIntegration_CreateAttachment_SoftDeletedOrMissingGig_IsErrNotFound(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()

	gone := seedGig(t)
	if _, err := testPool.Exec(ctx, `UPDATE gigs SET deleted_at = now() WHERE id = $1`, gone); err != nil {
		t.Fatalf("soft-delete gig: %v", err)
	}
	if _, err := repo.CreateAttachment(ctx, CreateAttachmentInput{GigID: gone}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attach to a soft-deleted gig: err = %v, want ErrNotFound (gigs are only soft-deleted, so the FK never fires)", err)
	}
	var n int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM rider_attachments WHERE gig_id = $1`, gone).Scan(&n); err != nil || n != 0 {
		t.Fatalf("an orphan attachment row was created: count=%d err=%v", n, err)
	}

	if _, err := repo.CreateAttachment(ctx, CreateAttachmentInput{GigID: uuid.New()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attach to a nonexistent gig: err = %v, want ErrNotFound", err)
	}
}

func TestIntegration_CreateAttachment_LiveGig_StillOnePerGig_AndKeepsTheSections(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	gigID := seedGig(t)
	tpl := seedTemplate(t, repo, "snapshot-source")

	att, err := repo.CreateAttachment(ctx, CreateAttachmentInput{
		GigID: gigID, TemplateID: &tpl.ID,
		RiderSectionValues: RiderSectionValues{Technical: "t", Hospitality: "h", Backline: "b", OtherNotes: "o"},
	})
	if err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}
	if att.TemplateID == nil || *att.TemplateID != tpl.ID || sections(att.Technical, att.Hospitality, att.Backline, att.OtherNotes) != sections("t", "h", "b", "o") {
		t.Fatalf("attachment lost data on insert: %+v", att)
	}
	if _, err := repo.CreateAttachment(ctx, CreateAttachmentInput{GigID: gigID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second live attachment for the same gig: err = %v, want ErrConflict", err)
	}
	// A blank-start attachment (no template) also inserts.
	if _, err := repo.CreateAttachment(ctx, CreateAttachmentInput{GigID: seedGig(t)}); err != nil {
		t.Fatalf("attachment without a template: %v", err)
	}
}

// ─── Size / blank constraints (the backstop behind the service's 422s) ──────

func TestIntegration_Constraints_RejectOversizedAndBlank(t *testing.T) {
	requireIntegration(t)
	ctx := context.Background()
	long := strings.Repeat("x", MaxSectionChars+1)

	reject := func(label, sql string, args ...any) {
		t.Helper()
		_, err := testPool.Exec(ctx, sql, args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: err = %v, want a check_violation (23514)", label, err)
		}
	}
	reject("empty name", `INSERT INTO rider_templates (name) VALUES ('')`)
	reject("blank name", `INSERT INTO rider_templates (name) VALUES ('   ')`)
	reject("name over 200", `INSERT INTO rider_templates (name) VALUES ($1)`, strings.Repeat("n", MaxNameChars+1))
	reject("template section over 20000", `INSERT INTO rider_templates (name, technical) VALUES ('c', $1)`, long)
	reject("attachment section over 20000", `INSERT INTO rider_attachments (gig_id, other_notes) VALUES ($1, $2)`, seedGig(t), long)
}

func TestIntegration_Constraints_CountCharactersNotBytes(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	// 200 two-byte characters (400 bytes) and 20000 of them per section.
	name, section := strings.Repeat("é", MaxNameChars), strings.Repeat("é", MaxSectionChars)

	tpl, err := repo.CreateTemplate(ctx, CreateTemplateInput{Name: name,
		RiderSectionValues: RiderSectionValues{Technical: section, Hospitality: section, Backline: section, OtherNotes: section}})
	if err != nil {
		t.Fatalf("limits are in characters; a maximum-size multi-byte template must be accepted: %v", err)
	}
	if len([]rune(tpl.Technical)) != MaxSectionChars {
		t.Fatalf("section was truncated: %d characters", len([]rune(tpl.Technical)))
	}
}

// ─── Unique names / template delete (real Postgres) ─────────────────────────

func TestIntegration_TemplateNames_UniqueCaseInsensitive_AmongLiveTemplates(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	first := seedTemplate(t, repo, "Unique Club Rider")

	for _, dup := range []string{"Unique Club Rider", "unique club rider", "UNIQUE CLUB RIDER"} {
		_, err := repo.CreateTemplate(ctx, CreateTemplateInput{Name: dup})
		if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("create %q: err = %v, want a 'name already exists' ErrInvalidInput", dup, err)
		}
	}

	other := seedTemplate(t, repo, "Unique Festival Rider")
	if _, err := repo.UpdateTemplate(ctx, other.ID, UpdateTemplateInput{Name: ptr("unique club RIDER"), UpdatedAt: other.UpdatedAt}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("rename into an existing name: err = %v, want ErrInvalidInput", err)
	}
	// The rejected rename changed nothing.
	got, _ := repo.GetTemplate(ctx, other.ID)
	if got.Name != "Unique Festival Rider" || !got.UpdatedAt.Equal(other.UpdatedAt) {
		t.Fatalf("a refused rename modified the row: %+v", got)
	}
	// Keeping your own name (only the case differs) is allowed.
	if _, err := repo.UpdateTemplate(ctx, first.ID, UpdateTemplateInput{Name: ptr("UNIQUE club rider"), UpdatedAt: first.UpdatedAt}); err != nil {
		t.Fatalf("re-casing your own name: %v", err)
	}

	// A deleted template frees its name.
	if err := repo.SoftDeleteTemplate(ctx, first.ID); err != nil {
		t.Fatalf("SoftDeleteTemplate: %v", err)
	}
	if _, err := repo.CreateTemplate(ctx, CreateTemplateInput{Name: "unique club rider"}); err != nil {
		t.Fatalf("a deleted template's name should be reusable: %v", err)
	}
}

func TestIntegration_SoftDeleteTemplate_ClearsAttachmentReferences_WithoutTouchingTheirContentOrToken(t *testing.T) {
	repo := requireIntegration(t)
	ctx := context.Background()
	tpl := seedTemplate(t, repo, "Detach Source")
	withTpl, err := repo.CreateAttachment(ctx, CreateAttachmentInput{GigID: seedGig(t), TemplateID: &tpl.ID,
		RiderSectionValues: RiderSectionValues{Technical: "keep me"}})
	if err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}
	other := seedTemplate(t, repo, "Unrelated Source")
	unrelated, _ := repo.CreateAttachment(ctx, CreateAttachmentInput{GigID: seedGig(t), TemplateID: &other.ID})

	if err := repo.SoftDeleteTemplate(ctx, tpl.ID); err != nil {
		t.Fatalf("SoftDeleteTemplate: %v", err)
	}

	got, err := repo.GetAttachment(ctx, withTpl.ID)
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if got.TemplateID != nil {
		t.Fatalf("template_id = %v, want NULL after the template was deleted", got.TemplateID)
	}
	if got.Technical != "keep me" {
		t.Fatalf("content changed: %q", got.Technical)
	}
	if !got.UpdatedAt.Equal(withTpl.UpdatedAt) {
		t.Fatalf("updated_at moved (%v -> %v): every open editor's token would go stale", withTpl.UpdatedAt, got.UpdatedAt)
	}
	stillThere, _ := repo.GetAttachment(ctx, unrelated.ID)
	if stillThere.TemplateID == nil || *stillThere.TemplateID != other.ID {
		t.Fatalf("an unrelated attachment lost its template: %+v", stillThere.TemplateID)
	}
	// And the token still works for a save.
	if _, err := repo.UpdateAttachment(ctx, withTpl.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{Technical: ptr("edited after template delete")}, UpdatedAt: got.UpdatedAt}); err != nil {
		t.Fatalf("save with the pre-delete token: %v", err)
	}

	if err := repo.SoftDeleteTemplate(ctx, tpl.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
}
