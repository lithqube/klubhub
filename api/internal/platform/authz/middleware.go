package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

type principalKey struct{}

// ContextWithPrincipal stores the verified principal (set by the auth
// middleware only; never from client input).
func ContextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the verified principal, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Route is one registered endpoint and the action it performs.
type Route struct {
	Method       string
	Pattern      string
	Action       string
	ResourceType string
	// Public routes skip authorisation (health, login). Each one is a
	// deliberate decision listed in the registry.
	Public bool
}

// Registry records the action of every route so tests can prove that no
// mounted route lacks a policy decision.
type Registry struct {
	mu     sync.Mutex
	routes map[string]Route
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{routes: map[string]Route{}} }

func (r *Registry) add(rt Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[rt.Method+" "+rt.Pattern] = rt
}

// Lookup returns the registered route for method and chi pattern.
func (r *Registry) Lookup(method, pattern string) (Route, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.routes[method+" "+pattern]
	return rt, ok
}

// Routes returns a copy of all registered routes.
func (r *Registry) Routes() []Route {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Route, 0, len(r.routes))
	for _, rt := range r.routes {
		out = append(out, rt)
	}
	return out
}

// Denied is called for every refusal (audit log hook).
type Denied func(r *http.Request, p Principal, action string, d Decision)

// Handle mounts h at method+pattern on mux, guarded by the policy.
// The resource's org is always the principal's tenant; handlers that load
// a specific resource authorise again with its real attributes.
func (e *Engine) Handle(mux chi.Router, reg *Registry, rt Route, h http.HandlerFunc, onDeny Denied) {
	reg.add(rt)
	if rt.Public {
		mux.Method(rt.Method, rt.Pattern, h)
		return
	}
	mux.Method(rt.Method, rt.Pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		d := e.Authorize(r.Context(), Request{
			Principal: p,
			Action:    rt.Action,
			Resource: Resource{
				Type:    rt.ResourceType,
				OrgID:   p.OrgID,
				EventID: chi.URLParam(r, "eventID"),
			},
			Route:  rt.Pattern,
			Method: rt.Method,
		})
		if !d.Allow {
			if onDeny != nil {
				onDeny(r, p, rt.Action, d)
			}
			writeError(w, http.StatusForbidden, d.Reason)
			return
		}
		h(w, r)
	}))
}

func writeError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": reason})
}
