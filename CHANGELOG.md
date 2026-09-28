# Changelog

All notable changes to KlubHub DJ are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/) and the
project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- README, `docs/INDEX.md` and `CLAUDE.md` synced to the current state
  of the repo: the project structure tree now shows `apps/promoter`,
  `libs/ui`, `api/cmd/promoter` and `api/internal/promoter` (previously
  DJ-only); the Node prerequisite corrected to 22.x (matches CI, was
  20.x); the `IMAGE_TAG=v1.0.1` publication caveat corrected to name
  the actual unpublished tag and the real one (`1.0.1`); the browser
  demo (`klubhub.io/demo`) linked from the README for the first time;
  `docs/INDEX.md` gained `container-images.md`, `INVOICING.md`,
  `DESIGN.md`, `github-pages.md` (all existed but weren't indexed) and
  a KlubHub Promoter self-hosting pointer; `CLAUDE.md`'s project-state
  summary (previously five months stale — still said Phase 4 was next
  and Promoter was "branch work") updated to the current phase status.

### Added

- **Phase 5 — Finance: earnings ledger (FIN-01–07, FIN-09).** The
  Finance page's Earnings panel now ships the full income / expense
  ledger (FIN-01, FIN-02), gig-linked auto-income when a gig's
  `payment_status` transitions to `paid` (FIN-03), persistent
  reconciliation prompts when the source gig's fee or currency changes
  or the gig reverts from paid (FIN-04, FIN-05), multi-currency
  monthly/yearly summaries with no cross-currency conversion (FIN-06,
  FIN-09) and per-gig / per-month / per-year P&L (FIN-07). New Go
  endpoints under `/api/v1/finance/entries`, `/summary`, `/profit-loss`
  and `/reconciliations`; new Pinia `useEarningsStore`; new
  components `EarningsWorkspace`, `EntryFormDialog`, `EntryList`,
  `EntrySummary`, `EntryProfitLoss`, `EntryReconciliationPanel` and
  `EntryReconciliationDialog`. All ledger money is handled as integer
  minor units in **JPY-aware** form (no fractional units, no float
  math). FIN-10 stays explicitly out of scope: no tax/VAT amounts are
  computed or shown anywhere in the Earnings panel.
- **`dj-ci.yml`**: KlubHub DJ's own CI gate. Go vet and `-race` tests for
  the DJ packages (`gig`, `epk`, `finance`, `social`, `tracklist`, `ra`,
  `settings`, `artwork`, `contact`, `venue`, `cmd/api`), govulncheck
  scoped to `cmd/api`, and DJ lint, typecheck and unit tests through
  `pnpm nx`. Previously these ran only on a developer's machine, or in CI
  only incidentally when a change also touched Promoter or shared files.
  DJ end-to-end (`apps/dj-e2e`) is not yet wired in: its Playwright
  `webServer` re-invokes `nx run @dev/dj:serve-static` on top of the one
  Nx's own task graph already starts, which races an unrelated
  `@dev/ui:build-static` inference and is flaky independent of this
  change.
- `containers-ci.yml` now scans the DJ image with Trivy (fails on fixable
  HIGH/CRITICAL), publishes an SPDX SBOM, and signs the published image
  with cosign (keyless, GitHub OIDC) — the same checks `promoter-ci.yml`
  already runs for the Promoter image. Still arm64-only.

### Fixed

- `apps/dj-e2e`'s `serve-static` served an empty `apps/dj/dist` instead
  of the app: `nuxt build --prerender` (`build-static`) writes to
  `apps/dj/.output/public`, so every request returned a bare directory
  listing. `apps/dj-e2e/src/example.spec.ts` was a stale scaffold
  placeholder asserting an `<h1>` containing "Welcome", which the
  dashboard has never had; it now checks the page's real title.
- `apps/dj/app/pages/index.vue`: two unused helper functions and two
  `v-if`/`v-for` on the same element (now wrapped in `<template v-if>`),
  the pre-existing lint errors that would have made `dj-ci.yml` red from
  its first run.

### Changed

- **`PROMOTER_TAG` is now required** in `docker-compose.promoter.yml` (no
  default). It previously defaulted to `v0.1.0`, which was never
  published — only `sha-<commit>` tags exist — so a manual
  `docker compose -f docker-compose.promoter.yml up` without an explicit
  tag failed to pull with no clear reason. It now fails immediately with
  a message pointing at [`docs/SELF-HOSTING.md`](docs/SELF-HOSTING.md#klubhub-promoter--self-hosting-preview).
  The one-line installer already resolved this correctly and is
  unaffected.
- klubhub.io's KlubHub Promoter badge changed from "COMING SOON" to
  "SELF-HOSTING PREVIEW": Promoter has been installable via the one-line
  installer since 2026-09-27.
- Corrected several places where the README, `.planning/ROADMAP.md` and
  the self-hosting docs had fallen behind the shipped code: the
  dashboard and Finance per-page status table, the Roadmap's "Now:
  Phase 5" line, `.planning/ROADMAP.md`'s Phase 0.5/4/9 status markers
  (each contradicted their own detail sections), and `IMAGE_TAG`
  guidance in `docs/SELF-HOSTING.md`, `docs/container-images.md` and
  `docs/PRODUCTION.md`, which named the wrong default (`v1.0.0`,
  no-longer-published) and the wrong reason (a 403 access issue) for
  the current one (`docker-compose.prod.yml` defaults to `v1.0.1`,
  which was never published; `1.0.1`, no `v`, is the real tag).

- **Resident Advisor (RA) import is now a licensed feature and is off by
  default.** This affects existing self-hosters: after upgrading, the RA
  import panels on the EPK and Gigs pages and the RA link field in the EPK
  editor are hidden. The API no longer mounts `/api/v1/epk/import-ra`,
  `/api/v1/gigs/import-ra` or `/api/v1/gigs/info/{slug}` (they return
  404), and it makes no requests to RA. Gigs and EPK data you already
  imported, and any RA link you saved, are kept. To turn RA import back
  on, set `FEATURE_RA_IMPORT=true` for the API and
  `NUXT_PUBLIC_FEATURES_RA_IMPORT=true` for the UI. With the production
  Compose file, `FEATURE_RA_IMPORT=true` in `.env` sets both. See
  [`docs/EDITIONS.md`](docs/EDITIONS.md).

### Added

- **Browser-only demo at [klubhub.io/demo](https://klubhub.io/demo/).** The
  app can be built as a static SPA with `NUXT_DEMO=1`
  (`pnpm nx run @dev/dj:build-demo` writes `dist/demo`). An in-browser API
  (`apps/dj/app/demo`) answers every `/api/v1` call from fictional sample
  data kept in the visitor's `localStorage`, with a banner and a
  **Reset demo** button. No request reaches the Go API or any third party.
  The invoicing rules now live in `apps/dj/shared/finance-mock`, shared by
  the demo and the Nitro dev mocks. The Pages workflow builds the demo and
  publishes it under `/demo/`, and the site links to it. Normal dev and
  production builds are unchanged. See [`docs/DEMO.md`](docs/DEMO.md).
- A general edition feature-flag mechanism: `Features` in the API config
  (`FEATURE_*` env vars), `runtimeConfig.public.features` with the
  `useFeatures()` composable in the UI, and one registry of licensed
  features in `apps/dj/app/utils/features.ts`. `GET /api/v1/health` now
  also returns a `features` map.

## [1.1.0] - 2026-09-25

### Phase 5 — Finance: invoicing, payments, agreements, email

Additive for existing modules: no existing endpoint changes shape. New
database migrations 011–022 run automatically at API start. The
invoicing contract (JSON shapes, endpoints, VAT rules, extension points)
is [`docs/INVOICING.md`](docs/INVOICING.md); background and diagrams are in
[`docs/release-notes/v1.1.0-phase5.md`](docs/release-notes/v1.1.0-phase5.md).

This is product functionality, not tax advice: have an accountant review
the legal wording printed on invoices for each market you invoice in.

#### Added

- **Invoicing UI** on the Finance page: invoice list with status filters
  (draft, issued, overdue, paid, void); create a draft from a gig in two
  or three clicks; detail sheet with editable *Bill to* and *Tax*, a
  pre-issue checklist, balance (outstanding, net payable, withheld,
  received, pending), payment ledger, and credit note / correct / cancel
  with confirmation. Invoice entry point in the gig form.
- **EU/US invoice compliance**:
  - Customer (bill-to) party on every invoice, pre-filled from the gig's
    promoter contact and frozen at issue. Contacts gain address, country,
    VAT ID, tax ID and business flag.
  - VAT treatment per invoice — domestic, EU reverse charge, exempt
    (small business), outside scope, US sales tax, none — suggested from
    supplier and customer country and VAT ID, with the matching legal note
    and a per-rate tax breakdown. Billing profiles gain
    `vat_exempt_small_business` and `default_vat_rate_bps`.
  - Pre-issue validation (`GET /invoices/{id}/issue-check`); issuing an
    incomplete invoice returns `422 not_issuable` with a problem list.
  - Gap-free numbering: drafts are unnumbered, numbers are allocated at
    issue per `(prefix, currency)` series under an advisory lock.
  - Credit notes (`CN` series) reverse an issued invoice; *correct* issues
    a credit note plus a replacement draft.
  - Artist withholding: net payable drives balances, "paid" and the gig's
    `payment_status`, which now syncs automatically from payments.
- **Payments**: deposits, partial payments and refunds with enforced
  limits — no over-collection, refunds never exceed receipts, same
  currency as the invoice, only on issued/paid invoices.
- **Agreements**: versioned templates and per-gig instances with a
  two-party signing workflow; signing is rejected once cancelled,
  expired, completed, already signed by that role, or for a role that
  isn't required.
- **Documents**: versioned storage on Garage S3 with SHA-256 checksums.
- **Invoice PDFs** via `codeberg.org/go-pdf/fpdf` with an embedded DejaVu
  font (full UTF-8): customer, supply date, VAT note, breakdown,
  withholding and credit-note references.
- **Transactional email outbox** delivered through
  [Plunk](https://github.com/useplunk/plunk) (hosted or self-hosted via the
  opt-in [`docker-compose.email.yml`](docker-compose.email.yml) and
  [`scripts/plunk-bootstrap.sh`](scripts/plunk-bootstrap.sh)). Rows are
  claimed with `FOR UPDATE SKIP LOCKED`, stuck sends are re-queued, and
  retries back off with an attempt cap. Email endpoints are mounted only
  when `PLUNK_BASE_URL`, `PLUNK_PROJECT_ID` and `PLUNK_API_KEY(_FILE)` are
  all set.
- **Website** (klubhub.io): full current feature set and an Editions
  section (self-hosted now; licensed and hosted editions coming soon).

#### Changed

- Webfonts rebuilt from Google Fonts (OFL) with Latin Extended-A/B and
  Additional coverage, so names such as "Łukasz" or "Perić" no longer fall
  back to the system font. Inter is now `Inter[wght].woff2` (the previous
  file had no slant axis).
- Finance `500` responses no longer include raw database errors.

#### Fixed

- Finance routes were never mounted in the API server; they are now
  served under `/api/v1/finance`.
- Invoice creation and correction crashed (nil-pointer scan) and called an
  undefined SQL function.
- Agreement-instance and email creation failed on NULL text columns
  (migration 019).
- `docker-compose.email.yml`: the Plunk API key secret was never mounted
  and Plunk sat on a different network from the app.
- SMTP header injection via CR/LF in subject, names or addresses.
- `scripts/typecheck-phase5.sh` reported success when vue-tsc never ran.

#### Upgrade notes

- Back up the database before upgrading; migrations 018–022 alter
  invoices, contacts and billing profiles and back-fill existing rows.
- Existing draft invoices lose their pre-assigned number (numbers are now
  allocated at issue) and need a customer and VAT treatment before they
  can be issued; the pre-issue checklist lists what's missing.
- Fill in your billing profile (address, country, VAT ID where
  applicable) before issuing invoices.

#### Verification

- `pnpm nx run api:test` — all Go packages green with `-race`, including
  real-Postgres integration tests (testcontainers) and a migration
  up/down/up round trip.
- `pnpm nx test @dev/dj` — 316/316 Vitest.
- `node --test apps/site/site.test.mjs` and `responsive.test.mjs` green.

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

- A GitHub Actions image-publish workflow under `.github/workflows/`
  (now `containers-ci.yml`): arm64 image publish to GHCR on push to
  main, with a post-build sanity check that fails the job if any
  non-arm64 platform is present in the published manifest. This entry
  previously also listed a `ci.yml` (lint, typecheck, unit tests,
  govulncheck, pnpm audit) and a `scan.yml` (gitleaks + CodeQL); neither
  was ever added, so KlubHub DJ's own Go and frontend packages had no CI
  gate until `dj-ci.yml` (below, unreleased).
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
