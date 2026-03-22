# Phase 3: EPK / Press Kit Builder — Research

**Researched:** 2026-03-22
**Domain:** PDF generation (gofpdf/fpdf), file upload (MinIO), Nuxt 4 / Vue 3 / Pinia, PostgreSQL JSONB
**Confidence:** HIGH (all findings verified against codebase or official docs)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**EPK Page Structure**
- Layout: Single scrollable page — all sections stacked vertically in order. No tabs, no sidebar navigation.
- Sections in order (top to bottom): ARTIST BIO → PRESS PHOTOS → TECH RIDER → STAGE PLOT → GIG HIGHLIGHTS → PRESS QUOTES → SOCIAL LINKS → CONTACT INFO → EXPORT PDF (with export history below it)
- Saving: Auto-save on change — 1-2 second debounce after user stops typing/interacting. Subtle `SAVED` indicator. No explicit Save button.
- Export panel: Fixed section at the bottom of the page containing: section toggle switches, EXPORT PDF button, and the export history list (previous PDFs from MinIO with download/delete).

**Bio Editor**
- Short bio: Plain text input with a character counter (recommended ~280 chars). No rich text.
- Long bio: Markdown textarea — user writes Markdown syntax (bold, italic, lists, links, H2/H3). No rendered preview. Markdown is rendered when generating the PDF.
- Critical: The PDF generation layer must parse Markdown into formatted fpdf output (bold: `**text**`, italic: `*text*`, H2/H3: `##`/`###`, unordered lists: `- item`, ordered lists: `1. item`, links: `[text](url)`).
- No Tiptap or other rich text library.

**Press Photo Management**
- Grid: 4-column thumbnail grid. Up to 20 photos (JPEG/PNG, max 10 MB each).
- Add photos: Drag-and-drop or click-to-upload drop zone.
- Remove photos: Each thumbnail has a delete button (x overlay on hover).
- No reordering, no captions, no primary photo concept.
- PDF includes all uploaded photos in upload order as a 3-column grid.
- At limit (20/20): Drop zone is hidden; label `20 / 20 PHOTOS — REMOVE ONE TO ADD MORE` replaces it.
- No lightbox — thumbnails are static. No click-to-zoom.

**PDF Output Design**
- Aesthetic: Professional white document. White background, near-black body text (#1A1A1A), DJ logo in header if uploaded, single accent colour from user settings (cyan token, accessible at 4.5:1 on white). Not dark/cyberpunk.
- Layout: Single-column, section-by-section. Each section: ALL_CAPS heading in accent colour, followed by content. Press photos as 3-column image grid.
- Default section order in PDF: 1) Bio (short then long), 2) Press Photos, 3) Gig Highlights, 4) Press Quotes, 5) Tech Rider, 6) Stage Plot (image), 7) Social Links, 8) Contact Info
- Section include/exclude: Toggle switches in export panel. Preferences persist to database.
- PDF library: fpdf (codeberg.org/go-pdf/fpdf) — the maintained successor to jung-kurt/gofpdf. No headless Chrome.

**EPK Data Storage**
- EPK reads from `user_settings` for `bio_short`, `bio_long`, `social_links`, `contact_info`.
- New `epk_content` table for EPK-only fields: tech rider text, stage plot image path, gig highlights (list), press quotes (list), section visibility preferences, photo paths (stored as array/JSONB referencing MinIO).
- When user edits bio in EPK page, it auto-saves to `user_settings` — single source of truth.

**EPK-10 (Import from Gigs)**
- Phase 4 (Gig Tracker) is not yet built. The "Import from Gigs" button is a stub — visible but disabled with `Coming Soon — available after Gig Tracker is set up` tooltip.

### Claude's Discretion
- Exact `epk_content` PostgreSQL schema (columns, JSONB vs separate tables for highlights/quotes)
- MinIO bucket and path conventions for press photos (`epk/photos/{uuid}.jpg`), stage plot (`epk/stage-plot/{uuid}.png`), and exported PDFs (`epk/exports/{uuid}.pdf`)
- Auto-save debounce implementation (composable or per-component)
- gofpdf Markdown parsing implementation (simple regex-based is fine for the supported subset)
- PDF header layout (logo position, DJ name typography)
- Empty section handling in PDF (skip empty sections entirely or show heading with "No content provided")
- Export history ordering (newest first)

