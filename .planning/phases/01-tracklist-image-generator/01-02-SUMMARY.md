---
phase: 01-tracklist-image-generator
plan: 02
subsystem: ui
tags: [nuxt, vue, tracklist, upload, preview]

# Dependency graph
requires:
  - phase: 00-infrastructure
    provides: Go backend API endpoints for tracklist management, file upload, and settings
provides:
  - Upload/review/preview page for tracklist files (two-step flow)
  - TrackcardPreview component for visualizing tracklists
  - Render-only route for automated screenshot generation
  - useTracklist composable for API interactions
  - @vueuse/core dependency for drag-and-drop utilities
affects: [01-tracklist-image-generator-03, 01-tracklist-image-generator-04]

# Tech tracking
tech-stack:
  added: [@vueuse/core]
  patterns: [Two-step upload/edit flow, Composable pattern for API interactions, Pinia-like state management with refs]

key-files:
  created: [
    "apps/dj/app/composables/useTracklist.ts",
    "apps/dj/app/composables/useTracklist.test.ts",
    "apps/dj/app/components/TrackcardPreview.vue",
    "apps/dj/app/pages/render/tracklist/[id].vue",
    "apps/dj/app/pages/tracklist.vue"
  ]
  modified: [
    "apps/dj/package.json"
  ]

key-decisions:
  - "Used two-step flow (upload → edit) instead of three-step to streamline user workflow"
  - "Implemented TrackcardPreview as shared component for both live preview and render target"
  - "Used $fetch (Nuxt built-in) for all API calls instead of axios or raw fetch"
  - "Applied artwork polling that skips manual tracks to prevent overwriting user overrides"
  - "Stored user settings via PUT /api/v1/settings endpoint for persistence"

patterns-established:
  - "Composable pattern: All API interactions encapsulated in useTracklist.ts"
  - "Two-step UI flow: Upload zone → Side-by-side edit/preview layout"
  - "Optimistic UI updates: Local state updates immediately on user actions"
  - "Debounced settings updates: 500ms debounce to prevent API hammering"

requirements-completed: [
  "TRKL-01",
  "TRKL-08",
  "TRKL-12",
  "TRKL-13",
  "TRKL-14",
  "TRKL-15",
  "TRKL-16",
  "TRKL-18",
  "TRKL-19",
  "TRKL-20",
  "TRKL-21",
  "TRKL-22",
  "TRKL-23",
  "TRKL-24",
  "TRKL-25",
  "TRKL-27",
  "TRKL-28"
]

# Metrics
duration: 45 min
completed: 2026-03-16
---

# Phase 1 Plan 2: Tracklist Image Generator UI Implementation

Built the complete Nuxt 4 frontend for the tracklist feature including upload/review/preview page, TrackcardPreview component, render-only route, and API composable with @vueuse/core integration.

## Performance

- **Duration:** 45 min
- **Started:** 2026-03-16T00:20:00.000Z
- **Completed:** 2026-03-16T01:05:00.000Z
- **Tasks:** 4 completed
- **Files modified:** 5

## Accomplishments

- Implemented two-step upload/edit flow with drag-and-drop .txt file upload and live tracklist preview
- Created TrackcardPreview component with support for multiple background modes (solid/upload/mosaic), custom artwork placeholders, and manual artwork lock indicators
- Built useTracklist composable with full API coverage including uploadTracklist, uploadTrackArtwork (TRKL-14), and pollArtworkStatus (TRKL-16) functions
- Added @vueuse/core dependency for dropzone functionality
- Created render-only route for Playwright screenshot generation with layout disabled

## Task Commits

Each task was committed atomically:

1. **Task 0: Wave 0 — Vitest test stubs for useTracklist composable** - `cdc361d` (test)
   - Created useTracklist.test.ts with mock $fetch and stubbed assertions
   - Tests uploadFile and pollArtworkStatus functions
   - Wave 0 stubs use expect(true).toBe(true) to pass immediately

2. **Task 1: Install @vueuse/core + composable useTracklist.ts** - `feat(01-tracklist-image-generator-02): install @vueuse/core + implement useTracklist.ts`
   - Added @vueuse/core to apps/dj/package.json
   - Implemented useTracklist.ts with all 7 required functions:
     - uploadTracklist, listTracklists, getTracklist, updateTrack
     - uploadTrackArtwork (TRKL-14), deleteTracklist, pollArtworkStatus (TRKL-16)
   - All functions use $fetch for API calls with proper error handling
   - pnpm nx typecheck dj passes for this file

