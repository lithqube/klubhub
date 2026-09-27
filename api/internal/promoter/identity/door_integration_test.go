package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/promoter/identity"
)

// newEvent inserts a minimal event for the owner's organisation (fixture,
// as the schema owner; the events service is tested in its own package).
func newEvent(t *testing.T, org string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	start := time.Now().Add(24 * time.Hour)
	if _, err := testDB.Owner.Exec(context.Background(), `INSERT INTO events (id, tenant_id, title, slug, starts_at, ends_at, timezone, city)
	  VALUES ($1, $2, 'Night', $3, $4, $5, 'UTC', 'Berlin')`, id, org, "night-"+id.String()[24:], start, start.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	return id
}

func ownerPrincipal(t *testing.T, svc *identity.Service) authz.Principal {
	t.Helper()
	bootstrapOwner(t, svc)
	res, err := svc.Login(context.Background(), identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: res.Cookie})
	p, err := svc.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDoorDeviceAndPinYieldEventScopedSession(t *testing.T) {
	svc, c := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	event := newEvent(t, owner.OrgID)

	device, err := svc.RegisterDoorDevice(ctx, owner, "Door iPhone 1")
	if err != nil {
		t.Fatal(err)
	}
	pin, err := svc.SetDoorPIN(ctx, owner, event, c.t.Add(18*time.Hour))
	if err != nil || len(pin) != 6 {
		t.Fatalf("pin %q %v", pin, err)
	}
	res, err := svc.DoorLogin(ctx, device.Token, event, pin)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: res.Cookie})
	p, err := svc.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "door" || p.EventScope != event.String() || p.DeviceID == "" {
		t.Fatalf("door principal %+v", p)
	}
	if !res.Expires.Equal(c.t.Add(18 * time.Hour)) {
		t.Fatalf("door session must end with the PIN window, got %v", res.Expires)
	}
}

func TestDoorLoginRefusesUnknownDevicesAndLocksOutPINGuessing(t *testing.T) {
	svc, c := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	event := newEvent(t, owner.OrgID)
	device, _ := svc.RegisterDoorDevice(ctx, owner, "Door 1")
	pin, _ := svc.SetDoorPIN(ctx, owner, event, c.t.Add(10*time.Hour))

	fake, _ := auth.NewSessionToken(uuid.MustParse(owner.OrgID))
	if _, err := svc.DoorLogin(ctx, fake.Cookie, event, pin); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("an unregistered device must be refused, got %v", err)
	}
	wrong := "000000"
	if wrong == pin {
		wrong = "111111"
	}
	for range 4 {
		if _, err := svc.DoorLogin(ctx, device.Token, event, wrong); !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("wrong pin: %v", err)
		}
	}
	// The 5th wrong try locks the PIN and already says so.
	var locked *identity.PINLockedError
	if _, err := svc.DoorLogin(ctx, device.Token, event, wrong); !errors.As(err, &locked) || !locked.Until.Equal(c.t.Add(15*time.Minute)) {
		t.Fatalf("the 5th wrong PIN must lock for 15 minutes, got %v", err)
	}
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); !errors.As(err, &locked) || locked.Wait != 15*time.Minute {
		t.Fatalf("PIN guessing must lock the event out, even for the right PIN, got %v", err)
	}
	// An unknown device still learns nothing about the lock.
	if _, err := svc.DoorLogin(ctx, fake.Cookie, event, pin); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("an unregistered device must not see the lock, got %v", err)
	}
	st, err := svc.DoorPINStatus(ctx, owner, event)
	if err != nil || st.Staff == nil || st.Staff.LockedUntil == nil || !st.Staff.LockedUntil.Equal(c.t.Add(15*time.Minute)) {
		t.Fatalf("pin status must report the lock: %+v %v", st, err)
	}
	c.t = c.t.Add(16 * time.Minute)
	if st, _ = svc.DoorPINStatus(ctx, owner, event); st.Staff == nil || st.Staff.LockedUntil != nil {
		t.Fatalf("a lock in the past is not reported: %+v", st)
	}
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); err != nil {
		t.Fatalf("lockout must expire: %v", err)
	}
}

func TestANewStaffPINUnlocksTheDoorAtOnce(t *testing.T) {
	svc, c := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	event := newEvent(t, owner.OrgID)
	device, _ := svc.RegisterDoorDevice(ctx, owner, "Door 1")
	pin, _ := svc.SetDoorPIN(ctx, owner, event, c.t.Add(10*time.Hour))
	wrong := "000000"
	if wrong == pin {
		wrong = "111111"
	}
	for range 5 {
		_, _ = svc.DoorLogin(ctx, device.Token, event, wrong)
	}
	var locked *identity.PINLockedError
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); !errors.As(err, &locked) {
		t.Fatalf("locked: %v", err)
	}
	pin, _ = svc.SetDoorPIN(ctx, owner, event, c.t.Add(10*time.Hour))
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); err != nil {
		t.Fatalf("a manager unlocks the door by generating a new PIN: %v", err)
	}
}

