---
phase: 02-social-media-scheduler
plan: "04"
subsystem: ui
tags: [vue, pinia, vitest, tdd, social-media, instagram, shadcn-vue, tailwind, cyberpunk]

# Dependency graph
requires:
  - phase: 02-social-media-scheduler
    plan: "03"
    provides: useSocialStore, useSocialPostForm, SocialAccount/ScheduledPost TypeScript types
  - phase: 1.5-design-system-foundation
    provides: glass-panel, gradient-cta, ghost-border utilities, font-terminal, font-command, text-primary/secondary tokens

provides:
  - social.vue page: thin layout with ?imageId param handling, loadPosts/loadAccount on mount, banners, SocialTabs
  - SocialPageHeader: SOCIAL SCHEDULER heading, connected account status dot + DISCONNECT button
  - SocialTabs: QUEUE/CALENDAR tabs with active underline, ANALYTICS/AUTOMATIONS coming-soon stubs, action buttons
  - SocialPostCompose: collapsible compose panel with Feed/Story toggle, caption textarea + char counter, image attachment, datetime + timezone combobox, submit guard
  - SocialQueueGrid: 3-column grid routing failed posts to SocialPostCardFailed and others to SocialPostCard
  - SocialPostCard: image thumbnail, status badge, datetime, caption excerpt, platform tag, inline expand-to-edit for scheduled status
  - SocialPostCardFailed: magenta ATTENTION REQUIRED banner, FAILED badge, lastError, RETRY SYNC + DOWNLOAD IMAGE actions
  - SocialNextSlotCard: dashed border next-hour slot card, click opens compose panel
affects: [02-05-calendar-view, 02-06-instagram-oauth]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Component imports useSocialStore directly (not via props) for action calls
    - data-testid attributes on interactive elements for test targeting
    - watch() on prop change to trigger side effects (generateCaption on prefilledImageId)
    - computed() for isSubmitDisabled guard combining multiple reactive sources
    - ~ alias added to vitest.config.ts for component test import resolution

key-files:
  created:
    - apps/dj/app/components/social/SocialPostCompose.vue
    - apps/dj/app/components/social/SocialQueueGrid.vue
    - apps/dj/app/components/social/SocialPostCard.vue
    - apps/dj/app/components/social/SocialPostCardFailed.vue
    - apps/dj/app/components/social/SocialNextSlotCard.vue
    - apps/dj/app/components/social/__tests__/SocialPostCompose.test.ts
    - apps/dj/app/components/social/__tests__/SocialQueueGrid.test.ts
  modified:
    - apps/dj/vitest.config.ts

key-decisions:
  - "data-testid attributes on submit-btn, retry-btn, download-btn, post-card, failed-card, next-slot-card for deterministic test queries"
  - "SocialPostCard and SocialPostCardFailed access useSocialStore() directly (not via props) — consistent with store-first pattern"
  - "~ alias added to vitest.config.ts resolve.alias — needed for component imports using Nuxt ~ convention to resolve in test environment"

patterns-established:
  - "Component test mocks: vi.mock('~/stores/social') with vi.fn() for each action used in component"
  - "TDD cycle: failing test commit (test:) -> GREEN implementation commit (feat:) per task"

requirements-completed: [SOCL-05, SOCL-06, SOCL-07, SOCL-08, SOCL-09, SOCL-12, SOCL-14, SOCL-17]

# Metrics
duration: 25min
completed: 2026-03-22
---

# Phase 02 Plan 04: Social Scheduler Frontend Summary

**Collapsible compose panel, 3-column queue grid with inline editing, failed post cards with magenta ATTENTION REQUIRED banner, and tab navigation — 21 new component tests GREEN, build passing**

## Performance

- **Duration:** 25 min
- **Started:** 2026-03-22T00:09:50Z
- **Completed:** 2026-03-22T00:25:00Z
- **Tasks:** 3
- **Files modified:** 8

## Accomplishments

