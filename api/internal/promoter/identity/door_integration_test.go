package identity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/promoter/identity"
)

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
	event := uuid.Must(uuid.NewV7())

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
	event := uuid.Must(uuid.NewV7())
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
	for range 5 {
		if _, err := svc.DoorLogin(ctx, device.Token, event, wrong); !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("wrong pin: %v", err)
		}
	}
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("PIN guessing must lock the event out, got %v", err)
	}
	c.t = c.t.Add(16 * time.Minute)
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); err != nil {
		t.Fatalf("lockout must expire: %v", err)
	}
}

func TestRevokedDeviceAndExpiredPINAreRefused(t *testing.T) {
	svc, c := fresh(t)
	owner := ownerPrincipal(t, svc)
	ctx := context.Background()
	event := uuid.Must(uuid.NewV7())
	device, _ := svc.RegisterDoorDevice(ctx, owner, "Door 1")
	pin, _ := svc.SetDoorPIN(ctx, owner, event, c.t.Add(time.Hour))

	c.t = c.t.Add(2 * time.Hour)
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("an expired PIN must be refused, got %v", err)
	}
	pin, _ = svc.SetDoorPIN(ctx, owner, event, c.t.Add(time.Hour))
	if err := svc.RevokeDoorDevice(ctx, owner, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DoorLogin(ctx, device.Token, event, pin); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("a revoked device must be refused, got %v", err)
	}
}
