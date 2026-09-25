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

[Features](#features) · [Quick start](#quick-start) · [Roadmap](#roadmap) · [Docs](./docs/INDEX.md) · [Contributing](./CONTRIBUTING.md) · [Security](./SECURITY.md) · [klubhub.io](https://klubhub.io)

</div>

---

## A monorepo for the KlubHub family

This repository is the shared home for the **[KlubHub](https://klubhub.io)** product family — one scene, a growing toolkit — managed as a single Nx + pnpm workspace under `apps/`. **KlubHub DJ** is the first product to live here; **KlubHub Promoter** and **KlubHub Label** are coming soon and will land the same way: a new `apps/<product>` workspace project sharing this repo's tooling, CI, and design system rather than starting from scratch.

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
| 5 | **Finance Tracker** | ✅ Shipped (Phase 5) | Invoices (draft → issued → paid), deposits + partial payments, event agreements with signing workflow, PDF invoice rendering ([gofpdf](https://github.com/jung-kurt/gofpdf)), transactional email via [Plunk](https://github.com/useplunk/plunk) (hosted or self-hosted). See [`docs/release-notes/v1.1.0-phase5.md`](./docs/release-notes/v1.1.0-phase5.md). |
| 6 | **Release Planner** | 🔜 Planned | Release status workflow, promo checklist, deadline tracking |
| 7 | **Tour Manager** | 🔜 Planned | Tour groups, per-stop logistics, budget aggregation |
| 8 | **Unified Dashboard** | 🔜 Planned | Cross-module overview, career analytics |
| 9 | **Production Hardening** | ✅ Complete (this release) | arm64 images on GHCR, hardened compose, OAuth state, body caps, secrets via Docker `secrets:` block, restore drill |

> **Note — architecture:** v1.0.0 images are `linux/arm64` only. amd64 is
> deferred to a later point release. See
> [SELF-HOSTING.md](./docs/SELF-HOSTING.md#system-requirements).

See [`docs/v1-release-plan.md`](./docs/v1-release-plan.md) for full module scope and v1 acceptance criteria.
See [`CHANGELOG.md`](./CHANGELOG.md) for what hardened between the last feature branch and the v1.0.0 tag.

---

## Quick start

Choose a mode. The [canonical setup guide](./docs/SELF-HOSTING.md) covers prerequisites, first startup, networking, and safe upgrades. The [production runbook](./docs/PRODUCTION.md) is the agent-focused operator reference for verify, upgrade, rotate, and troubleshoot.

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

**Publication caveat:** the production file defaults to `IMAGE_TAG=v1.0.1`. This default does not mean the existing tag contains the fixes in `v1.0.1`; v1.0.1 must be published to GHCR before production pull succeeds. See [container images](./docs/container-images.md) for tag verification, and [release notes](./docs/release-notes/v1.0.1.md) for the operator follow-up steps (Plunk workflow rename, GHCR publish trigger).

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
├── apps/                            # KlubHub product family workspace — future products land here as apps/<product>
│   ├── dj/                          # KlubHub DJ — Nuxt 4 frontend (first product in the family)
│   │   ├── app/
│   │   │   ├── pages/               # index, tracklist, social, epk, gigs, finance
│   │   │   ├── components/          # TheNav, tracklist/, social/, epk/, ui/
│   │   │   ├── composables/         # useTheme (3-way), useTracklist, useEpkAutosave
│   │   │   ├── stores/              # Pinia stores (tracklist, social, epk, settings, ui)
│   │   │   ├── types/               # TypeScript interfaces
│   │   │   └── assets/css/          # Tailwind @theme + Kinetic HUD component classes
│   │   └── server/
│   │       ├── api/v1/              # Mock data handlers (when NUXT_PUBLIC_API_BASE unset)
│   │       └── plugins/             # Playwright singleton (graceful fallback)
│   ├── dj-e2e/                      # Playwright end-to-end tests for KlubHub DJ
│   └── site/                        # klubhub.io — public brand site for the whole family
│
├── api/                             # Go backend for KlubHub DJ (future products bring their own)
│   ├── cmd/api/main.go              # Entry point (chi router, worker goroutine, SIGTERM)
│   ├── internal/
│   │   ├── platform/                # config, db, storage, crypto, http, migrations
│   │   │   └── migrations/          # goose SQL migrations (001–005f)
│   │   ├── tracklist/               # parser, repo, service, handler
│   │   ├── social/                  # repo, service, handler, worker, instagram client
│   │   ├── epk/                     # repo, service, handler, pdf generator
│   │   ├── settings/                # user settings CRUD
│   │   ├── gig/                     # gig CRUD, iCal feed, booking PDF
│   │   ├── venue/  contact/         # venue & contact databases
│   │   └── artwork/                 # cover art lookup (Spotify, Discogs)
│   └── testdata/                    # integration test fixtures
│
├── docs/                            # BRD, NFR, architecture, release plans  → see docs/INDEX.md
├── scripts/                         # backup.sh, restore.sh
├── .github/                         # issue & PR templates
├── CLAUDE.md                        # Claude AI assistant guidelines
├── AGENTS.md                        # Nx workspace guidelines
├── CONTRIBUTING.md                  # contribution guide
├── SECURITY.md                      # vulnerability disclosure policy
├── CODE_OF_CONDUCT.md               # Contributor Covenant v2.1
├── CHANGELOG.md                     # release history
└── docker-compose.yml
```

---

## Development

### Prerequisites

| Tool | Version |
|------|---------|
| Node | 20.x LTS |
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

- **Now:** Phase 5 — Finance Tracker (invoices, payments, agreements, PDF, Plunk email)
- **Next:** Phases 6 (Release Planner) → 7 (Tour Manager) → 8 (Unified Dashboard) → 9 (Production Hardening)

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
| `/` Dashboard | ✅ | Static | — (Phase 8 wiring) |
| `/tracklist` Tracklist | ✅ | ✅ GET | ✅ CRUD, parse, generate-image |
| `/social` Scheduler | ✅ | ✅ GET | ✅ CRUD, schedule, publish, worker |
| `/epk` Press Kit | ✅ | ✅ GET/PUT | ✅ CRUD, photo upload, PDF export |
| `/gigs` Gig Tracker | ✅ | Static | ✅ CRUD, iCal feed, booking confirmation PDF |
| `/finance` Finance | 🚧 | 🚧 | ✅ invoicing, payments, agreements, email outbox (Phase 5 — UI wiring in progress) |

---

## License

[MIT](./LICENSE) — © 2026 lithqube.

---

## Acknowledgments

- The [Garage](https://garage.deuxfleurs.fr) team for a great S3-compatible
  alternative to MinIO
- The Nuxt, Tailwind, and shadcn-vue communities
- Every DJ who has tried this and filed an issue
