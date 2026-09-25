# Changelog

All notable changes to KlubHub DJ are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/) and the
project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Phase 5 — Finance Tracker

Additive only. No breaking changes to existing endpoints. See
[`docs/release-notes/v1.1.0-phase5.md`](docs/release-notes/v1.1.0-phase5.md)
for the full surface (lifecycle diagrams, API curl examples, schema
graph, configuration matrix, mock-data policy, open production gates).

#### Added

- **Billing identity** (`billing_profiles`): singleton legal entity
  record (legal_name, tax_id, address, payment_instructions,
  default_currency). Snapshotted onto issued invoices as
  `billing_profile_snapshot` (JSONB).
- **Invoices**: draft → issued → paid | cancelled | corrected.
  Numbered per-(DJ, currency) via `invoice_number_sequences` with
  `FOR UPDATE` locking. Line items on a child table (`invoice_lines`)
  with automatic gig fee + extras population. PDF rendering via the
  pure-Go [`gofpdf`](https://github.com/jung-kurt/gofpdf) library.
- **Payments**: deposit / payment / refund kinds; pending / completed
  / failed / refunded statuses; sum-by-invoice aggregation used by the
  "auto-pay on total reached" derived state.
- **Documents**: persistent store behind a swappable `ObjectStore`
  interface; default backend is the existing **Garage S3** container;
  SHA-256 integrity check; auto-incrementing version per
  (owner_type, owner_id) with partial-unique-index for `is_current`.
- **Agreements**: versioned templates + per-gig instances with
  `draft → sent → signed | declined | expired` lifecycle and snapshot
  of template text at instance creation time.
- **Email outbox** (`email_messages`): durable queuing with retry,
  cron-style exponential backoff, status enum
  `queued | sending | sent | failed`.
- **Plunk transactional email** ([`useplunk/plunk`](https://github.com/useplunk/plunk)):
  `PlunkSender` posts to `POST {BaseURL}/api/v1/{ProjectID}/emails`
  with `Authorization: Bearer ***` and an `Idempotency-Key: <msg.ID>` header
  for retry dedupe. The same Go code works against hosted Plunk or a
  self-hosted instance — only `PLUNK_BASE_URL` changes.
- **Self-hosted Plunk overlay**: opt-in
  [`docker-compose.email.yml`](docker-compose.email.yml) adds
  `ghcr.io/useplunk/plunk:latest` under the `email` profile with a
  dedicated Postgres database on the stack's existing `db` service.
  [`scripts/plunk-bootstrap.sh`](scripts/plunk-bootstrap.sh)
  provisions `secrets/email.env` (gitignored, 0600). Documented under
  [`docs/SELF-HOSTING.md`](docs/SELF-HOSTING.md#optional-self-hosted-email-plunk).
- **Per-currency dashboard endpoint**: `GET /api/v1/finance/invoices/summaries`
  returns issued / paid / outstanding / deposits / refunds / minimum
  balance grouped by currency.

#### Added (frontend)

- Verification targets updated to include the Phase 5 finance routes
  in [`apps/dj/app/utils/__tests__/verification-targets.test.ts`](apps/dj/app/utils/__tests__/verification-targets.test.ts).
- Frontend mocks for Phase 5 surfaces stay in dev/staging builds and
  are excluded from `apps/dj/.output/`.

#### Backend (Go)

- 8 new migrations under `api/internal/platform/migrations/`:
  `011_billing_profiles.sql`, `012_invoices.sql`,
  `013_invoice_lines.sql`, `014_payments.sql`, `015_documents.sql`,
  `016_agreements.sql`, `017_email.sql`.
- New package `api/internal/finance/` (~3,500 lines + tests)
  composed of layer files (`model.go`, `repository.go`, `service.go`,
  `handler.go`) per sub-resource plus shared `mux.go`, `pdf_renderer.go`,
  `plunk_sender.go`, and a per-feature `*_contract_test.go`.
- New config: `PLUNK_BASE_URL`, `PLUNK_PROJECT_ID`, `PLUNK_API_KEY`,
  `PLUNK_API_KEY_FILE`, `PLUNK_FROM_EMAIL`, `PLUNK_FROM_NAME`,
  `DOCUMENT_STORE_BUCKET`.
- Dependency: `github.com/jung-kurt/gofpdf v1.16.2`.

#### Verification

- `pnpm nx run api:test` → all 14 packages green; finance at `-race` (79 tests).
- `pnpm nx test @dev/dj` → 196/196 Vitest passing.
- `pnpm nx run @dev/dj:typecheck` → Phase 5 surface: 0 errors
  (scripts/typecheck-phase5.sh).
- `python3 -m unittest discover -s scripts -p 'test_stack_setup.py'`
  → 5/5, includes new
  `test_email_overlay_wires_plunk_when_email_env_present`.

#### Open production gates (documented; out of Phase 5 scope)

- Jurisdiction + tax treatment per gig type (deposit tax, travel
  exemption, EU VAT B2B reverse-charge wording).
- Approved agreement wording (legal review per jurisdiction).
- Plunk API key + sender domain verification (DKIM/SPF/DMARC).
- Garage adapter wiring for `finance.NewDocumentService` (skeleton
  already in place; needs final `*minio.Client` signature bridge).
- CI gate that asserts no `mock:`-prefixed fixtures reach
  `apps/dj/.output/`.

## [1.0.1] - 2026-09-19

### Runtime, CI, and onboarding fixes since v1.0.0

This release hardens the runtime contract, the CI pipeline, and the
newsletter onboarding flow. No breaking changes. The Compose runtime,
GHCR image name (`ghcr.io/lithqube/klubhub-dj-api`), and the static site
contract are unchanged.

#### Fixed

- **Compose runtime**: renamed the service `api` → `app` and rewired
  the dev/prod contract. The dev compose binds `0.0.0.0:8080` inside
  the container and maps to host `127.0.0.1:8080:8080`; the prod
  compose pulls the per-product image `ghcr.io/lithqube/klubhub-dj-api`
  and serves the frontend at `/` (`SERVE_FRONTEND=true`). Dev and prod
  use distinct compose project names, distinct volumes, and no fixed
  `container_name` so both can coexist on the same host.
  (PR #5, commit `3842419`)
- **GitHub Pages build**: the `build` job now binds to the
  `github-pages` environment so it can read `PLUNK_PUBLIC_KEY` from
  environment secrets. PR builds skip the secret and use the inline
  placeholder `pk_ci_pr_preview_only_do_not_subscribe`. Fixes the
  empty-`PLUNK_PUBLIC_KEY` failure observed on the post-merge workflow
  run. (PR #9, commit `e40ea7f`)
- **Plunk welcome workflow**: the static site's newsletter signup now
  fires `event: 'klubhub.subscribed'` (a namespaced custom event)
  instead of relying on Plunk's documented `contact.subscribed` system
  event, which does not fire on contact upserts created via
  `POST /v1/track`. The Plunk workflow's trigger event name must be
  renamed to `klubhub.subscribed` in the Plunk UI for the welcome to
  fire. (PR #11, commit `1290340`)
- **Newsletter signup mobile layout**: the static site newsletter
  form is now responsive on mobile viewports. (PR #10, commit
  `c1549b3`)
- **GitHub Pages site copy**: replaced em dashes with commas in
  recipient-facing copy on the static site (page title, og:title,
  newsletter lede) for cleaner rendering across older email and
  RSS clients. (PR #12, commit `afb468c`)
- **Plunk newsletter key wiring**: the static site fails closed if
  `PLUNK_PUBLIC_KEY` is missing, placeholder, or the wrong prefix
  (`sk_*`). The PK is injected into `<meta name="plunk-public-key">`
  at build time; the SK is mounted as a Docker secret consumed via
  `PLUNK_SECRET_KEY_FILE` and never enters the repo. (PR #7, commit
  `8749bb4`)
- **Module contracts wired to the existing product**: the gig, epk,
  tracklist, and social modules now talk to the live Go API instead
  of local mocks. `apps/dj/app/stores/{gig,epk,tracklist,social}.ts`
  use the real fetch endpoints; `apps/dj/app/composables/useTracklist.ts`
  uses the live tracklist contract; `apps/dj/app/pages/{index,finance,social}.vue`
  consume the real APIs. New contract tests at
  `api/internal/{gig,epk,tracklist}/contract_test.go` cover the
  handler-to-service boundary. Followup commit `8fee098` hardens the
  gig service to return 404 on `ErrNotFound`, the social store to map
  snake_case responses, and the gig detail query to join venues and
  contacts in a single round-trip. (PR #13, commits `f5fce61`,
  `8fee098`)

#### Changed

- Default production image tag in `docker-compose.prod.yml` bumped
  from `v1.0.0` to `v1.0.1`. Operators override with `IMAGE_TAG`.
- `package.json` version bumped to `1.0.1`.
- `scripts/release-day-check.sh` default `KH_VER` bumped to `v1.0.1`.
- `scripts/test_stack_setup.py` assertion updated to pin the new
  default tag (`v1.0.1`).

#### Known gaps (not blockers)

- **GHCR publish for `v1.0.1`** is not yet complete at the time of
  this entry. The default `IMAGE_TAG=v1.0.1` will pull successfully
  only after `ghcr.io/lithqube/klubhub-dj-api:v1.0.1` is published.
  Triggered via `gh workflow run containers.dj.yml --ref main -f
  tag=v1.0.1` post-merge.
- **Plunk workflow trigger rename**: an operator action in the Plunk
  UI is required to rename the trigger event name from
  `contact.subscribed` to `klubhub.subscribed`. Zero executions have
  run, so the rename is allowed.
- **Plunk onboarding template body**: the live Plunk copy has a
  stray `;` in its footer paragraph that the local repo file does
  not. Edit the Plunk copy to match `apps/site/emails/onboarding.html`.

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
