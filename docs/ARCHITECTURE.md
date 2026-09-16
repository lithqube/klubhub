# KlubHub DJ — Architecture Blueprint

> **Historical baseline:** This document predates the v1.0.0 Garage migration.
> References to MinIO and its Compose examples describe the superseded design.
> The supported deployment uses Garage through its S3 API; see `README.md`,
> `CONFIGURATION.md`, and `SELF-HOSTING.md`. `minio-go` remains only as the
> protocol client library.

**Version:** 1.0
**Date:** 2026-03-11
**Status:** Approved baseline for implementation
**Companion docs:** BRD v2.0, BRD Addendum v2.0-A, NFR v1.0

---

## Table of Contents

1. [Architecture Decision Summary](#1-architecture-decision-summary)
2. [System Architecture](#2-system-architecture)
3. [Module Boundaries](#3-module-boundaries)
4. [Integration & Communication](#4-integration--communication)
5. [Data Architecture](#5-data-architecture)
6. [Cross-Cutting Concerns](#6-cross-cutting-concerns)
7. [Technology Stack](#7-technology-stack)
8. [Repository Layout](#8-repository-layout)
9. [Deployment Architecture](#9-deployment-architecture)
10. [v3 Multi-User Guardrails](#10-v3-multi-user-guardrails)

---

## 1. Architecture Decision Summary

| Decision | Choice | Driving NFR/FR |
|---|---|---|
| Architecture style | Modular monolith | NFR-800, NFR-301, BRD §5 |
| Backend language | Go | BRD §5, NFR-403 (static binary) |
| Frontend framework | Nuxt 3 (Vue 3 / TypeScript) | BRD §5, existing scaffold |
| Database | PostgreSQL 16 | BRD §5, NFR-103 (durability) |
| Object storage | MinIO (S3-compatible) | BRD §5, NFR-104, NFR-511 |
| API style | REST + JSON | NFR-801 (versioning), simplicity |
| Async strategy | In-process goroutines + DB-backed job queue | NFR-301 (no external queue) |
| Deployment | Docker Compose, single command | NFR-700 |
| Auth | None in v1 | BRD §2 |
| Image generation | Pure Go (gg + imaging) | NFR-403 (image size), NFR-207 (Unicode) |

---

## 2. System Architecture

### 2.1 Architecture Style

**Modular Monolith.** A single Go process owns all business logic, organized into clean internal packages per module (tracklist, social, epk, gig, finance, release, tour). Modules communicate through Go interfaces, never through shared database queries across boundaries.

**Why not microservices?**
- Single-user, single-host deployment (NFR-301) — no scaling benefit
- No message broker dependency (NFR-402 — zero paid deps)
- Operational simplicity is a first-class requirement for a self-hosted tool
- NFR-800 explicitly mandates modular monolith; extraction to services is a v3+ concern

**Why not a simple monolith?**
- 7 independent business capabilities ship in 7 phases; hard boundaries prevent regression
- v3 multi-user extraction requires module-level isolation to be viable

### 2.2 C4 Context Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         KlubHub DJ System                           │
│                                                                     │
│  ┌─────────────┐    HTTP/JSON     ┌──────────────────────────────┐  │
│  │  DJ (User)  │◄────────────────►│    Nuxt 3 Frontend           │  │
│  │  Browser    │                  │    (SSR + SPA)               │  │
│  └─────────────┘                  └──────────┬───────────────────┘  │
│                                             │ HTTP /api/v1          │
│                                  ┌──────────▼───────────────────┐  │
│                                  │    Go API (Modular Monolith)  │  │
│                                  │    + In-process Scheduler     │  │
│                                  └──┬───────┬────────────┬──────┘  │
│                                     │       │            │         │
│                             ┌───────▼─┐ ┌───▼──┐ ┌──────▼──────┐  │
│                             │Postgres │ │MinIO │ │ Migrations  │  │
│                             │   16    │ │(S3)  │ │ (embedded)  │  │
│                             └─────────┘ └──────┘ └─────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
         │                         │                    │
         ▼                         ▼                    ▼
   ┌───────────┐           ┌───────────────┐    ┌────────────────┐
   │ Spotify   │           │ Discogs API   │    │ MusicBrainz    │
   │ Web API   │           │               │    │ API            │
   └───────────┘           └───────────────┘    └────────────────┘
         │
         ▼
   ┌───────────────────────┐
   │ Instagram Graph API   │
   │ (Facebook Business)   │
   └───────────────────────┘
```

### 2.3 C4 Container Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Docker Compose Network: klubhub-dj-net                                     │
│                                                                             │
│  ┌──────────────────────────────────┐                                       │
│  │  klubhub-dj-frontend             │                                       │
│  │  Image: node:22-alpine           │                                       │
│  │  Port: 3000 (host-bound)         │  ◄── Browser (HTTP)                  │
│  │  Framework: Nuxt 3 / Vue 3       │                                       │
│  │  Role: SSR + SPA, proxies /api   │                                       │
│  └───────────────┬──────────────────┘                                       │
│                  │ /api/v1/* (proxied)                                       │
│  ┌───────────────▼──────────────────┐      ┌────────────────────────────┐  │
│  │  klubhub-dj-api                  │      │  klubhub-dj-storage        │  │
│  │  Image: golang:1.23-alpine       │      │  Image: minio/minio        │  │
│  │  Port: 8080 (internal only)      │─────►│  Port: 9000 (internal)     │  │
│  │  Role: REST API + Scheduler      │ S3   │  Port: 9001 (dev only)     │  │
│  │  Modules: 7 feature packages     │ API  │  Volume: storage-data      │  │
│  │  Migrations: embedded/goose      │      └────────────────────────────┘  │
│  └───────────────┬──────────────────┘                                       │
│                  │ pgx driver                                                │
│  ┌───────────────▼──────────────────┐                                       │
│  │  klubhub-dj-db                   │                                       │
│  │  Image: postgres:16-alpine       │                                       │
│  │  Port: 5432 (internal only)      │                                       │
│  │  Volume: db-data                 │                                       │
│  └──────────────────────────────────┘                                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.4 Startup Ordering

```
postgres (healthy) ──► api (migrations, then serve) ──► frontend (proxy ready)
minio   (healthy) ──┘
```

All services use Docker `depends_on` with `condition: service_healthy`. The API exits with non-zero if migrations fail (NFR-703).

---

## 3. Module Boundaries

### 3.1 Backend Package Structure

```
api/
├── cmd/
│   └── api/
│       └── main.go              # Wire everything together
├── internal/
│   ├── platform/                # Cross-cutting (no business logic)
│   │   ├── db/                  # pgx connection pool, transaction helpers
│   │   ├── storage/             # MinIO client wrapper
│   │   ├── http/                # Router setup, middleware, error helpers
│   │   ├── log/                 # Structured logger (zerolog)
│   │   ├── config/              # Env var loading (envconfig)
│   │   └── migrations/          # Embedded migration files
│   │
│   ├── tracklist/               # Phase 1a
│   │   ├── handler.go           # HTTP handlers
│   │   ├── service.go           # Business logic + CoverArtProvider calls
│   │   ├── repository.go        # PostgreSQL queries
│   │   ├── parser/
│   │   │   ├── parser.go        # FileParser interface
│   │   │   ├── rekordbox.go     # Rekordbox implementation
│   │   │   ├── serato.go        # Serato implementation
│   │   │   └── traktor.go       # Traktor implementation
│   │   ├── generator/
│   │   │   ├── generator.go     # Image generation (gg + imaging)
│   │   │   └── templates/       # Template definitions
│   │   └── model.go             # Tracklist, Track, GeneratedImage types
│   │
│   ├── social/                  # Phase 1b
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── scheduler.go         # In-process cron scheduler
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
│   ├── gig/                     # Phase 1d
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go
│   │
│   ├── finance/                 # Phase 1e
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   ├── invoice/
│   │   │   └── generator.go     # Invoice PDF (reuses PDFGenerator interface)
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
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── model.go
│   │
│   ├── dashboard/               # Aggregation across modules
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
└── fonts/                       # Bundled Noto Sans (Unicode coverage)
    ├── NotoSans-Regular.ttf
    ├── NotoSans-Bold.ttf
    └── NotoSansCJK-Regular.ttf
```

### 3.2 Cross-Module Dependency Rules

```
dashboard  ──► tracklist.Service (interface)
dashboard  ──► social.Service    (interface)
dashboard  ──► gig.Service       (interface)
dashboard  ──► finance.Service   (interface)
dashboard  ──► release.Service   (interface)
dashboard  ──► tour.Service      (interface)

finance    ──► gig.Service       (interface)  # auto-income on paid gig
epk        ──► gig.Service       (interface)  # import gig highlights
tour       ──► gig.Service       (interface)  # tour stop requires linked gig

PROHIBITED: any module querying another module's tables directly
PROHIBITED: circular dependencies between modules
```

The `gig` module is the most-depended-upon. Its public interface is:

```go
type GigReader interface {
    GetGig(ctx context.Context, id uuid.UUID) (*Gig, error)
    ListGigs(ctx context.Context, filter GigFilter) ([]*Gig, error)
}
```

Modules that need gig data depend on `GigReader`, not on `gig.Service` concretely, keeping coupling loose.

### 3.3 Interface Catalogue

| Interface | Package | Implementations |
|---|---|---|
| `FileParser` | `tracklist/parser` | `RekordboxParser`, `SeratoParser`, `TraktorParser` |
| `CoverArtProvider` | `tracklist` | `SpotifyProvider`, `DiscogsProvider`, `MusicBrainzProvider` |
| `SocialMediaPublisher` | `social/publisher` | `InstagramPublisher` (+ future: `TikTokPublisher`) |
| `PDFGenerator` | `epk/pdf` | `GofpdfGenerator` (+ future: `ChromedpGenerator`) |
| `GigReader` | `gig` | `GigService` |

---

## 4. Integration & Communication

### 4.1 Synchronous vs. Asynchronous

| Operation | Pattern | Rationale |
|---|---|---|
| All CRUD operations | Sync HTTP REST | Simple request-response; no fan-out |
| File parse + DB insert | Sync | Must complete before UI renders tracklist |
| Cover art fetching | **Async, concurrent** | I/O-bound; 5–10 concurrent outbound calls; returns progressively to frontend via polling or SSE |
| Image generation | Sync (fast) / Async (>30 tracks) | Short generations are synchronous; long ones return a job ID |
| Social post publishing | **Async, scheduled** | In-process cron polls DB every 60s for due posts |
| Token refresh | **Async, scheduled** | Daily background job |
| Orphan reconciliation | **Async, on-startup + manual** | Non-critical; must not block API startup |
| PDF generation | Sync | Under 15s max; acceptable as synchronous |

### 4.2 Cover Art — Async Fetch Flow

```
POST /api/v1/upload
 └── Parse file ──► INSERT tracklist + tracks ──► HTTP 200 {tracklist_id}
                                                        │
GET /api/v1/tracklists/:id/covers/status ◄── UI polls ◄┘
 └── Launch goroutine pool (5-10 workers)
      ├── Worker: check MinIO cache ──► cache hit: done
      └── Worker: Spotify → Discogs → MusicBrainz → placeholder
                  └── store in MinIO → UPDATE track.cover_art_path
                  └── UPDATE tracks.cover_art_source
```

Frontend polls `GET /api/v1/tracklists/:id/covers/status` until `pending_count == 0`. Individual track cover art updates are immediately reflected in subsequent `GET /tracklists/:id` calls.

**Why polling, not WebSockets/SSE?**
Cover art fetch is a one-time batch operation. Polling at 2-second intervals is simple, reliable, and avoids persistent connection management for a single-user app.

### 4.3 Image Generation — Long Job Pattern

For tracklists ≤ 30 tracks at solid/custom background: **synchronous** — POST returns the download URL directly.

For mosaic or large tracklists (>30 tracks):

```
POST /api/v1/tracklists/:id/generate
 └── Validate params
 └── Launch goroutine ──► returns HTTP 202 { job_id }

GET /api/v1/jobs/:job_id
 └── Returns { status: "pending|processing|done|failed", result_url? }
     (Backed by in-memory map + DB record in generated_images)
```

### 4.4 Social Post Scheduler

The scheduler is an in-process goroutine, not an external queue:

```go
// Polls every 60s (SCHEDULER_INTERVAL env var)
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

// processDuePosts queries:
// SELECT * FROM scheduled_posts
//   WHERE status = 'scheduled'
//     AND scheduled_at <= NOW()
//   ORDER BY scheduled_at ASC
//   LIMIT 10
//   FOR UPDATE SKIP LOCKED   -- prevents double-processing in future multi-process scenarios
```

Retry logic follows NFR-102: 1min / 5min / 15min backoff. After 3 failures → `permanently_failed`.

**Why no Redis/Asynq?** NFR-402 (zero external paid deps), NFR-301 (single instance), NFR-404 (idle CPU). A PostgreSQL-backed scheduler is sufficient for the 1-post-per-minute volume and provides durability without a message broker.

### 4.5 API Design

**Style:** REST, JSON, `/api/v1/` prefix (NFR-801).

**No GraphQL:** Over-engineered for a single-consumer (own frontend) with straightforward data access patterns. REST is simpler to implement, document, and debug.

**No gRPC:** Not needed for a browser-facing API. gRPC is useful for inter-service communication; we have no services to communicate between.

**Response envelope:**

```json
// Success
{ "data": { ... }, "meta": { "page": 1, "total": 42 } }

// Error (NFR-807)
{ "error": { "code": "TRACKLIST_PARSE_FAILED", "message": "...", "details": [] } }
```

**Pagination:** Cursor-based on `(created_at, id)`, default page size 25, max 100 (NFR-305).

**Optimistic concurrency:** All PUT requests include `updated_at` in body; API returns 409 on mismatch (FR-046a).

**OpenAPI:** Full OpenAPI 3.x spec generated from code (using `swaggo/swag` annotations or equivalent) and committed at `api/openapi.yaml`.

---

## 5. Data Architecture

### 5.1 Database Technology

**PostgreSQL 16** — justified by:
- Relational data with FK constraints between modules (NFR-103, data integrity)
- JSONB for flexible config fields (templates, social links, checklists) without a separate document store
- Partial indexes (NFR-306 — `scheduled_posts WHERE status='scheduled'`)
- Cursor-based pagination on timestamp + UUID composite (NFR-305)
- Native enum types for status fields
- `pg_isready` health check support (NFR-602)
- `pg_dump` backup support (NFR-107)

### 5.2 Entity-Relationship Diagram

```
┌─────────────────┐
│  user_settings  │  (singleton)
│─────────────────│
│ id (UUID PK)    │
│ dj_name         │
│ logo_path       │
│ default_colors  │
│ default_template│
│ visible_fields  │
│ social_links    │ (JSONB)
│ bio_short       │
│ bio_long        │
│ contact_info    │
│ invoice_prefix  │
│ user_id *       │ (v3 guardrail)
└─────────────────┘

                    ┌──────────────────────┐
                    │      tracklists      │
                    │──────────────────────│
                    │ id (UUID PK)         │
                    │ name                 │
                    │ source_format (enum) │
                    │ source_file_path     │
                    │ dj_name              │
                    │ event_name           │
                    │ event_date           │
                    │ gig_id (FK, NULL) ───┼──────────────────────┐
                    │ user_id *            │                      │
                    │ created_at           │                      │
                    │ updated_at           │                      │
                    │ deleted_at           │                      │
                    └──────────┬───────────┘                      │
                               │ 1:N                              │
              ┌────────────────┴──────────┐                       │
              │                           │                       │
    ┌─────────▼──────────┐  ┌─────────────▼──────────┐           │
    │       tracks       │  │    generated_images    │           │
    │────────────────────│  │────────────────────────│           │
    │ id (UUID PK)       │  │ id (UUID PK)           │           │
    │ tracklist_id (FK)  │  │ tracklist_id (FK)      │           │
    │ position           │  │ format (enum)          │           │
    │ artist             │  │ template_id            │           │
    │ title              │  │ config_json (JSONB)    │           │
    │ bpm                │  │ output_path            │           │
    │ key                │  │ user_id *              │           │
    │ label              │  │ created_at             │           │
    │ duration           │  │ updated_at             │           │
    │ played_at          │  │ deleted_at             │           │
    │ cover_art_path     │  └────────────────────────┘           │
    │ cover_art_source   │                                       │
    │ (enum)             │                                       │
    │ user_id *          │                                       │
    │ created_at         │                                       │
    │ updated_at         │                                       │
    │ deleted_at         │                                       │
    └────────────────────┘                                       │
                                                                 │
┌──────────────────────────────────────────────────────────┐    │
│                           gigs                           │◄───┘
│──────────────────────────────────────────────────────────│
│ id (UUID PK)                                             │
│ date                                                     │
│ venue_name, city, country                                │
│ event_name                                               │
│ promoter_name, promoter_email, promoter_phone            │
│ fee (DECIMAL 10,2), currency (ISO 4217)                  │
│ payment_status (enum: unpaid|deposit_paid|paid|overdue|waived) │
│ gig_status (enum: inquiry|confirmed|advanced|played|cancelled) │
│ notes                                                    │
│ tracklist_id (FK, NULL) ─────────────────────────────────┼──► tracklists
│ user_id *                                                │
│ created_at, updated_at, deleted_at                       │
└──────────────────────┬───────────────────────────────────┘
                       │ 1:N (optional)
       ┌───────────────┤
       │               │
┌──────▼────────┐  ┌───▼──────────────┐
│ income_entries│  │  expense_entries  │
│───────────────│  │──────────────────│
│ id (UUID PK)  │  │ id (UUID PK)     │
│ amount        │  │ amount           │
│ currency      │  │ currency         │
│ category(enum)│  │ category(enum)   │
│ date          │  │ date             │
│ description   │  │ description      │
│ gig_id (FK,N) │  │ tour_id (FK,N) ──┼──► tours
│ auto_generated│  │ user_id *        │
│ user_id *     │  │ created_at       │
│ created_at    │  │ updated_at       │
│ updated_at    │  │ deleted_at       │
│ deleted_at    │  └──────────────────┘
└──────┬────────┘
       │ 1:1 (optional)
┌──────▼────────┐
│   invoices    │
│───────────────│
│ id (UUID PK)  │
│ gig_id (FK)   │
│ invoice_number│
│ issued_date   │
│ due_date      │
│ amount        │
│ currency      │
│ status (enum) │
│ pdf_path      │
│ user_id *     │
│ created_at    │
│ updated_at    │
│ deleted_at    │
└───────────────┘

┌──────────────────────────────┐
│         social_accounts      │
│──────────────────────────────│
│ id (UUID PK)                 │
│ platform (enum: instagram)   │
│ account_name                 │
│ access_token (AES-256-GCM)   │
│ token_expiry                 │
│ status (enum: connected|     │
│         disconnected)        │
│ user_id *                    │
│ created_at, updated_at       │
└──────────────┬───────────────┘
               │ 1:N
┌──────────────▼───────────────┐
│       scheduled_posts        │
│──────────────────────────────│
│ id (UUID PK)                 │
│ social_account_id (FK)       │
│ image_path                   │
│ caption, hashtags            │
│ scheduled_at, timezone       │
│ status (enum: draft|scheduled│
│  |publishing|published|failed│
│  |permanently_failed)        │
│ published_at                 │
│ error_log, retry_count       │
│ next_retry_at                │
│ user_id *                    │
│ created_at, updated_at       │
└──────────────────────────────┘

┌──────────────────────────┐
│       epk_profiles       │
│──────────────────────────│
│ id (UUID PK)             │
│ bio_short, bio_long      │
│ press_photos (JSONB)     │
│ tech_rider_text          │
│ stage_plot_path          │
│ social_links (JSONB)     │
│ gig_highlights (JSONB)   │
│ press_quotes (JSONB)     │
│ contact_info             │
│ user_id *                │
│ created_at, updated_at   │
│ deleted_at               │
└──────────┬───────────────┘
           │ 1:N
┌──────────▼───────────────┐
│       epk_exports        │
│──────────────────────────│
│ id (UUID PK)             │
│ epk_profile_id (FK)      │
│ pdf_path                 │
│ sections_included (JSONB)│
│ user_id *                │
│ generated_at             │
│ deleted_at               │
└──────────────────────────┘

┌────────────────────────────────────┐
│            releases                │
│────────────────────────────────────│
│ id (UUID PK)                       │
│ title, artists (TEXT csv)          │
│ label, distributor                 │
│ release_date                       │
│ format (enum: digital|vinyl|both)  │
│ status (enum: demo_sent|accepted|  │
│  mastering|artwork|promo|released) │
│ platform_links (JSONB)             │
│ notes                              │
│ user_id *                          │
│ created_at, updated_at, deleted_at │
└──────────────┬─────────────────────┘
               │ 1:N
    ┌──────────┼──────────────────┐
    │          │                  │
┌───▼──────┐  ┌▼────────────────┐ ┌▼──────────────────────┐
│ release_ │  │ release_        │ │ promo_checklist_      │
│ deadlines│  │ promo_tasks     │ │ templates             │
│──────────│  │─────────────────│ │───────────────────────│
│ id       │  │ id              │ │ id (UUID PK)          │
│release_id│  │ release_id (FK) │ │ name                  │
│ name     │  │ task_name       │ │ items (JSONB)         │
│ due_date │  │ due_date        │ │ is_default            │
│ completed│  │ completed       │ │ user_id *             │
└──────────┘  └─────────────────┘ └───────────────────────┘

┌──────────────────────────────────┐
│             tours                │
│──────────────────────────────────│
│ id (UUID PK)                     │
│ name                             │
│ start_date, end_date             │
│ notes                            │
│ user_id *                        │
│ created_at, updated_at, deleted_at│
└───────────────┬──────────────────┘
                │ 1:N
┌───────────────▼──────────────────┐
│           tour_stops             │
│──────────────────────────────────│
│ id (UUID PK)                     │
│ tour_id (FK)                     │
│ gig_id (FK, REQUIRED)            │──► gigs
│ travel_details (JSONB)           │
│ accommodation (JSONB)            │
│ checklist (JSONB)                │
│ documents (JSONB)                │
│ user_id *                        │
│ created_at, updated_at, deleted_at│
└──────────────────────────────────┘
```

*`*` = nullable UUID column present in v1 for v3 multi-user guardrail (NFR-512)*

### 5.3 Index Strategy (NFR-306)

```sql
-- Foreign keys (all modules)
CREATE INDEX ON tracks(tracklist_id);
CREATE INDEX ON generated_images(tracklist_id);
CREATE INDEX ON scheduled_posts(social_account_id);
CREATE INDEX ON income_entries(gig_id);
CREATE INDEX ON expense_entries(tour_id);
CREATE INDEX ON invoices(gig_id);
CREATE INDEX ON tour_stops(tour_id);
CREATE INDEX ON tour_stops(gig_id);
CREATE INDEX ON release_deadlines(release_id);
CREATE INDEX ON release_promo_tasks(release_id);
CREATE INDEX ON epk_exports(epk_profile_id);

-- Soft-deletion (all tables)
-- All list queries include WHERE deleted_at IS NULL
-- Partial indexes cover hot queries:
CREATE INDEX ON gigs(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX ON tracklists(deleted_at) WHERE deleted_at IS NULL;

-- Scheduler hot query (NFR-306)
CREATE INDEX ON scheduled_posts(scheduled_at)
  WHERE status = 'scheduled' AND deleted_at IS NULL;

-- Ordered track retrieval
CREATE INDEX ON tracks(tracklist_id, position);

-- Image history
CREATE INDEX ON generated_images(tracklist_id, created_at DESC);

-- Gig filtering
CREATE INDEX ON gigs(date, gig_status, deleted_at);
CREATE INDEX ON income_entries(date, category);
CREATE INDEX ON expense_entries(date, category);

-- Cursor pagination (all list queries)
-- Use (created_at, id) composite — covered by standard indexes above
```

### 5.4 Object Storage (MinIO) Bucket Structure

```
klubhub-dj/                     (single bucket)
├── source-files/               # Uploaded DJ history files
│   └── {tracklist_id}/{filename}
├── cover-art/                  # Cached cover artwork
│   └── {artist_slug}/{title_slug}.{ext}
├── generated-images/           # Tracklist visuals
│   └── {tracklist_id}/{image_id}.png
├── epk-assets/                 # Press photos, stage plots, PDFs
│   └── {epk_profile_id}/{filename}
├── tour-documents/             # Contracts, itineraries
│   └── {tour_id}/{filename}
└── user-assets/                # Logos, watermarks, custom placeholder
    └── {filename}
```

> **v3 guardrail:** Prefix each top-level path with `users/{user_id}/` when multi-tenancy is added, without restructuring the scheme (NFR-106).

### 5.5 Caching Strategy

| What | Where | TTL | Invalidation |
|---|---|---|---|
| Cover art images | MinIO (permanent cache) | Indefinite | User explicit reset (FR-007a) |
| Cover art path per track | PostgreSQL `tracks.cover_art_path` | Until manually changed | User override upload |
| Bundled Unicode fonts | API process memory | Process lifetime | API restart |
| 50% preview images | Client browser cache | 5 minutes | Config change triggers new request |
| Dashboard aggregation | No cache — fast enough per NFR-200 | N/A | N/A |
| Generated full-res images | MinIO (permanent) | Indefinite | User deletes tracklist/image |

Cover art is the highest-value cache: 4,000 unique covers × ~500 KB = ~2 GB saved in external API calls after 2 years (NFR-304).

### 5.6 Backup Strategy (NFR-107)

```bash
# scripts/backup.sh
# 1. pg_dump → compressed SQL dump
docker exec klubhub-dj-db pg_dump -U $POSTGRES_USER $POSTGRES_DB \
  | gzip > backup/db-$(date +%Y%m%d-%H%M%S).sql.gz

# 2. MinIO mirror → local directory
docker exec klubhub-dj-storage \
  mc mirror /data backup/storage-$(date +%Y%m%d-%H%M%S)/

# 3. Archive both
tar -czf klubhub-backup-$(date +%Y%m%d).tar.gz backup/
```

Restore is non-destructive by default; requires `--force` to overwrite existing data.

---

## 6. Cross-Cutting Concerns

### 6.1 Authentication & Authorization

**v1: No authentication.** The API accepts all requests without credentials. All data is owned by a single implicit user.

**Network security instead of auth** (NFR-500):
- All services bind to `127.0.0.1` by default
- Frontend at `:3000`, API at `:8080` (internal), DB at `:5432` (internal), MinIO at `:9000` (internal)
- Only the frontend port is exposed to the host

**v3 guardrail structure** (NFR-507, NFR-512):
- Rate-limiting middleware exists but is disabled (`RATE_LIMIT_ENABLED=false`)
- All tables include a nullable `user_id` UUID column
- All repository methods accept `context.Context` — middleware will inject authenticated user in v3
- All list queries are written with a `WHERE user_id = $1 OR $1 IS NULL` pattern stub

### 6.2 Error Handling & Resilience

#### Error Type Hierarchy

```go
// platform/errors.go

type DomainError struct {
    Code    string  // e.g. "TRACKLIST_NOT_FOUND"
    Message string  // human-readable, safe to return
    Status  int     // HTTP status code
}

type ValidationError struct {
    DomainError
    Details []FieldError
}

// Infrastructure errors are wrapped with context, logged, and
// returned to the user as a generic "INTERNAL_ERROR"
```

#### Resilience Patterns by Subsystem

**Cover art API calls:**
```
Per-provider: 5s connect timeout, 30s read timeout (NFR-706)
Fallback chain: Spotify → Discogs → MusicBrainz → placeholder
MusicBrainz rate limit: 1 req/s token bucket (NFR-402)
Discogs: 60 req/min token bucket
On all fail: placeholder art, non-blocking
```

**Social post publishing (NFR-102):**
```
Attempt 1: immediate
Attempt 2: +1 minute
Attempt 3: +5 minutes
Attempt 4: +15 minutes
→ permanently_failed status, dashboard surfaced
```

**Instagram rate limiting (A6.4):**
```
Min 30s between posts
429 response → back off per Retry-After header (or 15min default)
Scheduler surfaces "rate limited until {time}" in queue view
```

**MinIO/PostgreSQL write consistency (NFR-106):**
```
1. Write blob to MinIO
2. Write record to PostgreSQL
   └─ on failure: attempt MinIO delete (best-effort)
   └─ return error to user
3. Background reconciler on startup + manual trigger
```

**Database connection:**
```
pgx connection pool: min 2, max 10 connections
Connect timeout: 30s
Query timeout: configurable (default 30s)
On disconnect: requests fail fast with 503; no retry (container restarts handle recovery)
```

### 6.3 Observability

#### Structured Logging (NFR-600)

Using `zerolog` for zero-allocation JSON logging:

```go
// Every HTTP handler gets a request-scoped logger
log.Info().
    Str("module", "tracklist").
    Str("request_id", reqID).
    Str("operation", "parse_file").
    Dur("duration_ms", elapsed).
    Int("track_count", n).
    Msg("File parsed successfully")
```

Log levels: `debug` (dev) / `info` (default) / `warn` / `error` / `fatal`.
No sensitive data in any log level (NFR-508, NFR-604).

#### Request Tracing (NFR-603)

```
Middleware: generate UUID request_id per request
→ inject into context
→ include in all log entries
→ return as X-Request-ID response header
→ propagate to outbound API calls as X-Klubhub-Request-ID
```

#### Health Endpoint (NFR-601)

```
GET /api/v1/health
→ SELECT 1 (database)
→ bucket stat (MinIO)
→ per-integration status from config
→ aggregated status: healthy | degraded | unhealthy
→ < 500ms response time
```

#### Storage Usage (NFR-606)

```
GET /api/v1/system/storage
→ MinIO ListObjectsV2 per prefix
→ per-category count + bytes
```

#### Docker Health Checks (NFR-602)

```yaml
# API
healthcheck:
  test: ["CMD", "wget", "-q", "-O-", "http://localhost:8080/api/v1/health"]
  interval: 30s
  timeout: 5s
  retries: 3

# Database
healthcheck:
  test: ["CMD", "pg_isready", "-U", "${POSTGRES_USER}"]
  interval: 30s
  timeout: 5s
  retries: 3
```

### 6.4 Input Validation

All request bodies validated before handler logic:

**File uploads (NFR-502):**
- Size check (`MAX_UPLOAD_SIZE_MB`, default 50 MB)
- Extension allowlist per endpoint type
- MIME-type vs extension mismatch check
- Magic-byte validation
- Path-traversal sanitization on filename

**API request bodies (NFR-503):**
- JSON schema validation using a Go validator library
- String max-length enforcement
- Enum value validation
- ISO 8601 dates only
- Currency amounts: decimal(10,2), non-negative
- JSONB fields validated against expected structure

**Rich text (EPK bio):**
- Sanitize to allowed tag subset (bold, italic, lists, links, H2, H3) before storage
- Strip unsupported tags with a warning response

### 6.5 Security Controls Summary

| Control | Implementation | NFR |
|---|---|---|
| Network isolation | `127.0.0.1` bind default | NFR-500 |
| Token encryption | AES-256-GCM, key from env | NFR-501 |
| SQL injection prevention | pgx parameterized queries only | NFR-504 |
| XSS prevention | Vue auto-escaping + sanitize rich text | NFR-505 |
| CORS | Allowlist from `CORS_ORIGIN` env var | NFR-506 |
| File upload safety | Extension + MIME + magic-byte checks | NFR-502 |
| CSP | Nuxt CSP header with nonces | NFR-510 |
| Secrets | `.env` only, never in code or logs | NFR-508 |
| Rate limiting | Middleware present but disabled | NFR-507 |
| MinIO access | No host port; API is sole gateway | NFR-511 |
| Dependency audit | CI vuln scan on deps + images | NFR-509 |

---

## 7. Technology Stack

Every choice maps to a specific requirement.

### 7.1 Backend

| Component | Choice | Version | Justification |
|---|---|---|---|
| Language | **Go** | 1.23+ | Static binary (NFR-403 <100MB), concurrency for cover art fetching (NFR-203), performance (NFR-200), cross-compile for ARM (NFR-701) |
| HTTP router | **chi** | v5 | Lightweight, idiomatic Go, middleware composable; no heavy framework overhead |
| Database driver | **pgx** | v5 | Native PostgreSQL protocol, prepared statements (NFR-504), COPY support |
| DB migrations | **goose** | v3 | Embedded migrations in binary (NFR-703), up/down support (NFR-806), sequential numbering |
| Object storage | **minio-go** | v7 | Official MinIO SDK, S3-compatible (future-proof) |
| Image generation | **gg** + **imaging** | latest | Pure Go (NFR-403, no Chromium), compositing + text rendering |
| Unicode fonts | **Noto Sans** family | — | Full CJK/Cyrillic/Arabic coverage (NFR-207), Apache 2.0 license |
| PDF generation | **gofpdf** (or **fpdf2**) | latest | Pure Go, no headless browser (NFR-403) |
| Structured logging | **zerolog** | v1 | Zero-allocation JSON (NFR-600), minimal overhead |
| Configuration | **envconfig** | v2 | Struct-based env var mapping (NFR-702), documented defaults |
| Validation | **go-playground/validator** | v10 | Battle-tested, struct tag-based (NFR-503) |
| HTML sanitizer | **bluemonday** | v1 | Allow-list based, EPK rich text (NFR-505) |
| Crypto | `crypto/aes` + `crypto/cipher` | stdlib | AES-256-GCM for token encryption (NFR-501) |
| Testing | `testing` + **testcontainers-go** | latest | Real DB in tests (NFR-804), no mocks for DB layer |
| API docs | **swaggo/swag** | v2 | OpenAPI 3.x from annotations (NFR-803) |

### 7.2 Frontend

| Component | Choice | Version | Justification |
|---|---|---|---|
| Framework | **Nuxt 3** | 3.x | BRD §5, existing scaffold in repo, SSR for fast initial load (NFR-206) |
| Language | **TypeScript** | 5.x | Type safety, existing tsconfig in repo |
| State management | **Pinia** | v2 | Nuxt-native, lightweight, no boilerplate |
| HTTP client | **$fetch** (ofetch) | Nuxt built-in | Auto-proxies to API, handles SSR/CSR transparently |
| UI components | **Nuxt UI** or **shadcn-vue** | latest | Accessible components (NFR-207 WCAG 2.1 AA), no runtime CSS-in-JS |
| Rich text editor | **Tiptap** | v2 | Extensible, Vue 3 native, output sanitizable (NFR-505, A8.1) |
| Date/time | **dayjs** | latest | Lightweight, timezone support (A6.3) |
| Package manager | **pnpm** | 9+ | Already in use (pnpm-workspace.yaml) |
| Monorepo | **Nx** | latest | Already scaffolded, task orchestration for build/test |

### 7.3 Infrastructure

| Component | Choice | Version | Justification |
|---|---|---|---|
| Database | **PostgreSQL** | 16-alpine | Relational integrity, JSONB, WAL durability (NFR-103), pg_dump (NFR-107) |
| Object storage | **MinIO** | latest stable | S3-compatible, self-hosted, single-drive mode (NFR-104), no cost (NFR-402) |
| Containerization | **Docker + Compose v2** | Engine 24+ | NFR-700, single-command deploy |
| Base image (API) | `golang:1.23-alpine` → `scratch` | — | Multi-stage; final image <100 MB (NFR-403) |
| Base image (Frontend) | `node:22-alpine` | — | Minimal size (NFR-403) |
| Multi-arch build | `linux/amd64` + `linux/arm64` | — | macOS Apple Silicon support (NFR-701) |

### 7.4 Stack Evaluation Against BRD Preferred Stack

The BRD specified: `Nuxt 3 / Go / PostgreSQL 16 / MinIO / gg+imaging / Instagram Graph API / Spotify+Discogs+MusicBrainz / gofpdf`.

**Verdict: Full alignment.** This architecture adopts the BRD stack exactly, adding:
- `chi` router (not specified, lightweight choice over Gin/Echo for idiomicity)
- `pgx` over `database/sql` (better PostgreSQL native features)
- `goose` for migrations (not specified, meets NFR-703 requirements)
- `zerolog` for logging (not specified, meets NFR-600 zero-allocation requirement)
- `testcontainers-go` (not specified, enables real DB tests per NFR-804)

The open question from BRD §20.7 (gofpdf vs chromedp) is resolved in favor of **gofpdf**: it meets NFR-403 image size targets and avoids the headless Chrome dependency weight. If richer PDF layouts are needed post-v1, gofpdf can be swapped without API contract changes (PDFGenerator interface, NFR-802).

The open question from BRD §20.6 (job scheduler: Asynq/Redis vs PostgreSQL polling) is resolved in favor of **PostgreSQL polling**: single-user workload, no external broker dependency (NFR-402), negligible CPU overhead at 60s poll interval (NFR-404).

---

## 8. Repository Layout

```
klubhub/                        (git root — this directory)
├── dev/                        # Nx monorepo workspace
│   ├── apps/
│   │   ├── dj/                 # Nuxt 3 frontend (existing scaffold)
│   │   └── dj-e2e/             # Playwright E2E tests (existing scaffold)
│   ├── docs/
│   │   ├── KlubHub_DJ_BRD_v2.0_Platform.md
│   │   ├── KlubHub_DJ_BRD_v2.0_Addendum.md
│   │   ├── KlubHub_DJ_NFR_v1.0.md
│   │   └── ARCHITECTURE.md     # This document
│   └── ...                     # Nx config, pnpm workspace
│
├── api/                        # Go backend (to be created)
│   ├── cmd/api/main.go
│   ├── internal/               # (see §3.1)
│   ├── migrations/
│   ├── fonts/
│   ├── Dockerfile
│   └── go.mod
│
├── scripts/
│   ├── backup.sh               # NFR-107
│   └── restore.sh              # NFR-107
│
├── docker-compose.yml          # Production deployment
├── docker-compose.dev.yml      # Dev overrides (MinIO console, hot reload)
├── .env.example                # All env vars documented
├── UPGRADING.md                # Version-specific migration notes
└── .gitignore
```

---

## 9. Deployment Architecture

### 9.1 docker-compose.yml Structure

```yaml
services:
  db:
    image: postgres:16-alpine
    container_name: klubhub-dj-db
    environment:
      POSTGRES_DB: klubhub
      POSTGRES_USER: klubhub
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - klubhub-dj-db-data:/var/lib/postgresql/data
    networks: [internal]
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "klubhub"]
      interval: 30s
      timeout: 5s
      retries: 3
    # resource limits: commented-out defaults per NFR-400

  storage:
    image: minio/minio:latest
    container_name: klubhub-dj-storage
    command: server /data
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
    volumes:
      - klubhub-dj-storage-data:/data
    networks: [internal]
    # No host port binding — internal only (NFR-511)
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 30s
      timeout: 5s
      retries: 3

  api:
    image: klubhub-dj-api:latest
    build: ./api
    container_name: klubhub-dj-api
    environment:
      DATABASE_URL: postgres://klubhub:${POSTGRES_PASSWORD}@db:5432/klubhub
      MINIO_ENDPOINT: storage:9000
      MINIO_ACCESS_KEY: ${MINIO_ROOT_USER}
      MINIO_SECRET_KEY: ${MINIO_ROOT_PASSWORD}
      TOKEN_ENCRYPTION_KEY: ${TOKEN_ENCRYPTION_KEY}
      BIND_ADDRESS: ${BIND_ADDRESS:-127.0.0.1}
      CORS_ORIGIN: ${CORS_ORIGIN:-http://127.0.0.1:3000}
      TZ: ${TZ:-UTC}
      LOG_LEVEL: ${LOG_LEVEL:-info}
    ports:
      - "${BIND_ADDRESS:-127.0.0.1}:8080:8080"
    networks: [internal]
    depends_on:
      db:
        condition: service_healthy
      storage:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 5s
      retries: 3

  frontend:
    image: klubhub-dj-frontend:latest
    build: ./dev/apps/dj
    container_name: klubhub-dj-frontend
    environment:
      NUXT_PUBLIC_API_BASE: http://api:8080
    ports:
      - "${BIND_ADDRESS:-127.0.0.1}:3000:3000"
    networks: [internal]
    depends_on:
      api:
        condition: service_healthy

volumes:
  klubhub-dj-db-data:
  klubhub-dj-storage-data:

networks:
  internal:
    driver: bridge
```

### 9.2 Environment Variables Reference

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `POSTGRES_PASSWORD` | Yes | — | Database password |
| `MINIO_ROOT_USER` | Yes | — | Object storage admin user |
| `MINIO_ROOT_PASSWORD` | Yes | — | Object storage admin password |
| `TOKEN_ENCRYPTION_KEY` | Yes | — | AES-256-GCM key (`openssl rand -hex 32`) |
| `BIND_ADDRESS` | No | `127.0.0.1` | Network bind (⚠️ set `0.0.0.0` for LAN only if understood) |
| `TZ` | No | `UTC` | Default scheduling timezone |
| `CORS_ORIGIN` | No | `http://127.0.0.1:3000` | Allowed frontend origin |
| `LOG_LEVEL` | No | `info` | API log level |
| `MAX_UPLOAD_SIZE_MB` | No | `50` | Max DJ history file size |
| `COVER_ART_CONCURRENCY` | No | `10` | Concurrent cover art fetch workers |
| `SCHEDULER_INTERVAL` | No | `60s` | Social post scheduler poll interval |
| `RATE_LIMIT_ENABLED` | No | `false` | Enable API rate limiting |
| `SPOTIFY_CLIENT_ID` | No | — | Spotify Web API credentials |
| `SPOTIFY_CLIENT_SECRET` | No | — | |
| `DISCOGS_TOKEN` | No | — | Discogs API token |
| `INSTAGRAM_APP_ID` | No | — | Facebook/Instagram App credentials |
| `INSTAGRAM_APP_SECRET` | No | — | |

---

## 10. v3 Multi-User Guardrails

These are architectural constraints applied in v1 specifically to avoid expensive refactoring when multi-user support is added in v3. They add minimal overhead today but eliminate painful migrations later.

| Guardrail | v1 Implementation | v3 Activation |
|---|---|---|
| `user_id` column on all tables (NFR-512) | Nullable UUID, not queried | NOT NULL, indexed, filtered on all queries |
| MinIO path prefix (NFR-106) | `users/{singleton_id}/...` from day 1 | No path restructuring needed |
| Repository `context.Context` parameter (NFR-800) | Context carries no user | Middleware injects authenticated user; repositories auto-filter |
| Pagination `WHERE user_id = $1 OR $1 IS NULL` stub (NFR-305) | Always passes NULL | Passes authenticated user ID |
| Rate limiting middleware (NFR-507) | Present, `RATE_LIMIT_ENABLED=false` | Switch to per-user key extraction |
| CORS multi-origin support (NFR-506) | Single origin env var | Comma-separated list in env var |
| API versioning `/api/v1` (NFR-801) | All endpoints prefixed | Breaking multi-user changes go to `/api/v2` |
| Structured logging `user_id` field (NFR-603) | Field absent | Middleware injects into log context |

---

*This document is the authoritative architecture blueprint for KlubHub DJ v1. All implementation decisions should trace back to a requirement in the BRD, Addendum, or NFR documents. Where this document conflicts with those, raise a discussion — conflicts indicate either an architectural gap or a requirement that needs clarification.*

*Last updated: 2026-03-11 — initial architecture definition*
