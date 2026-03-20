# Roadmap: KlubHub DJ

## Overview

KlubHub DJ ships in eleven phases (including one inserted phase), driven by a hard module-dependency graph. Phase 0 builds the four-service Docker Compose stack that everything else runs on. Phase 1 (tracklist image generator) is the flagship and the product's identity. Phases 2-8 add the social scheduler, press kit builder, gig tracker hub, finance tracker, release planner, tour manager, and unified dashboard — in an order dictated by which modules depend on which. The gig tracker (Phase 4) is the dependency hub: finance, tour, and EPK's live-data import cannot ship without it. The unified dashboard (Phase 8) is always last, aggregating all prior modules.

## Phases

**Phase Numbering:**

- Integer phases (0, 1, 2 ...): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 0: Infrastructure** - Four-service Docker Compose stack, Go API scaffold, health endpoint, embedded migrations, backup/restore scripts
- [ ] **Phase 1: Tracklist Image Generator** - DJ file upload and parsing, cover art fetch chain, image generation with presets, live preview, PNG/JPEG export
- [ ] **Phase 1.5: Design System Foundation** - INSERTED — Tailwind CSS + shadcn-vue + Cyberpunk HUD design language, Pinia state management, component decomposition, responsive design
- [ ] **Phase 2: Social Media Scheduler** - Instagram OAuth, timezone-aware post scheduling, retry with backoff, queue/calendar view
- [ ] **Phase 3: EPK / Press Kit Builder** - Artist bio, press photos, tech rider, PDF export with section control, export history
- [ ] **Phase 4: Gig Tracker** - Gig CRUD with status/payment workflows, calendar/list views, GigReader interface for downstream modules
- [ ] **Phase 5: Finance Tracker** - Income/expense logging, multi-currency summaries, auto-income on gig payment, PDF invoice generation
- [ ] **Phase 6: Release Planner** - Release CRUD with status workflow, deadline tracking, promo checklist with customizable default template
- [ ] **Phase 7: Tour Manager** - Named tour groups of linked gigs, per-stop logistics, tour budget aggregate
- [ ] **Phase 8: Unified Dashboard** - Aggregated view of all modules: upcoming gigs, scheduled posts, recent tracklists, finance summary, release deadlines
- [ ] **Phase 9: Production Hardening** - Documentation, security audit, CI/CD, testing, release engineering for v1.0.0 open-source release

## Phase Details

### Phase 0: Infrastructure

**Goal**: A working four-service Docker Compose stack that all future modules build on
**Depends on**: Nothing (first phase)
**Requirements**: INFRA-01, INFRA-02, INFRA-03, INFRA-04, INFRA-05, INFRA-06, INFRA-07, INFRA-08, INFRA-09, INFRA-10, INFRA-11
**Success Criteria** (what must be TRUE):

1. Running `docker compose up -d` starts all four services (frontend, api, db, storage) with no manual steps; browser can reach the Nuxt frontend
2. `GET /api/v1/health` returns a JSON response listing per-integration status for database, storage, Spotify, Discogs, MusicBrainz, and Instagram
3. API starts cleanly when optional API keys (Spotify, Discogs, Instagram) are absent; only PostgreSQL and MinIO are required to pass health checks
4. Database migrations run automatically at API startup with no manual intervention; migration state is visible in health response
5. `scripts/backup.sh` and `scripts/restore.sh` execute without error and produce/restore a valid data snapshot
   **Plans**: 4 plans
   Plans:

- [ ] 00-01-PLAN.md — Go module scaffold: platform packages (config, db, storage, log, crypto, embedded migrations)
- [ ] 00-02-PLAN.md — Docker Compose stack: four services, .env.example, Nuxt API proxy
- [ ] 00-03-PLAN.md — Health endpoint + user_settings CRUD with optimistic concurrency
- [ ] 00-04-PLAN.md — Backup/restore scripts + end-to-end smoke checkpoint

### Phase 1: Tracklist Image Generator

**Goal**: A DJ can upload their history file and get a professional, social-media-ready tracklist image without any design tool or external service
**Depends on**: Phase 0
**Requirements**: TRKL-01, TRKL-02, TRKL-03, TRKL-04, TRKL-05, TRKL-06, TRKL-07, TRKL-08, TRKL-09, TRKL-10, TRKL-11, TRKL-12, TRKL-13, TRKL-14, TRKL-15, TRKL-16, TRKL-17, TRKL-18, TRKL-19, TRKL-20, TRKL-21, TRKL-22, TRKL-23, TRKL-24, TRKL-25, TRKL-26, TRKL-27, TRKL-28
**Success Criteria** (what must be TRUE):

