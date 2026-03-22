---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
current_phase: 03-epk-press-kit-builder
current_plan: 03-02 complete (2 of N plans in phase 03)
status: executing
last_updated: "2026-03-22T20:18:09.867Z"
progress:
  total_phases: 13
  completed_phases: 4
  total_plans: 31
  completed_plans: 30
---

# Session State

## Project Reference

See: .planning/PROJECT.md

## Position

**Milestone:** v1.0 milestone
**Current phase:** 03-epk-press-kit-builder
**Current plan:** 03-02 complete (2 of N plans in phase 03)
**Completed phase:** 02-social-media-scheduler
**Status:** Phase 03 in progress — 03-02 complete
**Note:** Phase 1.5 inserted between Phase 1 and Phase 2 — now complete. Phase 02 also complete. Both verified.

## Decisions

- [Phase 01-tracklist-image-generator]: Position column (#) parsed from Rekordbox TSV and stored in database
- [Phase 01-tracklist-image-generator]: Soft delete implemented with TODO for Phase 4 gigs NULL cascade (TRKL-27)
- [Phase 01-tracklist-image-generator]: Storage interface uses io.Reader for MinIO uploads
- [Phase 01-tracklist-image-generator]: UTF-16 LE BOM detection with BOMOverride fallback
- [Phase 01-tracklist-image-generator]: Used two-step flow (upload → edit) instead of three-step to streamline user workflow
- [Phase 01-tracklist-image-generator]: Implemented TrackcardPreview as shared component for both live preview and render target
- [Phase 01-tracklist-image-generator]: Applied artwork polling that skips manual tracks to prevent overwriting user overrides
- [Phase 01-tracklist-image-generator]: Stored user settings via PUT /api/v1/settings endpoint for persistence across sessions
- [Phase 01-tracklist-image-generator]: Used $fetch (Nuxt built-in) for all API calls with consistent error handling
- [Phase 01-tracklist-image-generator]: Implemented artwork fetch pipeline (Spotify → Discogs → MusicBrainz/CAA) with MinIO cache and worker pool
- [Phase 01-tracklist-image-generator]: Implemented Playwright screenshot pipeline with Nitro plugin singleton, screenshot API route, Go generate-image endpoint
- [Phase 00-infrastructure]: TOKEN_ENCRYPTION_KEY uses :? (error if empty) — no safe default, must be set explicitly
- [Phase 00-infrastructure]: NUXT_PUBLIC_API_BASE used in routeRules proxy target to resolve correctly within Docker network
- [Phase 00-infrastructure]: Dev overlay uses profiles: [prod-only] to exclude api/frontend from local dev startup
- [Phase 00-infrastructure]: goose migration files use single-file convention (001_name.sql with Up/Down sections) not separate .up.sql/.down.sql files
- [Phase 00-infrastructure]: DBPinger and StorageHealthChecker interfaces allow health handler to be unit-tested without Docker
- [Phase 1.5-design-system-foundation]: Use @tailwindcss/vite Vite plugin (not @nuxtjs/tailwindcss module) — Tailwind v4 recommended approach
- [Phase 1.5-design-system-foundation]: All design tokens in @theme CSS block — Tailwind v4 CSS-only configuration, no tailwind.config.ts
- [Phase 1.5-design-system-foundation]: Fonts downloaded to public/fonts/ from GitHub source repos (not npm packages) — no CDN dependency
- [Phase 1.5-design-system-foundation]: All --radius tokens set to 0px — enforces zero rounded corners for Cyberpunk HUD aesthetic
- [Phase 1.5-design-system-foundation]: nav-item-active uses dashed border all 4 sides (not solid fill, not left-only) — per mockup sidebar active state
- [Phase 1.5-design-system-foundation]: PRESETS in design-tokens.ts are image-generation colors only — separate from CSS UI theme tokens in styles.css (no overlap)
- [Phase 1.5-design-system-foundation]: Pinia stores use setup function (composition API) style — not options API
- [Phase 1.5-design-system-foundation]: abortController in useTracklistStore is private (not returned) — internal implementation detail for cancellable uploads
- [Phase 1.5-design-system-foundation]: storeToRefs() enforced for reactive store destructuring — raw destructuring (const { step } = store) destroys reactivity
- [Phase 1.5-design-system-foundation]: TrackcardPreview.vue uses withDefaults(defineProps<Props>()) TypeScript-generic style — more compact than runtime props for orchestrators
- [Phase 1.5-design-system-foundation]: colors computed once in TrackcardPreview.vue orchestrator, passed as prop — avoids re-importing PRESETS in sub-components
- [Phase 1.5-design-system-foundation]: TrackcardPreviewPanel.vue uses bg-surface-container (opaque) per mockup spec override — not glass-panel
- [Phase 1.5]: Progress bars use fake increment timers since API has no streaming; upload: 0-85% on interval then 100% on success; export: indeterminate pulse
- [Phase 02-social-media-scheduler]: useSocialPostForm uses computed() for djName access — storeToRefs() incompatible with vi.mock plain object in tests
- [Phase 02-social-media-scheduler]: retryPost() updates individual post in-store by index splice rather than full reload to avoid loading flash
- [Phase 02-social-media-scheduler]: createPost() calls loadPosts() after POST to keep store in sync with server state
- [Phase 02-social-media-scheduler]: UNIQUE constraint on social_accounts(platform) added in migration 003 (not 001) to enable ON CONFLICT upsert
- [Phase 02-social-media-scheduler]: Two-level interface stack: handler→serviceIface, service→repoIface for independent mock testing without testcontainers
- [Phase 02-social-media-scheduler]: ValidateImage order: MIME → size → dimensions — cheapest checks first for fast rejection
- [Phase 02-social-media-scheduler]: parseDateTimeLocal uses time.LoadLocation + ParseInLocation for IANA timezone-aware UTC conversion
- [Phase 02-social-media-scheduler]: dotColorForStatus extracted to socialCalendarUtils.ts (not inline export from SFC) — Vue script setup does not allow named exports
- [Phase 02-social-media-scheduler]: SocialCalendarView accepts optional month prop for deterministic test rendering without mocking Date.now()
- [Phase 02-social-media-scheduler]: data-testid attributes on submit-btn, retry-btn, download-btn, post-card, failed-card, next-slot-card for deterministic test queries
- [Phase 02-social-media-scheduler]: ~ alias added to vitest.config.ts resolve.alias — needed for component imports using Nuxt ~ convention to resolve in test environment
- [Phase 02-social-media-scheduler]: Worker uses workerRepoIface/instagramIface/storageIface thin interfaces for full-mock unit testing without testcontainers
- [Phase 02-social-media-scheduler]: signal.NotifyContext added in main.go for SIGTERM/SIGINT — ensures worker goroutine exits cleanly on container shutdown
- [Phase 02-social-media-scheduler]: Worker.tick swallows RateLimitError (returns nil) — stops tick loop early without propagating error
- [Phase 02-social-media-scheduler]: lastExportStoryPath/lastExportSquarePath refs track export result in TracklistExporter; CTA only visible after successful export, cleared on each new export attempt
- [Phase 02-social-media-scheduler]: navigateTo('/social?imageId=...') with nextTick scroll to [data-compose-panel] used for cross-page prefill flow from TracklistExporter to SocialPostCompose
- [Phase 02-social-media-scheduler]: Option 1 chosen for gap #19 closure: remove dead code and update verification truth (not implement active PNG→JPEG conversion) — URL-based uploads accept PNG natively
- [Phase 1.5-design-system-foundation]: index.vue is a full Dashboard page (not a /tracklist redirect) — positive deviation; Dashboard is the correct entry point
- [Phase 1.5-design-system-foundation]: Responsive shell: TheNav sidebar hidden lg:flex + TheMobileHeader + TheBottomNav for mobile — dual-layout pattern at lg breakpoint
- [Phase 1.5-design-system-foundation]: min-h-dvh used in app.vue root element instead of min-h-screen — avoids mobile-browser-chrome viewport bug
- [Phase 03-epk-press-kit-builder]: Singleton epk_content row enforced at DB level via CREATE UNIQUE INDEX ON epk_content ((true)) — constant expression prevents >1 row
- [Phase 03-epk-press-kit-builder]: UpsertContent CASE WHEN value != '' preserves existing text fields when caller passes empty string (nil pointer = no update intent)
- [Phase 03-epk-press-kit-builder]: JSONB fields (gig_highlights, press_quotes, photo_paths, section_visibility) replace fully on each upsert — no partial JSONB merge
- [Phase 03-epk-press-kit-builder]: EPK unit tests use hand-written mockRepo (interface-substitution) — no testcontainers, no pgxmock — consistent with social service_test.go pattern
- [Phase 03-epk-press-kit-builder]: Thin storageIface (contentType string) + StorageClientAdapter bridges storage.Client (minio.PutObjectOptions) — service layer free of minio import
- [Phase 03-epk-press-kit-builder]: settingsServiceAdapter maps GetOrCreate+Update to GetSettings+UpdateSettings — zero coupling changes to settings package
- [Phase 03-epk-press-kit-builder]: renderInline strips bold/italic/link markers to plain text — fpdf v0.11.1 MultiCell has no per-run font switching within a single call
- [Phase 03-epk-press-kit-builder]: RemoveObject added to storage.Client as wrapper — required for EPK photo delete and export delete operations
- [Phase 03-epk-press-kit-builder]: saveStatus is a plain ref (not computed) — allows direct mutation from both store and useEpkAutosave composable
- [Phase 03-epk-press-kit-builder]: useEpkAutosave sets store.saveStatus directly (not storeToRefs) — composable is internal collaborator, not consumer
- [Phase 03-epk-press-kit-builder]: Direct store property access via ref/watch (not storeToRefs) in EPK components — storeToRefs incompatible with vi.mock plain objects in tests
- [Phase 03-epk-press-kit-builder]: Explicit Vue imports in EPK components (not Nuxt auto-imports) — required for vitest component resolution
- [Phase 03-epk-press-kit-builder]: @ alias added to vitest.config.ts pointing to app/ — resolves @/lib/utils in UI components during tests
- [Phase 03-epk-press-kit-builder]: EpkSocialLinksSection and EpkContactInfoSection call settings API directly — social_links/contact_info live in user_settings, not EPK content endpoint
- [Phase 03-epk-press-kit-builder]: Import from Gigs stub visible but disabled with title tooltip — deferred until Gig Tracker phase
- [Phase 03-epk-press-kit-builder]: EPK inserted between SOCIAL and GIGS in both nav components; TheBottomNav replaces FINANCE with EPK keeping 5 items

## Performance Metrics

| Phase                          | Plan | Duration | Tasks | Files |
| ------------------------------ | ---- | -------- | ----- | ----- |
| 01-tracklist-image-generator   | 02   | 45 min   | 4     | 5     |
| 01-tracklist-image-generator   | 03   | 45 min   | 3     | 8     |
| 01-tracklist-image-generator   | 04   | 10 min   | 3     | 11    |
| 01-tracklist-image-generator   | 05   | 2 min    | 2     | 0     |
| 1.5-design-system-foundation   | 01   | 20 min   | 3     | 6     |
| 1.5-design-system-foundation   | 03   | 45 min   | 2     | 12    |
| 1.5-design-system-foundation   | 04   | 25 min   | 3     | 9     |
| Phase 1.5 P05 | 15 | 3 tasks | 6 files |
| Phase 02-social-media-scheduler P03 | 15 | 2 tasks | 5 files |
| Phase 02-social-media-scheduler P01 | 10 min | 2 tasks | 11 files |
| Phase 02-social-media-scheduler P05 | 25min | 2 tasks | 6 files |
| Phase 02-social-media-scheduler P04 | 25 min | 3 tasks | 8 files |
| Phase 02-social-media-scheduler P02 | 15 min | 2 tasks | 5 files |
| Phase 02-social-media-scheduler P06 | 10 min | 1 tasks | 3 files |
| Phase 02-social-media-scheduler P07 | 15 min | 2 tasks | 0 files |
| Phase 02-social-media-scheduler P07 | 15 min | 2 tasks | 0 files |
| Phase 02-social-media-scheduler P08 | 3 min | 2 tasks | 2 files |
| Phase 1.5-design-system-foundation P06 | 15 min | 2 tasks | 0 files |
| 03-epk-press-kit-builder | 01 | 4 min | 2 | 4 |
| 03-epk-press-kit-builder | 02 | 9 min | 2 | 8 |
| Phase 03-epk-press-kit-builder P03 | 15 | 2 tasks | 4 files |
| Phase 03-epk-press-kit-builder P04 | 14 min | 2 tasks | 8 files |
| Phase 03-epk-press-kit-builder P05 | 8 min | 2 tasks | 7 files |
| Phase 03-epk-press-kit-builder P06 | 6 min | 2 tasks | 4 files |

## Session Log

- 2026-03-14: STATE.md regenerated by /gsd:health --repair
- 2026-03-14: Completed 00-02 (Docker Compose stack + Nuxt proxy config)
- 2026-03-14: Completed 00-03 (HTTP layer: chi router, health endpoint, user_settings CRUD)
- 2026-03-14: Completed 00-04 (backup.sh + restore.sh; human checkpoint approved)
- 2026-03-14: Phase 00-infrastructure COMPLETE — verified 10/11 requirements
- 2026-03-16: Completed 01-03 (artwork fetch pipeline)
- 2026-03-16: Completed 01-04 (Playwright screenshot pipeline)
- 2026-03-16: Completed 01-05 (E2E smoke test + human verification - all 28 TRKL requirements verified)
- 2026-03-20: Inserted Phase 1.5 (Design System Foundation) — Tailwind CSS + shadcn-vue + Cyberpunk HUD design language, Pinia state management, component decomposition
- 2026-03-20: Completed 1.5-01 (Tailwind v4 + Cyberpunk HUD design tokens + variable fonts + all mockup-derived utilities)
- 2026-03-20: Completed 1.5-03 (Pinia stores: settings, tracklist, ui + design-tokens.ts with 5 presets — 30 tests GREEN)
- 2026-03-20: Completed 1.5-04 (TrackcardPreview.vue decomposed into 6 sub-components + TrackcardPreviewPanel.vue — 464→60 lines)
- 2026-03-22: Completed 02-07 (Human verification checkpoint — backend 21s green, frontend 109 tests green, all 17 SOCL requirements verified)
- 2026-03-22: Phase 02-social-media-scheduler COMPLETE — all plans 01-07 executed and human-verified
- 2026-03-22: Phase 1.5-design-system-foundation COMPLETE — design system, mockup patterns, Pinia stores, component decomposition, singleton components, responsive mobile shell, light/dark theme
- 2026-03-22: Completed 03-01 (EPK data layer — migration 004, EPKContent/EPKExport models, repository with 5 methods, 10 unit tests GREEN)
- 2026-03-22: Completed 03-02 (EPK service layer + PDF generator + HTTP handler — 8 methods, renderEPKPDF+renderMarkdown, 8 routes at /api/v1/epk, 29 tests GREEN)

---

_Phase: 03-epk-press-kit-builder_
_Status: IN PROGRESS — plan 03-02 complete_
