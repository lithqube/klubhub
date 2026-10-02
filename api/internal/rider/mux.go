package rider

import (
	"encoding/json"
	"net/http"
)

// Mux dispatches /api/v1/rider/* between the templates and attachments
// handlers in one place. Compose at boot and mount under the parent
// router with prefix stripping at the parent.
//
// Either handler may be nil — that subdomain returns 503 ("rider handler
// not configured"), letting ops run a partial rider surface during
// rollouts. Mirrors the finance.Mux dispatcher contract.
type Mux struct {
	templates   http.Handler
	attachments http.Handler
}

// NewMux composes the rider routes. Either argument may be nil.
func NewMux(templates, attachments http.Handler) *Mux {
	return &Mux{
		templates:   templates,
		attachments: attachments,
	}
}

// ServeHTTP dispatches by path. Children parse the full
// /api/v1/rider/<resource>/... path themselves, so the request is passed
// through unchanged.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefixLen = 3 // ["api", "v1", "rider"]
	parts := splitRiderSegments(r.URL.Path)
	if len(parts) >= prefixLen &&
		parts[0] == "api" && parts[1] == "v1" && parts[2] == "rider" {
		parts = parts[prefixLen:]
	}

	if len(parts) == 0 {
		writeRiderError(w, http.StatusNotFound, "not_found", "rider path missing")
		return
	}

	switch parts[0] {
	case "templates":
		m.dispatch(w, r, m.templates)
	case "attachments":
		m.dispatch(w, r, m.attachments)
	default:
		writeRiderError(w, http.StatusNotFound, "not_found", "unknown rider resource: "+parts[0])
	}
}

func (m *Mux) dispatch(w http.ResponseWriter, r *http.Request, h http.Handler) {
	if h == nil {
		writeRiderError(w, http.StatusServiceUnavailable, "service_unavailable", "rider handler not configured")
		return
	}
	h.ServeHTTP(w, r)
}

// splitRiderSegments returns the non-empty segments of a URL path, split
// on '/'. Duplicated locally because finance.splitFinanceSegments is not
// exported — keeping packages independent avoids an import cycle.
func splitRiderSegments(s string) []string {
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

func writeRiderError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Marshalled, not concatenated: msg can embed a URL path segment.
	body, _ := json.Marshal(map[string]string{"error": code, "message": msg})
	_, _ = w.Write(body)
}
