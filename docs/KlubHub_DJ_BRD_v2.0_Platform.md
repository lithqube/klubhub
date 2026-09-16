# KLUBHUB DJ

> **Historical requirements record:** MinIO references below are superseded
> for v1.0.0 deployment by Garage (S3-compatible). They remain here to preserve
> the original requirements traceability, not as current operating guidance.

*The Open-Source Career Toolkit for DJs*

---

**Business Requirements Document**
Version 2.0 — March 2026

Open Source • Self-Hosted • Docker-Ready
Tracklist Visuals • Social Posting • EPK • Gig Management • Finances • Releases • Touring

---

## 1. Executive Summary

KlubHub DJ is an open-source, self-hosted web platform designed to help DJs and electronic music artists manage their entire career from a single dashboard. It combines creative tools (tracklist image generation, social media scheduling) with business management features (gig tracking, finances, EPK building, release planning, tour management) into one cohesive, Docker-deployable application.

The platform ships in progressive phases, starting with the flagship tracklist image generator — a tool that transforms DJ set history files into professional, social-media-ready visuals — and expanding module by module into a complete career toolkit. KlubHub DJ is built for the working DJ who wants to spend less time on admin and more time on music.

> **Vision:** Every DJ deserves a free, powerful, self-owned platform to manage their career — without relying on fragmented SaaS tools, spreadsheets, or expensive management teams.

---

## 2. Project Overview

| Field | Value |
|---|---|
| **Project Name** | KlubHub DJ |
| **Tagline** | The Open-Source Career Toolkit for DJs |
| **Type** | Open-source web platform (self-hosted) |
| **License** | TBD (MIT or Apache 2.0 recommended) |
| **Deployment** | Docker / Docker Compose |
| **Architecture** | Modular monolith (single Go service, clean module boundaries) |
| **Target Users** | DJs, electronic music producers, podcast hosts, radio show curators |
| **Auth Model** | No authentication (single-user, local-first tool) |
| **Release Strategy** | Phased delivery: 1a → 1b → 1c → 1d → 1e → 1f → 1g |

---

## 3. Problem Statement

Managing a DJ career today requires juggling a fragmented set of tools, platforms, and manual processes:

- **Tracklist sharing is manual:** DJs screenshot or type tracklists by hand, resulting in inconsistent and unprofessional social media posts.
- **Social media management requires separate paid tools** (Buffer, Later, Hootsuite) that aren't tailored to music artists.
- **Press kits are built in Google Docs or Canva** — disconnected from gig data, bios, and tech riders.
- **Gig tracking lives in spreadsheets:** dates, venues, fees, and promoter contacts scattered across files and email threads.
- **Financial tracking is nonexistent** for most independent DJs: income from gigs, streaming, and merch is untracked, making tax season painful.
- **Release planning** across labels, distributors, and platforms has no centralized coordination tool built for artists.
- **Tour logistics** (travel, accommodation, gear shipping) are managed ad-hoc with no structured workflow.
- **No single tool integrates all of these** into one platform, and the few that try are closed-source, cloud-only, and expensive.

KlubHub DJ addresses this by providing one self-hosted platform where DJs own their data and manage every aspect of their career.

---

## 4. Platform Vision & Module Map

KlubHub DJ is organized as a modular platform. Each module is a self-contained feature area with its own data, UI section, and API endpoints, but modules can share data and cross-reference each other (e.g., a gig links to its tracklist and generated images).

| Module | Phase | Description |
|---|---|---|
| **Tracklist Image Generator** | 1a (Flagship) | Upload DJ history files, generate social-media-ready tracklist images with cover art mosaics and templates |
| **Social Media Scheduler** | 1b | Schedule and publish posts to Instagram (feed + stories); auto-attach generated tracklist images |
| **EPK / Press Kit Builder** | 1c | Build and export downloadable PDF press kits with bio, photos, tech rider, stage plot, and selected gig history |
| **Gig Tracker** | 1d | Log gigs with dates, venues, fees, promoter contacts, and status tracking; link tracklists to gigs |
| **Finance Tracker** | 1e | Track income (gig fees, streaming, merch) and expenses (travel, gear, promo); basic invoicing |
| **Release Planner** | 1f | Plan upcoming releases with label, distributor, deadlines, promo timeline, and platform links |
| **Tour Manager** | 1g | Coordinate multi-date tours: travel, accommodation, logistics, per-city checklists |

All modules share a unified dashboard as the home screen, showing upcoming gigs, pending releases, scheduled posts, and recent tracklists at a glance.

---