1. User can drag-and-drop or file-pick a Rekordbox, Serato, or Traktor history file and see a parsed tracklist with all available metadata within seconds; files over 50 MB or unrecognized formats show a clear error
2. User can inline-edit any track field after parsing; cover art loads asynchronously per track via Spotify → Discogs → MusicBrainz fallback; tracks with no match show a placeholder tile with a "Retry Cover Art" button if all APIs are unreachable
3. User can choose from 3-5 design presets with color overrides, toggle metadata field visibility, set background mode (solid / custom upload / cover art mosaic), and upload a corner-positioned logo/watermark
4. User sees a live preview at 50% resolution before export; exporting produces a full-resolution PNG or JPEG at both 1080×1920 (Story) and 1080×1080 (Square) via a single "Export Both" action
5. User's last-used template, background mode, and field visibility settings are remembered across sessions; track names with CJK, Cyrillic, and Arabic characters render correctly in all generated images
   **Plans**: 5 plans
   Plans:

- [ ] 01-01-PLAN.md — Go backend: migration 002, Rekordbox parser, tracklist CRUD API (upload, list, get, update-track, delete)
- [ ] 01-02-PLAN.md — Nuxt frontend: tracklist page (upload + review table + inline edit + card preview + past list)
- [ ] 01-03-PLAN.md — Cover art fetcher: Spotify → Discogs → MusicBrainz chain, MinIO caching, goroutine worker pool
- [ ] 01-04-PLAN.md — Screenshot pipeline: Nitro Playwright plugin + screenshot endpoint + Go generate-image + Dockerfile update
- [ ] 01-05-PLAN.md — End-to-end smoke + human checkpoint

### Phase 1.5: Design System Foundation (INSERTED)

**Goal**: Establish a cohesive design system, component library, and state management layer so all future phases build on a consistent, accessible, responsive UI foundation
**Depends on**: Phase 1
**Requirements**: DSYS-01, DSYS-02, DSYS-03, DSYS-04, DSYS-05, DSYS-06, DSYS-07, DSYS-08, DSYS-09, DSYS-10, DSYS-11, DSYS-12, DSYS-13
**Success Criteria** (what must be TRUE):

1. Tailwind CSS is installed and configured with the full Cyberpunk HUD design token system (surface hierarchy, primary/secondary/tertiary colors, glow shadows, 0px border-radius, glassmorphism utilities); all existing pages render correctly
2. shadcn-vue components (Button, Input, Select, Dialog, Table, Tabs, Toast, Dropdown, Card, Badge, Popover, Sheet) are installed and restyled to match the Cyberpunk Editorial spec (gradient CTAs, glass panels, accent bar focus, no rounded corners, luminous hover states)
3. Pinia stores exist for settings, tracklist, and UI state; tracklist.vue page state is migrated from 30+ local refs to centralized stores
4. TrackcardPreview.vue is decomposed into 6 focused sub-components with shared design tokens extracted to a utility file; image generation presets remain separate from UI theme tokens
5. tracklist.vue is decomposed into 5 feature components (upload zone, editor, customizer, exporter, history) wired via Pinia stores
6. Responsive layout works across mobile (<640px), tablet (640-1024px), and desktop (>1024px) with 48dp touch targets on mobile and progressive disclosure for technical metadata
7. Error boundary and toast notification system are in place; API errors display cyberpunk-styled toast notifications instead of raw error text
   **Plans**: 7 plans
   Plans:

- [x] 1.5-01-PLAN.md — Tailwind v4 install + cyberpunk @theme token system + variable fonts
- [ ] 1.5-02-PLAN.md — shadcn-vue install + cyberpunk component overrides (14 components)
- [x] 1.5-03-PLAN.md — Pinia stores (settings, tracklist, ui) + design-tokens.ts (TDD)
- [ ] 1.5-04-PLAN.md — TrackcardPreview.vue decomposition → 6 trackcard/ sub-components
- [ ] 1.5-05-PLAN.md — tracklist.vue decomposition → 5 tracklist/ feature components
- [ ] 1.5-06-PLAN.md — app.vue cyberpunk shell + error boundary + index.vue redirect
- [ ] 1.5-07-PLAN.md — Human verification checkpoint

### Phase 2: Social Media Scheduler

**Goal**: Users can schedule generated tracklist images to Instagram Feed and Stories and trust the system to publish them reliably
**Depends on**: Phase 1
**Requirements**: SOCL-01, SOCL-02, SOCL-03, SOCL-04, SOCL-05, SOCL-06, SOCL-07, SOCL-08, SOCL-09, SOCL-10, SOCL-11, SOCL-12, SOCL-13, SOCL-14, SOCL-15, SOCL-16, SOCL-17
**Success Criteria** (what must be TRUE):

