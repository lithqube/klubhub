---
phase: 02-social-media-scheduler
plan: "03"
subsystem: ui
tags: [pinia, vue, composables, typescript, vitest, tdd, social-media, instagram]

# Dependency graph
requires:
  - phase: 02-social-media-scheduler
    provides: API contract for social endpoints (GET/POST /api/v1/social/posts, /accounts)
  - phase: 1.5-design-system-foundation
    provides: Pinia store patterns (composition API style, storeToRefs convention)
provides:
  - TypeScript interfaces SocialAccount, ScheduledPost, PostStatus, PostType, CreatePostRequest, EditPostRequest
  - useSocialStore with posts, account, loading, composePanelOpen, prefilledImageId state and all CRUD actions
  - useSocialPostForm composable with caption auto-gen, char counter, timezone filtering, UTC preview
affects: [02-04-compose-panel, 02-05-schedule-feed, 02-06-instagram-oauth]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Pinia composition API store with $fetch and loading state pattern
    - FormData POST for multipart image upload in store actions
    - storeToRefs() for reactive store destructuring in composables
    - Intl.supportedValuesOf('timeZone') for timezone combobox options
    - TDD RED-GREEN-COMMIT cycle per task

key-files:
  created:
    - apps/dj/app/types/social.ts
    - apps/dj/app/stores/social.ts
    - apps/dj/app/stores/__tests__/social.test.ts
    - apps/dj/app/composables/useSocialPostForm.ts
    - apps/dj/app/composables/__tests__/useSocialPostForm.test.ts
  modified: []

key-decisions:
  - "useSocialPostForm accesses settingsStore.djName via computed() instead of storeToRefs() to keep mocking clean in tests"
  - "retryPost() refreshes only the specific post in-store (via index splice) rather than re-fetching full list — avoids loading flash"
  - "createPost() calls loadPosts() after POST to ensure store is always in sync with server state"
  - "charLimit is null for story type (no Instagram limit) vs 2200 for feed — isOverLimit guard handles null"

patterns-established:
  - "Store actions wrap $fetch in try/catch with useUiStore().showError() in catch blocks"
  - "FormData built inside createPost() — fields appended individually, imageFile appended only if present"
  - "openComposePanel(imageId?) uses ?? null to normalise undefined vs null"

requirements-completed: [SOCL-03, SOCL-09, SOCL-10, SOCL-17]

# Metrics
duration: 15min
completed: 2026-03-21
---

# Phase 02 Plan 03: Social Types, Store, and Form Composable Summary

**Pinia social store with CRUD actions for ScheduledPost/SocialAccount, TypeScript interfaces matching backend API contract, and useSocialPostForm composable with caption auto-gen, char counter, timezone filter — 45 tests GREEN**

## Performance

- **Duration:** 15 min
- **Started:** 2026-03-21T16:56:42Z
- **Completed:** 2026-03-21T17:10:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- TypeScript type layer (PostStatus, PostType, SocialAccount, ScheduledPost, CreatePostRequest, EditPostRequest) mirrors backend API contract exactly
- useSocialStore exposes posts, account, composePanelOpen, prefilledImageId with loadPosts, createPost (FormData), retryPost, downloadImage, openComposePanel, closeComposePanel
- useSocialPostForm composable provides charLimit/charCount/isOverLimit (2200 for feed, null for story), timezoneOptions from Intl, generateCaption, resetForm

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: TypeScript types + useSocialStore tests** - `c16d983` (test)
2. **Task 1 GREEN: useSocialStore implementation** - `661e7dc` (feat)
3. **Task 2 RED: useSocialPostForm tests** - `21766b1` (test)
4. **Task 2 GREEN: useSocialPostForm implementation** - `de8b8bd` (feat)

_Note: TDD tasks committed as RED (test) then GREEN (feat) per TDD protocol._

## Files Created/Modified

- `apps/dj/app/types/social.ts` - TypeScript interfaces for all social domain types
- `apps/dj/app/stores/social.ts` - useSocialStore with 10 actions and 5 reactive state refs
- `apps/dj/app/stores/__tests__/social.test.ts` - 19 tests for store actions and state
- `apps/dj/app/composables/useSocialPostForm.ts` - Form composable with computed char counting, timezone filtering, caption generation
- `apps/dj/app/composables/__tests__/useSocialPostForm.test.ts` - 26 tests covering all computed and function behavior

## Decisions Made

- Used `computed(() => settingsStore.djName)` in `useSocialPostForm` instead of `storeToRefs()` — the mock returns a plain object with reactive `ref` but `storeToRefs()` requires Pinia internal markers; `computed()` accesses the same reactive value without the mock complexity
- `retryPost()` updates the individual post in-store by finding index and splicing — avoids a full reload and eliminates loading flash on retry
- Test mock for `useSettingsStore` uses `$id`, `$patch`, `$subscribe`, `$onAction`, `$dispose` shims to satisfy `storeToRefs` internal checks

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed storeToRefs() incompatibility with vi.mock plain object**
- **Found during:** Task 2 (useSocialPostForm GREEN phase)
- **Issue:** `storeToRefs(settingsStore)` returned undefined for djName when store was mocked as plain object — Pinia requires internal store markers ($id etc.)
- **Fix:** Updated test mock to include $id, $patch, $subscribe, $onAction, $dispose shims; returned djName as ref() so storeToRefs() works correctly
- **Files modified:** apps/dj/app/composables/__tests__/useSocialPostForm.test.ts
- **Verification:** 26 tests GREEN after fix
- **Committed in:** de8b8bd (Task 2 GREEN commit)

**2. [Rule 1 - Bug] Fixed vi.mock path for useUiStore**
- **Found during:** Task 1 (initial run after implementation)
- **Issue:** Mock path was `'../../stores/ui'` (from test perspective) but correct relative path is `'../ui'`
- **Fix:** Updated vi.mock path to `'../ui'`
- **Files modified:** apps/dj/app/stores/__tests__/social.test.ts
- **Verification:** All 19 social store tests pass
- **Committed in:** 661e7dc (Task 1 GREEN commit)

---

**Total deviations:** 2 auto-fixed (2 Rule 1 bugs)
**Impact on plan:** Both fixes required for tests to run correctly. No scope creep.

## Issues Encountered

- Vitest v4 uses `--testPathPattern` flag from the plan but the correct flag is positional file path argument — used `pnpm nx test dj -- social.test` pattern throughout

## User Setup Required

None - no external service configuration required.

## Self-Check: PASSED

All 5 files exist. All 4 task commits verified (c16d983, 661e7dc, 21766b1, de8b8bd).

## Next Phase Readiness

- Types, store, and form composable are ready for Plan 02-04 (compose panel UI) and 02-05 (schedule feed)
- Components import from `~/stores/social` and `~/composables/useSocialPostForm`
- All 45 tests GREEN (19 store + 26 form composable)

---
*Phase: 02-social-media-scheduler*
*Completed: 2026-03-21*
