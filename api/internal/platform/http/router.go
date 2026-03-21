package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/storage"
	"github.com/rs/zerolog"
)

// NewRouter constructs a chi router with all routes wired.
// settingsHandler handles both GET and PUT /api/v1/settings.
// tracklistHandler handles tracklist CRUD operations.
// socialHandler handles social scheduling routes under /api/v1/social.
func NewRouter(
	cfg *config.Config,
	pool *pgxpool.Pool,
	store *storage.Client,
	log zerolog.Logger,
	settingsHandler http.Handler,
	tracklistHandler http.Handler,
	socialHandler http.Handler,
) *chi.Mux {
	r := chi.NewRouter()

	// Built-in middleware
	r.Use(chiMiddleware.Recoverer)
	r.Use(RequestLogger(log))

	healthHandler := NewHealthHandler(pool, store, cfg)

	r.Get("/api/v1/health", healthHandler.ServeHTTP)

	r.Get("/api/v1/settings", settingsHandler.ServeHTTP)
	r.Put("/api/v1/settings", settingsHandler.ServeHTTP)

	// Tracklist routes
	r.Mount("/api/v1/tracklists", tracklistHandler)

	// Social scheduling routes
	r.Mount("/api/v1/social", socialHandler)

	return r
}
