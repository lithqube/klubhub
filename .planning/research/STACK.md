# Stack Research

**Domain:** Self-hosted DJ career management platform (Go + Nuxt + PostgreSQL + MinIO)
**Researched:** 2026-03-14
**Confidence:** MEDIUM-HIGH — core stack verified against project docs and known library status; external web verification unavailable in this session

---

## Executive Summary

The architecture document (ARCHITECTURE.md v1.0) has already made solid, well-justified stack decisions. This research validates those decisions, flags two library concerns (gofpdf and golang-migrate), and provides version-specific guidance. The Go backend and Nuxt 4 frontend choices are sound. The main correction needed is the PDF library: `jung-kurt/gofpdf` is archived; `go-pdf/fpdf` (the community-maintained fork) is the correct choice. The architecture doc already hedges this with "(or fpdf2)" — fpdf2 / go-pdf/fpdf is the right resolution. `golang-migrate` (mentioned in the question) is not in the architecture decision; the architecture correctly chose `goose` instead, which is the better choice for embedded-migration use cases.

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.23+ (target 1.24 when stable) | Backend language | Static binary, excellent concurrency for cover art fetch goroutine pool, cross-compiles to linux/amd64 + linux/arm64 for Docker multi-arch. NFR-403 <100MB image target achievable. |
| Nuxt | 4.x (currently ^4.0.0) | Frontend framework | Already scaffolded. Nuxt 4 is backward-compatible with Nuxt 3 patterns but uses a unified app/ directory structure. SSR for fast initial load. h3 is its native server-side HTTP layer. |
| Vue | 3.5.x | UI component model | Nuxt 4 bundles Vue 3. Composition API + `<script setup>` is the idiomatic pattern; no Options API needed. |
| TypeScript | 5.9.x | Type safety | Already configured (tsconfig strict mode in repo). |
| PostgreSQL | 16-alpine | Relational database | Relational integrity across 7 modules, JSONB for config fields, partial indexes for scheduler hot path, pg_dump for backup. NFR-103, NFR-107. |
| MinIO | latest stable | Object storage | S3-compatible, self-hosted, zero cost. Single-drive mode sufficient for single-user. NFR-104, NFR-511. |
| Docker + Compose v2 | Engine 24+ | Deployment | Single `docker compose up -d`. Four-service stack. NFR-700. |

### Go Backend Libraries

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| `go-chi/chi` | v5 | HTTP router | Lightweight, stdlib-compatible, middleware composable. No framework magic — just clean routing. Better than Gin/Echo for idiomatic Go; no reflect-based binding. |
| `jackc/pgx` | v5 | PostgreSQL driver | Native PostgreSQL protocol, prepared statements (SQL injection prevention NFR-504), COPY support, better performance than database/sql + lib/pq. pgxpool for connection pool (min 2, max 10 per arch doc). |
| **`pressly/goose`** | v3 | DB migrations | Embedded migrations via `embed.FS` (NFR-703 — migrations run at API startup, embedded in binary). Sequential numbering. Up/down support (NFR-806). Simple CLI also available. |
| `minio/minio-go` | v7 | MinIO / S3 client | Official SDK. S3-compatible — future-proofs against swapping MinIO for AWS S3. |
| **`go-pdf/fpdf`** | v2.7+ | PDF generation | Active community fork of the archived jung-kurt/gofpdf. Drop-in compatible. Pure Go (no Chromium). Suitable for EPK press kit and invoice PDFs. See WARNING below. |
| `fogleman/gg` | latest (v1.3.0) | 2D graphics / image composition | Pure Go canvas API. Handles composite layering (background + cover art grid + text overlays). Low maintenance but stable — API is not changing. Correct for this use case. |
| `disintegration/imaging` | v1.6.2 | Image resizing / cropping | Pure Go. Used for cover art thumbnails, mosaic tile preparation, image scaling. Stable and actively patched. Pairs naturally with gg. |
| `rs/zerolog` | v1 | Structured logging | Zero-allocation JSON logs (NFR-600). Better than log/slog for structured fields ergonomics at this point. No performance overhead for high-volume cover art operations. |
| `kelseyhightower/envconfig` | v2 | Env var config loading | Struct-tag-based env var mapping (NFR-702). Self-documenting via `desc` tags. Used by many self-hosted Go projects for exactly this pattern. |
| `go-playground/validator` | v10 | Request body validation | Battle-tested, struct-tag-based. Handles ISO 8601 dates, enum validation, decimal ranges (NFR-503). |
| `microcosm-cc/bluemonday` | v1 | HTML sanitizer | Allow-list-based XSS sanitizer for EPK rich text bios (NFR-505). Use with Tiptap output before storage. |
| `testcontainers/testcontainers-go` | latest | Real-DB integration tests | Spins up ephemeral PostgreSQL containers for tests. No mock DB. NFR-804. Required for repository layer tests. |
| `swaggo/swag` | v2 | OpenAPI 3.x spec generation | Annotation-based OpenAPI generation from Go handler comments. Committed `api/openapi.yaml`. NFR-803. |
| `google/uuid` | v1 | UUID generation | Standard library for UUID v4 primary keys. No alternative needed. |

