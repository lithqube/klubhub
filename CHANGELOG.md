# Changelog

All notable changes to KlubHub DJ are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/) and the
project adheres to [Semantic Versioning](https://semver.org/).

## [1.0.0] - 2026-09-16

### Hardening for the first public release

This is the first publicly consumable cut of KlubHub DJ. The feature
set mirrors `docs/v1-release-plan.md`; this entry records only the
shipping hardening work that happened between the last feature
branch and the v1.0.0 tag.

#### Added

- Three GitHub Actions workflows under `.github/workflows/`:
  - `ci.yml` — lint, typecheck, unit tests, govulncheck, pnpm audit.
  - `scan.yml` — gitleaks secret scan + CodeQL SAST (Go and JS/TS).
  - `image.yml` — arm64 image publish to GHCR on tag, with a
    post-build sanity check that fails the job if any non-arm64
    platform is present in the published manifest.
- `.dockerignore` at root, `api/`, and `apps/dj/`, scoping the build
  context so tracked secrets (`.env`, `garage.toml`, `backups/`)
  cannot accidentally end up in a published image layer.
- `apps/dj/server/api/v1/gigs/calendar.ics.get.ts` and
  `apps/dj/server/api/v1/gigs/[id]/pdf.get.ts` now refuse to run
  in production without `NUXT_PUBLIC_API_BASE` (returns 503).
- `scripts/backup.sh`, `scripts/restore.sh` rewritten to fail closed,
  use standard AWS-CLI env vars, and run with the prod compose
  file/project.
- `scripts/garage-bootstrap.sh` (new, idempotent).
- Testcontainers integration coverage for the complete Gig lifecycle against
  PostgreSQL 16 with all embedded Goose migrations applied.
- Bruno API smoke collection under `tests/bruno/` with 11 requests covering
  health, settings, tracklists, gigs/calendar security, EPK, and social APIs.

#### Changed

- `api/Dockerfile` runtime base pinned by digest
  (`gcr.io/distroless/static-debian12:nonroot@sha256:52dcfbab…`).
- `cmd/api/main.go` splits `main()` and `run()`, adds `-healthcheck`
  CLI flag, wires `srv.Shutdown` with a bounded `SHUTDOWN_TIMEOUT_SEC`
  (default 30 s).
- `gig.Handler` validates calendar/PDF bearer tokens via
  `crypto/subtle.ConstantTimeCompare` against `config.ICALSecret`
  (env-injected; required). The legacy placeholder literal
  `ICAL_SECRET` is rejected.
- `social` package: server-side state store (single-use, 10-minute
  TTL, cookie-bound), sanitized transport errors
  (`SanitizeTransportError`), and zero-write guarantees on every
  OAuth failure path.
- `platform/http.Middleware` enforces a 50 MiB absolute request ceiling;
  JSON/media handlers enforce tighter route-specific caps. All `http.Server` timeouts
  (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`,
  `IdleTimeout`) are set from config.
- `docker-compose.yml` now declares `stop_grace_period` for the api
  and frontend services, matches the API's `SHUTDOWN_TIMEOUT_SEC`.
- Garage is the sole supported object-storage service. Runtime configuration
  uses `S3_*`; legacy `MINIO_*` aliases were removed. `minio-go` remains only
  as the S3 protocol client library.
- `apps/dj/nuxt.config.ts` removes `icalSecret` from
  `runtimeConfig.public` (the secret is server-side only) and
  refuses to build a production image without `NUXT_PUBLIC_API_BASE`.
- `apps/dj/server/api/v1/epk/content.put.ts` no longer fake-200s
  in production; the entire directory is renamed to `v1.mock/` and
  excluded from production bundles via the `build:before` hook
  (Plan D).

#### Fixed

- `internal/platform/migrations/006..010_*.sql` — Go migrations
  had been suffixed `005b..005f_*.sql`; Goose's integer parse
  rejects nonnumeric prefixes, so the API crashed on startup.
  Renumbered to contiguous `006..010`.
- API healthcheck no longer uses `wget` against a scratch image
  (the runtime is distroless). Switched to a CLI flag
  `-healthcheck` invoked from compose as `CMD ["-healthcheck"]`.
- MusicBrainz client uses bounded body reads; replaced unbounded
  `http.Get` with `artworkClient` (timeout, no redirects, 5 MiB
  cap).
- `scripts/{backup,restore}.sh` no longer rely on `--no-verify-ssl`,
  hard-coded `--access-key` / `--secret-key` flags, or `|| true`
  error suppression.
- Manual artwork URLs now use Garage-compatible presigning rather than a
  hard-coded legacy `localhost:9000` URL.

#### Security

- Two security bugs caught in code review of B.4 — both fixed in the
  same PR:
  - DoS via missing-cookie state consumption.
  - Sanitization bypass in the 429 worker branch.
- Calendar/PDF tokens were a fixed literal in source. Now
  env-injected, constant-time compared, and the legacy placeholder
  literal is rejected.

## [0.x] Internal milestones

Pre-public development cycles. See git history and `.planning/`.
