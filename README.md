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
| 5 | **Finance Tracker** | 🔜 Planned | Income/expense logging, PDF invoices, gig-payout aggregation |
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

Three supported run modes — pick the one that matches what you're doing.

### 1. Frontend-only (mock data, no backend)

The fastest way to explore the UI or do frontend work. Nitro serves mock
handlers at `apps/dj/server/api/v1/`.

```bash
pnpm install
pnpm nx serve dj
```

→ Opens at **http://localhost:4200**.

### 2. Full stack (Docker Compose)

Production-like environment: Postgres, Garage S3, Go API, Nuxt frontend — all
containerised.

```bash
cp .env.example .env
cp garage.toml.example garage.toml

# Generate secrets — see "Garage storage setup" below
openssl rand -hex 32      # → GARAGE_RPC_SECRET
openssl rand -base64 32   # → GARAGE_ADMIN_TOKEN
openssl rand -hex 32      # → TOKEN_ENCRYPTION_KEY

# Edit garage.toml: replace the REPLACE_WITH_REAL_* placeholders

docker compose up -d
```

→ Frontend: **http://localhost:3000** &nbsp;·&nbsp; API health: **http://localhost:8080/api/v1/health** &nbsp;·&nbsp; Garage S3: **http://localhost:39000**

### 3. Hybrid (db + storage in Docker, api + frontend locally)

Best for backend development — fast Go rebuilds, hot-reloading Nuxt, full data persistence.

```bash
# Start only db and storage
docker compose up -d db storage

# Run Go API
pnpm nx serve api

# Run Nuxt, proxying API requests to the local Go service
NUXT_PUBLIC_API_BASE=http://localhost:8080 pnpm nx serve dj
```

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
pnpm nx serve dj

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

# End-to-end tests (Playwright) — requires `docker compose up -d`
pnpm nx e2e dj-e2e

# CI mode (non-watch) for individual projects
pnpm nx test-ci <project>
```

---

## Backup & restore

The stack includes two scripts that handle Postgres + Garage S3 together:

```bash
# Dump Postgres and mirror Garage S3 into a timestamped .tar.gz
bash scripts/backup.sh                      # writes to ./backups/

# Restore from an archive — DROPS and recreates the klubhub database
bash scripts/restore.sh backups/klubhub-backup-20260914-120000.tar.gz
```

Both scripts require the Docker stack to be running. See [`SECURITY.md`](./SECURITY.md)
for secret-rotation guidance.

---

## Roadmap

The full v1 release plan lives in [`docs/v1-release-plan.md`](./docs/v1-release-plan.md).
At a glance:

- **Now:** Phase 4 — Gig Tracker (CRUD, iCal, booking PDF, venue/contact DB)
- **Next:** Phases 4.5 (Rider Templates) → 5 (Finance Tracker) → 6 (Release Planner)
- **Later:** Phases 7 (Tour Manager) → 8 (Unified Dashboard) → 9 (Production Hardening)

Per-phase status is tracked in [`CHANGELOG.md`](./CHANGELOG.md) and the
roadmap doc. Releases follow [semver](https://semver.org) once v1.0.0 ships.

---

## Environment variables

All variables live in `.env` (gitignored) using `.env.example` as the template.
Bind addresses are sanitised to `*********` in `.env.example`; replace with
`localhost` for local-only or your LAN IP for remote access.

| Variable | Required | Description |
|----------|----------|-------------|
| `POSTGRES_PASSWORD` | Yes | PostgreSQL password for the `klubhub` user |
| `S3_ACCESS_KEY` | Yes | Garage access key (created via `garage key create`) |
| `S3_SECRET_KEY` | Yes | Garage secret key |
| `S3_REGION` | No | S3 region (default: `europe-west-1`) |
| `S3_ENDPOINT` | No | Internal Docker endpoint (default: `storage:39000`) |
| `S3_PUBLIC_ENDPOINT` | No | Public S3 URL for presigned URLs (default: `http://*********:39000`) |
| `S3_BUCKET` | No | S3 bucket name (default: `klubhub`) |
| `GARAGE_RPC_SECRET` | Yes | RPC secret for Garage node communication |
| `GARAGE_ADMIN_TOKEN` | Yes | Admin API token for Garage |
| `TOKEN_ENCRYPTION_KEY` | Yes | 32-byte key for encrypting OAuth tokens (AES-256-GCM) |
| `BIND_ADDRESS` | No | Service bind address (default: `*********`) |
| `CORS_ORIGIN` | No | Allowed CORS origin (default: `http://*********:3000`) |
| `LOG_LEVEL` | No | `debug` / `info` / `warn` / `error` (default: `info`) |
| `NUXT_PUBLIC_API_BASE` | No | Go API URL (enables proxy); omit for mock-data dev mode |
| `SPOTIFY_CLIENT_ID` | No | Spotify API — cover art fetching |
| `SPOTIFY_CLIENT_SECRET` | No | Spotify API — cover art fetching |
| `DISCOGS_API_KEY` | No | Discogs API — cover art fallback |
| `INSTAGRAM_CLIENT_ID` | No | Instagram OAuth |
| `INSTAGRAM_CLIENT_SECRET` | No | Instagram OAuth |