func TestRevokedDeviceAndExpiredPINAreRefused(t *testing.T) {
	svc, c := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	event := newEvent(t, owner.OrgID)
	device, _ := svc.RegisterDoorDevice(ctx, owner, "Door 1")
	pin, _ := svc.SetDoorPIN(ctx, owner, event, c.t.Add(time.Hour))

	c.t = c.t.Add(2 * time.Hour)
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); !errors.Is(err, identity.ErrPINExpired) {
		t.Fatalf("an expired PIN must be refused as expired, got %v", err)
	}
	fake, _ := auth.NewSessionToken(uuid.MustParse(owner.OrgID))
	if _, err := svc.DoorLogin(ctx, fake.Cookie, event, pin); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("an unregistered device must not learn that the PIN expired, got %v", err)
	}
	pin, _ = svc.SetDoorPIN(ctx, owner, event, c.t.Add(time.Hour))
	if err := svc.RevokeDoorDevice(ctx, owner, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("a revoked device must be refused, got %v", err)
	}
}

// The HTTP contract of the door login errors (see handler.go fail).
func TestDoorLoginErrorResponses(t *testing.T) {
	svc, c := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	event := newEvent(t, owner.OrgID)
	device, _ := svc.RegisterDoorDevice(ctx, owner, "Door 1")
	pin, _ := svc.SetDoorPIN(ctx, owner, event, c.t.Add(time.Hour))
	engine, err := authz.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	identity.NewHandler(svc).Mount(mux, engine, authz.NewRegistry(), nil)
	login := func(p string) (*httptest.ResponseRecorder, map[string]string) {
		body, _ := json.Marshal(map[string]string{"device_token": device.Token, "event_id": event.String(), "pin": p})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/door/login", bytes.NewReader(body)))
		var out map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w, out
	}
	wrong := "000000"
	if wrong == pin {
		wrong = "111111"
	}
	if w, out := login(wrong); w.Code != http.StatusUnauthorized || out["error"] != "invalid_credentials" {
		t.Fatalf("wrong PIN: %d %v", w.Code, out)
	}
	for range 3 {
		login(wrong)
	}
	w, out := login(wrong)
	if w.Code != http.StatusTooManyRequests || out["error"] != "pin_locked" || w.Header().Get("Retry-After") != "900" {
		t.Fatalf("locked: %d %v %q", w.Code, out, w.Header().Get("Retry-After"))
	}
	if until, err := time.Parse(time.RFC3339, out["retry_after"]); err != nil || !until.Equal(c.t.Add(15*time.Minute)) {
		t.Fatalf("retry_after %q %v", out["retry_after"], err)
	}
	c.t = c.t.Add(2 * time.Hour)
	if w, out := login(pin); w.Code != http.StatusUnauthorized || out["error"] != "pin_expired" {
		t.Fatalf("expired: %d %v", w.Code, out)
	}
}

func TestManagerPINIsSeparateFromTheStaffPIN(t *testing.T) {
	svc, c := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	event := newEvent(t, owner.OrgID)
	device, _ := svc.RegisterDoorDevice(ctx, owner, "Door 1")

	st, err := svc.DoorPINStatus(ctx, owner, event)
	if err != nil || st.Staff != nil || st.Manager != nil {
		t.Fatalf("no PINs yet: %+v %v", st, err)
	}
	staff, _ := svc.SetDoorPIN(ctx, owner, event, c.t.Add(10*time.Hour))
	manager, err := svc.SetManagerPIN(ctx, owner, event, c.t.Add(8*time.Hour))
	if err != nil || len(manager) != 6 {
		t.Fatalf("manager pin %q %v", manager, err)
	}
	if manager != staff {
		if _, err := svc.DoorLogin(ctx, device.Token, event, manager); !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("the manager PIN must not open a door session, got %v", err)
		}
	}
	if _, err := svc.DoorLogin(ctx, device.Token, event, staff); err != nil {
		t.Fatalf("setting the manager PIN must keep the staff PIN: %v", err)
	}
	st, _ = svc.DoorPINStatus(ctx, owner, event)
	if st.Staff == nil || !st.Staff.ValidUntil.Equal(c.t.Add(10*time.Hour)) || st.Manager == nil || !st.Manager.ValidUntil.Equal(c.t.Add(8*time.Hour)) {
		t.Fatalf("pin status: %+v", st)
	}
	c.t = c.t.Add(9 * time.Hour)
	if st, _ = svc.DoorPINStatus(ctx, owner, event); st.Manager != nil || st.Staff == nil {
		t.Fatalf("an expired PIN is reported as none: %+v", st)
	}
}

func TestDoorDeviceList(t *testing.T) {
	svc, _ := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	a, _ := svc.RegisterDoorDevice(ctx, owner, "Door A")
	b, _ := svc.RegisterDoorDevice(ctx, owner, "Door B")
	if err := svc.RevokeDoorDevice(ctx, owner, a.ID); err != nil {
		t.Fatal(err)
	}
	list, err := svc.DoorDevices(ctx, owner)
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[0].RevokedAt != nil || list[1].RevokedAt == nil {
		t.Fatalf("devices (active first): %+v %v", list, err)
	}
}
