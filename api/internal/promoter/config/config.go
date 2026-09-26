// Package config loads KlubHub Promoter settings from the environment.
// Every secret also accepts NAME_FILE (Docker secrets / rendered Infisical
// env files, plan D7). The process refuses to start on missing or weak
// security settings instead of falling back to insecure defaults.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kelseyhightower/envconfig"

	platformconfig "github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/envelope"
)

// Config holds Promoter settings (env prefix PROMOTER_).
type Config struct {
	// Runtime connection as the restricted klubhub_app role.
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	// Schema-owner connection, used only by `promoter migrate`/`bootstrap`.
	MigrateDatabaseURL string `envconfig:"MIGRATE_DATABASE_URL"`

	BindAddress string `envconfig:"BIND_ADDRESS" default:"127.0.0.1"`
	Port        string `envconfig:"PORT" default:"8081"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`

	// Browser origin(s) of the Promoter UI, comma-separated; CSRF allowlist
	// and the base of setup/invite links.
	PublicOrigin string `envconfig:"PUBLIC_ORIGIN" required:"true"`

	// Deployment key-encryption key: base64 of 32 bytes, plus its id.
	KEK   string `envconfig:"KEK" required:"true"`
	KEKID string `envconfig:"KEK_ID" required:"true"`
	// Previous KEKs during rotation: "id:base64,id:base64".
	PreviousKEKs string `envconfig:"PREVIOUS_KEKS"`

	// Identity provider: "local" (self-host) or "zitadel" (SaaS).
	AuthProvider    string `envconfig:"AUTH_PROVIDER" default:"local"`
	ZitadelIssuer   string `envconfig:"ZITADEL_ISSUER"`
	ZitadelAudience string `envconfig:"ZITADEL_AUDIENCE"`
	ZitadelJWKSURL  string `envconfig:"ZITADEL_JWKS_URL"`

	// NATS (plan D8). Empty servers disables the outbox relay (dev only).
	NATSServers   string `envconfig:"NATS_SERVERS"`
	NATSCredsFile string `envconfig:"NATS_CREDS_FILE"`
	// Outbox relay connection as klubhub_relay (required with NATS).
	RelayDatabaseURL string `envconfig:"RELAY_DATABASE_URL"`

	ServeFrontend   bool   `envconfig:"SERVE_FRONTEND" default:"false"`
	NuxtInternalURL string `envconfig:"NUXT_INTERNAL_URL" default:"http://127.0.0.1:3001"`
}

// Load reads PROMOTER_* variables (after resolving *_FILE secrets).
func Load() (*Config, error) {
	if err := platformconfig.LoadFileSecrets(); err != nil {
		return nil, err
	}
	var c Config
	if err := envconfig.Process("PROMOTER", &c); err != nil {
		return nil, err
	}
	return &c, c.Validate()
}

// Validate checks security-relevant settings.
func (c *Config) Validate() error {
	if _, err := c.KEKs(); err != nil {
		return err
	}
	if len(c.Origins()) == 0 {
		return errors.New("config: PROMOTER_PUBLIC_ORIGIN must list at least one http(s) origin")
	}
	if c.NATSServers != "" && c.RelayDatabaseURL == "" {
		return errors.New("config: PROMOTER_NATS_SERVERS requires PROMOTER_RELAY_DATABASE_URL (klubhub_relay)")
	}
	switch c.AuthProvider {
	case "local":
	case "zitadel":
		if c.ZitadelIssuer == "" || c.ZitadelAudience == "" {
			return errors.New("config: zitadel requires PROMOTER_ZITADEL_ISSUER and PROMOTER_ZITADEL_AUDIENCE")
		}
	default:
		return fmt.Errorf("config: unknown PROMOTER_AUTH_PROVIDER %q", c.AuthProvider)
	}
	return nil
}

// Origins parses PublicOrigin into normalised scheme://host origins.
func (c *Config) Origins() []string {
	var out []string
	for _, raw := range strings.Split(c.PublicOrigin, ",") {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			continue
		}
		out = append(out, u.Scheme+"://"+u.Host)
	}
	return out
}

// KEKs returns the current KEK followed by previous ones.
func (c *Config) KEKs() ([]*envelope.KEK, error) {
	cur, err := parseKEK(c.KEKID, c.KEK)
	if err != nil {
		return nil, fmt.Errorf("config: PROMOTER_KEK: %w", err)
	}
	keks := []*envelope.KEK{cur}
	for _, pair := range strings.Split(c.PreviousKEKs, ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		id, b64, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if !ok {
			return nil, errors.New("config: PROMOTER_PREVIOUS_KEKS must be id:base64 pairs")
		}
		k, err := parseKEK(id, b64)
		if err != nil {
			return nil, fmt.Errorf("config: previous KEK %q: %w", id, err)
		}
		keks = append(keks, k)
	}
	return keks, nil
}

func parseKEK(id, b64 string) (*envelope.KEK, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, errors.New("must be standard base64")
	}
	defer clear(raw)
	return envelope.NewKEK(id, raw)
}
