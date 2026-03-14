---
phase: 00-infrastructure
plan: 03
subsystem: api
tags: [chi, zerolog, pgx, testcontainers, minio, http, settings, migrations, goose]

requires:
  - phase: 00-infrastructure-01
    provides: platform packages (config, db, storage, log, migrations, crypto)

provides:
  - chi router wired with Recoverer + RequestLogger middleware
  - GET /api/v1/health with per-integration status (database, storage, spotify, discogs, musicbrainz, instagram)
  - GET /api/v1/settings and PUT /api/v1/settings with 409 optimistic concurrency
  - user_settings singleton CRUD (GetOrCreate, Get, Update)
  - Running HTTP server wired in cmd/api/main.go

affects:
  - all future feature phases building on the HTTP layer
  - phases needing user_settings data

tech-stack:
  added:
    - github.com/go-chi/chi/v5 (promoted to direct dep from router)
    - github.com/testcontainers/testcontainers-go v0.41.0 (integration testing)
    - github.com/testcontainers/testcontainers-go/modules/postgres v0.41.0
  patterns:
    - DBPinger interface on health handler (testable without real pgxpool)
    - StorageHealthChecker interface on health handler (mockable store)
    - ErrConflict sentinel mapped to HTTP 409 in handler layer
    - ErrNotFound sentinel for missing-row cases
    - WHERE deleted_at IS NULL on every settings query (soft-delete pattern)
    - Optimistic concurrency via updated_at timestamp match in SQL UPDATE WHERE clause
    - testcontainers-go integration tests in repository_test.go with TestMain pattern

key-files:
  created:
    - api/internal/platform/http/health.go
    - api/internal/platform/http/health_test.go
    - api/internal/platform/http/middleware.go
    - api/internal/platform/http/router.go
    - api/internal/settings/model.go
    - api/internal/settings/repository.go
    - api/internal/settings/repository_test.go
    - api/internal/settings/service.go
    - api/internal/settings/handler.go
    - api/internal/platform/migrations/001_initial_schema.sql
  modified:
    - api/cmd/api/main.go
    - api/internal/platform/storage/storage.go
    - api/go.mod
    - api/go.sum

key-decisions:
  - "DBPinger and StorageHealthChecker interfaces allow health handler to be unit-tested without Docker"
  - "Health endpoint always returns HTTP 200; JSON status field reflects degraded/unhealthy internals"
  - "musicbrainz always ok (no API key required); other optional integrations show unconfigured when key absent"
  - "goose migration files use single-file convention (001_name.sql with Up/Down sections) not separate .up.sql/.down.sql files"
  - "Repository uses raw pgx.Rows.Scan instead of pgx.RowToStructByName for explicit JSONB handling"

patterns-established:
  - "Sentinel errors (ErrConflict, ErrNotFound) defined in model.go, checked with errors.Is in handler"
  - "Integration tests use testcontainers-go TestMain pattern: start container -> run migrations -> run tests -> cleanup"
  - "Internal http package aliased as apphttp in main.go to avoid collision with stdlib net/http"
  - "JSONB columns manually marshalled/unmarshalled via encoding/json in repository scanOne helper"

requirements-completed: [INFRA-04, INFRA-05, INFRA-07, INFRA-10, INFRA-11]

duration: 23min
completed: 2026-03-14
---

# Phase 00 Plan 03: HTTP Layer Summary

**chi router + zerolog middleware + health endpoint with 6 integration statuses + user_settings CRUD with optimistic 409 concurrency, all wired in a running HTTP server**

## Performance

- **Duration:** 23 min
- **Started:** 2026-03-14T20:34:07Z
- **Completed:** 2026-03-14T20:57:28Z
- **Tasks:** 2
- **Files modified:** 14

## Accomplishments

- Health endpoint checking database (pgxpool.Ping), storage (MinIO BucketExists), and 4 optional integrations by key presence
- user_settings singleton with GetOrCreate (idempotent), Get, Update with optimistic concurrency (UPDATE WHERE updated_at=$N → 0 rows affected = ErrConflict → HTTP 409)
- Full startup sequence in main.go: config → log → migrations → pgxpool → MinIO → settings → router → HTTP server
- All tests pass: 6 unit tests for health handler, 6 integration tests for settings repository via testcontainers-go

## Task Commits

Each task was committed atomically:

1. **Task 1: Health endpoint + chi router + request middleware** - `3f274c2` (feat)
2. **Task 2: user_settings module + wire main.go** - `4d15652` (feat)

**Plan metadata:** (this SUMMARY commit)

_Note: TDD tasks — tests written first (RED), then implementation (GREEN)_

## Files Created/Modified

- `api/internal/platform/http/health.go` - HealthHandler with DBPinger/StorageHealthChecker interfaces; returns HealthResponse with 6 integrations
- `api/internal/platform/http/health_test.go` - 6 table-style tests covering healthy/unhealthy/unconfigured/all-keys/timing
- `api/internal/platform/http/middleware.go` - RequestLogger middleware with UUID request-id, X-Request-ID header, zerolog completion log
- `api/internal/platform/http/router.go` - NewRouter factory wiring chi with Recoverer + RequestLogger + health + settings routes
- `api/internal/settings/model.go` - UserSettings and UpdateSettingsRequest structs; ErrConflict and ErrNotFound sentinels
- `api/internal/settings/repository.go` - GetOrCreate, Get, Update with optimistic concurrency; all queries WHERE deleted_at IS NULL
- `api/internal/settings/repository_test.go` - 6 integration tests via testcontainers-go PostgreSQL 16
- `api/internal/settings/service.go` - Thin service wrapper over repository
- `api/internal/settings/handler.go` - GET returns 200 JSON, PUT returns 409 JSON on ErrConflict
- `api/cmd/api/main.go` - Full wiring with correct startup order
- `api/internal/platform/storage/storage.go` - Added HealthCheck() method for MinIO probe
- `api/internal/platform/migrations/001_initial_schema.sql` - Merged single-file migration (replaces .up.sql + .down.sql)

## Decisions Made

- DBPinger and StorageHealthChecker interfaces defined in the http package so the health handler can be unit-tested without Docker — fake structs in test file implement both
- Health endpoint always returns HTTP 200; internal failures set JSON status to "degraded" or "unhealthy"
- musicbrainz reports "ok" unconditionally (no API key needed); spotify/discogs/instagram report "unconfigured" when key absent
- Single-file goose migration convention chosen after discovering that the separate .up.sql/.down.sql pattern causes a panic in goose v3 legacy API

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed goose v3 duplicate version 1 panic from separate .up.sql/.down.sql files**
- **Found during:** Task 2 (integration test run via testcontainers-go)
- **Issue:** goose v3's legacy `Up()` API collects ALL `*.sql` files via glob, treats each file's numeric prefix as a version. Having both `001_initial_schema.up.sql` and `001_initial_schema.down.sql` causes `sortAndConnectMigrations` to panic with "duplicate version 1 detected".
- **Fix:** Merged both files into `001_initial_schema.sql` with `-- +goose Up` / `-- +goose Down` sections in a single file. Deleted the separate .up.sql and .down.sql files.
- **Files modified:** `api/internal/platform/migrations/001_initial_schema.sql` (created), `001_initial_schema.up.sql` + `.down.sql` (deleted)
- **Verification:** `go test ./internal/platform/migrations/...` still passes; integration tests complete successfully
- **Committed in:** `4d15652` (Task 2 commit)

**2. [Rule 2 - Missing Critical] Added HealthCheck() method to storage.Client**
- **Found during:** Task 1 (implementing health handler)
- **Issue:** Plan specified `store.HealthCheck(ctx)` call in the health handler but the method didn't exist on storage.Client from Plan 01
- **Fix:** Added `HealthCheck(ctx context.Context) error` to `storage/storage.go` performing a lightweight `mc.BucketExists()` probe
- **Files modified:** `api/internal/platform/storage/storage.go`
- **Verification:** Health tests pass with fake store implementing StorageHealthChecker interface
- **Committed in:** `3f274c2` (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (1 bug fix, 1 missing critical method)
**Impact on plan:** Both fixes required for plan to execute. No scope creep.

## Issues Encountered

None beyond the above deviations.

## User Setup Required

None - no external service configuration required beyond what Plan 01 documented.

## Next Phase Readiness

- HTTP server fully wired and ready for additional routes
- settings CRUD pattern established for future feature modules
- testcontainers-go integration test pattern ready to reuse for any new repository
- Blockers: none

---
*Phase: 00-infrastructure*
*Completed: 2026-03-14*
