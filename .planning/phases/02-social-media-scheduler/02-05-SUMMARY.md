---
phase: 02-social-media-scheduler
plan: "05"
subsystem: ui
tags: [vue, vitest, social, instagram, calendar, oauth, lucide-vue-next]

# Dependency graph
requires:
  - phase: 02-03
    provides: social store (useSocialStore), types (SocialAccount, ScheduledPost, PostStatus)
provides:
  - SocialConnectionBanner.vue: full-width OAuth CTA shown when account null/disconnected
  - SocialTokenWarningBanner.vue: conditional banners for token expiry (amber) and disconnected (magenta)
  - SocialCalendarView.vue: monthly calendar grid with post status dot indicators and day-click filter
  - socialCalendarUtils.ts: dotColorForStatus pure helper for post status to Tailwind class mapping
  - SocialTokenWarningBanner.test.ts: 4 Vitest tests for conditional banner rendering
  - SocialCalendarView.test.ts: 9 Vitest tests for postsByDate computation, dot colors, day-click emit
affects:
  - 02-06 (social page wiring — will import all four components)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Named exports from script setup not allowed in Vue SFC; extract pure helpers to co-located .ts utility files
    - dotColorForStatus extracted to socialCalendarUtils.ts for direct Vitest testing without mounting component
    - defineExpose used to expose reactive computed (postsByDate) for test assertions
    - Calendar grid built with plain Date API (no date library) using Monday-start week adjustment

key-files:
  created:
    - apps/dj/app/components/social/SocialConnectionBanner.vue
    - apps/dj/app/components/social/SocialTokenWarningBanner.vue
    - apps/dj/app/components/social/SocialCalendarView.vue
    - apps/dj/app/components/social/socialCalendarUtils.ts
    - apps/dj/app/components/social/__tests__/SocialCalendarView.test.ts
  modified:
    - apps/dj/app/components/social/__tests__/SocialTokenWarningBanner.test.ts (pre-existing RED test, now GREEN)

key-decisions:
  - "dotColorForStatus extracted to socialCalendarUtils.ts (not inline export from SFC) — Vue script setup does not allow named exports"
  - "SocialCalendarView accepts optional month prop for deterministic test rendering without mocking Date.now()"
  - "defineExpose({ postsByDate }) enables test assertions on computed Map without DOM traversal"
  - "TheNav Social nav entry was pre-existing — no changes needed to TheNav.vue"

patterns-established:
  - "Pure helpers in co-located *.ts file alongside Vue SFC — testable without component mount"
  - "Optional override props (month?) for time-sensitive components to enable deterministic tests"

requirements-completed:
  - SOCL-01
  - SOCL-02
  - SOCL-09
  - SOCL-15

# Metrics
duration: 25min
completed: 2026-03-22
---

# Phase 02 Plan 05: Social Connection UI, Token Banners, and Calendar View Summary

**OAuth connection CTA banner, conditional token warning banners (amber expiry + magenta disconnected), and monthly calendar grid with post status dots — 13 Vitest tests GREEN**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-03-22T00:10:00Z
- **Completed:** 2026-03-22T00:15:00Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- SocialConnectionBanner fetches `/api/v1/social/auth/url` and redirects to OAuth; shown when account null or disconnected
- SocialTokenWarningBanner conditionally renders amber (token expiry ≤7 days) or magenta (disconnected) banner with RE-AUTHORIZE CTA; renders nothing otherwise (4 tests GREEN)
- SocialCalendarView renders monthly grid with Monday-start layout, up to 3 dot indicators per day colored by post status, day-click emit with toggle-to-clear behavior (9 tests GREEN)
- dotColorForStatus helper extracted to socialCalendarUtils.ts: scheduled/draft→cyan, published/publishing→gray, failed/permanently_failed→magenta

## Task Commits

1. **Task 1: SocialConnectionBanner + SocialTokenWarningBanner + tests** - `06a89f0` (feat)
2. **Task 2: SocialCalendarView + calendar utility tests** - `46c4328` (feat)

## Files Created/Modified

- `apps/dj/app/components/social/SocialConnectionBanner.vue` - OAuth CTA shown when no account / disconnected
- `apps/dj/app/components/social/SocialTokenWarningBanner.vue` - Conditional amber/magenta warning banners
- `apps/dj/app/components/social/SocialCalendarView.vue` - Monthly calendar grid with dot indicators and day-click filter emit
- `apps/dj/app/components/social/socialCalendarUtils.ts` - dotColorForStatus pure helper, exported for direct Vitest testing
- `apps/dj/app/components/social/__tests__/SocialTokenWarningBanner.test.ts` - 4 tests (pre-existing RED, now GREEN)
- `apps/dj/app/components/social/__tests__/SocialCalendarView.test.ts` - 9 tests: dot colors, postsByDate, day-click emit

## Decisions Made

- **dotColorForStatus extracted to co-located .ts file** — Vue `<script setup>` does not allow named exports (compiler error). Extracting to `socialCalendarUtils.ts` allows both the SFC and the test to import it directly.
- **Optional `month` prop on SocialCalendarView** — Allows test to render a deterministic month (October 2026) without mocking `Date.now()`, keeping tests stable and predictable.
- **defineExpose for postsByDate** — Test assertions on the computed Map require access to the internal reactive state; `defineExpose` is the idiomatic way in `<script setup>` components.
- **TheNav pre-existing** — TheNav.vue already included the Social nav entry (`Send` icon, `/social` route). No changes needed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Moved dotColorForStatus from SFC to co-located utility file**
- **Found during:** Task 2 (SocialCalendarView implementation)
- **Issue:** Plan specified `export function dotColorForStatus` inside `<script setup>` — Vue SFC compiler rejects named exports from setup scripts with a compile error
- **Fix:** Extracted function to `socialCalendarUtils.ts`, imported in both SFC and test file
- **Files modified:** `apps/dj/app/components/social/socialCalendarUtils.ts` (new), `SocialCalendarView.vue`, `SocialCalendarView.test.ts`
- **Verification:** 9 SocialCalendarView tests GREEN including 6 dotColorForStatus unit tests
- **Committed in:** `46c4328` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 3 — blocking compile error)
**Impact on plan:** Necessary structural adjustment; utility file pattern is cleaner and more testable than inline helper.

## Issues Encountered

- Vitest v4 removed `--testPathPattern` flag (Jest-compat flag). Used positional filter argument instead (`pnpm nx test dj -- SocialCalendarView`). No code impact.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- All four components ready for wiring into the Social page (Plan 02-06)
- SocialConnectionBanner, SocialTokenWarningBanner, SocialCalendarView all export correctly
- TheNav Social entry already active — `/social` route navigation works
- 97 total tests across the project remain GREEN (no regressions)

---
*Phase: 02-social-media-scheduler*
*Completed: 2026-03-22*
