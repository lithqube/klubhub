package auth

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// ErrUnauthenticated means the request carries no usable credential.
var ErrUnauthenticated = errors.New("auth: unauthenticated")

// Authenticator is an identity provider: `local` for self-host (sessions),
// `zitadel` for SaaS (bearer JWTs). Both yield the same Principal, so
// authorisation and tenancy do not depend on the provider (plan D6).
type Authenticator interface {
	Authenticate(r *http.Request) (authz.Principal, error)
}

// Middleware attaches the verified principal and its tenant to the request
// context. Requests without a valid credential continue unauthenticated and
// are refused by the authz guard on any non-public route; invalid
// credentials are not distinguished from missing ones in the response.
func Middleware(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := a.Authenticate(r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			tenant, err := uuid.Parse(p.OrgID)
			if err != nil || tenant == uuid.Nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := authz.ContextWithPrincipal(r.Context(), p)
			ctx = tenantdb.ContextWithTenant(ctx, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
