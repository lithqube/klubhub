package authz

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/open-policy-agent/opa/v1/rego"
)

// KnownActions returns the action set declared by the policy.
func KnownActions(ctx context.Context) (map[string]bool, error) {
	var opts []func(*rego.Rego)
	opts = append(opts, rego.Query("data.klubhub.promoter.authz.known_actions"))
	src, err := policyFS.ReadFile("policies/authz.rego")
	if err != nil {
		return nil, err
	}
	opts = append(opts, rego.Module("authz.rego", string(src)))
	rs, err := rego.New(opts...).Eval(ctx)
	if err != nil || len(rs) != 1 {
		return nil, fmt.Errorf("authz: read known actions: %v", err)
	}
	set := map[string]bool{}
	for _, a := range rs[0].Expressions[0].Value.([]any) {
		set[a.(string)] = true
	}
	return set, nil
}

// VerifyCoverage fails if any route mounted on mux is missing from the
// registry, or a registered, non-public route declares an action the policy
// does not know. Run it in the router's tests (plan §13.2).
func VerifyCoverage(ctx context.Context, mux chi.Routes, reg *Registry) []error {
	known, err := KnownActions(ctx)
	if err != nil {
		return []error{err}
	}
	var problems []error
	_ = chi.Walk(mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		rt, ok := reg.Lookup(method, route)
		switch {
		case !ok:
			problems = append(problems, fmt.Errorf("%s %s is mounted without an authz registration", method, route))
		case !rt.Public && !known[rt.Action]:
			problems = append(problems, fmt.Errorf("%s %s declares unknown action %q", method, route, rt.Action))
		}
		return nil
	})
	return problems
}
