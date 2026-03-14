# Requirements: KlubHub DJ

**Defined:** 2026-03-14
**Core Value:** A DJ uploads their history file and gets a professional, social-media-ready tracklist image in under 60 seconds — no design tools, no manual typing.

---

## v1 Requirements

Requirements for full platform delivery across all 8 areas (infra + 7 modules). Each maps to a roadmap phase.

### Infrastructure / Platform (INFRA)

- [ ] **INFRA-01**: System deploys as four Docker Compose services (frontend, api, db, storage) with a single `docker-compose up -d` command
- [ ] **INFRA-02**: All services bind to `127.0.0.1` by default; configurable via `BIND_ADDRESS` env var
- [ ] **INFRA-03**: Go API runs database migrations automatically on startup using embedded goose migrations
- [ ] **INFRA-04**: `GET /api/v1/health` returns per-integration status: database, storage, Spotify, Discogs, MusicBrainz, Instagram
- [ ] **INFRA-05**: API starts and degrades gracefully when optional API keys (Spotify, Discogs, Instagram) are missing; only PostgreSQL and MinIO are hard dependencies
- [ ] **INFRA-06**: Nuxt 4 frontend proxies all `/api/v1/*` requests to the Go backend (no direct browser-to-Go-port communication)
- [ ] **INFRA-07**: `user_settings` singleton table stores DJ name, logo path, default colors, default template, visible fields, social links, bio, contact info
- [ ] **INFRA-08**: Project includes `scripts/backup.sh` (pg_dump + mc mirror) and `scripts/restore.sh` for manual data portability
- [ ] **INFRA-09**: Social media OAuth tokens stored in DB encrypted with AES-256-GCM using `TOKEN_ENCRYPTION_KEY` env var
- [ ] **INFRA-10**: All entities use UUID primary keys, `created_at`/`updated_at` timestamps, and soft deletion via `deleted_at`
- [ ] **INFRA-11**: All entity update (PUT) endpoints implement optimistic concurrency control: client sends `updated_at`, server returns HTTP 409 on conflict

### Tracklist Image Generator (TRKL)

- [ ] **TRKL-01**: User can upload a DJ history file via drag-and-drop or file picker
- [ ] **TRKL-02**: System auto-detects and parses Rekordbox (.txt, .csv, .xml), Serato (history format), and Traktor (.nml) without user input
- [ ] **TRKL-03**: System extracts artist, title, BPM, key, label, duration, and time played from all supported formats
- [ ] **TRKL-04**: System validates uploaded files and returns clear error messages for unsupported or malformed files
- [ ] **TRKL-05**: System rejects files exceeding configurable max upload size (default 50 MB) with a clear error message
- [ ] **TRKL-06**: System returns partial parse results with per-track warnings for malformed/missing metadata; tracks are never silently discarded
- [ ] **TRKL-07**: If a valid file has zero parseable tracks, system returns a specific error message
- [ ] **TRKL-08**: User can inline-edit any parsed track metadata field (artist, title, BPM, key, label, duration); edits persist to PostgreSQL; source file in MinIO is never modified
- [ ] **TRKL-09**: System stores parsed tracklists in PostgreSQL and raw source files in MinIO
- [ ] **TRKL-10**: System searches Spotify, then Discogs, then MusicBrainz for cover art using artist + title (fallback chain)
- [ ] **TRKL-11**: System caches fetched cover art in MinIO; all fetched artwork is cached to avoid redundant API calls
- [ ] **TRKL-12**: Cover art fetching is asynchronous with progressive UI updates
- [ ] **TRKL-13**: Placeholder cover art (dark tile with music note icon, 500×500 PNG) is used for unmatched tracks; user can upload a custom global placeholder in settings
- [ ] **TRKL-14**: User can upload a replacement cover art image per-track; manual override persists and is not replaced by API re-fetch unless explicitly reset
- [ ] **TRKL-15**: Tracklist configuration UI shows all cover art thumbnails; user can accept, reject, or replace each
- [ ] **TRKL-16**: If all cover art APIs are unreachable, all tracks receive placeholder art and UI shows a warning with a "Retry Cover Art" button
- [ ] **TRKL-17**: System generates tracklist images at 1080×1920 (Story) and 1080×1080 (Square)
- [ ] **TRKL-18**: User can choose background mode: custom upload, solid color (with color picker), or cover art mosaic
- [ ] **TRKL-19**: User can select from 3–5 built-in design presets with primary/accent color and font color overrides
- [ ] **TRKL-20**: User can toggle display of each metadata field: artist, title, BPM, key, label, time played, set duration, DJ name, event name, date
- [ ] **TRKL-21**: User can upload a logo/watermark (PNG with transparency) and choose corner position
- [ ] **TRKL-22**: Live preview renders at 50% resolution in the browser; exported image is full resolution; UI indicates the difference
- [ ] **TRKL-23**: User can export generated image as PNG or JPEG
- [ ] **TRKL-24**: "Export Both" button triggers Story and Square generation in a single action
- [ ] **TRKL-25**: Each template defines a `max_tracks_displayed` limit (default 30); UI shows track range selector for tracklists that exceed the limit
- [ ] **TRKL-26**: Image templates support full Unicode rendering including CJK, Cyrillic, and Arabic via bundled Noto Sans fonts; unrenderable glyphs show `?` substitution
- [ ] **TRKL-27**: User can delete a tracklist; cascade soft-deletes child tracks and generated images; linked gig references are set to NULL; confirmation dialog warns user
- [ ] **TRKL-28**: System persists user's last-used template, background mode, and metadata field settings