### Deferred Ideas (OUT OF SCOPE)
- EPK-10 (Import from Gigs) — Stub in Phase 3; implement in Phase 4 after Gig Tracker ships.
- Hosted public EPK URL (`klubhub.dj/epk/name`) — SaaS PRO v2 M4. Explicitly out of OSS v1 scope.
- Multiple EPK versions — no versioning in v1; single EPK per user.
- PDF template themes (minimal, editorial, full-color) — v1 ships one template only.
- Video embed links (YouTube/SoundCloud mix links) — not in EPK-01 through EPK-10.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| EPK-01 | User can write artist bio (short + long versions) with rich text editor supporting bold, italic, lists, links, H2/H3 headings | Locked as Markdown textarea (no rich text lib). PDF layer parses Markdown with regex. Settings store already has bio_short/bio_long. |
| EPK-02 | User can upload and manage up to 20 press photos (JPEG/PNG, max 10 MB each) | MinIO PutObject pattern established. mimetype library already in go.mod for MIME validation. |
| EPK-03 | User can upload tech rider content and stage plot image | Tech rider = Textarea (text). Stage plot = single image upload via MinIO. Same patterns as tracklist artwork. |
| EPK-04 | User can enter social/streaming profile links | social_links already in user_settings JSONB. EPK reads from there; updates propagate to settings singleton. |
| EPK-05 | User can manually enter gig history highlights, press quotes, and contact information | JSONB arrays in epk_content table for highlights/quotes lists. contact_info from user_settings. |
| EPK-06 | User can generate a professionally formatted PDF press kit applying their logo and color settings | codeberg.org/go-pdf/fpdf v0.11.1 — confirmed active. Logo from user_settings.logo_path (MinIO presigned URL). Accent colour from default_colors JSONB. |
| EPK-07 | User can select which sections to include or exclude in the PDF export | Toggle switches persisted in epk_content.section_visibility (JSONB). Existing Switch UI component. |
| EPK-08 | Each PDF export is stored in MinIO and listed in export history view with timestamps; user can download or delete | New epk_exports table: id, minio_path, created_at. MinIO PutObject for store, presigned URL for download. |
| EPK-09 | All EPK data stored in PostgreSQL; photo and document assets stored in MinIO | epk_content singleton + epk_photo_paths JSONB array + epk_exports table in migration 004. |
| EPK-10 | Import from Gigs stub — visible but disabled | Stub button with tooltip. No backend wiring in Phase 3. |
</phase_requirements>

---

## Summary

Phase 3 builds the EPK module on top of the established project patterns. The key technical domains are: PDF generation with `codeberg.org/go-pdf/fpdf`, MinIO file uploads for press photos and stage plot, PostgreSQL with JSONB for the EPK content singleton, and a Vue 3 / Pinia frontend following the social module's component pattern.

The critical implementation decision already locked in CONTEXT.md is that the long bio is stored as Markdown and rendered at PDF generation time — not a rich text editor. This means the Go PDF service must include a lightweight Markdown-to-fpdf renderer supporting the six constructs: bold, italic, H2/H3, unordered lists, ordered lists, and links. A regex-based line-by-line parser is sufficient for this constrained subset.

The `jung-kurt/gofpdf` package referenced in PROJECT.md has two successors: `github.com/go-pdf/fpdf` (archived March 2025) and the actively maintained fork at `codeberg.org/go-pdf/fpdf` (v0.11.1, April 2025, last commit February 2026). The Codeberg version is the correct one to add to go.mod.

**Primary recommendation:** Wire the EPK module as `api/internal/epk/` with handler/service/repository/model files matching the social package structure. Add `codeberg.org/go-pdf/fpdf` to go.mod. Create migration 004 with `epk_content` singleton and `epk_exports` table. EPK frontend follows `pages/social.vue` + `components/epk/` pattern with a Pinia `stores/epk.ts` store.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| codeberg.org/go-pdf/fpdf | v0.11.1 | PDF generation on Go backend | Maintained fork of jung-kurt/gofpdf; no headless Chrome; no external dependencies; supports image embedding, custom fonts, RGB color |
| github.com/minio/minio-go/v7 | v7.0.99 | Press photo + stage plot + PDF export storage | Already in go.mod; established PutObject/GetObject/PresignedGetObject pattern in storage.Client |
| github.com/jackc/pgx/v5 | v5.8.0 | EPK content persistence (JSONB, arrays) | Already in go.mod; used across all existing packages |
| github.com/gabriel-vasile/mimetype | v1.4.12 | MIME validation for photo uploads | Already in go.mod; used in social service for image validation |
| pinia (defineStore) | existing | EPK Pinia store | Composition API style; established in settings, social, tracklist stores |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| lucide-vue-next | existing | FileText icon for EPK nav item | TheNav.vue and TheBottomNav.vue use this pattern |
| @vueuse/core (useDebounce / watchDebounced) | existing in pnpm workspace | Auto-save debounce | If not available, implement with setTimeout/clearTimeout in composable |
| github.com/google/uuid | v1.6.0 | UUID generation for photo paths, export records | Already in go.mod |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| codeberg.org/go-pdf/fpdf | unidoc/unioffice, wkhtmltopdf, headless Chrome | fpdf has no infrastructure dependency; others require Chrome binary or paid license |
| Markdown regex parser (custom) | goldmark, blackfriday | Full Markdown parsers are overkill for 6 constructs; regex parser is ~100 lines and zero dependencies |
| JSONB array for photo paths | Separate `epk_photos` join table | Join table adds migration complexity for a fixed-order, single-user list; JSONB array is simpler |

