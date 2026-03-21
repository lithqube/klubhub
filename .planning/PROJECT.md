# KlubHub DJ

## What This Is

KlubHub DJ is an open-source, self-hosted career toolkit for DJs and electronic music artists. It ships as a single `docker-compose up` command and provides a unified dashboard for tracklist image generation, social media scheduling, press kit building, gig tracking, finance tracking, release planning, and tour management — all in one place, without relying on fragmented SaaS tools.

The platform is built for the working DJ who owns their data and wants to spend less time on admin. No authentication (single-user, local-first), no cloud lock-in.

## Core Value

A DJ can upload their DJ history file and get a professional, social-media-ready tracklist image in under 60 seconds — no design tools, no manual typing, no external services required.

## Requirements

### Validated

- ✓ Nuxt 4 frontend scaffold with Nx monorepo structure — existing
- ✓ pnpm workspace with TypeScript strict mode — existing
- ✓ Playwright e2e test setup — existing

### Active

#### Phase 1a — Tracklist Image Generator (Flagship)
- [ ] User can upload a DJ history file (drag-and-drop or file picker)
- [ ] System auto-detects and parses Rekordbox, Serato, and Traktor formats without user input
- [ ] User can review parsed tracklist with all metadata; missing fields shown with per-track warnings
- [ ] User can inline-edit any track metadata field after parsing
- [ ] System fetches cover art via Spotify → Discogs → MusicBrainz fallback chain with caching
- [ ] User can override cover art per-track with a manual upload
- [ ] User can generate tracklist images at 1080×1920 (Story) and 1080×1080 (Square)
- [ ] User can choose background: custom upload, solid color, or cover art mosaic
- [ ] User can select from 3–5 design presets with color customization
- [ ] User can toggle display of each metadata field (artist, title, BPM, key, label, time played, etc.)
- [ ] User can upload a logo/watermark with corner position control
- [ ] User sees a live preview before full-resolution export
- [ ] User can export as PNG or JPEG
- [ ] System persists user's last-used settings

#### Phase 1b — Social Media Scheduler
- [ ] User can connect an Instagram account via OAuth (Facebook Business)
- [ ] User can schedule posts to Instagram Feed and Stories with timezone support
- [ ] User can attach any generated tracklist image or custom upload to a scheduled post
- [ ] User sees a queue/calendar view of all scheduled and published posts
- [ ] System auto-publishes posts at the scheduled time with retry on failure (3× exponential backoff)
- [ ] User sees post status: scheduled, publishing, published, failed, permanently_failed
- [ ] System auto-refreshes OAuth tokens before expiry

#### Phase 1c — EPK / Press Kit Builder
- [ ] User can write artist bio (short + long) with rich text editor
- [ ] User can upload and manage press photos (up to 20, max 10 MB each)
- [ ] User can enter tech rider, stage plot, social links, gig highlights, press quotes, contact info
- [ ] User can export a professionally formatted PDF press kit
- [ ] User can select which sections to include in the PDF

#### Phase 1d — Gig Tracker
- [ ] User can create, edit, and delete gig entries with full metadata
- [ ] User can move gigs through status workflow: inquiry → confirmed → advanced → played → cancelled
- [ ] User can track payment status independently: unpaid → deposit_paid → paid → overdue → waived
- [ ] User can link a tracklist to a gig
- [ ] User can view gigs in calendar and list views with filtering

#### Phase 1e — Finance Tracker
- [ ] User can log income and expense entries with category, currency, and date
- [ ] System auto-creates income entry when a gig payment_status transitions to paid
- [ ] User sees monthly/yearly summaries grouped by currency (no auto-conversion)
- [ ] User can generate a PDF invoice from gig data

#### Phase 1f — Release Planner
- [ ] User can create and manage releases with status workflow and deadline tracking
- [ ] User can manage a promo checklist per release with a customizable default template

#### Phase 1g — Tour Manager
- [ ] User can create a tour as a named group of linked gigs
- [ ] User can track travel, accommodation, and checklists per tour stop
- [ ] User sees aggregate tour budget (fees vs. expenses) grouped by currency

#### Cross-Cutting
- [ ] Unified dashboard showing upcoming gigs, scheduled posts, recent tracklists, pending deadlines
- [ ] Docker Compose deployment: single `docker-compose up -d` command
- [ ] Health endpoint (`GET /api/v1/health`) reporting per-integration status
- [ ] Automatic database migrations on API startup (golang-migrate or goose)
- [ ] `scripts/backup.sh` and `scripts/restore.sh` for data portability

### Out of Scope

| Feature | Reason |
|---------|--------|
| Authentication / multi-user | Single-user v1; multi-user planned for v3.0+ |
| TikTok posting | v1.x follow-up; Instagram first |
| Tax calculation / VAT | Tracking tool only, not accounting software |
| AI-generated backgrounds | v2.0 roadmap item |
| External integrations (RA, Beatport, Bandcamp) | v2.0 roadmap item (Bandsintown outbound sync ships as optional Phase 4.8 in v1) |
| Mobile app | Web-first; mobile later |
| Real-time chat | Not core to DJ career management |
| Contract management / e-signatures | v3.0+ roadmap item (booking confirmation PDF without e-signature ships in Phase 4 as GIG-12) |
| Text-only tracklist export (1001tracklists format) | v1.x enhancement |

## Context

**Current state:** The repo is a Nuxt 4 Nx monorepo scaffold (`apps/dj/`) with placeholder pages and one demo API endpoint. No Go backend, no PostgreSQL, no MinIO yet.

