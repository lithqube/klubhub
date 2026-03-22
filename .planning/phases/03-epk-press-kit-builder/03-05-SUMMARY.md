---
phase: 03-epk-press-kit-builder
plan: "05"
subsystem: epk-frontend-components
tags: [vue, pinia, vitest, epk, frontend]
dependency_graph:
  requires:
    - 03-02 (EPK store, composables, types)
    - 03-03 (EPK service/handler — API contracts)
    - 03-04 (EpkBioSection pattern for component structure)
  provides:
    - EpkGigHighlightsSection (editable list + disabled import stub)
    - EpkPressQuotesSection (editable quote+source pairs)
    - EpkSocialLinksSection (7-platform inputs saved via settings API)
    - EpkContactInfoSection (name/email/phone saved via settings API)
    - EpkExportPanel (8 section toggles + gradient-cta export + history)
  affects:
    - apps/dj/app/pages/epk.vue (consumes all 5 components)
tech_stack:
  added: []
  patterns:
    - direct store property mutation (not storeToRefs) for vi.mock compatibility
    - debounced $fetch to settings API in social/contact components
    - reactive local state + watch for store sync
key_files:
  created:
    - apps/dj/app/components/epk/EpkGigHighlightsSection.vue
    - apps/dj/app/components/epk/EpkPressQuotesSection.vue
    - apps/dj/app/components/epk/EpkSocialLinksSection.vue
    - apps/dj/app/components/epk/EpkContactInfoSection.vue
    - apps/dj/app/components/epk/EpkExportPanel.vue
    - apps/dj/app/components/epk/__tests__/EpkExportPanel.test.ts
    - apps/dj/app/components/epk/__tests__/EpkGigHighlightsSection.test.ts
  modified: []
decisions:
  - EpkSocialLinksSection and EpkContactInfoSection call settings API directly (PUT /api/v1/settings) — social_links and contact_info live in user_settings, not EPK content endpoint
  - Import from Gigs stub is visible but disabled with title tooltip — deferred until Gig Tracker phase
  - EpkExportPanel initializes missing sectionVisibility keys to true — defensive default for PDF generation
metrics:
  duration: "8 min"
  completed_date: "2026-03-22"
  tasks: 2
  files: 7
---

# Phase 03 Plan 05: EPK Remaining Sections + Export Panel Summary

**One-liner:** 5 EPK section components with CRUD lists, auto-save, 8-section export panel with history — 19 tests green.

## Tasks Completed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 | EpkGigHighlightsSection + EpkPressQuotesSection + EpkSocialLinksSection + EpkContactInfoSection | ed88686 | 4 new components |
| 2 | EpkExportPanel + export panel and highlights tests | b8d3ba1 | 1 component, 2 test files |

## What Was Built

### EpkGigHighlightsSection
- Editable list of text inputs with per-item delete buttons
- "+ ADD HIGHLIGHT" button appends empty entry
- "IMPORT FROM GIGS" stub: disabled=true, title="Coming Soon — available after Gig Tracker is set up", data-testid="import-from-gigs-btn"
- Auto-saves via useEpkAutosave scheduleSave({ gigHighlights: [...] })

### EpkPressQuotesSection
- Editable list of Textarea (quote text) + Input (source) pairs
- "+ ADD QUOTE" button appends empty PressQuote
- Per-entry delete (×) button
- Auto-saves via scheduleSave({ pressQuotes: [...] })

### EpkSocialLinksSection
- 7 platform inputs: instagram, soundcloud, spotify, mixcloud, residentAdvisor, bandcamp, youtube
- Loads from GET /api/v1/settings on mount
- Debounced auto-save to PUT /api/v1/settings with { social_links: {...} }

### EpkContactInfoSection
- 3 inputs: name, email (type="email"), phone (type="tel")
- Loads from GET /api/v1/settings on mount
- Debounced auto-save to PUT /api/v1/settings with { contact_info: {...} }

### EpkExportPanel
- 8 Switch toggles for section visibility (bio, photos, gigHighlights, pressQuotes, techRider, stagePlot, socialLinks, contactInfo)
- Toggle changes call scheduleSave({ sectionVisibility: {...} })
- EXPORT PDF button: class="gradient-cta w-full", data-testid="export-pdf-btn"
- Loading state: Progress bar (animate-pulse) + disabled button during generation
- handleExport() calls store.generateExport() then store.loadExports()
- Export history: newest-first list with DOWNLOAD link + × delete button
- saveStatus indicator: shows "SAVED" text when store.saveStatus === 'saved'

## Verification Results

- Tests: 19/19 passing (11 EpkExportPanel + 8 EpkGigHighlightsSection)
- TypeCheck: no errors on new components (30 pre-existing TS6305 build output errors unrelated)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Architecture] EpkSocialLinksSection and EpkContactInfoSection use settings API directly**
- **Found during:** Task 1
- **Issue:** Backend PUT /api/v1/epk/content does not accept socialLinks or contactInfo fields; these fields live in user_settings per the Go model. The plan mentioned "use whatever field name the backend API uses" which confirmed this was expected.
- **Fix:** Components call PUT /api/v1/settings with `{ social_links: {...} }` and `{ contact_info: {...} }` respectively, with onMounted load from GET /api/v1/settings
- **Files modified:** EpkSocialLinksSection.vue, EpkContactInfoSection.vue
- **Commit:** ed88686

## Self-Check: PASSED

Files verified:
- FOUND: apps/dj/app/components/epk/EpkGigHighlightsSection.vue
- FOUND: apps/dj/app/components/epk/EpkPressQuotesSection.vue
- FOUND: apps/dj/app/components/epk/EpkSocialLinksSection.vue
- FOUND: apps/dj/app/components/epk/EpkContactInfoSection.vue
- FOUND: apps/dj/app/components/epk/EpkExportPanel.vue
- FOUND: apps/dj/app/components/epk/__tests__/EpkExportPanel.test.ts
- FOUND: apps/dj/app/components/epk/__tests__/EpkGigHighlightsSection.test.ts

Commits verified:
- FOUND: ed88686
- FOUND: b8d3ba1
