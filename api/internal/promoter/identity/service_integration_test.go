package identity_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/identity"
)

var testDB *pgtest.DB

func TestMain(m *testing.M) {
	flag.Parse()
	db, err := pgtest.StartPromoter()
	if err != nil {
		panic(err)
	}
	testDB = db
	code := m.Run()
	db.Close()
	os.Exit(code)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// fresh gives each test its own database state by truncating as the owner.
func fresh(t *testing.T) (*identity.Service, *clock) {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	if _, err := testDB.Owner.Exec(context.Background(), "TRUNCATE organizations CASCADE"); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("test-kek", raw)
	c := &clock{t: time.Date(2026, 9, 25, 20, 0, 0, 0, time.UTC)}
	svc := identity.NewService(tenantdb.New(testDB.App), envelope.NewKeyring(kek), identity.Options{
		Argon2: auth.Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
		Now:    c.now,
	})
	return svc, c
}

const ownerPass = "a long owner passphrase"

func bootstrapOwner(t *testing.T, svc *identity.Service) {
	t.Helper()
	tok, err := svc.Bootstrap(context.Background(), identity.BootstrapInput{
		OrgName: "Nachtwerk Collective", Slug: "nachtwerk", Timezone: "Europe/Berlin", Currency: "EUR",
		OwnerEmail: "Owner@Nachtwerk.example", OwnerName: "Mara Owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteSetup(context.Background(), tok, ownerPass); err != nil {
		t.Fatal(err)
	}
}

func authenticate(svc *identity.Service, cookie string) (*http.Request, error) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: cookie})
	_, err := svc.Authenticate(r)
	return r, err
}

func TestBootstrapSetupLoginAndSession(t *testing.T) {
	svc, _ := fresh(t)
	bootstrapOwner(t, svc)

	res, err := svc.Login(context.Background(), identity.LoginInput{Email: "owner@nachtwerk.example ", Password: ownerPass})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: res.Cookie})
	p, err := svc.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "owner" || len(p.AMR) != 1 || p.AMR[0] != "pwd" {
		t.Fatalf("unexpected principal %+v", p)
	}
	if res.TOTPEnrolled {
		t.Fatal("owner has not enrolled TOTP yet")
	}

	if _, err := svc.Bootstrap(context.Background(), identity.BootstrapInput{OrgName: "Second", Slug: "second", OwnerEmail: "x@y.z", OwnerName: "X"}); !errors.Is(err, identity.ErrAlreadyBootstrapped) {
		t.Fatalf("a self-hosted instance has one organisation, got %v", err)
	}
}

func TestSetupLinkIsSingleUseAndExpires(t *testing.T) {
	svc, c := fresh(t)
	tok, err := svc.Bootstrap(context.Background(), identity.BootstrapInput{OrgName: "N", Slug: "nw", OwnerEmail: "o@n.example", OwnerName: "O"})
	if err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(25 * time.Hour)
	if err := svc.CompleteSetup(context.Background(), tok, ownerPass); !errors.Is(err, identity.ErrInvalidLink) {
		t.Fatalf("expired link must be refused, got %v", err)
	}
	svc2, _ := fresh(t)
	tok, _ = svc2.Bootstrap(context.Background(), identity.BootstrapInput{OrgName: "N", Slug: "nw", OwnerEmail: "o@n.example", OwnerName: "O"})
	if err := svc2.CompleteSetup(context.Background(), tok, ownerPass); err != nil {
		t.Fatal(err)
	}
	if err := svc2.CompleteSetup(context.Background(), tok, "another long passphrase"); !errors.Is(err, identity.ErrInvalidLink) {
		t.Fatalf("a used link must be refused, got %v", err)
	}
	if err := svc2.CompleteSetup(context.Background(), "garbage", ownerPass); !errors.Is(err, identity.ErrInvalidLink) {
		t.Fatalf("garbage must be refused, got %v", err)
	}
}

func TestWeakPasswordRejectedAtSetup(t *testing.T) {
	svc, _ := fresh(t)
	tok, _ := svc.Bootstrap(context.Background(), identity.BootstrapInput{OrgName: "N", Slug: "nw", OwnerEmail: "o@n.example", OwnerName: "O"})
	if err := svc.CompleteSetup(context.Background(), tok, "short"); !errors.Is(err, identity.ErrWeakPassword) {
		t.Fatalf("weak password must be refused, got %v", err)
	}
}

func TestLoginFailuresAreGenericAndLockOut(t *testing.T) {
	svc, c := fresh(t)
	bootstrapOwner(t, svc)
	ctx := context.Background()
	if _, err := svc.Login(ctx, identity.LoginInput{Email: "nobody@nachtwerk.example", Password: ownerPass}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("unknown email: %v", err)
	}
	for range 5 {
		if _, err := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: "wrong but long enough"}); !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("wrong password: %v", err)
		}
	}
	if _, err := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("a locked account must refuse even the right password, got %v", err)
	}
	c.t = c.t.Add(16 * time.Minute)
	if _, err := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass}); err != nil {
		t.Fatalf("lockout must expire: %v", err)
	}
}