## 5. Technology Stack

| Component | Technology |
|---|---|
| **Frontend** | Nuxt 3 (Vue.js / TypeScript) — SSR-capable, modular page structure per feature |
| **Backend** | Go (Golang) — modular monolith with clean internal package boundaries per module |
| **Database** | PostgreSQL 16 — relational data for all modules; JSONB for flexible config |
| **Object Storage** | MinIO — S3-compatible; stores uploads, cover art, generated images, EPK assets, logos |
| **Image Processing** | Go image libraries (gg, imaging) for tracklist image generation |
| **Social Media APIs** | Instagram Graph API (feed + stories), TikTok API (planned v1.x) |
| **Cover Art APIs** | Spotify Web API, Discogs API, MusicBrainz API (fallback chain) |
| **PDF Generation** | Go PDF library (gofpdf or chromedp for rich layouts) for EPK export |
| **Containerization** | Docker + Docker Compose (single docker-compose.yml) |
| **Integration Architecture** | Plugin-style adapter interfaces designed from day one for future external integrations |

### 5.1 Integration-Ready Architecture

Although external integrations (Resident Advisor, Beatport, Bandcamp, etc.) are not built in v1, the backend is designed from the start with adapter interfaces for:

- Venue/event data providers (Resident Advisor, Songkick, Dice).
- Music platforms (Beatport, Bandcamp, Spotify for Artists, SoundCloud).
- Financial integrations (Stripe, PayPal for payment tracking).
- Calendar sync (Google Calendar, iCal export).
- Email contacts import.

Each integration point is defined as a Go interface that can be implemented by concrete adapters in future phases without refactoring core module logic.

---

## 6. Phase 1a — Tracklist Image Generator (Flagship)

The flagship feature that ships first. This is the primary draw for early adopters and establishes KlubHub DJ's brand.

### 6.1 DJ Software Support

| Software | Supported Formats | Extracted Metadata |
|---|---|---|
| **Rekordbox** | .txt history, .csv history, XML library database | Artist, title, BPM, key, label, duration, time played |
| **Serato DJ Pro** | Session history files (crate/history format) | Artist, title, BPM, key, duration, time played |
| **Traktor** | .nml collection/history files | Artist, title, BPM, key, label, duration, time played |

The system auto-detects file format on upload with no user input required.

### 6.2 Image Output

- Instagram Story: 1080 × 1920 pixels.
- Instagram Square: 1080 × 1080 pixels.
- Export as PNG (lossless) or JPEG (compressed).

### 6.3 Background Options

1. Upload a custom background image (JPEG/PNG).
2. Solid color background with color picker.
3. Cover art mosaic — automatic grid/collage layout from the set's album artwork.

### 6.4 Cover Art Fetching

- Automatic lookup using artist + title against: Spotify → Discogs → MusicBrainz (fallback chain).
- All fetched artwork cached in MinIO to avoid redundant API calls.
- Asynchronous fetching with progressive UI updates.
- Placeholder artwork for tracks with no match.

### 6.5 Design & Customization

- 3–5 built-in design presets (e.g., dark minimal, neon glow, classic vinyl, gradient wave, clean white).
- Light customization per preset: primary/accent color override, font color.
- Logo/watermark upload (PNG with transparency) with corner position control.
- User-configurable metadata display: artist, title, BPM, key, label, time played, set duration, DJ name, event name, date — toggle each on/off.
- Live preview before final high-resolution export.

### 6.6 Functional Requirements — Phase 1a

| ID | Requirement |
|---|---|
| **FR-001** | System shall accept file uploads via drag-and-drop or file picker. |
| **FR-002** | System shall auto-detect file format (Rekordbox, Serato, Traktor) without user input. |
| **FR-003** | System shall extract: artist, title, BPM, key, label, duration, and time played from each supported format. |
| **FR-004** | System shall validate uploaded files and return clear error messages for unsupported or malformed files. |
| **FR-005** | System shall store parsed tracklists in PostgreSQL and raw files in MinIO. |
| **FR-006** | System shall search Spotify, Discogs, and MusicBrainz APIs for cover art using artist + title. |
| **FR-007** | System shall follow fallback chain: Spotify → Discogs → MusicBrainz → placeholder. |
| **FR-008** | System shall cache fetched cover art in MinIO. |
| **FR-009** | System shall generate images at 1080×1920 and 1080×1080. |
| **FR-010** | System shall support three background modes: custom upload, solid color, cover art mosaic. |
| **FR-011** | System shall render user-selected metadata fields onto the image using the chosen template. |
| **FR-012** | System shall overlay logo/watermark at chosen corner position. |
| **FR-013** | System shall export as PNG and optionally JPEG. |
| **FR-014** | System shall provide live preview before export. |
| **FR-015** | System shall include 3–5 built-in design presets with color customization. |
| **FR-016** | System shall persist user's last-used settings for convenience. |

