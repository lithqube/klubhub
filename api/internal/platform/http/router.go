package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/storage"
	"github.com/rs/zerolog"
)

// RequestLogger emits a structured log line per HTTP request. It is
// referenced by NewRouter below and must live alongside the rest of
// the middleware in this package. Implementation kept terse on
// purpose — chi/logger and chiMiddleware.Logger exist but our logger
// is rs/zerolog, so we own the implementation.
func RequestLogger(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Int("bytes", ww.BytesWritten()).
				Dur("dur", time.Since(start)).
				Msg("http")
		})
	}
}

// NewRouter constructs a chi router with all routes wired.
// settingsHandler handles both GET and PUT /api/v1/settings.
// tracklistHandler handles tracklist CRUD operations.
// socialHandler handles social scheduling routes under /api/v1/social.
// epkHandler handles EPK/press-kit routes under /api/v1/epk.
// gigHandler handles gig CRUD routes under /api/v1/gigs.
// venueHandler handles venue CRUD routes under /api/v1/venues.
// contactHandler handles contact CRUD routes under /api/v1/contacts.
func NewRouter(
	cfg *config.Config,
	pool *pgxpool.Pool,
	store *storage.Client,
	log zerolog.Logger,
	settingsHandler http.Handler,
	tracklistHandler http.Handler,
	socialHandler http.Handler,
	epkHandler http.Handler,
	gigHandler http.Handler,
	venueHandler http.Handler,
	contactHandler http.Handler,
	financeHandler http.Handler,
) *chi.Mux {
	r := chi.NewRouter()

	// Built-in middleware
	r.Use(chiMiddleware.Recoverer)
	r.Use(RequestLogger(log))
	// Plan B.6: cap request bodies at 1 MiB by default. Routes that
	// legitimately need larger bodies (tracklist upload) wrap their
	// own http.MaxBytesReader before this middleware sees the body.
	r.Use(MaxBodyBytesMiddleware)

	healthHandler := NewHealthHandler(pool, store, cfg)

	r.Get("/api/v1/health", healthHandler.ServeHTTP)

	r.Get("/api/v1/settings", settingsHandler.ServeHTTP)
	r.Put("/api/v1/settings", settingsHandler.ServeHTTP)

	// Tracklist routes
	r.Mount("/api/v1/tracklists", tracklistHandler)

	// Social scheduling routes
	r.Mount("/api/v1/social", socialHandler)

	// EPK / press-kit routes
	r.Mount("/api/v1/epk", epkHandler)

	// Gig tracker routes
	r.Mount("/api/v1/gigs", gigHandler)

	// Venue database routes
	r.Mount("/api/v1/venues", venueHandler)

	// Contact database routes
	r.Mount("/api/v1/contacts", contactHandler)

	// Finance: billing profile, invoices, payments, agreements, email.
	// finance.Mux parses the full /api/v1/finance/... path itself.
	if financeHandler != nil {
		r.Mount("/api/v1/finance", financeHandler)
	}

	if cfg.ServeFrontend {
		r.NotFound(frontendHandler(cfg.NuxtInternalURL))
	}

	return r
}
