# KLUBHUB DJ

> **Historical requirements record:** MinIO variables and service references
> below are superseded by Garage and `S3_*` configuration for v1.0.0. Current
> deployment guidance lives in `CONFIGURATION.md` and `SELF-HOSTING.md`.

*Non-Functional Requirements Document*

---

**Version 1.0 — March 2026**

Canonical NFR Reference for KlubHub DJ v1.x with v3 Multi-User Guardrails

---

## Document Control

| Field | Value |
|---|---|
| **Status** | Approved |
| **Applies to** | KlubHub DJ v1.0 (Phases 1a through 1g) |
| **Supersedes** | BRD v2.0 Section 17 (inline NFRs), Addendum v2.0-A Section A16.2 (NFR-001 through NFR-006) |
| **Companion Docs** | `KlubHub_DJ_BRD_v2.0_Platform.md`, `KlubHub_DJ_BRD_v2.0_Addendum.md` |
| **Hardware Baseline** | Modern desktop/workstation: 4+ CPU cores, 8–16 GB RAM, SSD storage |
| **Deployment Model** | Docker Compose, 4 services (frontend, API, database, object storage) |
| **User Model** | Single-user, no authentication (v1); multi-user planned for v3.0+ |

---

## Relationship to Existing NFRs

The following requirements from the BRD and Addendum are **incorporated by reference** and superseded by this document. Where this document restates a requirement, the restated version with its new NFR-ID is authoritative.

| Original Source | Original ID/Section | Disposition in This Document |
|---|---|---|
| BRD Section 17 "Performance" | (inline, no ID) | Superseded by NFR-200 through NFR-206 |
| BRD Section 17 "Scalability" | (inline, no ID) | Superseded by NFR-300 through NFR-305 |
| BRD Section 17 "Reliability" | (inline, no ID) | Superseded by NFR-100 through NFR-107 |
| BRD Section 17 "Security" | (inline, no ID) | Superseded by NFR-500 through NFR-512 |
| BRD Section 17 "Portability" | (inline, no ID) | Superseded by NFR-700 through NFR-707 |
| BRD Section 17 "Accessibility" | (inline, no ID) | Superseded by NFR-207 |
| BRD Section 17 "Maintainability" | (inline, no ID) | Superseded by NFR-800 through NFR-811 |
| BRD Section 17 "Extensibility" | (inline, no ID) | Superseded by NFR-802, NFR-809 |
| Addendum NFR-001 | Bind 127.0.0.1 | Superseded by NFR-500 |
| Addendum NFR-002 | Auto migrations | Superseded by NFR-703 |
| Addendum NFR-003 | Health endpoint | Superseded by NFR-601 |
| Addendum NFR-004 | Unicode rendering | Superseded by NFR-207 |
| Addendum NFR-005 | MinIO write order | Superseded by NFR-106 |
| Addendum NFR-006 | Backup/restore scripts | Superseded by NFR-107 |

---

## 1. Availability and Reliability

These requirements define the uptime expectations, failure handling, and data durability guarantees for a self-hosted, single-user system running on the user's own hardware.

### NFR-100: Service Availability Target

**Requirement:** The system shall be available whenever the host machine is running and Docker services are started. There is no uptime SLA. The system shall start all four Docker services within 60 seconds of `docker compose up` on the baseline hardware target.

**Rationale:** As a self-hosted, single-user application, availability is bounded by the host machine's own uptime. The 60-second cold-start budget covers database initialization, object storage readiness, API migration execution, and frontend SSR hydration.

**Measurement:** Time from `docker compose up` command to HTTP 200 on the API health endpoint and HTTP 200 on the frontend root route.

### NFR-101: Graceful Degradation on External API Failure

**Requirement:** The system shall remain fully functional for all local operations when any or all external APIs (Spotify, Discogs, MusicBrainz, Instagram Graph API) are unreachable. Specifically:

| External Dependency | Degraded Behavior |
|---|---|
| All cover art APIs unreachable | Placeholder art used; "Retry Cover Art" button available; image generation proceeds |
| Spotify API only | Discogs/MusicBrainz fallback chain continues; no user-visible error unless all fail |
| Instagram Graph API | Social module endpoints return HTTP 503 with descriptive message; scheduling UI displays warning; all other modules unaffected |
| MusicBrainz rate-limited | Exponential backoff; moves to placeholder for affected tracks; retryable |

**Rationale:** DJs may run the platform on laptops during travel or in venues with poor connectivity. Core workflows (parsing, image generation, gig management) must never depend on internet access.

### NFR-102: Social Post Retry Policy

**Requirement:** Failed social media publish attempts shall be retried up to 3 times with exponential backoff. The backoff schedule shall be: 1st retry after 1 minute, 2nd retry after 5 minutes, 3rd retry after 15 minutes. After exhausting retries, the post transitions to `permanently_failed` status per FR-027a.

**Rationale:** Instagram API failures are commonly transient (rate limits, brief outages). Three retries with escalating delays balances recovery probability against user notification latency.

### NFR-103: Database Durability

**Requirement:** The database shall use default durability settings: `fsync=on` and `synchronous_commit=on`. The database data directory shall be mounted on a Docker named volume to survive container restarts and image updates. WAL (Write-Ahead Logging) shall remain enabled.

**Rationale:** Financial data, gig records, and tracklist metadata represent irreplaceable user data. Default durability settings provide crash-safe writes with no configuration burden on the user.

### NFR-104: Object Storage Durability

**Requirement:** The object storage service shall store data on a Docker named volume. The deployment shall use single-node, single-drive mode (erasure coding not required for single-user). Default data integrity checks (bitrot protection) shall remain enabled.

**Rationale:** Single-drive mode matches the self-hosted desktop target. Named volumes ensure data persists across container lifecycle events.

### NFR-105: Crash Recovery

**Requirement:** After an unclean shutdown (power failure, Docker kill, OOM kill), the system shall recover to a consistent state on next startup. Specifically:

- The database recovers via WAL replay (built-in).
- Object storage recovers via its healing process (built-in).
- In-progress image generation requests are lost and must be re-initiated by the user.
- In-progress social post publish operations are re-evaluated per NFR-102 retry logic on next scheduler tick.
- No manual intervention shall be required for recovery.

**Rationale:** Desktop users cannot be expected to run manual recovery tools. Recovery must be automatic and silent.

### NFR-106: Object Storage / Database Write Consistency

**Requirement:** For operations that write to both object storage and the database (image generation, file upload, EPK export), the write order shall be: object storage first, then database. On database write failure after successful object storage write, the system shall attempt best-effort deletion of the orphaned object and return an error to the user. An orphan reconciliation endpoint (`GET /api/v1/system/reconcile`) shall identify and optionally clean objects with no corresponding database record, applying a configurable age threshold (default: 24 hours). Reconciliation shall also run on API startup.

**Rationale:** Object storage writes are idempotent (overwrite by key), while database records provide the authoritative index. Writing to object storage first ensures the blob exists before the database reference is created, making orphans (blob without record) the only possible inconsistency, which is recoverable.

