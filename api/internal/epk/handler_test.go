package epk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/epk"
)

// ─── Mock service ─────────────────────────────────────────────────────────────

type mockService struct {
	getContentResult    *epk.EPKContent
	getContentErr       error
	upsertContentResult *epk.EPKContent
	upsertContentErr    error
	upsertContentRequest epk.UpsertEPKContentRequest
	uploadPhotoPath     string
	uploadPhotoErr      error
	deletePhotoErr      error
	uploadStagePath     string
	uploadStagePlotErr  error
	generatePDFResult   *epk.ExportResult
	generatePDFErr      error
	listExportsResult   []epk.EPKExport
	listExportsErr      error
	deleteExportErr     error
}

func (m *mockService) GetContent(_ context.Context) (*epk.EPKContent, error) {
	return m.getContentResult, m.getContentErr
}

func (m *mockService) PhotoURLs(_ context.Context, paths []string) map[string]string {
	urls := map[string]string{}
	for _, p := range paths {
		urls[p] = "https://signed.example/" + p
	}
	return urls
}

func (m *mockService) UpsertContent(_ context.Context, req epk.UpsertEPKContentRequest) (*epk.EPKContent, error) {
	m.upsertContentRequest = req
	return m.upsertContentResult, m.upsertContentErr
}

func (m *mockService) UploadPhoto(_ context.Context, _ []byte, _ string) (string, error) {
	return m.uploadPhotoPath, m.uploadPhotoErr
}

func (m *mockService) DeletePhoto(_ context.Context, _ string) error {
	return m.deletePhotoErr
}

func (m *mockService) UploadStagePlot(_ context.Context, _ []byte, _ string) (string, error) {
	return m.uploadStagePath, m.uploadStagePlotErr
}

func (m *mockService) GeneratePDF(_ context.Context) (*epk.ExportResult, error) {
	return m.generatePDFResult, m.generatePDFErr
}

func (m *mockService) ListExports(_ context.Context) ([]epk.EPKExport, error) {
	return m.listExportsResult, m.listExportsErr
}

func (m *mockService) DeleteExport(_ context.Context, _ uuid.UUID) error {
	return m.deleteExportErr
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newHandlerRoutes(svc *mockService) http.Handler {
	h := epk.NewHandler(svc, nil) // nil = default RA client, as in main.go
	return h.Routes()
}

func makeMultipartPhoto(t *testing.T, fieldName string, data []byte, filename string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestHandler_GetContent_200 verifies GET /content returns 200 with EPKContent JSON.
func TestHandler_GetContent_200(t *testing.T) {
	svc := &mockService{
		getContentResult: &epk.EPKContent{
			ID:       uuid.New(),
			BioShort: "Short bio",
		},
	}
	router := newHandlerRoutes(svc)

	req := httptest.NewRequest(http.MethodGet, "/content", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Contains(t, body, "data")
	data, ok := body["data"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, data["id"])
}

// TestHandler_PutContent_200 verifies PUT /content returns 200 with updated EPKContent.
func TestHandler_PutContent_200(t *testing.T) {
	bioShort := "Updated bio"
	svc := &mockService{
		upsertContentResult: &epk.EPKContent{
			ID:       uuid.New(),
			BioShort: bioShort,
		},
	}
	router := newHandlerRoutes(svc)

	body, _ := json.Marshal(map[string]string{"bio_short": bioShort})
	req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestHandler_PostPhoto_201 verifies POST /photos returns 201 with path.
func TestHandler_PostPhoto_201(t *testing.T) {
	svc := &mockService{uploadPhotoPath: "epk/photos/test.jpg"}
	router := newHandlerRoutes(svc)

	data := makeJPEG()
	buf, contentType := makeMultipartPhoto(t, "photo", data, "test.jpg")
	req := httptest.NewRequest(http.MethodPost, "/photos", buf)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "epk/photos/test.jpg", resp["path"])
}

// TestHandler_PostPhoto_422_Limit verifies POST /photos returns 422 at photo limit.
func TestHandler_PostPhoto_422_Limit(t *testing.T) {
	svc := &mockService{uploadPhotoErr: epk.ErrPhotoLimitExceeded}
	router := newHandlerRoutes(svc)

	data := makeJPEG()
	buf, contentType := makeMultipartPhoto(t, "photo", data, "test.jpg")
	req := httptest.NewRequest(http.MethodPost, "/photos", buf)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

// TestHandler_PostPhoto_415_BadMIME verifies POST /photos returns 415 for wrong MIME.
func TestHandler_PostPhoto_415_BadMIME(t *testing.T) {
	svc := &mockService{uploadPhotoErr: epk.ErrInvalidMIME}
	router := newHandlerRoutes(svc)

	data := makeJPEG()
	buf, contentType := makeMultipartPhoto(t, "photo", data, "test.pdf")
	req := httptest.NewRequest(http.MethodPost, "/photos", buf)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rr.Code)
}

// TestHandler_DeletePhoto_204 verifies DELETE /photos/{path} returns 204.
func TestHandler_DeletePhoto_204(t *testing.T) {
	svc := &mockService{}
	r := chi.NewRouter()
	h := epk.NewHandler(svc, nil) // nil = default RA client, as in main.go
	r.Mount("/", h.Routes())

	req := httptest.NewRequest(http.MethodDelete, "/photos/epk%2Fphotos%2Ftest.jpg", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

// TestHandler_PostExport_201 verifies POST /export returns 201 with download URL.
func TestHandler_PostExport_201(t *testing.T) {
	exportID := uuid.New()
	svc := &mockService{
		generatePDFResult: &epk.ExportResult{
			ID:          exportID,
			DownloadURL: "https://minio/export.pdf",
			CreatedAt:   time.Now(),
		},
	}
	router := newHandlerRoutes(svc)

	req := httptest.NewRequest(http.MethodPost, "/export", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, exportID.String(), resp["id"])
	assert.Equal(t, "https://minio/export.pdf", resp["downloadUrl"])
}

// TestHandler_GetExports_200 verifies GET /exports returns 200 with list.
func TestHandler_GetExports_200(t *testing.T) {
	svc := &mockService{
		listExportsResult: []epk.EPKExport{
			{ID: uuid.New(), MinioPath: "epk/exports/1.pdf", CreatedAt: time.Now()},
		},
	}
	router := newHandlerRoutes(svc)

	req := httptest.NewRequest(http.MethodGet, "/exports", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Contains(t, body, "data")
	resp, ok := body["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, resp, 1)
}

// TestHandler_DeleteExport_204 verifies DELETE /exports/{id} returns 204.
func TestHandler_DeleteExport_204(t *testing.T) {
	exportID := uuid.New()
	// Provide the export in list so handler can find it
	svc := &mockService{
		listExportsResult: []epk.EPKExport{
			{ID: exportID, MinioPath: "epk/exports/1.pdf", CreatedAt: time.Now()},
		},
	}
	r := chi.NewRouter()
	h := epk.NewHandler(svc, nil) // nil = default RA client, as in main.go
	r.Mount("/", h.Routes())

	req := httptest.NewRequest(http.MethodDelete, "/exports/"+exportID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}
