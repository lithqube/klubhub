package authz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/open-policy-agent/opa/v1/tester"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

const org = "0190f1d2-0000-7000-8000-000000000001"

func engine(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// TestRegoUnitTests runs policies/*_test.rego (the `opa test` gate) without
// needing the opa CLI in CI.
func TestRegoUnitTests(t *testing.T) {
	results, err := tester.Run(context.Background(), "policies")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 5 {
		t.Fatalf("expected the policy test suite to run, got %d results", len(results))
	}
	for _, r := range results {
		if !r.Pass() {
			t.Errorf("rego test %s failed: %v %s", r.Name, r.Error, r.Output)
		}
	}
}

func TestDecisions(t *testing.T) {
	e := engine(t)
	now := time.Unix(10_000, 0)
	fresh := now.Add(-time.Minute)
	cases := []struct {
		name   string
		p      authz.Principal
		action string
		res    authz.Resource
		allow  bool
		reason string
	}{
		{"booker edits events", authz.Principal{Sub: "u", OrgID: org, Roles: []string{"booker"}, AuthTime: fresh}, "event.write", authz.Resource{Type: "event", OrgID: org}, true, ""},
		{"marketing cannot see artist fees", authz.Principal{Sub: "u", OrgID: org, Roles: []string{"marketing"}, AMR: []string{"otp"}, AuthTime: fresh}, "artist_fee.read", authz.Resource{Type: "booking", OrgID: org}, false, "no_role_grant"},
		{"door cannot export audience", authz.Principal{Sub: "d", OrgID: org, Roles: []string{"door"}, EventScope: "e1", AuthTime: fresh}, "audience.export", authz.Resource{Type: "audience", OrgID: org, EventID: "e1"}, false, "no_role_grant"},
		{"other tenant", authz.Principal{Sub: "u", OrgID: org, Roles: []string{"owner"}, AuthTime: fresh}, "event.read", authz.Resource{Type: "event", OrgID: "someone-else"}, false, "tenant_mismatch"},
		{"missing tenant", authz.Principal{Sub: "u", Roles: []string{"owner"}, AuthTime: fresh}, "event.read", authz.Resource{Type: "event"}, false, "no_tenant"},
		{"finance without second factor", authz.Principal{Sub: "u", OrgID: org, Roles: []string{"finance"}, AMR: []string{"pwd"}, AuthTime: fresh}, "finance.read", authz.Resource{Type: "invoice", OrgID: org}, false, "mfa_required"},
		{"stale owner session", authz.Principal{Sub: "u", OrgID: org, Roles: []string{"owner"}, AMR: []string{"otp"}, AuthTime: now.Add(-time.Hour)}, "security.manage", authz.Resource{Type: "org", OrgID: org}, false, "reauthentication_required"},
		{"unknown action", authz.Principal{Sub: "u", OrgID: org, Roles: []string{"owner"}, AuthTime: fresh}, "event.nuke", authz.Resource{Type: "event", OrgID: org}, false, "unknown_action"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := e.Authorize(context.Background(), authz.Request{Principal: c.p, Action: c.action, Resource: c.res, Now: now})
			if d.Allow != c.allow || d.Reason != c.reason {
				t.Fatalf("got %+v, want allow=%v reason=%q", d, c.allow, c.reason)
			}
		})
	}
}

func TestGuardMiddleware(t *testing.T) {
	e := engine(t)
	reg := authz.NewRegistry()
	mux := chi.NewRouter()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	var denied []string
	onDeny := func(_ *http.Request, _ authz.Principal, action string, d authz.Decision) {
		denied = append(denied, action+":"+d.Reason)
	}
	e.Handle(mux, reg, authz.Route{Method: http.MethodGet, Pattern: "/api/v1/health", Public: true}, ok, onDeny)
	e.Handle(mux, reg, authz.Route{Method: http.MethodPost, Pattern: "/api/v1/events", Action: "event.write", ResourceType: "event"}, ok, onDeny)

	do := func(method, path string, p *authz.Principal) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if p != nil {
			req = req.WithContext(authz.ContextWithPrincipal(req.Context(), *p))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := do(http.MethodGet, "/api/v1/health", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("public route: %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/events", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no principal must be 401, got %d", rec.Code)
	}
	marketing := &authz.Principal{Sub: "m", OrgID: org, Roles: []string{"marketing"}, AuthTime: time.Now()}
	rec := do(http.MethodPost, "/api/v1/events", marketing)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "no_role_grant") {
		t.Fatalf("marketing must be 403 no_role_grant, got %d %s", rec.Code, rec.Body)
	}
	booker := &authz.Principal{Sub: "b", OrgID: org, Roles: []string{"booker"}, AuthTime: time.Now()}
	if rec := do(http.MethodPost, "/api/v1/events", booker); rec.Code != http.StatusNoContent {
		t.Fatalf("booker must pass, got %d", rec.Code)
	}
	if len(denied) != 1 || denied[0] != "event.write:no_role_grant" {
		t.Fatalf("denials must be reported for auditing, got %v", denied)
	}
}

func TestVerifyCoverage(t *testing.T) {
	e := engine(t)
	reg := authz.NewRegistry()
	mux := chi.NewRouter()
	h := func(http.ResponseWriter, *http.Request) {}
	e.Handle(mux, reg, authz.Route{Method: http.MethodGet, Pattern: "/api/v1/events", Action: "event.read", ResourceType: "event"}, h, nil)
	if p := authz.VerifyCoverage(context.Background(), mux, reg); len(p) != 0 {
		t.Fatalf("registered route flagged: %v", p)
	}
	mux.Get("/api/v1/sneaky", h) // mounted without the guard
	e.Handle(mux, reg, authz.Route{Method: http.MethodGet, Pattern: "/api/v1/odd", Action: "odd.action", ResourceType: "x"}, h, nil)
	p := authz.VerifyCoverage(context.Background(), mux, reg)
	joined := ""
	for _, err := range p {
		joined += err.Error() + "\n"
	}
	if !strings.Contains(joined, "/api/v1/sneaky is mounted without an authz registration") ||
		!strings.Contains(joined, `declares unknown action "odd.action"`) {
		t.Fatalf("coverage must catch unguarded routes and unknown actions, got:\n%s", joined)
	}
}
