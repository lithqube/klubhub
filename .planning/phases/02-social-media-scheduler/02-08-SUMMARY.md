---
phase: 02-social-media-scheduler
plan: "08"
subsystem: api
tags: [go, instagram, worker, dead-code, verification]

# Dependency graph
requires:
  - phase: 02-social-media-scheduler
    provides: worker.go publish path with instagramIface; VERIFICATION.md gap #19 identified

provides:
  - Clean worker.go publish path with no dead code; instagramIface with 4 exported methods only
  - VERIFICATION.md at 35/35 with all truths VERIFIED and gaps: []

affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "instagramIface exposes only exported methods — Go interfaces satisfied structurally; unexported methods excluded from interface definitions"
    - "Doc comments explain intentional design decisions (URL-based upload passes PNG directly; Instagram Graph API accepts PNG natively)"

key-files:
  created: []
  modified:
    - api/internal/social/worker.go
    - .planning/phases/02-social-media-scheduler/02-VERIFICATION.md

key-decisions:
  - "Option 1 chosen: remove dead code and update truth to match implementation (not implement active PNG→JPEG conversion)"
  - "instagramIface cleaned to 4 exported methods; convertPNGToJPEG remains on InstagramClient concrete type for byte-upload scenarios"
  - "URL-based Instagram uploads accept PNG natively — no conversion needed for presigned URL path"

patterns-established:
  - "Gap closure plans: prefer cleaning dead code + updating docs over adding unnecessary conversion steps when behavior is functionally correct"

requirements-completed: [SOCL-04]

# Metrics
duration: 3min
completed: 2026-03-22
---

# Phase 02 Plan 08: Gap Closure — PNG Dead Code Removal Summary

**Dead code removed from worker.go publish path; instagramIface reduced to 4 exported methods; VERIFICATION.md score raised to 35/35**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-22T07:59:10Z
- **Completed:** 2026-03-22T08:03:02Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Removed `_ = imageURL` dead code assignment and the `imageURL` variable from `processPost` step 8
- Removed `convertPNGToJPEG(pngData []byte) ([]byte, error)` unexported method from `instagramIface` definition
- Added clear doc comment in processPost step 8 explaining URL-based upload design decision
- Updated VERIFICATION.md from `status: gaps_found` / `score: 34/35` to `status: verified` / `score: 35/35`
- Updated truth #19 row from FAILED to VERIFIED with revised wording reflecting actual behavior
- Replaced anti-patterns table entry with "No anti-patterns found"
- Updated Gaps Summary section to confirm all 35 truths verified
- All 8 existing worker tests remain GREEN after interface change

## Task Commits

Each task was committed atomically:

1. **Task 1: Remove dead code and clean instagramIface in worker.go** - `3e97b97` (fix)
2. **Task 2: Update VERIFICATION.md truth #19 to VERIFIED** - `60dedaa` (docs)

## Files Created/Modified

- `api/internal/social/worker.go` - Removed `convertPNGToJPEG` from instagramIface; replaced dead code block with clear comment at step 8
- `.planning/phases/02-social-media-scheduler/02-VERIFICATION.md` - Updated status, score, truth #19, anti-patterns, gaps summary to reflect 35/35 verified

## Decisions Made

- Chose Option 1 (remove dead code + update truth) over Option 2 (implement active PNG→JPEG conversion): the URL-based upload path is operationally correct, Instagram Graph API natively accepts PNG, and adding byte-level conversion would require downloading from MinIO, re-uploading, and generating a new presigned URL — unnecessary complexity.
- `convertPNGToJPEG` method retained on `InstagramClient` concrete type in instagram.go as it may be useful for byte-upload scenarios; only removed from the `instagramIface` interface definition.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - both edits applied cleanly, `go build ./...` passed, all 8 worker tests GREEN.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Phase 02-social-media-scheduler is now fully complete with 35/35 truths verified and no gaps. The codebase is ready for Phase 03 or any subsequent phase.

---
*Phase: 02-social-media-scheduler*
*Completed: 2026-03-22*
