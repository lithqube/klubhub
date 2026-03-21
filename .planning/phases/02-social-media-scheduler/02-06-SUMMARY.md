---
phase: 02-social-media-scheduler
plan: "06"
subsystem: ui
tags: [vue, nuxt, pinia, tracklist, social, instagram, navigation]

# Dependency graph
requires:
  - phase: 02-04
    provides: social.vue page with openComposePanel store action and imageId wiring
  - phase: 02-05
    provides: SocialPostCompose with prefilledImageId watch and caption auto-generation
provides:
  - TracklistExporter "Schedule to Instagram" CTA button post-export
  - navigateTo('/social?imageId=...') navigation from tracklist to social page
  - social.vue scroll-to-compose panel on imageId mount
  - data-compose-panel attribute on SocialPostCompose root for scroll targeting
affects:
  - 03-monetization
  - end-to-end-flows

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Post-export CTA pattern: track result refs, show button conditionally, navigate with query param"
    - "Cross-page prefill pattern: imageId query param -> store.openComposePanel(id) -> nextTick scroll"
    - "data-compose-panel attribute as scroll target for programmatic smooth scroll"

key-files:
  created: []
  modified:
    - apps/dj/app/components/tracklist/TracklistExporter.vue
    - apps/dj/app/pages/social.vue
    - apps/dj/app/components/social/SocialPostCompose.vue

key-decisions:
  - "lastExportStoryPath/lastExportSquarePath refs track export result — CTA only visible after successful export, cleared on each new export attempt"
  - "Prefer story format (1080x1920) for Instagram navigation; fall back to square if story not exported"
  - "navigateTo() (Nuxt built-in) used for navigation — not window.location.href"
  - "data-compose-panel on SocialPostCompose root enables querySelector scroll targeting without coupling parent to child DOM structure"

patterns-established:
  - "Export result state tracking: clear refs on new export, set on success, use for conditional CTA rendering"
  - "Cross-page deep-link: navigateTo with query param + onMounted param read + store action + nextTick scroll"

requirements-completed:
  - SOCL-17

# Metrics
duration: 10min
completed: 2026-03-22
---

# Phase 02 Plan 06: Schedule to Instagram CTA Summary

**TracklistExporter "Schedule to Instagram" ghost-border button navigates to /social?imageId= with auto-prefilled compose panel and smooth scroll on arrival**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-22T00:00:00Z
- **Completed:** 2026-03-22T00:10:00Z
- **Tasks:** 1
- **Files modified:** 3

## Accomplishments
- TracklistExporter.vue tracks story/square export result paths in reactive refs; "SCHEDULE TO INSTAGRAM" button appears only after a successful export
- Clicking the CTA navigates to `/social?imageId={encodedPath}` (story format preferred, square fallback)
- social.vue adds nextTick smooth scroll to `[data-compose-panel]` element when compose panel opens via imageId param
- SocialPostCompose.vue root element gains `data-compose-panel` attribute as scroll target
- Build passes with zero TypeScript errors

## Task Commits

Each task was committed atomically:

1. **Task 1: TracklistExporter CTA + social.vue imageId wiring** - `f67a8c0` (feat)

**Plan metadata:** _(final docs commit — see below)_

## Files Created/Modified
- `apps/dj/app/components/tracklist/TracklistExporter.vue` - Added lastExportStoryPath/lastExportSquarePath refs, scheduleToInstagram() function, and "SCHEDULE TO INSTAGRAM" ghost-border button in template
- `apps/dj/app/pages/social.vue` - Added nextTick scroll-to-compose after store.openComposePanel(imageId)
- `apps/dj/app/components/social/SocialPostCompose.vue` - Added data-compose-panel attribute to root div

## Decisions Made
- Track export result paths in local refs (not uiStore) since they are component-local state specific to the last export action
- Clear lastExportStoryPath and lastExportSquarePath on each new export attempt to prevent stale CTA from previous export
- navigateTo() (Nuxt auto-import) used for navigation, not window.location.href — consistent with plan spec and Nuxt conventions

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None - social.vue imageId wiring was already implemented in plan 02-04 as expected. Only needed to add scroll behavior and data-compose-panel attribute.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- SOCL-17 complete: end-to-end flow from tracklist export to Instagram post scheduling is wired
- Phase 02 social-media-scheduler feature work complete; monetization phase can proceed
- No blockers

---
*Phase: 02-social-media-scheduler*
*Completed: 2026-03-22*
