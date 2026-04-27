# Phase 4: Gig Tracker — Implementation Plan

**Phase:** 04-gig-tracker
**Status:** Ready for implementation
**Started:** 2026-04-27

---

## Overview

Users manage their complete gig history and pipeline — from initial inquiry through played/cancelled — with full CRUD, status workflow, payment tracking, venue/contact database, iCal feed, and booking confirmation PDF. Downstream modules (Finance Tracker, Tour Manager, EPK-10 Import) consume gig data via a stable `GigReader` interface.

**Depends on:** Phase 0 (infrastructure scaffold)
**Scope boundary:** Gig Tracker is self-contained; downstream modules consume via interface.

---

## Migration Map

```
migration_005  →  gigs table
migration_005b →  venues table
migration_005c →  contacts table
migration_005d →  gig_venues linking table
migration_005e →  gig_contacts linking table
migration_005f →  tracklist_gigs linking table
```

---

## Waves

### Wave 1: Go Models, DB Schema/Migrations, GigReader Interface

**Goal:** Establish the complete data layer — tables, Go models, repository layer, and the `GigReader` interface consumed by downstream phases.

#### Files to create

| File | Purpose |
|------|---------|
| `api/internal/platform/migrations/005_gigs.sql` | All gig/venue/contact tables |
| `api/internal/gig/model.go` | `Gig`, `GigCreate`, `GigUpdate`, status enums, `GigReader` interface |
| `api/internal/gig/repository.go` | CRUD repository (embedded `Repo` pattern) |
| `api/internal/gig/service.go` | Business logic layer |
| `api/internal/gig/handler.go` | chi router wiring |
| `api/internal/venue/model.go` | `Venue`, `VenueCreate`, `VenueUpdate` |
| `api/internal/venue/repository.go` | Venue CRUD repository |
| `api/internal/venue/service.go` | Venue business logic |
| `api/internal/venue/handler.go` | Venue routes (`/api/v1/venues/*`) |
| `api/internal/contact/model.go` | `Contact`, `ContactCreate`, `ContactUpdate`, type enum |
| `api/internal/contact/repository.go` | Contact CRUD repository |
| `api/internal/contact/service.go` | Contact business logic |
| `api/internal/contact/handler.go` | Contact routes (`/api/v1/contacts/*`) |

#### Files to modify

| File | Change |
|------|--------|
| `api/internal/router.go` (or `cmd/server/main.go`) | Register gig, venue, contact route blocks |

#### Approach / Strategy

- **Gig status enum:** `inquiry`, `confirmed`, `advanced`, `played`, `cancelled`
- **Payment status enum:** `unpaid`, `deposit_paid`, `paid`, `overdue`, `waived`
- **Workflow rule:** `cancelled` is terminal; all other transitions allowed in both directions
- **Soft deletion:** `deleted_at` timestamp on all three tables
- **UUID primary keys** on all tables
- **GigReader interface** defined in `model.go` as a `//go:generate` interface; concrete implementation in `service.go` satisfies it; downstream phases import `klubhub.internal.gig.GigReader`
- **Embedded repo pattern:** `gig_repo.go` embeds a shared `repo.go` with `CRUD` methods using `sqlx`
- **Venue/Gig linking:** `gig_venues` join table with `gig_id`, `venue_id`, `is_primary`; same pattern for `gig_contacts`
- **Tracklist linking:** `tracklist_gigs` join table with `tracklist_id`, `gig_id`; GigReader exposes `Tracklists() []tracklist.Tracklist`

#### Dependencies on prior waves

- None (Phase 0 infrastructure only — db connection, router, migrations harness)

#### Verification

```bash
# Run migrations
make migrate-up
# Smoke: list gigs, create gig, create venue, create contact
curl localhost:8080/api/v1/gigs
curl localhost:8080/api/v1/venues?q=berg
curl localhost:8080/api/v1/contacts?q=promo
```

---

### Wave 2: API Endpoints (CRUD + Search), Venue/Contact Autocomplete

**Goal:** All REST API endpoints for gig CRUD, venue/contact CRUD, and autocomplete. Gig list filtering. Gig-venue / Gig-contact linking.

#### Files to create

