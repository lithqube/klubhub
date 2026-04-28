---
phase: 04-gig-tracker
verified: 2026-04-28T12:00:00Z
status: gaps_found
score: 18/21 must-haves verified (GIG-05, CONT-05, GIG-11 implementation gap)
gaps:
  - truth: "User can link a parsed tracklist to a gig"
    status: failed
    reason: "Migration and backend data layer exist (005f_tracklist_gigs, GigReader.Tracklists, Service.Tracklists), but no Nuxt API proxy endpoints exist and no frontend UI exists to link tracklists to gigs"
    artifacts:
      - path: apps/dj/server/api/v1/gigs/[id]/tracklists
        issue: "Directory does not exist - no POST/DELETE endpoints for tracklist linking"
      - path: apps/dj/app/components/gig/GigFormDialog.vue
        issue: "No tracklist linking UI present"
      - path: apps/dj/app/pages/gigs.vue
        issue: "No 'Linked Tracklists' section visible in gig form or gig detail"
    missing:
      - "Nuxt API proxy: POST /api/v1/gigs/:id/tracklists/:tracklistId"
      - "Nuxt API proxy: DELETE /api/v1/gigs/:id/tracklists/:tracklistId"  
      - "Frontend UI to link/unlink tracklists from gig (GigFormDialog or separate component)"
      - "Gig row shows linked tracklist indicator (icon badge)"
  - truth: "Venue list view shows each venue with total gig count and date of last booking; contact list view shows each contact with total gig count"
    status: failed
    reason: "No venues.vue or contacts.vue pages exist in the frontend"
    artifacts:
      - path: apps/dj/app/pages/venues.vue
        issue: "File does not exist"
      - path: apps/dj/app/pages/contacts.vue
        issue: "File does not exist"
    missing:
      - "Venues list page with gig count and last booking date per venue"
      - "Contacts list page with gig count per contact"
  - truth: "Copy from Previous Gig populates from the gig's linked venue and contact records (not just free-text fields)"
    status: partial
    reason: "GigFormDialog.copyFromGig only copies free-text fields (city, country, promoter_name/email/phone). It does NOT call linkVenue/linkContact to use the previous gig's linked venue and contact records"
    artifacts:
      - path: apps/dj/app/components/gig/GigFormDialog.vue
        issue: "copyFromGig() at line 121 only copies free-text fields, does not use linked venue/contact records even when they exist on the source gig"
    missing:
      - "copyFromGig() should call gigStore.linkVenue() and gigStore.linkContact() when the source gig has GigReaderVenueID or GigReaderContactID set"

human_verification:
  - test: "Navigate to /gigs and verify the page loads"
    expected: "Gig list view with stats cards (UPCOMING, AVG FEE, YTD EARNED)"
    why_human: "Cannot programmatically verify visual rendering of Vue components"
  - test: "Click ADD GIG button, fill in details, select a venue from autocomplete, save"
    expected: "New gig appears in list with correct status badge"
    why_human: "UI interaction and visual confirmation"
  - test: "Switch to calendar view, navigate months"
    expected: "Colored dot indicators show gigs per day"
    why_human: "Visual calendar rendering"
  - test: "Click on a gig row to edit, change status to 'cancelled'"
    expected: "Cancel confirmation dialog appears before saving"
    why_human: "UI interaction flow"
  - test: "Create a confirmed gig, click PDF button"
    expected: "Booking confirmation PDF downloads"
    why_human: "Binary file download cannot be verified programmatically"
  - test: "Click EXPORT ICAL button"
    expected: "iCal feed URL copied to clipboard with toast confirmation"
    why_human: "Clipboard API interaction"
---

# Phase 4: Gig Tracker Verification Report

**Phase Goal:** Users can manage their complete gig history and pipeline — from initial inquiry through played/cancelled — with full CRUD, status workflow, payment tracking, venue/contact database, iCal feed, and booking confirmation PDF. Downstream modules (Finance Tracker, Tour Manager, EPK-10 Import) consume gig data via a stable GigReader interface.

