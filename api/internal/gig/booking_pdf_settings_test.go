package gig

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/settings"
)

type bookingSettingsFixture struct {
	value *settings.UserSettings
	err   error
	calls int
}

func (s *bookingSettingsFixture) GetOrCreate(context.Context) (*settings.UserSettings, error) {
	s.calls++
	return s.value, s.err
}

type bookingRendererService struct {
	stubServiceIface
	gig    *Gig
	called bool
	err    error
}

func (s *bookingRendererService) GenerateBookingPDF(_ context.Context, id uuid.UUID, name string) ([]byte, string, error) {
	s.called = true
	if id != s.gig.ID {
		return nil, "", ErrNotFound
	}
	if s.err != nil {
		return nil, "", s.err
	}
	pdf, err := GenerateBookingPDF(s.gig, name)
	return pdf, "fixture.pdf", err
}

func bookingFixtureService() *bookingRendererService {
	return &bookingRendererService{gig: &Gig{ID: uuid.New(), Date: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Venue: "Fixture Club", Status: GigStatusConfirmed, FeeCurrency: "EUR"}}
}

func TestBookingPDFUsesSettingsDJNameInRenderedText(t *testing.T) {
	svc := bookingFixtureService()
	settingsFixture := &bookingSettingsFixture{value: &settings.UserSettings{DJName: "DJ Aurora Fixture"}}
	h := NewHandler(svc, testICalSecret, settingsFixture)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+svc.gig.ID.String()+"/pdf?secret="+testICalSecret, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/pdf" || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("PDF download headers missing")
	}
	if settingsFixture.calls != 1 {
		t.Fatalf("settings reads=%d", settingsFixture.calls)
	}
	text := extractBookingPDFText(t, rec.Body.Bytes())
	if !strings.Contains(text, settingsFixture.value.DJName) || strings.Contains(text, "DJ Name") {
		t.Fatalf("unexpected PDF text: %s", text)
	}
	t.Logf("downloaded PDF extracted text:\n%s", text)
}

func extractBookingPDFText(t *testing.T, pdf []byte) string {
	t.Helper()
	tool, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext unavailable; real PDF text inspection needs poppler")
	}
	path := filepath.Join(t.TempDir(), "booking.pdf")
	if err := os.WriteFile(path, pdf, 0600); err != nil {
		t.Fatal(err)
	}
	text, err := exec.Command(tool, path, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("PDF text extraction: %v: %s", err, text)
	}
	return string(text)
}

func TestBookingPDFFailsWithoutUsableSettings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture *bookingSettingsFixture
		status  int
	}{
		{"settings error", &bookingSettingsFixture{err: errors.New("settings unavailable")}, http.StatusInternalServerError},
		{"nil settings", &bookingSettingsFixture{}, http.StatusInternalServerError},
		{"empty DJ name", &bookingSettingsFixture{value: &settings.UserSettings{}}, http.StatusUnprocessableEntity},
		{"blank DJ name", &bookingSettingsFixture{value: &settings.UserSettings{DJName: " \t "}}, http.StatusUnprocessableEntity},
		{"missing dependency", nil, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := bookingFixtureService()
			// Avoid a typed nil interface for the missing-dependency case.
			var repo settingsRepoIface
			if tc.fixture != nil {
				repo = tc.fixture
			}
			rec := httptest.NewRecorder()
			NewHandler(svc, testICalSecret, repo).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+svc.gig.ID.String()+"/pdf?secret="+testICalSecret, nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if svc.called || bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF")) || strings.Contains(rec.Body.String(), "DJ Name") {
				t.Fatal("settings failure generated a fake PDF")
			}
		})
	}
}

func TestBookingPDFGenerationFailureDoesNotReturnPDF(t *testing.T) {
	svc := bookingFixtureService()
	svc.err = errors.New("storage unavailable")
	rec := httptest.NewRecorder()
	NewHandler(svc, testICalSecret, &bookingSettingsFixture{value: &settings.UserSettings{DJName: "Fixture DJ"}}).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+svc.gig.ID.String()+"/pdf?secret="+testICalSecret, nil))
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Content-Type") == "application/pdf" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBookingPDFRejectsUnauthorizedBeforeSettingsRead(t *testing.T) {
	fixture := &bookingSettingsFixture{}
	rec := httptest.NewRecorder()
	NewHandler(bookingFixtureService(), testICalSecret, fixture).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+uuid.NewString()+"/pdf", nil))
	if rec.Code != http.StatusForbidden || fixture.calls != 0 {
		t.Fatalf("status=%d settings reads=%d", rec.Code, fixture.calls)
	}
}
