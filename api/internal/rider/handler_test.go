package rider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/rider"
)

// ─── Mock service ───────────────────────────────────────────────────────────

type mockService struct {
	templates    []*rider.RiderTemplate
	attachments  []*rider.RiderAttachment
	pdfResult    *rider.ExportResult
	pdfErr       error

	// What the handler actually passed to the service on the last update.
	lastTplUpdate *rider.UpdateTemplateInput
	lastAttUpdate *rider.UpdateAttachmentInput

	// Error injection per method.
	listTplErr       error
	getTplErr        error
	createTplErr     error
	updateTplErr     error
	deleteTplErr     error
	getAttByGigErr   error
	getAttErr        error
	createAttErr     error
	updateAttErr     error
	deleteAttErr     error
}

func (m *mockService) ListTemplates(_ context.Context) ([]*rider.RiderTemplate, error) {
	return m.templates, m.listTplErr
}

func (m *mockService) GetTemplate(_ context.Context, id uuid.UUID) (*rider.RiderTemplate, error) {
	if m.getTplErr != nil {
		return nil, m.getTplErr
	}
	for _, t := range m.templates {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, rider.ErrNotFound
}

func (m *mockService) CreateTemplate(_ context.Context, in rider.CreateTemplateInput) (*rider.RiderTemplate, error) {
	if m.createTplErr != nil {
		return nil, m.createTplErr
	}
	now := time.Now()
	t := &rider.RiderTemplate{
		ID: uuid.New(), Name: in.Name,
		Technical: in.Technical, Hospitality: in.Hospitality,
		Backline: in.Backline, OtherNotes: in.OtherNotes,
		CreatedAt: now, UpdatedAt: now,
	}
	m.templates = append(m.templates, t)
	return t, nil
}

func (m *mockService) UpdateTemplate(_ context.Context, id uuid.UUID, in rider.UpdateTemplateInput) (*rider.RiderTemplate, error) {
	if m.updateTplErr != nil {
		return nil, m.updateTplErr
	}
	for _, t := range m.templates {
		if t.ID == id {
			if in.Name != nil {
				t.Name = *in.Name
			}
			m.lastTplUpdate = &in
			applyPatchForTest(in.RiderSectionPatch, &t.Technical, &t.Hospitality, &t.Backline, &t.OtherNotes)
			t.UpdatedAt = time.Now()
			return t, nil
		}
	}
	return nil, rider.ErrNotFound
}

func (m *mockService) DeleteTemplate(_ context.Context, id uuid.UUID) error {
	if m.deleteTplErr != nil {
		return m.deleteTplErr
	}
	for i, t := range m.templates {
		if t.ID == id {
			m.templates = append(m.templates[:i], m.templates[i+1:]...)
			return nil
		}
	}
	return rider.ErrNotFound
}

func (m *mockService) GetAttachmentByGig(_ context.Context, gigID uuid.UUID) (*rider.RiderAttachment, error) {
	if m.getAttByGigErr != nil {
		return nil, m.getAttByGigErr
	}
	for _, a := range m.attachments {
		if a.GigID == gigID {
			return a, nil
		}
	}
	return nil, rider.ErrNotFound
}

func (m *mockService) GetAttachment(_ context.Context, id uuid.UUID) (*rider.RiderAttachment, error) {
	if m.getAttErr != nil {
		return nil, m.getAttErr
	}
	for _, a := range m.attachments {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, rider.ErrNotFound
}

func (m *mockService) CreateAttachment(_ context.Context, in rider.CreateAttachmentInput) (*rider.RiderAttachment, error) {
	if m.createAttErr != nil {
		return nil, m.createAttErr
	}
	if in.GigID == uuid.Nil {
		return nil, rider.ErrInvalidInput
	}
	now := time.Now()
	a := &rider.RiderAttachment{
		ID: uuid.New(), GigID: in.GigID, TemplateID: in.TemplateID,
		Technical: in.Technical, Hospitality: in.Hospitality,
		Backline: in.Backline, OtherNotes: in.OtherNotes,
		CreatedAt: now, UpdatedAt: now,
	}
	m.attachments = append(m.attachments, a)
	return a, nil
}

func (m *mockService) UpdateAttachment(_ context.Context, id uuid.UUID, in rider.UpdateAttachmentInput) (*rider.RiderAttachment, error) {
	if m.updateAttErr != nil {
		return nil, m.updateAttErr
	}
	for _, a := range m.attachments {
		if a.ID == id {
			m.lastAttUpdate = &in
			applyPatchForTest(in.RiderSectionPatch, &a.Technical, &a.Hospitality, &a.Backline, &a.OtherNotes)
			a.UpdatedAt = time.Now()
			return a, nil
		}
	}
	return nil, rider.ErrNotFound
}

func (m *mockService) DeleteAttachment(_ context.Context, id uuid.UUID) error {
	if m.deleteAttErr != nil {
		return m.deleteAttErr
	}
	for i, a := range m.attachments {
		if a.ID == id {
			m.attachments = append(m.attachments[:i], m.attachments[i+1:]...)
			return nil
		}
	}
	return rider.ErrNotFound
}

func (m *mockService) GeneratePDF(_ context.Context, attachmentID uuid.UUID) (*rider.ExportResult, error) {
	if m.pdfErr != nil {
		return nil, m.pdfErr
	}
	if m.pdfResult != nil {
		return m.pdfResult, nil
	}
	return &rider.ExportResult{
		ID:          attachmentID,
		DownloadURL: "https://garage.example/dl/" + attachmentID.String() + ".pdf",
		CreatedAt:   time.Now(),
	}, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func newHandlerRoutes(svc *mockService) http.Handler {
	h := rider.NewHandler(svc)
	return h.Routes()
}

func doRequest(t *testing.T, h http.Handler, method, path string, body interface{}) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, rec.Body.Bytes()
}

// ─── Template handler tests ────────────────────────────────────────────────

func TestHandler_ListTemplates_EmptyReturnsEmptyArray(t *testing.T) {
	svc := &mockService{}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, string(body), `"data":[]`)
}

func TestHandler_CreateTemplate_ValidReturns201(t *testing.T) {
	svc := &mockService{}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/templates", map[string]string{
		"name": "Standard club", "technical": "2× CDJ", "hospitality": "water",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, "Standard club", resp.Data["name"])
	assert.NotEmpty(t, resp.Data["id"])
}