`MINIO_*` variables are no longer supported. Existing deployments must rename
them to the matching `S3_*` variables before upgrading to v1.0.0.

---

## Garage storage setup

Garage is an S3-compatible, self-hosted object store designed for small
clusters. We use it for cover art, generated tracklist images, EPK photos,
and any other binary blob.

### One-time setup (after the first `docker compose up -d`)

```bash
docker compose up -d storage

# 1. Assign layout (get the node id from `garage status`)
docker exec klubhub-storage /garage layout assign -z dc1 -c 1G <node_id>

# 2. Apply layout
docker exec klubhub-storage /garage layout apply --version 1

# 3. Create bucket + key, then grant access
docker exec klubhub-storage /garage bucket create klubhub
docker exec klubhub-storage /garage key create klubhub-app-key
docker exec klubhub-storage /garage bucket allow \
  --read --write --owner klubhub --key klubhub-app-key

# 4. Copy the printed Key ID and Secret into .env as S3_ACCESS_KEY / S3_SECRET_KEY
docker compose up -d   # start the rest of the stack
```

The full Garage docs are at <https://garage.deuxfleurs.fr/documentation/>.

### Why Garage, not MinIO?

| Feature | Garage | MinIO |
|---------|--------|-------|
| S3 API compatibility | ✅ Native | ✅ Native |
| Docker image | ✅ `dxflrs/garage` | ✅ `minio/minio` |
| Config file | ✅ `garage.toml` | ❌ Env vars only |
| Anonymous health probe | ❌ Requires auth | ✅ |
| Multi-node clustering | ✅ | ✅ |
| Footprint | Lightweight | Heavier (Java) |

---

## Mock vs backend mode

The frontend can run in two modes:

- **Mock mode** (default, no env vars): Nitro serves mock JSON from
  `apps/dj/server/api/v1/`. Great for frontend-only work and design reviews.
- **Backend mode**: set `NUXT_PUBLIC_API_BASE=http://localhost:8080` and Nitro
  proxies `/api/v1/*` to the Go API.

### Per-page status

| Page | UI | Mock data | Go backend |
|------|----|-----------|------------|
| `/` Dashboard | ✅ | Static | — (Phase 8 wiring) |
| `/tracklist` Tracklist | ✅ | ✅ GET | ✅ CRUD, parse, generate-image |
| `/social` Scheduler | ✅ | ✅ GET | ✅ CRUD, schedule, publish, worker |
| `/epk` Press Kit | ✅ | ✅ GET/PUT | ✅ CRUD, photo upload, PDF export |
| `/gigs` Gig Tracker | 🚧 | Static | 🚧 (Phase 4 — in progress) |
| `/finance` Finance | ⚠️ | Static | — (Phase 5) |

---

## License

[MIT](./LICENSE) — © 2026 lithqube.

---

## Acknowledgments

- The [Garage](https://garage.deuxfleurs.fr) team for a great S3-compatible
  alternative to MinIO
- The Nuxt, Tailwind, and shadcn-vue communities
- Every DJ who has tried this and filed an issue
