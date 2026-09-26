package auth

import (
	"net/http"
	"net/url"
)

// CSRFHeader must be present on every state-changing request. Browsers
// cannot send a custom header cross-site without a CORS preflight, which
// this API never grants; combined with the Origin check and SameSite=Lax
// cookies this blocks cross-site request forgery.
const CSRFHeader = "X-KlubHub-CSRF"

// CSRF rejects unsafe requests whose Origin (or Referer) is not one of
// allowedOrigins, or that lack CSRFHeader.
func CSRF(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				if ref, err := url.Parse(r.Header.Get("Referer")); err == nil && ref.Scheme != "" {
					origin = ref.Scheme + "://" + ref.Host
				}
			}
			if !allowed[origin] || r.Header.Get(CSRFHeader) == "" {
				http.Error(w, `{"error":"csrf"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
