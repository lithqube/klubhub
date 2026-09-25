// Package server assembles the KlubHub Promoter HTTP API: middleware order,
// the authz-guarded route table, health and the optional Nuxt proxy.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	platformhttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/identity"
)

// Pinger reports database health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators the router needs.
type Deps struct {
	Log           zerolog.Logger
	DB            *tenantdb.DB
	Authz         *authz.Engine
	Authn         auth.Authenticator
	Identity      *identity.Handler // nil when the provider is not local
	Events        *event.Handler
	Origins       []string
	ServeFrontend bool
	NuxtURL       string
}

// New returns the router and its authz registry (for coverage checks).
//
// Order: recover → log → body cap → security headers → CSRF → authn →
// per-route authz guard. Authentication never short-circuits; the guard
// decides, so an unauthenticated request to a protected route is a 401 and
// a known principal without a grant is a 403 with a reason.
func New(d Deps) (*chi.Mux, *authz.Registry) {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(platformhttp.RequestLogger(d.Log))
	r.Use(platformhttp.MaxBodyBytesMiddleware)
	r.Use(securityHeaders)
	r.Use(auth.CSRF(d.Origins))
	r.Use(auth.Middleware(d.Authn))

	reg := authz.NewRegistry()
	onDeny := func(req *http.Request, p authz.Principal, action string, dec authz.Decision) {
		route := chi.RouteContext(req.Context()).RoutePattern()
		d.Log.Warn().Str("event", "authz.denied").Str("action", action).
			Str("reason", dec.Reason).Str("route", route).Str("org", p.OrgID).Msg("request denied")
		tenant, err := uuid.Parse(p.OrgID)
		if err != nil {
			return
		}
		// Append-only audit trail, in the caller's tenant.
		if err := d.DB.WithTenant(req.Context(), tenant, func(tx pgx.Tx) error {
			return audit.Record(req.Context(), tx, tenant, audit.Entry{
				ActorID: p.Sub, Action: action, Resource: route, Reason: dec.Reason,
			})
		}); err != nil {
			d.Log.Error().Err(err).Msg("audit write failed")
		}
	}

	d.Authz.Handle(r, reg, authz.Route{Method: http.MethodGet, Pattern: "/api/v1/health", Public: true}, health(d.DB), onDeny)
	if d.Identity != nil {
		d.Identity.Mount(r, d.Authz, reg, onDeny)
	}
	if d.Events != nil {
		d.Events.Mount(r, d.Authz, reg, onDeny)
	}

	notFound := func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
	if d.ServeFrontend {
		front := platformhttp.FrontendHandler(d.NuxtURL)
		notFound = func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/api" || strings.HasPrefix(req.URL.Path, "/api/") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
				return
			}
			front(w, req)
		}
	}
	r.NotFound(notFound)
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	})
	return r, reg
}

// securityHeaders covers API responses. Pages proxied from Nuxt carry
// nuxt-security's headers (CSP with nonce, HSTS, …); setting them here too
// would duplicate X-Frame-Options, which some browsers then ignore.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cache-Control", "no-store")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

func health(db *tenantdb.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "database": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
