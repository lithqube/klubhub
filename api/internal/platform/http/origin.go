package http

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// UnsafeRequestBoundary protects DJ's network-private, single-owner API; it
// does not authenticate callers. CORSOrigin is an explicit trusted deployment
// origin (also supports TLS-terminating ingress). Forwarded headers are never
// trust inputs. Unlike Promoter's cookie CSRF middleware, DJ's existing fetch
// client needs no new token/header. Non-browser clients may omit Origin.
//
// DNS rebinding: Origin==Host alone is not enough, since an attacker domain
// rebound to a private address makes both equal. Unsafe /api/v1 requests
// therefore also require the Host to be loopback (localhost, 127.0.0.1, [::1])
// or the host of CORSOrigin, whether or not Origin is present. curl/CLI
// against loopback keeps working; reaching the API by LAN IP or another
// hostname returns 403 until CORS_ORIGIN is set to that origin.
func UnsafeRequestBoundary(trustedOrigin string) func(http.Handler) http.Handler {
	trusted := normalizedOrigin(trustedOrigin)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
				next.ServeHTTP(w, r)
				return
			}
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if !allowedHostname(r.Host, trusted) {
				unsafeRequestError(w, http.StatusForbidden, "csrf", "untrusted request host; if this hostname is legitimate, set CORSOrigin (CORS_ORIGIN) to its origin, e.g. https://dj.example")
				return
			}
			origins, present := r.Header["Origin"]
			if present {
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}
				origin := ""
				if len(origins) == 1 {
					origin = normalizedOrigin(origins[0])
				}
				if origin == "" || (origin != normalizedOrigin(scheme+"://"+r.Host) && (trusted == "" || origin != trusted)) {
					unsafeRequestError(w, http.StatusForbidden, "csrf", "untrusted request origin")
					return
				}
			} else if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				unsafeRequestError(w, http.StatusForbidden, "csrf", "untrusted browser request")
				return
			}
			// Bodyless actions (DELETE, generate, unlink) need no media type.
			// Any supplied type is still validated; unknown/chunked bodies are checked.
			types := r.Header.Values("Content-Type")
			if len(types) > 0 || (r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0) {
				media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				upload := strings.HasPrefix(r.URL.Path, "/api/v1/epk/") || strings.HasPrefix(r.URL.Path, "/api/v1/social/") || strings.HasPrefix(r.URL.Path, "/api/v1/tracklists/")
				if len(types) != 1 || err != nil || (media != "application/json" && !(media == "multipart/form-data" && params["boundary"] != "" && upload)) {
					unsafeRequestError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "use application/json or a supported multipart upload")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Only serialized HTTP(S) origins are accepted, not URLs, lists or opaque null.
func normalizedOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, " \t\r\n\\#") {
		return ""
	}
	host := strings.ToLower(u.Host)
	if (u.Scheme == "http" && u.Port() == "80") || (u.Scheme == "https" && u.Port() == "443") {
		host = strings.ToLower(u.Hostname())
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
	}
	return u.Scheme + "://" + host
}

func unsafeRequestError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

// CORSOriginWarning returns a startup warning when a non-empty CORSOrigin is
// unusable (trailing slash, path, missing scheme, unparseable). Such a value
// is silently ignored by the boundary, so operators should be told.
func CORSOriginWarning(raw string) string {
	if raw == "" || normalizedOrigin(raw) != "" {
		return ""
	}
	return "CORS_ORIGIN is not a bare http(s) origin (e.g. https://dj.example; no trailing slash or path) and is ignored: its host is not allowlisted and its Origin is not trusted"
}

// allowedHostname reports whether the Host header names a loopback host or
// the host of the trusted CORSOrigin. Ports are ignored: DNS rebinding is
// decided by the name, and the Origin==Host check still binds the port.
func allowedHostname(hostHeader, trustedOrigin string) bool {
	name := hostnameOf(hostHeader)
	if name == "" {
		return false
	}
	switch name {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	if trustedOrigin == "" {
		return false
	}
	u, err := url.Parse(trustedOrigin)
	return err == nil && name == strings.ToLower(u.Hostname())
}

func hostnameOf(hostHeader string) string {
	u, err := url.Parse("http://" + hostHeader)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || strings.ContainsAny(hostHeader, " \t\r\n\\/#?@") {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
