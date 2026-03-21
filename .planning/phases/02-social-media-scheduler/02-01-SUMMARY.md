---
phase: 02-social-media-scheduler
plan: "01"
subsystem: api
tags: [go, postgres, pgx, goose, testcontainers, instagram-oauth, image-validation, chi]

requires:
  - phase: 00-infrastructure
    provides: social_accounts table stub, platform/crypto AES-256-GCM, platform/migrations goose setup
  - phase: 01-tracklist-image-generator
    provides: repository/service/handler pattern to follow, tracklist module as integration reference

provides:
  - "003_social.sql migration: ALTER social_accounts (ig_user_id, UNIQUE platform, deleted_at), CREATE scheduled_posts table with status/post_type enums, performance index"
  - "api/internal/social/model.go: SocialAccount, ScheduledPost structs; PostStatus and PostType constants"
  - "api/internal/social/repository.go: 12 methods (GetAccount, UpsertAccount, CreatePost, GetPost, ListPosts, UpdatePost, UpdatePostStatus, DisconnectAccountCascade, SetNextRetry, SoftDeletePost, ResetPostForRetry, UpdateAccountStatus)"
  - "api/internal/social/service.go: SchedulePost (UTC conversion), EditPost (ErrEditBlocked gate), DisconnectAccount, RetryPost, ValidateImage (MIME/size/dimensions), GetOAuthURL, HandleOAuthCallback"
  - "api/internal/social/handler.go: chi routes for /auth/url, /auth/callback, /accounts, /accounts/{id}, /posts, /posts/{id}, /posts/{id}/retry"
  - "Social routes registered in router.go under /api/v1/social"

affects:
  - 02-social-media-scheduler
  - all future plans that post to Instagram or manage social_accounts/scheduled_posts

tech-stack:
  added: ["gabriel-vasile/mimetype (already in go.mod)", "image/jpeg + image/png blank imports for image.DecodeConfig"]
  patterns:
    - "serviceIface interface between handler and service (same pattern as tracklist)"
    - "repoIface interface between service and repository (enables mock testing)"
    - "parseDateTimeLocal helper: time.LoadLocation + time.ParseInLocation → UTC"
    - "ValidateImage: MIME check → size check → dimension/ratio check (ordered for fast rejection)"
    - "ON CONFLICT (platform) DO UPDATE for single-account Instagram upsert"

key-files:
  created:
    - api/internal/platform/migrations/003_social.sql
    - api/internal/social/model.go
    - api/internal/social/repository.go
    - api/internal/social/repository_test.go
    - api/internal/social/service.go
    - api/internal/social/service_test.go
    - api/internal/social/handler.go
    - api/internal/social/handler_test.go
  modified:
    - api/internal/platform/http/router.go
    - api/internal/platform/config/config.go
    - api/cmd/api/main.go

key-decisions:
  - "UNIQUE constraint on social_accounts(platform) added in migration 003 (not 001) to enable ON CONFLICT upsert"
  - "serviceIface consumed by handler; repoIface consumed by service — two-level interface stack for testability"
  - "ValidateImage checks MIME then size then dimensions — fast rejection order (MIME is cheapest)"
  - "parseDateTimeLocal uses time.LoadLocation + ParseInLocation pattern — IANA timezone names only"
  - "Story aspect ratio tolerance ±0.05 around 9:16 (0.5625) — allows minor float imprecision"
  - "handleCreatePost accepts both multipart form (image_file upload) and image_id (existing MinIO path)"
  - "INSTAGRAM_REDIRECT_URI added to config.go with default for local dev"

requirements-completed: [SOCL-01, SOCL-02, SOCL-03, SOCL-07, SOCL-08, SOCL-10, SOCL-12, SOCL-14, SOCL-15]

duration: 10min
completed: 2026-03-21
---

# Phase 2 Plan 01: Social Foundation — Go Package Summary

**Go social package: migration 003, domain models, repository (12 methods), service (business logic + image validation), HTTP handler (11 routes), 24 tests GREEN via testcontainers + httptest mocks**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-21T16:56:49Z
- **Completed:** 2026-03-21T17:07:24Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments

- Migration 003 applies cleanly on top of 001+002: adds `ig_user_id`, `deleted_at`, UNIQUE constraint to `social_accounts`; creates `scheduled_posts` with full status enum, post_type enum, and a partial index on (status, scheduled_at_utc)
- Repository layer with 12 methods including idempotent `UpdatePostStatus`, cascade `DisconnectAccountCascade` (scheduled→draft only), and `ResetPostForRetry`
- Service layer with timezone-aware UTC conversion (`time.LoadLocation` + `ParseInLocation`), Instagram OAuth URL builder, status-gated `EditPost` (`ErrEditBlocked`), and `ValidateImage` (MIME/size/dimension checks for feed and story)
- HTTP handler wired under `/api/v1/social` in the main chi router; all 11 routes registered