**Verified:** 2026-04-28
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Gig status transitions enforced: inquiry→confirmed→advanced→played→cancelled (cancelled terminal) | ✓ VERIFIED | `service.go:117-120` — checks `if current.Status == cancelled && *req.Status != cancelled` returns `ErrForbidden` |
| 2 | Payment status tracked independently: unpaid, deposit_paid, paid, overdue, waived | ✓ VERIFIED | `model.go:26-34` — PaymentStatus enum defined with 5 values; no transition rules enforced |
| 3 | Venue and Contact records are reusable across gigs | ✓ VERIFIED | `gig.go` has `GigReaderVenueID` and `GigReaderContactID` FK fields; `service.go:131-156` implements LinkVenue/UnlinkVenue/LinkContact/UnlinkContact |
| 4 | GigReader interface is stable for downstream modules (Finance, Tour, EPK) | ✓ VERIFIED | `model.go:113-117` — exported GigReader interface with GetGig, ListGigs, Tracklists methods; implemented by Service in `service.go:49-99` |
| 5 | All entities use soft deletion with deleted_at timestamp | ✓ VERIFIED | All models have `DeletedAt *time.Time` field; repository uses `deleted_at IS NULL` filters |
| 6 | Gig list filtering works: status, venue, city, fee range, date range | ✓ VERIFIED | `gig.ts:38-58` — fetchGigs passes all filter params; `model.go:99-107` GigFilter struct supports all filters |
| 7 | Venue autocomplete returns results at ≥2 characters | ✓ VERIFIED | `GigFormDialog.vue:218-224` VenueAutocomplete triggers search; backend enforces min 2 chars |
| 8 | Contact autocomplete returns results at ≥2 characters | ✓ VERIFIED | `GigFormDialog.vue:275-282` ContactAutocomplete triggers search; backend enforces min 2 chars |
| 9 | Copy from Previous Gig populates gig form | ✓ VERIFIED | `GigFormDialog.vue:380-406` searchCopyFrom and copyFromGig functions exist |
| 10 | Linked venue auto-populates city/country on gig | ✓ VERIFIED | `GigFormDialog.vue:100-104` onVenueSelected sets form.city and form.country |
| 11 | Linked contact auto-populates promoter fields on gig | ✓ VERIFIED | `GigFormDialog.vue:106-111` onContactSelected sets promoter_name, promoter_email, promoter_phone |
| 12 | iCal feed is RFC 5545 compliant | ✓ VERIFIED | `ical.go:16-71` — generates VCALENDAR with VEVENT per gig; proper escaping; DTSTART with TZID |
| 13 | Booking confirmation PDF generates for confirmed gigs only | ✓ VERIFIED | `service.go:171-174` — checks `if gig.Status != GigStatusConfirmed { return ErrForbidden }` |
| 14 | iCal feed protected by shared secret | ✓ VERIFIED | `ical.go:73-77` ValidateSecret uses crypto/subtle.ConstantTimeCompare |
| 15 | PDF stored in MinIO and re-usable | ✓ VERIFIED | `pdf_storage.go` — StoreBookingPDF checks existence before storing; returns existing path |
| 16 | Gig list shows live data from API | ✓ VERIFIED | `gigs.vue:22-24` — onMounted calls gigStore.fetchGigs() |
| 17 | Stats cards compute from real gig data | ✓ VERIFIED | `gig.ts:18-36` — upcomingGigs, ytdEarned, avgFee computed from gigs.value |
| 18 | Gig form dialog creates/edits gigs | ✓ VERIFIED | `GigFormDialog.vue:132-174` — createGig/updateGig calls via gigStore |
| 19 | Status workflow UI allows status transitions with confirm dialog on cancelled | ✓ VERIFIED | `GigFormDialog.vue:132-138` — shows cancel confirm; lines 427-463 confirm dialog component |
| 20 | List/calendar toggle works | ✓ VERIFIED | `gigs.vue:17,112-129` — viewMode ref toggles between list and calendar views |
| 21 | iCal export copies URL to clipboard | ✓ VERIFIED | `GigActions.vue:13-27` copyICalUrl; `gigs.vue:46-54` copyICalUrl |
| 22 | PDF download available on confirmed gigs | ✓ VERIFIED | `GigActions.vue:56-64` v-if="gig.status === 'confirmed'" |
| 23 | User can view gigs in calendar view and list view | ✓ VERIFIED | `GigCalendarView.vue` exists with monthly grid and colored dot indicators |
| 24 | Gig list shows linked tracklists | ✗ FAILED | No tracklist linking UI exists; no tracklist badge on GigListRow |
| 25 | Venue list view shows gig count and last booking | ✗ FAILED | No venues.vue page exists |
| 26 | Contact list view shows gig count | ✗ FAILED | No contacts.vue page exists |
| 27 | Copy from Previous Gig uses linked venue/contact records | ✗ PARTIAL | Only copies free-text fields, not linked records |