**v3 Guardrail:** When multi-user support is added, reconciliation must become per-user-scoped to prevent cross-user orphan deletion. The object storage bucket structure should include a user-scoped prefix (e.g., `users/{user_id}/generated-images/`) from v1, even though only one user exists initially.

### NFR-107: Backup and Restore

**Requirement:** The project shall include `scripts/backup.sh` and `scripts/restore.sh` that produce and consume a single timestamped archive containing a database dump and an object storage bucket mirror. Backup shall complete in under 5 minutes for the reference dataset (see NFR-304). Restore shall be non-destructive by default (refuses to overwrite existing data unless `--force` is passed). Both scripts shall validate prerequisites (Docker running, containers accessible) before executing.

**Rationale:** Self-hosted users need a simple, documented path to protect their data. Automated scheduled backups are deferred to keep v1 operationally simple.

---

## 2. Performance and Responsiveness

Performance budgets are categorized by operation type. All measurements assume the baseline hardware target (4+ cores, 8–16 GB RAM, SSD) with the reference dataset (see NFR-304).

### NFR-200: API CRUD Response Time

**Requirement:** All REST API endpoints performing standard CRUD operations shall respond within the following time budgets, measured from request receipt to response send at the HTTP handler boundary:

| Operation Type | p95 Target | p99 Target |
|---|---|---|
| Single entity GET | 50ms | 100ms |
| Paginated list GET (up to 100 items) | 100ms | 200ms |
| Single entity POST/PUT/DELETE | 100ms | 200ms |
| Dashboard aggregation GET | 300ms | 500ms |

**Rationale:** The BRD stated "API responses < 500ms" as a blanket target. This breakdown provides actionable budgets per operation class. The dashboard is granted a larger budget because it aggregates data across all modules.

### NFR-201: File Parse Response Time

**Requirement:** Parsing a DJ history file shall complete within the following time budgets:

| File Size / Track Count | Target |
|---|---|
| Small file (under 30 tracks) | Under 1 second |
| Medium file (30–100 tracks) | Under 3 seconds |
| Large file (100–500 tracks) | Under 10 seconds |
| Very large file (500+ tracks) | Under 30 seconds, with progress indication |

Parse time includes file format detection, metadata extraction, database insert, and source file storage. Cover art fetching is asynchronous and excluded from this budget.

**Rationale:** File parsing is the entry point to the application. Slow parsing damages the first-impression experience for the flagship feature.

### NFR-202: Image Generation Time

**Requirement:** Final image generation (full resolution export) shall complete within:

| Track Count | Background Mode | Target |
|---|---|---|
| Up to 30 tracks | Solid color | Under 3 seconds |
| Up to 30 tracks | Custom upload | Under 5 seconds |
| Up to 30 tracks | Cover art mosaic | Under 10 seconds |
| 31–100 tracks | Any mode | Under 15 seconds |
| 100+ tracks | Any mode | Under 30 seconds, with progress indication |

**Rationale:** The BRD target of "under 10s for 30 tracks" is refined here by background mode. Mosaic mode requires compositing multiple cover art images, justifying the higher budget. Progress indication for long operations prevents the user from perceiving the system as frozen.

### NFR-203: Cover Art Fetch Time

**Requirement:** Per-track cover art lookup across the fallback chain shall complete within 5 seconds per track. For a 30-track tracklist, total cover art fetching shall complete within 30 seconds using concurrent requests (minimum 5 concurrent fetches). Cache hits (art already in object storage) shall resolve in under 100ms.

**Rationale:** Cover art fetching is I/O-bound (external API calls). Concurrency amortizes the per-track latency. Cache-first strategy means repeat operations are near-instant.

### NFR-204: PDF Generation Time

**Requirement:** EPK PDF export shall complete within:

| EPK Complexity | Target |
|---|---|
| Minimal (bio + 3 photos + contact) | Under 5 seconds |
| Full (all sections, 20 photos, rich text) | Under 15 seconds |

Invoice PDF generation shall complete in under 3 seconds.

**Rationale:** PDF generation is CPU-intensive (layout engine, image embedding). The 15-second upper bound accommodates the maximum 20 press photos defined in Addendum A8.2.

### NFR-205: Live Preview Rendering

**Requirement:** The live preview in the tracklist image generator shall render at 50% resolution (per Addendum A5.1) and update within 500ms of any configuration change (color, template, field toggle, logo position). Initial preview load shall complete within 2 seconds.

**Rationale:** Live preview is the core interactive feedback loop. Responsiveness below 500ms feels immediate; above 1 second breaks flow.

### NFR-206: Dashboard Load Time

**Requirement:** The unified dashboard shall render with complete widget data within 2 seconds of navigation, including: page transition, API call for aggregated data, and client-side rendering of all active module widgets. Dashboard data that takes longer than 1 second to aggregate shall be loaded progressively (skeleton placeholders followed by data fill).

**Rationale:** The dashboard is the home screen and most frequently visited page. The 2-second budget from the BRD is maintained but augmented with progressive loading to ensure perceived performance is acceptable even as more modules activate.

### NFR-207: Accessibility — Font and Unicode Rendering

**Requirement:** Image generation shall use bundled fonts with full Unicode coverage (CJK, Cyrillic, Arabic, accented Latin). Font loading shall be performed once at API startup and cached in memory. Font fallback resolution (glyph lookup across font files) shall add no more than 100ms to image generation for a 30-track tracklist containing mixed scripts. All web UI text shall meet WCAG 2.1 AA color contrast ratios (4.5:1 for normal text, 3:1 for large text).

**Rationale:** Consolidates Addendum NFR-004 (Unicode support) and BRD Section 17 accessibility requirement. Font caching prevents repeated disk I/O.

---

## 3. Scalability and Capacity

KlubHub DJ v1 is a single-user, vertically-scaled application. These requirements define the data volumes and concurrency levels the system must handle without degradation, and the architectural guardrails that enable future horizontal scaling.

### NFR-300: Single-User Concurrency Model

**Requirement:** The system shall support one active user with up to 5 concurrent browser tabs/sessions interacting with the API simultaneously. All 5 concurrent requests shall be served within the performance budgets defined in Section 2. The system is not required to support multiple distinct users in v1.

**Rationale:** A single DJ may have the dashboard open alongside the tracklist generator, social scheduler, and gig tracker in separate tabs. Five concurrent sessions is a realistic upper bound for one person.

**Tradeoff:** No connection pooling or request queuing optimization is needed beyond defaults, keeping the system simple.

### NFR-301: No Horizontal Scaling Requirement

**Requirement:** The system shall run as a single instance of each service (one API process, one database instance, one object storage instance, one frontend instance). No load balancing, service replication, or distributed coordination is required.

**Rationale:** Single-user, self-hosted deployment. Horizontal scaling adds operational complexity with no benefit for the target use case.

### NFR-302: Concurrent Image Generation