| File | Purpose |
|------|---------|
| `api/internal/gig/handler.go` | Full REST handler: `GET /`, `POST /`, `GET /:id`, `PUT /:id`, `DELETE /:id` |
| `api/internal/venue/handler.go` | Venue CRUD + `GET /autocomplete?q=` |
| `api/internal/contact/handler.go` | Contact CRUD + `GET /autocomplete?q=` |
| `apps/dj/server/api/v1/gigs/index.get.ts` | Nuxt API proxy: list gigs |
| `apps/dj/server/api/v1/gigs/index.post.ts` | Nuxt API proxy: create gig |
| `apps/dj/server/api/v1/gigs/[id].get.ts` | Nuxt API proxy: get gig |
| `apps/dj/server/api/v1/gigs/[id].put.ts` | Nuxt API proxy: update gig |
| `apps/dj/server/api/v1/gigs/[id].delete.ts` | Nuxt API proxy: delete gig |
| `apps/dj/server/api/v1/gigs/autocomplete.get.ts` | Gig autocomplete for "Copy from Previous Gig" |
| `apps/dj/server/api/v1/venues/autocomplete.get.ts` | Venue autocomplete (≥2 char trigger) |
| `apps/dj/server/api/v1/venues/index.get.ts` | List venues |
| `apps/dj/server/api/v1/venues/index.post.ts` | Create venue |
| `apps/dj/server/api/v1/venues/[id].get.ts` | Get venue |
| `apps/dj/server/api/v1/venues/[id].put.ts` | Update venue |
| `apps/dj/server/api/v1/contacts/autocomplete.get.ts` | Contact autocomplete (≥2 char trigger) |
| `apps/dj/server/api/v1/contacts/index.get.ts` | List contacts |
| `apps/dj/server/api/v1/contacts/index.post.ts` | Create contact |
| `apps/dj/server/api/v1/contacts/[id].get.ts` | Get contact |
| `apps/dj/server/api/v1/contacts/[id].put.ts` | Update contact |
| `apps/dj/server/api/v1/gigs/[id]/link-venue.post.ts` | Link venue to gig |
| `apps/dj/server/api/v1/gigs/[id]/link-contact.post.ts` | Link contact to gig |

#### Files to modify

| File | Change |
|------|--------|
| `nuxt.config.ts` | Add proxy rewrite rules for `/api/v1/{gigs,venues,contacts}/*` if not already present |
| `apps/dj/server/api/v1/gigs/index.get.ts` | Wire to `GET /api/v1/gigs?venue=&city=&status=&fee_min=&fee_max=&from=&to=` filter params |

#### Approach / Strategy

- **Gig CRUD** — Standard REST: `GET /api/v1/gigs` (list + filter), `POST /api/v1/gigs` (create), `GET /:id`, `PUT /:id`, `DELETE /:id` (soft delete)
- **Gig filters (GIG-07):** `status`, `venue`, `city`, `fee_min`/`fee_max`, `from`/`to` date range; all via query params; handled in repository layer
- **Gig list response shape:** paginated `[]Gig` with total count header
- **Autocomplete (CONT-04, CONT-06):** `GET /api/v1/venues/autocomplete?q=berghain&limit=5` and `GET /api/v1/contacts/autocomplete?q=smith&limit=5`; minimum 2 chars enforced client-side; `ILIKE %q%` with `LIMIT` in SQL; results include `id`, `name`, `city`, `country`
- **Gig-venue linking (GIG-09):** `POST /api/v1/gigs/:id/link-venue` accepts `{venue_id, is_primary}`; auto-populates `venue_name`, `city`, `country` on the gig record; fields remain editable
- **Gig-contact linking (GIG-10):** `POST /api/v1/gigs/:id/link-contact` accepts `{contact_id, role}` (promoter/agent); auto-populates promoter fields; fields remain editable
- **Copy from Previous Gig (GIG-11):** `GET /api/v1/gigs/autocomplete?q=tresor` returns gigs for dropdown; selecting one populates linked venue + contact fields
- **Gig-tracklist linking (GIG-05):** `POST /api/v1/gigs/:id/link-tracklist` / `DELETE /api/v1/gigs/:id/link-tracklist/:tracklist_id` — repository only; UI in Wave 3

#### Dependencies on prior waves

- Wave 1 (models, migrations, GigReader must exist)

#### Verification

