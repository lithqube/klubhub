// Command promoter is the KlubHub Promoter API.
//
//	promoter [serve]      run the HTTP API (default)
//	promoter migrate      apply schema migrations (schema-owner connection)
//	promoter bootstrap    create the instance organisation and first owner
//	promoter healthcheck  probe the local /api/v1/health (container HEALTHCHECK)
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"

	"github.com/klubhub/dj/api/internal/platform/auth"
	"github.com/klubhub/dj/api/internal/platform/authz"
	platformconfig "github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	applog "github.com/klubhub/dj/api/internal/platform/log"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/config"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/identity"
	"github.com/klubhub/dj/api/internal/promoter/migrations"
	"github.com/klubhub/dj/api/internal/promoter/server"
)

// version is set at build time (-ldflags -X main.version).
var version = "dev"

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "migrate":
		err = migrate()
	case "bootstrap":
		err = bootstrap(args)
	case "healthcheck":
		err = healthcheck()
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("unknown command %q (serve | migrate | bootstrap | healthcheck)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "promoter:", err)
		os.Exit(1)
	}
}

type runtime struct {
	cfg    *config.Config
	log    zerolog.Logger
	pool   *pgxpool.Pool
	db     *tenantdb.DB
	keys   *envelope.Keyring
	engine *authz.Engine
}

// open loads config and connects as the restricted runtime role. It refuses
// to continue if that role could bypass row level security.
func open(ctx context.Context) (*runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	log := applog.New(cfg.LogLevel)
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := tenantdb.AssertRuntimeRole(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	keks, err := cfg.KEKs()
	if err != nil {
		pool.Close()
		return nil, err
	}
	engine, err := authz.New(ctx)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &runtime{
		cfg: cfg, log: log, pool: pool, db: tenantdb.New(pool),
		keys: envelope.NewKeyring(keks[0], keks[1:]...), engine: engine,
	}, nil
}

func serve() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	rt, err := open(ctx)
	if err != nil {
		return err
	}
	defer rt.pool.Close()

	deps := server.Deps{
		Log: rt.log, DB: rt.db, Authz: rt.engine, Origins: rt.cfg.Origins(),
		Events:        event.NewHandler(event.NewService(rt.db, rt.keys, nil)),
		ServeFrontend: rt.cfg.ServeFrontend, NuxtURL: rt.cfg.NuxtInternalURL,
	}
	switch rt.cfg.AuthProvider {
	case "local":
		svc := identity.NewService(rt.db, rt.keys, identity.Options{})
		deps.Authn, deps.Identity = svc, identity.NewHandler(svc)
	case "zitadel":
		jwks := rt.cfg.ZitadelJWKSURL
		if jwks == "" {
			jwks = strings.TrimSuffix(rt.cfg.ZitadelIssuer, "/") + "/oauth/v2/keys"
		}
		keys, err := auth.RemoteKeySet(ctx, jwks)
		if err != nil {
			return fmt.Errorf("zitadel keys: %w", err)
		}
		z, err := auth.NewZitadel(auth.ZitadelConfig{Issuer: rt.cfg.ZitadelIssuer, Audience: rt.cfg.ZitadelAudience, Keys: keys})
		if err != nil {
			return err
		}
		deps.Authn = z
	}
	mux, _ := server.New(deps)

	if rt.cfg.NATSServers != "" {
		stopRelay, err := startRelay(ctx, rt)
		if err != nil {
			return err
		}
		defer stopRelay()
	} else {
		rt.log.Warn().Msg("PROMOTER_NATS_SERVERS not set: events stay in the outbox (development only)")
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort(rt.cfg.BindAddress, rt.cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		rt.log.Info().Str("addr", srv.Addr).Str("auth", rt.cfg.AuthProvider).Msg("promoter api listening")
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// startRelay connects to NATS with the relay's nsc credentials, provisions
// the streams and moves committed outbox rows to JetStream.
func startRelay(ctx context.Context, rt *runtime) (func(), error) {
	pool, err := pgxpool.New(ctx, rt.cfg.RelayDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("relay database: %w", err)
	}
	nc, err := events.Connect(rt.cfg.NATSServers, rt.cfg.NATSCredsFile, "promoter-outbox-relay")
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("nats: %w", err)
	}
	js, err := jetstream.New(nc)
	if err == nil {
		err = events.EnsureStreams(ctx, js)
	}
	if err != nil {
		nc.Close()
		pool.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	relay := &events.Relay{Pool: pool, Publisher: events.JetStreamPublisher{JS: js}, Log: rt.log}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := relay.Run(ctx); err != nil {
			rt.log.Error().Err(err).Msg("outbox relay stopped")
		}
	}()
	return func() {
		<-done
		_ = nc.Drain()
		pool.Close()
	}, nil
}

// migrate needs only the schema-owner connection (and optional role
// passwords), so the one-shot migration container never holds the KEK.
func migrate() error {
	if err := platformconfig.LoadFileSecrets(); err != nil {
		return err
	}
	dsn := os.Getenv("PROMOTER_MIGRATE_DATABASE_URL")
	if dsn == "" {
		return errors.New("PROMOTER_MIGRATE_DATABASE_URL (schema owner) is required for migrate")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := migrations.Up(ctx, db); err != nil {
		return err
	}
	// Runtime roles get LOGIN and their passwords from secrets, never from a
	// migration file (plan §13.3). format(%L) quotes the literal server-side.
	for role, env := range map[string]string{"klubhub_app": "PROMOTER_APP_DB_PASSWORD", "klubhub_relay": "PROMOTER_RELAY_DB_PASSWORD"} {
		pw := os.Getenv(env)
		if pw == "" {
			continue
		}
		if len(pw) < 24 {
			return fmt.Errorf("%s must be at least 24 characters", env)
		}
		var stmt string
		if err := db.QueryRowContext(ctx, `SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', $1::text, $2::text)`, role, pw).Scan(&stmt); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("set %s password: %w", role, err)
		}
	}
	fmt.Println("promoter: migrations applied")
	return nil
}

func bootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	orgName := fs.String("org-name", "", "organisation / collective name")
	slug := fs.String("slug", "", "url-safe slug (a-z, 0-9, -)")
	tz := fs.String("timezone", "UTC", "IANA timezone, e.g. Europe/Berlin")
	currency := fs.String("currency", "EUR", "ISO 4217 currency")
	email := fs.String("owner-email", "", "first owner's email")
	name := fs.String("owner-name", "", "first owner's display name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	rt, err := open(ctx)
	if err != nil {
		return err
	}
	defer rt.pool.Close()
	if rt.cfg.AuthProvider != "local" {
		return errors.New("bootstrap is for self-hosted (local) identity; SaaS organisations come from Zitadel")
	}
	svc := identity.NewService(rt.db, rt.keys, identity.Options{})
	token, err := svc.Bootstrap(ctx, identity.BootstrapInput{
		OrgName: *orgName, Slug: *slug, Timezone: *tz, Currency: *currency, OwnerEmail: *email, OwnerName: *name,
	})
	if err != nil {
		return err
	}
	// The token travels in the URL fragment, which browsers never send to
	// servers or in Referer headers.
	fmt.Printf("Organisation created. Open this one-time link within 24 hours to set the owner password:\n\n  %s/setup#token=%s\n\n",
		rt.cfg.Origins()[0], token)
	return nil
}

func healthcheck() error {
	port := os.Getenv("PROMOTER_PORT")
	if port == "" {
		port = "8081"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/api/v1/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %d", resp.StatusCode)
	}
	return nil
}
