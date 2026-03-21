package main

import (
	"context"
	"database/sql"
	nethttp "net/http"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/klubhub/dj/api/internal/artwork"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/db"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	applog "github.com/klubhub/dj/api/internal/platform/log"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/platform/storage"
	"github.com/klubhub/dj/api/internal/settings"
	"github.com/klubhub/dj/api/internal/social"
	"github.com/klubhub/dj/api/internal/tracklist"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString("failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	logger := applog.New(cfg.LogLevel)

	ctx := context.Background()

	// 1. Open sql.DB for goose migrations (goose requires *sql.DB).
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to open sql.DB for migrations")
	}
	defer sqlDB.Close()

	// 2. Run migrations before anything else touches the DB.
	if err := migrations.RunMigrations(sqlDB); err != nil {
		logger.Fatal().Err(err).Msg("failed to run migrations")
	}

	// 3. Open pgxpool for application use.
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	// 4. Initialise MinIO storage client and ensure bucket exists.
	storeClient, err := storage.New(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create storage client")
	}

	if err := storeClient.EnsureBucket(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to ensure storage bucket exists")
	}

	// 5. Wire settings module.
	settingsRepo := settings.NewRepository(pool)
	settingsSvc := settings.NewService(settingsRepo)
	settingsHandler := settings.NewHandler(settingsSvc)

	// 6. Wire tracklist module with artwork service.
	tracklistRepo := tracklist.NewRepository(pool)

	// Build artwork clients (nil-safe when API keys absent)
	var spotifyClient artwork.SpotifySearcher
	if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
		spotifyClient = artwork.NewSpotifyClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret)
	}

	var discogsClient artwork.DiscogsSearcher
	if cfg.DiscogsAPIKey != "" {
		discogsClient = artwork.NewDiscogsClient(cfg.DiscogsAPIKey)
	}

	musicbrainzClient := artwork.NewMusicBrainzClient("KlubHub-DJ/1.0 (admin@klubhub.io)")

	// Build artwork cache with MinIO
	artworkCache := artwork.NewArtworkCache(storeClient, cfg.MinioBucket, cfg.MinioPublicEndpoint)

	// Build fetch chain
	fetchChain := artwork.NewFetchChain(spotifyClient, discogsClient, musicbrainzClient, artworkCache, "placeholder://default")

	// Build artwork service
	artworkSvc := artwork.NewService(fetchChain, tracklistRepo, "placeholder://default")

	// Build tracklist service with artwork wired
	tracklistSvc := tracklist.NewService(tracklistRepo, storeClient, artworkSvc, tracklist.ServiceConfig{
		NuxtInternalURL: cfg.NuxtInternalURL,
		StorageBucket:   cfg.MinioBucket,
		MaxUploadBytes:  10 * 1024 * 1024, // 10MB
	})
	tracklistHandler := tracklist.NewHandler(tracklistSvc)

	// 7. Wire social module.
	socialRepo := social.NewRepository(pool)
	socialSvc := social.NewService(socialRepo, social.ServiceConfig{
		InstagramAppID:       cfg.InstagramClientID,
		InstagramAppSecret:   cfg.InstagramClientSecret,
		InstagramRedirectURI: cfg.InstagramRedirectURI,
		TokenEncryptionKey:   []byte(cfg.TokenEncryptionKey),
	})
	socialHandler := social.NewHandler(socialSvc)

	// 8. Build router (internal http package aliased as apphttp).
	router := apphttp.NewRouter(cfg, pool, storeClient, logger, settingsHandler, tracklistHandler.Routes(), socialHandler.Routes())

	// 7. Start HTTP server.
	addr := cfg.BindAddress + ":" + cfg.Port
	logger.Info().
		Str("addr", addr).
		Msg("server starting")

	srv := &nethttp.Server{
		Addr:    addr,
		Handler: router,
	}

	if err := srv.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
		logger.Fatal().Err(err).Msg("server stopped unexpectedly")
	}
}