**Requirement:** The system shall support up to 2 concurrent image generation requests without degradation. The "Export Both" feature (Story + Square from Addendum A5.2) shall execute both generations concurrently, each within its individual performance budget.

**Rationale:** The "Export Both" feature creates two simultaneous generation jobs. Supporting 2 concurrent jobs covers this plus a small margin. Memory-bound operations (image compositing) need to be bounded to prevent OOM on 8 GB systems.

### NFR-303: Concurrent Cover Art Fetching

**Requirement:** The system shall support up to 10 concurrent outbound HTTP requests for cover art API lookups. This limit shall be configurable via a `COVER_ART_CONCURRENCY` environment variable (default: 10). When the limit is reached, additional lookups shall queue and execute as slots free.

**Rationale:** Balances fetch speed against external API rate limits and local resource usage. Configurable for users with slower connections or stricter API quotas.

### NFR-304: Reference Dataset and Data Growth Projections

**Requirement:** The system shall perform within all stated budgets against the following reference dataset representing 2 years of active use by a working DJ:

| Entity | Reference Count | Growth Rate Estimate |
|---|---|---|
| Tracklists | 200 | ~2 per week |
| Tracks (across all tracklists) | 6,000 | ~60 per week |
| Generated images | 500 | ~5 per week |
| Cover art (unique, cached) | 4,000 | ~40 per week (decelerating as cache fills) |
| Social posts | 300 | ~3 per week |
| Gigs | 150 | ~1.5 per week |
| Income/expense entries | 400 | ~4 per week |
| Releases | 20 | ~1 per month |
| Tours | 5 | ~2–3 per year |
| EPK exports | 15 | ~1 per month |

**Estimated storage after 2 years:**

| Storage Category | Estimated Size | Notes |
|---|---|---|
| Database | 50–100 MB | Relational data, JSONB fields |
| Source files | 100–200 MB | DJ history files (~0.5–1 MB each) |
| Cover art cache | 2–4 GB | ~500 KB avg per unique cover at 500x500 |
| Generated images | 1–3 GB | ~2–6 MB per image (PNG at 1080x1920) |
| EPK assets | 500 MB–1 GB | Press photos, stage plots, PDFs |
| Tour documents | 200–500 MB | Contracts, itineraries, boarding passes |
| **Total estimated** | **4–9 GB** | Well within desktop SSD capacity |

**Rationale:** Concrete projections allow implementers to size database indexes, plan bucket structures, and validate performance against realistic data volumes.

### NFR-305: Pagination Requirements

**Requirement:** All list API endpoints shall support pagination with a default page size of 25 and a maximum page size of 100. Pagination shall use cursor-based pagination (keyset pagination on `created_at` + `id`) for consistent ordering. Offset-based pagination may be provided as an alternative for UI convenience but is not required.

**Rationale:** Cursor-based pagination maintains stable performance regardless of dataset size and avoids the drift issues of offset pagination on mutable datasets.

**v3 Guardrail:** Pagination queries shall include a `WHERE` clause structure that can accept a `user_id` filter. In v1, this filter is omitted (single user), but query patterns shall not use unscoped `SELECT *` on any table. All repository methods shall accept a context parameter that can carry user scope in v3.

### NFR-306: Database Index Strategy

**Requirement:** The database migration set shall include indexes for:

- All foreign key columns.
- All columns used in `WHERE` clauses of list/filter operations (date ranges, status enums, `deleted_at IS NULL`).
- A composite index on `tracks(tracklist_id, position)` for ordered track retrieval.
- A composite index on `generated_images(tracklist_id, created_at)` for history listing.
- A partial index on `scheduled_posts(scheduled_at) WHERE status = 'scheduled'` for the social scheduler poll query.

Index additions shall be documented in migration files with comments explaining the query pattern they support.

**Rationale:** Proactive indexing prevents performance degradation as data grows toward the reference dataset.

---

## 4. Resource Constraints and Cost

### NFR-400: Docker Resource Limits — Recommended Defaults

**Requirement:** The `docker-compose.yml` shall include commented-out resource limit configurations with the following recommended values for the baseline hardware target (4 cores, 8–16 GB RAM):

| Service | CPU Limit | Memory Limit | Memory Reservation | Notes |
|---|---|---|---|---|
| API | 2.0 | 1 GB | 256 MB | Image generation is CPU-intensive |
| Frontend | 1.0 | 512 MB | 128 MB | SSR, mostly idle after hydration |
| Database | 1.0 | 1 GB | 256 MB | Default shared_buffers is fine |
| Object Storage | 0.5 | 512 MB | 128 MB | Single-drive mode is lightweight |
| **Total** | **4.5** | **3 GB** | **768 MB** | Leaves 5–13 GB for host OS and other apps |

These limits shall be commented out by default (so the system works on any hardware) with a documentation comment: "Uncomment to apply resource limits. Recommended for systems where KlubHub DJ runs alongside other applications."

**Rationale:** Commented defaults educate users about resource expectations without breaking deployments on constrained hardware. The total footprint of 3 GB leaves ample headroom on 8 GB systems.

### NFR-401: Minimum Disk Space

**Requirement:** The system shall check available disk space on startup and log a warning if less than 2 GB is available on the Docker volume mount. The system shall not refuse to start but shall log: "WARNING: Less than 2 GB of disk space available. Image generation and file uploads may fail."

**Rationale:** Proactive warning prevents cryptic failures when storage services cannot write.

### NFR-402: No External Paid Dependencies

**Requirement:** The system shall function with zero paid service dependencies. All external API integrations use free-tier API access. The system shall document the free-tier rate limits for each API and design usage patterns to stay within them.

| API | Free Tier Limit | System Design Implication |
|---|---|---|
| Spotify Web API | Variable; typically sufficient for personal use | Cache aggressively; deduplicate lookups |
| Discogs API | 60 requests/minute (authenticated) | Backoff/retry; second in fallback chain |
| MusicBrainz API | 1 request/second (no key needed) | Rate limiter built into client |
| Instagram Graph API | Rate limits per-account; sufficient for single-user scheduling | 30-second minimum interval between publishes |

**Rationale:** Open-source, self-hosted ethos means users should not need subscriptions to use the platform. Free-tier API usage is sustainable for a single DJ's volume.

### NFR-403: Docker Image Size

**Requirement:** Each Docker image shall be optimized for size:

| Service | Target Image Size | Strategy |
|---|---|---|
| API | Under 100 MB | Multi-stage build; static binary; minimal base |
| Frontend | Under 200 MB | Multi-stage build; alpine base; prune dev dependencies |
| Database | Official image | No custom build (~400 MB, not controllable) |
| Object Storage | Official image | No custom build (~150 MB, not controllable) |

Total pull size for first-time deployment shall be under 1 GB (compressed).

**Rationale:** Smaller images mean faster initial deployment and faster updates. The API benefits most from optimization because bundled Unicode fonts add ~30–50 MB.

### NFR-404: CPU Usage During Idle