### 6.7 User Workflow — Phase 1a

1. **Upload:** User opens KlubHub DJ, navigates to Tracklist Generator, uploads their DJ history file.
2. **Parse:** Backend auto-detects format, parses data, returns structured tracklist. UI displays all tracks with metadata.
3. **Configure:** User selects metadata fields to display, chooses output format (Story/Square), picks a design preset, adjusts colors, uploads logo.
4. **Background:** User chooses custom image upload, solid color, or cover art mosaic.
5. **Preview:** Live preview renders in the browser with all options applied.
6. **Export:** User clicks export; backend generates full-resolution image; browser downloads it.

---

## 7. Phase 1b — Social Media Scheduler

Tightly integrated with the tracklist generator. After creating a tracklist image, users can schedule it for posting directly from KlubHub DJ.

### 7.1 Supported Platforms (v1)

| Platform | Capabilities | Tier |
|---|---|---|
| **Instagram Feed** | Photo posts with caption and hashtags | Open-Source (free) |
| **Instagram Stories** | Image stories with optional link sticker (business accounts) | Open-Source (free) |
| **TikTok, Twitter/X, Facebook** | Cross-platform posting from the same scheduled post | KlubHub Cloud PRO |

Instagram scheduling is free and open-source — it completes the core tracklist→post workflow. Multi-platform posting (TikTok, Twitter/X, Facebook) is available in KlubHub Cloud PRO, where centralized API key management and rate limit orchestration across platforms are handled by the hosted infrastructure.

### 7.2 Core Features

- Schedule posts for a specific date and time with timezone support.
- Attach any generated tracklist image (or upload a custom image) to a scheduled post.
- Write captions with hashtag suggestions based on track genres/labels.
- Preview how the post will look on the target platform before scheduling.
- Queue view: calendar/list of all upcoming scheduled posts.
- Post status tracking: scheduled, published, failed (with retry).
- Auto-attach: option to automatically create a draft post when a new tracklist image is generated.
- **KlubHub Cloud PRO only:** AI-generated caption suggestions (3 variants) from tracklist data using Claude API.

### 7.3 Functional Requirements — Phase 1b

| ID | Requirement |
|---|---|
| **FR-020** | System shall authenticate with Instagram Graph API via OAuth (Facebook Business account). |
| **FR-021** | System shall allow scheduling posts for a future date/time with timezone selection. |
| **FR-022** | System shall publish scheduled posts automatically at the specified time. |
| **FR-023** | System shall support posting to Instagram Feed (single image + caption). |
| **FR-024** | System shall support posting to Instagram Stories (single image). |
| **FR-025** | System shall allow attaching any generated tracklist image or a custom upload to a post. |
| **FR-026** | System shall provide a queue/calendar view of all scheduled and published posts. |
| **FR-027** | System shall track post status (scheduled, publishing, published, failed) and support retry on failure. |
| **FR-028** | System shall store all post data and scheduling metadata in PostgreSQL. |

### 7.4 Technical Notes

- Instagram posting requires a Facebook Business account and Instagram Graph API app approval.
- The Go backend runs a scheduler (cron-style) that checks for posts due and publishes them.
- Failed posts are retried up to 3 times with exponential backoff.
- The .env file stores Instagram/Facebook API credentials.

---

## 8. Phase 1c — EPK / Press Kit Builder

A tool for building professional electronic press kits that DJs can send to promoters, venues, labels, and media.

### 8.1 EPK Content Sections

- Artist bio (short and long versions, rich text editor).
- Press photos (upload multiple, select which to include).
- Tech rider (equipment requirements, input list, stage plot diagram upload).
- Stage plot (upload image or build a simple one from templates).
- Social/streaming links (Instagram, SoundCloud, Spotify, Bandcamp, etc.).
- Selected gig history (auto-populated from Gig Tracker once available, or manual entry).
- Upcoming releases or recent releases (manual entry until Release Planner ships).
- Testimonials / press quotes (manual entry).
- Contact information.

### 8.2 Output Format

v1 generates a downloadable PDF press kit with a professional layout. The PDF includes the user's branding (logo, colors from their KlubHub DJ settings).

