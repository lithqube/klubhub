package finance

import (
	"net/http"
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
	entries            http.Handler
	attachments        http.Handler
}

// WithDocuments mounts the read-only document handler. Like attachments it
// needs object storage, so it is a separate step and answers 503 without it.
func (m *Mux) WithDocuments(h http.Handler) *Mux {
	m.documents = h
	return m
}

// WithAttachments mounts the receipt handler for /entries/{id}/attachments.
// It is a separate step (not a NewMux argument) because it needs object
// storage: without it those routes answer 503 and everything else is unchanged.
func (m *Mux) WithAttachments(h http.Handler) *Mux {
	m.attachments = h
	return m
}

// NewMux composes the finance routes. Any handler may be nil — that
// subdomain returns 503 ("finance handler not configured"). This lets ops
// run a partial finance surface during rollouts.
func NewMux(
	billing, invoices, payments, documents,
	agreementsTpl, agreementsInstance,
	emails, entries http.Handler,
) *Mux {
	return &Mux{
		billing:            billing,
		invoices:           invoices,
		payments:           payments,
		documents:          documents,
		agreementsTpl:      agreementsTpl,
		agreementsInstance: agreementsInstance,
		emails:             emails,
		entries:            entries,
	}
}

// ServeHTTP dispatches by path. Children parse the full
// /api/v1/finance/<resource>/... path themselves, so the request is passed
// through unchanged (chi's Mount keeps r.URL.Path intact as well).
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

	switch parts[0] {
	case "billing-profile":
		m.dispatch(w, r, m.billing)
	case "invoices":
		// /invoices/{id}/payments belongs to the payments handler.
		if len(parts) == 3 && parts[2] == "payments" {
			m.dispatch(w, r, m.payments)
			return
		}
		m.dispatch(w, r, m.invoices)
	case "payments":
		m.dispatch(w, r, m.payments)
	case "documents":
		m.dispatch(w, r, m.documents)
	case "agreements":
		if len(parts) >= 2 && parts[1] == "templates" {
			m.dispatch(w, r, m.agreementsTpl)
		} else if len(parts) >= 2 && parts[1] == "instances" {
			m.dispatch(w, r, m.agreementsInstance)
		} else {
			writeError(w, http.StatusNotFound, "not_found", "unknown agreements resource")
		}
	case "emails":
		m.dispatch(w, r, m.emails)
	case "entries", "summary", "profit-loss", "reconciliations":
		// /entries/{id}/attachments[/{aid}] belongs to the receipt handler.
		if parts[0] == "entries" && len(parts) >= 3 && parts[2] == "attachments" {
			m.dispatch(w, r, m.attachments)
			return
		}
		m.dispatch(w, r, m.entries)
	default:
		writeError(w, http.StatusNotFound, "not_found", "unknown finance resource: "+parts[0])
	}
}

// dispatch invokes h, or answers 503 when that subdomain isn't configured.
func (m *Mux) dispatch(w http.ResponseWriter, r *http.Request, h http.Handler) {
	if h == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "finance handler not configured")
		return
	}
	h.ServeHTTP(w, r)
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