**Requirement:** When no user activity is occurring and no scheduled posts are pending, the combined CPU usage of all four services shall be under 2% of a single core. The social post scheduler's polling interval shall be configurable (default: 60 seconds) and shall not consume measurable CPU between polls.

**Rationale:** Many users will leave the platform running in the background. Idle resource consumption should be negligible.

---

## 5. Security and Data Protection

KlubHub DJ v1 has no authentication layer. Security focuses on network binding, data encryption at rest, input validation, and architectural guardrails for future multi-user support.

### NFR-500: Default Network Binding

**Requirement:** All Docker services shall bind to `127.0.0.1` by default. The `BIND_ADDRESS` environment variable (default: `127.0.0.1`) shall control the bind address for all exposed ports. The `.env.example` shall include:

```
BIND_ADDRESS=127.0.0.1  # WARNING: Setting to 0.0.0.0 exposes all services to your network. Only change if you understand the security implications.
```

**Rationale:** Without authentication, binding to `0.0.0.0` would expose all user data to the local network. Localhost-only is the only safe default.

### NFR-501: Token Encryption at Rest

**Requirement:** All OAuth tokens and API credentials stored in the database shall be encrypted using AES-256-GCM. The encryption key shall be derived from the `TOKEN_ENCRYPTION_KEY` environment variable. If this variable is not set, the API shall refuse to start and log: "FATAL: TOKEN_ENCRYPTION_KEY is required. Generate one with: openssl rand -hex 32". Changing the key invalidates all stored tokens, requiring re-authorization.

**Rationale:** Even on a single-user system, database dumps (backups) could be shared or exposed. Encryption ensures tokens are not usable without the key.

### NFR-502: Input Validation — File Uploads

**Requirement:** All file upload endpoints shall validate:

| Check | Action on Failure |
|---|---|
| File size exceeds `MAX_UPLOAD_SIZE_MB` (default 50 MB) | HTTP 413; message with configured limit |
| File extension not in allowlist per endpoint | HTTP 415; message listing accepted formats |
| MIME type does not match declared extension | HTTP 415; message identifying mismatch |
| File content fails magic-byte validation | HTTP 422; message indicating corrupt or misidentified file |
| Filename contains path traversal characters (`../`, `..\\`) | HTTP 400; sanitized before any processing |

Upload allowlists by endpoint:

| Endpoint Category | Allowed Extensions |
|---|---|
| DJ history file upload | .txt, .csv, .xml, .nml |
| Image upload (cover art, background, logo, press photo) | .jpg, .jpeg, .png |
| Document upload (tour, stage plot) | .jpg, .jpeg, .png, .pdf |

**Rationale:** File upload is the primary attack surface for a web application, even a single-user one. Defense in depth protects against malformed inputs from any source.

### NFR-503: Input Validation — API Request Bodies

**Requirement:** All API endpoints accepting JSON request bodies shall validate against a defined schema. Invalid requests shall return HTTP 400 with a structured error response listing all validation failures. Specifically:

- String fields: maximum length enforced (e.g., `dj_name` max 200 chars, `caption` max 2200 chars matching Instagram limit).
- Enum fields: reject values not in the defined set.
- Date fields: reject malformed dates; accept ISO 8601 format only.
- Numeric fields: reject negative values where semantically invalid (fees, amounts); enforce decimal precision (2 places for currency).
- JSONB fields: validate against expected structure before storage.

**Rationale:** Strict validation prevents garbage data accumulation and protects the database from injection via JSONB fields.

### NFR-504: SQL Injection Prevention

**Requirement:** All database queries shall use parameterized queries or a query builder that generates parameterized queries. String concatenation into SQL statements is prohibited. This shall be enforced by code review convention and, where tooling permits, by static analysis.

**Rationale:** Standard security hygiene. Even without network-exposed endpoints, defense in depth applies.

### NFR-505: XSS Prevention

**Requirement:** The frontend shall sanitize all user-generated content before rendering in the DOM. Server-rendered HTML shall escape user data. Rich text editor output (EPK bio) shall be sanitized to the allowed tag subset defined in Addendum A8.1 before storage.

**Rationale:** If the BIND_ADDRESS is changed to expose the application on a network, XSS protections prevent cross-site attacks.

### NFR-506: CORS Policy

**Requirement:** The API shall set CORS headers allowing requests only from the configured frontend origin (default: `http://127.0.0.1:3000`). A `CORS_ORIGIN` environment variable shall allow override. Wildcard (`*`) origins are prohibited in the default configuration.

**Rationale:** Prevents cross-origin requests from other tabs or applications when the system is network-exposed.

**v3 Guardrail:** Multi-user deployments may require multiple allowed origins. The CORS implementation shall support a comma-separated list of origins in the environment variable.

### NFR-507: Rate Limiting (Structural Guardrail)

**Requirement:** The API shall implement a rate-limiting middleware that is **disabled by default** in v1 (single-user, localhost). The middleware shall be configurable via environment variables (`RATE_LIMIT_ENABLED=false`, `RATE_LIMIT_RPM=300`). When enabled, it enforces a per-IP request rate limit using a token bucket algorithm.

**Rationale:** Not needed for v1 single-user, but the middleware must exist as scaffolding for v3 multi-user.

**v3 Guardrail:** When authentication is added, rate limiting switches to per-user rather than per-IP. The middleware interface shall accept a key-extraction function that can be swapped from IP-based to user-ID-based.

### NFR-508: Secrets Management

**Requirement:** All secrets (API keys, database passwords, object storage credentials, TOKEN_ENCRYPTION_KEY) shall be stored exclusively in the `.env` file or environment variables. Secrets shall never appear in:

- Source code or configuration files committed to version control.
- API response bodies (including `/health` and error messages).
- Log output (structured or otherwise).
- Frontend JavaScript bundles or SSR-rendered HTML.

The `.env.example` file shall contain placeholder values with comments, not real credentials.

**Rationale:** Standard secrets hygiene. The `.env` file is the single source of truth for secrets in the Docker Compose deployment model.

### NFR-509: Dependency Vulnerability Scanning

**Requirement:** The project shall include CI configuration (or documented manual commands) for running vulnerability scans on:

- Backend dependencies (language-native vulnerability checker).
- Frontend dependencies (package manager audit).
- Docker base images (container scanning tool).

No known critical or high-severity vulnerabilities shall exist in direct dependencies at release time.

**Rationale:** Open-source projects attract security scrutiny. Proactive scanning builds trust with the community.

### NFR-510: Content Security Policy

**Requirement:** The frontend shall serve a Content Security Policy header that restricts script sources to self and explicitly trusted CDN origins (if any). Inline scripts shall use nonces. `eval()` and `unsafe-inline` shall not be permitted in production builds.

**Rationale:** CSP is a defense-in-depth measure against XSS, particularly important if the system is network-exposed.

### NFR-511: Object Storage Access Control

**Requirement:** The object storage service shall not expose its web console in the default `docker-compose.yml`. The S3 API shall be accessible only within the Docker network (no host port binding). The API service is the sole gateway to stored objects. A `docker-compose.dev.yml` override shall enable the console for development.