A hosted public EPK page with a shareable URL (`klubhub.dj/epk/your-name`) is available in **KlubHub Cloud PRO**. Custom domains (`epk.yourname.com`) are available in **KlubHub Cloud TEAM**. This is intentionally not in the self-hosted v1 — hosted pages require web routing, SSL certificate management, and CDN infrastructure that self-hosters should not need to manage. PDF export satisfies the core use case (emailing your press kit) without any server dependency.

### 8.3 Functional Requirements — Phase 1c

| ID | Requirement |
|---|---|
| **FR-030** | System shall provide a rich text editor for artist bio (short + long versions). |
| **FR-031** | System shall allow uploading and managing multiple press photos. |
| **FR-032** | System shall allow uploading tech rider content and stage plot images. |
| **FR-033** | System shall allow entering social/streaming profile links. |
| **FR-034** | System shall allow manual entry of gig history, press quotes, and contact info. |
| **FR-035** | System shall generate a professionally formatted PDF press kit from the entered data. |
| **FR-036** | System shall apply the user's logo and color settings to the PDF layout. |
| **FR-037** | System shall allow selecting which sections to include/exclude in the PDF. |
| **FR-038** | System shall store all EPK data in PostgreSQL and assets in MinIO. |

---

## 9. Phase 1d — Gig Tracker

A centralized place to log, track, and manage all gigs — past and upcoming.

### 9.1 Core Features

- Log gigs with: date, venue name, city/country, event name, promoter/booker contact, agreed fee, currency, payment status.
- Status workflow: inquiry → confirmed → advanced → played → played → cancelled.
- Link a tracklist (from the Tracklist Generator) to a gig.
- Notes field per gig (travel details, hotel, special requirements).
- Calendar view and list view of all gigs.
- Filter/search by date range, venue, city, status, fee range.
- **Venue database:** Create and manage reusable venue records (name, city, country, capacity, website, technical contact). Search and link a saved venue when creating or editing a gig — auto-populates venue fields.
- **Contact database:** Create and manage reusable promoter/agent/label contacts. Search and link a contact to a gig — auto-populates promoter fields.
- **Booking confirmation PDF:** Generate a formatted booking confirmation from any confirmed gig, pre-populated with DJ name, venue, date, fee, currency, set length, and technical contact. No e-signature required.
- **iCal feed:** `GET /api/v1/gigs/calendar.ics` exports all non-cancelled gigs as a standards-compliant iCal feed importable by Google Calendar, Apple Calendar, and any iCal-compatible client.

### 9.2 Functional Requirements — Phase 1d

| ID | Requirement |
|---|---|
| **FR-040** | System shall allow creating, editing, and deleting gig entries. |
| **FR-041** | System shall track gig status through a configurable workflow (inquiry → confirmed → advanced → played → cancelled). |
| **FR-042** | System shall store venue, promoter contact, fee, currency, and payment status per gig. |
| **FR-043** | System shall allow linking tracklists and generated images to gig entries. |
| **FR-044** | System shall provide calendar and list views for gigs. |
| **FR-045** | System shall support filtering by date, venue, city, status, and fee range. |
| **FR-046** | System shall store all gig data in PostgreSQL with soft deletion. |
| **FR-047** | System shall provide a reusable venue database: create, edit, delete venue records; search and link to gig with auto-populate. |
| **FR-048** | System shall provide a reusable contact database: create, edit, delete contact records; search and link to gig with auto-populate. |
| **FR-049** | System shall generate a booking confirmation PDF from any confirmed gig pre-populated with DJ and venue details. |
| **FR-050** | System shall expose `GET /api/v1/gigs/calendar.ics` returning a valid RFC 5545 iCal feed of all non-cancelled gigs. |

---

## 10. Phase 1e — Finance Tracker

Basic financial tracking for independent DJs to understand their income, expenses, and profitability.

### 10.1 Core Features

- Income tracking: gig fees (auto-linked from Gig Tracker), streaming revenue, merch sales, other income.
- Expense tracking: travel, accommodation, gear, promo/marketing, software subscriptions, other.
- Categories and tags for both income and expenses.
- Multi-currency support with manual exchange rate entry.
- Basic invoicing: generate a simple PDF invoice from gig data (DJ name, gig details, fee, bank details).
- Monthly/yearly summary views with totals and category breakdowns.
- Profit/loss overview per gig, per month, per year.

### 10.2 Functional Requirements — Phase 1e