```bash
# Create a gig
curl -X POST localhost:8080/api/v1/gigs \
  -H "Content-Type: application/json" \
  -d '{"date":"2026-05-15","venue":"Berghain","city":"Berlin","country":"DE","event_name":"Saturday Night","fee":2000,"currency":"EUR"}'

# List with filters
curl "localhost:8080/api/v1/gigs?status=confirmed&from=2026-01-01&to=2026-12-31"

# Venue autocomplete
curl "localhost:8080/api/v1/venues/autocomplete?q=berg"

# Link venue
curl -X POST localhost:8080/api/v1/gigs/{id}/link-venue \
  -H "Content-Type: application/json" \
  -d '{"venue_id":"{venue_uuid}"}'
```

---

### Wave 3: Vue UI (List View, Gig Form, Status Workflow)

**Goal:** Replace mock data in `gigs.vue` with live API, add gig form dialog, calendar/list toggle, venue/contact autocomplete in forms, "Copy from Previous Gig", status/payment workflow UI.

#### Files to create

| File | Purpose |
|------|---------|
| `apps/dj/app/stores/gig.ts` | `useGigStore` Pinia store: gig list state, CRUD actions, filters |
| `apps/dj/app/stores/venue.ts` | `useVenueStore` Pinia store: venue list, autocomplete |
| `apps/dj/app/stores/contact.ts` | `useContactStore` Pinia store: contact list, autocomplete |
| `apps/dj/app/components/gig/GigStatsCards.vue` | UPCOMING / AVG FEE / YTD EARNED glass cards |
| `apps/dj/app/components/gig/GigListRow.vue` | Single gig row with accent bar, date block, badge |
| `apps/dj/app/components/gig/GigCalendarView.vue` | Monthly calendar grid with colored dot indicators (placeholder panel) |
| `apps/dj/app/components/gig/GigFormDialog.vue` | Dialog/sheet: create/edit gig form with all fields |
| `apps/dj/app/components/gig/GigStatusBadge.vue` | Status badge component (confirmed/pending/past/draft/archived) |
| `apps/dj/app/components/gig/VenueAutocomplete.vue` | Venue search + select with auto-populate |
| `apps/dj/app/components/gig/ContactAutocomplete.vue` | Contact search + select with auto-populate |
| `apps/dj/app/components/gig/CopyFromPreviousGig.vue` | Dropdown to select prior gig and populate fields |

#### Files to modify

| File | Change |
|------|--------|
| `apps/dj/app/pages/gigs.vue` | Replace mock data with `useGigStore`, add list/calendar toggle, ADD GIG button opens dialog, row click opens gig detail/edit |
| `apps/dj/app/components/TheNav.vue` | Ensure Gigs nav item active state (already present) |
| `apps/dj/app/components/TheBottomNav.vue` | Ensure Gigs nav item (already present) |
| `apps/dj/app/layouts/default.vue` | Ensure gigs route registered |

#### Approach / Strategy

- **`useGigStore`** — State: `gigs[]`, `loading`, `filters{}`; Actions: `fetchGigs()`, `createGig()`, `updateGig()`, `deleteGig()`, `fetchVenuesAutocomplete()`, `fetchContactsAutocomplete()`, `linkVenue()`, `linkContact()`
- **Gig list view** — `gigs.vue` iterates `gigStore.gigs`; stats computed from filtered gigs (upcoming = status not played/cancelled, YTD = sum of played gigs fees by currency); row click opens `GigFormDialog` in edit mode
- **Gig form dialog** — `GigFormDialog` component with: date picker, venue autocomplete (triggers at ≥2 chars), city/country (auto-filled from venue, editable), event name, promoter contact autocomplete, fee + currency dropdown (ISO 4217), notes textarea, set length field
- **Copy from Previous Gig dropdown** — in gig form; calls `GET /api/v1/gigs/autocomplete?q=`; on selection, calls `GET /api/v1/gigs/:id` to get linked venue + contact, auto-populates form fields
- **Status workflow UI** — Status select/dropdown in edit gig form; payment status select; confirm dialog on terminal `cancelled` transition; badge colors match CSS (`accent-bar-ready`=confirmed, `accent-bar-draft`=pending/inquiry, `accent-bar-archived`=played/cancelled)
- **Calendar/list toggle** — Tabs or toggle button in page header switching between `GigListView` and `GigCalendarView`; calendar view shows monthly grid with colored dot per day (cyan=confirmed, magenta=pending, gray=played)
- **Autocomplete UX** — Debounced 300ms after ≥2 chars; results dropdown with name + city; "Create new venue/contact" option at bottom of results
- **iCal export button** — "EXPORT ICAL" ghost-border button in page header; on click copies subscription URL to clipboard and shows toast confirmation (actual ical generation in Wave 4)

