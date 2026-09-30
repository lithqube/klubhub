package rider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/settings"
)

// ─── Mock storage ────────────────────────────────────────────────────────────

type fakeStorage struct {
	putErr        error
	presignURL    string
	presignErr    error
	putKeys       []string
	lastPutData   []byte
	lastPutBucket string
	lastPutKey    string
	lastPutCT     string
}

func (m *fakeStorage) PutObject(_ context.Context, bucket, key string, r io.Reader, _ int64, contentType string) error {
	if m.putErr != nil {
		return m.putErr
	}
	data, _ := io.ReadAll(r)
	m.putKeys = append(m.putKeys, key)
	m.lastPutBucket = bucket
	m.lastPutKey = key
	m.lastPutData = data
	m.lastPutCT = contentType
	return nil
}

func (m *fakeStorage) PresignedGetObject(_ context.Context, _, key string, _ time.Duration) (string, error) {
	if m.presignErr != nil {
		return "", m.presignErr
	}
	if m.presignURL != "" {
		return m.presignURL, nil
	}
	return "https://garage.example.com/presigned/" + key, nil
}

func (m *fakeStorage) DeleteObject(_ context.Context, _, _ string) error {
	return nil
}

// ─── Mock settings service ──────────────────────────────────────────────────

type fakeSettings struct {
	settings *settings.UserSettings
	err      error
}

func (m *fakeSettings) GetSettings(_ context.Context) (*settings.UserSettings, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.settings == nil {
		return &settings.UserSettings{DJName: "Test DJ"}, nil
	}
	return m.settings, nil
}

// ─── Service interface compliance ───────────────────────────────────────────

var _ RepoIface = (*fakeRepo)(nil)
var _ StorageIface = (*fakeStorage)(nil)
var _ SettingsIface = (*fakeSettings)(nil)

// ─── Tests ─────────────────────────────────────────────────────────────────

func TestService_CreateTemplate_EmptyName_ReturnsInvalidInput(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeStorage{}, &fakeSettings{})
	_, err := svc.CreateTemplate(context.Background(), CreateTemplateInput{Name: ""})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_CreateTemplate_WhitespaceOnlyName_ReturnsInvalidInput(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeStorage{}, &fakeSettings{})
	_, err := svc.CreateTemplate(context.Background(), CreateTemplateInput{Name: "   "})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_CreateTemplate_TrimsNameWhitespace(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	out, err := svc.CreateTemplate(context.Background(), CreateTemplateInput{Name: "  Standard club  "})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Name != "Standard club" {
		t.Fatalf("expected trimmed name, got %q", out.Name)
	}
}

func TestService_UpdateTemplate_BlankNameRejected(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "Original"})
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	blank := "   "
	_, err := svc.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{Name: &blank})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_CreateAttachment_WithTemplate_CopiesValues(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{
		Name: "Standard club",
		RiderSectionValues: RiderSectionValues{
			Technical:   "tpl-tech",
			Hospitality: "tpl-hosp",
			Backline:    "tpl-back",
			OtherNotes:  "tpl-other",
		},
	})
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	att, err := svc.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID:      uuid.New(),
		TemplateID: &tpl.ID,
		// All four section fields left blank — the service should copy
		// from the template, not persist blanks.
	})
	require.NoError(t, err)
	assert.Equal(t, "tpl-tech", att.Technical)
	assert.Equal(t, "tpl-hosp", att.Hospitality)
	assert.Equal(t, "tpl-back", att.Backline)
	assert.Equal(t, "tpl-other", att.OtherNotes)
	require.NotNil(t, att.TemplateID)
	assert.Equal(t, tpl.ID, *att.TemplateID)

	// Snapshot semantics: subsequent template update must NOT mutate the
	// attachment.
	newName := "Renamed"
	if _, err := repo.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{Name: &newName}); err != nil {
		t.Fatalf("update template: %v", err)
	}
	again, err := repo.GetAttachment(context.Background(), att.ID)
	require.NoError(t, err)
	assert.Equal(t, "tpl-tech", again.Technical, "attachment must be a snapshot, not affected by template update")
}

func TestService_CreateAttachment_WithNilTemplate_UsesInputValues(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	att, err := svc.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID: uuid.New(),
		RiderSectionValues: RiderSectionValues{
			Technical:   "manual-tech",
			Hospitality: "manual-hosp",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "manual-tech", att.Technical)
	assert.Equal(t, "manual-hosp", att.Hospitality)
	assert.Empty(t, att.Backline)
	assert.Empty(t, att.OtherNotes)
	assert.Nil(t, att.TemplateID)
}

