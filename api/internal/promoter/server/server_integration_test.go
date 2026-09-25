package server_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/identity"
	"github.com/klubhub/dj/api/internal/promoter/server"
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

const origin = "https://promoter.example"

type client struct {
	t      *testing.T
	h      http.Handler
	cookie string
}

func (c *client) do(method, path string, body any, csrf bool) *httptest.ResponseRecorder {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	if csrf {
		req.Header.Set("Origin", origin)
		req.Header.Set(auth.CSRFHeader, "1")
	}
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.cookie})
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == auth.SessionCookie {
			c.cookie = ck.Value
		}
	}
	return rec
}

func stack(t *testing.T) (http.Handler, *identity.Service, func() []error) {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	if _, err := testDB.Owner.Exec(context.Background(), "TRUNCATE organizations CASCADE"); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	db := tenantdb.New(testDB.App)
	svc := identity.NewService(db, envelope.NewKeyring(kek), identity.Options{
		Argon2: auth.Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
	})
	engine, err := authz.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mux, reg := server.New(server.Deps{
		Log: zerolog.Nop(), DB: db, Authz: engine, Authn: svc,
		Identity: identity.NewHandler(svc), Origins: []string{origin},
		Events: event.NewHandler(event.NewService(db, envelope.NewKeyring(kek), nil)),
	})
	return mux, svc, func() []error { return authz.VerifyCoverage(context.Background(), mux, reg) }
}

func TestEveryRouteHasAPolicyDecision(t *testing.T) {
	_, _, coverage := stack(t)
	for _, err := range coverage() {
		t.Error(err)
	}
}

