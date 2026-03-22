---
phase: 03-epk-press-kit-builder
plan: "04"
subsystem: ui
tags: [typescript, vue, pinia, epk, components, tdd]

# Dependency graph
requires:
  - phase: 03-03
    provides: useEpkStore Pinia store and useEpkAutosave composable
provides:
  - EpkBioSection — short bio Input + char counter + long bio Markdown Textarea
  - EpkPhotosSection — 4-column photo grid + drag-and-drop upload zone + delete overlay
  - EpkTechRiderSection — tech rider Textarea with auto-save
  - EpkStagePlotSection — stage plot image preview or drop zone upload
affects:
  - 03-05-epk-remaining-sections
  - 03-06-epk-export-ui

# Tech tracking
tech-stack:
  added:
    - Textarea UI component (apps/dj/app/components/ui/textarea/Textarea.vue)
  patterns:
    - Explicit Vue component imports (not Nuxt auto-imports) for vitest test compatibility
    - Direct store property access via ref/watch instead of storeToRefs — avoids vi.mock incompatibility
    - reactive() store mock with getter properties for computed canAddPhoto/photoCount
    - @ alias added to vitest.config.ts — resolves @/lib/utils in UI components during tests

key-files:
  created:
    - apps/dj/app/components/epk/EpkBioSection.vue
    - apps/dj/app/components/epk/EpkPhotosSection.vue
    - apps/dj/app/components/epk/EpkTechRiderSection.vue
    - apps/dj/app/components/epk/EpkStagePlotSection.vue
    - apps/dj/app/components/ui/textarea/Textarea.vue
    - apps/dj/app/components/epk/__tests__/EpkBioSection.test.ts
    - apps/dj/app/components/epk/__tests__/EpkPhotosSection.test.ts
  modified:
    - apps/dj/vitest.config.ts

key-decisions:
  - "Direct store property access via ref/watch (not storeToRefs) — storeToRefs incompatible with vi.mock plain objects in tests (consistent with Phase 02 useSocialPostForm decision)"
  - "Explicit component imports in EPK components — Nuxt auto-imports not resolved in vitest test environment"
  - "@ alias added to vitest.config.ts pointing to app/ directory — required for Input.vue which imports @/lib/utils"
  - "reactive() for store mocks in tests — auto-unwraps nested refs and supports getter-based computed properties"

requirements-completed: [EPK-01, EPK-02, EPK-03]

# Metrics
duration: 14min
completed: 2026-03-22
---

# Phase 3 Plan 04: EPK Content Section Components Summary

**4 EPK content section Vue components (Bio, Photos, TechRider, StagePlot) + Textarea UI component + 17 tests GREEN — 154 total tests passing**

## Performance

- **Duration:** 14 min
- **Started:** 2026-03-22T19:43:22Z
- **Completed:** 2026-03-22T19:57:30Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Created EpkBioSection.vue: short bio Input (maxlength=500) with 'N / 280' counter, long bio Markdown Textarea (font-mono text-xs), both auto-save via useEpkAutosave; all data-testid attributes correct
- Created EpkPhotosSection.vue: 4-column CSS grid (grid-cols-4 gap-2), square thumbnails (aspect-square), hover × delete button overlay, ghost-border drag-and-drop upload zone hidden at 20 photos with replacement label
- Created EpkTechRiderSection.vue: full-width Textarea bound to store.techRider with auto-save on change
- Created EpkStagePlotSection.vue: conditional image preview (proxy URL) or ghost-border drop zone, stores uploaded filename
- Created Textarea.vue UI component following Input.vue pattern (useVModel + passive)
- 9 EpkBioSection tests + 8 EpkPhotosSection tests all GREEN; 154 total tests GREEN

## Task Commits

1. **Task 1: EpkBioSection + EpkTechRiderSection + EpkStagePlotSection** - `b387edc` (feat)
2. **TDD RED: failing tests for EpkBioSection and EpkPhotosSection** - `6355c02` (test)
3. **Task 2 GREEN: EpkPhotosSection + all tests passing** - `73b0aa2` (feat)

## Files Created/Modified

- `apps/dj/app/components/epk/EpkBioSection.vue` — short bio Input + counter + long bio Textarea; auto-saves via useEpkAutosave
- `apps/dj/app/components/epk/EpkPhotosSection.vue` — 4-col grid + upload zone + delete overlay + 20/20 limit enforcement
- `apps/dj/app/components/epk/EpkTechRiderSection.vue` — Textarea bound to techRider; auto-saves
- `apps/dj/app/components/epk/EpkStagePlotSection.vue` — stagePlotPath-conditional preview or drop zone
- `apps/dj/app/components/ui/textarea/Textarea.vue` — new UI component following Input.vue pattern
- `apps/dj/app/components/epk/__tests__/EpkBioSection.test.ts` — 9 tests for bio section
- `apps/dj/app/components/epk/__tests__/EpkPhotosSection.test.ts` — 8 tests for photos section
- `apps/dj/vitest.config.ts` — added @ alias to resolve @/lib/utils in UI components during tests

## Decisions Made

- Direct store property access via `ref(store.bioShort)` + `watch` instead of `storeToRefs` — consistent with Phase 02 decision that storeToRefs is incompatible with vi.mock plain objects
- Explicit Vue imports in component files (not Nuxt auto-imports) — required for vitest to resolve components
- `reactive()` for store mocks in tests — allows getter properties for canAddPhoto/photoCount without managing separate refs
- `@` alias added to vitest.config.ts — blocked by Input.vue's `@/lib/utils` import; required for tests to run

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Missing dep] Created Textarea UI component**
- **Found during:** Task 1
- **Issue:** Textarea.vue not present in apps/dj/app/components/ui/ — required by EpkBioSection and EpkTechRiderSection
- **Fix:** Created Textarea.vue following Input.vue pattern with useVModel(passive) + vueuse
- **Files modified:** apps/dj/app/components/ui/textarea/Textarea.vue
- **Commit:** b387edc

**2. [Rule 3 - Blocking] Added @ alias to vitest.config.ts**
- **Found during:** Task 2 (TDD GREEN)
- **Issue:** Input.vue imports `@/lib/utils` which was not resolved in vitest test environment
- **Fix:** Added `'@': path.resolve(import.meta.dirname, 'app')` to vitest.config.ts resolve.alias
- **Files modified:** apps/dj/vitest.config.ts
- **Commit:** 73b0aa2

**3. [Rule 1 - Bug] Replaced storeToRefs with direct store ref/watch pattern**
- **Found during:** Task 2 (TDD RED — tests failing)
- **Issue:** `storeToRefs` on vi.mock plain objects doesn't return reactive refs — tests reading `element.value` got empty string
- **Fix:** `ref(store.bioShort)` + `watch(() => store.bioShort, ...)` pattern for local reactive state; consistent with Phase 02 state decision
- **Files modified:** EpkBioSection.vue, EpkTechRiderSection.vue, EpkStagePlotSection.vue
- **Commit:** 73b0aa2

## Issues Encountered

- pnpm nx run dj:typecheck has pre-existing TS6305 errors (cached dist outputs out of sync with source) — out of scope, not caused by this plan

## Next Phase Readiness

- All 4 content section components ready for integration into EPK page (Plan 05/06)
- EpkPhotosSection ready for photo upload flow — uploadPhoto/deletePhoto wired to store
- EpkStagePlotSection ready — uploadStagePlot wired
- Pattern established: use explicit imports + reactive() mocks for all future EPK component tests

---
*Phase: 03-epk-press-kit-builder*
*Completed: 2026-03-22*
