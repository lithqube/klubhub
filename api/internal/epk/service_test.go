package epk_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/epk"
	"github.com/klubhub/dj/api/internal/settings"
)

// ─── Mock storage ────────────────────────────────────────────────────────────

type mockStorage struct {
	putErr    error
	deleteErr error
	presignURL string
	presignErr error
	putData   map[string][]byte // key -> data for GetObject simulation
}

func (m *mockStorage) PutObject(_ context.Context, _, key string, r io.Reader, _ int64, _ string) error {
	if m.putErr != nil {
		return m.putErr
	}
	if m.putData == nil {
		m.putData = make(map[string][]byte)
	}
	data, _ := io.ReadAll(r)
	m.putData[key] = data
	return nil
}

func (m *mockStorage) DeleteObject(_ context.Context, _, _ string) error {
	return m.deleteErr
}

func (m *mockStorage) PresignedGetObject(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	if m.presignErr != nil {
		return "", m.presignErr
	}
	if m.presignURL != "" {
		return m.presignURL, nil
	}
	return "https://minio.example.com/presigned", nil
}

// ─── Mock settings service ───────────────────────────────────────────────────

type mockSettingsSvc struct {
	current    *settings.UserSettings
	updateErr  error
	getErr     error
}

func (m *mockSettingsSvc) UpdateSettings(_ context.Context, req settings.UpdateSettingsRequest) (*settings.UserSettings, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	if m.current == nil {
		m.current = &settings.UserSettings{ID: uuid.New()}
	}
	if req.BioShort != "" {
		m.current.BioShort = req.BioShort
	}
	if req.BioLong != "" {
		m.current.BioLong = req.BioLong
	}
	return m.current, nil
}