**Installation:**
```bash
# Go — add to api/go.mod
go get codeberg.org/go-pdf/fpdf@v0.11.1
```

---

## Architecture Patterns

### Recommended Project Structure

```
api/internal/epk/
├── handler.go          # HTTP routes and request parsing
├── handler_test.go     # Handler unit tests (mock service)
├── model.go            # EPKContent, EPKExport, request/response types
├── repository.go       # PostgreSQL CRUD for epk_content, epk_exports
├── repository_test.go  # Repository integration tests
├── service.go          # Business logic: photo validation, PDF generation, auto-save
├── service_test.go     # Service unit tests (mock repo + mock storage)
└── pdf.go              # PDF rendering logic (fpdf, Markdown parser)

api/internal/platform/migrations/
└── 004_epk.sql         # epk_content singleton + epk_exports table

apps/dj/app/
├── pages/epk.vue                         # Page orchestrator (follows social.vue pattern)
├── stores/epk.ts                         # Pinia store for EPK data + auto-save
├── composables/useEpkAutosave.ts         # Debounced auto-save composable
└── components/epk/
    ├── EpkPageHeader.vue                 # Page title + status indicator
    ├── EpkBioSection.vue                 # Short bio input + long bio Markdown textarea
    ├── EpkPhotosSection.vue              # 4-column photo grid + drop zone
    ├── EpkTechRiderSection.vue           # Tech rider textarea
    ├── EpkStagePlotSection.vue           # Stage plot image upload
    ├── EpkGigHighlightsSection.vue       # Ordered list of gig highlight entries
    ├── EpkPressQuotesSection.vue         # List of press quote entries
    ├── EpkSocialLinksSection.vue         # Social/streaming link inputs (reads from settings)
    ├── EpkContactInfoSection.vue         # Contact info (reads from settings)
    ├── EpkExportPanel.vue                # Section toggles + EXPORT PDF button + export history
    └── __tests__/
        ├── EpkBioSection.test.ts
        ├── EpkPhotosSection.test.ts
        └── EpkExportPanel.test.ts
```

### Pattern 1: Go Package Structure (handler/service/repository)

**What:** Three-layer architecture with interface contracts for testability.
**When to use:** All backend modules in this project follow this exact pattern.

```go
// Source: api/internal/social/handler.go (established pattern)
// serviceIface consumed by handler — enables mock testing without real service
type serviceIface interface {
    GetEPKContent(ctx context.Context) (*EPKContent, error)
    UpsertEPKContent(ctx context.Context, req UpsertEPKContentRequest) (*EPKContent, error)
    UploadPhoto(ctx context.Context, data []byte, contentType string) (string, error)
    DeletePhoto(ctx context.Context, minioPath string) error
    GeneratePDF(ctx context.Context) (string, error)  // returns MinIO path of exported PDF
    ListExports(ctx context.Context) ([]EPKExport, error)
    DeleteExport(ctx context.Context, id uuid.UUID) error
}
```

### Pattern 2: Router Wiring

**What:** New module handler registered in `NewRouter` and mounted in `main.go`.
**When to use:** Every new backend module follows this exact pattern.

```go
// Source: api/internal/platform/http/router.go (established pattern)
// NewRouter gains an epkHandler parameter
func NewRouter(..., epkHandler http.Handler) *chi.Mux {
    // ...
    r.Mount("/api/v1/epk", epkHandler)
    return r
}
```

### Pattern 3: Pinia EPK Store (composition API)

**What:** Composition-API Pinia store following settings.ts/social.ts patterns.
**When to use:** All Vue state management in this project uses this pattern.

```typescript
// Source: apps/dj/app/stores/settings.ts (established pattern)
export const useEpkStore = defineStore('epk', () => {
  // EPK content
  const bioShort = ref<string>('')
  const bioLong = ref<string>('')
  const techRider = ref<string>('')
  const stagePlotPath = ref<string | null>(null)
  const gigHighlights = ref<string[]>([])
  const pressQuotes = ref<{ text: string; source: string }[]>([])
  const photoPaths = ref<string[]>([])
  const sectionVisibility = ref<Record<string, boolean>>({})
  const exports = ref<EPKExport[]>([])

  // Auto-save state
  const saveStatus = ref<'idle' | 'saving' | 'saved' | 'error'>('idle')

  async function loadFromApi(): Promise<void> { ... }
  async function updateContent(patch: Partial<EPKContentRequest>): Promise<void> { ... }

  return { bioShort, bioLong, techRider, stagePlotPath, gigHighlights,
           pressQuotes, photoPaths, sectionVisibility, exports, saveStatus,
           loadFromApi, updateContent }
})
```

