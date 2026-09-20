# Phase 4: Gig Tracker — Wave Breakdown

**Phase:** 04-gig-tracker
**Waves:** 5
**Started:** 2026-04-27

---

## Wave Summary

| Wave | Focus | Key Deliverables |
|------|-------|------------------|
| 1 | Go models, DB schema, GigReader interface | 005_migrations.sql, gig/venue/contact packages, GigReader interface |
| 2 | API endpoints (CRUD + search), autocomplete | Full REST API, venue/contact autocomplete, gig-venue/contact linking |
| 3 | Vue UI (list view, gig form, status workflow) | useGigStore, GigFormDialog, autocomplete inputs, list/calendar toggle |
| 4 | iCal feed, booking PDF generation | RFC 5545 ical endpoint, gofpdf booking PDF, download buttons |
| 5 | Linking to tracklists (Phase 1) | Gig-tracklist join table, "Link to Gig" UI, tracklist badge on gig rows |

---

## Wave 1: Go Models, DB Schema/Migrations, GigReader Interface

**Files to create:**

```
api/internal/platform/migrations/005_gigs.sql
api/internal/gig/model.go
api/internal/gig/repository.go
api/internal/gig/service.go
api/internal/gig/handler.go
api/internal/venue/model.go
api/internal/venue/repository.go
api/internal/venue/service.go
api/internal/venue/handler.go
api/internal/contact/model.go
api/internal/contact/repository.go
api/internal/contact/service.go
api/internal/contact/handler.go
```

**Files to modify:**

```
api/internal/router.go (or cmd/server/main.go) — register route blocks
```

**Approach:**
- Five SQL migrations: `005_gigs`, `005b_venues`, `005c_contacts`, `005d_gig_venues`, `005e_gig_contacts`, `005f_tracklist_gigs`
- Gig status enum: `inquiry | confirmed | advanced | played | cancelled`; payment: `unpaid | deposit_paid | paid | overdue | waived`
- `GigReader` interface in `model.go` — exported, consumed by Finance (Phase 5), Tour Manager (Phase 7), EPK-10 Import
- Embedded `repo.go` CRUD pattern matching existing Phase 1/3 structure
- Soft deletion + UUID PKs on all tables

**Dependencies:** Phase 0 only

**Smoke:**
```bash
curl localhost:8080/api/v1/gigs
curl "localhost:8080/api/v1/venues?q=berg"
curl "localhost:8080/api/v1/contacts?q=promo"
```

---

## Wave 2: API Endpoints (CRUD + Search), Venue/Contact Autocomplete

**Files to create:**

```
api/internal/gig/handler.go (extend — full REST)
api/internal/venue/handler.go (extend — autocomplete)
api/internal/contact/handler.go (extend — autocomplete)
apps/dj/server/api/v1/gigs/index.{get,post}.ts
apps/dj/server/api/v1/gigs/[id].{get,put,delete}.ts
apps/dj/server/api/v1/gigs/autocomplete.get.ts
apps/dj/server/api/v1/venues/autocomplete.get.ts
apps/dj/server/api/v1/venues/index.{get,post}.ts
apps/dj/server/api/v1/venues/[id].{get,put}.ts
apps/dj/server/api/v1/contacts/autocomplete.get.ts
apps/dj/server/api/v1/contacts/index.{get,post}.ts
apps/dj/server/api/v1/contacts/[id].{get,put}.ts
apps/dj/server/api/v1/gigs/[id]/link-venue.post.ts
apps/dj/server/api/v1/gigs/[id]/link-contact.post.ts
```

**Files to modify:**

```
nuxt.config.ts — proxy rules for /api/v1/{gigs,venues,contacts}/*
apps/dj/server/api/v1/gigs/index.get.ts — filter query params
```

**Approach:**
- Gig CRUD with filters: `status`, `venue`, `city`, `fee_min`, `fee_max`, `from`, `to`
- Venue/contact autocomplete: `ILIKE %q%` + `LIMIT`, min 2 chars enforced client-side
- Gig-venue linking auto-populates city/country; gig-contact linking auto-populates promoter fields; all auto-populated fields remain editable
- "Copy from Previous Gig" dropdown via `GET /api/v1/gigs/autocomplete?q=`

**Dependencies:** Wave 1