### Frontend Libraries

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| `pinia` | v2 | State management | Nuxt-native, Vue 3 idiomatic. Composable stores with TypeScript support. Lighter than Vuex. |
| `$fetch` / ofetch | Nuxt built-in | HTTP client | Auto-proxies SSR/CSR transparently via Nuxt's useAsyncData and useFetch composables. Handles API base URL routing. No additional Axios needed. |
| Nuxt UI | latest (v2/v3) | Component library | **Recommended over shadcn-vue for this project.** Built for Nuxt, ships accessible components (WCAG 2.1 AA per NFR-207), no runtime CSS-in-JS, integrates Tailwind. Fewer configuration steps than shadcn-vue for a Nuxt 4 project. |
| `tiptap` | v2 | Rich text editor | Vue 3 native, extensible, output is JSON or HTML sanitizable by bluemonday on the API side. Required for EPK bio field (A8.1). |
| `dayjs` | latest (1.11.x) | Date/time handling | Lightweight (2KB gzipped). Timezone plugin for scheduler timezone display (A6.3). Better bundle size than date-fns for this use case. |
| `h3` | v1.8.x | Server-side API routes | h3 is the HTTP framework underlying Nuxt server routes. Already a peer dependency (`"h3": "^1.8.2"` in package.json). Use for Nuxt API route handlers in `server/api/`. Not an alternative to the Go backend — used only for Nuxt's own server-side proxy handlers if needed. |

### Infrastructure / Tooling

| Tool | Version | Purpose | Why |
|------|---------|---------|-----|
| pnpm | 9+ | JS package manager | Already in use (pnpm-workspace.yaml). Faster installs, strict dependency isolation. |
| Nx | 22.5.x | Monorepo orchestration | Already scaffolded. Task dependency graph, build caching, affected-file detection. |
| Playwright | ^1.36.0 | E2E testing | Already scaffolded in dj-e2e. Standard for Vue/Nuxt E2E. |
| Vitest | ^4.0.x | Unit/integration testing (frontend) | Vite-native, fast. Already configured in vitest.workspace.ts. |
| Noto Sans family | — | Bundled Unicode fonts | Full CJK/Cyrillic/Arabic glyph coverage for image generator text rendering (NFR-207, A2.7). Apache 2.0 license. Bundle Regular, Bold, and NotoSansCJK-Regular. |

---

## WARNING: BRD Library Flags

### gofpdf (jung-kurt/gofpdf) — DO NOT USE

**Status: Archived.** The `jung-kurt/gofpdf` repository was archived by its author and is no longer maintained. It has known unresolved issues and no security patches since ~2021.

**Use instead: `go-pdf/fpdf` v2.7+**

