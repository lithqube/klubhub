package main

import (
	"context"
	"database/sql"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/db"
	applog "github.com/klubhub/dj/api/internal/platform/log"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/platform/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Use basic stderr output before logger is initialized
		os.Stderr.WriteString("failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	logger := applog.New(cfg.LogLevel)

	ctx := context.Background()

	// Open a *sql.DB alongside the pgxpool — goose requires *sql.DB.
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to open sql.DB for migrations")
	}
	defer sqlDB.Close()

	if err := migrations.RunMigrations(sqlDB); err != nil {
		logger.Fatal().Err(err).Msg("failed to run migrations")
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	storeClient, err := storage.New(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create storage client")
	}

	if err := storeClient.EnsureBucket(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to ensure storage bucket exists")
	}

	logger.Info().
		Str("bind", cfg.BindAddress).
		Str("port", cfg.Port).
		Msg("platform initialized — HTTP server not yet wired (Plan 03)")
}
