# Roadmap: KlubHub DJ

## Overview

KlubHub DJ ships in eleven phases (including inserted phases), driven by a hard module-dependency graph. Phase 0 builds the four-service Docker Compose stack. Phase 1 (tracklist image generator) is the flagship and the product's identity. Phases 2-8 add the social scheduler, press kit builder, gig tracker hub, finance tracker, release planner, tour manager, and unified dashboard — in an order dictated by which modules depend on which. The gig tracker (Phase 4) is the dependency hub: finance, tour, and EPK's live-data import cannot ship without it. The unified dashboard (Phase 8) is always last, aggregating all prior modules.

## Phases

**Phase Numbering:**

- Integer phases (0, 1, 2 ...): Planned milestone work
- Decimal phases (0.5, 1.5, 1.5.5, 4.5 ...): Inserted phases (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 0: Infrastructure** - Four-service Docker Compose stack, Go API scaffold, health endpoint, embedded migrations, backup/restore scripts
- [x] **Phase 0.5: Garage S3 Storage** - INSERTED — Replace MinIO with Garage for S3-compatible object storage; configure Garage in docker-compose.yml with same env var interface (MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY); add region support (MINIO_REGION); verify all storage operations (cover art, tracklist images, EPK photos, PDFs) work against Garage S3 API (completed 2026-04-27)
- [x] **Phase 1: Tracklist Image Generator** - DJ file upload and parsing, cover art fetch chain, image generation with presets, live preview, PNG/JPEG export
- [x] **Phase 1.5: Design System Foundation** - INSERTED — Tailwind CSS + shadcn-vue + Cyberpunk HUD design language, Pinia state management, component decomposition, responsive design
- [x] **Phase 1.5.5: Kinetic HUD UI Migration** - INSERTED — Full pixel-perfect redesign to Kinetic HUD aesthetic; new Dashboard page; EPK split panel; Gigs/Finance pages (UI); 3-way theme switcher; mock data layer
- [x] **Phase 2: Social Media Scheduler** - Instagram OAuth, timezone-aware post scheduling, retry with backoff, queue/calendar view
- [x] **Phase 3: EPK / Press Kit Builder** - Artist bio, press photos, tech rider, PDF export with section control, export history
- [ ] **Phase 4: Gig Tracker** - Gig CRUD with status/payment workflows, venue & contact database, iCal feed, booking confirmation PDF, calendar/list views, GigReader interface for downstream modules
- [ ] **Phase 4.5: Rider Templates** - INSERTED — Reusable technical/hospitality rider templates, per-gig attachment with overrides, PDF export
- [ ] **Phase 4.8: Bandsintown Sync** - INSERTED (optional) — Outbound event push to Bandsintown on gig confirmed
- [ ] **Phase 5: Finance Tracker** - Income/expense logging, multi-currency summaries, auto-income on gig payment, PDF invoice generation
- [ ] **Phase 6: Release Planner** - Release CRUD with status workflow, deadline tracking, promo checklist with customizable default template
- [ ] **Phase 7: Tour Manager** - Named tour groups of linked gigs, per-stop logistics, tour budget aggregate (OSS; collaboration is SaaS v2)
- [ ] **Phase 8: Unified Dashboard** - Aggregated view of all modules: upcoming gigs, scheduled posts, recent tracklists, finance summary, release deadlines, career analytics
- [ ] **Phase 9: Production Hardening** - Documentation, security audit, CI/CD, testing, release engineering for v1.0.0 open-source release

## Phase Details

### Phase 0: Infrastructure ✅

**Goal**: A working four-service Docker Compose stack that all future modules build on
**Depends on**: Nothing (first phase)
**Requirements**: INFRA-01 through INFRA-11
**Status**: Complete — 2026-03-14
**Plans**:

- [x] 00-01-PLAN.md — Go module scaffold: platform packages (config, db, storage, log, crypto, embedded migrations)
- [x] 00-02-PLAN.md — Docker Compose stack: four services, .env.example, Nuxt API proxy
- [x] 00-03-PLAN.md — Health endpoint + user_settings CRUD with optimistic concurrency
- [x] 00-04-PLAN.md — Backup/restore scripts + end-to-end smoke checkpoint

### Phase 0.5: Garage S3 Storage 🔜 (INSERTED)

**Goal**: Replace MinIO with Garage as the S3-compatible object storage layer. Garage is an open-source, actively-maintained S3 server (https://Garage.deuxfleurs.fr/) that works in single-node and distributed modes. The S3 API is compatible with the existing minio-go client — no application code changes needed.
**Depends on**: Phase 0
**Requirements**: INFRA-01 (storage layer swap)
**Status**: In progress — planning complete
**Plans**:

- [ ] 00.5-01-PLAN.md — Docker Compose + env vars + storage config for Garage
- [ ] 00.5-02-PLAN.md — Backup/restore scripts + storage operations verification

**Key deliverables:**
- Garage `storage` service in `docker-compose.yml` replacing MinIO
- All `MINIO_*` env vars renamed to `S3_*` (backward alias preserved for existing deployments)
- `MINIO_REGION` support added (Garage requires explicit region)
- All storage operations tested: cover art upload/cache, tracklist image export, EPK photo upload, PDF export
- Backup/restore scripts updated for Garage (`mc` CLI replaced with Garage-specific commands or `aws s3`)

### Phase 1: Tracklist Image Generator ✅

**Goal**: A DJ can upload their history file and get a professional, social-media-ready tracklist image without any design tool or external service
**Depends on**: Phase 0
**Requirements**: TRKL-01 through TRKL-30
**Status**: Complete (5/6 plans; plan 01-06 text export pending as 1.x backlog item) — 2026-03-16
**Plans**:

- [x] 01-01-PLAN.md — Go backend: migration 002, Rekordbox parser, tracklist CRUD API
- [x] 01-02-PLAN.md — Nuxt frontend: tracklist page (upload + review table + inline edit + card preview + past list)
- [x] 01-03-PLAN.md — Cover art fetcher: Spotify → Discogs → MusicBrainz chain, MinIO caching, goroutine worker pool
- [x] 01-04-PLAN.md — Screenshot pipeline: Nitro Playwright plugin + screenshot endpoint + Go generate-image + Dockerfile update
- [x] 01-05-PLAN.md — End-to-end smoke + human checkpoint
- [ ] 01-06-PLAN.md — Text tracklist export: three format modes (1001Tracklists, Mixcloud/SoundCloud, numbered) — backlog

### Phase 1.5: Design System Foundation ✅ (INSERTED)

**Goal**: Establish a cohesive design system, component library, and state management layer so all future phases build on a consistent, accessible, responsive UI foundation
**Depends on**: Phase 1
**Requirements**: DSYS-01 through DSYS-13
**Status**: Complete — 2026-03-22
**Plans**:

- [x] 1.5-01-PLAN.md — Tailwind v4 install + cyberpunk @theme token system + variable fonts
- [x] 1.5-02-PLAN.md — shadcn-vue install + cyberpunk component overrides (14 components)
- [x] 1.5-03-PLAN.md — Pinia stores (settings, tracklist, ui) + design-tokens.ts (TDD)
- [x] 1.5-04-PLAN.md — TrackcardPreview.vue decomposition → 6 trackcard/ sub-components
- [x] 1.5-05-PLAN.md — tracklist.vue decomposition → 5 tracklist/ feature components
- [x] 1.5-06-PLAN.md — app.vue cyberpunk shell + error boundary + index.vue redirect
- [x] 1.5-07-PLAN.md — Human verification checkpoint

### Phase 1.5.5: Kinetic HUD UI Migration ✅ (INSERTED)

**Goal**: Migrate entire Nuxt frontend to the "Kinetic HUD" pixel-perfect design spec from the `ui_kits/dj-app/index.html` prototype; add mock data layer for frontend-only development
**Depends on**: Phase 1.5
**Status**: Complete — 2026-04-27

**Key deliverables**:
- Full rewrite of `styles.css` with 400+ lines of HUD component classes
- `useTheme` expanded to 3-way `dark | system | light` mode with OS preference listener
- All pages (Dashboard, Tracklist, Social, EPK, Gigs, Finance) migrated to HUD design
- New pages: `gigs.vue` and `finance.vue` with static mock UI
- `TheNav`, `TheMobileHeader`, `TheBottomNav` redesigned to HUD spec
- All tracklist and EPK components updated to use HUD classes
- Mock data server handlers at `apps/dj/server/api/v1/` for all API routes
- `nuxt.config.ts` proxy conditional on `NUXT_PUBLIC_API_BASE` — mock mode by default
- Playwright plugin graceful fallback when browsers not installed

### Phase 2: Social Media Scheduler ✅

**Goal**: Users can schedule generated tracklist images to Instagram Feed and Stories and trust the system to publish them reliably
**Depends on**: Phase 1
**Requirements**: SOCL-01 through SOCL-17
**Status**: Complete — 2026-03-22
**Plans**:

- [x] 02-01-PLAN.md — Go social package: migration 003, model, repository, service, handler
- [x] 02-02-PLAN.md — Go publishing worker + Instagram API client (two-step publish, retry backoff, rate limit, token refresh)
- [x] 02-03-PLAN.md — Pinia useSocialStore + useSocialPostForm composable + TypeScript types
- [x] 02-04-PLAN.md — social.vue page + SocialPostCompose + SocialQueueGrid + SocialPostCard + SocialPostCardFailed
- [x] 02-05-PLAN.md — SocialCalendarView + SocialConnectionBanner + SocialTokenWarningBanner + TheNav social entry
- [x] 02-06-PLAN.md — TracklistExporter "Schedule to Instagram" CTA integration (SOCL-17)
- [x] 02-07-PLAN.md — Human verification checkpoint

### Phase 3: EPK / Press Kit Builder ✅

> **Scope note:** EPK is PDF-only in v1 OSS. Hosted public EPK pages are a SaaS v2 feature.

**Goal**: Users can assemble a professional electronic press kit and export it as a shareable PDF in a single session
**Depends on**: Phase 0
**Requirements**: EPK-01 through EPK-10
**Status**: Complete — 2026-03-23
**Plans**:

- [x] 03-01-PLAN.md — Go backend data layer: migration 004 (epk_content + epk_exports), model, repository
- [x] 03-02-PLAN.md — Go EPK service (photo upload, PDF generation, Markdown renderer), handler, router wiring
- [x] 03-03-PLAN.md — Pinia useEpkStore + useEpkAutosave composable + TypeScript types
- [x] 03-04-PLAN.md — EpkBioSection + EpkPhotosSection + EpkTechRiderSection + EpkStagePlotSection
- [x] 03-05-PLAN.md — EpkGigHighlightsSection + EpkPressQuotesSection + EpkSocialLinksSection + EpkContactInfoSection + EpkExportPanel
- [x] 03-06-PLAN.md — epk.vue page orchestrator + EpkPageHeader + TheNav + TheBottomNav EPK entry
- [x] 03-07-PLAN.md — Human verification checkpoint

### Phase 4: Gig Tracker 🔜

**Goal**: Users can manage their complete gig history and pipeline, with a reusable venue/contact database, iCal export, and booking confirmation PDF; downstream modules can read gig data via a stable interface
**Depends on**: Phase 0
**Requirements**: GIG-01 through GIG-12, CONT-01 through CONT-06, INT-02, INT-03
**Status**: Not started — NEXT
**Plans**:

- [ ] 04-01-PLAN.md — Go backend: gig CRUD + status/payment workflow + GigReader interface
- [ ] 04-02-PLAN.md — Contacts & Venue database: venues + contacts tables, gig FK links, autocomplete search
- [ ] 04-03-PLAN.md — Booking confirmation PDF + iCal feed endpoint
- [ ] 04-04-PLAN.md — Nuxt frontend: gig calendar/list views + contact/venue autocomplete + iCal export button + confirmation PDF download

> Note: `gigs.vue` page already exists with HUD-styled static mock data from Phase 1.5.5. Phase 04-04 will wire it to the real API.

### Phase 4.5: Rider Templates (INSERTED)

**Goal**: DJs can create reusable technical and hospitality rider templates and attach them to gigs in the advancing stage
**Depends on**: Phase 4
**Requirements**: RIDER-01 through RIDER-05
**Status**: Not started
**Plans**: TBD

### Phase 4.8: Spotify Integration (INSERTED)

**Goal**: Import artist data from Spotify into EPK (profile images, albums, releases, top tracks) and use track audio features for tracklist image generation.

**Depends on**: Phase 4
**Requirements**: SPOT-01 through SPOT-04, EPK-12, TRKL-31
**Status**: Not started — DOCUMENTED FOR LATER
**Plans**: 04.8-01-PLAN.md

**Key deliverables:**
- Spotify Web API Go client with OAuth (client credentials flow)
- EPK import: artist images, genres, followers, external URLs, albums list, top tracks
- Tracklist audio features: tempo, energy, danceability, valence, key, loudness for image generation
- Image suggestion generation from audio features (tempo→color, energy→contrast, danceability→layout)
- Frontend: EpKSpotifyImportPanel component, TracklistAudioFeatures component
- Config: SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET env vars

**Notes:**
- Spotify Web API has no artist biography (only available via Spotify for Artists dashboard)
- Requires SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET env vars
- Rate limit: 180,000 requests per 24 hours (client credentials flow)
- Preview URLs are 30-second snippets, may not be available for all tracks

### Phase 4.9: RA (Resident Advisor) Integration (INSERTED)

**Goal**: Import artist data from RA into EPK (bio, social links) and import RA events as gig records.

**Depends on**: Phase 4
**Requirements**: RA-01 through RA-05, EPK-11, GIG-13
**Status**: Not started
**Plans**: 04.9-01-PLAN.md

**Key deliverables:**
- RA GraphQL Go client (no auth required, ra.co/graphql)
- RA cache to avoid re-fetching artist/events
- EPK import from RA: artist bio, social links (Instagram, SoundCloud, Bandcamp, etc.)
- Gig import from RA: create gig records from RA events with venue matching
- Frontend: EpkRaImportPanel component, RaEventImport component
- Use case: DJ enters RA artist slug → fetches profile → imports to EPK or creates gigs

**Notes:**
- RA has no official API — uses public GraphQL endpoint
- Existing scrapers (github.com/djb-gt/resident-advisor-events-scraper) demonstrate queries
- Our implementation replaces Bandsintown (Phase 4.8 in original roadmap) as the optional event sync feature
- RA is the primary platform for electronic music — more relevant than Bandsintown for DJs

### Phase 5: Finance Tracker

**Goal**: Users can track all DJ income and expenses, see multi-currency summaries, and generate PDF invoices from gig data
**Depends on**: Phase 4
**Requirements**: FIN-01 through FIN-10
**Status**: Not started
**Plans**: TBD

> Note: `finance.vue` page already exists with HUD-styled static mock data from Phase 1.5.5. Phase 5 plans will wire it to the real API.

### Phase 6: Release Planner

**Goal**: Users can plan and track music releases with status workflows, deadlines, and customizable promo checklists
**Depends on**: Phase 0
**Requirements**: REL-01 through REL-07
**Status**: Not started
**Plans**: TBD

### Phase 7: Tour Manager

> **Scope note:** Real-time collaboration is a SaaS TEAM feature in v2.

**Goal**: Users can plan tours as named groups of linked gigs, track per-stop logistics, and see aggregate tour budgets
**Depends on**: Phase 4
**Requirements**: TOUR-01 through TOUR-07
**Status**: Not started
**Plans**: TBD

### Phase 8: Unified Dashboard

**Goal**: Users see a single-screen overview of their entire career status across all modules
**Depends on**: Phase 1, 2, 3, 4, 5, 6, 7
**Requirements**: DASH-01 through DASH-10
**Status**: Not started
**Plans**: TBD

> Note: `index.vue` Dashboard page already exists with HUD-styled static mock data. Phase 8 will wire all widgets to live module APIs.

### Phase 9: Production Hardening

**Goal**: Ship v1.0.0 as a polished, documented, production-ready open-source release suitable for public adoption
**Depends on**: All prior phases (0-8)
**Requirements**: PROD-01 through PROD-14
**Status**: Not started
**Plans**: TBD

---

**v2 SaaS Milestone Boundary**

After v1.0.0 release, SaaS migration work begins in a separate repository (`klubhub-dj-cloud`). The v2 milestones (M1-M9) are documented in `docs/v2-saas-migration-plan.md` and `.planning/research/V2-SAAS-ARCHITECTURE.md`. The open-source repo continues to receive updates independently.

## Progress

**Execution Order:**
Phases execute in numeric order: 0 → 0.5 → 1 → 1.5 → 1.5.5 → 2 → 3 → 4 → 4.5 → 4.8 → 4.9 → 5 → 6 → 7 → 8 → 9

| Phase                            | Plans Complete | Status       | Completed  |
| -------------------------------- | -------------- | ------------ | ---------- |
| 0. Infrastructure                | 4/4            | ✅ Complete  | 2026-03-14 |
| 0.5. Garage S3 Storage         | 2/2 | Complete   | 2026-04-27 |
| 1. Tracklist Image Generator     | 5/6            | ✅ Complete* | 2026-03-16 |
| 1.5. Design System Foundation    | 7/7            | ✅ Complete  | 2026-03-22 |
| 1.5.5. Kinetic HUD Migration     | —              | ✅ Complete  | 2026-04-27 |
| 2. Social Media Scheduler        | 7/7            | ✅ Complete  | 2026-03-22 |
| 3. EPK / Press Kit Builder       | 7/7            | ✅ Complete  | 2026-03-23 |
| 4. Gig Tracker                   | 4/5            | In Progress|            |
|| 4.5. Rider Templates             | 0/TBD          | Future       | —          |
|| 4.8. Spotify Integration (INSERTED) | 0/TBD       | Future       | —          |
|| 4.9. RA Integration (INSERTED)    | 3/3            | ✅ Complete  | 2026-09-20 |
|| 5. Finance Tracker               | 0/TBD          | Future       | —          |
| 6. Release Planner               | 0/TBD          | Future       | —          |
| 7. Tour Manager                  | 0/TBD          | Future       | —          |
| 8. Unified Dashboard             | 0/TBD          | Future       | —          |
| 9. Production Hardening          | 0/TBD          | Future       | —          |

*Phase 1 plan 01-06 (text tracklist export) is a backlog item — not blocking Phase 4.