1. User can connect an Instagram account via OAuth and see its connection status; the system proactively refreshes tokens and surfaces a re-authorization warning when the account is at risk of disconnecting
2. User can schedule a post (Feed or Story) with a generated or custom image, caption, and timezone; the system publishes at the scheduled time automatically with no user action required
3. Failed posts retry up to 3 times with exponential backoff; permanently failed posts appear in the queue with the error reason and offer a "Download Image" fallback so the user can post manually
4. User can view all posts (scheduled, publishing, published, failed, permanently_failed) in a queue/calendar view; scheduled posts can be fully edited before publishing begins
5. After exporting a tracklist image, a "Schedule Post" call-to-action pre-fills the new post form with that image attached
   **Plans**: TBD

### Phase 3: EPK / Press Kit Builder

**Goal**: Users can assemble a professional electronic press kit and export it as a shareable PDF in a single session
**Depends on**: Phase 0
**Requirements**: EPK-01, EPK-02, EPK-03, EPK-04, EPK-05, EPK-06, EPK-07, EPK-08, EPK-09, EPK-10
**Success Criteria** (what must be TRUE):

1. User can write short and long artist bios with a rich text editor (bold, italic, lists, links, H2/H3), upload up to 20 press photos, and enter tech rider, stage plot, social links, gig highlights, press quotes, and contact info
2. User can generate a professionally formatted PDF with their logo and colors applied; user controls which sections appear in the export via include/exclude toggles
3. Each PDF export is saved to MinIO and listed with timestamps; user can download or delete any previous export from the history view
4. When the Gig Tracker (Phase 4) is available, an "Import from Gigs" button populates the gig highlights section from selected gig entries
   **Plans**: TBD

### Phase 4: Gig Tracker

**Goal**: Users can manage their complete gig history and pipeline, and downstream modules can read gig data via a stable interface
**Depends on**: Phase 0
**Requirements**: GIG-01, GIG-02, GIG-03, GIG-04, GIG-05, GIG-06, GIG-07, GIG-08, GIG-09
**Success Criteria** (what must be TRUE):

1. User can create, edit, and delete gig entries with all fields (date, venue, city, country, event name, promoter contact, fee, currency, notes); "Copy from Previous Gig" auto-fills promoter fields from a prior gig
2. User can move a gig through the status workflow (inquiry → confirmed → advanced → played → cancelled) and track payment status independently (unpaid → deposit_paid → paid → overdue → waived)
3. User can view gigs in calendar and list views, filter by date range, venue, city, status, and fee range, and link a parsed tracklist to any gig
4. The GigReader interface is exported and consumable by Finance, Tour, and EPK modules without any direct cross-module database joins
   **Plans**: TBD

### Phase 5: Finance Tracker

**Goal**: Users can track all DJ income and expenses, see multi-currency summaries, and generate PDF invoices from gig data
**Depends on**: Phase 4
**Requirements**: FIN-01, FIN-02, FIN-03, FIN-04, FIN-05, FIN-06, FIN-07, FIN-08, FIN-09, FIN-10
**Success Criteria** (what must be TRUE):

1. User can log income entries (with optional gig link) and expense entries with category, currency, date, and description; all amounts are stored and displayed in their original currency with no automatic conversion
2. When a gig's payment status transitions to `paid`, the system automatically creates an income entry; if the gig fee later changes, the user is prompted to update the linked entry; if the gig reverts from `paid`, the user is prompted to delete, void, or keep the entry
3. User sees monthly and yearly summary views grouped by currency with profit/loss per gig, per month, and per year
4. User can generate a PDF invoice pre-populated from gig data with an auto-incrementing invoice number; the Finance UI clearly states that tax/VAT calculation is not performed
   **Plans**: TBD

### Phase 6: Release Planner

**Goal**: Users can plan and track music releases with status workflows, deadlines, and customizable promo checklists
**Depends on**: Phase 0
**Requirements**: REL-01, REL-02, REL-03, REL-04, REL-05, REL-06, REL-07
**Success Criteria** (what must be TRUE):

1. User can create, edit, and delete releases with all fields (title, artists, label, distributor, release date, format, notes) and move a release through its status workflow (demo_sent → accepted → mastering → artwork → promo → released)
2. User can track multiple named deadlines per release with due dates and completion status, and enter manual platform links (Beatport, Bandcamp, Spotify, SoundCloud) per release
3. User can customize the promo checklist per release and save their customized template as the new default for future releases; a timeline/calendar view shows upcoming releases and their deadlines
   **Plans**: TBD

### Phase 7: Tour Manager

**Goal**: Users can plan tours as named groups of linked gigs, track per-stop logistics, and see aggregate tour budgets
**Depends on**: Phase 4
**Requirements**: TOUR-01, TOUR-02, TOUR-03, TOUR-04, TOUR-05, TOUR-06, TOUR-07
**Success Criteria** (what must be TRUE):

