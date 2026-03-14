# Architecture Research

**Domain:** Self-hosted DJ career toolkit — Go modular monolith + Nuxt 4 + PostgreSQL 16 + MinIO
**Researched:** 2026-03-14
**Confidence:** HIGH (primary source: `docs/ARCHITECTURE.md` blueprint v1.0 — approved baseline)

---

## Standard Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                  Docker Compose Network: klubhub-dj-net             │
│                                                                     │
│  ┌──────────────────────────────────┐                               │
│  │  frontend  (node:22-alpine)      │  ◄── Browser :3000 (host)    │
│  │  Nuxt 4 / Vue 3 / TypeScript     │                               │
│  │  SSR + SPA; proxies /api/v1/*    │                               │
│  └───────────────┬──────────────────┘                               │
│                  │ HTTP /api/v1/* (proxy via Nitro routeRules)       │
│  ┌───────────────▼──────────────────┐   ┌──────────────────────┐   │
│  │  api  (golang:1.23-alpine/scratch│   │  storage (minio)     │   │
│  │  Go modular monolith             │──►│  :9000 internal      │   │
│  │  :8080 internal only             │S3 │  :9001 dev console   │   │
│  │  chi router + middleware         │   │  vol: storage-data   │   │
│  │  in-process scheduler (goroutine)│   └──────────────────────┘   │
│  │  embedded goose migrations       │                               │
│  └───────────────┬──────────────────┘                               │
│                  │ pgx v5                                            │
│  ┌───────────────▼──────────────────┐                               │
│  │  db  (postgres:16-alpine)        │                               │
│  │  :5432 internal only             │                               │
│  │  vol: db-data                    │                               │
│  └──────────────────────────────────┘                               │
└─────────────────────────────────────────────────────────────────────┘
         │                    │                     │
         ▼                    ▼                     ▼
   Spotify Web API    Discogs API          MusicBrainz API
   Instagram Graph API (OAuth outbound)
```

### Component Responsibilities

| Component | Responsibility | Implementation |
|-----------|----------------|----------------|
| Frontend | Render UI, user interaction, proxy `/api/v1/*` to Go API | Nuxt 4 (Vue 3 / TypeScript), Pinia state, $fetch / ofetch |
| Go API | All business logic, REST endpoints, background scheduling, migrations | chi v5 router, modular internal packages, zerolog |
| PostgreSQL 16 | Relational data, FK integrity, job queue state, JSONB config | pgx v5 driver, goose migrations embedded in binary |
| MinIO | Binary blob storage: DJ files, cover art cache, generated images, PDFs | minio-go v7, S3-compatible, single-bucket layout |
| In-process Scheduler | Poll DB for due social posts, token refresh jobs | Goroutine with ticker, no external queue |

---

## Go Module Structure (Modular Monolith)

### Internal Package Layout

```
api/
├── cmd/
│   └── api/
│       └── main.go              # Wire all modules; start router + scheduler
├── internal/
│   ├── platform/                # Cross-cutting infrastructure (no business logic)
│   │   ├── db/                  # pgx connection pool, transaction helpers
│   │   ├── storage/             # MinIO client wrapper
│   │   ├── http/                # chi router setup, shared middleware
│   │   ├── log/                 # zerolog structured logger
│   │   ├── config/              # envconfig struct-based env var loading
│   │   └── migrations/          # embedded goose migration files (go:embed)
│   │
│   ├── tracklist/               # Phase 1a — core module
│   │   ├── handler.go           # HTTP handlers (register routes on chi router)
│   │   ├── service.go           # Business logic; calls parser + generator + cover art
│   │   ├── repository.go        # PostgreSQL queries (pgx only, no cross-module joins)
│   │   ├── model.go             # Domain types: Tracklist, Track, GeneratedImage
│   │   ├── parser/
│   │   │   ├── parser.go        # FileParser interface (auto-detect + parse)
│   │   │   ├── rekordbox.go     # Rekordbox XML implementation
│   │   │   ├── serato.go        # Serato CSV implementation
│   │   │   └── traktor.go       # Traktor NML implementation
│   │   └── generator/
│   │       ├── generator.go     # Image generation (gg + imaging)
│   │       └── templates/       # Template preset definitions
│   │
│   ├── social/                  # Phase 1b
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── scheduler.go         # in-process 60s poll goroutine
│   │   ├── repository.go
│   │   ├── publisher/
│   │   │   ├── publisher.go     # SocialMediaPublisher interface
│   │   │   └── instagram.go     # Instagram Graph API implementation
│   │   └── model.go
│   │
│   ├── epk/                     # Phase 1c
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   ├── pdf/
│   │   │   ├── generator.go     # PDFGenerator interface
│   │   │   └── gofpdf.go        # gofpdf implementation
│   │   └── model.go
│   │
│   ├── gig/                     # Phase 1d — most-depended-upon module
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go             # Exports GigReader interface
│   │
│   ├── finance/                 # Phase 1e
│   │   ├── handler.go
│   │   ├── service.go           # Depends on gig.GigReader
│   │   ├── repository.go
│   │   ├── invoice/
│   │   │   └── generator.go     # Reuses PDFGenerator interface
│   │   └── model.go
│   │
│   ├── release/                 # Phase 1f
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go
│   │
│   ├── tour/                    # Phase 1g
│   │   ├── handler.go
│   │   ├── service.go           # Depends on gig.GigReader
│   │   ├── repository.go
│   │   └── model.go
│   │
│   ├── dashboard/               # Cross-cutting aggregation
│   │   ├── handler.go
│   │   └── service.go           # Calls other module services via interfaces
│   │
│   └── settings/                # User settings singleton
│       ├── handler.go
│       ├── service.go
│       └── repository.go
│
├── migrations/
│   ├── 001_initial_schema.up.sql
│   ├── 001_initial_schema.down.sql
│   └── ...
│
└── fonts/                       # Bundled Noto Sans (unicode coverage)
    ├── NotoSans-Regular.ttf
    ├── NotoSans-Bold.ttf
    └── NotoSansCJK-Regular.ttf
```

### Cross-Module Dependency Rules

```
dashboard  ──► tracklist.Service (via interface)
dashboard  ──► social.Service    (via interface)
dashboard  ──► gig.Service       (via interface)
dashboard  ──► finance.Service   (via interface)
dashboard  ──► release.Service   (via interface)
dashboard  ──► tour.Service      (via interface)

finance    ──► gig.GigReader     (auto-income on paid gig)
epk        ──► gig.GigReader     (import gig highlights)
tour       ──► gig.GigReader     (tour stop requires linked gig)

RULE: No module may query another module's database tables directly
RULE: No circular dependencies between modules
```

`gig` is the hub module. All consumers depend on `GigReader` (read-only interface), not on `gig.Service` directly:

```go
type GigReader interface {
    GetGig(ctx context.Context, id uuid.UUID) (*Gig, error)
    ListGigs(ctx context.Context, filter GigFilter) ([]*Gig, error)
}
```

### Interface Catalogue

| Interface | Package | Implementations |
|-----------|---------|-----------------|
| `FileParser` | `tracklist/parser` | `RekordboxParser`, `SeratoParser`, `TraktorParser` |
| `CoverArtProvider` | `tracklist` | `SpotifyProvider`, `DiscogsProvider`, `MusicBrainzProvider` |
| `SocialMediaPublisher` | `social/publisher` | `InstagramPublisher` (+ future `TikTokPublisher`) |
| `PDFGenerator` | `epk/pdf` | `GofpdfGenerator` (swappable to headless Chrome in v2) |
| `GigReader` | `gig` | `GigService` |

---

## Nuxt 4 ↔ Go Backend Communication

### Proxy Pattern (Nitro)

Nuxt 4 is the sole entry point for the browser. All `/api/v1/*` requests are proxied through Nuxt's Nitro server to the Go API at `http://api:8080`. The frontend never talks directly to the Go API from the browser.

```typescript
// nuxt.config.ts
export default defineNuxtConfig({
  nitro: {
    routeRules: {
      '/api/v1/**': {
        proxy: process.env.NUXT_PUBLIC_API_BASE + '/api/v1/**'
      }
    }
  }
})
```

**In development** (without Docker), use `nitro.devProxy`:

```typescript
nitro: {
  devProxy: {
    '/api/v1': {
      target: 'http://localhost:8080/api/v1',
      changeOrigin: true
    }
  }
}
```

### CORS Configuration

Because the browser routes through the Nuxt proxy (same origin), the Go API's CORS allowlist only needs to trust the Nuxt container origin, not the browser directly.

```
CORS_ORIGIN=http://127.0.0.1:3000   # default (single env var)
```

The Go API sets `Access-Control-Allow-Origin` to the value of `CORS_ORIGIN`. In Docker Compose the frontend is the only caller; CORS is a secondary concern. Rate-limiting middleware exists but is disabled in v1 (`RATE_LIMIT_ENABLED=false`).

### Frontend HTTP Client

Nuxt's built-in `$fetch` (ofetch) handles both SSR and CSR transparently — during SSR it makes direct server-to-server calls (no proxy hop needed); during CSR it routes through the browser to the Nuxt proxy. No separate Axios/axios-based setup is needed.

```typescript
// composables/useTracklist.ts
const { data } = await useFetch('/api/v1/tracklists')  // works SSR and CSR
```

---

## Recommended Repository Layout

```
klubhub-dj/                    (git root)
├── dev/                       # Nx monorepo workspace (existing)
│   ├── apps/
│   │   ├── dj/                # Nuxt 4 frontend
│   │   └── dj-e2e/            # Playwright E2E
│   └── ...
│
├── api/                       # Go backend (to be created)
│   ├── cmd/api/main.go
│   ├── internal/              # see module layout above
│   ├── migrations/
│   ├── fonts/
│   ├── Dockerfile
│   └── go.mod
│
├── scripts/
│   ├── backup.sh
│   └── restore.sh
│
├── docker-compose.yml
├── docker-compose.dev.yml     # MinIO console, hot reload overrides
├── .env.example
└── UPGRADING.md
```

---

## Architectural Patterns

### Pattern 1: Module-per-Feature (Modular Monolith)

**What:** Each business domain (tracklist, gig, social, etc.) is a self-contained Go package with its own `handler.go` / `service.go` / `repository.go` / `model.go`. Modules communicate only through exported Go interfaces, never through shared database queries.

**When to use:** Always — this is the foundational pattern for the entire Go API.

**Trade-offs:** Slightly more boilerplate per module than a flat structure. Pays off when features are added in phases and when module extraction to services is planned for v3+.

```go
// Each module registers its own routes on the shared chi router
func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Post("/api/v1/tracklists/upload", h.UploadFile)
    r.Get("/api/v1/tracklists", h.ListTracklists)
    r.Get("/api/v1/tracklists/{id}", h.GetTracklist)
    r.Put("/api/v1/tracklists/{id}", h.UpdateTracklist)
    r.Delete("/api/v1/tracklists/{id}", h.DeleteTracklist)
}
```

### Pattern 2: Interface-First for External Adapters

**What:** Define Go interfaces for all integration points (cover art providers, social publishers, PDF generators) before implementing them. Wire concrete implementations in `main.go`.

**When to use:** Any code that calls an external API or depends on a swappable implementation. Enables testing with fakes and future provider additions without touching business logic.

**Trade-offs:** Requires defining interface contracts up front. Cost is low (a few lines); benefit is testability and extensibility.

```go
// tracklist/cover_art.go
type CoverArtProvider interface {
    FetchCoverArt(ctx context.Context, artist, title string) ([]byte, error)
}

// main.go wires the chain
providers := []tracklist.CoverArtProvider{
    spotify.NewProvider(cfg.SpotifyClientID, cfg.SpotifyClientSecret),
    discogs.NewProvider(cfg.DiscogsToken),
    musicbrainz.NewProvider(),
}
```

### Pattern 3: DB-Backed Job Queue (No External Broker)

**What:** The social post scheduler is an in-process goroutine that polls the `scheduled_posts` table every 60 seconds for due posts. Retry state is persisted in PostgreSQL. `FOR UPDATE SKIP LOCKED` prevents double-processing.

**When to use:** Single-instance, low-volume async work (1–2 social posts per hour at most). Avoids Redis/Asynq dependency.

**Trade-offs:** Not suitable for high-throughput or multi-instance deployments. For v1 single-user self-hosted, this is the correct level of complexity.

```go
func (s *Scheduler) Run(ctx context.Context) {
    ticker := time.NewTicker(s.interval)
    for {
        select {
        case <-ticker.C:
            s.processDuePosts(ctx)
        case <-ctx.Done():
            return
        }
    }
}
```

### Pattern 4: MinIO-First for Binary Assets

**What:** When persisting a user-generated asset (image, PDF, source file), write to MinIO first, then write the record to PostgreSQL. On PostgreSQL write failure, attempt MinIO delete (best-effort) and surface the error.

**When to use:** Any handler that stores a file. Prevents orphaned records pointing to non-existent objects.

**Trade-offs:** MinIO writes that succeed but PostgreSQL writes that fail leave orphaned blobs. A background reconciler handles cleanup on startup and via manual trigger endpoint.

### Pattern 5: Soft Deletion + Optimistic Concurrency

**What:** All tables have `deleted_at TIMESTAMPTZ` (soft delete) and `updated_at TIMESTAMPTZ`. All list queries filter `WHERE deleted_at IS NULL`. All `PUT` endpoints accept `updated_at` in the request body and return `409 Conflict` if it does not match the current DB value.

**When to use:** All entities — enforced universally.

**Trade-offs:** List queries must always include the `deleted_at IS NULL` predicate (use partial indexes to keep this fast). Optimistic concurrency requires the frontend to track `updated_at` on all edit forms.

---

## Data Flow

### Standard CRUD Request Flow

```
Browser
  │
  │ HTTP request (same-origin to :3000)
  ▼
Nuxt 4 Frontend (Nitro proxy)
  │
  │ HTTP /api/v1/* forwarded to api:8080
  ▼
chi Router → Middleware (request-id, CORS, logging, validation)
  │
  ▼
Module Handler (e.g. tracklist/handler.go)
  │
  ├──► Module Service (business logic, interface orchestration)
  │         │
  │         ├──► Module Repository (pgx queries → PostgreSQL)
  │         └──► platform/storage (minio-go → MinIO)
  │
  ◄── JSON response with data envelope
  │
  ▼
Browser
```

### Cover Art Async Fetch Flow

```
POST /api/v1/tracklists/upload
  └── Parse file → INSERT tracklist + tracks → HTTP 200 { tracklist_id }

UI polls GET /api/v1/tracklists/:id/covers/status (every 2s)
  └── Goroutine pool (5–10 workers, COVER_ART_CONCURRENCY env)
       ├── Check MinIO cache → hit: done
       └── Miss: Spotify → Discogs → MusicBrainz → placeholder
                └── Write to MinIO → UPDATE tracks.cover_art_path

When pending_count == 0 → UI stops polling, renders full tracklist
```

### Image Generation — Long Job Flow

```
POST /api/v1/tracklists/:id/generate
  ├── ≤30 tracks, solid bg → synchronous → HTTP 200 { download_url }
  └── >30 tracks or mosaic → goroutine → HTTP 202 { job_id }

GET /api/v1/jobs/:job_id
  └── { status: pending|processing|done|failed, result_url? }
      (backed by in-memory map + generated_images DB record)
```

### Social Post Scheduler Flow

```
Scheduler goroutine (every SCHEDULER_INTERVAL, default 60s)
  └── SELECT * FROM scheduled_posts
        WHERE status = 'scheduled' AND scheduled_at <= NOW()
        ORDER BY scheduled_at ASC LIMIT 10
        FOR UPDATE SKIP LOCKED
  └── For each post:
       ├── Publish via SocialMediaPublisher interface
       ├── Success → UPDATE status = 'published', published_at = NOW()
       └── Failure → retry: +1m / +5m / +15m → permanently_failed after 3 attempts
```

### Object Storage Write Order

```
Any file-producing operation:
  1. Write blob → MinIO (buckets/prefix/object)
  2. Write record → PostgreSQL (path stored in DB)
     ├── Success → return response
     └── Failure → attempt MinIO delete (best-effort) → return error

On startup: background reconciler checks for orphaned MinIO objects
```

---

## Build Order (Phase Dependencies)

The recommended build sequence follows hard technical dependencies:

```
Phase 0: Infrastructure scaffolding
  ├── docker-compose.yml (db + storage + api + frontend containers)
  ├── Go module init (api/go.mod)
  ├── platform/ packages (db, storage, http, config, log, migrations)
  └── First passing health endpoint: GET /api/v1/health

Phase 1a: Tracklist module (independent, flagship feature)
  ├── Requires: platform/ complete
  ├── migrations/001_tracklists.sql
  └── tracklist/ (model, repository, parser, generator, handler, service)

Phase 1b: Social module (depends on tracklist for image attachment)
  ├── Requires: tracklist/ complete (generated_images)
  ├── migrations/002_social.sql
  └── social/ (model, repository, publisher, scheduler, handler, service)

Phase 1c: EPK module (depends on gig for highlights — defer gig dep to Phase 1d)
  ├── Requires: platform/ complete (standalone module)
  ├── migrations/003_epk.sql
  └── epk/ (model, repository, pdf generator, handler, service)

Phase 1d: Gig module (becomes shared hub — build before finance/tour)
  ├── Requires: platform/ complete
  ├── migrations/004_gigs.sql
  └── gig/ (model, repository, handler, service, GigReader interface export)

Phase 1e: Finance module (depends on gig.GigReader)
  ├── Requires: gig/ complete (GigReader interface)
  ├── migrations/005_finance.sql
  └── finance/ (model, repository, invoice generator, handler, service)

Phase 1f: Release module (independent of other modules)
  ├── Requires: platform/ complete
  ├── migrations/006_releases.sql
  └── release/ (model, repository, handler, service)

Phase 1g: Tour module (depends on gig.GigReader)
  ├── Requires: gig/ complete (GigReader interface)
  ├── migrations/007_tours.sql
  └── tour/ (model, repository, handler, service)

Phase 1h: Dashboard (depends on all module services via interfaces)
  ├── Requires: all modules complete
  └── dashboard/ (aggregation handler + service)
```

**Critical ordering constraint:** `gig/` must be built before `finance/`, `tour/`, and full `epk/` (gig highlights). `platform/` must be built before any module.

---

## Deployment Architecture

### Docker Compose Startup Order

```
db (postgres:16-alpine)
  └── healthcheck: pg_isready
      └── HEALTHY ──► api starts
                      └── goose migrations run
                          └── api healthcheck passes
                              └── HEALTHY ──► frontend starts

storage (minio)
  └── healthcheck: curl /minio/health/live
      └── HEALTHY ──► api starts (parallel with db)
```

All services use `depends_on: condition: service_healthy`. API exits non-zero on migration failure. Frontend will not start until API is healthy.

### Port Exposure

| Service | Internal Port | Host Binding |
|---------|--------------|--------------|
| frontend | 3000 | `${BIND_ADDRESS:-127.0.0.1}:3000` |
| api | 8080 | `${BIND_ADDRESS:-127.0.0.1}:8080` (internal only in production) |
| db | 5432 | Not exposed to host |
| storage | 9000 | Not exposed to host (API is sole gateway) |
| storage console | 9001 | Dev compose only |

### MinIO Bucket Structure

```
klubhub-dj/                    (single bucket)
├── source-files/              {tracklist_id}/{filename}
├── cover-art/                 {artist_slug}/{title_slug}.{ext}
├── generated-images/          {tracklist_id}/{image_id}.png
├── epk-assets/                {epk_profile_id}/{filename}
├── tour-documents/            {tour_id}/{filename}
└── user-assets/               {filename}
```

v3 guardrail: prefix all paths with `users/{user_id}/` without restructuring.

---

## Scaling Considerations

This is a single-user self-hosted tool. Traditional scaling does not apply. The architectural focus is on operational simplicity and future extensibility to multi-user (v3).

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 1 user (v1 target) | Current architecture: modular monolith, DB-backed scheduler, single Docker Compose |
| 2–10 users (v2 multi-install) | Same: each user runs their own Docker Compose instance |
| Multi-tenant v3 | Activate user_id filtering, add auth middleware, promote scheduler to Asynq/Redis |

### v3 Multi-User Guardrails Built Into v1

The following are already implemented in v1 to avoid painful refactoring later:

| Guardrail | v1 State | v3 Activation |
|-----------|----------|---------------|
| `user_id` column (nullable) on all tables | Present, not queried | NOT NULL, indexed |
| MinIO path with user prefix | Uses singleton ID from day 1 | No restructuring |
| Repository `context.Context` parameter | Context carries no user | Middleware injects user |
| `WHERE user_id = $1 OR $1 IS NULL` query stubs | Always NULL | Pass authenticated user |
| Rate-limiting middleware | Present, `RATE_LIMIT_ENABLED=false` | Switch to per-user key |
| `/api/v1` versioning | All endpoints prefixed | Breaking changes go to `/api/v2` |

---

## Anti-Patterns

### Anti-Pattern 1: Cross-Module Database Queries

**What people do:** A handler in the `finance` module writes a raw SQL query that joins `finance.income_entries` with `tracklist.tracks` directly.

**Why it's wrong:** Defeats module isolation. Any schema change in `tracklist` silently breaks `finance`. Makes future module extraction impossible without a rewrite.

**Do this instead:** `finance.service` calls `tracklist.Service` via the defined interface. The tracklist module is responsible for its own data.

### Anti-Pattern 2: Writing to PostgreSQL Before MinIO

**What people do:** Save the record to PostgreSQL with a file path, then upload the blob to MinIO.

**Why it's wrong:** If MinIO write fails after the DB write succeeds, the record points to a non-existent object. The user sees a broken link with no recovery path.

**Do this instead:** MinIO write first, PostgreSQL write second (see Pattern 4). On DB failure, attempt MinIO cleanup.

### Anti-Pattern 3: Synchronous Cover Art Fetching in Upload Handler

**What people do:** The upload handler awaits all cover art API calls before returning HTTP 200.

**Why it's wrong:** 5–10 serial external API calls at 200ms each = 1–2s minimum latency per track × 50 tracks = response timeout. Blocks the user at the most critical moment (after upload).

**Do this instead:** Return HTTP 200 immediately after parse + DB insert. Fire goroutine pool for cover art. Frontend polls `/covers/status`.

### Anti-Pattern 4: Skipping Soft Deletion

**What people do:** Use hard DELETE for tracklists or gigs because it's simpler.

**Why it's wrong:** Cascade deletes can destroy linked data (gigs linked to tracklists, income entries linked to gigs). Recovery is impossible without a backup restore.

**Do this instead:** Set `deleted_at = NOW()` on all deletes. Enforce `WHERE deleted_at IS NULL` on all list queries. Use partial indexes to keep performance identical.

### Anti-Pattern 5: Embedding Business Logic in Handlers

**What people do:** Put database queries and domain decisions directly in chi handler functions.

**Why it's wrong:** Handlers become untestable without HTTP context. Logic can't be reused by the scheduler, dashboard service, or CLI scripts.

**Do this instead:** Handlers only parse/validate input, call the service layer, and serialize output. All logic lives in `service.go`. All DB access in `repository.go`.

---

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Spotify Web API | Client credentials OAuth, REST | Cover art fallback tier 1; token TTL managed by Go client |
| Discogs API | API token header, REST | Cover art fallback tier 2; 60 req/min rate limit |
| MusicBrainz API | No auth, REST | Cover art fallback tier 3; 1 req/s rate limit (NFR-402) |
| Instagram Graph API | User OAuth (Facebook Business), REST | Social scheduling; tokens encrypted AES-256-GCM in DB |

### Internal Module Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| frontend ↔ api | HTTP REST /api/v1/* via Nuxt proxy | Browser never hits api:8080 directly |
| api ↔ PostgreSQL | pgx v5 native protocol, connection pool (min 2, max 10) | All queries parameterized, no raw string interpolation |
| api ↔ MinIO | minio-go v7 S3 SDK, presigned URLs not used (API is proxy) | Single bucket, prefix-based namespacing |
| tracklist ↔ cover art providers | `CoverArtProvider` interface, fallback chain | Concurrent goroutines per track, 5s connect / 30s read timeout |
| finance ↔ gig | `GigReader` interface (read-only) | Auto-income on payment_status → paid transition |
| tour ↔ gig | `GigReader` interface (read-only) | Tour stop requires a valid gig ID |
| epk ↔ gig | `GigReader` interface (read-only) | Import gig highlights for EPK |
| dashboard ↔ all modules | Per-module service interfaces | Aggregation only; no direct DB queries |

---

## Sources

- `docs/ARCHITECTURE.md` — KlubHub DJ Architecture Blueprint v1.0 (2026-03-11, approved baseline) — HIGH confidence, authoritative
- `docs/KlubHub_DJ_NFR_v1.0.md` — Referenced throughout architecture decisions
- `docs/KlubHub_DJ_BRD_v2.0_Platform.md` — BRD stack decisions (Go / Nuxt / PostgreSQL / MinIO)
- Go documentation: https://go.dev/blog/organizing-go-code — package naming and organization principles
- Nuxt 4 Nitro proxy pattern: routeRules / devProxy (standard Nuxt 3/4 pattern, backward compatible)

---

*Architecture research for: KlubHub DJ — self-hosted DJ career toolkit*
*Researched: 2026-03-14*
