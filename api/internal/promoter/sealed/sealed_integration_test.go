package sealed_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/pgtest"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/door"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/identity"
	"github.com/klubhub/dj/api/internal/promoter/retention"
	"github.com/klubhub/dj/api/internal/promoter/sealed"
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

// stack wires identity, the sealed tier, the door and retention over the
// shared database. The sealed and retention clock is movable; identity and
// the door use the real clock (door sessions).
type stack struct {
	keys      *envelope.Keyring
	db        *tenantdb.DB
	clock     *clock
	id        *identity.Service
	events    *event.Service
	retention *retention.Service
	mux       http.Handler
}

func newStack(t *testing.T) *stack {
	t.Helper()
	if testing.Short() || testDB == nil {
		t.Skip("integration: postgres unavailable or -short")
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	kek, _ := envelope.NewKEK("kek-test", raw)
	keys := envelope.NewKeyring(kek)
	db := tenantdb.New(testDB.App)
	s := &stack{
		keys: keys, db: db, clock: &clock{t: time.Now().UTC()},
		id: identity.NewService(db, keys, identity.Options{
			Argon2: auth.Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
		}),
		events: event.NewService(db, keys, nil),
	}
	s.retention = retention.NewService(db, s.clock.now)
	engine, err := authz.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	reg := authz.NewRegistry()
	identity.NewHandler(s.id).Mount(r, engine, reg, nil)
	sealed.NewHandler(sealed.NewService(db, keys, s.clock.now)).Mount(r, engine, reg, nil)
	door.NewHandler(door.NewService(db, keys, s.id, nil)).Mount(r, engine, reg, nil)
	s.mux = r
	return s
}

type org struct {
	s     *stack
	t     *testing.T
	id    uuid.UUID
	ctx   context.Context
	owner uuid.UUID
}

func (s *stack) newOrg(t *testing.T) *org {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if err := tenantdb.WithTenant(context.Background(), testDB.App, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Nachtwerk', $2)`, id, "nw-"+id.String()[24:])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	o := &org{s: s, t: t, id: id, ctx: tenantdb.ContextWithTenant(context.Background(), id)}
	o.owner = o.member("Mara Owner", "mara@nachtwerk.example", "owner")
	return o
}

// member creates a user with sealed email and name (as identity does) and
// its memberships.
func (o *org) member(name, email string, roles ...string) uuid.UUID {
	o.t.Helper()
	id := uuid.Must(uuid.NewV7())
	ctx := context.Background()
	err := o.s.db.WithTenant(ctx, o.id, func(tx pgx.Tx) error {
		dek, err := o.s.keys.Current(ctx, tx, o.id)
		if err != nil {
			return err
		}
		e, _ := dek.Seal(o.id, envelope.Field{Table: "users", Column: "email_enc", RowID: id}, []byte(email))
		n, _ := dek.Seal(o.id, envelope.Field{Table: "users", Column: "display_name_enc", RowID: id}, []byte(name))
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, tenant_id, email_enc, email_bidx, display_name_enc) VALUES ($1, $2, $3, $4, $5)`,
			id, o.id, e, dek.BlindIndex("users", "email", email), n); err != nil {
			return err
		}
		for _, r := range roles {
			if _, err := tx.Exec(ctx, `INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, $3)`, o.id, id, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		o.t.Fatal(err)
	}
	return id
}

// as is a fresh, MFA session of user with roles.
func (o *org) as(user uuid.UUID, roles ...string) authz.Principal {
	return authz.Principal{Sub: "local:" + user.String(), OrgID: o.id.String(), Roles: roles, AMR: []string{"pwd", "otp"}, AuthTime: time.Now()}
}

func (o *org) ownerP() authz.Principal { return o.as(o.owner, "owner") }

func (o *org) call(p authz.Principal, method, path string, body any) *httptest.ResponseRecorder {
	o.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	ctx := authz.ContextWithPrincipal(req.Context(), p)
	req = req.WithContext(tenantdb.ContextWithTenant(ctx, o.id))
	rec := httptest.NewRecorder()
	o.s.mux.ServeHTTP(rec, req)
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, contains ...string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
	for _, c := range contains {
		if !strings.Contains(rec.Body.String(), c) {
			t.Fatalf("response lacks %q: %s", c, rec.Body)
		}
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	return v
}

func count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.Owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ---- test vectors: syntactically valid, cryptographically meaningless ----

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func blob(n int) string {
	b := randBytes(n)
	b[0] = 0x01
	return b64(b)
}

// pubKey is a random canonical X25519 public key (top bit clear; the
// server refuses non-canonical and low-order encodings).
func pubKey() []byte {
	b := randBytes(32)
	b[31] &= 0x7f
	return b
}

func wrapBlob() string { return blob(sealed.MinWrap) }

func memberKeyBody(pub []byte) map[string]any {
	return map[string]any{
		"public_key": b64(pub), "private_sealed": blob(sealed.MinPrivateSealed),
		"kdf": map[string]any{"alg": "argon2id", "m": 65536, "t": 3, "p": 1, "salt": b64(randBytes(16))},
	}
}

func recoveryBody() map[string]any {
	pub := pubKey()
	return map[string]any{"public_key": b64(pub), "fingerprint": sealed.Fingerprint(pub), "wrap": wrapBlob()}
}

func memberWrap(user uuid.UUID) map[string]any {
	return map[string]any{"recipient_kind": "member", "recipient_id": user, "wrap": wrapBlob()}
}

func deviceWrap(device uuid.UUID) map[string]any {
	return map[string]any{"recipient_kind": "device", "recipient_id": device, "wrap": wrapBlob()}
}

// giveKey creates the member key of user.
func (o *org) giveKey(user uuid.UUID, roles ...string) {
	o.t.Helper()
	expect(o.t, o.call(o.as(user, roles...), http.MethodPut, "/api/v1/keys/me", memberKeyBody(pubKey())), http.StatusCreated)
}

// setUp creates the owner's key and OSK version 1 with the owner's wrap
// plus extra wraps; it returns the setup body.
func (o *org) setUp(extra ...map[string]any) map[string]any {
	o.t.Helper()
	o.giveKey(o.owner, "owner")
	body := map[string]any{"version": 1, "recovery": recoveryBody(), "wraps": append([]map[string]any{memberWrap(o.owner)}, extra...)}
	expect(o.t, o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/setup", body), http.StatusCreated, `"status":"ready"`, `"version":1`)
	return body
}

func (o *org) status(p authz.Principal) sealed.OrgStatus {
	o.t.Helper()
	rec := o.call(p, http.MethodGet, "/api/v1/keys/org", nil)
	expect(o.t, rec, http.StatusOK)
	return decode[sealed.OrgStatus](o.t, rec)
}

func (o *org) ban(p authz.Principal, version int, expires time.Time) sealed.BanEntry {
	o.t.Helper()
	rec := o.call(p, http.MethodPost, "/api/v1/ban-list", map[string]any{
		"id": uuid.Must(uuid.NewV7()), "key_version": version, "entry_sealed": blob(200), "expires_at": expires,
	})
	expect(o.t, rec, http.StatusCreated)
	return decode[sealed.BanEntry](o.t, rec)
}

// device registers a door device (with a public key when withKey) and, for
// an event, logs it in and returns the door principal too.
func (o *org) device(label string, withKey bool, eventID *uuid.UUID) (uuid.UUID, authz.Principal) {
	o.t.Helper()
	ctx := context.Background()
	var pub []byte
	if withKey {
		pub = pubKey()
	}
	d, err := o.s.id.RegisterDoorDeviceWithKey(ctx, o.ownerP(), label, pub)
	if err != nil {
		o.t.Fatal(err)
	}
	if eventID == nil {
		return d.ID, authz.Principal{}
	}
	pin, err := o.s.id.SetDoorPIN(ctx, o.ownerP(), *eventID, time.Now().Add(10*time.Hour))
	if err != nil {
		o.t.Fatal(err)
	}
	res, err := o.s.id.DoorLogin(ctx, d.Token, *eventID, pin)
	if err != nil {
		o.t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: res.Cookie})
	p, err := o.s.id.Authenticate(r)
	if err != nil {
		o.t.Fatal(err)
	}
	return d.ID, p
}

func (o *org) night() uuid.UUID {
	o.t.Helper()
	start := time.Now().Add(-time.Hour).UTC()
	ev, err := o.s.events.CreateEvent(o.ctx, event.EventInput{
		Title: "Sealed Night", StartsAt: start, EndsAt: start.Add(8 * time.Hour), Timezone: "Europe/Berlin", City: "Berlin",
	})
	if err != nil {
		o.t.Fatal(err)
	}
	return ev.ID
}

// -------------------------------------------------------------- tests ---

func TestMemberKeyLifecycle(t *testing.T) {
	o := newStack(t).newOrg(t)
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	bp := o.as(booker, "booker")

	expect(t, o.call(bp, http.MethodGet, "/api/v1/keys/me", nil), http.StatusNotFound, `"no_member_key"`)
	pub := pubKey()
	body := memberKeyBody(pub)
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/me", body), http.StatusCreated, b64(pub))
	got := decode[sealed.MemberKey](t, o.call(bp, http.MethodGet, "/api/v1/keys/me", nil))
	if got.PublicKey != b64(pub) || got.PrivateSealed != body["private_sealed"] || got.KDF.M != 65536 || got.KDF.Alg != "argon2id" {
		t.Fatalf("stored key differs: %+v", got)
	}

	// Passphrase change: same public key, new private blob and salt.
	rewrap := memberKeyBody(pub)
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/me", rewrap), http.StatusOK)
	if got := decode[sealed.MemberKey](t, o.call(bp, http.MethodGet, "/api/v1/keys/me", nil)); got.PrivateSealed != rewrap["private_sealed"] {
		t.Fatal("re-wrap not stored")
	}
	// A different public key is not a re-wrap.
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/me", memberKeyBody(pubKey())), http.StatusConflict, `"public_key_mismatch"`)

	// Structural refusals.
	bad := memberKeyBody(pub)
	bad["public_key"] = b64(randBytes(31))
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/me", bad), http.StatusUnprocessableEntity, `"field":"public_key"`)
	bad = memberKeyBody(pub)
	bad["kdf"].(map[string]any)["m"] = 1024
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/me", bad), http.StatusUnprocessableEntity, `"field":"kdf.m"`)
	bad = memberKeyBody(pub)
	bad["private_sealed"] = b64(append([]byte{0x02}, randBytes(80)...))
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/me", bad), http.StatusUnprocessableEntity, `"field":"private_sealed"`)

	// Door sessions have no member key.
	ev := o.night()
	_, doorP := o.device("Door", true, &ev)
	expect(t, o.call(doorP, http.MethodGet, "/api/v1/keys/me", nil), http.StatusForbidden)
}

func TestOwnerReplacesOwnKeyAndLosesOldWraps(t *testing.T) {
	o := newStack(t).newOrg(t)
	o.setUp()
	if o.status(o.ownerP()).MyWrap == nil {
		t.Fatal("owner has a wrap after setup")
	}
	// Recovery with the kit: a new keypair replaces the old one (owner +
	// security.manage), and wraps sealed to the old key are dropped.
	expect(t, o.call(o.ownerP(), http.MethodPut, "/api/v1/keys/me", memberKeyBody(pubKey())), http.StatusOK)
	if o.status(o.ownerP()).MyWrap != nil {
		t.Fatal("wraps of the replaced key must be deleted")
	}
	// Without a recent sign-in the replacement is refused.
	stale := o.ownerP()
	stale.AuthTime = time.Now().Add(-time.Hour)
	expect(t, o.call(stale, http.MethodPut, "/api/v1/keys/me", memberKeyBody(pubKey())), http.StatusConflict, `"public_key_mismatch"`)
}

func TestSetupHappyPathAndOnlyOnce(t *testing.T) {
	o := newStack(t).newOrg(t)
	if st := o.status(o.ownerP()); st.Status != "not_setup" || st.Version != nil || st.MyWrap != nil || st.RecoveryFingerprint != nil {
		t.Fatalf("fresh org: %+v", st)
	}
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	o.giveKey(booker, "booker")
	devID, _ := o.device("Door 1", true, nil)
	body := o.setUp(memberWrap(booker), deviceWrap(devID))

	st := o.status(o.ownerP())
	rec := body["recovery"].(map[string]any)
	if st.Status != "ready" || *st.Version != 1 || *st.RecoveryFingerprint != rec["fingerprint"] ||
		*st.MyWrap != body["wraps"].([]map[string]any)[0]["wrap"] {
		t.Fatalf("status after setup: %+v", st)
	}
	if bs := o.status(o.as(booker, "booker")); bs.MyWrap == nil || *bs.MyWrap != body["wraps"].([]map[string]any)[1]["wrap"] {
		t.Fatalf("booker's wrap: %+v", bs)
	}
	if n := count(t, `SELECT count(*) FROM org_key_wraps WHERE tenant_id = $1 AND version = 1`, o.id); n != 4 {
		t.Fatalf("recovery + 3 wraps, got %d", n)
	}

	got := decode[sealed.Recovery](t, o.call(o.ownerP(), http.MethodGet, "/api/v1/keys/org/recovery", nil))
	if got.Version != 1 || got.PublicKey != rec["public_key"] || got.Fingerprint != rec["fingerprint"] || got.Wrap != rec["wrap"] {
		t.Fatalf("recovery: %+v", got)
	}

	again := map[string]any{"version": 1, "recovery": recoveryBody(), "wraps": []map[string]any{memberWrap(o.owner)}}
	expect(t, o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/setup", again), http.StatusConflict, `"already_setup"`)
}

func TestSetupRefusals(t *testing.T) {
	o := newStack(t).newOrg(t)
	setup := func(wraps ...map[string]any) *httptest.ResponseRecorder {
		return o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/setup", map[string]any{"version": 1, "recovery": recoveryBody(), "wraps": wraps})
	}
	expect(t, setup(memberWrap(o.owner)), http.StatusConflict, `"no_member_key"`)
	o.giveKey(o.owner, "owner")

	keyless := o.member("Kim NoKey", "kim@nachtwerk.example", "booker")
	gone := o.member("Gil Gone", "gil@nachtwerk.example") // user without a membership
	revoked, _ := o.device("Revoked", true, nil)
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/door/devices/"+revoked.String(), nil), http.StatusNoContent)
	keylessDevice, _ := o.device("No key", false, nil)

	expect(t, setup(), http.StatusUnprocessableEntity, `"field":"wraps"`, "your own wrap")
	expect(t, setup(memberWrap(o.owner), memberWrap(uuid.New())), http.StatusUnprocessableEntity, `"field":"wraps[1].recipient_id"`, "unknown member")
	expect(t, setup(memberWrap(o.owner), memberWrap(gone)), http.StatusUnprocessableEntity, "unknown member")
	expect(t, setup(memberWrap(o.owner), memberWrap(keyless)), http.StatusUnprocessableEntity, "member has no key")
	expect(t, setup(memberWrap(o.owner), deviceWrap(revoked)), http.StatusUnprocessableEntity, "device is revoked")
	expect(t, setup(memberWrap(o.owner), deviceWrap(keylessDevice)), http.StatusUnprocessableEntity, "device has no public key")
	expect(t, setup(memberWrap(o.owner), deviceWrap(uuid.New())), http.StatusUnprocessableEntity, "unknown device")
	expect(t, setup(memberWrap(o.owner), map[string]any{"recipient_kind": "recovery", "wrap": wrapBlob()}), http.StatusUnprocessableEntity, `"field":"wraps[1].recipient_kind"`)

	rec := recoveryBody()
	rec["fingerprint"] = "00000000000000ff"
	expect(t, o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/setup", map[string]any{"version": 1, "recovery": rec, "wraps": []any{memberWrap(o.owner)}}),
		http.StatusUnprocessableEntity, `"field":"recovery.fingerprint"`)
	expect(t, o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/setup", map[string]any{"version": 2, "recovery": recoveryBody(), "wraps": []any{memberWrap(o.owner)}}),
		http.StatusUnprocessableEntity, `"field":"version"`)

	if st := o.status(o.ownerP()); st.Status != "not_setup" {
		t.Fatalf("refused setups must write nothing: %+v", st)
	}
	if n := count(t, `SELECT count(*) FROM org_key_wraps WHERE tenant_id = $1`, o.id); n != 0 {
		t.Fatalf("no wraps may be left behind, got %d", n)
	}
}

func TestRecipients(t *testing.T) {
	o := newStack(t).newOrg(t)
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker", "finance")
	o.member("Kim NoKey", "kim@nachtwerk.example", "marketing")
	o.giveKey(booker, "booker")
	withKey, _ := o.device("Door key", true, nil)
	noKey, _ := o.device("Door nokey", false, nil)
	revoked, _ := o.device("Door old", true, nil)
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/door/devices/"+revoked.String(), nil), http.StatusNoContent)
	o.setUp(deviceWrap(withKey))

	r := decode[sealed.Recipients](t, o.call(o.ownerP(), http.MethodGet, "/api/v1/keys/org/recipients", nil))
	if len(r.Members) != 3 || len(r.Devices) != 3 || r.Version == nil || *r.Version != 1 {
		t.Fatalf("recipients: %+v", r)
	}
	byEmail := map[string]sealed.MemberRecipient{}
	for _, m := range r.Members {
		byEmail[m.Email] = m
	}
	if m := byEmail["mara@nachtwerk.example"]; m.Name != "Mara Owner" || m.Role != "owner" || m.PublicKey == nil || !m.HasWrap {
		t.Fatalf("owner: %+v", m)
	}
	if m := byEmail["ben@nachtwerk.example"]; m.Role != "booker" || m.PublicKey == nil || m.HasWrap || m.UserID != booker {
		t.Fatalf("booker (highest role, key, no wrap): %+v", m)
	}
	if m := byEmail["kim@nachtwerk.example"]; m.PublicKey != nil || m.HasWrap {
		t.Fatalf("keyless member: %+v", m)
	}
	byID := map[uuid.UUID]sealed.DeviceRecipient{}
	for _, d := range r.Devices {
		byID[d.DeviceID] = d
	}
	if d := byID[withKey]; d.PublicKey == nil || !d.HasWrap || d.Revoked || d.Label != "Door key" {
		t.Fatalf("provisioned device: %+v", d)
	}
	if d := byID[noKey]; d.PublicKey != nil || d.HasWrap {
		t.Fatalf("keyless device: %+v", d)
	}
	if d := byID[revoked]; !d.Revoked {
		t.Fatalf("revoked device: %+v", d)
	}

	// The staff device list shows the sealed status too (additive fields).
	rec := o.call(o.ownerP(), http.MethodGet, "/api/v1/door/devices", nil)
	expect(t, rec, http.StatusOK)
	var list []identity.DoorDeviceInfo
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	for _, d := range list {
		if d.ID == withKey && (!d.HasWrap || d.PublicKey == nil) || d.ID == noKey && (d.HasWrap || d.PublicKey != nil) {
			t.Fatalf("device list sealed status: %+v", d)
		}
	}
}

func TestSecurityManageNeedsOwnerMFAAndStepUp(t *testing.T) {
	o := newStack(t).newOrg(t)
	admin := o.member("Ada Admin", "ada@nachtwerk.example", "admin")
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	noMFA := o.ownerP()
	noMFA.AMR = []string{"pwd"}
	stale := o.ownerP()
	stale.AuthTime = time.Now().Add(-16 * time.Minute)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/keys/org/recipients"},
		{http.MethodPost, "/api/v1/keys/org/setup"},
		{http.MethodPost, "/api/v1/keys/org/wraps"},
		{http.MethodPost, "/api/v1/keys/org/rotate"},
		{http.MethodGet, "/api/v1/keys/org/recovery"},
	}
	for _, rt := range routes {
		for _, c := range []struct {
			p      authz.Principal
			reason string
		}{
			{o.as(admin, "admin"), "no_role_grant"},
			{o.as(booker, "booker"), "no_role_grant"},
			{noMFA, "mfa_required"},
			{stale, "reauthentication_required"},
		} {
			rec := o.call(c.p, rt.method, rt.path, map[string]any{})
			expect(t, rec, http.StatusForbidden, c.reason)
		}
	}
	// account.self routes stay open to every staff member.
	expect(t, o.call(o.as(booker, "booker"), http.MethodGet, "/api/v1/keys/org", nil), http.StatusOK, `"not_setup"`)
}

func TestAddWrapsAndDeviceWraps(t *testing.T) {
	o := newStack(t).newOrg(t)
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	o.giveKey(booker, "booker")
	dev, _ := o.device("Door", true, nil)
	keyless, _ := o.device("No key", false, nil)

	add := func(body map[string]any) *httptest.ResponseRecorder {
		return o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/wraps", body)
	}
	expect(t, add(map[string]any{"wraps": []any{memberWrap(booker)}}), http.StatusConflict, `"not_setup"`)
	expect(t, o.call(o.as(booker, "booker"), http.MethodPut, "/api/v1/keys/devices/"+dev.String()+"/wrap", map[string]any{"wrap": wrapBlob()}),
		http.StatusConflict, `"not_setup"`)
	o.setUp()

	expect(t, add(map[string]any{"version": 2, "wraps": []any{memberWrap(booker)}}), http.StatusConflict, `"version_conflict"`)
	expect(t, add(map[string]any{"wraps": []any{memberWrap(uuid.New())}}), http.StatusUnprocessableEntity, "unknown member")
	expect(t, add(map[string]any{"wraps": []any{}}), http.StatusUnprocessableEntity, `"field":"wraps"`)
	// Aliases of the wrap fields are accepted.
	expect(t, add(map[string]any{"version": 1, "wraps": []any{map[string]any{"kind": "member", "recipient_id": booker, "wrap_sealed": wrapBlob()}}}),
		http.StatusOK, `"added":1`)
	if o.status(o.as(booker, "booker")).MyWrap == nil {
		t.Fatal("booker was granted")
	}

	// Door devices: booker has door.device.manage.
	bp := o.as(booker, "booker")
	w := wrapBlob()
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/devices/"+dev.String()+"/wrap", map[string]any{"wrap": w, "version": 1}), http.StatusOK, `"version":1`)
	if n := count(t, `SELECT count(*) FROM org_key_wraps WHERE recipient_kind = 'device' AND recipient_id = $1`, dev); n != 1 {
		t.Fatalf("device wrap stored: %d", n)
	}
	// Replacing it keeps one wrap.
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/devices/"+dev.String()+"/wrap", map[string]any{"wrap": wrapBlob()}), http.StatusOK)
	if n := count(t, `SELECT count(*) FROM org_key_wraps WHERE recipient_kind = 'device' AND recipient_id = $1`, dev); n != 1 {
		t.Fatalf("device wrap replaced, not duplicated: %d", n)
	}
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/devices/"+keyless.String()+"/wrap", map[string]any{"wrap": wrapBlob()}),
		http.StatusUnprocessableEntity, "no public key")
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/devices/"+uuid.NewString()+"/wrap", map[string]any{"wrap": wrapBlob()}), http.StatusNotFound)
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/devices/"+dev.String()+"/wrap", map[string]any{"wrap": b64(randBytes(20))}),
		http.StatusUnprocessableEntity, `"field":"wrap"`)
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/door/devices/"+dev.String(), nil), http.StatusNoContent)
	expect(t, o.call(bp, http.MethodPut, "/api/v1/keys/devices/"+dev.String()+"/wrap", map[string]any{"wrap": wrapBlob()}),
		http.StatusUnprocessableEntity, "revoked")
	expect(t, add(map[string]any{"wraps": []any{deviceWrap(dev)}}), http.StatusUnprocessableEntity, "revoked")
}

func TestBanListCRUD(t *testing.T) {
	s := newStack(t)
	o := s.newOrg(t)
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	bp := o.as(booker, "booker")
	now := s.clock.t
	month := now.AddDate(0, 1, 0)
	post := func(body map[string]any) *httptest.ResponseRecorder {
		return o.call(bp, http.MethodPost, "/api/v1/ban-list", body)
	}
	entry := func(version int, exp time.Time) map[string]any {
		return map[string]any{"id": uuid.Must(uuid.NewV7()), "key_version": version, "entry_sealed": blob(200), "expires_at": exp}
	}

	list := decode[sealed.BanList](t, o.call(bp, http.MethodGet, "/api/v1/ban-list", nil))
	if list.KeyVersion != nil || len(list.Entries) != 0 {
		t.Fatalf("empty list before setup: %+v", list)
	}
	expect(t, post(entry(1, month)), http.StatusConflict, `"not_setup"`)
	o.setUp()

	e := entry(1, month)
	expect(t, post(e), http.StatusCreated, `"key_version":1`, e["entry_sealed"].(string))
	expect(t, post(e), http.StatusConflict, `"ban_entry_exists"`)
	expect(t, post(entry(2, month)), http.StatusConflict, `"key_version_stale"`)
	expect(t, post(entry(1, now.Add(12*time.Hour))), http.StatusUnprocessableEntity, `"field":"expires_at"`)
	expect(t, post(entry(1, now.AddDate(3, 0, 2))), http.StatusUnprocessableEntity, `"field":"expires_at"`)
	expect(t, post(entry(1, now.AddDate(3, 0, 0))), http.StatusCreated)
	expect(t, post(entry(1, now.Add(24*time.Hour))), http.StatusCreated)
	noID := entry(1, month)
	delete(noID, "id")
	expect(t, post(noID), http.StatusUnprocessableEntity, `"field":"id"`)
	short := entry(1, month)
	short["entry_sealed"] = blob(20)
	expect(t, post(short), http.StatusUnprocessableEntity, `"field":"entry_sealed"`)

	list = decode[sealed.BanList](t, o.call(bp, http.MethodGet, "/api/v1/ban-list", nil))
	if *list.KeyVersion != 1 || len(list.Entries) != 3 || list.Entries[0].ID != e["id"] {
		t.Fatalf("list: %+v", list)
	}
	// Who added each entry (the staff member's display name, opened server-side).
	for _, x := range list.Entries {
		if x.CreatedByName == nil || *x.CreatedByName != "Ben Booker" {
			t.Fatalf("created_by_name: %+v", x)
		}
	}
	expect(t, o.call(bp, http.MethodGet, "/api/v1/ban-list", nil), http.StatusOK, `"created_by_name":"Ben Booker"`)

	id := e["id"].(uuid.UUID).String()
	upd := map[string]any{"key_version": 1, "entry_sealed": blob(300), "expires_at": now.AddDate(0, 6, 0)}
	rec := o.call(bp, http.MethodPut, "/api/v1/ban-list/"+id, upd)
	expect(t, rec, http.StatusOK, upd["entry_sealed"].(string))
	if got := decode[sealed.BanEntry](t, rec); got.ExpiresAt.Sub(now.AddDate(0, 6, 0)).Abs() > time.Millisecond {
		t.Fatalf("expiry not updated: %v", got.ExpiresAt)
	}
	upd["key_version"] = 3
	expect(t, o.call(bp, http.MethodPut, "/api/v1/ban-list/"+id, upd), http.StatusConflict, `"key_version_stale"`)
	upd["key_version"], upd["id"] = 1, uuid.New()
	expect(t, o.call(bp, http.MethodPut, "/api/v1/ban-list/"+id, upd), http.StatusUnprocessableEntity, `"field":"id"`)
	delete(upd, "id")
	expect(t, o.call(bp, http.MethodPut, "/api/v1/ban-list/"+uuid.NewString(), upd), http.StatusNotFound)

	expect(t, o.call(bp, http.MethodDelete, "/api/v1/ban-list/"+id, nil), http.StatusNoContent)
	expect(t, o.call(bp, http.MethodDelete, "/api/v1/ban-list/"+id, nil), http.StatusNotFound)

	// Writes are audited and announced with ids only.
	if n := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action LIKE 'banlist.entry_%' AND resource = $2`, o.id, "ban_entry:"+id); n != 3 {
		t.Fatalf("add, update, remove audited: %d", n)
	}
	if n := count(t, `SELECT count(*) FROM outbox WHERE tenant_id = $1 AND subject LIKE '%.banlist.updated' AND payload->'refs'->>'entry_id' = $2`, o.id, id); n != 3 {
		t.Fatalf("banlist.updated per write: %d", n)
	}

	// A door session gets the ban list through its bundle only.
	ev := o.night()
	_, doorP := o.device("Door", true, &ev)
	expect(t, o.call(doorP, http.MethodGet, "/api/v1/ban-list", nil), http.StatusForbidden)
}

func TestExpiredEntriesAreHiddenAndPurged(t *testing.T) {
	s := newStack(t)
	o := s.newOrg(t)
	o.setUp()
	soon := o.ban(o.ownerP(), 1, s.clock.t.Add(25*time.Hour))
	later := o.ban(o.ownerP(), 1, s.clock.t.AddDate(1, 0, 0))

	s.clock.t = s.clock.t.Add(48 * time.Hour)
	list := decode[sealed.BanList](t, o.call(o.ownerP(), http.MethodGet, "/api/v1/ban-list", nil))
	if len(list.Entries) != 1 || list.Entries[0].ID != later.ID {
		t.Fatalf("expired entry must be hidden: %+v", list.Entries)
	}
	expect(t, o.call(o.ownerP(), http.MethodPut, "/api/v1/ban-list/"+soon.ID.String(),
		map[string]any{"key_version": 1, "entry_sealed": blob(64), "expires_at": s.clock.t.AddDate(0, 1, 0)}), http.StatusNotFound)

	n, err := s.retention.ExpiredBans(context.Background())
	if err != nil || n < 1 {
		t.Fatalf("dry run counts the expired entry: %d %v", n, err)
	}
	res, err := s.retention.PurgeExpiredBans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res {
		found = found || (r.TenantID == o.id && r.Deleted == 1)
	}
	if !found {
		t.Fatalf("purge result: %+v", res)
	}
	if c := count(t, `SELECT count(*) FROM ban_entries WHERE tenant_id = $1`, o.id); c != 1 {
		t.Fatalf("only the unexpired entry remains, got %d", c)
	}
	if c := count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'banlist.expired_purged' AND reason LIKE '1 expired%'`, o.id); c != 1 {
		t.Fatalf("purge audited with the count: %d", c)
	}
	if c := count(t, `SELECT count(*) FROM outbox WHERE tenant_id = $1 AND payload->'refs'->>'entry_id' = $2`, o.id, soon.ID.String()); c != 2 {
		t.Fatalf("banlist.updated on add and on expiry: %d", c)
	}
}

func TestMemberRemoval(t *testing.T) {
	o := newStack(t).newOrg(t)
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	plain := o.member("Kim Keyless", "kim@nachtwerk.example", "marketing")
	admin := o.member("Ada Admin", "ada@nachtwerk.example", "admin")
	o.giveKey(booker, "booker")
	o.setUp(memberWrap(booker))
	ctx := context.Background()
	if err := o.s.db.WithTenant(ctx, o.id, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO sessions (id, tenant_id, token_hash, user_id, amr, auth_time, expires_at)
		  VALUES ($1, $2, $3, $4, ARRAY['pwd'], now(), now() + interval '1 day')`, uuid.New(), o.id, randBytes(32), booker)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// A member without a wrap never held the key: no rotation needed.
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/members/"+plain.String(), nil), http.StatusNoContent)
	if st := o.status(o.ownerP()); st.Status != "ready" {
		t.Fatalf("keyless member removed: %+v", st)
	}

	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/members/"+booker.String(), nil), http.StatusNoContent)
	for q, want := range map[string]int{
		`SELECT count(*) FROM memberships WHERE user_id = $1`:                                                   0,
		`SELECT count(*) FROM member_keys WHERE user_id = $1`:                                                   0,
		`SELECT count(*) FROM org_key_wraps WHERE recipient_id = $1`:                                            0,
		`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`:                               0,
		`SELECT count(*) FROM audit_log WHERE action = 'member.removed' AND resource = 'user:' || $1`:           1,
		`SELECT count(*) FROM audit_log WHERE action = 'keys.recipient_removed' AND resource = 'member:' || $1`: 1,
	} {
		if n := count(t, q, booker); n != want {
			t.Errorf("%s: got %d, want %d", q, n, want)
		}
	}
	if st := o.status(o.ownerP()); st.Status != "rotation_pending" {
		t.Fatalf("removing a key holder sets rotation_pending: %+v", st)
	}
	if n := count(t, `SELECT count(*) FROM org_key_state WHERE tenant_id = $1 AND rotation_reason = 'member_removed' AND pending_since IS NOT NULL`, o.id); n != 1 {
		t.Fatal("reason member_removed")
	}
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/members/"+booker.String(), nil), http.StatusNotFound)

	// Refusals.
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/members/"+o.owner.String(), nil), http.StatusConflict, `"cannot_remove_self"`)
	expect(t, o.call(o.as(admin, "admin"), http.MethodDelete, "/api/v1/members/"+o.owner.String(), nil), http.StatusForbidden, `"owner_required"`)
	ghostOwner := authz.Principal{Sub: "local:" + uuid.NewString(), OrgID: o.id.String(), Roles: []string{"owner"}, AMR: []string{"otp"}, AuthTime: time.Now()}
	expect(t, o.call(ghostOwner, http.MethodDelete, "/api/v1/members/"+o.owner.String(), nil), http.StatusConflict, `"last_owner"`)
	second := o.member("Otto Owner", "otto@nachtwerk.example", "owner")
	expect(t, o.call(o.as(second, "owner"), http.MethodDelete, "/api/v1/members/"+o.owner.String(), nil), http.StatusNoContent)
	noMFA := o.as(second, "owner")
	noMFA.AMR = []string{"pwd"}
	expect(t, o.call(noMFA, http.MethodDelete, "/api/v1/members/"+admin.String(), nil), http.StatusForbidden, "mfa_required")
}

