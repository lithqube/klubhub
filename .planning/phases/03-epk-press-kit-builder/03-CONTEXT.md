# Phase 3: EPK / Press Kit Builder — Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Users fill in their press kit data (bio, photos, tech rider, stage plot, gig highlights, press quotes, social links, contact info) and export a professionally formatted PDF. PDF-only — no hosted public URL (that's SaaS v2 M4). Export history is tracked; previous exports are downloadable. EPK-10 (Import from Gigs) is a stub — Phase 4 not yet built.

</domain>

<decisions>
## Implementation Decisions

### EPK Page Structure
- **Layout:** Single scrollable page — all sections stacked vertically in order. No tabs, no sidebar navigation.
- **Sections in order (top to bottom):** ARTIST BIO → PRESS PHOTOS → TECH RIDER → STAGE PLOT → GIG HIGHLIGHTS → PRESS QUOTES → SOCIAL LINKS → CONTACT INFO → EXPORT PDF (with export history below it)
- **Saving:** Auto-save on change — 1-2 second debounce after user stops typing/interacting. Subtle `SAVED` indicator. No explicit Save button.
- **Export panel:** Fixed section at the bottom of the page containing: section toggle switches, EXPORT PDF button, and the export history list (previous PDFs from MinIO with download/delete).

### Bio Editor
- **Short bio:** Plain text input with a character counter (recommended ~280 chars). No rich text.
- **Long bio:** Markdown textarea — user writes Markdown syntax (bold, italic, lists, links, H2/H3). Rendered preview not required on screen (the Markdown is rendered when generating the PDF).
  - **Critical:** The PDF generation layer must parse Markdown into formatted gofpdf output (bold: `**text**`, italic: `*text*`, H2/H3: `##`/`###`, unordered lists: `- item`, ordered lists: `1. item`, links: `[text](url)`).
  - No Tiptap or other rich text library — simpler, no extra dependencies.

### Press Photo Management
- **Grid:** 4-column thumbnail grid. Up to 20 photos (JPEG/PNG, max 10 MB each).
- **Add photos:** Drag-and-drop or click-to-upload drop zone above or alongside the grid.
- **Remove photos:** Each thumbnail has a delete button (× overlay on hover).
- **No reordering, no captions, no primary photo concept** — PDF includes all uploaded photos in upload order as a 3-column grid.
- **At limit (20/20):** Drop zone is hidden entirely; a label `20 / 20 PHOTOS — REMOVE ONE TO ADD MORE` replaces it.
- **No lightbox** — thumbnails are static. No click-to-zoom.

### PDF Output Design
- **Aesthetic:** Professional white document. White background, near-black body text (`#1A1A1A`), DJ logo in the header if uploaded, single accent colour from user's settings (the cyan token used for headings and dividers — accessible at 4.5:1 on white). Not dark/cyberpunk.
- **Layout:** Single-column, section-by-section. Each section starts with an ALL_CAPS heading in the accent colour, followed by its content. Press photos appear as a 3-column image grid.
- **Default section order in PDF:**
  1. Bio (short then long)
  2. Press Photos
  3. Gig Highlights
  4. Press Quotes
  5. Tech Rider
  6. Stage Plot (image)
  7. Social Links
  8. Contact Info
- **Section include/exclude:** Toggle switches in the export panel (using existing `Switch` component). Preferences persist to the database — next export uses the same toggles.
- **PDF library:** `gofpdf` — already decided in PROJECT.md. No headless Chrome.

### EPK Data Storage
- **Note from codebase:** `user_settings` table already stores `bio_short`, `bio_long`, `social_links`, and `contact_info`. The EPK module must decide whether to read/write these shared fields or maintain its own copies.
  - **Decision:** EPK reads from `user_settings` for `bio_short`, `bio_long`, `social_links`, `contact_info` but also has its own `epk_content` table for EPK-only fields: tech rider text, stage plot image path, gig highlights (list), press quotes (list), section visibility preferences, photo paths (stored as array/JSONB referencing MinIO).
  - When the user edits bio in the EPK page, it auto-saves to `user_settings` (same record as the settings page) — single source of truth.

### EPK-10 (Import from Gigs)
- Phase 4 (Gig Tracker) is not yet built. The "Import from Gigs" button is a **stub** — visible but disabled with a `Coming Soon — available after Gig Tracker is set up` tooltip.