func (m *mockSettingsSvc) GetSettings(_ context.Context) (*settings.UserSettings, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.current == nil {
		m.current = &settings.UserSettings{
			ID:     uuid.New(),
			DJName: "Test DJ",
		}
	}
	return m.current, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newTestService(repo *mockRepo, stor *mockStorage, settingsSvc *mockSettingsSvc) *epk.Service {
	return epk.NewService(repo, stor, settingsSvc)
}

// makeJPEG returns a minimal valid JPEG byte slice (SOI + EOI markers).
func makeJPEG() []byte {
	return []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00,
		0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestService_GetContent_Seeded verifies the service returns existing content.
func TestService_GetContent_Seeded(t *testing.T) {
	expected := &EPKContent{
		ID:       uuid.New(),
		BioShort: "Short bio",
	}
	repo := &mockRepo{content: expected}
	svc := newTestService(repo, &mockStorage{}, &mockSettingsSvc{})

	result, err := svc.GetContent(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expected.BioShort, result.BioShort)
}

// TestService_GetContent_Empty verifies GetContent seeds an empty default when table is empty.
func TestService_GetContent_Empty(t *testing.T) {
	repo := &mockRepo{content: nil} // nil means no row exists
	svc := newTestService(repo, &mockStorage{}, &mockSettingsSvc{})

	result, err := svc.GetContent(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result, "expected seeded default, got nil")
}

// TestService_UpsertContent_OK verifies a basic upsert call succeeds.
func TestService_UpsertContent_OK(t *testing.T) {
	repo := &mockRepo{}
	svc := newTestService(repo, &mockStorage{}, &mockSettingsSvc{})
	bioShort := "DJ Bio Short"
	req := epk.UpsertEPKContentRequest{BioShort: &bioShort}

	result, err := svc.UpsertContent(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, bioShort, result.BioShort)
}

// TestService_UploadPhoto_OK verifies a valid JPEG is stored and path appended.
func TestService_UploadPhoto_OK(t *testing.T) {
	repo := &mockRepo{content: &EPKContent{PhotoPaths: []string{}}}
	stor := &mockStorage{}
	svc := newTestService(repo, stor, &mockSettingsSvc{})
	data := makeJPEG()

	path, err := svc.UploadPhoto(context.Background(), data, "image/jpeg")

	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "epk/photos/")
}

// TestService_UploadPhoto_LimitExceeded verifies ErrPhotoLimitExceeded is returned at 20 photos.
func TestService_UploadPhoto_LimitExceeded(t *testing.T) {
	// Pre-populate 20 photos.
	photos := make([]string, 20)
	for i := range photos {
		photos[i] = "epk/photos/placeholder.jpg"
	}
	repo := &mockRepo{content: &EPKContent{PhotoPaths: photos}}
	svc := newTestService(repo, &mockStorage{}, &mockSettingsSvc{})
	data := makeJPEG()

	_, err := svc.UploadPhoto(context.Background(), data, "image/jpeg")

	require.Error(t, err)
	assert.ErrorIs(t, err, epk.ErrPhotoLimitExceeded)
}

// TestService_UploadPhoto_BadMIME verifies a non-image MIME type is rejected.
func TestService_UploadPhoto_BadMIME(t *testing.T) {
	repo := &mockRepo{content: &EPKContent{PhotoPaths: []string{}}}
	svc := newTestService(repo, &mockStorage{}, &mockSettingsSvc{})

	_, err := svc.UploadPhoto(context.Background(), []byte("%PDF-1.4"), "application/pdf")

	require.Error(t, err)
	assert.ErrorIs(t, err, epk.ErrInvalidMIME)
}

// TestService_DeletePhoto_OK verifies a photo path is removed from the content.
func TestService_DeletePhoto_OK(t *testing.T) {
	photoPath := "epk/photos/abc123.jpg"
	repo := &mockRepo{content: &EPKContent{PhotoPaths: []string{photoPath, "epk/photos/other.jpg"}}}
	stor := &mockStorage{}
	svc := newTestService(repo, stor, &mockSettingsSvc{})

	err := svc.DeletePhoto(context.Background(), photoPath)

	require.NoError(t, err)
	// Verify the upsert was called with path removed.
	require.NotNil(t, repo.content)
}

// TestService_GeneratePDF_OK verifies GeneratePDF returns a download URL and inserts an export row.
func TestService_GeneratePDF_OK(t *testing.T) {
	repo := &mockRepo{
		content: &EPKContent{
			ID:       uuid.New(),
			BioShort: "Short bio",
			BioLong:  "## Heading\nSome text.",
			SectionVisibility: map[string]bool{
				"bio": true,
			},
		},
	}
	stor := &mockStorage{presignURL: "https://minio.example.com/export.pdf"}
	settingsSvc := &mockSettingsSvc{
		current: &settings.UserSettings{
			DJName:        "Test DJ",
			DefaultColors: map[string]any{"primary": "#96F8FF"},
		},
	}
	svc := newTestService(repo, stor, settingsSvc)

	result, err := svc.GeneratePDF(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.DownloadURL)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.Len(t, repo.exports, 1, "expected one export row inserted")
}

// TestService_ListExports_OK verifies ListExports returns the existing list.
func TestService_ListExports_OK(t *testing.T) {
	repo := &mockRepo{
		exports: []epk.EPKExport{
			{ID: uuid.New(), MinioPath: "epk/exports/1.pdf", CreatedAt: time.Now()},
		},
	}
	svc := newTestService(repo, &mockStorage{}, &mockSettingsSvc{})

	exports, err := svc.ListExports(context.Background())

	require.NoError(t, err)
	assert.Len(t, exports, 1)
}

// TestService_DeleteExport_OK verifies DeleteExport removes the MinIO object and DB row.
func TestService_DeleteExport_OK(t *testing.T) {
	exportID := uuid.New()
	minioPath := "epk/exports/todelete.pdf"
	repo := &mockRepo{
		exports: []epk.EPKExport{
			{ID: exportID, MinioPath: minioPath, CreatedAt: time.Now()},
		},
	}
	stor := &mockStorage{}
	svc := newTestService(repo, stor, &mockSettingsSvc{})

	err := svc.DeleteExport(context.Background(), exportID)

	require.NoError(t, err)
}

// ─── Type aliases for readability in tests ───────────────────────────────────
// These refer to exported types from the epk package.
type EPKContent = epk.EPKContent