func TestHandler_CreateTemplate_EmptyNameReturns422(t *testing.T) {
	svc := &mockService{createTplErr: rider.ErrInvalidInput}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/templates", map[string]string{"name": ""})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandler_GetTemplate_ExistingReturns200(t *testing.T) {
	id := uuid.New()
	svc := &mockService{templates: []*rider.RiderTemplate{{ID: id, Name: "Test"}}}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates/"+id.String(), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, string(body), `"Test"`)
}

func TestHandler_GetTemplate_MissingReturns404(t *testing.T) {
	svc := &mockService{}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates/"+uuid.New().String(), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_GetTemplate_InvalidUUIDReturns400(t *testing.T) {
	svc := &mockService{}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_UpdateTemplate_PartialOnlyUpdatesNamedFields(t *testing.T) {
	id := uuid.New()
	svc := &mockService{templates: []*rider.RiderTemplate{{
		ID: id, Name: "Original",
		Technical: "tech-A", Hospitality: "hosp-A", Backline: "back-A", OtherNotes: "other-A",
	}}}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(), map[string]interface{}{
		"name":        "Renamed",
		"technical":   "tech-B",
		"hospitality": "hosp-A",
		"backline":    "back-A",
		"otherNotes":  "other-A",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, "Renamed", resp.Data["name"])
	assert.Equal(t, "tech-B", resp.Data["technical"])
	assert.Equal(t, "hosp-A", resp.Data["hospitality"])
}

func TestHandler_DeleteTemplate_Returns204(t *testing.T) {
	id := uuid.New()
	svc := &mockService{templates: []*rider.RiderTemplate{{ID: id, Name: "X"}}}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodDelete, "/templates/"+id.String(), nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, svc.templates)
}

