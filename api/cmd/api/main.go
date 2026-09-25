package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/klubhub/dj/api/internal/artwork"
	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/epk"
	"github.com/klubhub/dj/api/internal/finance"
	"github.com/klubhub/dj/api/internal/gig"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/db"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	applog "github.com/klubhub/dj/api/internal/platform/log"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/klubhub/dj/api/internal/platform/storage"
	"github.com/klubhub/dj/api/internal/ra"
	"github.com/klubhub/dj/api/internal/settings"
	"github.com/klubhub/dj/api/internal/social"
	"github.com/klubhub/dj/api/internal/tracklist"
	"github.com/klubhub/dj/api/internal/venue"
	"github.com/rs/zerolog"
)

// healthcheckTimeout bounds the CLI `-healthcheck` probe so a hung server
// still exits with a clear status within a known time window.
const healthcheckTimeout = 5 * time.Second

// Package identifier. Each KlubHub product builds its own binary under a
// distinct image (e.g. ghcr.io/lithqube/klubhub-dj-api). The product name is
// injected at build time so `/api -version` reports both the product and
// the released version in a single line.
//
// Build via:
//
//	-ldflags="-X main.productName=KlubHub-DJ -X main.version=1.2.3"
//
// for release builds. The defaults below apply when no ldflags are passed.
var productName = "KlubHub-DJ"
var version = "1.0.0"

func main() {
	// Plan B.2 — `-healthcheck` is the JSON-array-friendly form used by
	// the distroless runtime image which has no shell, no wget, no curl.
	// The flag must work in the JSON-array ENTRYPOINT form (i.e. as the first
	// argument to `/api`), so we accept both `-healthcheck` and `-healthcheck=true`.
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // do not pollute stdout when used in containers

	// Plan v1.0.0 — `-version` prints the binary version and exits 0.
	// Useful for `docker exec klubhub-storage /api -version` and the
	// GHCR image manifest sanity check.
	printVersion := fs.Bool("version", false, "print the binary version and exit")
	healthcheck := fs.Bool("healthcheck", false, "probe /api/v1/health and exit 0 on 200, non-zero otherwise")
	if err := fs.Parse(os.Args[1:]); err != nil {
		// flag.ContinueOnError returns ErrHelp on -h/-help; treat that as
		// a non-fatal success so `docker run /api -help` does not crash.
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(os.Stderr, "invalid flags: %v\n", err)
			os.Exit(2)
		}
	}
	if *printVersion {
		// Emit "<product> <version>" so operators can identify which
		// KlubHub product (dj, promoter, label, ...) a given binary
		// belongs to alongside its released version.
		fmt.Printf("%s %s\n", productName, version)
		os.Exit(0)
	}
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// gracefulShutdown orchestrates graceful shutdown of the HTTP server and the
// publish worker goroutine. It bounds the total time at `timeout`:
//   - srv.Shutdown(ctx) stops accepting new connections and waits for
//     in-flight requests to finish.
//   - After the server drains, we wait for workerDone (signal-driven exit
//     of the background publish worker) using the same budget.
//
// If the budget elapses, srv.Shutdown returns ctx.Err() and we
// force-close to release keep-alive sockets so the container can exit.
//
// This is the single, testable seam for graceful-shutdown behavior. The
// production main() passes the real *http.Server, the real worker done
// channel returned by social.StartPublishWorker, and the
// SHUTDOWN_TIMEOUT_SEC-derived timeout. Tests can plug in a tiny
// httptest.Server and a fake worker done channel.
func gracefulShutdown(srv *nethttp.Server, workerDone <-chan struct{}, timeout time.Duration, logger zerolog.Logger) error {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("graceful shutdown timed out; forcing close")
		_ = srv.Close()
		return err
	}
	logger.Info().Msg("http server drained cleanly")

	select {
	case <-workerDone:
		logger.Info().Msg("publish worker stopped")
	case <-shutdownCtx.Done():
		logger.Warn().Msg("publish worker did not stop within shutdown budget; exiting anyway")
		return shutdownCtx.Err()
	}
	return nil
}