**Smoke:**
```bash
curl -X POST localhost:8080/api/v1/gigs \
  -H "Content-Type: application/json" \
  -d '{"date":"2026-05-15","venue":"Berghain","city":"Berlin","country":"DE","fee":2000,"currency":"EUR"}'
curl "localhost:8080/api/v1/gigs?status=confirmed&from=2026-01-01"
curl "localhost:8080/api/v1/venues/autocomplete?q=berg"
curl -X POST localhost:8080/api/v1/gigs/{id}/link-venue \
  -H "Content-Type: application/json" -d '{"venue_id":"{uuid}"}'
```

---

## Wave 3: Vue UI (List View, Gig Form, Status Workflow)

**Files to create:**

```
apps/dj/app/stores/gig.ts
apps/dj/app/stores/venue.ts
apps/dj/app/stores/contact.ts
apps/dj/app/components/gig/GigStatsCards.vue
apps/dj/app/components/gig/GigListRow.vue
apps/dj/app/components/gig/GigCalendarView.vue
apps/dj/app/components/gig/GigFormDialog.vue
apps/dj/app/components/gig/GigStatusBadge.vue
apps/dj/app/components/gig/VenueAutocomplete.vue
apps/dj/app/components/gig/ContactAutocomplete.vue
apps/dj/app/components/gig/CopyFromPreviousGig.vue
```

**Files to modify:**

```
apps/dj/app/pages/gigs.vue — wire to useGigStore, add list/calendar toggle, ADD GIG button
apps/dj/app/components/TheNav.vue — active state
apps/dj/app/components/TheBottomNav.vue — active state
apps/dj/app/layouts/default.vue — route registration
```

**Approach:**
- `useGigStore`: `gigs[]`, `loading`, `filters{}`; actions: `fetchGigs()`, `createGig()`, `updateGig()`, `deleteGig()`, `linkVenue()`, `linkContact()`
- Stats cards computed from filtered gigs (upcoming count, avg fee, YTD earned per currency)
- Gig form dialog with: date picker, venue autocomplete, city/country (auto + editable), event name, promoter autocomplete, fee + currency, notes, set length
- Status workflow: dropdown in edit form; confirm dialog on `cancelled` transition
- List/calendar toggle via page header tabs — calendar shows monthly grid with colored dot indicators
- "EXPORT ICAL" ghost button placeholder (copies URL to clipboard; feed generation in Wave 4)
- Autocomplete: debounced 300ms, ≥2 chars, "Create new…" option at bottom of results

**Dependencies:** Wave 1 (models), Wave 2 (API endpoints)

**Smoke:**
- [ ] Live gigs list renders
- [ ] ADD GIG opens form with venue autocomplete
- [ ] Created gig appears in list immediately
- [ ] Status badge updates on edit
- [ ] List/calendar toggle works
- [ ] Copy from Previous Gig populates promoter fields

---

## Wave 4: iCal Feed, Booking PDF Generation

**Files to create:**

```
api/internal/gig/ical.go
api/internal/gig/ical_test.go
api/internal/gig/pdf.go
api/internal/gig/pdf_test.go
api/internal/gig/handler.go (extend — /calendar.ics and /:id/pdf)
api/internal/gig/service.go (extend — GenerateICal, GenerateBookingPDF)
apps/dj/server/api/v1/gigs/calendar.ics.get.ts
apps/dj/server/api/v1/gigs/[id]/pdf.get.ts
apps/dj/app/components/gig/GigActions.vue
```

**Files to modify:**

```
apps/dj/app/pages/gigs.vue — add EXPORT ICAL button
apps/dj/app/components/gig/GigFormDialog.vue — add DOWNLOAD CONFIRMATION button
apps/dj/app/stores/gig.ts — add generateICalUrl() helper
```

**Approach:**
- iCal: RFC 5545 `VCALENDAR` + `VEVENT` per non-cancelled gig; `?secret=` query param; `DTSTART`, `SUMMARY`, `LOCATION`, `DESCRIPTION` per event
- PDF: `gofpdf` (same as EPK); pre-populated: DJ name, venue + city, date, fee + currency, set length, tech contact; stored in MinIO at `gig-exports/{year}/{gig_id}.pdf`; return existing file if already generated
- iCal button: copies full subscription URL (including `?secret=`) to clipboard; toast confirmation
- PDF button: visible only on `confirmed` gigs; direct download

