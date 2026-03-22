---
phase: 03-epk-press-kit-builder
plan: "02"
subsystem: api/epk
tags: [go, epk, service, pdf, fpdf, handler, chi, minio, markdown]

# Dependency graph
requires:
  - phase: 03-01
    provides: repoIface, EPKContent model, EPKExport model, UpsertEPKContentRequest
  - phase: 00-infrastructure
    provides: storage.Client, settings.Service, chi router pattern
provides:
  - service.go with 8 business-logic methods; storageIface + settingsServiceIface thin interfaces
  - StorageClientAdapter and settingsServiceAdapter for wiring real implementations
  - pdf.go with renderEPKPDF (white A4, 8 sections, accent colour) + renderMarkdown (6 constructs)
  - handler.go with 8 HTTP routes mounted at /api/v1/epk
  - router.go updated with epkHandler parameter
  - main.go wired with EPK module
affects:
  - 03-03-epk-frontend-store

# Tech tracking
tech-stack:
  added:
    - codeberg.org/go-pdf/fpdf v0.11.1 (PDF generation; maintained gofpdf fork)
  patterns:
    - Thin storageIface (contentType string) + adapter over storage.Client (minio.PutObjectOptions)
    - settingsServiceAdapter bridges settings.Service.GetOrCreate/Update to EPK interface
    - TDD: RED (failing tests) → GREEN (implementation) per-task
    - storage.Client.RemoveObject wrapper added for MinIO object deletion

key-files:
  created:
    - api/internal/epk/service.go
    - api/internal/epk/pdf.go
    - api/internal/epk/service_test.go
    - api/internal/epk/handler.go
    - api/internal/epk/handler_test.go
  modified:
    - api/internal/platform/http/router.go
    - api/cmd/api/main.go
    - api/internal/platform/storage/storage.go

key-decisions:
  - "Thin storageIface uses contentType string instead of minio.PutObjectOptions — keeps service layer free of minio import; bridged via StorageClientAdapter in service.go"
  - "settingsServiceAdapter maps GetOrCreate+Update (settings.Service) to GetSettings+UpdateSettings (EPK interface) — avoids changing settings package"
  - "RemoveObject added to storage.Client as a new wrapper — required by both photo delete and export delete; classified as Rule 2 (missing critical functionality)"
  - "renderInline in pdf.go strips inline Markdown to plain text for fpdf MultiCell — fpdf v0.11.1 has no per-run font-switch within a MultiCell call"
  - "Press photos listed as count reference in PDF (not embedded images) — MinIO byte fetch from service layer not in scope for pdf.go renderer"

# Metrics
duration: ~9min
completed: 2026-03-22
---

# Phase 3 Plan 02: EPK Service Layer + PDF Generator Summary

**Go EPK service (8 methods), PDF renderer with 6-construct Markdown parser, 8 HTTP routes wired to /api/v1/epk — 29 tests all GREEN; go build clean**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-03-22T19:09:32Z
- **Completed:** 2026-03-22T19:18:03Z
- **Tasks:** 2
- **Files modified/created:** 8

## Accomplishments

- `service.go`: `Service` with 8 methods (GetContent, UpsertContent, UploadPhoto, DeletePhoto, UploadStagePlot, GeneratePDF, ListExports, DeleteExport); bio sync to user_settings on BioShort/BioLong update; 20-photo cap with ErrPhotoLimitExceeded; ErrInvalidMIME for non-JPEG/PNG
- `pdf.go`: `renderEPKPDF` white A4 portrait, 20mm margins, accent colour from settings, 8 sections skipped when disabled or empty; `renderMarkdown` handles ##, ###, **bold**, *italic*, - list, 1. ordered list, [text](url) constructs
- `handler.go`: 8 chi routes; multipart upload for photos and stage plot; correct status codes (422 limit, 415 MIME, 404 not found, 204 no content, 201 created)
- Router and main.go updated; `go build ./...` passes
- `storage.Client.RemoveObject` wrapper added (required for delete operations)
- `codeberg.org/go-pdf/fpdf@v0.11.1` added to go.mod

## Task Commits

1. **Task 1: Service layer + PDF generator** - `af494c3` (feat)
2. **Task 2: HTTP handler + router wiring + handler tests** - `b14c5fb` (feat)

## Files Created/Modified

- `api/internal/epk/service.go` — Service struct, 8 methods, storageIface, settingsServiceIface, StorageClientAdapter, settingsServiceAdapter
- `api/internal/epk/pdf.go` — renderEPKPDF, renderMarkdown, renderInline, hexToRGB, writeSectionHeading
- `api/internal/epk/service_test.go` — 10 service unit tests (mockRepo + mockStorage + mockSettingsSvc)
- `api/internal/epk/handler.go` — Handler, NewHandler, Routes(), 8 route handlers
- `api/internal/epk/handler_test.go` — 9 handler unit tests (mock serviceIface)
- `api/internal/platform/http/router.go` — epkHandler parameter + r.Mount("/api/v1/epk", epkHandler)
- `api/cmd/api/main.go` — EPK wiring: epkRepo, epkStorage, epkSettingsSvc, epkSvc, epkHandler
- `api/internal/platform/storage/storage.go` — RemoveObject wrapper added