func TestService_CreateAttachment_WithMissingTemplate_ReturnsNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	bogus := uuid.New()
	_, err := svc.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID:      uuid.New(),
		TemplateID: &bogus,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_CreateAttachment_DuplicateGig_PropagatesConflict(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	gigID := uuid.New()
	_, err := svc.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: gigID})
	require.NoError(t, err)
	_, err = svc.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: gigID})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestService_GeneratePDF_RendersAndStores(t *testing.T) {
	repo := newFakeRepo()
	storage := &fakeStorage{presignURL: "https://garage/dl"}
	settingsSvc := &fakeSettings{settings: &settings.UserSettings{DJName: "Test DJ"}}
	svc := NewService(repo, storage, settingsSvc)

	att, _ := svc.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID: uuid.New(),
		RiderSectionValues: RiderSectionValues{
			Technical:   "2× CDJ-3000",
			Hospitality: "4× bottled water",
			Backline:    "DJM-A9",
			OtherNotes:  "Arrival 2h before doors",
		},
	})

	res, err := svc.GeneratePDF(context.Background(), att.ID)
	require.NoError(t, err)
	assert.Equal(t, att.ID, res.ID)
	assert.Equal(t, "https://garage/dl", res.DownloadURL)
	assert.False(t, res.CreatedAt.IsZero())

	// Verify storage was called with the expected key + content-type.
	assert.Equal(t, "rider/exports/"+att.ID.String()+".pdf", storage.lastPutKey)
	assert.Equal(t, StorageBucket, storage.lastPutBucket)
	assert.Equal(t, "application/pdf", storage.lastPutCT)

	// Verify the PDF is non-empty and starts with the %PDF- header.
	require.NotEmpty(t, storage.lastPutData)
	assert.True(t, bytes.HasPrefix(storage.lastPutData, []byte("%PDF-")),
		"expected PDF magic header, got %q", storage.lastPutData[:8])
}

func TestService_GeneratePDF_SettingsErrorPropagates(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{err: errors.New("settings down")})
	att, _ := svc.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: uuid.New()})
	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "settings")
}

func TestService_GeneratePDF_StorageErrorPropagates(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{putErr: errors.New("garage down")}, &fakeSettings{})
	att, _ := svc.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: uuid.New()})
	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store PDF")
}

func TestService_GeneratePDF_MissingAttachmentReturnsNotFound(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeStorage{}, &fakeSettings{})
	_, err := svc.GeneratePDF(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_DeleteTemplate_CallsSoftDelete(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "X"})
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	if err := svc.DeleteTemplate(context.Background(), tpl.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := repo.GetTemplate(context.Background(), tpl.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestService_DeleteAttachment_CallsSoftDelete(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	att, _ := svc.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: uuid.New()})
	if err := svc.DeleteAttachment(context.Background(), att.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := repo.GetAttachmentByGig(context.Background(), att.GigID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

// ─── PDF renderer direct test ──────────────────────────────────────────────

func TestRenderRiderPDF_HappyPath_StartsWithPDFHeader(t *testing.T) {
	att := &RiderAttachment{
		ID:          uuid.New(),
		Technical:   "2× CDJ-3000",
		Hospitality: "4× bottled water",
		Backline:    "DJM-A9",
		OtherNotes:  "Arrival 2h before doors",
	}
	us := &settings.UserSettings{DJName: "Test DJ"}
	var buf bytes.Buffer
	err := renderRiderPDF(att, "Berghain", "Berlin", "Germany", time.Date(2026, 3, 14, 22, 0, 0, 0, time.UTC), us, &buf)
	require.NoError(t, err)
	require.NotEmpty(t, buf.Bytes())
	assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")),
		"renderer should produce a valid PDF magic header")
}

func TestRenderRiderPDF_SkipsBlankSections(t *testing.T) {
	att := &RiderAttachment{
		ID:        uuid.New(),
		Technical: "only-tech",
		// Hospitality, Backline, OtherNotes intentionally blank
	}
	us := &settings.UserSettings{DJName: "Solo Tech DJ"}
	var buf bytes.Buffer
	err := renderRiderPDF(att, "", "", "", time.Time{}, us, &buf)
	require.NoError(t, err)
	// Sanity check the PDF still produces output even with most fields
	// blank — the renderer should not crash.
	assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
}

func TestRenderRiderPDF_FallsBackAccentWhenColorsEmpty(t *testing.T) {
	att := &RiderAttachment{ID: uuid.New(), Technical: "tech"}
	us := &settings.UserSettings{DJName: "X"} // DefaultColors nil
	var buf bytes.Buffer
	require.NoError(t, renderRiderPDF(att, "", "", "", time.Time{}, us, &buf))
	assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
}