### Social Media Scheduler (SOCL)

- [ ] **SOCL-01**: User can connect an Instagram account via OAuth (Facebook Business account flow)
- [ ] **SOCL-02**: System automatically refreshes Instagram OAuth tokens before expiry; marks account as `disconnected` on refresh failure and surfaces a re-authorization warning
- [ ] **SOCL-03**: User can schedule a post for a specific date and time with timezone selection
- [ ] **SOCL-04**: System publishes scheduled posts automatically at the specified time
- [ ] **SOCL-05**: User can post to Instagram Feed (single image + caption with hashtags)
- [ ] **SOCL-06**: User can post to Instagram Stories (single image)
- [ ] **SOCL-07**: User can attach any generated tracklist image or a custom uploaded image to a post
- [ ] **SOCL-08**: Custom-uploaded images are validated for format (JPEG/PNG), size (<8 MB), dimensions, and aspect ratio before scheduling
- [ ] **SOCL-09**: User sees a queue/calendar view of all scheduled, publishing, published, failed, and permanently failed posts
- [ ] **SOCL-10**: Post status tracks: draft, scheduled, publishing, published, failed, permanently_failed
- [ ] **SOCL-11**: Failed posts retry up to 3 times with exponential backoff; after exhaustion, post is marked `permanently_failed`
- [ ] **SOCL-12**: Permanently failed posts surface on dashboard and queue with error reason; user can retry once manually or download the image for manual posting
- [ ] **SOCL-13**: Scheduler respects Instagram rate limits: minimum 30-second interval between posts; backs off on 429 responses per response headers (default 15 minutes)
- [ ] **SOCL-14**: Scheduled posts can be fully edited (caption, image, time, timezone) while in `scheduled` status; edits are blocked once `publishing` begins
- [ ] **SOCL-15**: Disconnecting a social account moves all its `scheduled` posts to `draft` status; user is warned before disconnect
- [ ] **SOCL-16**: Before retrying a failed post, scheduler checks whether the post was already published on Instagram (duplicate prevention)
- [ ] **SOCL-17**: After image export in Tracklist Generator, UI shows a "Schedule Post" CTA that pre-fills the new post form with the generated image attached

### EPK / Press Kit Builder (EPK)

- [ ] **EPK-01**: User can write artist bio (short + long versions) with rich text editor supporting bold, italic, lists, links, and H2/H3 headings
- [ ] **EPK-02**: User can upload and manage up to 20 press photos (JPEG/PNG, max 10 MB each)
- [ ] **EPK-03**: User can upload tech rider content and stage plot image
- [ ] **EPK-04**: User can enter social/streaming profile links
- [ ] **EPK-05**: User can manually enter gig history highlights, press quotes, and contact information
- [ ] **EPK-06**: User can generate a professionally formatted PDF press kit applying their logo and color settings
- [ ] **EPK-07**: User can select which sections to include or exclude in the PDF export
- [ ] **EPK-08**: Each PDF export is stored in MinIO and listed in an export history view with timestamps; user can download or delete any previous export
- [ ] **EPK-09**: All EPK data stored in PostgreSQL; photo and document assets stored in MinIO
- [ ] **EPK-10**: When Gig Tracker (Phase 1d) is available, an "Import from Gigs" button populates gig highlights from selected gig entries

### Gig Tracker (GIG)

- [ ] **GIG-01**: User can create, edit, and delete gig entries with: date, venue, city, country, event name, promoter name/email/phone, fee, currency, notes
- [ ] **GIG-02**: Gig status workflow: `inquiry` → `confirmed` → `advanced` → `played` → `cancelled`; forward skipping and backward transitions allowed; `cancelled` is terminal
- [ ] **GIG-03**: Payment status tracked independently: `unpaid`, `deposit_paid`, `paid`, `overdue`, `waived`
- [ ] **GIG-04**: Currency stored as ISO 4217 code; no automatic conversion; searchable currency dropdown
- [ ] **GIG-05**: User can link a parsed tracklist to a gig
- [ ] **GIG-06**: User can view gigs in calendar view and list view
- [ ] **GIG-07**: User can filter gigs by date range, venue, city, status, and fee range
- [ ] **GIG-08**: "Copy from Previous Gig" dropdown auto-fills promoter fields from a selected prior gig
- [ ] **GIG-09**: All gig data stored in PostgreSQL with soft deletion

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
| Contract management / e-signatures | v3.0+ roadmap item |
| TikTok posting | v1.x follow-up |
| OAuth for non-Instagram platforms | Only Instagram in v1 |

---

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement Group | Phase | Status |
|-------------------|-------|--------|
| INFRA-01 to INFRA-11 | Phase 0 | Pending |
| TRKL-01 to TRKL-28 | Phase 1a | Pending |
| SOCL-01 to SOCL-17 | Phase 1b | Pending |
| EPK-01 to EPK-10 | Phase 1c | Pending |
| GIG-01 to GIG-09 | Phase 1d | Pending |
| FIN-01 to FIN-10 | Phase 1e | Pending |
| REL-01 to REL-07 | Phase 1f | Pending |
| TOUR-01 to TOUR-07 | Phase 1g | Pending |
| DASH-01 to DASH-08 | Phase 1h | Pending |

**Coverage:**
- v1 requirements: 93 total
- Mapped to phases: 93
- Unmapped: 0 ✓

---
*Requirements defined: 2026-03-14*
*Last updated: 2026-03-14 after initial definition*