**Rationale:** Per Addendum A1.2. Prevents direct access to stored files bypassing application-level controls.

### NFR-512: Audit-Ready Data Model

**Requirement:** All database tables shall include `created_at`, `updated_at`, and `deleted_at` (soft deletion) timestamp columns. The `updated_at` column is used for optimistic concurrency control (per FR-046a). These columns provide a minimal audit trail for data changes.

**v3 Guardrail:** All tables shall include a `user_id` column (UUID, nullable in v1, set to a default singleton value). In v1, this column exists but is not enforced or filtered. In v3, it becomes a required, indexed column with row-level filtering on all queries. Adding the column in v1 avoids a data migration on upgrade.

---

## 6. Observability and Diagnostics

KlubHub DJ uses structured logging to stdout as its sole observability mechanism. No external monitoring stack is included.

### NFR-600: Structured Logging Format

**Requirement:** The API shall emit structured JSON log lines to stdout. Each log entry shall include:

| Field | Type | Description |
|---|---|---|
| `timestamp` | ISO 8601 | When the event occurred |
| `level` | string | `debug`, `info`, `warn`, `error`, `fatal` |
| `message` | string | Human-readable description |
| `module` | string | Which module generated the log (e.g., `tracklist`, `social`, `gig`) |
| `request_id` | string (UUID) | Correlation ID for the HTTP request (if applicable) |
| `duration_ms` | number | Elapsed time for the operation (if applicable) |
| `error` | string | Error message/stack (if applicable) |

The default log level shall be `info`, configurable via `LOG_LEVEL` environment variable.

**Rationale:** Structured JSON enables `docker compose logs | jq` for filtering and analysis. The `module` field supports per-module debugging. No external log aggregation is needed for single-user.

**Tradeoff:** JSON logging is less human-readable in a terminal than plain text. Users can pipe through `jq` or a log formatter. The structured format is future-proof for log aggregation in v3.

### NFR-601: Health Check Endpoint

**Requirement:** `GET /api/v1/health` shall return a JSON response indicating the status of all system components and external integrations:

```json
{
  "status": "healthy | degraded | unhealthy",
  "uptime_seconds": 3600,
  "version": "1.0.0",
  "database": "connected | disconnected",
  "storage": "connected | disconnected",
  "integrations": {
    "spotify": "configured | not_configured | error",
    "discogs": "configured | not_configured | error",
    "musicbrainz": "available | rate_limited | error",
    "instagram": "configured | not_configured | disconnected | error"
  }
}
```

- `healthy`: database and storage connected, at least one cover art source available.
- `degraded`: database and storage connected, but one or more integrations in error state.
- `unhealthy`: database or storage disconnected.

Response time: under 500ms. The health endpoint shall not perform expensive checks. Database check shall use a simple `SELECT 1`. Object storage check shall use a bucket existence check.

**Rationale:** Provides machine-readable health status for Docker health checks and user-facing diagnostics.

### NFR-602: Docker Health Checks

**Requirement:** Each service in `docker-compose.yml` shall include a `healthcheck` configuration:

| Service | Health Check | Interval | Timeout | Retries |
|---|---|---|---|---|
| API | HTTP GET to health endpoint | 30s | 5s | 3 |
| Frontend | HTTP GET to root route | 30s | 5s | 3 |
| Database | `pg_isready` | 30s | 5s | 3 |
| Object Storage | HTTP GET to health/live endpoint | 30s | 5s | 3 |

**Rationale:** Docker health checks enable `docker compose ps` to show service health and allow `depends_on` conditions with `condition: service_healthy` for startup ordering.

### NFR-603: Request Tracing

**Requirement:** The API shall assign a UUID `request_id` to every incoming HTTP request. This ID shall be:

- Included in all log entries generated during that request.
- Returned in the `X-Request-ID` response header.
- Propagated to outbound HTTP calls (cover art APIs, Instagram API) in a custom header for debugging.

**Rationale:** Request-scoped correlation enables tracing a single user action through all log entries without distributed tracing infrastructure.

**v3 Guardrail:** When multi-user support is added, a `user_id` field shall be added to the log context. The request ID generation and propagation mechanism shall be compatible with OpenTelemetry trace context headers for future integration with distributed tracing.

### NFR-604: Error Logging Standards

**Requirement:** All errors shall be logged at `error` level with:

- The error message and, if available, the underlying error chain.
- The module, operation, and entity ID (if applicable).
- No sensitive data (API keys, tokens, passwords) in any log level.
- Stack traces for unexpected panics only (not for expected validation errors or external API failures).

User-facing error messages shall be generic and safe. Detailed diagnostic information shall appear only in logs.

**Rationale:** Separating user-facing messages from diagnostic logs prevents information leakage while maintaining debuggability.

### NFR-605: Startup Diagnostics

**Requirement:** On API startup, the system shall log at `info` level:

- Application version and build timestamp.
- Configured bind address and port.
- Database connection status (connected, migration count applied).
- Object storage connection status (connected, bucket verification).
- Integration configuration status (which API keys are present, not their values).
- Any warnings (low disk space per NFR-401, missing optional API keys per Addendum A14.4).

**Rationale:** Startup logs are the first debugging tool when something goes wrong. They should provide a complete picture of the system's configuration state.

### NFR-606: Storage Usage Reporting

**Requirement:** `GET /api/v1/system/storage` shall return object storage usage broken down by category:

| Category | Path Prefix |
|---|---|
| Source files | `source-files/` |
| Cover art cache | `cover-art/` |
| Generated images | `generated-images/` |
| EPK assets | `epk-assets/` |
| Tour documents | `tour-documents/` |
| Logos/watermarks | `user-assets/` |

Response includes per-category object count and total bytes. This endpoint powers the Settings storage view defined in Addendum A14.1.

**Rationale:** Users need visibility into what is consuming disk space to make informed cleanup decisions.

---

## 7. Portability and Deployment

### NFR-700: Docker Compose as Sole Deployment Method

**Requirement:** The system shall deploy via a single `docker-compose.yml` file. All four services, volumes, and networking shall be defined in this one file. The deployment procedure shall be:

```
git clone <repo> && cd klubhub && cp .env.example .env && docker compose up -d
```

No additional tools, scripts, or manual configuration steps shall be required beyond editing `.env` for API keys.

**Rationale:** Minimizing deployment friction is critical for adoption of a self-hosted open-source tool. One command to start.

### NFR-701: OS Compatibility

**Requirement:** The system shall run on any host that supports Docker Engine 24+ and Docker Compose v2+:

| Platform | Status |
|---|---|
| Linux (x86_64) | Primary target; CI-tested |
| macOS (Apple Silicon, Intel) via Docker Desktop | Supported; CI-tested on one variant |
| Windows via Docker Desktop (WSL2 backend) | Supported; tested manually |

Docker images shall be built as multi-architecture (`linux/amd64`, `linux/arm64`) to support both Intel and ARM hosts natively.

