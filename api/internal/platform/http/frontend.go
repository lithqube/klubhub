package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const frontendProbeHeader = "X-Klubhub-Frontend-Probe"

func checkFrontend(ctx context.Context, target string) error {
	upstream, err := url.Parse(target)
	if err != nil || upstream.Host == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") {
		return fmt.Errorf("invalid frontend upstream")
	}
	// Always request the root, even if a misconfigured URL contains an API path.
	upstream.Path, upstream.RawPath, upstream.RawQuery, upstream.Fragment = "/", "", "", ""
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set(frontendProbeHeader, "1")
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("frontend unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("frontend returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// frontendHandler keeps the entire API namespace local, including unknown
// routes. Sending an API 404 to Nitro would loop through its /api/v1 proxy.
func frontendHandler(target string) http.HandlerFunc {
	upstream, err := url.Parse(target)
	if err != nil || upstream.Host == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "invalid frontend upstream", http.StatusBadGateway)
		}
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	return func(w http.ResponseWriter, r *http.Request) {
		// A frontend probe reaching the API means the target is wrong. Do not
		// forward it again (including self-target configurations).
		if r.Header.Get(frontendProbeHeader) != "" {
			http.Error(w, "frontend probe reached API", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/") {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}
}