### Claude's Discretion
- Exact `epk_content` PostgreSQL schema (columns, JSONB vs separate tables for highlights/quotes)
- MinIO bucket and path conventions for press photos (`epk/photos/{uuid}.jpg`) and stage plot (`epk/stage-plot/{uuid}.png`) and exported PDFs (`epk/exports/{uuid}.pdf`)
- Auto-save debounce implementation (composable or per-component)
- gofpdf Markdown parsing implementation (simple regex-based is fine for the supported subset)
- PDF header layout (logo position, DJ name typography)
- Empty section handling in PDF (skip empty sections entirely or show heading with "No content provided")
- Export history ordering (newest first)

</decisions>

<specifics>
## Specific Ideas

- Bio sections auto-populate from existing `user_settings.bio_short` / `user_settings.bio_long` if already filled in from the tracklist settings flow — no re-entry required.
- The EXPORT PDF button in the export panel should use the `gradient-cta` class (cyan-to-turquoise) consistent with the app's primary CTA pattern.
- Section toggle switches use the existing `Switch` component (sharp/square, cyberpunk styled) — consistent with the design system.
- Markdown textarea for long bio should use the existing `Textarea` or `Input` component with a `font-mono text-xs` style to make the Markdown syntax legible as code.
- Press photo thumbnails: square aspect ratio (1:1), object-fit cover, matching the 0px radius constraint.

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `app/components/ui/switch/Switch.vue` — Sharp toggle for section include/exclude in export panel.
- `app/components/ui/badge/Badge.vue` — Status badges for export history (e.g. `ready` variant for completed exports).
- `app/components/ui/progress/Progress.vue` — 4px progress bar for PDF generation progress.
- `app/components/ui/input/Input.vue` / `Textarea` — For all text fields including the Markdown bio textarea.
- `app/stores/settings.ts` — `djName`, `logoPath` from settings store. EPK reads these for PDF header generation.
- `ghost-border` CSS class — Dashed border drop zone. Reuse for photo upload drop zone and stage plot upload zone.
- `glass-panel` CSS class — Section container styling consistent with the rest of the app.
- `gradient-cta` CSS class — Primary CTA button styling for the EXPORT PDF button.

### Established Patterns
- Go module structure: new `api/internal/epk/` package with handler, service, repository, model files — matches tracklist and social patterns.
- MinIO upload: `storage.Client.PutObject(ctx, bucket, objectName, reader, size, opts)` — same pattern used in artwork cache and social image uploads.
- `$fetch('/api/v1/epk/...')` — no Axios, direct Nuxt $fetch. All EPK API calls follow this.
- Pinia store: composition API style `defineStore('epk', () => { ... })` — consistent with settings, tracklist, social stores.
- Auto-save: same debounce pattern used for inline track editing in `TracklistEditor.vue`.

### Integration Points
- `api/internal/settings/` — EPK bio and contact fields write to the same `user_settings` singleton. EPK handler may need to call the settings service for bio updates, or the EPK service manages them directly.
- `TheNav.vue` — EPK nav item needs to be added (icon: FileText or similar from lucide-vue-next). Currently has: Tracklist, Social. EPK goes between Social and Gigs.
- `TheBottomNav.vue` — Mobile bottom nav has 5 items: DASH, TRACKS, SOCIAL, GIGS, FINANCE. EPK would be a 6th, exceeding Hick's Law limit of 5. Claude's discretion: drop FINANCE from bottom nav (accessible via sidebar), replace with EPK, or keep bottom nav at 5 and put EPK in the sidebar only.
- `pages/epk.vue` — New page, follows `pages/social.vue` pattern (page as layout, feature components in `components/epk/`).

</code_context>

<deferred>
## Deferred Ideas

- **EPK-10 (Import from Gigs)** — Stub in Phase 3; implement in Phase 4 after Gig Tracker ships.
- **Hosted public EPK URL** (`klubhub.dj/epk/name`) — SaaS PRO v2 M4. Explicitly out of OSS v1 scope.
- **Multiple EPK versions** ("Techno set version" vs "House set version") — no versioning in v1; single EPK per user.
- **PDF template themes** (minimal, editorial, full-color) — v1 ships one template only.
- **Video embed links** (YouTube/SoundCloud mix links) in EPK — not in EPK-01 through EPK-10; potential v1.x addition.

</deferred>

---

*Phase: 03-epk-press-kit-builder*
*Context gathered: 2026-03-22*