1. User can create a named tour and add gig entries as stops; a "Create Gig + Add to Tour" shortcut creates a new gig and links it to the tour in one action
2. User can record travel and accommodation details per stop, manage a customizable checklist per stop, and upload tour documents (contracts, itineraries, boarding passes) to MinIO
3. User sees a timeline/calendar view of tour dates and an aggregate budget showing total fees vs. total expenses grouped by currency across all stops
   **Plans**: TBD

### Phase 8: Unified Dashboard

**Goal**: Users see a single-screen overview of their entire career status across all modules
**Depends on**: Phase 1, Phase 2, Phase 3, Phase 4, Phase 5, Phase 6, Phase 7
**Requirements**: DASH-01, DASH-02, DASH-03, DASH-04, DASH-05, DASH-06, DASH-07, DASH-08
**Success Criteria** (what must be TRUE):

1. Dashboard shows upcoming gigs (next 30 days) with status indicators, scheduled social posts (next 7 days), recently generated tracklist images, pending release deadlines, active tour status, and monthly income/expense summary — all on one screen
2. Quick action buttons (upload tracklist, schedule post, log gig, create invoice) are visible and functional from the dashboard without navigating away first
3. Widgets for modules with no data or not yet deployed are hidden rather than showing empty states; each module query has a 500ms timeout and the dashboard renders with partial data if any module is slow
   **Plans**: TBD

### Phase 9: Production Hardening

**Goal**: Ship v1.0.0 as a polished, documented, production-ready open-source release suitable for public adoption
**Depends on**: All prior phases (0-8)
**Requirements**: PROD-01, PROD-02, PROD-03, PROD-04, PROD-05, PROD-06, PROD-07, PROD-08, PROD-09, PROD-10, PROD-11, PROD-12, PROD-13, PROD-14
**Success Criteria** (what must be TRUE):

1. README.md contains a hero screenshot, 3-step quick start, feature overview, and tech stack badges; SELF-HOSTING.md covers system requirements, env vars, reverse proxy setup, and backup schedule; CONTRIBUTING.md covers dev setup, code style, and PR process
2. All API endpoints pass input validation audit (no SQL injection, path traversal, or unbounded queries); CORS defaults to same-origin; rate limiting is configured; `govulncheck` and `pnpm audit` report no high/critical CVEs
3. GitHub Actions CI pipeline runs lint, test, build, and E2E on every push; Go test coverage is ≥70% on service/repository layers; Docker multi-arch build (amd64 + arm64) succeeds
4. Go API drains in-flight requests on SIGTERM; all 4 Docker services have HEALTHCHECK directives; structured JSON logging with configurable level is enabled
5. Git tag v1.0.0 is created; Docker images are published to ghcr.io; GitHub Release includes release notes, SELF-HOSTING link, and upgrade instructions
   **Plans**: TBD

---

**v2 SaaS Milestone Boundary**

After v1.0.0 release, SaaS migration work begins in a separate repository (`klubhub-dj-cloud`). The v2 milestones (M1-M9) are documented in `docs/v2-saas-migration-plan.md` and `.planning/research/V2-SAAS-ARCHITECTURE.md`. The open-source repo continues to receive updates independently.

## Progress

**Execution Order:**
Phases execute in numeric order: 0 → 1 → 1.5 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9

Note: Phase 1.5 (Design System) is an inserted phase that must complete before Phase 2, as the scheduler UI depends on the component library and design tokens. Phase 3 (EPK) and Phase 4 (Gig) have no dependency on each other and can be parallelized. Phase 6 (Release) is independent of Phase 5 (Finance) and can be parallelized with it. Phase 2 (Social) depends on Phase 1.5. Phases 5 and 7 depend on Phase 4. Phase 8 depends on all others. Phase 9 (Production Hardening) is the final phase before v1.0.0 release — it covers documentation, security, CI/CD, testing, and release engineering.

| Phase                            | Plans Complete | Status      | Completed  |
| -------------------------------- | -------------- | ----------- | ---------- |
| 0. Infrastructure                | 4/4            | Complete    | 2026-03-14 |
| 1. Tracklist Image Generator     | 5/5            | Complete    | 2026-03-16 |
| 1.5 Design System Foundation     | 0/TBD          | Not started | -          |
| 2. Social Media Scheduler        | 0/TBD          | Not started | -          |
| 3. EPK / Press Kit Builder       | 0/TBD          | Not started | -          |
| 4. Gig Tracker                   | 0/TBD          | Not started | -          |
| 5. Finance Tracker               | 0/TBD          | Not started | -          |
| 6. Release Planner               | 0/TBD          | Not started | -          |
| 7. Tour Manager                  | 0/TBD          | Not started | -          |
| 8. Unified Dashboard             | 0/TBD          | Not started | -          |
| 9. Production Hardening          | 0/TBD          | Not started | -          |