// run is the testable entry point for the API server. It returns an error
// instead of calling os.Exit so tests can assert on failure paths.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := applog.New(cfg.LogLevel)

	// Root context cancelled on SIGTERM/SIGINT for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// 1. Open sql.DB for goose migrations (goose requires *sql.DB).
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open sql.DB: %w", err)
	}
	defer sqlDB.Close()

	// 2. Run migrations before anything else touches the DB.
	if err := migrations.RunMigrations(sqlDB); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// 3. Open pgxpool for application use.
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	// 4. Initialise the Garage S3 client and ensure the bucket exists.
	storeClient, err := storage.New(cfg)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}

	if err := storeClient.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("ensure storage bucket: %w", err)
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

	// Build artwork cache against Garage's S3 API.
	artworkCache := artwork.NewArtworkCache(storeClient, cfg.S3Bucket, cfg.S3PublicEndpoint)

	// Build fetch chain
	fetchChain := artwork.NewFetchChain(spotifyClient, discogsClient, musicbrainzClient, artworkCache, "placeholder://default")

	// Build artwork service
	artworkSvc := artwork.NewService(fetchChain, tracklistRepo, "placeholder://default")

	// Build tracklist service with artwork wired
	tracklistSvc := tracklist.NewService(tracklistRepo, storeClient, artworkSvc, tracklist.ServiceConfig{
		NuxtInternalURL: cfg.NuxtInternalURL,
		StorageBucket:   cfg.S3Bucket,
		MaxUploadBytes:  10 * 1024 * 1024, // 10MB
	})
	tracklistHandler := tracklist.NewHandler(tracklistSvc)

	// 7. Wire social module.
	socialRepo := social.NewRepository(pool)
	socialStateStore := social.NewStateStore(10 * time.Minute)
	defer socialStateStore.Stop()
	socialSvc := social.NewServiceWithState(socialRepo, social.ServiceConfig{
		InstagramAppID:       cfg.InstagramClientID,
		InstagramAppSecret:   cfg.InstagramClientSecret,
		InstagramRedirectURI: cfg.InstagramRedirectURI,
		TokenEncryptionKey:   []byte(cfg.TokenEncryptionKey),
	}, socialStateStore)
	socialHandler := social.NewHandler(socialSvc, storeClient)

	// 7a. Wire and start the background publish worker. The returned
	// `workerDone` channel is closed when the worker goroutine has fully
	// exited; main() waits on it during shutdown so we don't race the
	// worker against connection teardown.
	igClient := social.NewInstagramClient([]byte(cfg.TokenEncryptionKey))
	publishWorker := social.NewWorker(socialRepo, igClient, storeClient, []byte(cfg.TokenEncryptionKey), logger)
	workerDone := social.StartPublishWorker(ctx, publishWorker)
	logger.Info().Msg("social publish worker started")

	// 7b. Watch for SIGTERM/SIGINT and orchestrate graceful shutdown.
	//
	// On signal:
	//   1. Stop accepting new HTTP connections (srv.Shutdown).
	//   2. Wait up to ShutdownTimeoutSec for in-flight requests to finish.
	//   3. Wait for the publish worker goroutine to drain (bounded by the
	//      same timeout budget).
	// If the budget elapses, srv.Shutdown returns ctx.Err() and we fall
	// through to srv.Close() in the main cleanup below so we don't block
	// the container's stop_grace_period indefinitely.
	stopSig := make(chan os.Signal, 1)
	signal.Notify(stopSig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stopSig)

	// 8. Wire EPK module.
	epkRepo := epk.NewRepository(pool)
	epkStorage := epk.NewStorageAdapter(storeClient)
	epkSettingsSvc := epk.NewSettingsServiceAdapter(settingsSvc)
	epkSvc := epk.NewService(epkRepo, epkStorage, epkSettingsSvc)
	epkHandler := epk.NewHandler(epkSvc, nil) // nil = create default RA client internally

	// 9. Wire gig, venue, and contact modules.
	gigRepo := gig.NewRepository(pool)
	venueRepo := venue.NewRepository(pool)
	contactRepo := contact.NewRepository(pool)
	gigStorage := gig.NewStorageAdapter(storeClient)

	gigSvc := gig.NewService(gigRepo, venueRepo, contactRepo, tracklistRepo, gigStorage)
	// Plan B.5: pass the calendar/PDF bearer secret from config into the
	// gig handler. The previous signature used a single argument and the
	// handler internally hard-coded the literal "ICAL_SECRET" as the
	// expected value, which meant any caller that supplied
	// `?secret=ICAL_SECRET` was accepted. The secret is now env-injected
	// (config.ICAL_SECRET, required) and the handler fails closed on
	// empty/missing/placeholder values.
	gigHandler := gig.NewHandler(gigSvc, cfg.ICALSecret)

	venueSvc := venue.NewService(venueRepo)
	venueHandler := venue.NewHandler(venueSvc)

	contactSvc := contact.NewService(contactRepo)
	contactHandler := contact.NewHandler(contactSvc)

	// The RA import routes live on the gig router but need the venue and
	// contact services. This must happen before gigHandler.Routes() is
	// called below; without it the routes were mounted on a nil handler and
	// every /gigs/info/{slug} and /gigs/import-ra request panicked.
	gigHandler.SetRAImportHandler(gig.NewRAImportHandler(gigSvc, venueSvc, contactSvc, ra.NewRAClient()))

	// 9a. Wire finance module (billing profile, invoices, payments,
	// agreements, email). Documents have no HTTP surface yet (nil → 503);
	// the document service is still used by agreements to store PDFs.
	financeHandler := buildFinanceHandler(cfg, pool, storeClient, logger)

	// 10. Build router (internal http package aliased as apphttp).
	router := apphttp.NewRouter(cfg, pool, storeClient, logger,
		settingsHandler,
		tracklistHandler.Routes(),
		socialHandler.Routes(),
		epkHandler.Routes(),
		gigHandler.Routes(),
		venueHandler.Routes(),
		contactHandler.Routes(),
		financeHandler,
	)

	// 11. Start HTTP server. Run ListenAndServe in a goroutine so main can
	// orchestrate graceful shutdown on SIGTERM/SIGINT without blocking
	// on Accept(). The bounded shutdown timeout is SHUTDOWN_TIMEOUT_SEC
	// (default 30s); align with the api service's `stop_grace_period` in
	// docker-compose.{yml,prod.yml} so the container doesn't SIGKILL us
	// before we finish draining.
	addr := cfg.BindAddress + ":" + cfg.Port
	logger.Info().
		Int("shutdown_timeout_sec", cfg.ShutdownTimeoutSec).
		Str("addr", addr).
		Msg("server starting")

	srv := &nethttp.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: time.Duration(cfg.HTTPReadHeaderTimeoutSec) * time.Second,
		ReadTimeout:       time.Duration(cfg.HTTPReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTPWriteTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(cfg.HTTPIdleTimeoutSec) * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Block until either the server fails on its own, or we receive a
	// stop signal (SIGTERM/SIGINT). Note: signal.NotifyContext at the top
	// already cancels ctx on signal, but we use the stopSig channel here
	// because we want a select against the in-flight server error too.
	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server stopped unexpectedly: %w", err)
		}
		return nil
	case sig := <-stopSig:
		logger.Info().Str("signal", sig.String()).Msg("shutdown signal received; draining")
	}

	// Graceful shutdown: stop accepting new connections, drain in-flight
	// requests, then wait for the publish worker to exit. The whole
	// sequence is bounded by cfg.ShutdownTimeoutSec (default 30s), which
	// should equal docker-compose's `stop_grace_period` for the api
	// service to avoid SIGKILL during drain.
	shutdownTimeout := time.Duration(cfg.ShutdownTimeoutSec) * time.Second
	if err := gracefulShutdown(srv, workerDone, shutdownTimeout, logger); err != nil {
		// Non-fatal from the container's POV: the process is exiting
		// anyway. Surface for logs/metrics but don't propagate (matches
		// the pre-B.3 behavior of returning nil when server closed).
		logger.Error().Err(err).Msg("shutdown completed with error")
	}

	return nil
}

