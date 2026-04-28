# Requirements: KlubHub DJ

**Defined:** 2026-03-14
**Core Value:** A DJ uploads their history file and gets a professional, social-media-ready tracklist image in under 60 seconds — no design tools, no manual typing.

---

## v1 Requirements

Requirements for full platform delivery across all 9 areas (infra + 7 modules + production hardening). Each maps to a roadmap phase.

### Infrastructure / Platform (INFRA)

- [x] **INFRA-01**: System deploys as four Docker Compose services (frontend, api, db, storage) with a single `docker-compose up -d` command
- [x] **INFRA-02**: All services bind to `127.0.0.1` by default; configurable via `BIND_ADDRESS` env var
- [ ] **INFRA-03**: Go API runs database migrations automatically on startup using embedded goose migrations
- [x] **INFRA-04**: `GET /api/v1/health` returns per-integration status: database, storage, Spotify, Discogs, MusicBrainz, Instagram
- [x] **INFRA-05**: API starts and degrades gracefully when optional API keys (Spotify, Discogs, Instagram) are missing; only PostgreSQL and MinIO are hard dependencies
- [x] **INFRA-06**: Nuxt 4 frontend proxies all `/api/v1/*` requests to the Go backend (no direct browser-to-Go-port communication)
- [x] **INFRA-07**: `user_settings` singleton table stores DJ name, logo path, default colors, default template, visible fields, social links, bio, contact info
- [x] **INFRA-08**: Project includes `scripts/backup.sh` (pg_dump + mc mirror) and `scripts/restore.sh` for manual data portability
- [x] **INFRA-09**: Social media OAuth tokens stored in DB encrypted with AES-256-GCM using `TOKEN_ENCRYPTION_KEY` env var
- [x] **INFRA-10**: All entities use UUID primary keys, `created_at`/`updated_at` timestamps, and soft deletion via `deleted_at`
- [x] **INFRA-11**: All entity update (PUT) endpoints implement optimistic concurrency control: client sends `updated_at`, server returns HTTP 409 on conflict

### Tracklist Image Generator (TRKL)

- [x] **TRKL-01**: User can upload a DJ history file via drag-and-drop or file picker
- [x] **TRKL-02**: System auto-detects and parses Rekordbox (.txt, .csv, .xml), Serato (history format), and Traktor (.nml) without user input
- [x] **TRKL-03**: System extracts artist, title, BPM, key, label, duration, and time played from all supported formats
- [x] **TRKL-04**: System validates uploaded files and returns clear error messages for unsupported or malformed files
- [x] **TRKL-05**: System rejects files exceeding configurable max upload size (default 50 MB) with a clear error message
- [x] **TRKL-06**: System returns partial parse results with per-track warnings for malformed/missing metadata; tracks are never silently discarded
- [x] **TRKL-07**: If a valid file has zero parseable tracks, system returns a specific error message
- [x] **TRKL-08**: User can inline-edit any parsed track metadata field (artist, title, BPM, key, label, duration); edits persist to PostgreSQL; source file in MinIO is never modified
- [x] **TRKL-09**: System stores parsed tracklists in PostgreSQL and raw source files in MinIO
- [x] **TRKL-10**: System searches Spotify, then Discogs, then MusicBrainz for cover art using artist + title (fallback chain)
- [x] **TRKL-11**: System caches fetched cover art in MinIO; all fetched artwork is cached to avoid redundant API calls
- [x] **TRKL-12**: Cover art fetching is asynchronous with progressive UI updates
- [x] **TRKL-13**: Placeholder cover art (dark tile with music note icon, 500×500 PNG) is used for unmatched tracks; user can upload a custom global placeholder in settings
- [x] **TRKL-14**: User can upload a replacement cover art image per-track; manual override persists and is not replaced by API re-fetch unless explicitly reset
- [x] **TRKL-15**: Tracklist configuration UI shows all cover art thumbnails; user can accept, reject, or replace each
- [x] **TRKL-16**: If all cover art APIs are unreachable, all tracks receive placeholder art and UI shows a warning with a "Retry Cover Art" button
- [x] **TRKL-17**: System generates tracklist images at 1080×1920 (Story) and 1080×1080 (Square)
- [x] **TRKL-18**: User can choose background mode: custom upload, solid color (with color picker), or cover art mosaic
- [x] **TRKL-19**: User can select from 3–5 built-in design presets with primary/accent color and font color overrides
- [x] **TRKL-20**: User can toggle display of each metadata field: artist, title, BPM, key, label, time played, set duration, DJ name, event name, date
- [x] **TRKL-21**: User can upload a logo/watermark (PNG with transparency) and choose corner position
- [x] **TRKL-22**: Live preview renders at 50% resolution in the browser; exported image is full resolution; UI indicates the difference
- [x] **TRKL-23**: User can export generated image as PNG or JPEG
- [x] **TRKL-24**: "Export Both" button triggers Story and Square generation in a single action
- [x] **TRKL-25**: Each template defines a `max_tracks_displayed` limit (default 30); UI shows track range selector for tracklists that exceed the limit
- [x] **TRKL-26**: Image templates support full Unicode rendering including CJK, Cyrillic, and Arabic via bundled Noto Sans fonts; unrenderable glyphs show `?` substitution
- [x] **TRKL-27**: User can delete a tracklist; cascade soft-deletes child tracks and generated images; linked gig references are set to NULL; confirmation dialog warns user
- [x] **TRKL-28**: System persists user's last-used template, background mode, and metadata field settings
- [ ] **TRKL-29**: User can export a parsed tracklist as plain text in three formats: (a) 1001Tracklists format `HH:MM - Artist - Title`, (b) Mixcloud/SoundCloud description format `Artist - Title` (one per line), (c) numbered list `1. Artist - Title (Label)` with label omitted if missing
- [ ] **TRKL-30**: Text export respects the active track range selector (same range as image export); empty metadata fields are omitted gracefully from output