### Pattern 4: Auto-Save Composable (debounce)

**What:** A `useEpkAutosave` composable that watches a value ref and calls the store update after a debounce delay. Follows the inline track editing pattern in TracklistEditor.vue.
**When to use:** Any field in the EPK form that should auto-save.

```typescript
// Recommended implementation pattern (composable per CONTEXT.md discretion)
export function useEpkAutosave(
  store: ReturnType<typeof useEpkStore>,
  delayMs = 1500
) {
  let timer: ReturnType<typeof setTimeout> | null = null

  function scheduleSave(patch: Partial<EPKContentRequest>): void {
    store.saveStatus.value = 'saving'
    if (timer) clearTimeout(timer)
    timer = setTimeout(async () => {
      await store.updateContent(patch)
    }, delayMs)
  }

  return { scheduleSave }
}
```

### Pattern 5: fpdf PDF Generation

**What:** Server-side PDF generation using `codeberg.org/go-pdf/fpdf`. White document, custom font, accent colour from settings, logo image, section-by-section layout.
**When to use:** EPK export endpoint in `pdf.go`.

```go
// Source: codeberg.org/go-pdf/fpdf v0.11.1 official docs
import "codeberg.org/go-pdf/fpdf"

func renderEPKPDF(content *EPKContent, settings *settings.UserSettings, buf *bytes.Buffer) error {
    f := fpdf.New("P", "mm", "A4", "")
    f.SetMargins(20, 20, 20)
    f.SetAutoPageBreak(true, 20)
    f.AddPage()

    // Parse accent color from settings.DefaultColors["accent"] hex string
    // e.g. "#96F8FF" -> r=150, g=248, b=255
    r, g, b := hexToRGB(accentHex)

    // Logo in header if available (loaded from MinIO bytes)
    if settings.LogoPath != "" {
        // Register image from bytes reader
        f.RegisterImageOptionsReader("logo", reader, fpdf.ImageOptions{ImageType: "PNG"})
        f.ImageOptions("logo", 20, 15, 30, 0, false, fpdf.ImageOptions{}, 0, "")
    }

    // Section heading
    f.SetFont("Helvetica", "B", 11)
    f.SetTextColor(r, g, b)
    f.Cell(0, 8, "BIO", "", 1, "L", false, 0, "")

    // Divider line in accent colour
    f.SetDrawColor(r, g, b)
    f.SetLineWidth(0.5)
    f.Line(20, f.GetY(), 190, f.GetY())
    f.Ln(4)

    // Body text
    f.SetFont("Helvetica", "", 10)
    f.SetTextColor(26, 26, 26) // #1A1A1A
    f.MultiCell(0, 5, content.BioShort, "", "L", false)

    // Long bio (Markdown parsed line-by-line)
    renderMarkdown(f, content.BioLong)

    // Write PDF to bytes.Buffer
    return f.Output(buf)
}
```

### Pattern 6: Markdown Regex Parser for fpdf

**What:** Line-by-line parser that translates the 6 supported Markdown constructs into fpdf font-style switches.
**When to use:** Rendering `bio_long` Markdown in `pdf.go`.

```go
// Handles: H2 (##), H3 (###), bold (**text**), italic (*text*),
//          unordered list (- item), ordered list (1. item), links ([text](url))
func renderMarkdown(f *fpdf.Fpdf, md string) {
    lines := strings.Split(md, "\n")
    for i, line := range lines {
        trimmed := strings.TrimSpace(line)
        switch {
        case strings.HasPrefix(trimmed, "### "):
            f.SetFont("Helvetica", "B", 10)
            f.MultiCell(0, 5, strings.TrimPrefix(trimmed, "### "), "", "L", false)
            f.SetFont("Helvetica", "", 10)
        case strings.HasPrefix(trimmed, "## "):
            f.SetFont("Helvetica", "B", 11)
            f.MultiCell(0, 6, strings.TrimPrefix(trimmed, "## "), "", "L", false)
            f.SetFont("Helvetica", "", 10)
        case strings.HasPrefix(trimmed, "- "):
            f.MultiCell(0, 5, "• "+renderInline(strings.TrimPrefix(trimmed, "- ")), "", "L", false)
        case listItemRe.MatchString(trimmed):
            // ordered list: "1. item"
            f.MultiCell(0, 5, renderInline(trimmed), "", "L", false)
        case trimmed == "":
            f.Ln(3)
        default:
            renderInlineLine(f, trimmed)
        }
        _ = i
    }
}
// Inline rendering switches font for **bold** and *italic* runs
// Links: [text](url) — rendered as underlined text (no live hyperlink in PDF)
```

