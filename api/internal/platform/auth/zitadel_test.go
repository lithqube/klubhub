package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

const (
	issuer   = "https://auth.klubhub.test"
	audience = "331234567890123456"
	orgA     = "259242039378444290"
	orgB     = "259242039378444999"
)

type signer struct {
	priv jwk.Key
	pub  jwk.Set
}

func newSigner(t *testing.T, kid string) signer {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := jwk.Import(k)
	if err != nil {
		t.Fatal(err)
	}
	_ = priv.Set(jwk.KeyIDKey, kid)
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256())
	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	set := jwk.NewSet()
	_ = set.AddKey(pub)
	return signer{priv: priv, pub: set}
}

func (s signer) token(t *testing.T, mutate func(jwt.Token)) string {
	t.Helper()
	now := time.Now()
	tok := jwt.New()
	_ = tok.Set(jwt.IssuerKey, issuer)
	_ = tok.Set(jwt.AudienceKey, []string{audience})
	_ = tok.Set(jwt.SubjectKey, "user-1")
	_ = tok.Set(jwt.IssuedAtKey, now)
	_ = tok.Set(jwt.ExpirationKey, now.Add(10*time.Minute))
	_ = tok.Set(claimResourceOwner, orgA)
	_ = tok.Set(claimRoles, map[string]any{
		"booker":  map[string]any{orgA: "nachtwerk.klubhub.io"},
		"finance": map[string]any{orgB: "other.klubhub.io"},
	})
	_ = tok.Set("amr", []string{"pwd", "otp"})
	_ = tok.Set("auth_time", now.Add(-time.Minute).Unix())
	if mutate != nil {
		mutate(tok)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), s.priv))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func verifier(t *testing.T, keys jwk.Set) *Zitadel {
	t.Helper()
	z, err := NewZitadel(ZitadelConfig{Issuer: issuer, Audience: audience, Keys: func(context.Context) (jwk.Set, error) { return keys, nil }})
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func TestZitadelRequiresAudience(t *testing.T) {
	if _, err := NewZitadel(ZitadelConfig{Issuer: issuer, Keys: func(context.Context) (jwk.Set, error) { return nil, nil }}); err == nil {
		t.Fatal("a verifier without an audience must not be constructible")
	}
}

func TestZitadelMapsOnlyTheTokensOrgRoles(t *testing.T) {
	s := newSigner(t, "k1")
	p, err := verifier(t, s.pub).Verify(context.Background(), s.token(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if p.OrgID != TenantFromZitadelOrg(orgA).String() {
		t.Fatalf("tenant = %s", p.OrgID)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "booker" {
		t.Fatalf("roles from another org leaked in: %v", p.Roles)
	}
	if p.Sub != "zitadel:user-1" || len(p.AMR) != 2 || p.AuthTime.IsZero() {
		t.Fatalf("claims not mapped: %+v", p)
	}
}

func TestZitadelRejectsBadTokens(t *testing.T) {
	s := newSigner(t, "k1")
	other := newSigner(t, "k2")
	z := verifier(t, s.pub)
	cases := map[string]string{
		"wrong audience": s.token(t, func(tok jwt.Token) { _ = tok.Set(jwt.AudienceKey, []string{"another-app"}) }),
		"wrong issuer":   s.token(t, func(tok jwt.Token) { _ = tok.Set(jwt.IssuerKey, "https://evil.example") }),
		"expired":        s.token(t, func(tok jwt.Token) { _ = tok.Set(jwt.ExpirationKey, time.Now().Add(-time.Hour)) }),
		"foreign key":    other.token(t, nil),
		"no org claim":   s.token(t, func(tok jwt.Token) { _ = tok.Remove(claimResourceOwner) }),
		"garbage":        "not.a.jwt",
		"alg none":       "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ4In0.",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := z.Verify(context.Background(), raw); err == nil {
				t.Fatalf("%s accepted", name)
			}
		})
	}
}

func TestTenantDerivationIsStable(t *testing.T) {
	if TenantFromZitadelOrg(orgA) != TenantFromZitadelOrg(orgA) || TenantFromZitadelOrg(orgA) == TenantFromZitadelOrg(orgB) {
		t.Fatal("tenant ids must be stable per org and distinct across orgs")
	}
}

func TestMiddlewareAttachesPrincipalAndTenant(t *testing.T) {
	s := newSigner(t, "k1")
	var gotP authz.Principal
	var gotOK, tenantOK bool
	h := Middleware(verifier(t, s.pub))(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotP, gotOK = authz.PrincipalFrom(r.Context())
		_, tenantOK = tenantdb.TenantFromContext(r.Context())
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+s.token(t, nil))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !gotOK || !tenantOK || gotP.OrgID == "" {
		t.Fatalf("principal and tenant must be attached: %+v", gotP)
	}

	gotOK, tenantOK = false, false
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+newSigner(t, "x").token(t, nil))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if gotOK || tenantOK {
		t.Fatal("an invalid token must leave the request unauthenticated")
	}
}
