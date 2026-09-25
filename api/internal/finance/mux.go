package finance

import (
	"net/http"
	"strings"
)

// Mux dispatches /api/v1/finance/* across all finance handlers in one place.
// Compose this once at boot and mount under the parent router with prefix
// stripping at the parent. Each handler keeps its own subdomain routes
// (e.g. /invoices, /agreements/templates) just like internal/gig/social.
//
// Wire-up done at runtime by api/cmd/api/main.go based on which credentials
// are present: PlunkEmailSender is built from PLUNK_* env, EmailService is
// built from a Postgres EmailRepository, and all handlers are composited
// here. Static-only construction (no DB) so this package compiles cleanly
// even when finance is not wired.
type Mux struct {
	billing            http.Handler
	invoices           http.Handler
	payments           http.Handler
	documents          http.Handler
	agreementsTpl      http.Handler
	agreementsInstance http.Handler
	emails             http.Handler
}

// NewMux composes the finance routes. Any handler may be nil — that
// subdomain returns 503 ("finance handler not configured"). This lets ops
// run a partial finance surface during rollouts.
func NewMux(
	billing, invoices, payments, documents,
	agreementsTpl, agreementsInstance,
	emails http.Handler,
) *Mux {
	return &Mux{
		billing:            billing,
		invoices:           invoices,
		payments:           payments,
		documents:          documents,
		agreementsTpl:      agreementsTpl,
		agreementsInstance: agreementsInstance,
		emails:             emails,
	}
}

// ServeHTTP dispatches by path. Path may or may not start with /api/v1/finance
// depending on how the parent mounted this — handle both: when the prefix
// is present, drop those leading segments before matching.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := splitFinanceSegments(r.URL.Path)

	// Strip leading /api/v1/finance when present.
	const prefixLen = 3 // ["api","v1","finance"]
	if len(parts) >= prefixLen &&
		parts[0] == "api" && parts[1] == "v1" && parts[2] == "finance" {
		parts = parts[prefixLen:]
	}

	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "finance path missing")
		return
	}

	// Make sure rewritten paths match what the children expect.
	original := r.URL.Path
	defer func() { r.URL.Path = original }()

	switch parts[0] {
	case "billing-profile":
		m.dispatchChild(w, r, m.billing, parts[1:])
	case "invoices":
		m.dispatchChild(w, r, m.invoices, parts[1:])
	case "payments":
		m.dispatchChild(w, r, m.payments, parts[1:])
	case "documents":
		m.dispatchChild(w, r, m.documents, parts[1:])
	case "agreements":
		if len(parts) >= 2 && parts[1] == "templates" {
			m.dispatchChild(w, r, m.agreementsTpl, parts[2:])
		} else if len(parts) >= 2 && parts[1] == "instances" {
			m.dispatchChild(w, r, m.agreementsInstance, parts[2:])
		} else {
			writeError(w, http.StatusNotFound, "not_found", "unknown agreements resource")
		}
	case "emails":
		m.dispatchChild(w, r, m.emails, parts[1:])
	default:
		writeError(w, http.StatusNotFound, "not_found", "unknown finance resource: "+parts[0])
	}
}

// dispatchChild invokes h with the rewritten path. Restores r.URL.Path
// after delegation so callers can't observe the change after ServeHTTP
// returns. Each child handler expects paths under /api/v1/<sub>/...
func (m *Mux) dispatchChild(w http.ResponseWriter, r *http.Request, h http.Handler, rest []string) {
	if h == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "finance handler not configured")
		return
	}
	if len(rest) == 0 {
		// No tail; just hand off — child handles its bare root.
		h.ServeHTTP(w, r)
		return
	}
	clone := r.Clone(r.Context())
	clone.URL.Path = "/api/v1/" + strings.Join(rest, "/")
	h.ServeHTTP(w, clone)
}

// splitPath returns the non-empty segments of a URL path, split on '/'.
func splitFinanceSegments(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	cleaned := out[:0]
	for _, p := range out {
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return cleaned
}