<div align="center">

# KlubHub DJ

**All-in-one career management platform for independent DJs** — tracklist image generator, social media scheduler, press kit builder, gig tracker, finance tracker, and more.

Self-hosted. Open-source. Built for DJs who want to own their workflow.

[![License: MIT](https://img.shields.io/badge/license-MIT-22d3ee.svg)](./LICENSE)
[![Made with Nuxt](https://img.shields.io/badge/Nuxt-4.3-00DC82.svg)](https://nuxt.com)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev)
[![Nx](https://img.shields.io/badge/Nx-22-0a1a2b.svg)](https://nx.dev)
[![pnpm](https://img.shields.io/badge/pnpm-9-F69220.svg)](https://pnpm.io)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED.svg)](https://docs.docker.com/compose/)

[Features](#features) · [Quick start](#quick-start) · [Try the demo ↗](https://klubhub.io/demo/) · [Roadmap](#roadmap) · [Docs](./docs/INDEX.md) · [Contributing](./CONTRIBUTING.md) · [Security](./SECURITY.md) · [klubhub.io](https://klubhub.io)

</div>

---

## A monorepo for the KlubHub family

This repository is the shared home for the **[KlubHub](https://klubhub.io)** product family — one scene, a growing toolkit — managed as a single Nx + pnpm workspace under `apps/`. **KlubHub DJ** is the first product to live here. **KlubHub Promoter** (for promoters, collectives and event organisers) is in development under `apps/promoter` and `api/cmd/promoter`; see [its self-hosting preview](./docs/SELF-HOSTING.md#klubhub-promoter--self-hosting-preview) and [security model](./SECURITY.md#klubhub-promoter-security-model). **KlubHub Label** will land the same way. Products share the Kinetic HUD design system as a Nuxt layer in `libs/ui`.

Most KlubHub DJ features will stay open source under this repository's MIT license; some future features across the family may be offered as SaaS. SaaS scope, pricing, and availability have not been announced.

The public brand site for the whole family lives at `apps/site/` and deploys to **[klubhub.io](https://klubhub.io)** — see [GitHub Pages setup](./docs/github-pages.md) for local preview, deployment, and DNS configuration.

Everything else in this README describes **KlubHub DJ** specifically, unless noted otherwise.

## Why KlubHub DJ?

Most DJ tooling is fragmented: a SaaS here for analytics, another for scheduling, a separate site for the press kit, a spreadsheet for finances. KlubHub DJ brings it all under one self-hosted roof with a single design language, one database, one backup story.

You get:

- **Ownership** — your data, your storage (Garage S3), your Postgres, your machine.
- **A coherent UI** — the "Kinetic HUD" design system across every module.
- **A working offline dev loop** — the Nuxt app runs against Nitro mock handlers, so frontend work needs zero infrastructure.

---

## Features

| # | Module | Status | What it does |
|---|--------|--------|--------------|
| 0 | **Infrastructure** | ✅ Complete | Docker Compose stack (Postgres 16, Garage S3, Go API, Nuxt), health endpoint, migrations, backup/restore |
| 1 | **Tracklist Image Generator** | ✅ Complete | Upload 1001Tracklists / Mixcloud / SoundCloud export → parsed tracklist → cover art (Spotify / Discogs) → PNG/JPEG |
| 1.5 | **Design System Foundation** | ✅ Complete | Tailwind v4, shadcn-vue, Pinia, responsive shell, 3-way theme switcher |
| 1.5.5 | **Kinetic HUD** | ✅ Complete | Cyberpunk HUD aesthetic — glass panels, bracket boxes, pulse dots, variable-font typography |
| 2 | **Social Media Scheduler** | ✅ Complete | Instagram OAuth, timezone-aware scheduling, retry with exponential backoff, calendar view |
| 3 | **EPK / Press Kit Builder** | ✅ Complete | Bio, press photos, tech rider, PDF export (go-pdf/fpdf) |
| 4 | **Gig Tracker** | ✅ Complete | CRUD, status / payment workflows, venue & contact database, iCal feed, booking confirmation PDF |
| 4.5 | **Rider Templates** | 🔜 Planned | Named tech/hospitality templates attachable to gigs with per-gig overrides |
| 5 | **Finance Tracker** | ✅ Shipped (Phase 5) | Invoices (draft → issued → paid), deposits + partial payments, event agreements with signing workflow, EU/US-ready invoicing (customer details, VAT treatment incl. reverse charge, credit notes, artist withholding), PDF invoice rendering ([go-pdf/fpdf](https://codeberg.org/go-pdf/fpdf)), transactional email via [Plunk](https://github.com/useplunk/plunk) (hosted or self-hosted). See [`docs/INVOICING.md`](./docs/INVOICING.md) and [`docs/release-notes/v1.1.0-phase5.md`](./docs/release-notes/v1.1.0-phase5.md). |
| 6 | **Release Planner** | 🔜 Planned | Release status workflow, promo checklist, deadline tracking |
| 7 | **Tour Manager** | 🔜 Planned | Tour groups, per-stop logistics, budget aggregation |
| 8 | **Unified Dashboard** | 🔜 Planned | Cross-module overview, career analytics |
| 9 | **Production Hardening** | ✅ Complete (this release) | arm64 images on GHCR, hardened compose, OAuth state, body caps, secrets via Docker `secrets:` block, restore drill |

> **Note — architecture:** the KlubHub DJ image is `linux/arm64` only, by
> design (Apple Silicon, Raspberry Pi 5, Graviton/Ampere) — not a
> temporary gap; the installer offers amd64 emulation as a fallback. See
> [SELF-HOSTING.md](./docs/SELF-HOSTING.md#system-requirements). KlubHub
> Promoter's image is built natively for both `linux/amd64` and
> `linux/arm64`.

See [`docs/v1-release-plan.md`](./docs/v1-release-plan.md) for full module scope and v1 acceptance criteria.
See [`CHANGELOG.md`](./CHANGELOG.md) for what hardened between the last feature branch and the v1.0.0 tag.

---

## Quick start

Not ready to install anything? **[Try the browser-only demo](https://klubhub.io/demo/)** — the real app with fictional sample data, running entirely client-side. Nothing you enter leaves your browser. See [`docs/DEMO.md`](./docs/DEMO.md).

Choose a mode. The [canonical setup guide](./docs/SELF-HOSTING.md) covers prerequisites, first startup, networking, and safe upgrades. The [production runbook](./docs/PRODUCTION.md) is the agent-focused operator reference for verify, upgrade, rotate, and troubleshoot.

**Run it in production with one line** (Docker required; asks DJ, Promoter or both):

```bash
curl -fsSL https://klubhub.io/install.sh | bash
```

The same file is served from GitHub (`https://raw.githubusercontent.com/lithqube/klubhub/main/scripts/install.sh`) and published as a [gist](https://gist.github.com/lithqube/3a73d34d4dbdcbb602359362eff31e47): `curl -fsSL https://gist.githubusercontent.com/lithqube/3a73d34d4dbdcbb602359362eff31e47/raw/klubhub-install.sh | bash`.

It checks the machine, fetches the scripts and matching images, creates every secret, starts the stack and prints the URLs, and for Promoter creates your organisation with a one-time owner link. Re-run it (or `~/klubhub/klubhub update`) to update; secrets and data are kept. Options such as `--promoter --yes`, `--bind`, ports and public URLs: [one-line install](./docs/SELF-HOSTING.md#one-line-install).

| Mode | Command | What runs |
|---|---|---|
| Frontend mocks | `pnpm install`, then `pnpm nx serve @dev/dj` | Local Nuxt at http://localhost:4200; leave `NUXT_PUBLIC_API_BASE` unset. |
| Backend development | `bash scripts/setup.sh dev` | Postgres, Garage, and a locally built API in Docker; run Nuxt locally as shown below. |
| Production | `bash scripts/setup.sh prod` | Postgres, Garage, and one published app image; UI and API at http://127.0.0.1:8080. |

For development with the real API, run this in another terminal:

```bash
NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080 pnpm nx serve @dev/dj --host 0.0.0.0 --port 4200
```

The API calls the host Nuxt server at `http://host.docker.internal:4200` for image rendering. The all-interface Nuxt bind allows that callback; use a trusted development network and firewall.

Setup defaults to `dev`, preserves existing credentials, creates private file secrets and Garage configuration under `.local/dev` or `.local/prod`, bootstraps Garage, and starts the selected stack. Development and production use separate Compose projects and volumes, but their default host ports overlap: stop one before starting the other, or configure distinct ports.

**Publication caveat:** the production file defaults to `IMAGE_TAG=v1.0.1`, which **was never published** — only `1.0.1` (no `v`) is, and it predates the v1.1.0 invoicing work. Export `IMAGE_TAG=1.0.1` explicitly, or the newest `sha-<commit>` tag, until a v1.2.0 release publishes matching tags. See [container images](./docs/container-images.md) for tag verification, and [release notes](./docs/release-notes/v1.0.1.md) for background on the earlier v1.0.0/v1.0.1 hardening.

**Existing installation?** Do not start a new project over an old deployment without a migration plan. The old `klubhub-dj` project's volumes need explicit reuse or migration; see [safe upgrades](./docs/SELF-HOSTING.md#upgrades-and-existing-installations).

**Security:** this is a network-private, single-user app, not an authenticated public SaaS. TLS and CORS do not provide authentication. Keep it private or add an authentication gateway before any public exposure.

**Production operators** (agents or humans) should read [`docs/PRODUCTION.md`](./docs/PRODUCTION.md) for the verify / upgrade / rotate / troubleshoot checklist. The setup guide covers first-time install; the production doc covers ongoing operation.

---

## Tech stack

| Layer | Tech |
|-------|------|
| **Frontend** | [Nuxt 4.3](https://nuxt.com) · Vue 3.5 · TypeScript 5.9 |
| **UI** | [Tailwind CSS v4](https://tailwindcss.com) · [shadcn-vue](https://www.shadcn-vue.com) · "Kinetic HUD" design system |
| **State** | [Pinia](https://pinia.vuejs.org) (setup-function stores) |
| **Backend** | [Go 1.26](https://go.dev) · [chi](https://github.com/go-chi/chi) router · [pgx v5](https://github.com/jackc/pgx) · [goose](https://github.com/pressly/goose) migrations |
| **Database** | [PostgreSQL 16](https://www.postgresql.org) |
| **Object storage** | [Garage](https://garage.deuxfleurs.fr) — S3-compatible, self-hosted |
| **Image generation** | [Playwright](https://playwright.dev) headless Chromium |
| **PDF generation** | [go-pdf/fpdf](https://codeberg.org/go-pdf/fpdf) |
| **Monorepo** | [Nx 22](https://nx.dev) + [pnpm workspaces](https://pnpm.io/workspaces) |

---

## Project structure

```
klubhub/
├── apps/                            # KlubHub product family workspace
│   ├── dj/                          # KlubHub DJ — Nuxt 4 frontend (first product in the family)
│   │   ├── app/
│   │   │   ├── pages/               # index, tracklist, social, epk, gigs, finance
│   │   │   ├── components/          # TheNav, tracklist/, social/, epk/, ui/
│   │   │   ├── composables/         # useTheme (3-way), useTracklist, useEpkAutosave
│   │   │   ├── stores/              # Pinia stores (tracklist, social, epk, settings, ui)
│   │   │   ├── types/               # TypeScript interfaces
│   │   │   └── assets/css/          # DJ-specific Kinetic HUD classes (tokens live in libs/ui)
│   │   └── server/
│   │       ├── api/v1/              # Mock data handlers (when NUXT_PUBLIC_API_BASE unset)
│   │       └── plugins/             # Playwright singleton (graceful fallback)
│   ├── dj-e2e/                      # Playwright end-to-end tests for KlubHub DJ
│   ├── promoter/                    # KlubHub Promoter — Nuxt 4 frontend (self-hosting preview)
│   ├── promoter-e2e/                # Playwright end-to-end tests for KlubHub Promoter
│   └── site/                        # klubhub.io — public brand site for the whole family
│
├── libs/
│   └── ui/                          # Shared Kinetic HUD design system, as a Nuxt layer (`#kui`)
│
├── api/                             # Go module shared by both product binaries
│   ├── cmd/
│   │   ├── api/main.go              # KlubHub DJ entry point (chi router, worker goroutine, SIGTERM)
│   │   └── promoter/main.go         # KlubHub Promoter entry point
│   ├── internal/
│   │   ├── platform/                # config, db, storage, crypto, http, auth, authz, migrations — shared by both products
│   │   │   └── migrations/          # goose SQL migrations for KlubHub DJ (numbered 001, 002, ...)
│   │   ├── tracklist/               # parser, repo, service, handler (DJ)
│   │   ├── social/                  # repo, service, handler, worker, instagram client (DJ)
│   │   ├── epk/                     # repo, service, handler, pdf generator (DJ)
│   │   ├── finance/                 # invoicing, payments, agreements, email (DJ)
│   │   ├── settings/                # user settings CRUD (DJ)
│   │   ├── gig/                     # gig CRUD, iCal feed, booking PDF (DJ)
│   │   ├── venue/  contact/         # venue & contact databases (DJ)
│   │   ├── artwork/                 # cover art lookup (Spotify, Discogs) (DJ)
│   │   ├── ra/                      # Resident Advisor import client (DJ, licensed feature)
│   │   └── promoter/                # org, event, lineup, guestlist, door, retention, sealed ban list (Promoter)
│   │       └── migrations/          # goose SQL migrations for KlubHub Promoter
│   └── testdata/                    # integration test fixtures
│
├── docs/                            # BRD, NFR, architecture, release plans  → see docs/INDEX.md
├── .claude/plans/                   # Design docs for larger features (Promoter phases, licensed-integration research)
├── scripts/                         # setup.sh, backup.sh, restore.sh, install.sh
├── .github/                         # issue & PR templates
│   └── workflows/                   # dj-ci.yml, promoter-ci.yml, containers-ci.yml, pages.yml
├── CLAUDE.md                        # Claude AI assistant guidelines
├── AGENTS.md                        # Nx workspace guidelines
├── CONTRIBUTING.md                  # contribution guide
├── SECURITY.md                      # vulnerability disclosure policy
├── CODE_OF_CONDUCT.md               # Contributor Covenant v2.1
├── CHANGELOG.md                     # release history
├── docker-compose.yml               # base service definitions
├── docker-compose.dev.yml           # DJ backend development overlay
├── docker-compose.prod.yml          # DJ production deployment (image-only)
└── docker-compose.promoter.yml      # Promoter deployment (separate Compose project)
```

---

## Development

### Prerequisites

| Tool | Version |
|------|---------|
| Node | 22.x (matches CI) |
| pnpm | 9.x |
| Go | 1.26+ |
| Docker | 24+ with Compose v2 |

### Common commands

Always run tasks through `pnpm nx …` — never `nx`, `go`, `nuxt`, `vitest`, or
`playwright` directly. See [`AGENTS.md`](./AGENTS.md) for the rationale.

```bash
# Start the frontend (mock data mode)
pnpm nx serve @dev/dj

# Build, lint, typecheck
pnpm nx build <project>
pnpm nx lint <project>
pnpm nx typecheck <project>

# Run a single project's unit tests
pnpm nx test <project>

# Run unit tests across the whole workspace
pnpm nx run-many -t test

# End-to-end tests (requires Docker stack)
pnpm nx e2e dj-e2e
```

The `@dev/dj` typecheck target currently prints a disabled notice rather than checking TypeScript. Do not report its exit status as a typecheck pass.

### Code conventions

- **Frontend** — Vue 3 Composition API with `<script setup lang="ts">`, Pinia
  stores in **setup-function style**, all `$fetch` calls in stores /
  composables, theme tokens from the Kinetic HUD design system.
- **Backend** — `handler → service → repository` three-layer pattern,
  structured logging via `rs/zerolog`, configuration via
  `kelseyhightower/envconfig`.

Full conventions in [`CONTRIBUTING.md`](./CONTRIBUTING.md).

---

## Testing

```bash
# Unit tests (Vitest)
pnpm nx run-many -t test

# End-to-end tests (Playwright) — configure the real backend per SELF-HOSTING.md
pnpm nx e2e dj-e2e

# CI mode (non-watch) for individual projects
pnpm nx test-ci <project>
```

---

## Backup & restore

Back up both Postgres and Garage, along with the private configuration needed to restore them. See [Operations](./docs/OPERATIONS.md#backup-and-restore) for script prerequisites and destructive-restore precautions. Always target the correct Compose project and mode.

---

## Roadmap

The full v1 release plan lives in [`docs/v1-release-plan.md`](./docs/v1-release-plan.md).
At a glance:

- **Now:** finishing Phase 5 — invoices, payments, agreements and PDF/Plunk email shipped in v1.1.0; income/expense tracking (earnings) is the remaining piece
- **Next:** Phase 6 (Release Planner) → 7 (Tour Manager) → 8 (Unified Dashboard)

Per-phase status is tracked in [`CHANGELOG.md`](./CHANGELOG.md) and the
roadmap doc. Releases follow [semver](https://semver.org) once v1.0.0 ships.

---

## Environment variables

Setup stores mode-specific private state under `.local/dev` and `.local/prod` (gitignored). Do not print, commit, or copy those credentials into bug reports. See the [configuration reference](./docs/CONFIGURATION.md) for bind addresses, browser-visible S3 URLs, and optional integrations.

Garage provisioning belongs to `scripts/setup.sh`; do not manually copy printed keys or evaluate bootstrap output. Follow the [single setup guide](./docs/SELF-HOSTING.md) for both modes.

---

## Mock vs backend mode

The frontend can run in two modes:

- **Mock mode** (default, no env vars): Nitro serves mock JSON from
  `apps/dj/server/api/v1/`. Great for frontend-only work and design reviews.
- **Backend mode**: set `NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080` and Nitro
  proxies `/api/v1/*` to the Go API.

### Per-page status

| Page | UI | Mock data | Go backend |
|------|----|-----------|------------|
| `/` Dashboard | ✅ | Live gig, social & tracklist data | ✅ (full cross-module aggregation is Phase 8) |
| `/tracklist` Tracklist | ✅ | ✅ GET | ✅ CRUD, parse, generate-image |
| `/social` Scheduler | ✅ | ✅ GET | ✅ CRUD, schedule, publish, worker |
| `/epk` Press Kit | ✅ | ✅ GET/PUT | ✅ CRUD, photo upload, PDF export |
| `/gigs` Gig Tracker | ✅ | Static | ✅ CRUD, iCal feed, booking confirmation PDF |
| `/finance` Finance | ✅ | ✅ | ✅ invoicing, payments, agreements, email outbox (shipped in v1.1.0); the Earnings panel (income/expense log) is not yet wired |

---

## License

[MIT](./LICENSE) — © 2026 lithqube.

KlubHub is a project by EUN Records, Berlin.

---

## Acknowledgments

- The [Garage](https://garage.deuxfleurs.fr) team for a great S3-compatible
  alternative to MinIO
- The Nuxt, Tailwind, and shadcn-vue communities
- Every DJ who has tried this and filed an issue