## Decisions Made

1. **Thin storageIface with contentType string:** EPK service uses `contentType string` in its interface rather than `minio.PutObjectOptions`. This keeps the business layer free of minio dependencies. A `StorageClientAdapter` bridges the real `storage.Client` (which uses `minio.PutObjectOptions`) to the EPK interface. Consistent with the goal of keeping service layers independent.

2. **settingsServiceAdapter:** The `settings.Service` exposes `GetOrCreate` and `Update` but the EPK service needed `GetSettings`/`UpdateSettings`. Rather than changing the settings package, an adapter in service.go bridges the two signatures with zero coupling changes.

3. **RemoveObject on storage.Client:** Neither photo delete nor export delete was possible without a MinIO object removal method. Added `RemoveObject` to `storage.Client` as a thin wrapper over `minio.Client.RemoveObject`. Classified as Rule 2 (missing critical functionality) — without it delete operations would be data-inconsistent.

4. **renderInline strips to plain text:** fpdf v0.11.1 `MultiCell` does not support per-run font switching within a single call. `renderInline` strips bold/italic/link markers to plain text for MultiCell output. Headings (## / ###) switch the font for the entire line before calling MultiCell, which correctly renders heading weight.

5. **Press photos listed as count reference:** The `renderEPKPDF` function lists photo count rather than embedding images, because byte-fetching photos from MinIO would require an IO interface on the PDF renderer. This is a pragmatic scope boundary — the PDF is still professionally formatted; photos are noted as "available on request."

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical Functionality] Added RemoveObject to storage.Client**
- **Found during:** Task 1 implementation
- **Issue:** `service.DeletePhoto` and `service.DeleteExport` require object removal from MinIO. `storage.Client` had no `RemoveObject` method.
- **Fix:** Added `RemoveObject(ctx, bucketName, objectName string) error` wrapper to `api/internal/platform/storage/storage.go`
- **Files modified:** `api/internal/platform/storage/storage.go`
- **Commit:** `af494c3`

**2. [Rule 1 - Bug] fpdf Cell() API mismatch**
- **Found during:** Task 1 build
- **Issue:** RESEARCH.md Pattern 5 referenced `f.Cell(0, 12, text, "", 1, "L", false, 0, "")` (9 args), but `codeberg.org/go-pdf/fpdf@v0.11.1` Cell() only accepts `(w, h float64, txtStr string)`. The extended signature is `CellFormat`.
- **Fix:** Replaced all `Cell(...)` calls with `MultiCell(0, h, text, "", "L", false)` which achieves the same line-break behaviour and matches the verified API.
- **Files modified:** `api/internal/epk/pdf.go`
- **Commit:** `af494c3`

**3. [Rule 1 - Bug] storageIface vs storage.Client PutObject signature mismatch**
- **Found during:** Task 1 design
- **Issue:** Plan specified a thin `storageIface` with `contentType string`, but `storage.Client.PutObject` takes `minio.PutObjectOptions`. These are incompatible interfaces.
- **Fix:** Created `StorageClientAdapter` in `service.go` that wraps `storage.Client` and adapts its `PutObject(... minio.PutObjectOptions)` and `PresignedGetObject(... reqParams map[string]string)` signatures to the thin EPK interface.
- **Files modified:** `api/internal/epk/service.go`
- **Commit:** `af494c3`

## Issues Encountered

- `fpdf.Cell` API in v0.11.1 differs from the RESEARCH.md example — fixed immediately during build; no data loss.
- Go binary found at `/usr/local/Cellar/go/1.26.1/libexec/bin` (not in default shell PATH) — resolved by explicit PATH export before every command.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- All 8 EPK service methods implemented, tested, and wired to HTTP routes
- EPK routes registered at `/api/v1/epk/*` — ready for frontend store (03-03)
- `go build ./...` and all 29 EPK tests GREEN
- No blockers for plan 03-03 (Pinia store + Vue components)

---
*Phase: 03-epk-press-kit-builder*
*Completed: 2026-03-22*

## Self-Check: PASSED

- FOUND: api/internal/epk/service.go
- FOUND: api/internal/epk/pdf.go
- FOUND: api/internal/epk/service_test.go
- FOUND: api/internal/epk/handler.go
- FOUND: api/internal/epk/handler_test.go
- COMMIT af494c3 verified (feat(03-02): EPK service layer + PDF generator)
- COMMIT b14c5fb verified (feat(03-02): EPK HTTP handler, router wiring, and handler tests)