**Rationale:** DJs use macOS heavily (especially Apple Silicon MacBooks). ARM support is not optional.

### NFR-702: Environment Configuration

**Requirement:** All configurable parameters shall be exposed as environment variables in `.env`. The `.env.example` file shall document every variable with:

- A description comment.
- The default value.
- Whether the variable is required or optional.
- Security implications (for network binding, encryption keys).

Minimum required variables for a functional system (no external APIs):

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `POSTGRES_PASSWORD` | Yes | (none) | Database password |
| `MINIO_ROOT_USER` | Yes | (none) | Object storage admin user |
| `MINIO_ROOT_PASSWORD` | Yes | (none) | Object storage admin password |
| `TOKEN_ENCRYPTION_KEY` | Yes | (none) | AES-256-GCM key for token encryption |
| `BIND_ADDRESS` | No | `127.0.0.1` | Network bind address |
| `TZ` | No | `UTC` | Default timezone |

All other variables (API keys, ports, limits) have functional defaults.

### NFR-703: Automatic Database Migrations

**Requirement:** The API shall run database migrations automatically on startup, bringing the schema to the latest version. Migrations shall be embedded in the binary (no external files to manage). The migration framework shall support both `up` (apply) and `down` (rollback) migrations. On migration failure, the API shall log the error and exit with a non-zero status code (do not start serving traffic on a partially-migrated database).

**Rationale:** Users should never need to run manual database commands.

### NFR-704: Data Volume Persistence

**Requirement:** The `docker-compose.yml` shall define named volumes for database data and object storage data:

| Volume Name | Mount Point | Purpose |
|---|---|---|
| `klubhub-dj-db-data` | Database data directory | Database persistence |
| `klubhub-dj-storage-data` | Object storage data directory | File/blob persistence |

Named volumes survive `docker compose down` (without `-v`). Documentation shall warn users that `docker compose down -v` destroys all data.

### NFR-705: Version Tagging

**Requirement:** Docker images shall be tagged with semantic version numbers (e.g., `1.2.3`). A `latest` tag shall point to the most recent stable release. The API shall expose its version in the `/health` endpoint response and in startup logs.

**Rationale:** Version tagging enables users to pin deployments and roll back to known-good versions.

### NFR-706: Offline Operation

**Requirement:** The system shall start and operate in a fully offline environment (no internet access) with the following limitations:

- Cover art fetching returns placeholders for all tracks.
- Social media scheduling is non-functional (posts remain in `scheduled` status).
- All other modules (tracklist parsing, image generation, gig tracking, finance, EPK, tours) are fully functional.

The system shall not make any outbound network calls on startup (beyond what Docker itself requires). External API clients shall use timeouts (5 seconds for connect, 30 seconds for read) and fail gracefully.

**Rationale:** DJs at festivals or traveling may have no internet. Core workflows must work offline.

### NFR-707: Update Procedure

**Requirement:** Updating to a new version shall be achievable with:

```
git pull && docker compose up -d --build
```

The API handles migrations automatically (NFR-703). An `UPGRADING.md` file in the repository root shall document any version-specific manual steps (expected to be rare). The system shall not require data exports, re-imports, or fresh installations for minor or patch version updates.

**Rationale:** Simple updates encourage users to stay current with security patches and new features.

---

## 8. Maintainability and Extensibility

### NFR-800: Modular Monolith Architecture

**Requirement:** The backend shall be organized as a modular monolith with the following package structure constraints:

- Each module (tracklist, social, epk, gig, finance, release, tour) shall have its own package containing: routes, handlers, service layer, repository layer, and models.
- Cross-module dependencies shall flow through defined service interfaces, never through direct database queries across module boundaries.
- Adding a new module shall require: creating a new package, registering its routes, and adding its migration files. No changes to existing module code.
- A shared platform package may provide cross-cutting concerns: database connection, object storage client, HTTP client, logging, middleware.

**Rationale:** Clean module boundaries enable independent development per phase (1a through 1g) and future extraction into microservices if needed for v3.

**v3 Guardrail:** Each module's repository layer shall accept a context parameter capable of carrying tenant/user scope. In v1, this context carries no user information. In v3, middleware injects the authenticated user, and repositories filter all queries by user ID automatically.

### NFR-801: API Versioning

**Requirement:** All API endpoints shall be prefixed with `/api/v1`. The version number is part of the URL path, not a header. When breaking changes are needed (anticipated for v3 multi-user), a `/api/v2` prefix shall be introduced while maintaining `/api/v1` for backward compatibility for at least one major version.

**Rationale:** URL-based versioning is the simplest and most visible approach. It allows side-by-side old and new API behavior during transitions.

### NFR-802: Integration Adapter Interfaces

**Requirement:** External integrations shall be implemented behind abstract interfaces:

- `CoverArtProvider` — methods for search and fetch.
- `SocialMediaPublisher` — methods for auth, publish, and status check.
- `FileParser` — methods for detect-format and parse.
- `PDFGenerator` — methods for render.

Each concrete implementation (Spotify, Discogs, Instagram, Rekordbox parser, etc.) shall implement the relevant interface. New integrations (e.g., TikTok in v1.x, Beatport in v2.0) shall be addable by implementing the interface with no changes to calling code.

**Rationale:** The BRD explicitly requires adapter interfaces from day one. This formalizes the requirement with specific interface names and contracts.

### NFR-803: Code Documentation Standards

**Requirement:**

- All exported functions and types shall have documentation comments.
- All API endpoints shall be documented in an OpenAPI 3.x specification file maintained alongside the code.
- All environment variables shall be documented in `.env.example` (per NFR-702) and in a dedicated `docs/configuration.md` file.
- All database migrations shall include a header comment explaining the change.

**Rationale:** Open-source projects live or die by documentation quality. Contributors need clear entry points.

### NFR-804: Test Coverage Targets

**Requirement:**

| Test Type | Coverage Target | Scope |
|---|---|---|
| Unit tests | 80% line coverage on service and repository layers | Per-module |
| Integration tests | All API endpoints exercised with success and error cases | Cross-module |
| Parser tests | 100% of supported format variations (Rekordbox txt/csv/xml, Serato, Traktor) | Tracklist module |
| Image generation tests | Visual regression tests for each template at both output sizes | Tracklist module |

Test execution shall complete in under 2 minutes on the baseline hardware target without requiring external services (use test containers or embedded mocks).

**Rationale:** Phased delivery means each new module could break existing functionality. Automated tests are the safety net.

### NFR-805: Dependency Management

**Requirement:** Backend module dependencies shall be pinned to specific versions. Frontend dependencies shall use a lockfile. Dependency updates shall be a deliberate, reviewed action, not automatic. The project shall document its policy for evaluating and applying dependency updates.

**Rationale:** Predictable builds are essential for open-source projects where contributors build on different machines.

### NFR-806: Database Migration Conventions

**Requirement:** Database migrations shall follow these conventions:

- Sequential numbering: `001_`, `002_`, etc.
- Naming: `{number}_{description}.up.sql` and `{number}_{description}.down.sql`.
- Each migration is atomic (all-or-nothing within a transaction).
- Down migrations restore the previous state exactly (drop what was added, recreate what was dropped).
- Migrations never modify existing data in production-critical columns without a documented manual review step.

**Rationale:** Reliable migrations are critical for the update procedure (NFR-707). Down migrations enable rollback when updates go wrong.

### NFR-807: Error Handling Conventions

**Requirement:** The API shall use a consistent error handling pattern:

- Domain errors (validation, not found, conflict) are typed errors with HTTP status codes.
- Infrastructure errors (database, object storage, external API) are wrapped with context and logged.
- All API error responses use a consistent JSON structure:

```json
{
  "error": {
    "code": "MACHINE_READABLE_CODE",
    "message": "Human-readable message",
    "details": []
  }
}
```

- Error codes are namespaced by module (e.g., `TRACKLIST_PARSE_FAILED`, `SOCIAL_TOKEN_EXPIRED`).

**Rationale:** Consistent error handling enables the frontend to display meaningful messages and enables API consumers to handle errors programmatically.

### NFR-808: Frontend Module Awareness

**Requirement:** The frontend shall detect active modules by querying module-specific health sub-endpoints (per Addendum A7.2). Unshipped modules shall have no UI presence (no menu items, no dashboard widgets, no CTAs). Module activation shall be automatic on the next page load after the API is updated with a new module.

**Rationale:** Phased delivery means the frontend must gracefully handle a partially-deployed backend. No dead links or broken UI for modules that do not exist yet.

### NFR-809: Parser Extensibility

**Requirement:** Adding support for a new DJ software format (e.g., VirtualDJ in v3) shall require implementing the `FileParser` interface and registering the implementation. No existing parser code or format detection logic shall need modification. The format detection mechanism shall support a registry of parsers that declare their detection patterns (magic bytes, file extensions, header signatures).

**Rationale:** Community contributors are the likely source of new parser implementations. The barrier to contribution must be minimal.

### NFR-810: Frontend Build and Bundle Standards

**Requirement:** The frontend production build shall:

- Produce a server bundle under 2 MB (compressed).
- Produce client-side JavaScript bundles under 500 KB total (compressed, excluding images/fonts).
- Use code splitting so each module page loads only its own JavaScript.
- Achieve a Lighthouse Performance score of 80+ on the dashboard page.

**Rationale:** Bundle size affects initial load time (NFR-206). Code splitting ensures that adding Phase 1g code does not slow down Phase 1a pages.

### NFR-811: Configuration Validation on Startup

**Requirement:** The API shall validate all environment variable configuration on startup before opening connections or serving traffic. Validation rules:

| Condition | Behavior |
|---|---|
| Required variable missing | FATAL log and exit |
| Required variable empty | FATAL log and exit |
| Numeric variable non-numeric | FATAL log with variable name and exit |
| Port variable out of range (1–65535) | FATAL log and exit |
| TOKEN_ENCRYPTION_KEY not 64 hex characters (32 bytes) | FATAL log with key-generation instruction and exit |
| Optional API key missing | INFO log indicating degraded functionality |

**Rationale:** Fail fast with clear messages rather than cryptic errors later in execution.

---

## Appendix A: Performance Budget Summary

Consolidated view of all quantitative performance targets from Section 2.

| Category | Operation | p95 Target | p99 Target | Notes |
|---|---|---|---|---|
| **API CRUD** | Single entity GET | 50ms | 100ms | |
| **API CRUD** | Paginated list (up to 100) | 100ms | 200ms | |
| **API CRUD** | Single entity POST/PUT/DELETE | 100ms | 200ms | |
| **API CRUD** | Dashboard aggregation | 300ms | 500ms | Progressive loading if >1s |
| **File Parse** | Under 30 tracks | 1s | 2s | Excludes cover art fetch |
| **File Parse** | 30–100 tracks | 3s | 5s | |
| **File Parse** | 100–500 tracks | 10s | 15s | |
| **File Parse** | 500+ tracks | 30s | 45s | With progress indication |
| **Image Gen** | Up to 30 tracks, solid bg | 3s | 5s | |
| **Image Gen** | Up to 30 tracks, custom bg | 5s | 8s | |
| **Image Gen** | Up to 30 tracks, mosaic bg | 10s | 15s | BRD "under 10s" target |
| **Image Gen** | 31–100 tracks, any bg | 15s | 20s | |
| **Image Gen** | 100+ tracks, any bg | 30s | 45s | With progress indication |
| **Cover Art** | Per-track API lookup (cache miss) | 5s | 10s | Across full fallback chain |
| **Cover Art** | Per-track cache hit | 100ms | 200ms | Object storage fetch |
| **Cover Art** | 30-track batch (concurrent) | 30s | 45s | 5+ concurrent fetches |
| **PDF Gen** | Minimal EPK | 5s | 8s | Bio + 3 photos |
| **PDF Gen** | Full EPK (20 photos) | 15s | 20s | All sections |
| **PDF Gen** | Invoice | 3s | 5s | Single gig |
| **Preview** | Initial render | 2s | 3s | 50% resolution |
| **Preview** | Update on config change | 500ms | 800ms | 50% resolution |
| **Dashboard** | Full page load | 2s | 3s | All active widgets |
| **Health** | Health check response | 200ms | 500ms | No expensive checks |
| **System** | Cold start (all 4 services) | 60s | 90s | To first healthy response |

---

## Appendix B: Data Growth Projections and Storage Management

### B.1 Growth Model Assumptions

Based on a working DJ performing 1–2 gigs per week, generating tracklist images for each set, and actively using social posting:

| Year | Tracklists | Tracks | Images | Cover Art (unique) | Posts | Gigs | Est. Total Storage |
|---|---|---|---|---|---|---|---|
| Year 1 | 100 | 3,000 | 250 | 2,500 | 150 | 75 | 2–5 GB |
| Year 2 | 200 | 6,000 | 500 | 4,000 | 300 | 150 | 4–9 GB |
| Year 3 | 300 | 9,000 | 750 | 5,000 | 450 | 225 | 6–12 GB |
| Year 5 | 500 | 15,000 | 1,250 | 7,000 | 750 | 375 | 10–20 GB |

Cover art unique count decelerates because the same popular tracks recur across sets.

### B.2 Storage Growth by Category

| Category | Growth Driver | Avg Size per Object | 5-Year Projection |
|---|---|---|---|
| Source files | Tracklist uploads | 0.5–1 MB | 250–500 MB |
| Cover art cache | Unique artist/track combos | ~500 KB | 3–4 GB |
| Generated images (PNG) | 2 per tracklist (story+square) | 2–6 MB | 2.5–7.5 GB |
| EPK assets | Press photos, PDFs | 2–5 MB per photo | 500 MB–1 GB |
| Tour documents | PDFs, scans | Variable | 200–500 MB |
| Database | All relational data | N/A | 100–300 MB |

### B.3 Storage Management Recommendations (To Document for Users)