**Dependencies:** Wave 1 (Gig model), Wave 2 (Gig CRUD API)

**Smoke:**
```bash
curl "localhost:8080/api/v1/gigs/calendar.ics?secret=$ICAL_SECRET" -o /tmp/test.ics
file /tmp/test.ics  # iCalendar
curl "localhost:8080/api/v1/gigs/{id}/pdf?secret=$ICAL_SECRET" -o /tmp/test.pdf
file /tmp/test.pdf  # PDF document
```

---

## Wave 5: Linking to Tracklists (Phase 1)

**Files to create:**

```
api/internal/gig/handler.go (extend — POST/DELETE /:id/tracklists/:tracklistId)
api/internal/gig/service.go (extend — LinkTracklist, UnlinkTracklist, GetTracklists)
api/internal/gig/repository.go (extend — LinkTracklist, UnlinkTracklist)
apps/dj/server/api/v1/gigs/[id]/tracklists/[tracklistId].post.ts
apps/dj/server/api/v1/gigs/[id]/tracklists/[tracklistId].delete.ts
apps/dj/app/components/tracklist/TracklistGigLinker.vue
```

**Files to modify:**

```
apps/dj/app/components/tracklist/TracklistDetail.vue (or tracklist page) — "Link to Gig" button
apps/dj/app/stores/tracklist.ts — add gigs state if needed
apps/dj/app/pages/gigs.vue — tracklist icon badge on gig rows
apps/dj/app/components/gig/GigFormDialog.vue — linked tracklists section
```

**Approach:**
- `tracklist_gigs` join table already created in Wave 1 migration
- "Link to Gig" button in tracklist detail view opens searchable gig selector (uses `GET /api/v1/gigs/autocomplete`)
- Gig row shows tracklist icon badge when linked; hover tooltip shows tracklist title
- Gig form "LINKED TRACKLISTS" section lists linked tracklists with unlink action
- GigReader exposes `Tracklists() []tracklist.Tracklist` for downstream consumption

**Dependencies:** Wave 1 (linking table, GigReader), Wave 2 (link endpoints), Wave 3 (gig form UI)

**Smoke:**
- [ ] Tracklist detail page has "Link to Gig" button
- [ ] Selecting a gig from dropdown links it
- [ ] Gig row in list shows tracklist badge when linked
- [ ] Gig form shows linked tracklists section

---

## Wave Dependency Graph

```
Wave 1  ──────────────────────────────────────────────────────→ Wave 2
  │                                                                   │
  │                                                            Wave 3 ↓
  │                                                                   │
  │                              Wave 4 ←──────────────────────────────┘
  │                                  (needs Wave 1 models + Wave 2 API)
  │
  └──────────────────────────────────────────────────────────────────→ Wave 5
      (needs Wave 1 linking table + Wave 2 endpoints + Wave 3 UI)
```

**Wave 1** is the foundation — all subsequent waves depend on it.
**Wave 2** depends on Wave 1 (models, migrations).
**Wave 3** depends on Wave 2 (API endpoints wired).
**Wave 4** depends on Wave 1 + Wave 2 (GigReader + CRUD API).
**Wave 5** depends on Wave 1 + Wave 2 + Wave 3 (full linking table + API + form UI).

## Wave 5 Status: ✅ Complete (2026-09-20)

Tracklist ↔ gig bidirectional linking shipped on `feat/phase-4-full`. The `tracklist_gigs` join table (migration `005f`) is populated by `LinkTracklist` / `UnlinkTracklist` ops on the gig service, exposed via `POST /api/v1/gigs/{id}/tracklists/{tracklistId}` and `DELETE` variants, proxied by Nitro routes, and surfaced in the UI as:

- `TracklistGigLinker.vue` — embedded in `render/tracklist/[id].vue`, shows linked status buttons (JOIN/BOOKED/PAST) + "Link existing gig" autocomplete dialog
- `GigFormDialog.vue` — "Linked Tracklists" section in Edit gig form with unlink buttons + select row for unlinked past tracklists
- `GigListRow.vue` — `tracklist-badge` pill showing count when `g.tracklists?.length > 0`
- `useGigStore.linkTracklist` / `unlinkTracklist` — store actions hitting proxy routes
- `useTracklistStore.linkedGigs` — bidirectional `Record<string, string[]>` cache
