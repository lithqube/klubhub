package social

import "regexp"

// Patterns used to redact provider credentials from transport error
// messages. Each pattern targets a specific kind of leak observed in
// the audit:
//
//   - access_token=X (query param value, body key=value, fragment)
//   - client_secret=X (query or body)
//   - client_id=X (query param value)
//
// We replace the *value* with a stable marker rather than the key, so
// the resulting string still hints at *what* was redacted without
// exposing the secret.
var (
	// key=value (query, body, fragment) where value continues until
	// `&`, whitespace, or quote.
	reAccessToken = regexp.MustCompile(`(?i)(access_token|accessToken|access-token)=([^&\s"']+)`)
	reClientSec   = regexp.MustCompile(`(?i)(client_secret|clientSecret|client-secret)=([^&\s"']+)`)
	reClientID    = regexp.MustCompile(`(?i)(client_id|clientId|client-id)=([^&\s"']+)`)

	// Bare-token fragment form: `#token=XYZ` after a URL.
	reFragToken = regexp.MustCompile(`(?i)#token=([^&\s"']+)`)
)

// genericMessage is returned by SanitizeTransportError when the input
// is non-empty but its content was fully redacted, so callers don't
// silently lose the error signal.
const genericMessage = "instagram api error: response omitted for safety"

// SanitizeTransportError returns a copy of the input string with all
// OAuth provider credentials replaced by `[REDACTED]`. It is intended
// for use at the trust boundary where the application hands an error
// message to an external client (HTTP response body) or to a log line
// that may be retained or forwarded.
//
// The function is conservative: any value bound to a known-sensitive
// key in a URL query string, fragment, or `key=value` body form is
// removed. URLs without secrets pass through unchanged.
//
// SanitizeTransportError never returns the input verbatim if a known-
// sensitive key is present.
func SanitizeTransportError(s string) string {
	if s == "" {
		return ""
	}
	out := s
	out = reAccessToken.ReplaceAllString(out, "${1}=[REDACTED]")
	out = reClientSec.ReplaceAllString(out, "${1}=[REDACTED]")
	out = reClientID.ReplaceAllString(out, "${1}=[REDACTED]")
	out = reFragToken.ReplaceAllString(out, "#token=[REDACTED]")
	return out
}