1. **Generated images** are the largest and most disposable category. Recommend periodic cleanup of images older than 6 months using the bulk delete tool (Addendum A14.1).
2. **Cover art cache** is the second largest but provides ongoing value (speeds up future image generation for repeat tracks). Recommend keeping indefinitely unless disk space is critical.
3. **Source files** can be deleted after parsing since the parsed data lives in the database. Recommend keeping for audit/re-parse purposes but documenting that deletion is safe.
4. **Backup archive size** will approximate total object storage usage plus ~100 MB for the database dump. Users should plan for 2x their current usage for backup storage.

---

## Appendix C: v3 Multi-User Guardrails Summary

All v3 guardrails mentioned throughout this document, consolidated for architectural planning.

| NFR | Guardrail | v1 Implementation | v3 Activation |
|---|---|---|---|
| NFR-106 | User-scoped storage bucket prefixes | `users/default/` prefix on all paths | Replace `default` with `{user_id}` |
| NFR-305 | User-scoped repository queries | Context parameter exists but carries no user info | Middleware injects authenticated user; repositories filter |
| NFR-506 | Multi-origin CORS | Single origin in env var | Comma-separated list of origins |
| NFR-507 | Per-user rate limiting | Rate-limit middleware exists but is disabled | Enable; swap key function from IP to user ID |
| NFR-512 | `user_id` column on all tables | Column exists, nullable, set to default singleton UUID | Column becomes NOT NULL; foreign key to users table |
| NFR-603 | User ID in log context | Request ID only | Add `user_id` to structured log fields |
| NFR-800 | User-scoped service context | Context parameter on all repository methods | Context carries authenticated user |

**Architectural principle:** Every data access path in v1 shall pass through a context-aware repository method. The context is empty in v1 but provides the injection point for user scoping in v3. This is a zero-cost abstraction in v1 that prevents a full rewrite in v3.

---

## Appendix D: Requirement Traceability Matrix

| NFR ID | Dimension | Short Title | Origin |
|---|---|---|---|
| NFR-100 | Availability | Service availability target | New |
| NFR-101 | Availability | Graceful degradation | BRD S17, Addendum A4.4/A14.4 |
| NFR-102 | Availability | Social post retry | BRD S17, S7.4 |
| NFR-103 | Availability | Database durability | New |
| NFR-104 | Availability | Object storage durability | New |
| NFR-105 | Availability | Crash recovery | New |
| NFR-106 | Availability | Write consistency | Addendum NFR-005 (superseded) |
| NFR-107 | Availability | Backup/restore | Addendum NFR-006 (superseded) |
| NFR-200 | Performance | API CRUD response time | BRD S17 (refined) |
| NFR-201 | Performance | File parse time | New (detailed) |
| NFR-202 | Performance | Image generation time | BRD S17 (refined) |
| NFR-203 | Performance | Cover art fetch time | New |
| NFR-204 | Performance | PDF generation time | New |
| NFR-205 | Performance | Live preview rendering | Addendum A5.1 (formalized) |
| NFR-206 | Performance | Dashboard load time | BRD S17 (augmented) |
| NFR-207 | Performance | Font/Unicode/Accessibility | Addendum NFR-004 + BRD S17 (merged) |
| NFR-300 | Scalability | Concurrency model | BRD S17 (formalized) |
| NFR-301 | Scalability | No horizontal scaling | BRD S17 (restated) |
| NFR-302 | Scalability | Concurrent image gen | Addendum A5.2 (formalized) |
| NFR-303 | Scalability | Concurrent cover art fetch | New |
| NFR-304 | Scalability | Reference dataset | New |
| NFR-305 | Scalability | Pagination | New |
| NFR-306 | Scalability | Index strategy | New |
| NFR-400 | Resource | Docker resource limits | New |
| NFR-401 | Resource | Minimum disk space | New |
| NFR-402 | Resource | No paid dependencies | New (implicit in BRD) |
| NFR-403 | Resource | Docker image size | New |
| NFR-404 | Resource | Idle CPU usage | New |
| NFR-500 | Security | Network binding | Addendum NFR-001 (superseded) |
| NFR-501 | Security | Token encryption | Addendum A1.3 (formalized) |
| NFR-502 | Security | File upload validation | BRD S17 (detailed) |
| NFR-503 | Security | Request body validation | New |
| NFR-504 | Security | SQL injection prevention | New (standard) |
| NFR-505 | Security | XSS prevention | New (standard) |
| NFR-506 | Security | CORS policy | New |
| NFR-507 | Security | Rate limiting (guardrail) | New (v3 prep) |
| NFR-508 | Security | Secrets management | BRD S17 (detailed) |
| NFR-509 | Security | Dependency scanning | New |
| NFR-510 | Security | Content Security Policy | New |
| NFR-511 | Security | Object storage access control | Addendum A1.2 (formalized) |
| NFR-512 | Security | Audit-ready data model | Addendum A13.2 (extended) |
| NFR-600 | Observability | Structured logging | New |
| NFR-601 | Observability | Health endpoint | Addendum NFR-003 (superseded) |
| NFR-602 | Observability | Docker health checks | New |
| NFR-603 | Observability | Request tracing | New |
| NFR-604 | Observability | Error logging standards | New |
| NFR-605 | Observability | Startup diagnostics | New |
| NFR-606 | Observability | Storage usage reporting | Addendum A14.1 (formalized) |
| NFR-700 | Portability | Docker Compose deployment | BRD S16 (formalized) |
| NFR-701 | Portability | OS compatibility | BRD S17 (detailed) |
| NFR-702 | Portability | Environment configuration | BRD S16 (detailed) |
| NFR-703 | Portability | Auto migrations | Addendum NFR-002 (superseded) |
| NFR-704 | Portability | Data volume persistence | New |
| NFR-705 | Portability | Version tagging | New |
| NFR-706 | Portability | Offline operation | New |
| NFR-707 | Portability | Update procedure | Addendum A14.3 (formalized) |
| NFR-800 | Maintainability | Modular monolith | BRD S17 (detailed) |
| NFR-801 | Maintainability | API versioning | New |
| NFR-802 | Maintainability | Adapter interfaces | BRD S5.1, S17 (formalized) |
| NFR-803 | Maintainability | Documentation standards | New |
| NFR-804 | Maintainability | Test coverage | New |
| NFR-805 | Maintainability | Dependency management | New |
| NFR-806 | Maintainability | Migration conventions | Addendum A14.3 (formalized) |
| NFR-807 | Maintainability | Error handling conventions | New |
| NFR-808 | Maintainability | Frontend module awareness | Addendum A7.2 (formalized) |
| NFR-809 | Maintainability | Parser extensibility | BRD S17 (detailed) |
| NFR-810 | Maintainability | Frontend bundle standards | New |
| NFR-811 | Maintainability | Config validation on startup | Addendum A14.4 (formalized) |

---

*End of Non-Functional Requirements Document. This document, together with BRD v2.0 and Addendum v2.0-A, forms the complete requirements baseline for KlubHub DJ.*
