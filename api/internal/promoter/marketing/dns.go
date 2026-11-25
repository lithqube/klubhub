// Package marketing holds the Promoter email-platform pieces (P3.2). This
// file is the sending-domain DNS checker: read-only SPF/DKIM/DMARC TXT
// lookups, bounded by a timeout so a dead resolver is an error, never a hang.
package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Resolver is the one DNS call the checker needs; *net.Resolver satisfies it.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// Record is one check's result. Error is empty on pass, otherwise one of
// not_found, timeout, lookup_failed, multiple_records, invalid_record.
type Record struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Pass  bool   `json:"pass"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// Status is the sending domain's authentication state.
type Status struct {
	Domain string `json:"domain"`
	SPF    Record `json:"spf"`
	DKIM   Record `json:"dkim"`
	DMARC  Record `json:"dmarc"`
}

// Checker runs the three lookups under one shared deadline.
type Checker struct {
	Resolver Resolver
	Timeout  time.Duration
}

// Check looks up SPF on domain, DKIM on selector._domainkey.domain and
// DMARC on _dmarc.domain. It always returns within Timeout.
func (c Checker) Check(ctx context.Context, domain, selector string) Status {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	return Status{
		Domain: domain,
		SPF:    c.lookup(ctx, "spf", domain, "v=spf1"),
		DKIM:   c.lookup(ctx, "dkim", selector+"._domainkey."+domain, ""),
		DMARC:  c.lookup(ctx, "dmarc", "_dmarc."+domain, "v=dmarc1"),
	}
}

// lookup finds the single TXT record starting with prefix (DKIM: the one
// carrying a non-empty p= key).
func (c Checker) lookup(ctx context.Context, typ, name, prefix string) Record {
	rec := Record{Type: typ, Name: name}
	txt, err := c.Resolver.LookupTXT(ctx, name)
	if err != nil {
		rec.Error = lookupError(ctx, err)
		return rec
	}
	var found []string
	for _, t := range txt {
		if (prefix == "" && dkimKey(t) != "") || (prefix != "" && strings.HasPrefix(strings.ToLower(t), prefix)) {
			found = append(found, t)
		}
	}
	switch {
	case len(found) == 1:
		rec.Pass, rec.Value = true, found[0]
	case len(found) > 1:
		rec.Error = "multiple_records" // RFC 7208 §4.5 / RFC 7489 §6.6.3: more than one is a failure
	case len(txt) > 0:
		rec.Error = "invalid_record"
	default:
		rec.Error = "not_found"
	}
	return rec
}

// dkimKey returns the p= tag of a DKIM key record; empty means revoked or absent.
func dkimKey(t string) string {
	for _, tag := range strings.Split(t, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(tag), "=")
		if ok && strings.TrimSpace(k) == "p" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func lookupError(ctx context.Context, err error) string {
	var dnsErr *net.DNSError
	switch {
	case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &dnsErr) && dnsErr.IsTimeout):
		return "timeout"
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return "not_found"
	default:
		return "lookup_failed"
	}
}

// Handler exposes the checker under /api/v1/marketing.
type Handler struct{ checker Checker }

// NewHandler builds the HTTP adapter.
func NewHandler(c Checker) *Handler { return &Handler{checker: c} }

// Mount registers the route through the authz guard; campaign.read is
// already declared in the Rego policy (owner, admin, marketing).
func (h *Handler) Mount(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	e.Handle(r, reg, authz.Route{Method: http.MethodGet, Pattern: "/api/v1/marketing/dns-status", Action: "campaign.read", ResourceType: "sending_domain"}, h.dnsStatus, onDeny)
}

var hostLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// validDomain accepts a dotted hostname with at least two labels.
func validDomain(d string) bool {
	labels := strings.Split(d, ".")
	if len(d) > 253 || len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !hostLabel.MatchString(l) {
			return false
		}
	}
	return true
}

// GET /api/v1/marketing/dns-status?domain=<sending domain>&selector=<DKIM selector>
func (h *Handler) dnsStatus(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain"))), ".")
	selector := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("selector")))
	w.Header().Set("Content-Type", "application/json")
	switch {
	case !validDomain(domain):
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid", "field": "domain"})
	case !hostLabel.MatchString(selector):
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid", "field": "selector"})
	default:
		_ = json.NewEncoder(w).Encode(h.checker.Check(r.Context(), domain, selector))
	}
}