`go-pdf/fpdf` is the community-maintained fork with active development, bug fixes, and the same API surface. It is a drop-in replacement. The architecture document already hedges this correctly ("gofpdf or fpdf2") — resolve to `go-pdf/fpdf`.

```
go get github.com/go-pdf/fpdf@v2
```

The `PDFGenerator` interface in `epk/pdf/generator.go` means the implementation detail is isolated. The swap adds zero architectural risk.

**Confidence: HIGH** — gofpdf archive status is a well-known fact in the Go ecosystem.

---

### golang-migrate — NOT IN THIS PROJECT'S STACK

**The question asks about `golang-migrate` but this project uses `goose` (pressly/goose v3).** This is the correct decision. Here is the comparison:

| Concern | golang-migrate | goose (chosen) |
|---------|----------------|----------------|
| Embedded migrations | Requires custom source driver | Native `embed.FS` support built-in |
| API startup integration | CLI-first, awkward in-process | Designed for in-process use |
| Migration file format | `.up.sql` / `.down.sql` pairs | Same, plus Go-based migrations |
| v3 multi-user support | Equal | Equal |
| Maintenance | Active | Active |

**goose is the right choice.** Do not switch to golang-migrate. The embedded migration pattern (migrations shipped inside the API binary, run at startup) is cleaner with goose.

**Confidence: HIGH** — Based on direct API comparison and both being actively maintained with different strengths.

---

### h3 — CORRECT USE, NOT AN ALTERNATIVE TO GO API

**h3** (`"h3": "^1.8.2"` in package.json) is the HTTP micro-framework underlying Nuxt's server directory (`server/api/`, `server/routes/`). It is already present as a peer dependency.

h3 is **not** an alternative to the Go backend. It is used for:
- Nuxt server-side API route handlers in `server/api/` (if any are needed for frontend-side concerns)
- Middleware in the Nuxt server layer
- The Go API is the authoritative backend; h3 routes in Nuxt should only be used for thin proxying or BFF patterns if required

**No action needed.** h3 is correctly present and at the right version.

**Confidence: HIGH** — Package.json confirms current installation; h3 role is well-documented in Nuxt 4 docs.

---

### Instagram Graph API — CORRECT CHOICE, WITH CAVEATS

**Instagram Graph API (via Facebook Business / Meta Graph API)** is the correct and only supported programmatic API for Instagram feed and story publishing for business/creator accounts.

**Key constraints for implementation:**
- Requires Facebook Business account + Instagram Professional account (Creator or Business)
- Media Container + Publish two-step flow for image posts
- Stories publishing requires the same two-step approach
- Rate limit: 25 posts per 24 hours per account (enforce in scheduler)
- Access tokens: long-lived tokens last 60 days; implement the daily refresh job per A6.1
- The v1 scheduler design (30s minimum between posts, retry backoff) is correct

**No alternative exists** for programmatic Instagram posting. The TikTok Content Posting API is a future follow-up (out of scope v1).

**Confidence: MEDIUM** — Based on known Meta Graph API behavior; specific rate limit numbers and token lifetimes should be verified against current Meta documentation before Phase 1b implementation.

---

### gg (fogleman/gg) — ACCEPTABLE, WITH AWARENESS

`gg` is in low-maintenance mode (last meaningful commit ~2022) but the API is stable and it remains the most capable pure-Go 2D graphics library with text rendering. Alternatives:

| Library | Verdict |
|---------|---------|
| `fogleman/gg` | Chosen. Stable API. Handles compositing + text. Low maintenance but no active breaking changes. |
| `golang.org/x/image` | Too low-level; would require building layout logic from scratch |
| headless Chrome / chromedp | Explicitly rejected (NFR-403 — Docker image size, no external runtime) |
| `imagor` (image server) | Over-engineered; adds another service |

**Accept gg.** Its maintenance status is not a risk for this use case — the feature set needed (DrawImage, DrawString, LoadFont, compositing) is all present and stable.