### Pattern 7: Photo Upload to MinIO

**What:** Multipart form upload endpoint, MIME validation via mimetype library, PutObject to MinIO at `epk/photos/{uuid}.jpg`.
**When to use:** POST /api/v1/epk/photos endpoint.

```go
// Source: api/internal/platform/storage/storage.go + social service.go patterns
func (s *Service) UploadPhoto(ctx context.Context, data []byte, mimeType string) (string, error) {
    // Validate MIME
    if mimeType != "image/jpeg" && mimeType != "image/png" {
        return "", ErrInvalidMIME
    }
    if len(data) > 10*1024*1024 {
        return "", ErrFileTooLarge
    }
    // Generate path
    ext := ".jpg"
    if mimeType == "image/png" { ext = ".png" }
    objectName := "epk/photos/" + uuid.New().String() + ext

    // Upload to MinIO
    _, err := s.store.PutObject(ctx, s.bucket, objectName,
        bytes.NewReader(data), int64(len(data)),
        minio.PutObjectOptions{ContentType: mimeType})
    if err != nil {
        return "", fmt.Errorf("upload photo: %w", err)
    }
    return objectName, nil
}
```

### Pattern 8: Vue Photo Drop Zone

**What:** Reuse `ghost-border` CSS class for the drop zone. HTML5 drag-and-drop events (`dragover`, `drop`). `<input type="file" multiple accept="image/jpeg,image/png">` as fallback.
**When to use:** EpkPhotosSection.vue.

```vue
<!-- ghost-border class is the established drop zone pattern -->
<div
  v-if="photoCount < 20"
  class="ghost-border p-6 flex flex-col items-center gap-2 cursor-pointer"
  @dragover.prevent
  @drop.prevent="handleDrop"
  @click="fileInput?.click()"
>
  <span class="font-terminal tracking-terminal text-xs uppercase text-tertiary">
    DROP PHOTOS HERE OR CLICK TO UPLOAD
  </span>
</div>
<div v-else class="font-terminal tracking-terminal text-xs uppercase text-secondary py-4">
  20 / 20 PHOTOS — REMOVE ONE TO ADD MORE
</div>
<input ref="fileInput" type="file" multiple accept="image/jpeg,image/png" class="hidden" @change="handleFileInput" />
```

### Anti-Patterns to Avoid

- **Loading MinIO images via presigned URL per-render in PDF**: Pre-fetch image bytes in Go before calling fpdf; presigned URLs are for browser download, not for Go server-side image loading.
- **Rich text library for Markdown bio**: CONTEXT.md explicitly bans Tiptap/similar. Use plain `<Textarea>` with `font-mono text-xs`.
- **Separate endpoint per EPK field update**: Use a single `PUT /api/v1/epk/content` that accepts a partial update payload; debounce on frontend.
- **Storing full binary image in PostgreSQL**: All image data in MinIO; only MinIO object paths stored in JSONB.
- **Optimistic concurrency on epk_content**: The EPK content is a singleton that auto-saves. Conflict detection is less critical here than for the shared settings endpoint. Skip OCC on EPK content unless needed.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PDF layout engine | Custom PDF binary writer | codeberg.org/go-pdf/fpdf | PDF spec is 500+ pages; fpdf handles fonts, image DPI, page breaks, coordinate math |
| MIME type detection | Checking file extension | github.com/gabriel-vasile/mimetype | Already in go.mod; magic byte detection is reliable; extension is spoofable |
| File size limit enforcement | Client-side JS only | Server-side check in service layer | Client can bypass JS; server must validate |
| UUID generation for MinIO paths | rand.Intn() | github.com/google/uuid | Already in go.mod; collision-proof |
| Debounce | Custom `time.Sleep` goroutine | `setTimeout`/`clearTimeout` pattern in composable | Standard JS pattern; goroutine approach is unnecessary for frontend debounce |

**Key insight:** PDF generation has subtle correctness requirements (font encoding, image DPI, coordinate overflow across pages). fpdf handles all of this; custom solutions would miss edge cases.

---

## Common Pitfalls

### Pitfall 1: fpdf Image Requires Bytes, Not URL

**What goes wrong:** Calling `f.Image("https://minio.../epk/photos/uuid.jpg", ...)` — fpdf cannot fetch HTTP URLs server-side.
**Why it happens:** fpdf's `Image()` call takes a file path or requires a pre-registered reader. MinIO returns presigned HTTP URLs, not file paths.
**How to avoid:** Use `f.RegisterImageOptionsReader(name, ioReader, opts)` — fetch image bytes from MinIO with `store.GetObject()` first, then pass the bytes as an `io.Reader`.
**Warning signs:** `runtime error: invalid memory address` or empty image regions in the output PDF.