| ID | Requirement |
|---|---|
| **FR-060** | System shall allow logging income entries with amount, currency, category, date, and optional gig link. |
| **FR-061** | System shall allow logging expense entries with amount, currency, category, date, and notes. |
| **FR-062** | System shall auto-create income entries from gig fees when a gig is marked as paid. |
| **FR-063** | System shall provide monthly and yearly summary views with category breakdowns. |
| **FR-064** | System shall calculate profit/loss per gig, per month, and per year. |
| **FR-065** | System shall generate a basic PDF invoice from gig and user settings data. |
| **FR-066** | System shall support multiple currencies with manual exchange rates. |

---

## 11. Phase 1f — Release Planner

A planning tool to coordinate music releases across labels, distributors, and platforms.

### 11.1 Core Features

- Log upcoming releases: track/EP/album title, artists, label, distributor, release date, format (digital/vinyl/both).
- Status workflow: demo sent → accepted → mastering → artwork → promo → released.
- Deadline tracking: mastering due, artwork due, promo start, release date.
- Platform links: Beatport, Bandcamp, Spotify, SoundCloud (manually entered until integrations are built).
- Promo timeline: checklist of promo tasks leading up to release (send to DJs, submit to playlists, social posts).
- Notes field per release for label communication, contract details, etc.

### 11.2 Functional Requirements — Phase 1f

| ID | Requirement |
|---|---|
| **FR-060** | System shall allow creating, editing, and deleting release entries. |
| **FR-061** | System shall track release status through a configurable workflow. |
| **FR-062** | System shall track multiple deadlines per release with date and status. |
| **FR-063** | System shall allow entering platform links per release. |
| **FR-064** | System shall provide a promo checklist per release with task completion tracking. |
| **FR-065** | System shall provide a timeline/calendar view of upcoming releases and their deadlines. |

---

## 12. Phase 1g — Tour Manager

Coordinate multi-date tours and extended booking runs with logistics tracking. The Tour Manager ships as a core open-source feature — DJs own their tour logistics data. Real-time collaborative tour management (inviting managers, agents, or promoters to view and edit tour details with live updates) is a KlubHub Cloud TEAM feature available in v2.

### 12.1 Core Features

- Create a tour as a named group of linked gigs (from Gig Tracker).
- Per-stop logistics: travel (flights, trains, car), accommodation (hotel, Airbnb), ground transport.
- Per-stop checklist: customizable items (e.g., confirm sound check, send tech rider, arrange gear shipping).
- Tour budget: aggregate income (fees) vs. expenses (travel, hotels) across all stops.
- Timeline view: map + calendar showing tour route and dates.
- Document storage per tour: contracts, itineraries, boarding passes (stored in MinIO).

### 12.2 Functional Requirements — Phase 1g

| ID | Requirement |
|---|---|
| **FR-070** | System shall allow creating a tour as a named collection of linked gigs. |
| **FR-071** | System shall track travel and accommodation details per tour stop. |
| **FR-072** | System shall provide customizable checklists per tour stop. |
| **FR-073** | System shall calculate aggregate tour budget (total fees vs. total expenses). |
| **FR-074** | System shall provide a timeline/calendar view of tour dates. |
| **FR-075** | System shall allow uploading and storing documents per tour in MinIO. |

---

## 13. Unified Dashboard

The KlubHub DJ home screen provides an at-a-glance overview of the DJ's career activity. It surfaces data from all active modules:

- Upcoming gigs (next 30 days) with status indicators.
- Scheduled social media posts (next 7 days).
- Recent tracklist images generated.
- Pending release deadlines.
- Active tour status (if any).
- Monthly income/expense summary.
- Quick actions: upload tracklist, schedule post, log gig, create invoice.

The dashboard adapts based on which modules are active — if a module hasn't shipped yet or has no data, its widget is hidden.

---

## 14. Data Model (PostgreSQL)

Core entities organized by module. All entities use UUID primary keys, created_at/updated_at timestamps, and soft deletion. Cross-module relationships enable the integrated experience.

### 14.1 Shared / Settings

| Table | Fields |
|---|---|
| **user_settings** | Singleton: dj_name, logo_path, default_colors, default_template, visible_fields, social_links, bio_short, bio_long, contact_info |

### 14.2 Tracklist Module

| Table | Fields |
|---|---|
| **tracklists** | id, name, source_format (enum), source_file_path (MinIO), dj_name, event_name, event_date, gig_id (FK, nullable) |
| **tracks** | id, tracklist_id (FK), position, artist, title, bpm, key, label, duration, played_at, cover_art_path (MinIO) |
| **generated_images** | id, tracklist_id (FK), format (enum), template_id, config_json (JSONB), output_path (MinIO) |

### 14.3 Social Media Module

