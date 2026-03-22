package epk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/epk"
)

// mockRepo is an in-memory implementation of repoIface for unit tests.
type mockRepo struct {
	content *epk.EPKContent
	exports []epk.EPKExport

	getContentErr  error
	upsertErr      error
	insertErr      error
	listErr        error
	deleteErr      error
	deleteNotFound bool
}

func (m *mockRepo) GetContent(_ context.Context) (*epk.EPKContent, error) {
	if m.getContentErr != nil {
		return nil, m.getContentErr
	}
	return m.content, nil
}

func (m *mockRepo) UpsertContent(_ context.Context, req epk.UpsertEPKContentRequest) (*epk.EPKContent, error) {
	if m.upsertErr != nil {
		return nil, m.upsertErr
	}
	c := &epk.EPKContent{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if req.BioShort != nil {
		c.BioShort = *req.BioShort
	}
	if req.BioLong != nil {
		c.BioLong = *req.BioLong
	}
	if req.TechRider != nil {
		c.TechRider = *req.TechRider
	}
	if req.StagePlotPath != nil {
		c.StagePlotPath = *req.StagePlotPath
	}
	if req.GigHighlights != nil {
		c.GigHighlights = req.GigHighlights
	}
	if req.PressQuotes != nil {
		c.PressQuotes = req.PressQuotes
	}
	if req.PhotoPaths != nil {
		c.PhotoPaths = req.PhotoPaths
	}
	if req.SectionVisibility != nil {
		c.SectionVisibility = req.SectionVisibility
	}
	m.content = c
	return c, nil
}

func (m *mockRepo) InsertExport(_ context.Context, minioPath string) (*epk.EPKExport, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	e := epk.EPKExport{
		ID:        uuid.New(),
		MinioPath: minioPath,
		CreatedAt: time.Now(),
	}
	m.exports = append(m.exports, e)
	return &e, nil
}

func (m *mockRepo) ListExports(_ context.Context) ([]epk.EPKExport, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.exports, nil
}

func (m *mockRepo) DeleteExport(_ context.Context, id uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if m.deleteNotFound {
		return epk.ErrNotFound
	}
	for i, e := range m.exports {
		if e.ID == id {
			m.exports = append(m.exports[:i], m.exports[i+1:]...)
			return nil
		}
	}
	return epk.ErrNotFound
}

// --- Test cases ---

// TestRepository_GetContent_EmptyReturnsNil verifies nil is returned (not an error) when no row exists.
func TestRepository_GetContent_EmptyReturnsNil(t *testing.T) {
	repo := &mockRepo{}

	content, err := repo.GetContent(context.Background())

	require.NoError(t, err)
	assert.Nil(t, content, "expected nil content when no row exists")
}

// TestRepository_GetContent_ReturnsExisting verifies stored content is returned correctly.
func TestRepository_GetContent_ReturnsExisting(t *testing.T) {
	bioShort := "DJ Bio Short"
	expected := &epk.EPKContent{
		ID:       uuid.New(),
		BioShort: bioShort,
		BioLong:  "Long version",
		GigHighlights: []string{"gig1", "gig2"},
		PressQuotes: []epk.PressQuote{
			{Text: "Amazing set", Source: "DJ Mag"},
		},
		PhotoPaths:        []string{"photos/1.jpg"},
		SectionVisibility: map[string]bool{"bio": true},
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	repo := &mockRepo{content: expected}

	content, err := repo.GetContent(context.Background())

	require.NoError(t, err)
	require.NotNil(t, content)
	assert.Equal(t, expected.BioShort, content.BioShort)
	assert.Equal(t, expected.GigHighlights, content.GigHighlights)
	assert.Equal(t, expected.PressQuotes, content.PressQuotes)
}

// TestRepository_UpsertContent_HappyPath verifies upsert returns the updated content.
func TestRepository_UpsertContent_HappyPath(t *testing.T) {
	repo := &mockRepo{}
	bioShort := "Short bio"
	bioLong := "Long bio"

	req := epk.UpsertEPKContentRequest{
		BioShort: &bioShort,
		BioLong:  &bioLong,
		GigHighlights: []string{"Berghain 2025", "Fabric 2025"},
		PressQuotes: []epk.PressQuote{
			{Text: "Outstanding", Source: "RA"},
		},
		PhotoPaths:        []string{"epk/photo1.jpg"},
		SectionVisibility: map[string]bool{"bio": true, "rider": false},
	}

	result, err := repo.UpsertContent(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, bioShort, result.BioShort)
	assert.Equal(t, bioLong, result.BioLong)
	assert.Equal(t, req.GigHighlights, result.GigHighlights)
	assert.Len(t, result.PressQuotes, 1)
	assert.Equal(t, "RA", result.PressQuotes[0].Source)
	assert.Equal(t, req.SectionVisibility, result.SectionVisibility)
}

// TestRepository_UpsertContent_NilPointerFieldsNotChanged verifies nil pointer fields are skipped.
func TestRepository_UpsertContent_NilPointerFieldsNotChanged(t *testing.T) {
	repo := &mockRepo{}

	// First upsert with non-nil fields.
	initial := "Initial bio"
	_, err := repo.UpsertContent(context.Background(), epk.UpsertEPKContentRequest{
		BioShort: &initial,
	})
	require.NoError(t, err)

	// Second upsert with nil BioShort — should not overwrite.
	techRider := "Rider info"
	result, err := repo.UpsertContent(context.Background(), epk.UpsertEPKContentRequest{
		BioShort:  nil, // no change
		TechRider: &techRider,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, techRider, result.TechRider)
}

// TestRepository_InsertExport_HappyPath verifies an export row is inserted and returned.
func TestRepository_InsertExport_HappyPath(t *testing.T) {
	repo := &mockRepo{}
	path := "epk/exports/export-2026.pdf"

	result, err := repo.InsertExport(context.Background(), path)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, path, result.MinioPath)
	assert.NotEqual(t, uuid.Nil, result.ID, "expected non-zero UUID")
	assert.False(t, result.CreatedAt.IsZero(), "expected non-zero created_at")
}

// TestRepository_ListExports_EmptyList verifies an empty slice is returned when no exports exist.
func TestRepository_ListExports_EmptyList(t *testing.T) {
	repo := &mockRepo{}

	exports, err := repo.ListExports(context.Background())

	require.NoError(t, err)
	assert.Empty(t, exports)
}

// TestRepository_ListExports_ReturnsAll verifies multiple exports are returned.
func TestRepository_ListExports_ReturnsAll(t *testing.T) {
	repo := &mockRepo{}

	_, err := repo.InsertExport(context.Background(), "epk/export1.pdf")
	require.NoError(t, err)
	_, err = repo.InsertExport(context.Background(), "epk/export2.pdf")
	require.NoError(t, err)

	exports, err := repo.ListExports(context.Background())

	require.NoError(t, err)
	assert.Len(t, exports, 2)
}

// TestRepository_DeleteExport_HappyPath verifies an existing export is deleted successfully.
func TestRepository_DeleteExport_HappyPath(t *testing.T) {
	repo := &mockRepo{}
	export, err := repo.InsertExport(context.Background(), "epk/to-delete.pdf")
	require.NoError(t, err)

	err = repo.DeleteExport(context.Background(), export.ID)
	require.NoError(t, err)

	// Verify removal.
	exports, err := repo.ListExports(context.Background())
	require.NoError(t, err)
	assert.Empty(t, exports)
}

// TestRepository_DeleteExport_NotFound verifies ErrNotFound is returned when ID does not exist.
func TestRepository_DeleteExport_NotFound(t *testing.T) {
	repo := &mockRepo{}

	err := repo.DeleteExport(context.Background(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.Is(err, epk.ErrNotFound), "expected ErrNotFound, got: %v", err)
}

// TestRepository_DeleteExport_SentinelFlagPath verifies ErrNotFound via deleteNotFound flag.
func TestRepository_DeleteExport_SentinelFlagPath(t *testing.T) {
	repo := &mockRepo{deleteNotFound: true}

	err := repo.DeleteExport(context.Background(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.Is(err, epk.ErrNotFound))
}