### Pitfall 2: fpdf Uses mm Units by Default (not px)

**What goes wrong:** Specifying image dimensions in pixels (e.g., `f.Image(..., 0, 0, 1080, 1080, ...)`) produces a huge image that overflows the page.
**Why it happens:** `fpdf.New("P", "mm", "A4", "")` uses millimeters. A4 page is 210×297mm.
**How to avoid:** Use mm throughout. Photos in 3-column grid: column width ~56mm ((210 - 20 - 20 - 8) / 3). Set height to 0 to let fpdf auto-scale aspect ratio.
**Warning signs:** Images are cut off or overlap section boundaries.

### Pitfall 3: Markdown Parser State Leaks Between Sections

**What goes wrong:** Font is left in Bold/Italic state after a heading line. Subsequent body text renders bold throughout rest of section.
**Why it happens:** fpdf uses mutable font state. If a bold heading's font reset is missed, all subsequent calls inherit bold.
**How to avoid:** Always reset to `f.SetFont("Helvetica", "", 10)` after rendering headings, bold runs, or italics in `renderMarkdown`.

### Pitfall 4: JSONB Array Ordering in PostgreSQL

**What goes wrong:** Photos appear in different order in the PDF than shown in the UI.
**Why it happens:** PostgreSQL JSONB does not guarantee array element order unless stored as a JSON array (not object). `jsonb_array_elements` preserves order, but incorrect column type can lose it.
**How to avoid:** Store photo paths as `JSONB NOT NULL DEFAULT '[]'` — JSON array, not object. Use `json_array_elements_text()` or scan directly into `[]string` via pgx.

### Pitfall 5: TheBottomNav Hick's Law Constraint

**What goes wrong:** Adding EPK as a 6th item to TheBottomNav breaks the 5-item Miller/Hick's Law constraint (documented in TheBottomNav.vue comments).
**Why it happens:** TheBottomNav explicitly has 5 items. EPK is a new nav item.
**How to avoid:** Per CONTEXT.md discretion — recommend replacing FINANCE in TheBottomNav with EPK (finance accessible via sidebar). TheNav (sidebar, desktop) is unconstrained — add EPK between SOCIAL and GIGS. The planner must choose one option.

### Pitfall 6: Auto-Save to Two Tables (epk_content + user_settings)

**What goes wrong:** Bio changes in EPK page only save to `epk_content`, not to `user_settings`. Settings page then shows stale bio.
**Why it happens:** The EPK module has its own table, but bio/social_links/contact_info are shared with the settings singleton.
**How to avoid:** The EPK service's `UpsertEPKContent` method must call the settings service (or directly update `user_settings`) when bio_short, bio_long, social_links, or contact_info change. Two writes in a single transaction.

---

## Code Examples

Verified patterns from codebase and official fpdf docs:

### Migration 004: epk_content + epk_exports

```sql
-- +goose Up
CREATE TABLE epk_content (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tech_rider          TEXT NOT NULL DEFAULT '',
    stage_plot_path     TEXT NOT NULL DEFAULT '',
    gig_highlights      JSONB NOT NULL DEFAULT '[]',
    press_quotes        JSONB NOT NULL DEFAULT '[]',
    photo_paths         JSONB NOT NULL DEFAULT '[]',
    section_visibility  JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);
CREATE INDEX ON epk_content(deleted_at) WHERE deleted_at IS NULL;

CREATE TABLE epk_exports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    minio_path  TEXT NOT NULL,
    file_size   BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX ON epk_exports(created_at DESC) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS epk_exports;
DROP TABLE IF EXISTS epk_content;
```

### MinIO Object Path Conventions (Claude's Discretion — Recommendation)

```
epk/photos/{uuid}.jpg        # press photos (JPEG or PNG, stored with .jpg ext unless PNG)
epk/photos/{uuid}.png        # PNG press photos
epk/stage-plot/{uuid}.png    # stage plot image
epk/exports/{uuid}.pdf       # exported PDFs
```

### EPK Handler Routes

```go
// Source: api/internal/social/handler.go pattern
func (h *Handler) Routes() http.Handler {
    r := chi.NewRouter()
    r.Get("/content",               h.handleGetContent)        // GET full EPK content
    r.Put("/content",               h.handleUpsertContent)     // PUT partial update (auto-save)
    r.Post("/photos",               h.handleUploadPhoto)       // POST multipart upload
    r.Delete("/photos",             h.handleDeletePhoto)       // DELETE ?path=epk/photos/...
    r.Post("/stage-plot",           h.handleUploadStagePlot)   // POST multipart upload
    r.Post("/exports",              h.handleGeneratePDF)       // POST trigger PDF generation
    r.Get("/exports",               h.handleListExports)       // GET export history
    r.Get("/exports/{id}/download", h.handleDownloadExport)    // GET presigned URL redirect
    r.Delete("/exports/{id}",       h.handleDeleteExport)      // DELETE export record + MinIO
    return r
}
```

