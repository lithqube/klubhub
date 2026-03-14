package main

import (
	"context"
	"database/sql"
	nethttp "net/http"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/db"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	applog "github.com/klubhub/dj/api/internal/platform/log"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/platform/storage"
	"github.com/klubhub/dj/api/internal/settings"
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

	// 6. Build router (internal http package aliased as apphttp).
	router := apphttp.NewRouter(cfg, pool, storeClient, logger, settingsHandler)

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