**Confidence: MEDIUM** — Based on known library status; verify latest commit activity before Phase 1a implementation.

---

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Go HTTP router | chi v5 | Gin, Echo, stdlib | Gin/Echo add framework overhead; stdlib `net/http` mux lacks middleware ergonomics for 7-module routing |
| DB migration | goose v3 | golang-migrate | golang-migrate is CLI-first; goose designed for in-process embedded migration at startup |
| PDF generation | go-pdf/fpdf | gofpdf (jung-kurt) | jung-kurt/gofpdf is archived; go-pdf/fpdf is the active fork |
| PDF generation | go-pdf/fpdf | chromedp | chromedp requires headless Chrome runtime — violates NFR-403 (image size) and NFR-403 (no external runtime) |
| Frontend state | Pinia | Vuex 4 | Vuex 4 is deprecated in favor of Pinia for Vue 3 |
| Frontend date/time | dayjs | date-fns, Luxon | date-fns is larger bundle; Luxon is good but dayjs suffices for timezone display + formatting |
| UI components | Nuxt UI | shadcn-vue | shadcn-vue is excellent but requires more manual setup in Nuxt 4; Nuxt UI is purpose-built for Nuxt |
| Logging | zerolog | log/slog (stdlib) | slog is viable but zerolog's ergonomics for structured fields in handler middleware are superior; zerolog is already specified in arch doc |
| Job queue | DB-backed polling (postgres) | Asynq + Redis | Redis adds a 5th Docker service; NFR-301 (single instance), NFR-402 (zero paid deps). Polling at 60s is sufficient for 1-post/minute volume |
| Image generation | gg + imaging | image/draw (stdlib) | Stdlib lacks font rendering, text layout, compositing helpers needed for multi-track visual templates |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `jung-kurt/gofpdf` | Archived, unmaintained since ~2021. Security vulnerabilities may not be patched. | `go-pdf/fpdf` v2.7+ |
| Redis / Asynq | Adds 5th Docker service, violates NFR-402 (zero external paid/infra deps) and NFR-301 (single instance self-hosted) | DB-backed polling with goose-managed `scheduled_posts` table |
| Vuex 4 | Deprecated. Vue/Nuxt team officially recommends Pinia for Vue 3 | Pinia v2 |
| Axios in Nuxt | Redundant — Nuxt 4's `$fetch` / ofetch handles SSR/CSR isomorphic requests natively | `useFetch` / `useAsyncData` / `$fetch` |
| `gorilla/mux` | Archived (2022). No active maintenance. | `go-chi/chi` v5 |
| `lib/pq` (PostgreSQL driver) | Maintenance mode; missing pgx's native protocol benefits | `jackc/pgx` v5 |
| `database/sql` + pgx | pgx v5 has its own idiomatic API (pgxpool) that is more ergonomic and performant than the database/sql wrapper | `pgxpool.Pool` directly |
| headless Chrome / chromedp | Adds ~1.5GB to Docker image, Chrome runtime dependency, headless launch complexity — violates NFR-403 | `go-pdf/fpdf` for PDFs, `gg` + `imaging` for images |
| `golang-migrate/migrate` | Not wrong, just not chosen. Goose is a better fit for in-process embedded migration at startup. | `pressly/goose` v3 |
| Kubernetes / Helm | Over-engineered for single-user self-hosted tool. NFR-700 requires `docker compose up` | Docker Compose v2 |

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| Nuxt ^4.0.0 | Vue ^3.5.x, h3 ^1.8.x | Already in package.json. Nuxt 4 uses `app/` directory; Nuxt 3's `pages/` still works via compat flag |
| Pinia v2 | Vue 3.5.x, Nuxt 4 | Nuxt 4 auto-imports Pinia stores |
| pgx v5 | PostgreSQL 16 | pgx v5 requires Go 1.19+; Go 1.23 in use — no issue |
| goose v3 | PostgreSQL 16, `embed.FS` | Requires Go 1.16+ for embed; Go 1.23 — no issue |
| go-pdf/fpdf v2 | Go 1.18+ | Module path: `github.com/go-pdf/fpdf/v2` |
| testcontainers-go | Docker Engine 24+ | Docker Compose v2 already required for deployment — dev environment has it |
| Nx 22.5.x | Nuxt ^4.0.0, Node 22 | `@nx/nuxt` 22.5.4 already in package.json |
| TypeScript 5.9.x | Vue-tsc 2.2.x | Already in package.json — matched |