### Presigned URL for Photo Download (browser-side rendering)

```go
// Source: api/internal/platform/storage/storage.go
// Press photos need presigned URLs for <img> rendering in the browser
url, err := store.PresignedGetObject(ctx, bucket, objectName, 1*time.Hour, nil)
```

### fpdf 3-Column Photo Grid

```go
// Source: codeberg.org/go-pdf/fpdf API
const (
    pageW      = 210.0  // A4 width mm
    marginL    = 20.0
    marginR    = 20.0
    gapX       = 4.0    // gap between columns
    cols       = 3
    colW       = (pageW - marginL - marginR - (cols-1)*gapX) / cols  // ~56.67mm
)

x := marginL
for i, photoPath := range photoPaths {
    imgBytes := fetchFromMinIO(photoPath)
    reader := bytes.NewReader(imgBytes)
    name := fmt.Sprintf("photo-%d", i)
    f.RegisterImageOptionsReader(name, reader, fpdf.ImageOptions{ImageType: detectType(photoPath)})
    f.ImageOptions(name, x, f.GetY(), colW, 0, false, fpdf.ImageOptions{}, 0, "")

    x += colW + gapX
    if (i+1)%cols == 0 {
        x = marginL
        f.Ln(colW + gapX)  // approximate: advance by column height
    }
}
```

### Vue Section Component Pattern (glass-panel)

