package gig_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/gig"
	"github.com/klubhub/dj/api/internal/settings"
	"github.com/klubhub/dj/api/internal/tracklist"
	"github.com/shopspring/decimal"
)

type bookingCaptureStorage struct {
	bucket, key, contentType string
	data                     []byte
	err                      error
}

func (s *bookingCaptureStorage) PutObject(_ context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	s.bucket, s.key, s.contentType = bucket, key, contentType
	var err error
	s.data, err = io.ReadAll(r)
	if err != nil {
		return err
	}
	if int64(len(s.data)) != size {
		return errors.New("incorrect PDF storage size")
	}
	return s.err
}

func TestIntegrationBookingPDFStoredDJNameAndStorage(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	ctx := context.Background()
	settingsSvc := settings.NewService(settings.NewRepository(testPool))
	current, err := settingsSvc.GetOrCreate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const djName = "DJ Stored Aurora Fixture"
	stored, err := settingsSvc.Update(ctx, settings.UpdateSettingsRequest{
		DJName: djName, LogoPath: current.LogoPath, DefaultColors: current.DefaultColors,
		DefaultTemplate: current.DefaultTemplate, VisibleFields: current.VisibleFields,
		SocialLinks: current.SocialLinks, BioShort: current.BioShort, BioLong: current.BioLong,
		ContactInfo: current.ContactInfo, InvoicePrefix: current.InvoicePrefix, UpdatedAt: current.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.DJName != djName {
		t.Fatalf("stored name=%q", stored.DJName)
	}
	storage := &bookingCaptureStorage{}
	svc := gig.NewService(gig.NewRepository(testPool), nil, nil, nil, storage)
	created, err := svc.CreateGig(ctx, &gig.GigCreate{Date: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Venue: "Fixture Booking Club", FeeAmount: decimal.NewFromInt(750), FeeCurrency: "EUR", SetLengthMinutes: 90})
	if err != nil {
		t.Fatal(err)
	}
	confirmed := gig.GigStatusConfirmed
	_, err = svc.UpdateGig(ctx, created.ID, &gig.GigUpdate{Status: &confirmed, UpdatedAt: created.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	router := gig.NewHandler(svc, "test-secret", settingsSvc).Routes()
	path := "/" + created.ID.String() + "/pdf?secret=test-secret"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("download=%d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/pdf" || rec.Header().Get("Content-Disposition") != "attachment; filename=booking-confirmation.pdf" {
		t.Fatal("incorrect PDF download headers")
	}
	if !bytes.Equal(rec.Body.Bytes(), storage.data) || storage.bucket != "gig-exports" || storage.key != "2026/"+created.ID.String()+".pdf" || storage.contentType != "application/pdf" {
		t.Fatalf("PDF storage contract changed: %+v", storage)
	}
	tool, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext unavailable; PDF generated/stored but text not inspected")
	}
	pdfPath := filepath.Join(t.TempDir(), "booking.pdf")
	if err := os.WriteFile(pdfPath, rec.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	text, err := exec.Command(tool, pdfPath, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v %s", err, text)
	}
	if !strings.Contains(string(text), djName) || strings.Contains(string(text), "DJ Name") {
		t.Fatalf("PDF text: %s", text)
	}
	t.Logf("real migrated settings -> service -> HTTP download -> pdftotext:\n%s", text)
	// Exercise actual service storage failure, not just a mocked renderer error.
	storage.err = errors.New("fixture storage unavailable")
	failed := httptest.NewRecorder()
	router.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, path, nil))
	if failed.Code != http.StatusInternalServerError || failed.Header().Get("Content-Type") == "application/pdf" {
		t.Fatalf("storage failure returned download: %d", failed.Code)
	}
}

func TestIntegrationGigHTTPConflictRefreshRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	svc := gig.NewService(gig.NewRepository(testPool), nil, nil, tracklist.NewRepository(testPool), nil)
	created, err := svc.CreateGig(context.Background(), &gig.GigCreate{Date: time.Now().UTC(), Venue: "Concurrency fixture", FeeCurrency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	router := gig.NewHandler(svc, "test-secret", nil).Routes()
	path := "/" + created.ID.String()
	put := func(body any, want int) *gig.Gig {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, path, bytes.NewReader(data)))
		if rec.Code != want {
			t.Fatalf("PUT status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
		}
		if want != http.StatusOK {
			return nil
		}
		var envelope struct {
			Data *gig.Gig `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data == nil {
			t.Fatal("missing B4 data envelope")
		}
		return envelope.Data
	}
	first := put(map[string]any{"notes": "other writer", "updated_at": created.UpdatedAt}, http.StatusOK)
	if first.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatal("successful PUT did not return a new timestamp")
	}
	put(map[string]any{"notes": "stale edit", "updated_at": created.UpdatedAt}, http.StatusConflict)
	// Nullable/omitted tokens are not a bypass of the Go time.Time comparison.
	put(map[string]any{"notes": "unknown version", "updated_at": nil}, http.StatusConflict)
	put(map[string]any{"notes": "missing version"}, http.StatusConflict)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET=%d %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data *gig.GigWithRelations `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	fresh := envelope.Data
	if fresh == nil || fresh.Notes != "other writer" || !fresh.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("GET lost authoritative token: %+v", fresh)
	}
	if fresh.LinkedVenues == nil || fresh.LinkedContacts == nil || fresh.LinkedTracklists == nil {
		t.Fatal("C2 empty relation arrays missing")
	}
	retry := put(map[string]any{"notes": "explicit retry", "updated_at": fresh.UpdatedAt}, http.StatusOK)
	if retry.Notes != "explicit retry" || retry.UpdatedAt.Equal(fresh.UpdatedAt) {
		t.Fatalf("retry=%+v", retry)
	}
	t.Logf("stale/null/omitted tokens -> 409; GET refreshed %s; explicit retry -> 200, %s", fresh.UpdatedAt.Format(time.RFC3339Nano), retry.UpdatedAt.Format(time.RFC3339Nano))
}