- SocialPostCompose: Feed/Story toggle, caption with live char counter (N/2200 for feed, story note), image attachment from tracklist or upload, datetime+timezone combobox, submit disabled guard, auto-gen caption watch
- SocialQueueGrid + SocialPostCard + SocialPostCardFailed: 3-column grid routing; scheduled card expands inline to editable form; failed cards show magenta banner, lastError, RETRY SYNC + DOWNLOAD IMAGE buttons wired to store
- SocialNextSlotCard: dashed border with next whole-hour slot, click to open compose panel

## Task Commits

Each task was committed atomically:

1. **Task 2 RED: SocialPostCompose tests** - `d66d122` (test)
2. **Task 2 GREEN: SocialPostCompose implementation + ~ vitest alias** - `5bfddf1` (feat)
3. **Task 3 RED: SocialQueueGrid tests** - `3f09e09` (test)
4. **Task 3 GREEN: SocialQueueGrid + SocialPostCard + SocialPostCardFailed + SocialNextSlotCard** - `d5a5d05` (feat)

_Note: Task 1 (social.vue + SocialPageHeader + SocialTabs) was already committed in prior commits (1c0ea2d, 06a89f0). TDD tasks committed as RED then GREEN._

## Files Created/Modified

- `apps/dj/app/components/social/SocialPostCompose.vue` - Collapsible compose panel with full form, submit guard, auto-gen caption watch
- `apps/dj/app/components/social/SocialQueueGrid.vue` - 3-column grid routing to correct card type
- `apps/dj/app/components/social/SocialPostCard.vue` - Queue card with inline expand-to-edit for scheduled posts
- `apps/dj/app/components/social/SocialPostCardFailed.vue` - Failed post card with magenta banner and RETRY/DOWNLOAD actions
- `apps/dj/app/components/social/SocialNextSlotCard.vue` - Dashed border next-slot card
- `apps/dj/app/components/social/__tests__/SocialPostCompose.test.ts` - 9 tests: caption auto-gen, char counter, story note, submit guard
- `apps/dj/app/components/social/__tests__/SocialQueueGrid.test.ts` - 12 tests: card routing, RETRY/DOWNLOAD actions, ATTENTION REQUIRED content
- `apps/dj/vitest.config.ts` - Added `~` path alias for component test import resolution

## Decisions Made

- Used `data-testid` attributes on key interactive elements (submit-btn, retry-btn, download-btn, post-card, failed-card, next-slot-card) for deterministic test queries without CSS selector fragility
- SocialPostCard and SocialPostCardFailed call `useSocialStore()` directly rather than receiving store as prop — consistent with the store-first composition pattern established in Phase 1.5

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added ~ path alias to vitest.config.ts**
- **Found during:** Task 2 (SocialPostCompose GREEN)
- **Issue:** Component used `~/composables/useSocialPostForm` (Nuxt ~ convention), but vitest config had no resolve alias — import failed with "Could not load ~/composables/..."
- **Fix:** Added `resolve.alias: { '~': path.resolve(import.meta.dirname, 'app') }` to vitest.config.ts
- **Files modified:** apps/dj/vitest.config.ts
- **Verification:** All 9 SocialPostCompose tests pass after alias added
- **Committed in:** 5bfddf1 (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 Rule 3 blocking)
**Impact on plan:** Fix required for component test resolution. No scope creep — existing store tests used relative imports and were unaffected.

## Issues Encountered

- vitest `--testPathPattern` flag in plan verify commands uses `|` which shell interprets as pipe; ran tests separately per suite (`pnpm nx test dj -- SocialPostCompose` and `pnpm nx test dj -- SocialQueueGrid`)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- All queue view components ready for 02-05 (calendar view) integration
- SocialQueueGrid is imported by SocialTabs; build passes with all components in place
- 21 new tests GREEN (9 SocialPostCompose + 12 SocialQueueGrid); total project tests: 109 GREEN

## Self-Check: PASSED

All 8 files exist. All 4 task commits verified (d66d122, 5bfddf1, 3f09e09, d5a5d05).

---
*Phase: 02-social-media-scheduler*
*Completed: 2026-03-22*
