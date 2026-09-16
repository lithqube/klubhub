package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/klubhub/dj/api/internal/platform/config"
)

// DBPinger is satisfied by *pgxpool.Pool.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// StorageHealthChecker is satisfied by *storage.Client (after adding HealthCheck method).
type StorageHealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// HealthResponse is the JSON shape returned by GET /api/v1/health.
type HealthResponse struct {
	Status       string                       `json:"status"`
	Integrations map[string]IntegrationStatus `json:"integrations"`
	MigrationsOK bool                         `json:"migrations_ok"`
}

// IntegrationStatus describes the health of a single downstream integration.
type IntegrationStatus struct {
	Status  string `json:"status"`            // "ok" | "error" | "unconfigured"
	Message string `json:"message,omitempty"` // present only on error
}

// NewHealthHandler returns an http.Handler for GET /api/v1/health.
// It checks the DB, Garage object storage, and optional integrations.
func NewHealthHandler(pool DBPinger, store StorageHealthChecker, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		integrations := make(map[string]IntegrationStatus, 6)
		unhealthy := false

		// 1. Database ping (400 ms deadline)
		dbCtx, cancel := context.WithTimeout(r.Context(), 400*time.Millisecond)
		defer cancel()
		if err := pool.Ping(dbCtx); err != nil {
			integrations["database"] = IntegrationStatus{Status: "error", Message: err.Error()}
			unhealthy = true
		} else {
			integrations["database"] = IntegrationStatus{Status: "ok"}
		}

		// 2. Storage ping
		storeCtx, cancelStore := context.WithTimeout(r.Context(), 400*time.Millisecond)
		defer cancelStore()
		if err := store.HealthCheck(storeCtx); err != nil {
			integrations["storage"] = IntegrationStatus{Status: "error", Message: err.Error()}
			unhealthy = true
		} else {
			integrations["storage"] = IntegrationStatus{Status: "ok"}
		}

		// 3. Optional integrations — key-present checks only
		if cfg.SpotifyClientID == "" {
			integrations["spotify"] = IntegrationStatus{Status: "unconfigured"}
		} else {
			integrations["spotify"] = IntegrationStatus{Status: "ok"}
		}

		if cfg.DiscogsAPIKey == "" {
			integrations["discogs"] = IntegrationStatus{Status: "unconfigured"}
		} else {
			integrations["discogs"] = IntegrationStatus{Status: "ok"}
		}

		// MusicBrainz requires no API key
		integrations["musicbrainz"] = IntegrationStatus{Status: "ok"}

		if cfg.InstagramClientID == "" {
			integrations["instagram"] = IntegrationStatus{Status: "unconfigured"}
		} else {
			integrations["instagram"] = IntegrationStatus{Status: "ok"}
		}

		// 4. Compute overall status and HTTP status code.
		//    Any required subsystem (database or storage) being unhealthy flips
		//    the response to HTTP 503 so Docker's healthcheck (and external
		//    monitors) see a real failure rather than an always-200 probe.
		overallStatus := "healthy"
		httpStatus := http.StatusOK
		if unhealthy {
			overallStatus = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
		}

		resp := HealthResponse{
			Status:       overallStatus,
			Integrations: integrations,
			MigrationsOK: true, // migrations ran at startup; if we're here, they succeeded
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(resp)
	})
}