#### Dependencies on prior waves

- Wave 1 (GigReader interface not needed for UI, but models must exist)
- Wave 2 (all API endpoints must be wired to `apps/dj/server/api/v1/`)

#### Verification

- [ ] Page loads with live data from API
- [ ] ADD GIG button opens form dialog
- [ ] Venue autocomplete returns results at ≥2 chars
- [ ] Creating a gig appears in the list immediately
- [ ] Editing a gig and changing status updates the badge
- [ ] List/calendar toggle works
- [ ] Copy from Previous Gig populates promoter fields
- [ ] Stats cards reflect actual data

---

### Wave 4: iCal Feed, Booking PDF Generation

**Goal:** RFC 5545 iCal feed endpoint, booking confirmation PDF generation and download.

#### Files to create

| File | Purpose |
|------|---------|
| `api/internal/gig/ical.go` | iCal feed generation: RFC 5545 compliant `VCALENDAR` with `VEVENT` per gig |
| `api/internal/gig/ical_test.go` | Unit tests for iCal generation |
| `api/internal/gig/pdf.go` | Booking confirmation PDF generator using `gofpdf` |
| `api/internal/gig/pdf_test.go` | Unit tests for PDF content |
| `api/internal/gig/handler.go` | Add `GET /calendar.ics` handler; add `GET /:id/pdf` handler |
| `api/internal/gig/service.go` | Add `GenerateICal()` and `GenerateBookingPDF()` methods |
| `apps/dj/server/api/v1/gigs/calendar.ics.get.ts` | Nuxt proxy: `GET /api/v1/gigs/calendar.ics` → Go backend |
| `apps/dj/server/api/v1/gigs/[id]/pdf.get.ts` | Nuxt proxy: `GET /api/v1/gigs/:id/pdf` → Go backend |
| `apps/dj/app/components/gig/GigActions.vue` | Row actions: iCal export button, PDF download button |

#### Files to modify

| File | Change |
|------|--------|
| `apps/dj/app/pages/gigs.vue` | Add iCal export button near page header; wire row click to open gig detail panel |
| `apps/dj/app/components/gig/GigFormDialog.vue` | Add "DOWNLOAD CONFIRMATION" button visible when gig status is `confirmed` |
| `apps/dj/app/stores/gig.ts` | Add `generateICalUrl()` helper returning full URL with `?secret={ICAL_SECRET}` |

#### Approach / Strategy

- **iCal feed (`GET /api/v1/gigs/calendar.ics`):**
  - RFC 5545 `VCALENDAR` with `VEVENT` per non-cancelled gig
  - Protected by `?secret={ICAL_SECRET}` query param; `ICAL_SECRET` env var compared via `crypto/sha256` constant-time compare
  - Each `VEVENT`: `UID=gig.UUID`, `DTSTART=gig.Date + T gig.StartTime if set`, `SUMMARY=gig.EventName || gig.Venue`, `LOCATION=gig.Venue, gig.City, gig.Country`, `DESCRIPTION=gig.Fee formatted + currency + promoter email (omit if empty)`
  - No authentication required beyond shared secret

- **Booking confirmation PDF (`GET /api/v1/gigs/:id/pdf`):**
  - Available only when gig status is `confirmed` (return 400 otherwise)
  - Uses `gofpdf` (same library as EPK PDF); reference `api/internal/epk/pdf.go` for pattern
  - Content: DJ name (from settings), venue name + city, event date, fee + currency, set length, technical contact name + email
  - PDF stored in MinIO (`gig-exports/{year}/{gig_id}.pdf`); if already exists, return existing path
  - Download via `storage.Client.GetObject` presigned URL or direct redirect

- **iCal download UX:** Button in page header copies subscription URL to clipboard (includes `?secret=` param); toast shows "iCal feed URL copied!"

#### Dependencies on prior waves

- Wave 1 (Gig model must exist)
- Wave 2 (Gig CRUD API must exist to fetch gigs for iCal)

#### Verification

```bash
# iCal feed
curl "localhost:8080/api/v1/gigs/calendar.ics?secret=testsecret" -o calendar.ics
# Open in Apple Calendar or Google Calendar — should render correct events

# PDF generation
curl "localhost:8080/api/v1/gigs/{id}/pdf?secret=testsecret" -o confirmation.pdf
# PDF opens in browser/pdf reader
```

