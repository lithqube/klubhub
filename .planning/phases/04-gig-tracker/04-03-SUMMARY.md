---
phase: 04-gig-tracker
plan: 03
subsystem: api
tags: [ical, pdf, gig, calendar, booking-confirmation]

# Dependency graph
requires:
  - phase: [04-01]
    provides: Gig model, GigReader interface, GigService, Repository
provides:
  - RFC 5545 iCal feed endpoint (calendar.ics)
  - Booking confirmation PDF generation (gofpdf)
  - MinIO storage for booking PDFs (gig-exports bucket)
affects: [04-04 (frontend wiring), 05-finance-tracker (GigReader consumer)]

# Tech tracking
tech-stack:
  added: [gofpdf (codeberg/go-pdf/fpdf)]
  patterns:
    - RFC 5545 VCALENDAR/VEVENT generation with proper escaping
    - Constant-time secret comparison (crypto/subtle)
    - Storage adapter pattern for MinIO integration
    - PDFGenerator struct with section-writing helpers

key-files:
  created:
    - api/internal/gig/ical.go (RFC 5545 iCal generation)
    - api/internal/gig/ical_test.go (ValidateSecret + escapeICalText tests)
    - api/internal/gig/pdf.go (BookingPDFGenerator + GenerateBookingPDF)
    - api/internal/gig/pdf_test.go (PDF generation tests)
    - api/internal/gig/pdf_storage.go (StorageAdapter for MinIO)
    - apps/dj/server/api/v1/gigs/calendar.ics.get.ts (Nuxt proxy)
    - apps/dj/server/api/v1/gigs/[id]/pdf.get.ts (Nuxt proxy)
  modified:
    - api/internal/gig/handler.go (calendar.ics + pdf routes)
    - api/internal/gig/service.go (GenerateCalendar + GenerateBookingPDF methods)
    - api/cmd/api/main.go (gigStorage adapter wiring)

key-decisions:
  - "gofpdf used for booking PDF (same library as EPK PDF for consistency)"
  - "Storage adapter pattern bridges storage.Client (minio.PutObjectOptions) to PDFStorageClientIface (contentType string)"
  - "ICAL_SECRET env var for URL parameter validation via constant-time compare"
  - "PDF only available for confirmed gigs (400 returned otherwise)"
  - "VEVENT DTSTART uses Europe/Berlin timezone per RFC 5545"

patterns-established:
  - "StorageAdapter wraps MinIO/Garage client for service-layer use without minio import"
  - "Generator structs (BookingPDFGenerator) with Generate() method returning []byte"
  - "Section-writing helper methods for consistent PDF layout"

requirements-completed: [GIG-12, INT-02, INT-03]

# Metrics
duration: 45 min
completed: 2026-04-28
---

# Phase 4 Plan 3: iCal Feed + Booking Confirmation PDF Summary

**RFC 5545 iCal feed generation with gofpdf booking confirmation PDF, MinIO storage, and Nuxt proxy routes**

## Performance

- **Duration:** 45 min
- **Started:** 2026-04-28T08:54:24Z
- **Completed:** 2026-04-28T09:39:00Z
- **Tasks:** 4
- **Files modified:** 9

## Accomplishments
- RFC 5545 compliant iCal VCALENDAR generation with VEVENT per non-cancelled gig
- Constant-time secret validation via crypto/subtle.ConstantTimeCompare
- Booking confirmation PDF generation using gofpdf (same library as EPK PDF)
- MinIO storage adapter for booking PDF persistence
- Nuxt API proxy routes for calendar.ics and PDF download
- All unit tests passing for iCal and PDF generation

## Task Commits

Each task was committed atomically:

1. **Task 1: RFC 5545 iCal feed generation** - `8749cf2` (feat)
2. **Task 2: Booking confirmation PDF generation** - `17b962e` (feat)
3. **Task 3: Extend gig handler with calendar and PDF endpoints** - `c1cbd00` (feat)
4. **Task 4: Create Nuxt API proxy routes** - `8d9a43c` (feat)

**Plan metadata:** `4a8f0c1` (docs)

## Files Created/Modified

- `api/internal/gig/ical.go` - RFC 5545 iCal generation with GenerateCalendar, ValidateSecret, escapeICalText
- `api/internal/gig/ical_test.go` - Unit tests for ValidateSecret and escapeICalText
- `api/internal/gig/pdf.go` - BookingPDFGenerator struct with GenerateBookingPDF
- `api/internal/gig/pdf_test.go` - Unit tests for PDF generator
- `api/internal/gig/pdf_storage.go` - StorageAdapter for MinIO PDF storage
- `api/internal/gig/handler.go` - calendar.ics and pdf endpoint handlers added to Routes()
- `api/internal/gig/service.go` - GenerateCalendar and GenerateBookingPDF service methods
- `api/cmd/api/main.go` - gigStorage adapter wired into NewService call
- `apps/dj/server/api/v1/gigs/calendar.ics.get.ts` - Nuxt proxy for iCal endpoint
- `apps/dj/server/api/v1/gigs/[id]/pdf.get.ts` - Nuxt proxy for PDF download

## Decisions Made

- Used gofpdf (codeberg/go-pdf/fpdf) for booking PDF — same library as EPK PDF for consistency
- Storage adapter pattern bridges storage.Client (minio.PutObjectOptions) to PDFStorageClientIface (contentType string)
- ICAL_SECRET env var for URL parameter validation via constant-time compare (prevents timing attacks)
- PDF only available for confirmed gigs (400 returned for non-confirmed)
- VEVENT DTSTART uses Europe/Berlin timezone per RFC 5545

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- StatObject interface mismatch: storage.Client.StatObject returns (minio.ObjectInfo, error) but my simplified interface expected (bool, error) — resolved by creating separate StorageAdapter that doesn't use StatObject for existence check

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- iCal endpoint ready for 04-04 frontend wiring (Export button in gigs.vue)
- PDF endpoint ready for 04-04 frontend wiring (Download Confirmation button for confirmed gigs)
- GigReader interface already stable from 04-01 for downstream consumers

---
*Phase: 04-gig-tracker*
*Completed: 2026-04-28*