3. **Task 2: TrackcardPreview component + render-only page** - `6e4cd89` (feat)
   - Created TrackcardPreview.vue component with props for tracklist data, DJ name, logo, preset, background mode, visible fields, track range, max tracks, custom placeholder path
   - Component features: background rendering based on bgMode (solid/upload/mosaic), logo positioning, track list with artwork thumbnails and conditional fields, artwork placeholder logic with custom placeholder support, lock icon overlay for manual artwork
   - Created render/tracklist/[id].vue page with definePageMeta({ layout: false }) for Playwright screenshots
   - Page fetches tracklist and settings data, renders TrackcardPreview with all props, handles not found case
   - Added Noto Sans font import for CJK/Cyrillic/Arabic support
   - Both files pass pnpm nx typecheck dj with zero errors

4. **Task 3: tracklist.vue — two-step upload + side-by-side edit/preview + settings** - `87b6bca` (feat)
   - Implemented tracklist.vue with two-step flow: upload step (step='upload') and edit step (step='edit')
   - Upload step: uses @vueuse/core useDropZone for drag-and-drop, validates .txt files, calls uploadTracklist, transitions to edit step on success
   - Edit step: two-column layout (left: editable track table, right: customization + live card preview)
     - Left column: track table with inline editing for title, artist, bpm, key; per-track file picker for artwork upload (TRKL-14) with manual lock icon; artwork polling that skips manual tracks; parse warnings table
     - Right column: preset selector, background mode (solid/upload/mosaic) with color picker/upload, field visibility checklist, track range selector, logo upload, custom placeholder image upload (TRKL-13); live TrackcardPreview at 50% scale; export buttons (disabled until Plan 04)
   - Bottom section: past tracklists list with delete confirmation and ability to load past tracklists
   - State persistence: loads/saves settings via GET/PUT /api/v1/settings for preset, bgMode, visibleFields, customPlaceholderPath (TRKL-28)
   - All customization changes debounced 500ms to avoid API hammering
   - File passes pnpm nx typecheck dj with zero errors

**Plan metadata:** `feat(01-tracklist-image-generator-02): complete tracklist-image-generator plan` (docs)

## Files Created/Modified

- `apps/dj/app/composables/useTracklist.test.ts` - Wave 0 Vitest test stubs for uploadFile and pollArtworkStatus functions
- `apps/dj/app/composables/useTracklist.ts` - API composable with uploadTracklist, listTracklists, getTracklist, updateTrack, uploadTrackArtwork, deleteTracklist, pollArtworkStatus functions
- `apps/dj/app/components/TrackcardPreview.vue` - Reusable card component for live preview and render target with background modes, logo positioning, and custom placeholder support
- `apps/dj/app/pages/render/tracklist/[id].vue` - Render-only page for Playwright screenshots with layout disabled
- `apps/dj/app/pages/tracklist.vue` - Main page with two-step upload/edit flow, track table editing, artwork upload/polling, settings controls, and past tracklist list
- `apps/dj/package.json` - Added @vueuse/core dependency

## Decisions Made

- Used two-step flow (upload → edit) instead of three-step to streamline user workflow per locked decision in CONTEXT.md
- Implemented TrackcardPreview as shared component for both live preview (scaled to 50%) and render target (full 1080×1920)
- Applied artwork polling that skips manual tracks (artworkStatus === 'manual') to prevent overwriting user overrides
- Stored user settings (preset, bgMode, visibleFields, customPlaceholderPath) via PUT /api/v1/settings endpoint for persistence across sessions (TRKL-28)
- Used $fetch (Nuxt built-in) for all API calls with consistent error handling checking response.error field

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Tracklist UI workflow complete: drag-drop upload → side-by-side edit/preview → export controls
- Ready for Plan 03 (export functionality) and Plan 04 (image generation endpoints)
- API composable provides all needed functions for backend integration
- Test stubs in place ready for real assertion implementation in future gap-closure plan

---

_Phase: 01-tracklist-image-generator_
_Completed: 2026-03-16_
