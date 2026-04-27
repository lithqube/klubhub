# Phase 4: Gig Tracker — Context

**Gathered:** 2026-04-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can manage their complete gig history and pipeline — from initial inquiry through played/cancelled — with full CRUD, status workflow, payment tracking, venue/contact database, iCal feed, and booking confirmation PDF. Downstream modules (Finance Tracker, Tour Manager, EPK-10 Import) consume gig data via a stable `GigReader` interface.

**Phase scope is FIXED.** New capabilities belong in their own phase.

</domain>

<decisions>
## Implementation Decisions

### Gig List View (from Phase 1.5.5 mock)
- **Current state:** `gigs.vue` already has a list view with date blocks (MON/DAY), venue name, details line (ROOM · CITY, COUNTRY), fee, and status badge
- **Status badges:** `CONFIRMED` (cyan outline), `PENDING` (magenta text), `PAST` (gray outline)
- **Accent bars:** 3-4px left vertical bar per row — cyan=confirmed, magenta=pending/draft, yellow=archived
- **Stats row:** Three glass cards — UPCOMING count, AVG FEE, YTD EARNED with TrendingUp icon

### Gig Status Workflow
- **Workflow:** `inquiry` → `confirmed` → `advanced` → `played` → `cancelled`
- Forward skipping and backward transitions allowed; `cancelled` is terminal
- **Payment status tracked independently:** `unpaid`, `deposit_paid`, `paid`, `overdue`, `waived`

### Gig CRUD Fields (GIG-01)
- date, venue, city, country, event name, promoter name/email/phone, fee, currency, notes
- Currency stored as ISO 4217 code; no automatic conversion; searchable dropdown

### Calendar View (GIG-06)
- User can view gigs in calendar view AND list view
- Tabs or toggle between list and calendar

### Venue & Contact Database (CONT-01 to CONT-06)
- **Venues:** name, city, country, capacity (optional), website (optional), technical contact name/email/phone, notes
- **Contacts:** name, company (optional), email, phone, type (promoter/agent/label/other), notes
- **Autocomplete:** minimum 2 characters to trigger search
- **Linking:** linking a venue auto-populates venue name, city, country; linking a contact auto-populates promoter fields; all auto-populated fields remain editable after link

### Gig-Venue/Gig-Contact Linking (GIG-09, GIG-10, GIG-11)
- User can link a gig to a venue record and a promoter contact record
- Linked records auto-populate fields; fields remain editable after link
- "Copy from Previous Gig" populates from the gig's linked venue and contact records (not just free-text fields) when those records exist

### Tracklist-Gig Linking (GIG-05)
- User can link a parsed tracklist to a gig
- "Copy from Previous Gig" dropdown auto-fills promoter fields from a selected prior gig

### iCal Feed (INT-02, INT-03)
- `GET /api/v1/gigs/calendar.ics` returns RFC 5545 compliant iCal feed
- Protected by shared secret in URL query param (`?secret={ICAL_SECRET}` env var)
- Each event includes: UID (gig UUID), DTSTART (gig date + time if known), SUMMARY (event name or venue name), LOCATION (venue name, city, country), DESCRIPTION (fee + currency + promoter contact email — omitted if empty)
- Importable by Google Calendar, Apple Calendar, any iCal-compatible client

### Booking Confirmation PDF (GIG-12)
- Available when gig is in `confirmed` status
- Pre-populated with: DJ name, venue name + city, event date, fee + currency, set length, technical contact name + email
- No e-signature required
- PDF stored in MinIO and downloadable
- Uses existing `gofpdf` pattern (same as EPK PDF)

### GigReader Interface (downstream contract)
- Downstream modules (Finance Tracker, Tour Manager, EPK-10 Import) read gig data via a stable `GigReader` interface
- Interface defined in Go as part of 04-01

### Gig Filters (GIG-07)
- Filter by: date range, venue, city, status, fee range

</decisions>

<specifics>
## Specific Ideas

### UI Patterns (from gigs.vue mock)
- Date block: 2-line date display (MON in accent color + DAY in large bold)
- Gig row: glass container with left accent bar, date block left, venue/details center, fee right, badge far right
- Stats cards: 3-column grid at top — UPCOMING (cyan accent), AVG FEE (violet accent), YTD EARNED (neutral)
- Badge labels: ALL CAPS, 8px font, letter-spacing 0.05em