**Architecture decided:** Go modular monolith (backend) + Nuxt 4 (frontend) + PostgreSQL 16 + MinIO + Docker Compose. Backend uses clean module boundaries per feature (each module: routes, handlers, service, repository). No auth layer in v1.

**Key design patterns:**
- Parser interface: all DJ format parsers (Rekordbox, Serato, Traktor) implement a shared Go interface
- Integration adapters: interfaces defined from day one for future external integrations (RA, Beatport, etc.)
- Write order: MinIO first, then PostgreSQL for generated assets
- Soft deletion everywhere with `deleted_at` timestamp
- Optimistic concurrency control via `updated_at` on all PUT endpoints

**Nuxt version note:** BRD specifies Nuxt 3 but scaffold uses Nuxt 4. Nuxt 4 is backward-compatible; this is not a blocker. The project proceeds on Nuxt 4.

## Constraints

- **Tech stack**: Go + Nuxt 4 + PostgreSQL + MinIO — decided in architecture doc, not up for revision in v1
- **Deployment**: Docker Compose only; no Kubernetes, no managed cloud services
- **Auth**: None in v1 — API keys in env vars, no session management
- **Security**: All services bind to `127.0.0.1` by default; social tokens encrypted with AES-256-GCM
- **Image generation**: Pure Go (gg + imaging libraries) — no headless Chrome in v1
- **PDF generation**: Pure Go (gofpdf) — no headless Chrome in v1

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Nuxt 4 instead of Nuxt 3 | Scaffold already uses Nuxt 4; backward compatible, no migration needed | — Pending |
| Pure Go image generation (gg + imaging) | Smaller Docker image, simpler deployment; rich templates deferred | — Pending |
| No job queue (DB-backed polling) | No Redis/external queue; simpler ops for self-hosters | — Pending |
| Phased delivery (1a first) | Tracklist generator is the flagship draw; validates product before full build | — Pending |
| MIT license | Maximum adoption for open-source tool | — Pending |
| Tailwind CSS + shadcn-vue (not Nuxt UI) | shadcn-vue gives full component ownership (copy-paste, not dependency); user prefers shadcn ecosystem; Tailwind maps design tokens directly | — Decided 2026-03-20 |
| Cyberpunk HUD design language | "The Kinetic HUD" — dark-first, glassmorphism, 0px radii, luminous halos, no traditional borders; see `.planning/phases/1.5-design-system-foundation/1.5-DESIGN-SYSTEM-SPEC.md` | — Decided 2026-03-20 |
| Pinia for state management | 30+ local refs unsustainable; cross-module data sharing needed for scheduler/dashboard | — Decided 2026-03-20 |
| Dual color token architecture | UI theme (Tailwind config) separate from image preset colors (design-tokens.ts) — image presets are product data, not UI chrome | — Decided 2026-03-20 |
| Space Grotesk + Inter + Manrope typography | Three-font system: Command (headlines), Data (body), Terminal (labels); bundled locally as variable woff2 | — Decided 2026-03-20 |
| v1 open-source + v2 SaaS release strategy | v1.0.0 ships all modules as MIT open-source with Phase 9 production hardening; v2 SaaS in separate private repo (`klubhub-dj-cloud`) imports v1 as Go module and adds auth, billing, service extraction, K8s | — Decided 2026-03-20 |
| Open-core monetization model (revised 2026-03-21) | **OSS forever:** tracklist, gig, finance, release, tour manager, unified dashboard, Instagram scheduling, contacts/venue DB, rider templates, iCal feed, text tracklist export, career analytics. **SaaS PRO ($9/mo):** multi-platform social posting (TikTok/Twitter/Facebook), AI caption generation, hosted EPK page with shareable URL. **SaaS TEAM ($25/mo):** multi-user roster, collaborative tour planning, custom EPK domain. Golden rule: "Own your data free, rent infrastructure paid." | — Revised 2026-03-21 |
| Separate repository for SaaS (not fork) | OSS repo stays clean; SaaS repo imports OSS as Go module; feature flags in frontend (`multiPlatformSocial`, `hostedEPK`, `aiCaptions`, `collaboration`); contributors never see billing/auth code | — Decided 2026-03-20 |
| Instagram scheduling = OSS (revised 2026-03-21) | Instagram scheduling completes the core tracklist→post workflow and must be free. SaaS PRO adds multi-platform (TikTok, Twitter/X, Facebook) + AI captions — these require centralized API key management and rate limit orchestration across tenants that self-hosters cannot replicate easily | — Revised 2026-03-21 |
| Tour Manager = OSS (revised 2026-03-21) | Basic tour planning (named gig groups, per-stop logistics, budget aggregate) belongs to the user's data and ships as OSS. Collaborative tour sharing (invite manager/agent, real-time shared checklists) is SaaS TEAM in v2 | — Revised 2026-03-21 |
| EPK hosted pages = SaaS v2 M4, not v3 (revised 2026-03-21) | PDF-only confirmed for OSS v1 — PDF generation requires no server infrastructure. Hosted public EPK page (`klubhub.dj/epk/name`, custom domain, CDN, SSL) is SaaS PRO in v2 M4. Reclassified from v3 to v2 SaaS launch feature | — Revised 2026-03-21 |
| Gig Tracker scope extended (2026-03-21) | Phase 4 now includes: reusable venue and contact database (CONT), iCal feed export (INT-02, INT-03), and booking confirmation PDF (GIG-12). These are table-stakes features competitors ship in their core offering; without them the gig tracker cannot replace a working DJ's existing tools | — Decided 2026-03-21 |

---
*Last updated: 2026-03-20 after v1/v2 release planning*
