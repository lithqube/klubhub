# KlubHub DJ

**All-in-one career management platform for independent DJs** — tracklist image generator, social media scheduler, press kit builder, gig tracker, finance tracker, and more.

Self-hosted, open-source, built for DJs who want to own their workflow.

---

## Features

| Phase | Module | Status |
|---|---|---|
| 0 | Infrastructure (Docker Compose stack, Go API, health endpoint) | ✅ Complete |
| 1 | Tracklist Image Generator (upload, parse, cover art, PNG/JPEG export) | ✅ Complete |
| 1.5 | Design System Foundation (Tailwind v4, shadcn-vue, Pinia, responsive shell) | ✅ Complete |
| 1.5.5 | Kinetic HUD UI Migration (full redesign to cyberpunk HUD aesthetic) | ✅ Complete |
| 2 | Social Media Scheduler (Instagram OAuth, scheduling, retry, calendar view) | ✅ Complete |
| 3 | EPK / Press Kit Builder (bio, photos, tech rider, PDF export) | ✅ Complete |
| 4 | Gig Tracker (CRUD, status workflow, iCal, venue/contact DB) | 🔜 Next |
| 4.5 | Rider Templates (reusable tech/hospitality templates) | 🔜 Planned |
| 5 | Finance Tracker (income/expense logging, PDF invoices) | 🔜 Planned |
| 6 | Release Planner (status workflow, deadlines, promo checklist) | 🔜 Planned |
| 7 | Tour Manager (tour groups, per-stop logistics, budget aggregate) | 🔜 Planned |
| 8 | Unified Dashboard (all-modules overview, career analytics) | 🔜 Planned |
| 9 | Production Hardening (docs, security audit, CI/CD, release) | 🔜 Planned |

---

## Tech Stack

| Layer | Tech |
|---|---|
| Frontend | Nuxt 4.3 / Vue 3.5 / TypeScript 5.9 |
| UI | Tailwind CSS v4 · shadcn-vue · "Kinetic HUD" cyberpunk design system |
| State | Pinia (setup function stores) |
| Backend API | Go 1.26 (chi router, pgx v5, goose migrations) |
| Database | PostgreSQL 16 |
| Object Storage | MinIO |
| Image Generation | Playwright (headless Chromium) |
| PDF Generation | go-pdf/fpdf |
| Monorepo | Nx 22 + pnpm workspaces |

---

## Quick Start

### Frontend Development (mock data, no backend)

```bash
pnpm install
pnpm nx serve dj
```

Opens at **http://localhost:4200** with mock data for all API routes. No Go backend, Docker, or database required.

### Full Stack (Docker)

```bash
cp .env.example .env
docker compose up -d
```

- Frontend: **http://localhost:3000**
- API Health: **http://localhost:8080/api/v1/health**
- MinIO UI: **http://localhost:9001**

### Backend + Frontend (hybrid)

```bash
# Start only db and storage
docker compose up -d db storage

# Start Go API
pnpm nx serve api

# Start Nuxt (proxies to Go API)
NUXT_PUBLIC_API_BASE=http://localhost:8080 pnpm nx serve dj
```

---

## Testing

```bash
# Unit tests
pnpm nx run-many -t test

# E2E tests (requires Docker stack)
docker compose up -d
pnpm nx e2e dj-e2e
```

---

## Project Structure

```
klubhub-dj/
├── apps/
│   ├── dj/                        # Nuxt 4 frontend
│   │   ├── app/
│   │   │   ├── pages/             # index, tracklist, social, epk, gigs, finance
│   │   │   ├── components/       # TheNav, tracklist/, social/, epk/, ui/
│   │   │   ├── composables/       # useTheme (3-way), useTracklist, useEpkAutosave
│   │   │   ├── stores/            # Pinia stores (tracklist, social, epk, settings, ui)
│   │   │   ├── types/              # TypeScript interfaces
│   │   │   └── assets/css/        # Tailwind @theme + Kinetic HUD component classes
│   │   └── server/
│   │       ├── api/v1/            # Mock data handlers (when NUXT_PUBLIC_API_BASE unset)
│   │       └── plugins/           # Playwright singleton (graceful fallback)
│   └── dj-e2e/                    # Playwright E2E tests
│
├── api/                           # Go backend
│   ├── cmd/api/main.go            # Entry point (chi router, worker goroutine, SIGTERM)
│   ├── internal/
│   │   ├── platform/              # config, db, storage, crypto, http, migrations
│   │   ├── tracklist/             # parser, repo, service, handler
│   │   ├── social/                # repo, service, handler, worker, instagram client
│   │   ├── epk/                  # repo, service, handler, pdf generator
│   │   └── settings/              # user settings CRUD
│   └── migrations/               # goose SQL migrations (001–004)
│
├── docs/                          # BRD, NFR, architecture, v1/v2 release plans
├── scripts/                        # backup.sh, restore.sh
├── .planning/                      # GSD planning (STATE, ROADMAP, phase plans)
├── CLAUDE.md                       # Claude AI assistant guidelines
├── AGENTS.md                       # Nx workspace guidelines
└── docker-compose.yml
```