## Installation

### Go Backend (`api/go.mod` additions)

```bash
go get github.com/go-chi/chi/v5@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/pressly/goose/v3@latest
go get github.com/minio/minio-go/v7@latest
go get github.com/go-pdf/fpdf/v2@latest
go get github.com/fogleman/gg@latest
go get github.com/disintegration/imaging@latest
go get github.com/rs/zerolog@latest
go get github.com/kelseyhightower/envconfig@latest
go get github.com/go-playground/validator/v10@latest
go get github.com/microcosm-cc/bluemonday@latest
go get github.com/google/uuid@latest
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/swaggo/swag@latest
go get github.com/swaggo/http-swagger/v2@latest
```

### Frontend (`pnpm` additions to dj app)

```bash
# Core additions (pnpm install in dev/ workspace)
pnpm add pinia @pinia/nuxt
pnpm add @nuxt/ui
pnpm add @tiptap/vue-3 @tiptap/pm @tiptap/starter-kit @tiptap/extension-link
pnpm add dayjs
# h3 is already a devDependency — no action needed
```

## Stack Patterns by Variant

**If the PDF output needs richer formatting (post-v1):**
- Implement `ChromedpGenerator` behind the existing `PDFGenerator` interface
- Make it opt-in via `PDF_ENGINE=chromedp` env var
- The interface boundary in `epk/pdf/generator.go` is the clean extension point
- This is the explicit v2+ upgrade path in the architecture doc

**If social posting expands to TikTok (v1.x):**
- Add `TikTokPublisher` implementing the existing `SocialMediaPublisher` interface
- TikTok Content Posting API requires a separate app registration on the TikTok Developer Portal
- No stack change required — same scheduler, same DB table with `platform` enum extended

**If multi-user is added (v3.0+):**
- Add authentication middleware — all repository methods already accept `context.Context`
- All tables already have nullable `user_id` UUID columns
- MinIO paths can be prefixed with `users/{user_id}/` per the bucket structure guardrail
- The only stack addition: a session/JWT library (e.g., `golang-jwt/jwt`)

---

## Sources

- `/docs/ARCHITECTURE.md` v1.0 — Stack decisions table (§7), justified against NFRs. **PRIMARY SOURCE.** HIGH confidence.
- `/docs/KlubHub_DJ_NFR_v1.0.md` v1.0 — NFR-200, NFR-301, NFR-402, NFR-403, NFR-503, NFR-600, NFR-703, NFR-804 — library selection drivers
- `/docs/KlubHub_DJ_BRD_v2.0_Platform.md` §5 — Original BRD technology stack table
- `/docs/KlubHub_DJ_BRD_v2.0_Addendum.md` — A6.1 (token refresh), A8.1 (rich text), A6.3 (timezone), A6.4 (rate limiting)
- `/package.json` — Confirmed: nuxt ^4.0.0, h3 ^1.8.2, vue ^3.5.13, typescript ~5.9.2, vitest ^4.0.x, Nx 22.5.4
- Training knowledge — `jung-kurt/gofpdf` archive status, `gorilla/mux` archive status, `lib/pq` maintenance-mode status, goose vs golang-migrate comparison. MEDIUM confidence — web verification unavailable in this session.

---

*Stack research for: KlubHub DJ — self-hosted Go + Nuxt 4 + PostgreSQL + MinIO DJ career platform*
*Researched: 2026-03-14*