```vue
<!-- Established pattern from Phase 1.5 design system -->
<template>
  <section class="glass-panel p-6 space-y-4">
    <h2 class="font-terminal tracking-terminal text-xs uppercase text-on-surface-variant">
      ARTIST BIO
    </h2>
    <!-- content -->
  </section>
</template>
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| jung-kurt/gofpdf | codeberg.org/go-pdf/fpdf (active fork) | go-pdf/fpdf archived March 2025 | Must use codeberg module path; not github.com/jung-kurt or github.com/go-pdf |
| github.com/go-pdf/fpdf | codeberg.org/go-pdf/fpdf | March 2025 | github.com/go-pdf/fpdf is archived and points to Codeberg |

**Deprecated/outdated:**
- `github.com/jung-kurt/gofpdf`: Archived; last maintainer stepped away. Do not use.
- `github.com/go-pdf/fpdf`: Also archived as of March 2025, redirects to Codeberg. Do not use.
- Headless Chrome PDF: Requires Chromium binary in Docker, adds 200MB+. Out of scope per CONTEXT.md.

---

## Open Questions

1. **Bottom nav: EPK vs FINANCE slot**
   - What we know: TheBottomNav has exactly 5 items (DASH, TRACKS, SOCIAL, GIGS, FINANCE). EPK would be item 6, violating the documented Hick's Law constraint.
   - What's unclear: Whether to replace FINANCE with EPK or keep EPK sidebar-only on mobile.
   - Recommendation: Planner should decide. CONTEXT.md leaves this as Claude's discretion. Suggest: Replace FINANCE in bottom nav with EPK (EPK is more likely to be used on mobile for quick reference). FINANCE accessible via sidebar.

2. **Photo presigned URL expiry**
   - What we know: `PresignedGetObject` requires an expiry duration. The social module does not use presigned URLs for image display (it uses direct MinIO paths in the app layer).
   - What's unclear: Whether photo thumbnails in the UI should use short-lived presigned URLs (1h) or a proxy endpoint.
   - Recommendation: Use 1-hour presigned URLs returned alongside the EPK content from the API. The store can refresh on page load.

3. **fpdf RegisterImageOptionsReader name uniqueness**
   - What we know: fpdf caches registered images by name string. Using the same name twice may return a cached version.
   - What's unclear: Whether MinIO object paths are unique enough as image names.
   - Recommendation: Use the MinIO object path as the image name (e.g., `"epk/photos/uuid.jpg"`). These are UUIDs and guaranteed unique per upload.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework (Go) | go test (built-in) + testify v1.11.1 |
| Framework (Vue) | Vitest (jsdom) — vitest.config.ts at apps/dj/vitest.config.ts |
| Config file | apps/dj/vitest.config.ts — no changes needed |
| Quick run (Go) | `cd api && go test ./internal/epk/... -count=1` |
| Quick run (Vue) | `pnpm nx test dj --testPathPattern=epk` |
| Full suite (Go) | `cd api && go test ./... -count=1` |
| Full suite (Vue) | `pnpm nx test dj` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| EPK-01 | Bio short char counter, long bio markdown textarea saves correctly | unit (Vue) | `pnpm nx test dj --testPathPattern=EpkBioSection` | Wave 0 |
| EPK-02 | Photo upload validates MIME + size; rejects >20 photos | unit (Go service) | `cd api && go test ./internal/epk/... -run TestUploadPhoto -count=1` | Wave 0 |
| EPK-03 | Tech rider text saves; stage plot image upload stores path | unit (Go service) | `cd api && go test ./internal/epk/... -run TestUpsertContent -count=1` | Wave 0 |
| EPK-04 | Social links read from user_settings, saved back | unit (Go service) | `cd api && go test ./internal/epk/... -run TestSocialLinksSync -count=1` | Wave 0 |
| EPK-05 | Gig highlights + press quotes JSONB round-trip | unit (Go repo) | `cd api && go test ./internal/epk/... -run TestRepository -count=1` | Wave 0 |
| EPK-06 | PDF generated with sections; non-empty bytes returned | unit (Go service) | `cd api && go test ./internal/epk/... -run TestGeneratePDF -count=1` | Wave 0 |
| EPK-07 | Section visibility toggles persist; excluded sections absent from PDF | unit (Go service) | `cd api && go test ./internal/epk/... -run TestSectionVisibility -count=1` | Wave 0 |
| EPK-08 | Export stored in epk_exports; download returns presigned URL; delete soft-deletes | unit (Go service) | `cd api && go test ./internal/epk/... -run TestExportHistory -count=1` | Wave 0 |
| EPK-09 | epk_content row persisted to PostgreSQL; photos in JSONB array | unit (Go repo) | `cd api && go test ./internal/epk/... -run TestRepository -count=1` | Wave 0 |
| EPK-10 | Import from Gigs button exists in DOM, is disabled, shows tooltip | unit (Vue) | `pnpm nx test dj --testPathPattern=EpkGigHighlightsSection` | Wave 0 |

### Sampling Rate
- **Per task commit:** `cd api && go test ./internal/epk/... -count=1` and `pnpm nx test dj --testPathPattern=epk`
- **Per wave merge:** `cd api && go test ./... -count=1` and `pnpm nx test dj`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `api/internal/epk/service_test.go` — mockRepo + mockStorage stubs; covers EPK-02 through EPK-09
- [ ] `api/internal/epk/repository_test.go` — covers EPK-05, EPK-08, EPK-09 (may use testcontainers like social repo tests)
- [ ] `api/internal/epk/handler_test.go` — HTTP handler routing tests
- [ ] `apps/dj/app/components/epk/__tests__/EpkBioSection.test.ts` — covers EPK-01
- [ ] `apps/dj/app/components/epk/__tests__/EpkPhotosSection.test.ts` — covers EPK-02
- [ ] `apps/dj/app/components/epk/__tests__/EpkExportPanel.test.ts` — covers EPK-06, EPK-07, EPK-08
- [ ] `apps/dj/app/components/epk/__tests__/EpkGigHighlightsSection.test.ts` — covers EPK-10 stub

---

## Sources

### Primary (HIGH confidence)
- Codebase: `api/internal/social/` — handler/service/repository/model pattern (verified by reading files)
- Codebase: `api/internal/platform/storage/storage.go` — PutObject, GetObject, PresignedGetObject API
- Codebase: `api/internal/platform/migrations/` — goose Up/Down format with single-file convention
- Codebase: `api/cmd/api/main.go` — module wiring pattern (NewRouter, DI chain)
- Codebase: `apps/dj/app/stores/settings.ts` — Pinia composition API store pattern
- Codebase: `apps/dj/app/components/TheNav.vue`, `TheBottomNav.vue` — nav item patterns
- Codebase: `apps/dj/app/pages/social.vue` — page orchestrator pattern
- codeberg.org/go-pdf/fpdf (WebFetch verified): v0.11.1, April 2025, active maintenance as of Feb 2026

### Secondary (MEDIUM confidence)
- WebFetch github.com/go-pdf/fpdf: Confirmed archived March 2025, redirects to Codeberg
- pkg.go.dev/github.com/go-pdf/fpdf: API surface verified (same API as Codeberg fork)
- WebSearch "gofpdf golang 2025": Confirms jung-kurt/gofpdf is unmaintained

### Tertiary (LOW confidence)
- None — all critical claims verified against codebase or official sources

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — codeberg.org/go-pdf/fpdf confirmed active; all other libraries already in go.mod
- Architecture: HIGH — patterns verified by reading existing packages (social, tracklist, settings)
- Pitfalls: HIGH — fpdf image/mm pitfalls documented in official API; others derived from codebase patterns
- Database schema: MEDIUM — Claude's discretion per CONTEXT.md; recommendations are consistent with existing migration patterns

**Research date:** 2026-03-22
**Valid until:** 2026-06-22 (fpdf is stable; other dependencies are pinned in go.mod)