// runHealthcheck performs a single GET /api/v1/health probe against the
// configured API address and returns 0 if the response is HTTP 200,
// non-zero otherwise. This is the binary-mode equivalent of `wget -q -O-`.
//
// The probe is run against the same BIND_ADDRESS/PORT the server uses, but
// over plain HTTP without TLS. The function is invoked by Docker's
// HEALTHCHECK CMD which has no shell, so it must self-contain all I/O.
func runHealthcheck() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: load config: %v\n", err)
		return 1
	}

	addr := cfg.BindAddress + ":" + cfg.Port
	url := fmt.Sprintf("http://%s/api/v1/health", addr)

	client := &nethttp.Client{
		Timeout: healthcheckTimeout,
	}

	req, err := nethttp.NewRequest(nethttp.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: build request: %v\n", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: request failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: unhealthy status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

// buildFinanceHandler composes the finance routes. Email is only mounted
// when Plunk is fully configured; otherwise /finance/emails answers 503
// instead of queuing mail that can never be delivered.
func buildFinanceHandler(cfg *config.Config, pool *pgxpool.Pool, storeClient *storage.Client, logger zerolog.Logger) *finance.Mux {
	billingSvc := finance.NewService(finance.NewRepository(pool))
	invoiceSvc := finance.NewInvoiceService(finance.NewInvoiceRepository(pool), billingSvc, finance.NewPGGigFeeProvider(pool))
	paymentSvc := finance.NewPaymentService(finance.NewPaymentRepository(pool))
	docSvc := finance.NewDocumentService(finance.NewDocumentRepository(pool), finance.NewStorageAdapter(storeClient), cfg.S3Bucket)
	tplRepo := finance.NewAgreementTemplateRepository(pool)
	tplSvc := finance.NewAgreementTemplateService(tplRepo)
	instSvc := finance.NewAgreementInstanceService(finance.NewAgreementInstanceRepository(pool), tplRepo, docSvc)

	var emailHandler nethttp.Handler
	if cfg.PlunkBaseURL != "" && cfg.PlunkProjectID != "" && cfg.PlunkAPIKey != "" {
		sender := finance.NewPlunkSender(finance.PlunkConfig{
			BaseURL:   cfg.PlunkBaseURL,
			ProjectID: cfg.PlunkProjectID,
			APIKey:    cfg.PlunkAPIKey,
			FromEmail: cfg.PlunkFromEmail,
			FromName:  cfg.PlunkFromName,
		})
		emailHandler = finance.NewEmailHandler(finance.NewEmailService(finance.NewEmailRepository(pool), sender))
		logger.Info().Str("plunk_base_url", cfg.PlunkBaseURL).Msg("finance email enabled (plunk)")
	} else {
		logger.Info().Msg("finance email disabled: PLUNK_BASE_URL, PLUNK_PROJECT_ID and PLUNK_API_KEY(_FILE) are not all set")
	}

	return finance.NewMux(
		finance.NewHandler(billingSvc),
		finance.NewInvoiceHandler(invoiceSvc),
		finance.NewPaymentHandler(paymentSvc),
		nil, // documents: no HTTP handler yet
		finance.NewAgreementTemplateHandler(tplSvc),
		finance.NewAgreementInstanceHandler(instSvc),
		emailHandler,
	)
}