func TestHandler_DeleteTemplate_MissingReturns404(t *testing.T) {
	svc := &mockService{}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodDelete, "/templates/"+uuid.New().String(), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ─── Attachment handler tests ──────────────────────────────────────────────

func TestHandler_CreateAttachment_WithTemplateID(t *testing.T) {
	svc := &mockService{}
	tplID := uuid.New()
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/attachments", map[string]interface{}{
		"gigId":      uuid.New().String(),
		"templateId": tplID.String(),
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, tplID.String(), resp.Data["templateId"])
}

func TestHandler_CreateAttachment_WithoutTemplateID_NullInResponse(t *testing.T) {
	svc := &mockService{}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/attachments", map[string]string{
		"gigId": uuid.New().String(),
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Nil(t, resp.Data["templateId"])
}

func TestHandler_CreateAttachment_InvalidGigIDReturns400(t *testing.T) {
	svc := &mockService{}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/attachments", map[string]string{
		"gigId": "not-a-uuid",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateAttachment_DuplicateGigReturns409(t *testing.T) {
	svc := &mockService{createAttErr: rider.ErrConflict}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/attachments", map[string]string{
		"gigId": uuid.New().String(),
	})
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestHandler_GetAttachmentByGig_Returns200(t *testing.T) {
	gigID := uuid.New()
	attID := uuid.New()
	svc := &mockService{attachments: []*rider.RiderAttachment{{
		ID: attID, GigID: gigID, Technical: "tech",
	}}}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/attachments/by-gig/"+gigID.String(), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, string(body), `"tech"`)
}

func TestHandler_GetAttachmentByGig_MissingReturns200WithNull(t *testing.T) {
	svc := &mockService{}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/attachments/by-gig/"+uuid.New().String(), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, string(body), `"data":null`)
}