### Gig Form (ADD GIG button)
- "ADD GIG" button in page header — cyan gradient CTA, matches `btn-hud-cta` class
- Form likely in a Dialog/Sheet — new gig or edit gig
- Fields: date picker, venue autocomplete (links to CONT), city/country (auto from venue or manual), event name, room/details, promoter contact autocomplete (links to CONT), fee + currency dropdown, notes textarea

### iCal Export Button
- Button in page header or stats area — "EXPORT ICAL" with ghost-border style
- Downloads `.ics` file on click OR copies subscription URL to clipboard (user choice)

### PDF Download Button
- Available on confirmed gigs — "DOWNLOAD CONFIRMATION" button
- Likely in gig detail view or as a row action

### "Copy from Previous Gig" Dropdown
- Dropdown in gig form that populates promoter fields from a prior gig's linked venue and contact records

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `gigs.vue` — existing page with HUD-styled mock data; 04-04 wires it to real API
- `glass`, `glass-panel` — section containers
- `accent-bar-*` CSS classes — left bar colors for status indication
- `badge-hud` + `badge-ready`/`badge-draft`/`badge-archived` — status badges already in styles.css
- `btn-hud`, `btn-hud-cta` — button classes
- `gig-list-row`, `gig-date-block`, `gig-venue`, `gig-meta`, `gig-fee` — gig-specific classes already defined
- `page-header`, `page-title`, `page-sub` — page chrome
- `section-lbl` — uppercase label styling
- `useTheme composable` — themeMode access
- `$fetch` — all API calls follow this pattern
- Pinia stores — settings.ts for djName/logoPath; create new `useGigStore` for gig CRUD state

### Established Patterns
- Go module structure: `api/internal/gig/` package with handler, service, repository, model files
- Gig status badge colors: cyan=confirmed, magenta=draft/pending, gray=archived
- PDF generation: `gofpdf` library (same as EPK); existing `EPKPDFGenerator` as reference
- MinIO storage: `storage.Client.PutObject` pattern for PDF storage
- Soft deletion everywhere with `deleted_at` timestamp
- UUID primary keys on all entities

### Integration Points
- `pages/gigs.vue` — wires to `/api/v1/gigs/*` endpoints (04-04)
- `TheNav.vue` — Gigs nav item already present
- `TheBottomNav.vue` — Gigs nav item present
- Go backend: new `api/internal/gig/`, `api/internal/venue/`, `api/internal/contact/` packages
- Finance Tracker (Phase 5): reads gig data via GigReader interface; auto-creates income when payment_status → paid
- Tour Manager (Phase 7): reads gig data via GigReader; groups gigs into tours
- EPK-10: "Import from Gigs" button in epk.vue becomes active after Phase 4

</code_context>

<deferred>
## Deferred Ideas

- **Calendar view detail** — monthly grid with colored dot indicators per day; specific interaction (click day → filter list, or navigate to day view) not yet discussed
- **iCal subscription URL** vs **download .ics file** — which UX for iCal export?
- **Bandsintown outbound sync** (Phase 4.8) — when gig transitions to confirmed and API key is set, push to Bandsintown
- **Rider Templates** (Phase 4.5) — attached when gig transitions to `advanced` status
- **Finance auto-income** — Phase 5 handles auto-creation of income entry when gig payment_status → paid

</deferred>

<constraints>
## Constraints

- **Pure Go PDF generation** — `gofpdf` library; no headless Chrome
- **iCal RFC 5545 compliant** — standard format, no proprietary extensions
- **ICAL_SECRET env var** — URL query param protection, no authentication
- **ISO 4217 currency codes** — no automatic conversion; display grouped by currency
- **GigReader interface** — must be stable for downstream module consumption
- **No e-signature** — booking confirmation PDF is informational only
- **Phase 0 dependency** — Go backend infrastructure must be in place (chi router, DB connection, migrations)

</constraints>

---

*Phase: 04-gig-tracker*
*Context gathered: 2026-04-27*