---

## Design System — Kinetic HUD

Custom cyberpunk design system built on Tailwind CSS v4:

- **Palette**: Radioactive cyan (`#96F8FF`) on obsidian (`#101012`); violet accent; amber archived state
- **Typography**: Space Grotesk (headlines), Manrope (body), Inter (data) — variable fonts, served locally
- **Glass panels**: `backdrop-filter:blur(24px)` + RGBA backgrounds + 1px semi-transparent border
- **HUD components**: `.hud-card`, `.bracket-box`, `.pulse-dot`, `.data-frag`, `.upload-zone`, `.btn-hud`, `.hud-input`
- **Theme modes**: `dark | system | light` — 3-way switcher with OS preference listener

All tokens in `apps/dj/app/assets/css/styles.css` inside Tailwind `@theme {}` block.

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `MINIO_ENDPOINT` | Yes | MinIO host |
| `MINIO_ACCESS_KEY` | Yes | MinIO access key |
| `MINIO_SECRET_KEY` | Yes | MinIO secret key |
| `TOKEN_ENCRYPTION_KEY` | Yes | 32-byte key for encrypting OAuth tokens |
| `NUXT_PUBLIC_API_BASE` | No | Go API URL (enables proxy); omit for mock-data dev mode |
| `SPOTIFY_CLIENT_ID` | No | Spotify API — cover art fetching |
| `DISCOGS_TOKEN` | No | Discogs API — cover art fallback |
| `INSTAGRAM_APP_ID` | No | Instagram OAuth |
| `INSTAGRAM_APP_SECRET` | No | Instagram OAuth |

---

## Object Storage — MinIO vs GarageHQ

By default the project uses **MinIO** as the S3-compatible object storage layer (included in `docker-compose.yml` as the `storage` service).

An alternative is **GarageHQ** (https://garagehq.deuxfleurs.fr/), an open-source S3-compatible storage server designed for self-hosting. It is lightweight, actively maintained, and supports both local and distributed deployment. To use GarageHQ instead of MinIO:

1. Replace the `storage` service in `docker-compose.yml` with GarageHQ
2. Update `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, and add `MINIO_REGION` env vars
3. GarageHQ's S3 API is compatible with the existing `minio-go` client — no code changes needed

See `.planning/phases/00-infrastructure/` for the GarageHQ-compatible `docker-compose.yml` variant.

---

## Object Storage Compatibility

| Feature | MinIO | GarageHQ |
|---|---|---|
| S3 API compatibility | ✅ Native | ✅ Native |
| Docker image | ✅ `minio/minio` | ✅ `registry.deuxfleurs.fr/garagehq/garage:latest` |
| Embedded in compose | ✅ (storage service) | 🔜 Phase 0.5 |
| Multi-node / clustering | ✅ | ✅ |
| Health check | ✅ | TBD |
| S3 signature version | v4 | v4 |

---

## Mock vs Backend Status

The frontend can run in two modes:

- **Mock mode** (default, no env vars needed): Nuxt Nitro serves mock data handlers at `apps/dj/server/api/v1/`
- **Backend mode**: Set `NUXT_PUBLIC_API_BASE=http://localhost:8080` to proxy all `/api/v1/*` to the Go API

### Per-Page Mock vs Backend Map

| Page | UI Status | Mock Data | Go Backend |
|---|---|---|---|
| `/` Dashboard | ✅ Complete | Static mock | — (Phase 8 wiring) |
| `/tracklist` Tracklist Generator | ✅ Complete | GET /api/v1/tracklists (mock) | ✅ Full CRUD, parse, generate-image |
| `/social` Social Scheduler | ✅ Complete | GET /api/v1/social/posts + accounts (mock) | ✅ Full CRUD, schedule, publish, worker |
| `/epk` Press Kit Builder | ✅ Complete | GET/PUT /api/v1/epk/content (mock) | ✅ Full CRUD, photo upload, PDF export |
| `/gigs` Gig Tracker | ⚠️ Mock UI | Static mock | — (Phase 4) |
| `/finance` Finance Tracker | ⚠️ Mock UI | Static mock | — (Phase 5) |

### Per-Feature Backend Implementation

| Feature | Go Handler | Service | Repository | DB Migration |
|---|---|---|---|---|
| Health | ✅ `/api/v1/health` | ✅ | — | — |
| Settings | ✅ GET/PUT | ✅ | ✅ | 001 |
| Tracklist | ✅ CRUD + generate | ✅ | ✅ | 002 |
| Social Posts | ✅ CRUD + retry | ✅ | ✅ | 003 |
| Social Worker | ✅ publish + retry | ✅ | ✅ | 003 |
| Instagram OAuth | ✅ connect/disconnect | ✅ | ✅ | 003 |
| EPK Content | ✅ GET/PUT | ✅ | ✅ | 004 |
| EPK Photos | ✅ upload/delete | ✅ | ✅ | 004 |
| EPK Export | ✅ PDF generation | ✅ | ✅ | 004 |
| Gig Tracker | — | — | — | — (Phase 4) |
| Finance | — | — | — | — (Phase 5) |

---

## License

MIT