---
phase: 02-social-media-scheduler
plan: "02"
subsystem: api
tags: [go, instagram-graph-api, background-worker, retry, rate-limit, oauth-token-refresh, png-to-jpeg]

requires:
  - phase: 02-social-media-scheduler/02-01
    provides: "repository.go with UpdatePostStatus/SetNextRetry/UpdateAccountStatus; model.go with PostStatus/PostType/ScheduledPost/SocialAccount; platform/crypto AES-256-GCM Decrypt/Encrypt; storage.Client.PresignedGetObject"

provides:
  - "api/internal/social/instagram.go: InstagramClient (CreateContainer, PublishContainer, CheckContainerStatus, RefreshToken, convertPNGToJPEG), RateLimitError, ContainerStatus constants"
  - "api/internal/social/worker.go: Worker struct, StartPublishWorker goroutine, tick/processPost, backoffTime (5/20/80 min), refreshExpiringTokens"
  - "api/internal/social/repository.go: added ListDuePosts, ListExpiringAccounts, UpdateContainerID, UpdateAccountToken methods"
  - "api/cmd/api/main.go: signal.NotifyContext for graceful shutdown, go StartPublishWorker registered on startup"

affects:
  - 02-social-media-scheduler
  - future phases requiring post publishing verification

tech-stack:
  added: ["os/signal + syscall (graceful shutdown - stdlib)", "image/jpeg + image/png (PNG→JPEG conversion - stdlib)"]
  patterns:
    - "workerRepoIface/instagramIface/storageIface: thin interfaces enable full mock testing of worker without testcontainers"
    - "RateLimitError sentinel type: allows caller to distinguish rate-limit from generic errors and stop tick early"
    - "backoffTime(retryCount) helper: returns absolute next-retry time from [5,20,80]-minute schedule"
    - "30-second mutex-protected lastPublishedAt tracker: enforces Instagram API call spacing"
    - "Container status pre-check before retry: prevents duplicate publish when container already PUBLISHED"

key-files:
  created:
    - api/internal/social/instagram.go
    - api/internal/social/worker.go
    - api/internal/social/worker_test.go
  modified:
    - api/internal/social/repository.go
    - api/cmd/api/main.go

key-decisions:
  - "Worker uses workerRepoIface/instagramIface/storageIface thin interfaces for full-mock unit testing without testcontainers"
  - "Worker.tick swallows RateLimitError (returns nil) after stopping loop — avoids retry storm, lets caller proceed normally"
  - "signal.NotifyContext added in main.go for SIGTERM/SIGINT — ensures worker goroutine exits cleanly on container shutdown"
  - "IsPNG helper exposed at package level using mimetype.Detect — used when raw bytes are available for conversion"
  - "UpdateAccountToken added to repository for token refresh write path (separate from UpdateAccountStatus)"

patterns-established:
  - "Worker dependency injection via thin interfaces: all external I/O mockable without real infrastructure"
  - "Retry backoff table pattern: []int{5,20,80} minutes indexed by retry_count with bounds clamping"

requirements-completed: [SOCL-04, SOCL-05, SOCL-06, SOCL-11, SOCL-12, SOCL-13, SOCL-16]

duration: 15min
completed: 2026-03-21
---

# Phase 02 Plan 02: Instagram API Client and Background Publishing Worker Summary

**Instagram Graph API client + 60s ticker goroutine with exponential backoff (5/20/80 min), rate-limit detection, duplicate-publish prevention via container status pre-check, and token auto-refresh**

## Performance

- **Duration:** 15 min
- **Started:** 2026-03-21T23:09:44Z
- **Completed:** 2026-03-21T23:25:09Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Instagram Graph API client with full two-step publish flow (CreateContainer → PublishContainer), CheckContainerStatus, and RefreshToken
- Background worker goroutine polls DB every 60s, enforces 30s API spacing, retries with 5/20/80 min backoff, marks permanently_failed after 3 retries
- Container status pre-check before every retry prevents duplicate publishes
- Token refresh cycle runs each tick: refreshes expiring OAuth tokens, marks accounts disconnected on failure
- PNG→JPEG conversion at quality 95 using stdlib `image/png` + `image/jpeg`
- Worker registered in main.go with signal.NotifyContext for SIGTERM/SIGINT graceful shutdown
- 4 new repository methods: ListDuePosts, ListExpiringAccounts, UpdateContainerID, UpdateAccountToken