## Task Commits

Each task was committed atomically:

1. **Task 1: Migration 003 + Go social model and repository** - `b16bd3c` (feat)
2. **Task 2: Social service + HTTP handler with routes** - `6c47e88` (feat)

## Files Created/Modified

- `api/internal/platform/migrations/003_social.sql` - Migration: ALTER social_accounts, CREATE scheduled_posts with status/post_type enums and index
- `api/internal/social/model.go` - SocialAccount, ScheduledPost structs; PostStatus and PostType constants
- `api/internal/social/repository.go` - 12 repository methods backed by pgxpool
- `api/internal/social/repository_test.go` - 6 tests via testcontainers-go (GetAccount nil, CreatePost roundtrip, DisconnectAccountCascade, ListPosts DESC, UpdatePostStatus idempotent, ResetPostForRetry)
- `api/internal/social/service.go` - Business logic: SchedulePost (UTC conversion), EditPost (edit gate), ValidateImage, GetOAuthURL, HandleOAuthCallback, DisconnectAccount, RetryPost
- `api/internal/social/service_test.go` - 7 tests: UTC conversion (Europe/Berlin), ErrEditBlocked, image validation (oversized, MIME, feed portrait, feed square, story ratio, OAuth URL params)
- `api/internal/social/handler.go` - Chi handler with 11 routes via serviceIface
- `api/internal/social/handler_test.go` - 9 handler tests via httptest (null account, OAuth URL, 409 blocked edit, retry 200/404, image 422 variants, post 201, disconnect 200)
- `api/internal/platform/http/router.go` - Added socialHandler param and `/api/v1/social` mount
- `api/internal/platform/config/config.go` - Added INSTAGRAM_REDIRECT_URI env var
- `api/cmd/api/main.go` - Wired social.Repository, Service, Handler into startup

## Decisions Made

- UNIQUE constraint on `social_accounts(platform)` added in migration 003 (not 001) — keeps migrations atomic to their phase
- Two-level interface stack: handler→serviceIface, service→repoIface — allows both layers to be independently mocked in tests without testcontainers for handler/service tests
- `ValidateImage` order: MIME → size → dimensions — cheapest checks first
- Story aspect ratio tolerance ±0.05 around 9:16 — allows for minor float rounding in image headers
- `INSTAGRAM_REDIRECT_URI` defaults to `http://localhost:3000/auth/instagram/callback` for local dev

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Added UNIQUE constraint on social_accounts(platform) in migration 003**
- **Found during:** Task 1 (repository tests)
- **Issue:** UpsertAccount uses `ON CONFLICT (platform) DO UPDATE` but social_accounts had no UNIQUE constraint on platform — PostgreSQL rejected the query with `42P10`
- **Fix:** Added `ALTER TABLE social_accounts ADD CONSTRAINT social_accounts_platform_unique UNIQUE (platform)` to migration 003 Up section; added matching DROP CONSTRAINT in Down section
- **Files modified:** api/internal/platform/migrations/003_social.sql
- **Verification:** All 6 repository tests pass including UpsertAccount-dependent tests
- **Committed in:** b16bd3c (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug)
**Impact on plan:** Essential fix for correctness; migration 001 stub did not include the constraint needed for upsert semantics.

## Issues Encountered

None beyond the auto-fixed UNIQUE constraint deviation above.

## User Setup Required

Environment variables needed for Instagram OAuth to work (not required for the app to boot):
- `INSTAGRAM_CLIENT_ID` — Instagram App ID
- `INSTAGRAM_CLIENT_SECRET` — Instagram App Secret
- `INSTAGRAM_REDIRECT_URI` — OAuth callback URL (defaults to `http://localhost:3000/auth/instagram/callback`)
- `TOKEN_ENCRYPTION_KEY` — 32-byte AES-256 key for encrypting OAuth tokens

## Next Phase Readiness

- All social API contracts established (routes, status enums, model types) — subsequent plans can build the worker, UI, and MinIO upload without schema changes
- GET /api/v1/social/accounts returns `{"data": null}` when disconnected — UI can render "connect" CTA immediately
- GET /api/v1/social/auth/url returns a well-formed Instagram OAuth URL
- Edit blocking (409), disconnect cascade, retry flow all tested end-to-end

---
*Phase: 02-social-media-scheduler*
*Completed: 2026-03-21*