| Table | Fields |
|---|---|
| **social_accounts** | id, platform (enum: instagram), account_name, access_token (encrypted), token_expiry |
| **scheduled_posts** | id, social_account_id (FK), image_path (MinIO), caption, hashtags, scheduled_at, timezone, status (enum), published_at, error_log, retry_count |

### 14.4 EPK Module

| Table | Fields |
|---|---|
| **epk_profiles** | id, bio_short, bio_long, press_photos (JSONB array of MinIO paths), tech_rider_text, stage_plot_path, social_links (JSONB), gig_highlights (JSONB), press_quotes (JSONB), contact_info |
| **epk_exports** | id, epk_profile_id (FK), pdf_path (MinIO), sections_included (JSONB), generated_at |

### 14.5 Gig Module

| Table | Fields |
|---|---|
| **venues** | id, name, city, country, capacity (nullable), website (nullable), technical_contact_name, technical_contact_email, technical_contact_phone, notes |
| **contacts** | id, name, company (nullable), email, phone, type (enum: promoter, agent, label, other), notes |
| **gigs** | id, date, venue_id (FK→venues, nullable), venue_name, city, country, event_name, contact_id (FK→contacts, nullable), promoter_name, promoter_email, promoter_phone, fee, currency, payment_status (enum), gig_status (enum), notes, tracklist_id (FK, nullable) |
| **gig_riders** | id, gig_id (FK), technical (text), hospitality (text), backline (text), other (text), rider_template_id (FK→rider_templates, nullable, source reference only) |
| **rider_templates** | id, name, technical (text), hospitality (text), backline (text), other (text) |

### 14.6 Finance Module

| Table | Fields |
|---|---|
| **income_entries** | id, amount, currency, category (enum), date, description, gig_id (FK, nullable) |
| **expense_entries** | id, amount, currency, category (enum), date, description, tour_id (FK, nullable) |
| **invoices** | id, gig_id (FK), invoice_number, issued_date, due_date, amount, currency, status (enum), pdf_path (MinIO) |

### 14.7 Release Module

| Table | Fields |
|---|---|
| **releases** | id, title, artists, label, distributor, release_date, format (enum), status (enum), platform_links (JSONB), notes |
| **release_deadlines** | id, release_id (FK), name, due_date, completed |
| **release_promo_tasks** | id, release_id (FK), task_name, due_date, completed |

### 14.8 Tour Module

| Table | Fields |
|---|---|
| **tours** | id, name, start_date, end_date, notes |
| **tour_stops** | id, tour_id (FK), gig_id (FK), travel_details (JSONB), accommodation (JSONB), checklist (JSONB), documents (JSONB array of MinIO paths) |

---

## 15. API Design

The Go backend exposes a RESTful API organized by module. All endpoints are prefixed with `/api/v1`. Key endpoint groups:

### 15.1 Tracklist Module

| Endpoint | Description |
|---|---|
| `POST /upload` | Upload and auto-parse a DJ history file; returns structured tracklist |
| `GET /tracklists` | List all parsed tracklists |
| `GET /tracklists/:id` | Get tracklist with full track metadata |
| `POST /tracklists/:id/generate` | Generate image with template, background, and metadata options |
| `GET /covers/:artist/:title` | Fetch/return cover art (cache-first) |
| `GET /templates` | List available design templates |
| `GET /export/:id` | Download generated image |

### 15.2 Social Media Module

| Endpoint | Description |
|---|---|
| `POST /social/accounts` | Connect a social media account (OAuth flow) |
| `GET /social/accounts` | List connected accounts |
| `POST /social/posts` | Create/schedule a post |
| `GET /social/posts` | List all posts (scheduled, published, failed) |
| `PUT /social/posts/:id` | Update a scheduled post |
| `DELETE /social/posts/:id` | Cancel a scheduled post |
| `POST /social/posts/:id/retry` | Retry a failed post |

### 15.3 EPK Module

| Endpoint | Description |
|---|---|
| `GET /epk` | Get EPK profile data |
| `PUT /epk` | Update EPK profile |
| `POST /epk/photos` | Upload press photos |
| `POST /epk/export` | Generate PDF press kit |
| `GET /epk/exports` | List generated EPK PDFs |

### 15.4 Gig, Finance, Release, Tour Modules

Each follows standard REST patterns: GET (list/detail), POST (create), PUT (update), DELETE (remove). Finance module adds `GET /finance/summary` for aggregated views. Tour module adds `GET /tours/:id/budget` for aggregated financials.

Additional Gig module endpoints:

| Endpoint | Description |
|---|---|
| `GET /api/v1/gigs/calendar.ics` | iCal feed of all non-cancelled gigs (RFC 5545) |
| `POST /api/v1/gigs/:id/confirmation-pdf` | Generate booking confirmation PDF for a confirmed gig |
| `GET /api/v1/venues` | List all venue records |
| `POST /api/v1/venues` | Create a venue record |
| `PUT /api/v1/venues/:id` | Update a venue record |
| `DELETE /api/v1/venues/:id` | Soft-delete a venue record |
| `GET /api/v1/contacts` | List all contact records |
| `POST /api/v1/contacts` | Create a contact record |
| `PUT /api/v1/contacts/:id` | Update a contact record |
| `DELETE /api/v1/contacts/:id` | Soft-delete a contact record |
| `GET /api/v1/rider-templates` | List all rider templates |
| `POST /api/v1/rider-templates` | Create a rider template |
| `PUT /api/v1/rider-templates/:id` | Update a rider template |
| `POST /api/v1/gigs/:id/rider` | Attach a rider template to a gig (creates per-gig copy) |
| `PUT /api/v1/gigs/:id/rider` | Update the per-gig rider |
| `POST /api/v1/gigs/:id/rider/pdf` | Export the per-gig rider as PDF |

### 15.5 System

| Endpoint | Description |
|---|---|
| `GET /health` | Health check for all services |
| `GET /settings` | Get user settings |
| `PUT /settings` | Update user settings |
| `GET /dashboard` | Aggregated dashboard data across all modules |

---

## 16. Deployment Architecture

KlubHub DJ ships as a single `docker-compose.yml` with four services:

| Service | Description |
|---|---|
| **klubhub-dj** | Single image (distroless Go API binary + Nuxt 3 frontend) listening on port 8080; API at `/api/v1/*`, UI at `/` |
| **klubhub-dj-db** | PostgreSQL 16 with named volume for persistence |
| **klubhub-dj-storage** | S3-compatible object store (Garage v2) with named volume; S3 API on port 3900 inside the container |

Deployment is a single command:

```bash
git clone https://github.com/lithqube/klubhub && cd klubhub && cp .env.example .env && docker compose -f docker-compose.prod.yml up -d
```

The `.env` file configures: Spotify/Discogs/MusicBrainz API keys, Instagram/Facebook API credentials, PostgreSQL connection, S3 credentials, ports, and timezone.

> **Image naming.** The published package is
> `ghcr.io/lithqube/klubhub-dj:v1.0.0` (linux/arm64 only). The single
> image carries both the API and the embedded UI; there is no separate
> `klubhub-dj-frontend` image. See `docs/container-images.md`.

---

## 17. Non-Functional Requirements

| Area | Requirement |
|---|---|
| **Performance** | Image generation < 10s for 30 tracks. Dashboard loads in < 2s. API responses < 500ms. |
| **Scalability** | Single-user design; no horizontal scaling required. Modular monolith supports future extraction. |
| **Reliability** | Graceful degradation if external APIs are unreachable. Social post retry with exponential backoff. |
| **Security** | No auth layer. API keys in env vars only. Sanitize all file uploads. Encrypted token storage for social accounts. |
| **Portability** | Runs on any Docker-compatible host (Linux, macOS, Windows via Docker Desktop). |
| **Accessibility** | Web UI meets WCAG 2.1 AA color contrast ratios. |
| **Maintainability** | Each module is a clean Go package with its own routes, handlers, service layer, and repository. Adding a module requires no changes to existing modules. |
| **Extensibility** | Integration adapter interfaces defined from day one. New DJ software parsers implement a shared Parser interface. |

---

## 18. Release Roadmap

### 18.1 v1 — Phased Core Platform

| Phase | Module | Key Deliverables |
|---|---|---|
| **1a** | Tracklist Image Generator | File parsing (3 formats), image gen (2 sizes, 3 bg modes), cover art, templates, live preview, text export (1001Tracklists / Mixcloud / SoundCloud) |
| **1b** | Social Media Scheduler | Instagram feed + stories, scheduling, queue view, auto-attach from tracklist gen (Instagram is OSS; multi-platform is KlubHub Cloud PRO) |
| **1c** | EPK / Press Kit Builder | Bio editor, photo management, tech rider, stage plot, PDF export (PDF is OSS; hosted EPK URL is KlubHub Cloud PRO) |
| **1d** | Gig Tracker | Gig CRUD, status workflow, venue & contact database, iCal feed, booking confirmation PDF, calendar/list views, tracklist linking |
| **1d+** | Rider Templates | Named rider templates, per-gig attachment with overrides, rider PDF export |
| **1d+** | Bandsintown Sync (optional) | Outbound event push to Bandsintown on gig confirmed |
| **1e** | Finance Tracker | Income/expense logging, gig fee auto-linking, summaries, basic PDF invoicing |
| **1f** | Release Planner | Release CRUD, deadline tracking, promo checklists, timeline view |
| **1g** | Tour Manager | Tour creation, per-stop logistics, checklists, budget aggregation, document storage (OSS; collaboration is KlubHub Cloud TEAM) |

