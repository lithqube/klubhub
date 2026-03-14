# Project Research Summary

**Project:** KlubHub DJ
**Domain:** Self-hosted DJ career management platform — open-source, single-user
**Researched:** 2026-03-14
**Confidence:** MEDIUM-HIGH

## Executive Summary

KlubHub DJ is a self-hosted, open-source career toolkit for DJs that chains together a workflow no existing product covers end-to-end: DJ history file → tracklist image generation → social scheduling → gig tracking → finance/invoicing → EPK press kit. The correct technical approach is a Go modular monolith (chi router, pgx v5, goose migrations) paired with a Nuxt 4 frontend, backed by PostgreSQL 16 and MinIO — all orchestrated via a single `docker compose up`. This four-service stack is well-justified against the NFRs and is the approved baseline in the existing architecture document. The Nx monorepo workspace is already scaffolded with Nuxt 4, so infrastructure foundation work is partially complete.

The recommended build sequence is infrastructure first, then the tracklist image generator (the flagship feature that defines the product's identity), then the social scheduler, followed by EPK, the gig tracker hub, and the dependent modules (finance, tour, release). The gig tracker is a hard dependency for finance, tour manager, and EPK's live-data import — it must be built before those three. The social scheduler carries the single highest external dependency risk: Instagram Graph API App Review must be submitted at the start of Phase 1b, not the end, or the entire phase ships without real-account validation.

The three most consequential risks across all phases are: (1) Instagram App Review blocking Phase 1b launch for 1–4 weeks, (2) cover art fetch thundering herd / singleflight omission causing Spotify 429 errors at tracklist parse time, and (3) font rendering failures for non-Latin track names (CJK, Arabic, Cyrillic) in generated images — which must be solved in Phase 1a because it is launch-blocking for a global audience. All three have clear prevention strategies documented in the pitfalls research and should be written into phase acceptance criteria.

## Key Findings

### Recommended Stack

The stack is already well-decided and validated. The Go backend uses chi v5 for routing, pgx v5 for PostgreSQL, goose v3 for embedded-binary migrations, minio-go v7 for object storage, go-pdf/fpdf v2 for PDF generation (NOT the archived jung-kurt/gofpdf), gg + imaging for image composition, and zerolog for structured logging. The frontend is Nuxt 4 + Vue 3 + Pinia + Nuxt UI, with no Axios needed (use Nuxt's built-in `$fetch`). There are two critical library corrections relative to the BRD's original mentions: `jung-kurt/gofpdf` is archived and must not be used — use `go-pdf/fpdf` v2.7+; `gorilla/mux` is also archived — use `go-chi/chi` v5.

**Core technologies:**
- Go 1.23+: Backend language — static binary, excellent concurrency for cover art goroutine pool, <100MB Docker image achievable
- Nuxt 4 / Vue 3 / TypeScript: Frontend — already scaffolded in repo; SSR for fast initial load, Nitro proxy for API routing
- PostgreSQL 16-alpine: Relational database — FK integrity across 7 modules, JSONB for config fields, pg_dump for backup
- MinIO (latest stable): Object storage — S3-compatible self-hosted, zero cost, single-bucket prefix structure
- Docker Compose v2: Deployment — single `docker compose up -d`, four-service stack
- go-pdf/fpdf v2.7+: PDF generation — active community fork; replaces archived jung-kurt/gofpdf
- gg (fogleman/gg) + imaging: Image composition — stable pure-Go pipeline for tracklist visuals; low maintenance but safe for this use case
- goose v3: DB migrations — embedded via `embed.FS`, runs at API startup (correct choice over golang-migrate for this pattern)
- Noto Sans family: Bundled fonts — required for Unicode rendering (CJK, Arabic, Cyrillic) in generated images

### Expected Features

No single competitor (Buffer/Later, Canva, EPK.io, Sonicbids, spreadsheets) covers the full workflow. The differentiation is the combination and the self-hosted, open-source nature — not any individual feature.

**Must have (table stakes for v1):**
- DJ file parsing for Rekordbox, Serato, and Traktor formats — missing any format excludes 1/3 of the audience
- Cover art auto-fetch with fallback chain (Spotify → Discogs → MusicBrainz → placeholder) — visual quality is the core product promise
- Tracklist image generation with presets, Story and Square sizes, mosaic background — the flagship differentiator
- Live preview at 50% resolution — required for the experience to feel trustworthy
- Instagram social scheduling with retry, timezone-aware, queue/calendar view — no silent failures
- Gig CRUD with status workflow and calendar/list views — table stakes for any gig tracker
- Finance income/expense logging with PDF invoice generation — many markets require invoices for payment
- EPK bio, press photos, PDF export with section control — an EPK that can't be emailed as an attachment is not usable
- Unified dashboard aggregating all modules — without this, the app feels like 7 disconnected tools
- Unicode rendering (Noto Sans bundled) — electronic music has global artist names; launch-blocking gap
- Backup/restore scripts — self-hosters ask "how do I backup?" on day one

**Should have (competitive differentiation):**
- Cover art mosaic background from set artwork — genuinely novel; no competitor can produce this
- Post-export "Schedule Post" CTA — one-click handoff from tracklist to social post; unique cross-module linkage
- Gig-linked tracklists and images — unified record across career data
- Auto income entry on gig payment status → paid transition — eliminates a manual spreadsheet step
- Track range selector for large sets (100+ tracks) — trust-building for real-world 4-hour sets
- EPK "Import from Gigs" button — always-current EPK vs. static Word/Canva alternative
- Tour manager with per-stop logistics and budget aggregate

**Defer to v2+:**
- AI-generated backgrounds — requires GPU or paid API; cover art mosaic is the genuine differentiator
- External platform integrations (Resident Advisor, Beatport, Bandcamp) — adapter interfaces planned from day one; implementations in v2
- Multi-user / authentication — v3.0+; single-user design keeps v1 deployment trivially simple
- Full drag-and-drop design editor — 3–5 presets satisfy 80% of users at lower complexity cost
- TikTok posting — v1.x after Instagram is proven stable
- Tax calculation — out of scope; "tracking tool, not accounting software"
- Currency auto-conversion — group by currency is the correct v1 answer

### Architecture Approach

The architecture is a Go modular monolith with strict module isolation: each business domain (tracklist, social, epk, gig, finance, release, tour) has its own handler/service/repository/model package and communicates with other modules only via exported Go interfaces — never via cross-module DB joins. Nuxt 4 is the browser entry point and proxies all `/api/v1/*` requests to the Go API via Nitro routeRules. Binary assets (cover art, generated images, PDFs, DJ files) live in MinIO; metadata lives in PostgreSQL. Social post scheduling is an in-process goroutine polling the `scheduled_posts` table every 60 seconds — no Redis, no external queue.

**Major components:**
1. Go API (modular monolith, chi router) — all business logic, REST endpoints, background scheduler, embedded migrations
2. Nuxt 4 frontend (Nitro proxy) — UI, SSR, single entry point for all browser traffic
3. PostgreSQL 16 — relational data, FK integrity, job queue state, soft-delete pattern
4. MinIO — all binary assets; write MinIO first, then PostgreSQL (never reverse)
5. In-process scheduler goroutine — DB-backed, `FOR UPDATE SKIP LOCKED`, 60s tick, startup catch-up scan

**Key cross-module dependency rule:** `gig` is the hub module. `finance`, `tour`, and `epk` depend on `gig.GigReader` (read-only interface). No module queries another module's tables directly. Build order is not optional.

### Critical Pitfalls

1. **Instagram App Review delay (Phase 1b)** — Submit the Facebook App for `instagram_content_publish` permission at the start of Phase 1b, not the end. Review takes 1–4 weeks and requires a demo video and privacy policy. Without approval, the scheduler only works with test accounts.

2. **Instagram long-lived token silent expiry (Phase 1b)** — Tokens expire after 60 days. Implement proactive token refresh on every scheduler tick, not only during publishing. Store `token_expiry` and check on API startup; surface UI warning at 15 days remaining. Map `OAuthException error code 190` to a human-readable reconnect prompt.

3. **Font rendering failures for non-Latin track names (Phase 1a)** — Bundle Noto Sans family via Go `embed`. Implement a font fallback chain (primary font → Noto Latin → Noto CJK → Noto Arabic → replacement character). Add a Unicode test fixture with CJK, Arabic, and Cyrillic tracks to the image generator test suite before Phase 1a ships.

4. **Cover art fetch race condition / thundering herd (Phase 1a)** — Implement `golang.org/x/sync/singleflight` keyed on normalized `artist:title` in the cover art fetcher from day one. Without it, concurrent goroutines for repeated tracks hit the Spotify API redundantly and trigger 429 rate limits on large sets.

5. **DB-backed scheduler missed-tick catch-up (Phase 1b)** — On API startup, scan for posts with `status = scheduled` and `scheduled_at < NOW()`: publish within 10-minute window; mark older posts as `missed` and surface a notification. Without this, posts scheduled during an API restart are silently lost or published very late.

6. **Parser silent regression on DJ software updates (Phase 1a)** — Parse CSV formats by column header name, not column index. Maintain test fixture files for at least two versions of each DJ software format. Add format-confidence detection that prompts the user when auto-detection is uncertain.

7. **MinIO orphaned objects (Phase 1a)** — Implement orphan reconciliation (`GET /api/v1/system/reconcile`) at the same time as the first MinIO write. Run on API startup. Defer cleanup is a disk-exhaustion risk for power users who regenerate images frequently.

## Implications for Roadmap

Based on combined research, the architecture document's build order is correct and the phase structure should follow it closely. The primary constraint is the `gig` module hub dependency: finance, tour, and EPK (live data) cannot be meaningfully built before gig. Instagram App Review is the single largest external timeline risk and must be de-risked by parallel submission during Phase 1a.

### Phase 0: Infrastructure Scaffolding
**Rationale:** All modules depend on platform/ packages (db, storage, http, config, log, migrations). Nothing else can start without a working four-service Docker Compose stack, Go module, and passing health endpoint.
**Delivers:** Running `docker compose up`, Go API returning `GET /api/v1/health`, embedded goose migrations running at startup, MinIO bucket created, Nuxt proxy routing to Go API
**Addresses:** NFR-700 (single compose command), NFR-703 (embedded migrations), deployment architecture from ARCHITECTURE.md
**Avoids:** The "migrations run manually" anti-pattern (tech debt table in PITFALLS.md), MinIO presigned URL internal hostname pitfall (establish `MINIO_PUBLIC_ENDPOINT` env var from day one)
**Research flag:** Standard patterns — skip research-phase

### Phase 1a: Tracklist Image Generator
**Rationale:** This is the flagship feature that defines product identity and attracts early adopters. It is independent of all other modules (no gig/finance dependencies). It also establishes the cover art infrastructure and MinIO write patterns used by all later phases. The three most critical Phase 1a pitfalls (Unicode fonts, singleflight cover art, parser resilience) must all be solved here.
**Delivers:** DJ file upload (Rekordbox + Serato + Traktor parsing), cover art fetch with fallback chain, 3–5 design presets, Story and Square image generation, live preview at 50% resolution, PNG/JPEG export, logo/watermark overlay, track range selector, Unicode rendering, MinIO cover art cache, orphan reconciler
**Addresses:** Module 1a table stakes (FEATURES.md), cover art mosaic differentiator, Unicode rendering (launch-blocking), persist user settings
**Avoids:** Font rendering failure (Pitfall 3), cover art singleflight race (Pitfall 4), parser silent regression (Pitfall 7), MinIO orphan accumulation (Pitfall 6), synchronous cover art in upload handler (Architecture anti-pattern 3)
**Research flag:** Needs research-phase — cover art API integration (Spotify Client Credentials flow, Discogs User-Agent requirement, MusicBrainz 1 req/sec limit) has enough gotchas to warrant a dedicated phase research pass before implementation

### Phase 1b: Social Media Scheduler
**Rationale:** Depends on Phase 1a (scheduler attaches generated images). This is the highest-risk phase due to the Instagram App Review external dependency. App Review submission must begin during Phase 1a — the review process is parallel, not sequential.
**Delivers:** Instagram OAuth flow (token storage with AES-256-GCM encryption), post scheduling (timezone-aware), scheduler goroutine with 60s tick, retry with exponential backoff (3x), `permanently_failed` state + Download Image fallback, startup catch-up scan, token expiry proactive refresh, post queue/calendar view, post-export CTA from Phase 1a
**Addresses:** Module 1b table stakes (FEATURES.md), post-export CTA differentiator, retry + manual fallback differentiator
**Avoids:** Instagram App Review blocking launch (Pitfall 1 — submit during 1a), token silent expiry (Pitfall 2), scheduler missed-tick catch-up (Pitfall 5), unencrypted token storage (security table), Instagram media container processing delay (integration gotchas)
**Research flag:** Needs research-phase — Instagram Graph API publishing flow (container creation → FINISHED poll → publish), Story vs Feed container type distinction, token refresh endpoint, App Review requirements

### Phase 1c: EPK / Press Kit Builder
**Rationale:** Mostly independent of gig at this stage (the gig-highlights import is additive and can ship in Phase 1d after gig is built). EPK is the next-highest user value after social posting, and its PDF generation infrastructure (go-pdf/fpdf PDFGenerator interface) will be reused by Phase 1e's invoice generator. Building EPK before gig avoids a hard dependency chain blocker.
**Delivers:** EPK bio (rich text via Tiptap, sanitized via bluemonday), press photo upload (up to 20 × 10MB), tech rider + stage plot, section include/exclude, PDF export via go-pdf/fpdf
**Addresses:** Module 1c table stakes (FEATURES.md), PDF email-ability as table stakes
**Avoids:** Using archived jung-kurt/gofpdf (STACK.md warning — use go-pdf/fpdf v2.7+), PDF generation blocking HTTP handler (run in goroutine, return 202 + job ID, per performance traps), OOM on 20-photo EPK (performance traps)
**Research flag:** Standard patterns for EPK CRUD; go-pdf/fpdf usage is well-documented

### Phase 1d: Gig Tracker
**Rationale:** Gig is the hub module that finance, tour, and EPK live-data all depend on. It must ship as a standalone phase before those three. It is also independently valuable — DJs who have used Phase 1a–1c will immediately want to log the gigs those tracklists came from.
**Delivers:** Gig CRUD with status workflow (inquiry → confirmed → played → cancelled), payment status (independent enum), calendar and list views, filter by date/status/fee, gig-linked tracklist association, `GigReader` interface exported for downstream modules, soft delete with restore
**Addresses:** Module 1d table stakes (FEATURES.md), gig-linked tracklists differentiator
**Avoids:** Cross-module DB queries (Architecture rule — downstream modules use GigReader interface, never join gig tables directly), skipping soft delete (Architecture anti-pattern 4)
**Research flag:** Standard patterns — skip research-phase

### Phase 1e: Finance Tracker + Invoice Generation
**Rationale:** Depends on Phase 1d (gig.GigReader for auto-income on payment status transition). PDF invoice generation reuses the PDFGenerator interface established in Phase 1c.
**Delivers:** Income and expense CRUD with categories, monthly/yearly summary views, multi-currency display (group by currency — no conversion), PDF invoice generation with auto-population from gig data, auto income entry on gig `payment_status → paid` transition, invoice number sequencing
**Addresses:** Module 1e table stakes (FEATURES.md), gig→invoice workflow differentiator, auto income entry differentiator
**Avoids:** Currency auto-conversion anti-feature (FEATURES.md anti-features), tax calculation scope creep (FEATURES.md anti-features), `updated_at` optimistic concurrency omission (now mandatory per tech debt table since multiple state-heavy modules exist)
**Research flag:** Standard patterns — skip research-phase

### Phase 1f: Release Planner
**Rationale:** Fully independent of all other modules. Low complexity (CRUD + enum + deadlines + promo checklist). Can be parallelized with Phase 1e if team capacity allows.
**Delivers:** Release CRUD with status workflow, deadline tracking per release (mastering/artwork/release date), promo checklist with default template + per-release customization
**Addresses:** Module 1f table stakes (FEATURES.md)
**Avoids:** No notable pitfalls specific to this module
**Research flag:** Standard patterns — skip research-phase

### Phase 1g: Tour Manager
**Rationale:** Depends on Phase 1d (gig.GigReader for tour stop–gig linkage). Lower priority than finance (financial compliance is more pressing than tour budgeting). Shares the gig hub dependency pattern with Phase 1e.
**Delivers:** Tour CRUD with named grouping of gigs, per-stop logistics (JSONB flexible fields: travel, accommodation, notes), tour budget aggregate across stops (grouped by currency)
**Addresses:** Module 1g table stakes (FEATURES.md), tour budget differentiator
**Avoids:** Cross-module DB queries (use gig.GigReader interface, not direct table joins)
**Research flag:** Standard patterns — skip research-phase

### Phase 1h: Unified Dashboard
**Rationale:** Depends on all modules completing (aggregates upcoming gigs, scheduled posts, recent tracklists, finance summary). Must be last in the sequence. Dashboard service calls other module services via interfaces only — no direct DB queries.
**Delivers:** Dashboard aggregating upcoming gigs, queued social posts, recent tracklists, finance summary, release deadlines; query-level timeout (500ms) with partial data fallback
**Addresses:** Cross-cutting unified dashboard table stakes (FEATURES.md)
**Avoids:** Dashboard aggregation query timeout (performance traps — use 500ms statement_timeout context)
**Research flag:** Standard patterns — skip research-phase

### Phase Ordering Rationale

- Phase 0 → 1a → 1b is a hard technical chain: platform before modules, tracklist before social (scheduler uses generated images)
- Phase 1c (EPK) can run before 1d (Gig) because EPK PDF infrastructure benefits Phase 1e and EPK is independently valuable
- Phase 1d must precede 1e (finance), 1g (tour), and the EPK gig-import feature — this is the hard dependency constraint identified in FEATURES.md and ARCHITECTURE.md
- Phase 1e and 1f can be parallel if team capacity allows — no shared dependency
- Phase 1h is always last (aggregates all modules)
- Instagram App Review submission is not a phase but a parallel action: begin during Phase 1a, target approval by Phase 1b start

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Primary source is the approved ARCHITECTURE.md v1.0 blueprint; package choices verified against repo package.json; library warnings (gofpdf archive, gorilla/mux archive) are well-known stable facts |
| Features | MEDIUM | Based on BRD/Addendum documents (high confidence for scope) plus training-data competitive analysis (medium confidence for competitor feature details) |
| Architecture | HIGH | Directly sourced from the approved architecture blueprint with explicit design decisions and rationale; confirmed against repo structure |
| Pitfalls | MEDIUM-HIGH | Based on well-documented platform behaviors (Instagram API, Spotify API, MusicBrainz, Go gg library); external web verification unavailable but findings assessed as stable |

**Overall confidence:** MEDIUM-HIGH

### Gaps to Address

- **Instagram Graph API current rate limits and token lifetime:** The 60-day token lifetime and 25 posts/24 hours rate limit are based on training data (August 2025 cutoff). Verify against current Meta documentation before Phase 1b implementation — Meta changes API terms periodically.
- **go-pdf/fpdf v2 API surface:** The library is the correct choice (confirmed). Verify actual API ergonomics against the specific EPK layout requirements (multi-column, embedded photos) before committing to the implementation approach in Phase 1c.
- **gg (fogleman/gg) font fallback chain implementation:** The font fallback approach (iterate font slice, use first font with glyph) is the correct pattern but `gg` does not natively support multi-font fallback. The implementation will require a custom glyph-lookup wrapper. This complexity should be scoped in Phase 1a planning.
- **Rekordbox v7.x export format:** Parser test fixtures should be sourced from actual Rekordbox 6.x and 7.x exports before Phase 1a ships. The exact column structure for each version needs confirmation against real files.
- **MusicBrainz cover art endpoint specifics:** MusicBrainz's Cover Art Archive API path and search query format should be verified before Phase 1a cover art implementation.

## Sources

### Primary (HIGH confidence)
- `/docs/ARCHITECTURE.md` v1.0 (2026-03-11) — approved architecture blueprint; primary source for stack decisions, module structure, build order, patterns
- `/docs/KlubHub_DJ_NFR_v1.0.md` — non-functional requirements that constrain all stack and feature decisions
- `/docs/KlubHub_DJ_BRD_v2.0_Platform.md` — feature specification for all 7 modules
- `/docs/KlubHub_DJ_BRD_v2.0_Addendum.md` — clarified requirements, edge cases, data model amendments
- `package.json` — confirmed current frontend dependency versions (nuxt ^4.0.0, h3 ^1.8.2, vue ^3.5.13, typescript ~5.9.2, Nx 22.5.4)

### Secondary (MEDIUM confidence)
- Training-data knowledge of Go ecosystem library status (gofpdf archived, gorilla/mux archived, lib/pq maintenance mode, goose vs golang-migrate comparison) — verified against known Go community consensus
- Training-data knowledge of Instagram Graph API behavior (token lifecycle, App Review process, container publishing flow) — assessed MEDIUM confidence; verify against current Meta docs before Phase 1b
- Training-data knowledge of Spotify, Discogs, MusicBrainz API behaviors — assessed MEDIUM confidence for rate limits and authentication patterns

### Tertiary (LOW confidence)
- Competitive landscape analysis (Buffer, Later, Canva, EPK.io, Sonicbids) — training data; product features may have changed since August 2025 cutoff

---
*Research completed: 2026-03-14*
*Ready for roadmap: yes*
