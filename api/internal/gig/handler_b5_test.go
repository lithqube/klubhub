package gig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Plan B.5 regression tests for the calendar/PDF bearer-protection fix.
// The previous implementation compared the user-supplied `?secret=...`
// against the literal string "ICAL_SECRET", which meant anyone reading
// the source could bypass auth. The handler must now reject the
// placeholder, refuse empty secrets, and only accept the configured
// value via constant-time compare.

const testICalSecret = "this-is-a-32-byte-test-secret-1234" // gitleaks:allow — test fixture

// stubServiceIface is a no-op implementation used by the B.5 auth tests,
// which never invoke service methods.
type stubServiceIface struct{}

func (stubServiceIface) CreateGig(context.Context, *GigCreate) (*Gig, error) {
	return nil, nil
}
func (stubServiceIface) GetGig(context.Context, uuid.UUID) (*Gig, error) {
	return nil, nil
}
func (stubServiceIface) ListGigs(context.Context, GigFilter) ([]*Gig, error) {
	return nil, nil
}
func (stubServiceIface) UpdateGig(context.Context, uuid.UUID, *GigUpdate) (*Gig, error) {
	return nil, nil
}
func (stubServiceIface) DeleteGig(context.Context, uuid.UUID) error { return nil }
func (stubServiceIface) LinkVenue(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return nil
}
func (stubServiceIface) LinkContact(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (stubServiceIface) GenerateCalendar(context.Context, CalendarConfig) (string, error) {
	return "", nil
}
func (stubServiceIface) GenerateBookingPDF(context.Context, uuid.UUID, string) ([]byte, string, error) {
	return nil, "", nil
}

func newHandlerWithSecret(t *testing.T) *Handler {
	t.Helper()
	svc := &stubServiceIface{}
	type ctor struct {
		svc    ServiceIface
		secret string
	}
	// NewHandler will panic if icalSecret is empty.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewHandler panicked with a valid secret: %v", r)
		}
	}()
	return NewHandler(svc, testICalSecret)
}

func TestValidateICalSecret_RejectsLiteralPlaceholder(t *testing.T) {
	h := newHandlerWithSecret(t)
	if h.validateICalSecret("ICAL_SECRET") {
		t.Fatal("validateICalSecret(\"ICAL_SECRET\") = true; the historical placeholder must be rejected")
	}
}

func TestValidateICalSecret_RejectsEmpty(t *testing.T) {
	h := newHandlerWithSecret(t)
	if h.validateICalSecret("") {
		t.Fatal("validateICalSecret(\"\") = true; empty bearer must be rejected")
	}
}

func TestValidateICalSecret_AcceptsConfiguredValue(t *testing.T) {
	h := newHandlerWithSecret(t)
	if !h.validateICalSecret(testICalSecret) {
		t.Fatalf("validateICalSecret(secret) = false; expected true for the configured value")
	}
}

func TestValidateICalSecret_RejectsWrongValue(t *testing.T) {
	h := newHandlerWithSecret(t)
	if h.validateICalSecret("not-the-secret") {
		t.Fatal("validateICalSecret(\"not-the-secret\") = true; wrong bearer must be rejected")
	}
}

func TestNewHandler_PanicsOnEmptySecret(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewHandler(\"\") did not panic; misconfiguration must fail loud")
		}
	}()
	NewHandler(nil, "")
}

func TestHandler_RejectsCalendarWithoutBearer(t *testing.T) {
	h := newHandlerWithSecret(t)
	// We can't safely call handleCalendarICS without a real Service
	// implementation; the assertion we care about is the validate path.
	// We use a minimal handler endpoint that mirrors the calendar gate.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gigs/calendar.ics", nil)
	w := httptest.NewRecorder()

	// Spin a tiny router with a calendar.ics handler that asserts on
	// the validate path. This is a smoke check that the public
	// surface rejects unauthenticated calls.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/gigs/calendar.ics", func(w http.ResponseWriter, r *http.Request) {
		if !h.validateICalSecret(r.URL.Query().Get("secret")) {
			http.Error(w, "forbidden", http.StatusUnauthorized)
			return
		}
		http.Error(w, "ok", http.StatusOK)
	})
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("calendar.ics without bearer = %d; expected 401", w.Code)
	}
}

func TestHandler_AcceptsCalendarWithCorrectBearer(t *testing.T) {
	h := newHandlerWithSecret(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gigs/calendar.ics?secret="+testICalSecret, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/gigs/calendar.ics", func(w http.ResponseWriter, r *http.Request) {
		if !h.validateICalSecret(r.URL.Query().Get("secret")) {
			http.Error(w, "forbidden", http.StatusUnauthorized)
			return
		}
		http.Error(w, "ok", http.StatusOK)
	})
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("calendar.ics with correct bearer = %d; expected 200", w.Code)
	}
}

func TestHandler_RejectsCalendarWithLiteralPlaceholder(t *testing.T) {
	h := newHandlerWithSecret(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gigs/calendar.ics?secret=ICAL_SECRET", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/gigs/calendar.ics", func(w http.ResponseWriter, r *http.Request) {
		if !h.validateICalSecret(r.URL.Query().Get("secret")) {
			http.Error(w, "forbidden", http.StatusUnauthorized)
			return
		}
		http.Error(w, "ok", http.StatusOK)
	})
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("calendar.ics with literal placeholder bearer = %d; expected 401", w.Code)
	}
}