## Task Commits

Each task was committed atomically:

1. **Task 1: Instagram Graph API client** - `a43e925` (feat)
2. **Task 2: Publishing worker + main.go registration** - `dfe3cec` (feat)

**Plan metadata:** [pending final commit] (docs: complete plan)

## Files Created/Modified
- `api/internal/social/instagram.go` - InstagramClient struct with 5 methods + RateLimitError + ContainerStatus constants
- `api/internal/social/worker.go` - Worker struct, StartPublishWorker goroutine, processPost, backoffTime, refreshExpiringTokens
- `api/internal/social/worker_test.go` - 8 unit tests: permanently_failed, rate-limit stop, successful publish, retry dedup, token refresh failure/success, backoff schedule, publish failure retry
- `api/internal/social/repository.go` - Added ListDuePosts, ListExpiringAccounts, UpdateContainerID, UpdateAccountToken
- `api/cmd/api/main.go` - Added signal.NotifyContext, NewInstagramClient, NewWorker, go StartPublishWorker

## Decisions Made
- Worker uses thin interfaces (workerRepoIface, instagramIface, storageIface) so all 8 tests run without testcontainers
- `tick` swallows RateLimitError instead of propagating it — the tick simply stops processing remaining posts and returns nil, avoiding error log spam
- `signal.NotifyContext` in main.go replaces bare `context.Background()` — worker goroutine exits cleanly on container SIGTERM
- `UpdateAccountToken` added as a separate repository method for the token refresh write path (distinct from status-only `UpdateAccountStatus`)
- `backoffTime` uses bounds clamping so retry_count > 2 always gets 80 minutes (defensive against unexpected state)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added ListDuePosts, ListExpiringAccounts, UpdateContainerID, UpdateAccountToken to repository**
- **Found during:** Task 2 (worker.go implementation)
- **Issue:** Plan required worker to call these methods but they did not exist in repository.go from plan 02-01
- **Fix:** Added all 4 methods to repository.go with correct SQL queries matching the spec
- **Files modified:** api/internal/social/repository.go
- **Verification:** Worker tests pass and go build succeeds
- **Committed in:** dfe3cec (Task 2 commit)

**2. [Rule 2 - Missing Critical] Added signal.NotifyContext for graceful shutdown**
- **Found during:** Task 2 (main.go wiring)
- **Issue:** main.go used bare context.Background() — worker goroutine would never stop cleanly on container SIGTERM, causing Docker to hard-kill after 10s
- **Fix:** Replaced context.Background() with signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
- **Files modified:** api/cmd/api/main.go
- **Verification:** go build succeeds; signal handling is stdlib with no new dependencies
- **Committed in:** dfe3cec (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 missing critical — missing repo methods, 1 missing critical — graceful shutdown)
**Impact on plan:** Both auto-fixes required for correctness. Repository methods were listed in the plan interfaces section but omitted from 02-01's scope. Signal handling is required for the "stops cleanly on context cancellation" must-have truth.

## Issues Encountered
None — all tests passed on first run.

## User Setup Required
None - worker starts automatically on `docker compose up`. Instagram credentials (INSTAGRAM_CLIENT_ID, INSTAGRAM_CLIENT_SECRET, TOKEN_ENCRYPTION_KEY) must be set for actual publishing — these were documented in plan 02-01.

## Next Phase Readiness
- Publishing infrastructure complete: posts scheduled via the API will be auto-published by the worker
- All 7 must-have truths verified in tests or by code inspection
- Worker registered and starts on `go StartPublishWorker` goroutine in main.go
- Ready for UI integration (schedule composer, post queue) in subsequent plans

---
*Phase: 02-social-media-scheduler*
*Completed: 2026-03-21*
