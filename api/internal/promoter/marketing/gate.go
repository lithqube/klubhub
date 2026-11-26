package marketing

import (
	"context"
	"errors"
	"net/http"
)

// ErrNotPermitted is returned when the operator has not attested that the
// configured provider's terms permit marketing sends. Mapped to
// 422 marketing_not_permitted. See docs/MARKETING-PROVIDER.md.
var ErrNotPermitted = errors.New(
	"marketing: sending is disabled: the operator has not attested that the " +
		"email provider's terms permit marketing email (MARKETING_PROVIDER_PERMITS_MARKETING)")

// Gate is the fail-closed marketing capability check, same pattern as
// platform/email.OutreachGate. The zero value and a nil *Gate refuse.
// Finance and outreach email are unaffected by it.
type Gate struct{ permitted bool }

// NewGate must be fed from an explicit operator setting; never infer it from
// a provider being configured, since that says nothing about its terms.
func NewGate(permitted bool) *Gate { return &Gate{permitted: permitted} }

// AllowsMarketing reports whether marketing sends may proceed.
func (g *Gate) AllowsMarketing() bool { return g != nil && g.permitted }

// Require is route middleware: 422 marketing_not_permitted unless attested.
func (g *Gate) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.AllowsMarketing() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"marketing_not_permitted"}` + "\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Gated wraps p so SendMarketing refuses with ErrNotPermitted unless the gate
// allows it. Webhook handling stays open: bounces and unsubscribes must still
// be recorded even when sending is switched off.
func Gated(g *Gate, p MarketingProvider) MarketingProvider { return gatedProvider{g, p} }

type gatedProvider struct {
	gate *Gate
	MarketingProvider
}

func (p gatedProvider) SendMarketing(ctx context.Context, msg *Message) (string, error) {
	if !p.gate.AllowsMarketing() {
		return "", ErrNotPermitted
	}
	return p.MarketingProvider.SendMarketing(ctx, msg)
}