func TestTOTPEnrolmentUpgradesSessionAndIsThenRequired(t *testing.T) {
	svc, c := fresh(t)
	bootstrapOwner(t, svc)
	ctx := context.Background()
	res, _ := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass})
	r, _ := authenticate(svc, res.Cookie)
	p, _ := svc.Authenticate(r)

	secret, uri, err := svc.EnrollTOTP(ctx, p)
	if err != nil || uri == "" {
		t.Fatalf("enrol: %v", err)
	}
	code := identity.TOTPCodeForTest(secret, c.t)
	if err := svc.ConfirmTOTP(ctx, p, res.Cookie, code); err != nil {
		t.Fatal(err)
	}
	r, _ = authenticate(svc, res.Cookie)
	p, _ = svc.Authenticate(r)
	if len(p.AMR) != 2 || p.AMR[1] != "otp" {
		t.Fatalf("confirming TOTP must upgrade the session amr, got %v", p.AMR)
	}

	if _, err := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass}); !errors.Is(err, identity.ErrTOTPRequired) {
		t.Fatalf("TOTP must now be required, got %v", err)
	}
	c.t = c.t.Add(31 * time.Second)
	code = identity.TOTPCodeForTest(secret, c.t)
	res2, err := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass, TOTP: code})
	if err != nil || !res2.TOTPEnrolled {
		t.Fatalf("login with TOTP: %v", err)
	}
	if _, err := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass, TOTP: code}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("a TOTP code must not be replayed, got %v", err)
	}
}

func TestLogoutRevokesAndExpiredSessionsFail(t *testing.T) {
	svc, c := fresh(t)
	bootstrapOwner(t, svc)
	ctx := context.Background()
	res, _ := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass})
	if err := svc.Logout(ctx, res.Cookie); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticate(svc, res.Cookie); err == nil {
		t.Fatal("a revoked session must not authenticate")
	}
	res, _ = svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass})
	c.t = c.t.Add(13 * time.Hour) // idle timeout is 12h
	if _, err := authenticate(svc, res.Cookie); err == nil {
		t.Fatal("an idle session must expire")
	}
}

func TestInviteAcceptAndRole(t *testing.T) {
	svc, _ := fresh(t)
	bootstrapOwner(t, svc)
	ctx := context.Background()
	res, _ := svc.Login(ctx, identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass})
	r, _ := authenticate(svc, res.Cookie)
	owner, _ := svc.Authenticate(r)

	tok, err := svc.Invite(ctx, owner, "Booker@Nachtwerk.example", "booker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, owner, "x@nachtwerk.example", "superuser"); !errors.Is(err, identity.ErrInvalidRole) {
		t.Fatalf("unknown role must be refused, got %v", err)
	}
	if err := svc.AcceptInvite(ctx, tok, "Bo Booker", "the booker's own passphrase"); err != nil {
		t.Fatal(err)
	}
	res, err = svc.Login(ctx, identity.LoginInput{Email: "booker@nachtwerk.example", Password: "the booker's own passphrase"})
	if err != nil {
		t.Fatal(err)
	}
	r, _ = authenticate(svc, res.Cookie)
	p, _ := svc.Authenticate(r)
	if len(p.Roles) != 1 || p.Roles[0] != "booker" {
		t.Fatalf("roles %v", p.Roles)
	}
}

func TestPersonalDataIsEncryptedAtRest(t *testing.T) {
	svc, _ := fresh(t)
	bootstrapOwner(t, svc)
	var email, name, hash []byte
	err := testDB.Owner.QueryRow(context.Background(),
		`SELECT email_enc, display_name_enc, password_hash_enc FROM users`).Scan(&email, &name, &hash)
	if err != nil {
		t.Fatal(err)
	}
	for label, v := range map[string][]byte{"email": email, "name": name, "hash": hash} {
		for _, plain := range [][]byte{[]byte("nachtwerk"), []byte("Mara"), []byte("argon2id")} {
			if bytes.Contains(bytes.ToLower(v), bytes.ToLower(plain)) {
				t.Fatalf("%s column leaks %q", label, plain)
			}
		}
	}
}

func TestOrgIsReadableOnlyForItsTenant(t *testing.T) {
	svc, _ := fresh(t)
	bootstrapOwner(t, svc)
	res, _ := svc.Login(context.Background(), identity.LoginInput{Email: "owner@nachtwerk.example", Password: ownerPass})
	r, _ := authenticate(svc, res.Cookie)
	p, _ := svc.Authenticate(r)
	var ctx = context.Background()
	org, err := svc.Org(ctx, p)
	if err != nil || org.Slug != "nachtwerk" || org.Timezone != "Europe/Berlin" {
		t.Fatalf("org: %+v %v", org, err)
	}
}
