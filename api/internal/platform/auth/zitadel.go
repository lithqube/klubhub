package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Zitadel claim names.
const (
	claimRoles         = "urn:zitadel:iam:org:project:roles"
	claimResourceOwner = "urn:zitadel:iam:user:resourceowner:id"
)

// zitadelTenantNamespace derives tenant UUIDs from Zitadel's numeric org IDs
// (UUIDv5), so the same collective has the same tenant id everywhere without
// a metadata lookup. Changing it would orphan every SaaS tenant.
var zitadelTenantNamespace = uuid.MustParse("6f1d3c2e-9a4b-5c7d-8e1f-2a3b4c5d6e7f")

// TenantFromZitadelOrg maps a Zitadel organization id to the tenant UUID.
func TenantFromZitadelOrg(orgID string) uuid.UUID {
	return uuid.NewSHA1(zitadelTenantNamespace, []byte("zitadel-org:"+orgID))
}

// KeySetFunc returns the issuer's current signing keys (JWKS).
type KeySetFunc func(ctx context.Context) (jwk.Set, error)

// ZitadelConfig configures bearer-token verification (SaaS only, plan D6).
type ZitadelConfig struct {
	Issuer   string // e.g. https://auth.klubhub.io
	Audience string // the Zitadel project id; required
	Keys     KeySetFunc
	Skew     time.Duration
	Now      func() time.Time
}

// Zitadel verifies JWT access tokens forwarded by the Nuxt BFF.
type Zitadel struct{ cfg ZitadelConfig }

// NewZitadel refuses to build a verifier without issuer, audience or keys:
// a missing audience would accept tokens minted for any other application.
func NewZitadel(cfg ZitadelConfig) (*Zitadel, error) {
	if cfg.Issuer == "" || cfg.Audience == "" || cfg.Keys == nil {
		return nil, errors.New("auth: zitadel issuer, audience and key set are required")
	}
	if cfg.Skew == 0 {
		cfg.Skew = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Zitadel{cfg: cfg}, nil
}

// RemoteKeySet fetches and caches the issuer's JWKS. jwksURL is usually
// "<issuer>/oauth/v2/keys"; inside Docker point it at the internal address.
func RemoteKeySet(ctx context.Context, jwksURL string) (KeySetFunc, error) {
	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		return nil, err
	}
	if err := cache.Register(ctx, jwksURL); err != nil {
		return nil, err
	}
	return func(ctx context.Context) (jwk.Set, error) { return cache.Lookup(ctx, jwksURL) }, nil
}

// Authenticate implements Authenticator.
func (z *Zitadel) Authenticate(r *http.Request) (authz.Principal, error) {
	h := r.Header.Get("Authorization")
	raw, ok := strings.CutPrefix(h, "Bearer ")
	if !ok || raw == "" {
		return authz.Principal{}, ErrUnauthenticated
	}
	return z.Verify(r.Context(), raw)
}

// Verify checks signature, issuer, audience and lifetime, then maps claims.
func (z *Zitadel) Verify(ctx context.Context, raw string) (authz.Principal, error) {
	keys, err := z.cfg.Keys(ctx)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("auth: load signing keys: %w", err)
	}
	tok, err := jwt.Parse([]byte(raw),
		jwt.WithKeySet(keys),
		jwt.WithValidate(true),
		jwt.WithIssuer(z.cfg.Issuer),
		jwt.WithAudience(z.cfg.Audience),
		jwt.WithAcceptableSkew(z.cfg.Skew),
		jwt.WithClock(jwt.ClockFunc(z.cfg.Now)),
	)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	sub, ok := tok.Subject()
	if !ok || sub == "" {
		return authz.Principal{}, ErrUnauthenticated
	}
	var org string
	if err := tok.Get(claimResourceOwner, &org); err != nil || org == "" {
		return authz.Principal{}, fmt.Errorf("%w: no organisation claim", ErrUnauthenticated)
	}
	// Roles are {role: {orgId: domain}}; only grants for the token's own
	// organisation count, so a user of two collectives never carries one
	// collective's roles into the other.
	var rawRoles any
	_ = tok.Get(claimRoles, &rawRoles)
	grants, _ := rawRoles.(map[string]any)
	var roles []string
	for role, orgs := range grants {
		if m, ok := orgs.(map[string]any); ok {
			if _, ok := m[org]; ok {
				roles = append(roles, role)
			}
		}
	}
	sort.Strings(roles)
	var rawAMR any
	_ = tok.Get("amr", &rawAMR)
	var amr []string
	if list, ok := rawAMR.([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok {
				amr = append(amr, s)
			}
		}
	}
	var rawAuthTime any
	_ = tok.Get("auth_time", &rawAuthTime)
	return authz.Principal{
		Sub:      "zitadel:" + sub,
		OrgID:    TenantFromZitadelOrg(org).String(),
		Roles:    roles,
		AMR:      amr,
		AuthTime: time.Unix(numericSeconds(rawAuthTime), 0),
	}, nil
}

func numericSeconds(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}
