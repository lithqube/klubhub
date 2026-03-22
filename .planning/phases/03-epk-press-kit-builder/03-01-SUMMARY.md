---
phase: 03-epk-press-kit-builder
plan: "01"
subsystem: database
tags: [go, postgres, pgx, goose, migration, jsonb, epk, repository]

# Dependency graph
requires:
  - phase: 00-infrastructure
    provides: goose migration system, pgxpool.Pool pattern, single-file SQL migration format
  - phase: 02-social-media-scheduler
    provides: repository interface pattern, ErrNotFound sentinel, JSONB decode pattern
provides:
  - migration 004_epk.sql with epk_content singleton table and epk_exports table
  - EPKContent, EPKExport, UpsertEPKContentRequest, PressQuote Go types
  - Repository with 5 DB methods: GetContent, UpsertContent, InsertExport, ListExports, DeleteExport
  - repoIface interface for service/handler mock testing
affects:
  - 03-02-epk-service
  - 03-03-epk-handler
  - 03-04-epk-frontend-store

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Singleton table enforced via CREATE UNIQUE INDEX ON table ((true)) — constant expression unique index
    - JSONB fields stored as []byte, decoded via json.Unmarshal after Scan
    - Interface-substitution mock (hand-written mockRepo) for unit tests without testcontainers
    - repoIface defined in repository.go, test mock in repository_test.go (external test package)

key-files:
  created:
    - api/internal/platform/migrations/004_epk.sql
    - api/internal/epk/model.go
    - api/internal/epk/repository.go
    - api/internal/epk/repository_test.go
  modified: []

key-decisions:
  - "Singleton epk_content row enforced at DB level via CREATE UNIQUE INDEX ON epk_content ((true)) — constant expression prevents >1 row without application-level guards"
  - "UpsertContent uses CASE WHEN value != '' THEN value ELSE existing END pattern to preserve existing text fields when empty string passed (nil pointer = no update intent)"
  - "JSONB fields (gig_highlights, press_quotes, photo_paths, section_visibility) replace fully on each upsert — no partial merge at JSONB level"
  - "DeleteExport returns ErrNotFound sentinel (errors.New) when RowsAffected == 0 — hard delete, no soft delete"
  - "Unit tests use hand-written mockRepo implementing repoIface — no testcontainers, no pgxmock — consistent with service_test.go pattern in social package"

patterns-established:
  - "Singleton table pattern: CREATE UNIQUE INDEX ON table ((true)) enforces exactly one row at DB level"
  - "JSONB decode pattern: scan to []byte then json.Unmarshal into typed slice/map"
  - "Repository interface-substitution test pattern: hand-written mock in _test.go external package"

requirements-completed: [EPK-09]

# Metrics
duration: 4min
completed: 2026-03-22
---

# Phase 3 Plan 01: EPK Data Layer Summary

**PostgreSQL migration 004 with epk_content singleton table (JSONB fields, unique-index singleton) + epk_exports, Go model types, pgxpool Repository with 5 methods, 10 passing unit tests using interface-substitution mocks**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-22T19:00:54Z
- **Completed:** 2026-03-22T19:05:00Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Created `004_epk.sql` migration with goose Up/Down sections; singleton enforced via `CREATE UNIQUE INDEX ON epk_content ((true))`
- Defined `EPKContent`, `EPKExport`, `UpsertEPKContentRequest`, `PressQuote` Go structs with correct JSON tags
- Implemented `Repository` with all 5 methods (GetContent, UpsertContent, InsertExport, ListExports, DeleteExport) matching migration schema
- 10 unit tests all GREEN: happy paths + ErrNotFound sentinel + empty-list cases; `go vet` clean

## Task Commits

Each task was committed atomically:

1. **Task 1: Migration 004 — epk_content + epk_exports tables** - `2bd3474` (feat)
2. **Task 2: EPK model + repository + repository tests** - `f9e429e` (feat)

## Files Created/Modified
- `api/internal/platform/migrations/004_epk.sql` - goose migration: epk_content singleton + epk_exports tables
- `api/internal/epk/model.go` - EPKContent, EPKExport, UpsertEPKContentRequest, PressQuote types
- `api/internal/epk/repository.go` - repoIface + Repository implementing all 5 DB methods
- `api/internal/epk/repository_test.go` - 10 unit tests via hand-written mockRepo (no testcontainers)

## Decisions Made
- Singleton enforcement via `CREATE UNIQUE INDEX ON epk_content ((true))`: a constant expression unique index prevents inserting a second row at DB level without requiring application guards. Same ON CONFLICT clause handles both first-insert and update paths.
- UpsertContent preserves existing text fields when caller passes empty string (nil pointer semantics carried into SQL CASE WHEN). JSONB arrays/maps always replace fully — no partial merge within JSONB needed at this layer.
- Used hand-written `mockRepo` (interface-substitution) rather than pgxmock: pgxmock was not in go.mod and adding it would be scope creep. The service_test.go in social package uses the same pattern.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Go binary not in default shell PATH (`/usr/local/bin/go`); resolved by explicit PATH export before test commands. No code changes required.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All EPK types, repoIface, and 5 DB methods are defined — plan 03-02 (EPK service layer) can import `epk.repoIface` directly
- Migration 004 is pending DB apply (runs automatically on next container start via goose)
- No blockers for subsequent plans

---
*Phase: 03-epk-press-kit-builder*
*Completed: 2026-03-22*

## Self-Check: PASSED

- FOUND: api/internal/platform/migrations/004_epk.sql
- FOUND: api/internal/epk/model.go
- FOUND: api/internal/epk/repository.go
- FOUND: api/internal/epk/repository_test.go
- FOUND: .planning/phases/03-epk-press-kit-builder/03-01-SUMMARY.md
- COMMIT 2bd3474 verified (feat: migration 004_epk.sql)
- COMMIT f9e429e verified (feat: EPK data layer)