---

### Wave 5: Linking to Tracklists (Phase 1)

**Goal:** Bidirectional linking between gigs and tracklists. "Link to Gig" button in tracklist detail view. Gig list shows linked tracklists. "Copy from Previous Gig" uses tracklist data.

#### Files to create

| File | Purpose |
|------|---------|
| `api/internal/gig/handler.go` | Add `POST /:id/tracklists/:tracklist_id` and `DELETE /:id/tracklists/:tracklist_id` endpoints |
| `api/internal/gig/service.go` | Add `LinkTracklist()`, `UnlinkTracklist()`, `GetTracklists()` methods |
| `api/internal/gig/repository.go` | Add `LinkTracklist()`, `UnlinkTracklist()` to repo |
| `apps/dj/server/api/v1/gigs/[id]/tracklists/[tracklistId].post.ts` | Nuxt proxy |
| `apps/dj/server/api/v1/gigs/[id]/tracklists/[tracklistId].delete.ts` | Nuxt proxy |
| `apps/dj/app/components/tracklist/TracklistGigLinker.vue` | "Link to Gig" dropdown/modal in tracklist detail view |

#### Files to modify

| File | Change |
|------|--------|
| `apps/dj/app/components/tracklist/TracklistDetail.vue` (or existing tracklist page) | Add "LINK TO GIG" button; opens `TracklistGigLinker` component |
| `apps/dj/app/stores/tracklist.ts` | Add `gigs[]` state and `fetchLinkedGig()` action if needed |
| `apps/dj/app/pages/gigs.vue` | Gig row shows linked tracklist indicator (icon badge) |
| `apps/dj/app/components/gig/GigFormDialog.vue` | Gig form shows linked tracklists; allows linking/unlinking |

#### Approach / Strategy

- **Gig-tracklist linking (GIG-05):** `tracklist_gigs` join table (`tracklist_id`, `gig_id`) already created in Wave 1 migration
- **"Link to Gig" in tracklist view:** In tracklist detail/page, add "LINK TO GIG" button that opens a searchable gig selector (using `GET /api/v1/gigs/autocomplete`); on selection, calls `POST /api/v1/gigs/:id/tracklists/:tracklistId`
- **Gig list shows linked tracklists:** Gig row has a small tracklist icon badge when linked; hovering/tooltip shows tracklist title
- **Gig form shows linked tracklists:** In `GigFormDialog`, a "LINKED TRACKLISTS" section lists linked tracklists with unlink action
- **Copy from Previous Gig (GIG-11):** Wave 2 endpoint already returns gig data with linked venue/contact; Wave 3 UI populates from selection; Wave 5 extends to also expose linked tracklists for reference

#### Dependencies on prior waves

- Wave 1 (linking table migration, GigReader with Tracklists method)
- Wave 2 (gig CRUD + link endpoints)
- Wave 3 (gig form UI)

#### Verification

- [ ] Tracklist detail page has "Link to Gig" button
- [ ] Selecting a gig from dropdown links it
- [ ] Gig row in gigs list shows tracklist badge when linked
- [ ] Gig form shows linked tracklists section

---

## Verification Commands

After each wave, run the smoke commands documented in that wave. After Wave 5:

```bash
# Full smoke
curl localhost:8080/api/v1/gigs | jq length          # list gigs
curl localhost:8080/api/v1/gigs?status=confirmed     # filter
curl "localhost:8080/api/v1/venues/autocomplete?q=berg"  # venue autocomplete
curl "localhost:8080/api/v1/contacts/autocomplete?q=pro"  # contact autocomplete
curl localhost:8080/api/v1/gigs/calendar.ics?secret=$ICAL_SECRET -o /tmp/test.ics
file /tmp/test.ics                                    # should be iCalendar
curl localhost:8080/api/v1/gigs/{id}/pdf?secret=$ICAL_SECRET -o /tmp/test.pdf
file /tmp/test.pdf                                    # should be PDF
```

---

## Non-Blocking Backlog (Deferred)

- **Calendar view detail** — monthly grid detail beyond placeholder; specific click-day interaction TBD
- **Bandsintown sync (INT-01)** — Phase 4.8; outbound event push on gig confirmed
- **Rider templates (RIDER-01..05)** — Phase 4.5; attached when gig transitions to `advanced`
- **Finance auto-income** — Phase 5 handles auto-creation when payment_status → paid