func TestDeviceRevokeSetsRotationPending(t *testing.T) {
	o := newStack(t).newOrg(t)
	provisioned, _ := o.device("Door A", true, nil)
	bare, _ := o.device("Door B", true, nil)
	o.setUp(deviceWrap(provisioned))

	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/door/devices/"+bare.String(), nil), http.StatusNoContent)
	if st := o.status(o.ownerP()); st.Status != "ready" {
		t.Fatalf("a device without a wrap never held the key: %+v", st)
	}
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/door/devices/"+provisioned.String(), nil), http.StatusNoContent)
	if st := o.status(o.ownerP()); st.Status != "rotation_pending" {
		t.Fatalf("revoking a provisioned device: %+v", st)
	}
	if n := count(t, `SELECT count(*) FROM org_key_wraps WHERE recipient_id = $1`, provisioned); n != 0 {
		t.Fatalf("device wraps deleted: %d", n)
	}
	if n := count(t, `SELECT count(*) FROM org_key_state WHERE tenant_id = $1 AND rotation_reason = 'device_revoked'`, o.id); n != 1 {
		t.Fatal("reason device_revoked")
	}
}

func TestRotateIsAtomic(t *testing.T) {
	s := newStack(t)
	o := s.newOrg(t)
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	o.giveKey(booker, "booker")
	dev, _ := o.device("Door", true, nil)
	o.setUp(memberWrap(booker), deviceWrap(dev))
	a := o.ban(o.ownerP(), 1, s.clock.t.AddDate(0, 1, 0))
	b := o.ban(o.ownerP(), 1, s.clock.t.AddDate(0, 2, 0))
	expired := o.ban(o.ownerP(), 1, s.clock.t.Add(25*time.Hour))
	s.clock.t = s.clock.t.Add(26 * time.Hour)
	expect(t, o.call(o.ownerP(), http.MethodDelete, "/api/v1/door/devices/"+dev.String(), nil), http.StatusNoContent)

	newA, newB := blob(220), blob(230)
	body := func(from, to int, entries ...map[string]any) map[string]any {
		return map[string]any{"from_version": from, "to_version": to, "recovery": recoveryBody(),
			"wraps": []any{memberWrap(o.owner), memberWrap(booker)}, "ban_entries": entries}
	}
	ea := map[string]any{"id": a.ID, "entry_sealed": newA}
	eb := map[string]any{"id": b.ID, "entry_sealed": newB}
	rotate := func(b map[string]any) *httptest.ResponseRecorder {
		return o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/rotate", b)
	}
	unchanged := func() {
		t.Helper()
		if n := count(t, `SELECT count(*) FROM org_sealed_keys WHERE tenant_id = $1`, o.id); n != 1 {
			t.Fatalf("no new version may exist: %d", n)
		}
		if n := count(t, `SELECT count(*) FROM ban_entries WHERE tenant_id = $1 AND key_version = 1`, o.id); n != 3 {
			t.Fatalf("entries untouched: %d", n)
		}
		if st := o.status(o.ownerP()); st.Status != "rotation_pending" || *st.Version != 1 {
			t.Fatalf("status untouched: %+v", st)
		}
	}

	expect(t, rotate(body(1, 2, ea)), http.StatusUnprocessableEntity, `"field":"ban_entries"`, "1 missing")
	unchanged()
	expect(t, rotate(body(1, 2, ea, eb, map[string]any{"id": uuid.New(), "entry_sealed": blob(64)})), http.StatusUnprocessableEntity, "1 unknown")
	expect(t, rotate(body(1, 2, ea, eb, map[string]any{"id": expired.ID, "entry_sealed": blob(64)})), http.StatusUnprocessableEntity, "1 unknown")
	expect(t, rotate(body(1, 2, ea, ea)), http.StatusUnprocessableEntity, "duplicate")
	expect(t, rotate(body(2, 3, ea, eb)), http.StatusConflict, `"version_conflict"`)
	expect(t, rotate(body(1, 3, ea, eb)), http.StatusUnprocessableEntity, `"field":"to_version"`)
	noSelf := body(1, 2, ea, eb)
	noSelf["wraps"] = []any{memberWrap(booker)}
	expect(t, rotate(noSelf), http.StatusUnprocessableEntity, "your own wrap")
	withRevoked := body(1, 2, ea, eb)
	withRevoked["wraps"] = []any{memberWrap(o.owner), deviceWrap(dev)}
	expect(t, rotate(withRevoked), http.StatusUnprocessableEntity, "revoked")
	unchanged()

	good := body(1, 2, ea, eb)
	rec := rotate(good)
	expect(t, rec, http.StatusOK, `"status":"ready"`, `"version":2`)
	st := decode[sealed.OrgStatus](t, rec)
	if *st.RecoveryFingerprint != good["recovery"].(map[string]any)["fingerprint"] || *st.MyWrap != good["wraps"].([]any)[0].(map[string]any)["wrap"] {
		t.Fatalf("new version's recovery and own wrap: %+v", st)
	}
	for q, want := range map[string]int{
		`SELECT count(*) FROM org_sealed_keys WHERE tenant_id = $1 AND version = 1 AND status = 'retired' AND retired_at IS NOT NULL`: 1,
		`SELECT count(*) FROM org_sealed_keys WHERE tenant_id = $1 AND version = 2 AND status = 'active'`:                             1,
		`SELECT count(*) FROM org_key_wraps WHERE tenant_id = $1 AND version = 1`:                                                     0,
		`SELECT count(*) FROM org_key_wraps WHERE tenant_id = $1 AND version = 2`:                                                     3,
		`SELECT count(*) FROM ban_entries WHERE tenant_id = $1 AND key_version = 2`:                                                   2,
		`SELECT count(*) FROM ban_entries WHERE tenant_id = $1`:                                                                       2,
		`SELECT count(*) FROM org_key_state WHERE tenant_id = $1 AND NOT rotation_pending AND rotation_reason IS NULL`:                1,
		`SELECT count(*) FROM outbox WHERE tenant_id = $1 AND subject LIKE '%.keys.rotated'`:                                          1,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'keys.rotated'`:                                             1,
	} {
		if n := count(t, q, o.id); n != want {
			t.Errorf("%s: got %d, want %d", q, n, want)
		}
	}
	list := decode[sealed.BanList](t, o.call(o.ownerP(), http.MethodGet, "/api/v1/ban-list", nil))
	got := map[uuid.UUID]string{}
	for _, e := range list.Entries {
		got[e.ID] = e.EntrySealed
		if e.KeyVersion != 2 {
			t.Fatalf("entry still on the old version: %+v", e)
		}
	}
	if *list.KeyVersion != 2 || got[a.ID] != newA || got[b.ID] != newB {
		t.Fatalf("entries replaced by their re-encryption: %+v", list)
	}
	if bs := o.status(o.as(booker, "booker")); bs.MyWrap == nil {
		t.Fatal("booker holds a wrap of version 2")
	}
	// Writers still on version 1 are refused.
	expect(t, o.call(o.ownerP(), http.MethodPost, "/api/v1/ban-list", map[string]any{
		"id": uuid.New(), "key_version": 1, "entry_sealed": blob(64), "expires_at": s.clock.t.AddDate(0, 1, 0),
	}), http.StatusConflict, `"key_version_stale"`)
	expect(t, rotate(body(1, 2, ea, eb)), http.StatusConflict, `"version_conflict"`)
}

func TestDoorBundleSealedBlock(t *testing.T) {
	s := newStack(t)
	o := s.newOrg(t)
	ev := o.night()
	dev, doorP := o.device("Door", true, &ev)
	bundle := func() door.Bundle {
		t.Helper()
		rec := o.call(doorP, http.MethodGet, "/api/v1/door/bundle", nil)
		expect(t, rec, http.StatusOK)
		if !strings.Contains(rec.Body.String(), `"sealed":`) {
			t.Fatalf("bundle always carries the sealed key: %s", rec.Body)
		}
		return decode[door.Bundle](t, rec)
	}
	if bundle().Sealed != nil {
		t.Fatal("not set up: sealed is null")
	}
	o.setUp()
	e := o.ban(o.ownerP(), 1, time.Now().AddDate(0, 1, 0))
	if bundle().Sealed != nil {
		t.Fatal("device without a wrap: sealed is null")
	}
	w := wrapBlob()
	expect(t, o.call(o.ownerP(), http.MethodPut, "/api/v1/keys/devices/"+dev.String()+"/wrap", map[string]any{"wrap": w}), http.StatusOK)
	sb := bundle().Sealed
	if sb == nil || sb.KeyVersion != 1 || sb.Wrap != w || len(sb.BanEntries) != 1 || sb.BanEntries[0].ID != e.ID || sb.BanEntries[0].EntrySealed != e.EntrySealed {
		t.Fatalf("sealed block: %+v", sb)
	}
	// Another device's wrap is never handed out.
	other, _ := o.device("Other", true, nil)
	expect(t, o.call(o.ownerP(), http.MethodPut, "/api/v1/keys/devices/"+other.String()+"/wrap", map[string]any{"wrap": wrapBlob()}), http.StatusOK)
	if got := bundle().Sealed; got.Wrap != w {
		t.Fatal("bundle must carry this device's wrap only")
	}
}

func TestTenantIsolation(t *testing.T) {
	s := newStack(t)
	a, b := s.newOrg(t), s.newOrg(t)
	a.setUp()
	entry := a.ban(a.ownerP(), 1, time.Now().AddDate(0, 1, 0))

	if st := b.status(b.ownerP()); st.Status != "not_setup" {
		t.Fatalf("org B sees org A's key: %+v", st)
	}
	list := decode[sealed.BanList](t, b.call(b.ownerP(), http.MethodGet, "/api/v1/ban-list", nil))
	if list.KeyVersion != nil || len(list.Entries) != 0 {
		t.Fatalf("org B sees org A's ban list: %+v", list)
	}
	expect(t, b.call(b.ownerP(), http.MethodDelete, "/api/v1/ban-list/"+entry.ID.String(), nil), http.StatusNotFound)
	b.giveKey(b.owner, "owner")
	body := map[string]any{"version": 1, "recovery": recoveryBody(), "wraps": []any{memberWrap(b.owner), memberWrap(a.owner)}}
	expect(t, b.call(b.ownerP(), http.MethodPost, "/api/v1/keys/org/setup", body), http.StatusUnprocessableEntity, "unknown member")
	// A's owner principal against B's tenant has no member key there.
	cross := a.ownerP()
	cross.OrgID = b.id.String()
	expect(t, b.call(cross, http.MethodGet, "/api/v1/keys/me", nil), http.StatusNotFound, "no_member_key")
	if n := count(t, `SELECT count(*) FROM ban_entries WHERE id = $1`, entry.ID); n != 1 {
		t.Fatal("org A's entry must survive org B's delete")
	}
}

func TestCiphertextNeverReachesAuditOrOutbox(t *testing.T) {
	s := newStack(t)
	o := s.newOrg(t)
	booker := o.member("Ben Booker", "ben@nachtwerk.example", "booker")
	pub := pubKey()
	keyBody := memberKeyBody(pub)
	expect(t, o.call(o.as(booker, "booker"), http.MethodPut, "/api/v1/keys/me", keyBody), http.StatusCreated)
	body := o.setUp(memberWrap(booker))
	e := o.ban(o.ownerP(), 1, time.Now().AddDate(0, 1, 0))
	o.call(o.ownerP(), http.MethodGet, "/api/v1/keys/org/recovery", nil)
	newEntry := blob(100)
	rot := map[string]any{"from_version": 1, "to_version": 2, "recovery": recoveryBody(), "wraps": []any{memberWrap(o.owner)},
		"ban_entries": []any{map[string]any{"id": e.ID, "entry_sealed": newEntry}}}
	expect(t, o.call(o.ownerP(), http.MethodPost, "/api/v1/keys/org/rotate", rot), http.StatusOK)

	secrets := []string{b64(pub), keyBody["private_sealed"].(string), e.EntrySealed, newEntry,
		body["recovery"].(map[string]any)["wrap"].(string), rot["recovery"].(map[string]any)["wrap"].(string)}
	for _, w := range body["wraps"].([]map[string]any) {
		secrets = append(secrets, w["wrap"].(string))
	}
	var dump strings.Builder
	rows, err := testDB.Owner.Query(context.Background(), `SELECT action || ' ' || resource || ' ' || coalesce(reason, '') FROM audit_log WHERE tenant_id = $1
	  UNION ALL SELECT subject || ' ' || payload::text FROM outbox WHERE tenant_id = $1`, o.id)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		dump.WriteString(l + "\n")
	}
	for _, sct := range secrets {
		// Any 16-char window of the base64url would do; the whole value
		// and its standard-base64 form are checked.
		raw, _ := base64.RawURLEncoding.DecodeString(sct)
		for _, form := range []string{sct, base64.StdEncoding.EncodeToString(raw), sct[:24]} {
			if strings.Contains(dump.String(), form) {
				t.Fatalf("ciphertext or key material found in audit/outbox:\n%s", dump.String())
			}
		}
	}
	if !strings.Contains(dump.String(), "keys.rotated") || !strings.Contains(dump.String(), "banlist.entry_added") {
		t.Fatalf("expected audit entries missing:\n%s", dump.String())
	}
}