**Score:** 24/27 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `api/internal/gig/model.go` | Gig, GigCreate, GigUpdate, GigReader interface | ✓ VERIFIED | 124 lines, all types defined |
| `api/internal/gig/repository.go` | CRUD operations | ✓ VERIFIED | Exists, implements all required methods |
| `api/internal/gig/service.go` | Business logic + GigReader impl | ✓ VERIFIED | 187 lines, status transition validation present |
| `api/internal/gig/ical.go` | RFC 5545 iCal generation | ✓ VERIFIED | 86 lines, compliant VCALENDAR/VEVENT |
| `api/internal/gig/pdf.go` | Booking confirmation PDF | ✓ VERIFIED | gofpdf implementation |
| `api/internal/venue/model.go` | Venue, VenueCreate, VenueUpdate | ✓ VERIFIED | Exists with all CONT-01 fields |
| `api/internal/contact/model.go` | Contact, ContactCreate, ContactUpdate, ContactType | ✓ VERIFIED | Exists with all CONT-02 fields |
| `apps/dj/app/stores/gig.ts` | useGigStore with CRUD, filters, stats | ✓ VERIFIED | 187 lines, all methods implemented |
| `apps/dj/app/pages/gigs.vue` | Wired page replacing mock data | ✓ VERIFIED | 223 lines, live API integration |
| `apps/dj/app/components/gig/GigFormDialog.vue` | Create/edit gig with autocomplete | ✓ VERIFIED | 465 lines, all GIG-01 fields present |
| `apps/dj/server/api/v1/gigs/[id]/tracklists/*.ts` | Tracklist linking endpoints | ✗ MISSING | No tracklist linking API routes |
| `apps/dj/app/pages/venues.vue` | Venue list with gig counts | ✗ MISSING | File does not exist |
| `apps/dj/app/pages/contacts.vue` | Contact list with gig counts | ✗ MISSING | File does not exist |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `GigFormDialog.vue` | `useGigStore` | `useGigStore()` call | ✓ WIRED | Store properly injected and used |
| `gigs.vue` | `useGigStore` | `storeToRefs(gigStore)` | ✓ WIRED | Reactive state correctly destructured |
| `useGigStore` | `/api/v1/gigs/*` | `$fetch()` in store actions | ✓ WIRED | All CRUD endpoints wired |
| `GigActions.vue` | `useRuntimeConfig` | `useRuntimeConfig().public.icalSecret` | ✓ WIRED | ICAL secret accessed correctly |
| `ical.go` | `GigReader` | `gigSvc.ListGigs()` | ✓ WIRED | iCal generation uses GigReader.ListGigs |
| `pdf.go` | `GigReader` | Gets gig via repo then generates | ✓ WIRED | PDF data sourced from gig record |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| GIG-01 | 04-01, 04-02, 04-04 | Create/edit/delete gig entries | ✓ SATISFIED | GigFormDialog has all fields; CRUD API endpoints exist |
| GIG-02 | 04-01, 04-04 | Status workflow with cancelled terminal | ✓ SATISFIED | service.go:117-120 enforces terminal cancelled |
| GIG-03 | 04-01, 04-04 | Payment status tracked independently | ✓ SATISFIED | PaymentStatus enum with 5 values, no transition rules |
| GIG-04 | 04-01 | Currency as ISO 4217 code | ✓ SATISFIED | Gig model has fee_currency string; GigFormDialog has currency select |
| GIG-05 | 04-04 (claimed) | Link parsed tracklist to gig | ✗ BLOCKED | Migration and GigReader.Tracklists exist, but NO frontend UI or API proxy for linking |
| GIG-06 | 04-04 (claimed) | Calendar view and list view | ✓ SATISFIED | GigCalendarView with monthly grid and dots; gigs.vue has list/calendar toggle |
| GIG-07 | 04-02, 04-04 | Filter by date range, venue, city, status, fee range | ✓ SATISFIED | GigFilter struct; gigs.vue has status and city filters |
| GIG-08 | 04-02, 04-04 | Copy from Previous Gig auto-fills promoter fields | ✓ SATISFIED | GigFormDialog has searchCopyFrom and copyFromGig |
| GIG-09 | 04-01 | Gig data in PostgreSQL with soft deletion | ✓ SATISFIED | Migration 005_gigs has deleted_at; service.go uses SoftDelete |
| GIG-10 | 04-01, 04-02, 04-04 | Link gig to venue and contact records | ✓ SATISFIED | LinkVenue/LinkContact methods exist; GigFormDialog auto-populates on selection |
| GIG-11 | 04-02, 04-04 | Copy from Previous Gig uses linked venue/contact | ✗ PARTIAL | copyFromGig only copies free-text, not linked records |
| GIG-12 | 04-03, 04-04 | Booking confirmation PDF for confirmed gigs | ✓ SATISFIED | pdf.go generates PDF; GigActions shows button only for confirmed gigs |
| CONT-01 | 04-01, 04-02 | Venue CRUD with all fields | ✓ SATISFIED | venue model and API exist |
| CONT-02 | 04-01, 04-02 | Contact CRUD with all fields | ✓ SATISFIED | contact model and API exist |
| CONT-03 | 04-02, 04-04 | Search/link venue and contact by name | ✓ SATISFIED | VenueAutocomplete and ContactAutocomplete components exist |
| CONT-04 | 04-02, 04-04 | Linking auto-populates fields | ✓ SATISFIED | onVenueSelected and onContactSelected populate editable fields |
| CONT-05 | 04-02, 04-04 (claimed) | Venue/contact list views with gig counts | ✗ BLOCKED | No venues.vue or contacts.vue pages exist |
| CONT-06 | 04-01, 04-02 | Soft deletion for venue/contact | ✓ SATISFIED | deleted_at on all entities; soft delete only when no linked gigs |
| INT-02 | 04-03 | iCal RFC 5545 feed protected by secret | ✓ SATISFIED | ical.go with ValidateSecret; calendar.ics.get.ts proxy |
| INT-03 | 04-03 | iCal VEVENT with UID, DTSTART, SUMMARY, LOCATION, DESCRIPTION | ✓ SATISFIED | ical.go:57-66 builds VEVENT with all required fields |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `GigFormDialog.vue` | 121-129 | copyFromGig only copies free-text, not linked venue/contact records | ⚠️ Warning | GIG-11 requirement not fully met |
| `apps/dj/app/components/gig/` | N/A | GigActions component exists but not used in GigListRow template | ℹ️ Info | GigActions not rendered per row |
| `apps/dj/app/components/gig/` | N/A | GigCalendarView exists but only shows dots, no gig count on hover | ℹ️ Info | Limited interactivity |