func TestHandler_DeleteAttachment_Returns204(t *testing.T) {
	id := uuid.New()
	svc := &mockService{attachments: []*rider.RiderAttachment{{ID: id}}}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodDelete, "/attachments/"+id.String(), nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// ─── PDF endpoint ──────────────────────────────────────────────────────────

func TestHandler_GeneratePDF_Returns201WithDownloadURL(t *testing.T) {
	id := uuid.New()
	svc := &mockService{pdfResult: &rider.ExportResult{
		ID: id, DownloadURL: "https://garage/dl", CreatedAt: time.Now(),
	}}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/attachments/"+id.String()+"/pdf", nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, string(body), `"downloadUrl"`)
	assert.Contains(t, string(body), `"https://garage/dl"`)
}

func TestHandler_GeneratePDF_MissingAttachmentReturns404(t *testing.T) {
	svc := &mockService{pdfErr: rider.ErrNotFound}
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/attachments/"+uuid.New().String()+"/pdf", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ─── Envelope shape tests ──────────────────────────────────────────────────

// TestContract_ListUsesDataArrayEnvelope asserts the wire-format shape
// (Pitfall H from go-backend-testing): list responses wrap the slice in
// {"data": [...]}; single-object responses use {"data": {...}}; error
// responses use {"error": "<code>", "message": "<text>"}.
func TestContract_ListUsesDataArrayEnvelope(t *testing.T) {
	svc := &mockService{templates: []*rider.RiderTemplate{{ID: uuid.New(), Name: "A"}}}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	require.Len(t, env.Data, 1)
	assert.Equal(t, "A", env.Data[0]["name"])
}

func TestContract_SingleObjectUsesDataObjectEnvelope(t *testing.T) {
	id := uuid.New()
	svc := &mockService{templates: []*rider.RiderTemplate{{ID: id, Name: "Solo"}}}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates/"+id.String(), nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, "Solo", env.Data["name"])
	assert.Equal(t, id.String(), env.Data["id"])
}

func TestContract_ErrorEnvelopeShape(t *testing.T) {
	svc := &mockService{}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates/"+uuid.New().String(), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, "not_found", env.Error, "a short machine-readable code")
	assert.NotEmpty(t, env.Message, "and a human-readable message")
}

func TestContract_PDFReturnsBareEnvelope(t *testing.T) {
	id := uuid.New()
	svc := &mockService{pdfResult: &rider.ExportResult{
		ID: id, DownloadURL: "https://garage/dl", CreatedAt: time.Now(),
	}}
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/attachments/"+id.String()+"/pdf", nil)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var env struct {
		ID          string `json:"id"`
		DownloadURL string `json:"downloadUrl"`
		CreatedAt   string `json:"createdAt"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, id.String(), env.ID)
	assert.Equal(t, "https://garage/dl", env.DownloadURL)
}

// ─── Partial updates: per-field semantics ───────────────────────────────────
//
// A PUT naming only some sections must leave the others untouched; a section
// sent as "" must clear just that one. These assert both what the handler
// hands the service (nil vs pointer-to-empty) and the resulting record.

func strPtr(v string) *string { return &v }

func applyPatchForTest(p rider.RiderSectionPatch, tech, hosp, back, other *string) {
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

func seededTemplateSvc(id uuid.UUID) *mockService {
	return &mockService{templates: []*rider.RiderTemplate{{
		ID: id, Name: "Original",
		Technical: "tech-A", Hospitality: "hosp-A", Backline: "back-A", OtherNotes: "other-A",
	}}}
}

func seededAttachmentSvc(id uuid.UUID) *mockService {
	return &mockService{attachments: []*rider.RiderAttachment{{
		ID: id, GigID: uuid.New(),
		Technical: "tech-A", Hospitality: "hosp-A", Backline: "back-A", OtherNotes: "other-A",
	}}}
}

func sectionsOf(t *testing.T, body []byte) [4]string {
	t.Helper()
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	return [4]string{
		resp.Data["technical"].(string), resp.Data["hospitality"].(string),
		resp.Data["backline"].(string), resp.Data["otherNotes"].(string),
	}
}

func TestHandler_UpdateTemplate_SingleSection_LeavesOtherSectionsAlone(t *testing.T) {
	id := uuid.New()
	svc := seededTemplateSvc(id)
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(),
		map[string]interface{}{"technical": "tech-B"})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, [4]string{"tech-B", "hosp-A", "back-A", "other-A"}, sectionsOf(t, body))
	in := svc.lastTplUpdate
	require.NotNil(t, in)
	assert.Nil(t, in.Name)
	require.NotNil(t, in.Technical)
	assert.Equal(t, "tech-B", *in.Technical)
	assert.Nil(t, in.Hospitality, "an unsent section must reach the service as nil, not \"\"")
	assert.Nil(t, in.Backline)
	assert.Nil(t, in.OtherNotes)
}

func TestHandler_UpdateTemplate_EmptyString_ClearsOnlyThatSection(t *testing.T) {
	id := uuid.New()
	svc := seededTemplateSvc(id)
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(),
		map[string]interface{}{"hospitality": ""})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, [4]string{"tech-A", "", "back-A", "other-A"}, sectionsOf(t, body))
	require.NotNil(t, svc.lastTplUpdate.Hospitality, "an explicit empty string is a clear, not an omission")
	assert.Equal(t, "", *svc.lastTplUpdate.Hospitality)
	assert.Nil(t, svc.lastTplUpdate.Technical)
}

func TestHandler_UpdateTemplate_NameOnly_LeavesAllSectionsAlone(t *testing.T) {
	id := uuid.New()
	svc := seededTemplateSvc(id)
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(),
		map[string]interface{}{"name": "Renamed"})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, [4]string{"tech-A", "hosp-A", "back-A", "other-A"}, sectionsOf(t, body))
	in := svc.lastTplUpdate
	require.NotNil(t, in)
	require.NotNil(t, in.Name)
	assert.Equal(t, "Renamed", *in.Name)
	assert.Equal(t, rider.RiderSectionPatch{}, in.RiderSectionPatch)
}

func TestHandler_UpdateAttachment_SingleSection_LeavesOtherSectionsAlone(t *testing.T) {
	id := uuid.New()
	svc := seededAttachmentSvc(id)
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/attachments/"+id.String(),
		map[string]interface{}{"backline": "back-B"})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, [4]string{"tech-A", "hosp-A", "back-B", "other-A"}, sectionsOf(t, body))
	in := svc.lastAttUpdate
	require.NotNil(t, in)
	require.NotNil(t, in.Backline)
	assert.Nil(t, in.Technical)
	assert.Nil(t, in.Hospitality)
	assert.Nil(t, in.OtherNotes)
}

func TestHandler_UpdateAttachment_EmptyString_ClearsOnlyThatSection(t *testing.T) {
	id := uuid.New()
	svc := seededAttachmentSvc(id)
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/attachments/"+id.String(),
		map[string]interface{}{"otherNotes": ""})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, [4]string{"tech-A", "hosp-A", "back-A", ""}, sectionsOf(t, body))
	require.NotNil(t, svc.lastAttUpdate.OtherNotes)
}

func TestHandler_UpdateAttachment_EmptyBody_ChangesNothing(t *testing.T) {
	id := uuid.New()
	svc := seededAttachmentSvc(id)
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/attachments/"+id.String(),
		map[string]interface{}{})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, [4]string{"tech-A", "hosp-A", "back-A", "other-A"}, sectionsOf(t, body))
	assert.Equal(t, rider.RiderSectionPatch{}, svc.lastAttUpdate.RiderSectionPatch)
}


// ─── updatedAt token (optimistic concurrency) ───────────────────────────────

func TestHandler_UpdateTemplate_ForwardsUpdatedAtToken(t *testing.T) {
	id := uuid.New()
	svc := seededTemplateSvc(id)
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(),
		map[string]interface{}{"technical": "x", "updatedAt": "2026-10-02T12:00:00.123456Z"})
	require.Equal(t, http.StatusOK, rec.Code)

	want := time.Date(2026, 10, 2, 12, 0, 0, 123456000, time.UTC)
	require.NotNil(t, svc.lastTplUpdate)
	assert.True(t, svc.lastTplUpdate.UpdatedAt.Equal(want),
		"token must reach the service with microsecond precision; got %s", svc.lastTplUpdate.UpdatedAt)
}

func TestHandler_UpdateAttachment_ForwardsUpdatedAtToken(t *testing.T) {
	id := uuid.New()
	svc := seededAttachmentSvc(id)
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/attachments/"+id.String(),
		map[string]interface{}{"technical": "x", "updatedAt": "2026-10-02T12:00:00.123456Z"})
	require.Equal(t, http.StatusOK, rec.Code)

	want := time.Date(2026, 10, 2, 12, 0, 0, 123456000, time.UTC)
	require.NotNil(t, svc.lastAttUpdate)
	assert.True(t, svc.lastAttUpdate.UpdatedAt.Equal(want))
}

func TestHandler_Update_StaleToken_Returns409(t *testing.T) {
	tid, aid := uuid.New(), uuid.New()
	tplSvc := seededTemplateSvc(tid)
	tplSvc.updateTplErr = rider.ErrStaleUpdate
	rec, body := doRequest(t, newHandlerRoutes(tplSvc), http.MethodPut, "/templates/"+tid.String(),
		map[string]interface{}{"technical": "x", "updatedAt": "2026-10-02T12:00:00Z"})
	assert.Equal(t, http.StatusConflict, rec.Code)
	// Not the "attachment already exists" wording of ErrConflict.
	assert.Contains(t, string(body), "changed since it was read")
	assert.NotContains(t, string(body), "already exists")

	attSvc := seededAttachmentSvc(aid)
	attSvc.updateAttErr = rider.ErrStaleUpdate
	rec, _ = doRequest(t, newHandlerRoutes(attSvc), http.MethodPut, "/attachments/"+aid.String(),
		map[string]interface{}{"technical": "x", "updatedAt": "2026-10-02T12:00:00Z"})
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestHandler_Update_MissingToken_Returns422(t *testing.T) {
	tid, aid := uuid.New(), uuid.New()
	tplSvc := seededTemplateSvc(tid)
	tplSvc.updateTplErr = fmt.Errorf("%w: updatedAt is required", rider.ErrInvalidInput)
	rec, _ := doRequest(t, newHandlerRoutes(tplSvc), http.MethodPut, "/templates/"+tid.String(),
		map[string]interface{}{"technical": "x"})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	attSvc := seededAttachmentSvc(aid)
	attSvc.updateAttErr = fmt.Errorf("%w: updatedAt is required", rider.ErrInvalidInput)
	rec, _ = doRequest(t, newHandlerRoutes(attSvc), http.MethodPut, "/attachments/"+aid.String(),
		map[string]interface{}{"technical": "x"})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandler_Update_MalformedToken_Returns400(t *testing.T) {
	id := uuid.New()
	svc := seededTemplateSvc(id)
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(),
		map[string]interface{}{"technical": "x", "updatedAt": "not-a-timestamp"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, svc.lastTplUpdate, "a malformed request must not reach the service")
}


// ─── Request size cap, no leaked internals, safe mux JSON ───────────────────

func TestHandler_BodyTooLarge_Returns413_AndNeverReachesTheService(t *testing.T) {
	huge := strings.Repeat("x", rider.MaxRequestBytes+1)
	tid, aid := uuid.New(), uuid.New()
	tplSvc, attSvc := seededTemplateSvc(tid), seededAttachmentSvc(aid)
	cases := []struct {
		name, method, path string
		h                  http.Handler
		body               map[string]interface{}
	}{
		{"create template", http.MethodPost, "/templates", newHandlerRoutes(tplSvc), map[string]interface{}{"name": "n", "technical": huge}},
		{"update template", http.MethodPut, "/templates/" + tid.String(), newHandlerRoutes(tplSvc), map[string]interface{}{"technical": huge, "updatedAt": "2026-10-02T12:00:00Z"}},
		{"create attachment", http.MethodPost, "/attachments", newHandlerRoutes(attSvc), map[string]interface{}{"gigId": uuid.NewString(), "technical": huge}},
		{"update attachment", http.MethodPut, "/attachments/" + aid.String(), newHandlerRoutes(attSvc), map[string]interface{}{"technical": huge, "updatedAt": "2026-10-02T12:00:00Z"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, body := doRequest(t, tc.h, tc.method, tc.path, tc.body)
			assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, string(body[:min(len(body), 120)]))
		})
	}
	assert.Nil(t, tplSvc.lastTplUpdate)
	assert.Nil(t, attSvc.lastAttUpdate)
	assert.Len(t, tplSvc.templates, 1, "nothing was created")
}

func TestHandler_BodyAtTheLimit_IsStillAccepted(t *testing.T) {
	id := uuid.New()
	svc := seededTemplateSvc(id)
	// Four maximum-size sections of 2-byte characters stay well under the cap.
	section := strings.Repeat("é", rider.MaxSectionChars)
	rec, _ := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(), map[string]interface{}{
		"technical": section, "hospitality": section, "backline": section, "otherNotes": section,
		"updatedAt": "2026-10-02T12:00:00Z",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_InternalErrors_AreNotLeakedToTheClient(t *testing.T) {
	leak := errors.New(`ERROR: relation "rider_templates" does not exist (SQLSTATE 42P01)`)
	check := func(name string, rec *httptest.ResponseRecorder, body []byte) {
		t.Helper()
		assert.Equal(t, http.StatusInternalServerError, rec.Code, name)
		assert.NotContains(t, string(body), "rider_templates", name)
		assert.NotContains(t, string(body), "SQLSTATE", name)
		assert.Contains(t, string(body), "internal error", name)
	}
	id := uuid.New()

	svc := seededTemplateSvc(id)
	svc.listTplErr = leak
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates", nil)
	check("list", rec, body)

	svc = seededTemplateSvc(id)
	svc.createTplErr = leak
	rec, body = doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/templates", map[string]interface{}{"name": "n"})
	check("create", rec, body)

	svc = seededTemplateSvc(id)
	svc.updateTplErr = leak
	rec, body = doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(),
		map[string]interface{}{"name": "n", "updatedAt": "2026-10-02T12:00:00Z"})
	check("update (via translateServiceError)", rec, body)
}

func TestHandler_InvalidInput_StillExplainsItself(t *testing.T) {
	id := uuid.New()
	svc := seededTemplateSvc(id)
	svc.updateTplErr = fmt.Errorf("%w: technical is too long (20001 characters; the limit is 20000)", rider.ErrInvalidInput)
	rec, body := doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(),
		map[string]interface{}{"technical": "x", "updatedAt": "2026-10-02T12:00:00Z"})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, string(body), "too long", "validation messages are for the user and are kept")
}

func TestMux_ErrorBodyIsValidJSON_EvenWhenThePathContainsQuotes(t *testing.T) {
	mux := rider.NewMux(nil, nil)
	// The path decodes to /api/v1/rider/"x followed by a backslash.
	req := httptest.NewRequest(http.MethodGet, `/api/v1/rider/%22x%5C`, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var parsed map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed), "body: %s", rec.Body.String())
	assert.Equal(t, "not_found", parsed["error"])
	assert.Contains(t, parsed["message"], `"x\`)
}


func TestHandler_ErrorBodies_CarryACodeAndAMessage(t *testing.T) {
	id := uuid.New()
	huge := strings.Repeat("x", rider.MaxRequestBytes+1)
	cases := []struct {
		name     string
		status   int
		code     string
		run      func() (*httptest.ResponseRecorder, []byte)
		contains string
	}{
		{"not found", http.StatusNotFound, "not_found", func() (*httptest.ResponseRecorder, []byte) {
			return doRequest(t, newHandlerRoutes(&mockService{}), http.MethodGet, "/templates/"+uuid.NewString(), nil)
		}, "not found"},
		{"bad uuid", http.StatusBadRequest, "bad_request", func() (*httptest.ResponseRecorder, []byte) {
			return doRequest(t, newHandlerRoutes(&mockService{}), http.MethodGet, "/templates/not-a-uuid", nil)
		}, ""},
		{"validation", http.StatusUnprocessableEntity, "validation_failed", func() (*httptest.ResponseRecorder, []byte) {
			svc := seededTemplateSvc(id)
			svc.createTplErr = fmt.Errorf("%w: name is required", rider.ErrInvalidInput)
			return doRequest(t, newHandlerRoutes(svc), http.MethodPost, "/templates", map[string]interface{}{"name": ""})
		}, "name is required"},
		{"stale update", http.StatusConflict, "conflict", func() (*httptest.ResponseRecorder, []byte) {
			svc := seededTemplateSvc(id)
			svc.updateTplErr = rider.ErrStaleUpdate
			return doRequest(t, newHandlerRoutes(svc), http.MethodPut, "/templates/"+id.String(), map[string]interface{}{"name": "n", "updatedAt": "2026-10-02T12:00:00Z"})
		}, "changed since it was read"},
		{"too large", http.StatusRequestEntityTooLarge, "payload_too_large", func() (*httptest.ResponseRecorder, []byte) {
			return doRequest(t, newHandlerRoutes(&mockService{}), http.MethodPost, "/templates", map[string]interface{}{"name": "n", "technical": huge})
		}, "too large"},
		{"internal", http.StatusInternalServerError, "internal_error", func() (*httptest.ResponseRecorder, []byte) {
			svc := &mockService{listTplErr: errors.New("boom")}
			return doRequest(t, newHandlerRoutes(svc), http.MethodGet, "/templates", nil)
		}, "internal error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, body := tc.run()
			require.Equal(t, tc.status, rec.Code)
			var env map[string]string
			require.NoError(t, json.Unmarshal(body, &env), string(body))
			assert.Equal(t, tc.code, env["error"])
			assert.NotEmpty(t, env["message"])
			assert.Contains(t, env["message"], tc.contains)
		})
	}
}