### Design System Foundation (DSYS) — INSERTED

- [x] **DSYS-01**: Tailwind CSS v4 is installed via `@nuxtjs/tailwindcss` with full Cyberpunk HUD design token system mapped in `tailwind.config.ts` (surface hierarchy, primary/secondary/tertiary/error colors, glow shadows, glassmorphism utilities, 0px border-radius enforced globally)
- [ ] **DSYS-02**: shadcn-vue component library is installed with components restyled to Cyberpunk Editorial spec: gradient CTAs on primary buttons, glass panels on dialogs/cards, accent bar focus on inputs, no rounded corners, luminous hover states, alternating surface colors instead of row dividers in tables
- [x] **DSYS-03**: Pinia state management is installed with stores for user settings (`stores/settings.ts`), tracklist CRUD state (`stores/tracklist.ts`), and UI state (`stores/ui.ts`); all 30+ local refs in tracklist.vue are migrated to centralized stores
- [x] **DSYS-04**: Image generation preset colors are extracted to `app/utils/design-tokens.ts` as shared constants; TrackcardPreview.vue imports from this file instead of inline hardcoded objects; image presets remain separate from UI theme tokens
- [x] **DSYS-05**: TrackcardPreview.vue (465 lines) is decomposed into 6 focused sub-components inside `app/components/trackcard/`: TrackCard (container), TrackCardHeader, TrackCardTrackList, TrackCardTrackRow, TrackCardLogo, TrackCardFooter; TrackcardPreview.vue becomes a thin orchestrator
- [x] **DSYS-06**: tracklist.vue page (~50KB) is decomposed into 5 feature components inside `app/components/tracklist/`: TracklistUploadZone, TracklistEditor, TracklistCustomizer, TracklistExporter, TracklistHistory; page becomes a layout wiring components via Pinia stores
- [x] **DSYS-07**: Typography system uses three font families bundled locally in `public/fonts/` as variable woff2 files: Space Grotesk (display/headlines), Inter (body), Manrope (labels/metadata); Google Fonts CDN import is removed from TrackcardPreview; Noto Sans retained for image generation CJK support
- [x] **DSYS-08**: Responsive layout uses Tailwind breakpoints: mobile (<640px) single column with 48dp touch targets, tablet (640-1024px) two-column where possible, desktop (>1024px) full side-by-side editor + preview; TrackcardPreview scales dynamically per viewport
- [x] **DSYS-09**: Error boundary (`<NuxtErrorBoundary>`) wraps all pages in app.vue with a cyberpunk-styled fallback UI; Toast notification system uses shadcn-vue Toast with cyberpunk overrides (glass-panel, glow-error for destructive variant)
- [x] **DSYS-10**: NxWelcome.vue (881-line placeholder) is deleted; `pages/index.vue` redirects to `/tracklist` or displays a cyberpunk-styled landing page
- [ ] **DSYS-11**: Component naming convention is established: `ui/` for shadcn-vue base components, `trackcard/` for image generation components, `tracklist/` for tracklist feature components, `The{Name}.vue` for app-level singletons
- [x] **DSYS-12**: All UI surfaces follow the "No-Line" rule: no traditional 1px solid borders for sectioning; boundaries defined by tonal transitions between surface levels or luminous thresholds (outline at 20% opacity paired with primary glow)
- [ ] **DSYS-13**: All floating panels use Glassmorphism: surface_variant at 60% opacity with backdrop-blur (20-40px); primary CTAs use 135-degree gradient from primary (#96F8FF) to primary_container (#00F1FD)

### Social Media Scheduler (SOCL)

- [x] **SOCL-01**: User can connect an Instagram account via OAuth (Facebook Business account flow)
- [x] **SOCL-02**: System automatically refreshes Instagram OAuth tokens before expiry; marks account as `disconnected` on refresh failure and surfaces a re-authorization warning
- [x] **SOCL-03**: User can schedule a post for a specific date and time with timezone selection
- [x] **SOCL-04**: System publishes scheduled posts automatically at the specified time
- [x] **SOCL-05**: User can post to Instagram Feed (single image + caption with hashtags)
- [x] **SOCL-06**: User can post to Instagram Stories (single image)
- [x] **SOCL-07**: User can attach any generated tracklist image or a custom uploaded image to a post
- [x] **SOCL-08**: Custom-uploaded images are validated for format (JPEG/PNG), size (<8 MB), dimensions, and aspect ratio before scheduling
- [x] **SOCL-09**: User sees a queue/calendar view of all scheduled, publishing, published, failed, and permanently failed posts
- [x] **SOCL-10**: Post status tracks: draft, scheduled, publishing, published, failed, permanently_failed
- [x] **SOCL-11**: Failed posts retry up to 3 times with exponential backoff; after exhaustion, post is marked `permanently_failed`
- [x] **SOCL-12**: Permanently failed posts surface on dashboard and queue with error reason; user can retry once manually or download the image for manual posting
- [x] **SOCL-13**: Scheduler respects Instagram rate limits: minimum 30-second interval between posts; backs off on 429 responses per response headers (default 15 minutes)
- [x] **SOCL-14**: Scheduled posts can be fully edited (caption, image, time, timezone) while in `scheduled` status; edits are blocked once `publishing` begins
- [x] **SOCL-15**: Disconnecting a social account moves all its `scheduled` posts to `draft` status; user is warned before disconnect
- [x] **SOCL-16**: Before retrying a failed post, scheduler checks whether the post was already published on Instagram (duplicate prevention)
- [x] **SOCL-17**: After image export in Tracklist Generator, UI shows a "Schedule Post" CTA that pre-fills the new post form with the generated image attached

### EPK / Press Kit Builder (EPK)

- [x] **EPK-01**: User can write artist bio (short + long versions) with rich text editor supporting bold, italic, lists, links, and H2/H3 headings
- [x] **EPK-02**: User can upload and manage up to 20 press photos (JPEG/PNG, max 10 MB each)
- [x] **EPK-03**: User can upload tech rider content and stage plot image
- [x] **EPK-04**: User can enter social/streaming profile links
- [x] **EPK-05**: User can manually enter gig history highlights, press quotes, and contact information
- [x] **EPK-06**: User can generate a professionally formatted PDF press kit applying their logo and color settings
- [x] **EPK-07**: User can select which sections to include or exclude in the PDF export
- [x] **EPK-08**: Each PDF export is stored in MinIO and listed in an export history view with timestamps; user can download or delete any previous export
- [x] **EPK-09**: All EPK data stored in PostgreSQL; photo and document assets stored in MinIO
- [x] **EPK-10**: When Gig Tracker (Phase 4) is available, an "Import from Gigs" button populates gig highlights from selected gig entries

### Gig Tracker (GIG)

- [x] **GIG-01**: User can create, edit, and delete gig entries with: date, venue, city, country, event name, promoter name/email/phone, fee, currency, notes
- [x] **GIG-02**: Gig status workflow: `inquiry` → `confirmed` → `advanced` → `played` → `cancelled`; forward skipping and backward transitions allowed; `cancelled` is terminal
- [x] **GIG-03**: Payment status tracked independently: `unpaid`, `deposit_paid`, `paid`, `overdue`, `waived`
- [x] **GIG-04**: Currency stored as ISO 4217 code; no automatic conversion; searchable currency dropdown
- [ ] **GIG-05**: User can link a parsed tracklist to a gig
- [ ] **GIG-06**: User can view gigs in calendar view and list view
- [x] **GIG-07**: User can filter gigs by date range, venue, city, status, and fee range
- [x] **GIG-08**: "Copy from Previous Gig" dropdown auto-fills promoter fields from a selected prior gig
- [x] **GIG-09**: All gig data stored in PostgreSQL with soft deletion
- [x] **GIG-10**: User can link a gig to a venue record (CONT) and a promoter contact record (CONT); linked records auto-populate venue name, city, country, and promoter fields; fields remain editable after link
- [x] **GIG-11**: "Copy from Previous Gig" populates from the gig's linked venue and contact records (not just free-text fields) when those records exist
- [ ] **GIG-12**: When a gig is in `confirmed` status, user can generate a booking confirmation PDF pre-populated with: DJ name, venue name + city, event date, fee + currency, set length, technical contact name + email; no e-signature required; PDF stored in MinIO and downloadable

### Finance Tracker (FIN)

- [ ] **FIN-01**: User can log income entries with amount, currency, category, date, description, and optional gig link
- [ ] **FIN-02**: User can log expense entries with amount, currency, category, date, and notes
- [ ] **FIN-03**: When a gig's `payment_status` transitions to `paid`, system auto-creates an income entry flagged as `auto_generated`
- [ ] **FIN-04**: If gig fee changes after auto-creation, system prompts user to update the linked income entry (no silent modification)
- [ ] **FIN-05**: If gig reverts from `paid`, system prompts user to delete, void, or keep the linked income entry
- [ ] **FIN-06**: Monthly and yearly summary views show totals grouped by currency; no cross-currency aggregation
- [ ] **FIN-07**: User can calculate profit/loss per gig, per month, and per year
- [ ] **FIN-08**: User can generate a PDF invoice from gig data with auto-incrementing invoice number (`{PREFIX}-{YYYY}-{NNN}`); prefix configurable in settings
- [ ] **FIN-09**: Multi-currency summary display groups totals by currency code with no automatic conversion
- [ ] **FIN-10**: Tax/VAT calculation is explicitly not implemented; Finance module UI documents this limitation

### Release Planner (REL)

- [ ] **REL-01**: User can create, edit, and delete release entries with: title, artists (comma-separated), label, distributor, release date, format (digital/vinyl/both), notes
- [ ] **REL-02**: Release status workflow: `demo_sent` → `accepted` → `mastering` → `artwork` → `promo` → `released`
- [ ] **REL-03**: User can track multiple named deadlines per release with due date and completion status
- [ ] **REL-04**: User can enter platform links per release (Beatport, Bandcamp, Spotify, SoundCloud — manual entry)
- [ ] **REL-05**: Each release has a promo checklist; system ships with a default 6-item template; user can customize per release
- [ ] **REL-06**: User can save a customized checklist as the new default template for future releases
- [ ] **REL-07**: User sees a timeline/calendar view of upcoming releases and their deadlines

### Tour Manager (TOUR)

- [ ] **TOUR-01**: User can create a named tour as a collection of linked gig entries
- [ ] **TOUR-02**: Tour stops require a linked gig (`gig_id` is required); UI provides "Create Gig + Add to Tour" shortcut
- [ ] **TOUR-03**: User can track travel details and accommodation per tour stop
- [ ] **TOUR-04**: User can manage a customizable checklist per tour stop
- [ ] **TOUR-05**: Tour budget shows aggregate income (fees) vs. expenses across all stops, grouped by currency
- [ ] **TOUR-06**: User sees a timeline/calendar view of tour dates
- [ ] **TOUR-07**: User can upload and store documents per tour (contracts, itineraries, boarding passes) in MinIO

### Unified Dashboard (DASH)

- [ ] **DASH-01**: Dashboard shows upcoming gigs (next 30 days) with status indicators
- [ ] **DASH-02**: Dashboard shows scheduled social media posts (next 7 days)
- [ ] **DASH-03**: Dashboard shows recently generated tracklist images
- [ ] **DASH-04**: Dashboard shows pending release deadlines
- [ ] **DASH-05**: Dashboard shows active tour status (if any)
- [ ] **DASH-06**: Dashboard shows monthly income/expense summary
- [ ] **DASH-07**: Dashboard provides quick actions: upload tracklist, schedule post, log gig, create invoice
- [ ] **DASH-08**: Dashboard widgets are hidden when their module has no data or has not been deployed
- [ ] **DASH-09**: Dashboard shows a career analytics section: gigs per month (last 12 months), top 5 venues by gig count, average fee trend (last 3 years), and peak booking month; all computed from existing PostgreSQL gig data with no external API calls
- [ ] **DASH-10**: Analytics section is hidden if fewer than 3 gig entries exist; each analytic widget degrades independently if data is insufficient

### Contacts & Venue Database (CONT)

- [x] **CONT-01**: User can create, edit, and delete venue records with: name, city, country, capacity (optional), website (optional), technical contact name, technical contact email, technical contact phone, notes
- [x] **CONT-02**: User can create, edit, and delete contact records with: name, company (optional), email, phone, type (enum: promoter, agent, label, other), notes
- [x] **CONT-03**: When creating or editing a gig, user can search and link a venue record and a contact record by name (autocomplete search, minimum 2 characters)
- [x] **CONT-04**: Linking a venue to a gig auto-populates venue name, city, and country fields; linking a contact auto-populates promoter name, email, and phone; all auto-populated fields remain editable after link
- [x] **CONT-05**: Venue list view shows each venue with total gig count and date of last booking; contact list view shows each contact with total gig count
- [x] **CONT-06**: All venue and contact data stored in PostgreSQL with soft deletion; a venue or contact with linked non-deleted gigs cannot be hard-deleted — soft delete only

### Rider Templates (RIDER)

- [ ] **RIDER-01**: User can create, edit, and delete named rider templates with four sections: technical requirements, hospitality, backline, other notes (each section is free-text)
- [ ] **RIDER-02**: When a gig transitions to `advanced` status, user can select a rider template to attach to the gig; attachment creates a per-gig copy, not a live link to the template
- [ ] **RIDER-03**: User can override any section of the per-gig rider copy without modifying the source template
- [ ] **RIDER-04**: User can export the per-gig rider as a PDF using the existing PDFGenerator interface; PDF includes DJ name, venue, date, and all four rider sections
- [ ] **RIDER-05**: All rider template and per-gig rider data stored in PostgreSQL with soft deletion

### Integrations (INT)

- [ ] **INT-01**: When a gig transitions to `confirmed` status and `BANDSINTOWN_API_KEY` is set in `.env`, system pushes the event to the user's Bandsintown artist page via the Bandsintown API; success and failure are logged; if the API key is absent or the push fails after 3 retries, the gig is still saved without error surfaced to the user
- [ ] **INT-02**: `GET /api/v1/gigs/calendar.ics` returns an iCal feed (RFC 5545) of all non-cancelled gigs; importable by Google Calendar, Apple Calendar, and any iCal-compatible client without authentication (protected by shared secret in URL query param configurable via `ICAL_SECRET` env var)
- [ ] **INT-03**: Each iCal event includes: UID (gig UUID), DTSTART (gig date + time if known), SUMMARY (event name or venue name), LOCATION (venue name, city, country), DESCRIPTION (fee + currency + promoter contact email — omitted if empty)

---

## v2 Requirements

Deferred to v1.x and v2.0. Tracked but not in current roadmap.

### Social Expansion
- **SOCL-v2-01**: TikTok posting support
- **SOCL-v2-02**: Twitter/X and Facebook cross-posting
- **SOCL-v2-03**: Social media content calendar view

### Tracklist Enhancements
- **TRKL-v2-01**: Text-only tracklist export (1001tracklists, Mixcloud, SoundCloud formats)
- **TRKL-v2-02**: Additional image output formats (Twitter/X banner, A4 poster, custom dimensions)
- **TRKL-v2-03**: AI-generated and procedural/gradient backgrounds

### Integrations
- **INT-v2-01**: Resident Advisor event data sync
- **INT-v2-02**: Beatport catalog integration
- **INT-v2-03**: Bandcamp integration
- **INT-v2-04**: Spotify for Artists integration
- **INT-v2-05**: Google Calendar / iCal sync

### Platform
- **PLAT-v2-01**: Data export (CSV/JSON) for all modules
- **PLAT-v2-02**: Contacts / network CRM module
- **PLAT-v2-03**: Analytics dashboard (gig stats, income trends)
- **PLAT-v2-04**: Promo / marketing asset generator

### v3
- Multi-user support with authentication
- Full design editor (custom fonts, drag-and-drop)
- Additional DJ software support (VirtualDJ, djay Pro, Engine DJ)
- Mobile-responsive or native mobile app
- Community template marketplace
- Hosted EPK web pages with shareable links
- Contract management and e-signatures

### Production Hardening (PROD)

- [ ] **PROD-01**: README.md includes hero screenshot, 3-step quick start (`docker compose up -d`), feature overview grid, tech stack badges, and links to docs
- [ ] **PROD-02**: `docs/SELF-HOSTING.md` covers system requirements (2GB RAM, 10GB disk), all env vars, HTTPS reverse proxy setup (Caddy and nginx examples), and backup schedule recommendation
- [ ] **PROD-03**: `docs/CONFIGURATION.md` documents every environment variable with type, default value, example, and which module uses it
- [ ] **PROD-04**: `docs/CONTRIBUTING.md` covers dev environment setup, code style (Go + Vue), PR process, module architecture overview, and testing guide
- [ ] **PROD-05**: `CHANGELOG.md` contains v1.0.0 release notes covering all modules shipped
- [ ] **PROD-06**: MIT `LICENSE` file is present at repository root
- [ ] **PROD-07**: All API endpoints pass input validation audit; no SQL injection, path traversal, or unbounded query vulnerabilities
- [ ] **PROD-08**: Rate limiting is configurable via `RATE_LIMIT_RPM` env var (default: 100 req/min per IP) and active on all endpoints
- [ ] **PROD-09**: CORS defaults to same-origin only; configurable via `CORS_ORIGINS` env var
- [ ] **PROD-10**: `govulncheck ./...` reports no known Go vulnerabilities; `pnpm audit` reports no high/critical npm CVEs
- [ ] **PROD-11**: GitHub Actions CI pipeline runs lint, unit tests, build, and E2E tests on every push to main and every PR
- [ ] **PROD-12**: Go unit test coverage is ≥70% on service and repository layers; Playwright E2E tests cover all critical user flows
- [ ] **PROD-13**: Go API implements graceful shutdown (SIGTERM drains in-flight requests within 30s); all Docker services have HEALTHCHECK directives
- [ ] **PROD-14**: Docker images are published to ghcr.io on git tag; multi-arch build supports amd64 and arm64; GitHub Release includes release notes and upgrade instructions

---

## Out of Scope

| Feature | Reason |
|---------|--------|
| Authentication / multi-user | Single-user v1; multi-user planned for v3.0+ per BRD |
| Tax calculation / VAT | Tracking tool only, not accounting software (A10.2) |
| Automatic currency conversion | No auto-conversion anywhere; multi-currency grouped by code |
| AI-generated backgrounds | v2.0 roadmap item |
| Live currency exchange rates | Manual rates only; fully deferred to user |
| Real-time chat | Not core to DJ career management |
| Contract management / e-signatures | v3.0+ roadmap item (booking confirmation PDF without e-signature ships in Phase 4 as GIG-12) |
| TikTok posting | v1.x follow-up |
| OAuth for non-Instagram platforms | Only Instagram in v1 |

---

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement Group | Phase | Status |
|-------------------|-------|--------|
| INFRA-01 to INFRA-11 | Phase 0: Infrastructure | Pending |
| TRKL-01 to TRKL-30 | Phase 1: Tracklist Image Generator | Pending |
| SOCL-01 to SOCL-17 | Phase 2: Social Media Scheduler | Pending |
| EPK-01 to EPK-10 | Phase 3: EPK / Press Kit Builder | Pending |
| GIG-01 to GIG-12, CONT-01 to CONT-06, INT-02 to INT-03 | Phase 4: Gig Tracker | Pending |
| RIDER-01 to RIDER-05 | Phase 4.5: Rider Templates | Pending |
| INT-01 | Phase 4.8: Bandsintown Sync (optional) | Pending |
| FIN-01 to FIN-10 | Phase 5: Finance Tracker | Pending |
| REL-01 to REL-07 | Phase 6: Release Planner | Pending |
| TOUR-01 to TOUR-07 | Phase 7: Tour Manager | Pending |
| DASH-01 to DASH-10 | Phase 8: Unified Dashboard | Pending |
| PROD-01 to PROD-14 | Phase 9: Production Hardening | Pending |

**Coverage:**
- v1 requirements: 130 total (116 feature + 14 production hardening)
- Mapped to phases: 130
- Unmapped: 0

---
*Requirements defined: 2026-03-14*
*Last updated: 2026-03-14 — traceability updated after roadmap creation (phases 0-8)*
