---
phase: 01-tracklist-image-generator
plan: 03
subsystem: api
tags: [go, artwork, spotify, discogs, musicbrainz, minio, async-worker]

# Dependency graph
requires:
  - phase: 00-infrastructure
    provides: MinIO storage, PostgreSQL, HTTP router
  - phase: 01-tracklist-image-generator
    provides: Track model with ArtworkStatus fields
provides:
  - Artwork fetch pipeline: Spotify → Discogs → MusicBrainz/CAA fallback
  - MinIO-backed artwork cache with 7-day presigned URLs
  - Goroutine worker pool (4 workers) for async artwork fetching
  - Wire artwork service into tracklist service
affects: [01-tracklist-image-generator]

# Tech tracking
tech-stack:
  added: [artwork package]
  patterns: [fetch chain pattern, worker pool pattern, nil-safe client initialization]

key-files:
  created:
    - api/internal/artwork/model.go - ArtworkResult, Spotify/Discogs/MusicBrainz clients, FetchChain
    - api/internal/artwork/cache.go - MinIO-backed artwork cache
    - api/internal/artwork/worker.go - Service with FetchAll worker pool
    - api/internal/artwork/fetcher_test.go - 6 unit tests (mock HTTP servers)
  modified:
    - api/cmd/api/main.go - Wire artwork service
    - api/internal/platform/storage/storage.go - Add wrapper methods
    - api/internal/platform/http/router.go - Add tracklist routes
    - api/internal/tracklist/service.go - Export ServiceConfig, fix UpdateTrack return

key-decisions:
  - 'Used nil-safe client initialization - Spotify/Discogs clients nil when credentials absent'
  - 'MusicBrainz always attempted - no auth required'
  - 'Worker pool uses 4 goroutines with channel-based job distribution'
  - "Manual artwork (artwork_source='manual') never overwritten by API fetch (TRKL-14)"

patterns-established:
  - 'FetchChain pattern: cache check → provider 1 → provider 2 → provider 3 → placeholder'
  - 'Worker pool pattern: buffered channel + wait group + error logging without panic'

requirements-completed: [TRKL-10, TRKL-11, TRKL-12, TRKL-13, TRKL-16]

# Metrics
duration: 45min
completed: 2026-03-16
---

# Phase 1 Plan 3: Artwork Fetch Pipeline Summary

**Go cover art fetch pipeline with Spotify → Discogs → MusicBrainz/CAA fallback chain, MinIO artwork cache, and goroutine worker pool wired into tracklist service**

## Performance

- **Duration:** 45 min
- **Started:** 2026-03-16T00:28:38Z
- **Completed:** 2026-03-16T07:XX:XXZ
- **Tasks:** 3
- **Files modified:** 8

## Accomplishments

- Created artwork package with fetch chain (Spotify/Discogs/MusicBrainz)
- Implemented MinIO-backed cache with presigned URLs
- Built async worker pool (4 goroutines) for artwork fetching after tracklist save
- Wired artwork service into tracklist service and main.go
- Added tracklist HTTP routes to router
- All 6 artwork unit tests pass with mock HTTP servers

## Task Commits

1. **Task 1 & 2: Wave 0 tests + GREEN implementation** - `4935cc1` (feat)
   - Created artwork package with model.go, cache.go, worker.go
   - Added fetcher_test.go with 6 passing tests
   - Modified storage to add required wrapper methods

2. **Task 3: Worker + wiring** - `4935cc1` (feat)
   - Wired artwork service in main.go
   - Added tracklist routes to router
   - Fixed service interface mismatches

**Plan metadata:** `4935cc1` (docs: complete plan)

## Files Created/Modified

- `api/internal/artwork/model.go` - ArtworkResult, clients, FetchChain
- `api/internal/artwork/cache.go` - MinIO artwork cache
- `api/internal/artwork/worker.go` - Service with worker pool
- `api/internal/artwork/fetcher_test.go` - 6 unit tests
- `api/cmd/api/main.go` - Artwork service wiring
- `api/internal/platform/storage/storage.go` - Added wrapper methods
- `api/internal/platform/http/router.go` - Added tracklist routes
- `api/internal/tracklist/service.go` - Export config, fix return type

## Decisions Made

- Used nil-safe client initialization for optional API credentials
- MusicBrainz always attempted (no credentials required)
- Manual artwork overrides are never replaced by API fetch
- Worker pool uses 4 concurrent goroutines

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- Fixed storage.Client missing wrapper methods (PutObject, GetObject, StatObject, PresignedGetObject) - added to storage.go
- Fixed tracklist.Service.UpdateTrack return type mismatch with handler interface - changed to return \*Track

## Next Phase Readiness

- Artwork fetch pipeline complete and wired
- Ready for Phase 2 (likely integration/testing)
- No blockers

---

_Phase: 01-tracklist-image-generator_
_Completed: 2026-03-16_