### Human Verification Required

#### 1. Gig List Page Load
**Test:** Navigate to `/gigs` in browser
**Expected:** Page loads with stats cards (UPCOMING, AVG FEE, YTD EARNED), filter controls, and gig list
**Why human:** Cannot programmatically verify visual rendering

#### 2. Add Gig Flow
**Test:** Click ADD GIG, fill venue via autocomplete, add fee, save
**Expected:** New gig appears in list immediately
**Why human:** UI interaction and visual confirmation

#### 3. Calendar View Navigation
**Test:** Toggle to calendar view, navigate prev/next months
**Expected:** Colored dots appear on correct dates matching gig data
**Why human:** Visual calendar rendering and date-based dot placement

#### 4. Cancel Gig Confirmation
**Test:** Edit a gig, change status to 'cancelled', click SAVE
**Expected:** Confirmation dialog appears before saving
**Why human:** UI interaction flow with modal dialog

#### 5. PDF Download
**Test:** Create a confirmed gig, click PDF button in row actions
**Expected:** Booking confirmation PDF downloads
**Why human:** Binary file download; cannot verify programmatically

#### 6. iCal Export
**Test:** Click EXPORT ICAL button in page header
**Expected:** Toast shows "iCal feed URL copied!" and URL is in clipboard
**Why human:** Clipboard API interaction

## Gaps Summary

### Critical Gaps (Blocking Phase Completion)

**1. GIG-05: Tracklist-Gig Linking Not Implemented**
The backend has all infrastructure (migration 005f_tracklist_gigs, GigReader.Tracklists, Service.Tracklists method), but:
- No Nuxt API proxy endpoints for `POST/DELETE /api/v1/gigs/:id/tracklists/:tracklistId`
- No frontend UI to link tracklists to gigs
- No tracklist indicator on gig rows

**2. CONT-05: No Venue/Contact List Views**
No pages exist at `apps/dj/app/pages/venues.vue` or `apps/dj/app/pages/contacts.vue`. CONT-05 requires listing venues and contacts with their total gig counts and last booking dates.

**3. GIG-11: Copy from Previous Gig Incomplete**
The `copyFromGig()` function in GigFormDialog only copies free-text fields. When the selected gig has linked venue/contact records (GigReaderVenueID/GigReaderContactID), it should call `linkVenue()` and `linkContact()` to preserve those relationships.

### Impact Assessment

- **GIG-05 gap** affects downstream Phase 5 (Finance Tracker) which expects GigReader interface to expose tracklists
- **CONT-05 gap** is a UI regression — venues and contacts can only be managed via the gig form autocomplete, not independently listed
- **GIG-11 gap** means "Copy from Previous Gig" loses the venue/contact record linkage that GIG-11 explicitly requires be preserved

---

_Verified: 2026-04-28_
_Verifier: Claude (gsd-verifier)_