### 18.2 v1.x — Incremental Enhancements

- Additional image output formats (Twitter/X banner, A4 poster, custom dimensions).
- Data export (CSV/JSON) for all modules.
- VirtualDJ, djay Pro, Engine DJ history file parsing.

### 18.3 v2.0 — KlubHub Cloud SaaS + Integrations

- **KlubHub Cloud PRO:** Multi-platform social posting (TikTok, Twitter/X, Facebook), AI caption generation, hosted EPK pages.
- **KlubHub Cloud TEAM:** Multi-user roster, collaborative tour planning, custom EPK domains.
- External integrations: Resident Advisor, Beatport, Bandcamp, Spotify for Artists.
- AI-generated and procedural/gradient backgrounds for tracklist images.
- Promo / marketing asset generator (flyers, banners).

### 18.4 v3.0+ — Future Vision

- Multi-user support with authentication (shared instances, collectives, agencies) in self-hosted OSS.
- Full design editor (custom fonts, drag-and-drop layout).
- Mobile-responsive or native mobile app.
- Community template marketplace.
- Contract management and e-signatures.

---

## 19. Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| External API rate limits (Spotify, Discogs, Instagram) | Cover art / posting may fail | Fallback chain, aggressive caching, retry with backoff, clear user feedback |
| Instagram API policy changes or app review delays | Social posting blocked or delayed | Design module to degrade gracefully; support manual posting workflow as fallback |
| DJ software format changes in new versions | Parsing breaks for updated formats | Modular parser architecture; community contributions; version-pinned specs |
| Scope creep across 7 modules | Delayed delivery, quality issues | Strict phased delivery; each phase ships independently; no cross-phase dependencies |
| Large tracklists (100+ tracks) causing slow image generation | Poor UX | Async generation with progress bar; rendering optimization; track display limits per template |
| MinIO storage growth over time | Disk exhaustion | Configurable retention; cleanup endpoints; storage management docs |
| Single-user design limits adoption | Communities/agencies can't use it | Planned multi-user support in v3; modular auth can be added without refactoring |

---

## 20. Open Questions

1. **License:** MIT (maximum adoption) vs Apache 2.0 (patent protection)? Recommend MIT.
2. **Image rendering:** pure Go (gg + imaging) vs headless Chromium (richer templates, heavier Docker image)?
3. **Spotify API** requires app registration — guide users through setup or prioritize Discogs/MusicBrainz?
4. **Mosaic layout:** support multiple grid patterns (4×4, 3×3, staggered) or one default for v1?
5. **Instagram Graph API** requires Facebook Business account — is this acceptable for the target audience?
6. **Job scheduler:** lightweight job queue (like Asynq/Redis) or simple PostgreSQL-based polling?
7. **EPK PDF:** pure Go generation (gofpdf) or headless Chrome for richer layouts? Same trade-off as image gen.
8. **Module boundaries:** should the modular monolith use Go's `internal/` package pattern to enforce boundaries at compile time?

---

## 21. Glossary

| Term | Definition |
|---|---|
| **KlubHub DJ** | The platform being described in this document |
| **EPK** | Electronic Press Kit — a package of promotional materials for an artist |
| **Rekordbox** | Pioneer DJ's music management and performance software |
| **Serato** | Serato DJ Pro — professional DJ software |
| **Traktor** | Native Instruments' DJ software |
| **MinIO** | Open-source S3-compatible object storage |
| **Modular monolith** | A single deployable application with clearly separated internal modules |
| **Cover art mosaic** | A grid/collage of album covers from the tracks in a set |
| **Tech rider** | Document specifying the technical equipment a DJ requires at a venue |
| **Stage plot** | Diagram showing equipment placement and positioning on stage |
| **BPM** | Beats per minute — the tempo of a track |
| **Key** | The musical key of a track (e.g., Am, 8A in Camelot notation) |
| **Instagram Graph API** | Facebook/Meta's official API for programmatic Instagram posting |