func TestEndToEndSelfHostedFlow(t *testing.T) {
	h, svc, _ := stack(t)
	ctx := context.Background()
	setup, err := svc.Bootstrap(ctx, identity.BootstrapInput{
		OrgName: "Nachtwerk Collective", Slug: "nachtwerk", Timezone: "Europe/Berlin",
		OwnerEmail: "owner@nachtwerk.example", OwnerName: "Mara",
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := &client{t: t, h: h}

	if rec := owner.do(http.MethodPost, "/api/v1/auth/setup", map[string]string{"token": setup, "password": "the owner passphrase"}, false); rec.Code != http.StatusForbidden {
		t.Fatalf("state change without CSRF proof must be refused, got %d", rec.Code)
	}
	if rec := owner.do(http.MethodPost, "/api/v1/auth/setup", map[string]string{"token": setup, "password": "the owner passphrase"}, true); rec.Code != http.StatusNoContent {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	if rec := owner.do(http.MethodGet, "/api/v1/org", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("org without a session must be 401, got %d", rec.Code)
	}

	rec := owner.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": "owner@nachtwerk.example", "password": "the owner passphrase"}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	setCookie := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"__Host-kh_session=", "HttpOnly", "Secure", "SameSite=Lax", "Path=/"} {
		if !strings.Contains(setCookie, want) {
			t.Fatalf("session cookie missing %q: %s", want, setCookie)
		}
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("security headers missing on API responses")
	}

	rec = owner.do(http.MethodGet, "/api/v1/org", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"slug":"nachtwerk"`) {
		t.Fatalf("org: %d %s", rec.Code, rec.Body)
	}

	// Managing members needs a second factor the owner does not have yet.
	rec = owner.do(http.MethodPost, "/api/v1/members/invites", map[string]string{"email": "booker@nachtwerk.example", "role": "booker"}, true)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "mfa_required") {
		t.Fatalf("invite before MFA must be 403 mfa_required, got %d %s", rec.Code, rec.Body)
	}

	rec = owner.do(http.MethodPost, "/api/v1/auth/totp/enroll", nil, true)
	var enrol struct {
		URI string `json:"otpauth_uri"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &enrol)
	u, err := url.Parse(enrol.URI)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("enrol: %d %s", rec.Code, rec.Body)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(u.Query().Get("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if rec := owner.do(http.MethodPost, "/api/v1/auth/totp/confirm", map[string]string{"code": auth.TOTPCode(secret, time.Now())}, true); rec.Code != http.StatusNoContent {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body)
	}
	rec = owner.do(http.MethodPost, "/api/v1/members/invites", map[string]string{"email": "booker@nachtwerk.example", "role": "booker"}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite after MFA: %d %s", rec.Code, rec.Body)
	}

	// Door: register a device, set an event PIN, log in on the device.
	rec = owner.do(http.MethodPost, "/api/v1/door/devices", map[string]string{"label": "Door iPhone"}, true)
	var device struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &device)
	var orgID string
	if err := testDB.Owner.QueryRow(ctx, `SELECT id::text FROM organizations`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	event := uuid.Must(uuid.NewV7())
	start := time.Now().Add(time.Hour)
	if _, err := testDB.Owner.Exec(ctx, `INSERT INTO events (id, tenant_id, title, slug, starts_at, ends_at, timezone, city)
	  VALUES ($1, $2, 'Night', 'night', $3, $4, 'UTC', 'Berlin')`, event, orgID, start, start.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rec = owner.do(http.MethodPost, "/api/v1/door/events/"+event.String()+"/pin", map[string]any{"valid_until": time.Now().Add(8 * time.Hour)}, true)
	var pin struct {
		PIN string `json:"pin"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &pin)
	if rec.Code != http.StatusCreated || len(pin.PIN) != 6 {
		t.Fatalf("pin: %d %s", rec.Code, rec.Body)
	}
	door := &client{t: t, h: h}
	if rec := door.do(http.MethodPost, "/api/v1/door/login", map[string]any{"device_token": device.Token, "event_id": event, "pin": pin.PIN}, true); rec.Code != http.StatusNoContent {
		t.Fatalf("door login: %d %s", rec.Code, rec.Body)
	}
	rec = door.do(http.MethodGet, "/api/v1/org", nil, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("door staff must not read org settings, got %d", rec.Code)
	}
	rec = door.do(http.MethodGet, "/api/v1/auth/me", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), event.String()) {
		t.Fatalf("door me: %d %s", rec.Code, rec.Body)
	}

	// Side effects: the refusal is audited, and domain changes reached the outbox.
	var denials int
	if err := testDB.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE decision = 'deny' AND reason = 'mfa_required' AND action = 'member.manage'`).Scan(&denials); err != nil || denials != 1 {
		t.Fatalf("the MFA refusal must be in the audit log once, got %d (%v)", denials, err)
	}
	var subjects []string
	rows, _ := testDB.Owner.Query(ctx, `SELECT subject FROM outbox ORDER BY created_at`)
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		subjects = append(subjects, s[strings.LastIndex(s[:strings.LastIndex(s, ".")], ".")+1:])
	}
	rows.Close()
	if strings.Join(subjects, ",") != "member.invited,door.session_started" {
		t.Fatalf("outbox events: %v", subjects)
	}

	if rec := owner.do(http.MethodPost, "/api/v1/auth/logout", nil, true); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := owner.do(http.MethodGet, "/api/v1/org", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout the session must be gone, got %d", rec.Code)
	}
	if rec := owner.do(http.MethodGet, "/api/v1/does-not-exist", nil, false); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown API route: %d", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	h, _, _ := stack(t)
	c := &client{t: t, h: h}
	rec := c.do(http.MethodGet, "/api/v1/health", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"database":"ok"`) {
		t.Fatalf("health: %d %s", rec.Code, rec.Body)
	}
}

func TestEventsAPI(t *testing.T) {
	h, svc, _ := stack(t)
	ctx := context.Background()
	setup, _ := svc.Bootstrap(ctx, identity.BootstrapInput{OrgName: "Nachtwerk", Slug: "nachtwerk", OwnerEmail: "o@n.example", OwnerName: "O"})
	_ = svc.CompleteSetup(ctx, setup, "the owner passphrase")
	c := &client{t: t, h: h}
	c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": "o@n.example", "password": "the owner passphrase"}, true)

	rec := c.do(http.MethodPost, "/api/v1/venues", map[string]any{"name": "Tresor.West", "city": "Berlin", "timezone": "Europe/Berlin",
		"rooms": []map[string]any{{"name": "Main Room"}}, "protected": map[string]string{"address": "Unterstraße 3"}}, true)
	var venue struct {
		ID        string          `json:"id"`
		Protected map[string]bool `json:"protected"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &venue)
	if rec.Code != http.StatusCreated || !venue.Protected["address"] || strings.Contains(rec.Body.String(), "Unterstraße") {
		t.Fatalf("create venue must mask protected fields: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/v1/venues/"+venue.ID+"/reveal", nil, true); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "mfa_required") {
		t.Fatalf("reveal without a second factor must be refused: %d %s", rec.Code, rec.Body)
	}

	start := time.Unix(time.Now().Add(72*time.Hour).Unix()/3600*3600, 0).UTC()
	rec = c.do(http.MethodPost, "/api/v1/events", map[string]any{"title": "Klubnacht", "starts_at": start, "ends_at": start.Add(8 * time.Hour),
		"timezone": "Europe/Berlin", "venue_id": venue.ID}, true)
	var ev struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
		Stages  []struct {
			ID string `json:"id"`
		} `json:"stages"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ev)
	if rec.Code != http.StatusCreated || len(ev.Stages) != 1 {
		t.Fatalf("create event: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/v1/events", map[string]any{"title": ""}, true); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid event must be 422, got %d", rec.Code)
	}

	lineup := []map[string]any{
		{"display_name": "A", "stage_id": ev.Stages[0].ID, "set_start": start, "set_end": start.Add(3 * time.Hour)},
		{"display_name": "B", "stage_id": ev.Stages[0].ID, "set_start": start.Add(2 * time.Hour), "set_end": start.Add(5 * time.Hour)},
	}
	rec = c.do(http.MethodPut, "/api/v1/events/"+ev.ID+"/lineup", map[string]any{"version": ev.Version, "lineup": lineup}, true)
	_ = json.Unmarshal(rec.Body.Bytes(), &ev)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":"overlap"`) {
		t.Fatalf("conflicted lineup saves with issues: %d %s", rec.Code, rec.Body)
	}
	rec = c.do(http.MethodPost, "/api/v1/events/"+ev.ID+"/status", map[string]any{"version": ev.Version, "status": "published"}, true)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "timetable_errors") {
		t.Fatalf("publish must be blocked: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodGet, "/api/v1/events/"+ev.ID+"/export/ics", nil, false); rec.Code != http.StatusConflict {
		t.Fatalf("export must be blocked while conflicted: %d", rec.Code)
	}
	lineup[1]["set_start"] = start.Add(3 * time.Hour)
	rec = c.do(http.MethodPut, "/api/v1/events/"+ev.ID+"/lineup", map[string]any{"version": ev.Version, "lineup": lineup}, true)
	_ = json.Unmarshal(rec.Body.Bytes(), &ev)
	rec = c.do(http.MethodGet, "/api/v1/events/"+ev.ID+"/export/ics", nil, false)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/calendar; charset=utf-8" || !strings.Contains(rec.Body.String(), "BEGIN:VEVENT") {
		t.Fatalf("ics: %d %s", rec.Code, rec.Header())
	}
	rec = c.do(http.MethodGet, "/api/v1/events/"+ev.ID+"/export/jsonld", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"@type":"MusicEvent"`) {
		t.Fatalf("jsonld: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodGet, "/api/v1/events?view=drafts", nil, false); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"act_count":2`) {
		t.Fatalf("list drafts: %d %s", rec.Code, rec.Body)
	}
}
