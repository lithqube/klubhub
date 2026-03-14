# Phase 1: Tracklist Image Generator — Research

**Researched:** 2026-03-14
**Domain:** Go file parsing + PostgreSQL + MinIO + Nuxt 4 Nitro Playwright screenshot pipeline
**Confidence:** HIGH (core stack verified via official docs); MEDIUM (cover-art API integration patterns)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **Image rendering:** Nuxt SSR screenshot via Playwright inside Nitro
  - Nuxt exposes `/api/render/tracklist/[id]` returning HTML
  - Nuxt Nitro handler `/api/screenshot/tracklist/[id]` runs Playwright, returns PNG binary
  - Go calls the Nuxt screenshot endpoint, stores PNG in MinIO, returns object URL to client
  - Playwright runs as a Nitro plugin — no separate service
- **Upload flow:** Two-step — upload file → inline-edit review table → confirm → save
- **Customization:** Global settings only (`dj_name`, `logo_path`, `default_colors` from `user_settings`)
- **Page:** Single `pages/tracklist.vue` with upload + preview side-by-side
- **No public share URLs in Phase 1**
- **Inline editing allowed in review step**
- **File format:** Rekordbox UTF-16 LE TSV (columns: #, Artwork, Track Title, Artist, Album, Genre, BPM, Rating, Time, Key, Date Added)
- **Backend:** Go (not Python, not Node)
- **Frontend:** Nuxt 4 / Vue 3 / TypeScript
- **DB:** PostgreSQL with pgx/v5
- **Storage:** MinIO for PNG objects
- **Existing Phase 0 infra:** Go chi router at `/api/v1/`, `user_settings` table, pgxpool, MinIO client, goose migrations

### Claude's Discretion

- Database schema for tracklists and tracks tables (UUID PK, soft-delete pattern from Phase 0)
- Rekordbox TSV parser implementation details (encoding detection, error handling)
- Playwright configuration for screenshot sizing and quality
- Pagination approach for tracklist list view
- Error states for failed parses (malformed file, wrong encoding)
- MinIO path convention for stored PNGs

### Deferred Ideas (OUT OF SCOPE)

- Public shareable URL (`/share/[id]`)
- Per-set template overrides
- Social media sharing integrations
- Key normalization / display toggle
- Advanced layout algorithm for large tracklists (50+ tracks)
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| TRKL-01 | Drag-and-drop or file picker upload | Vue composable `useDropZone` (VueUse) or native HTML5 drag-and-drop input |
| TRKL-02 | Auto-detect Rekordbox, Serato, Traktor | BOM + column-header heuristics in Go parser |
| TRKL-03 | Extract artist, title, BPM, key, label, duration, time played | Rekordbox TSV column mapping documented in CONTEXT.md |
| TRKL-04 | Validate files, return clear error messages | `http.MaxBytesReader` + format detection in Go handler |
| TRKL-05 | Reject files > 50 MB | `http.MaxBytesReader(w, r.Body, 50<<20)` in Go |
| TRKL-06 | Partial parse results with per-track warnings | `ParseResult{Tracks []Track, Warnings []ParseWarning}` return type |
| TRKL-07 | Zero-parseable-tracks specific error | Explicit check after parse loop |
| TRKL-08 | Inline-edit any track metadata field | Vue reactive table with per-row edit mode; PUT `/api/v1/tracklists/{id}` |
| TRKL-09 | Store tracklists in PostgreSQL, raw files in MinIO | `tracklists` + `tracks` tables; MinIO `raw-uploads/` bucket path |
| TRKL-10 | Cover art fetch: Spotify → Discogs → MusicBrainz fallback | Go async worker per track; status column on tracks table |
| TRKL-11 | Cache fetched cover art in MinIO | MinIO path `cover-art/{hash}.jpg`; skip fetch if object exists |
| TRKL-12 | Cover art fetching asynchronous with progressive UI updates | Go goroutine pool; frontend polls GET `/api/v1/tracklists/{id}` for status |
| TRKL-13 | Placeholder cover art for unmatched tracks | Pre-generated 500×500 PNG stored in MinIO; user can upload replacement in settings |
| TRKL-14 | Per-track manual cover art override | PUT `/api/v1/tracklists/{id}/tracks/{track_id}/artwork` accepts multipart |
| TRKL-15 | Tracklist config UI shows all cover art thumbnails | `<img>` with presigned MinIO URLs; accept/reject/replace UI per track |
| TRKL-16 | All cover art APIs unreachable warning + retry button | Frontend detects all tracks in `artwork_pending` state after timeout |
| TRKL-17 | Generate 1080×1920 (Story) and 1080×1080 (Square) | Playwright `browser.newContext({ viewport: { width, height } })` |
| TRKL-18 | Background mode: custom upload, solid color, cover art mosaic | CSS background on render route; data passed as query params or DB state |
| TRKL-19 | 3–5 design presets with color/font overrides | CSS classes on render route; preset name stored in tracklist record |
| TRKL-20 | Toggle display of each metadata field | `visible_fields` JSONB on tracklist record; CSS show/hide on render route |
| TRKL-21 | Logo/watermark with corner position | `logo_path` from `user_settings`; CSS `position: absolute` on render route |
| TRKL-22 | Live preview at 50% resolution; exported at full resolution | Preview: CSS `transform: scale(0.5)`; export: full viewport screenshot |
| TRKL-23 | PNG or JPEG export | Playwright `page.screenshot({ type: 'png' | 'jpeg', quality })` |
| TRKL-24 | "Export Both" generates Story + Square | Two sequential Playwright calls in one Nitro handler |
| TRKL-25 | `max_tracks_displayed` limit with range selector | `max_tracks` column on tracklist; frontend range input |
| TRKL-26 | Unicode rendering incl. CJK, Cyrillic, Arabic via Noto Sans | Dockerfile installs `fonts-noto`, `fonts-noto-cjk`; CSS `font-family: Noto Sans` |
| TRKL-27 | Delete tracklist: cascade soft-delete, gig refs NULL, confirmation dialog | SQL: UPDATE tracks SET deleted_at + UPDATE gig references; Vue confirm dialog |
| TRKL-28 | Persist last-used template, background mode, field visibility | `tracklist_preferences` JSONB on `user_settings` or separate `user_preferences` column |
</phase_requirements>

---

## Summary

Phase 1 requires four tightly coupled subsystems: (1) a Go file upload and parsing API, (2) a PostgreSQL/MinIO data layer for tracklists, tracks, and cover art, (3) an async Go cover-art fetch pipeline using Spotify → Discogs → MusicBrainz, and (4) a Nuxt 4 Playwright screenshot pipeline that renders a styled Vue component to PNG inside Nitro.

The **screenshot pipeline** is the most novel element. The pattern is: Go triggers `GET http://frontend/api/screenshot/tracklist/{id}`, which Nuxt handles via a Nitro server route. That route runs Playwright headlessly against the same Nuxt instance's render route (`/render/tracklist/{id}`). The Nitro server plugin initialises a single Chromium browser at startup and reuses it across requests (singleton pattern). This avoids spawning a new browser per request and keeps memory bounded.

The **Rekordbox parser** in Go must handle UTF-16 LE with BOM using `golang.org/x/text/encoding/unicode`. The `BOMOverride` transformer auto-detects the BOM; after transcoding to UTF-8, a standard `encoding/csv.Reader` with `Comma: '\t'` handles the tab-delimited rows. Artwork and # columns are skipped; Artist may be empty.

The **cover art pipeline** runs as a Go goroutine worker pool after tracklist save. Tracks update their `artwork_status` column (`pending` → `fetched` | `placeholder`) as each fetch resolves. The frontend polls the tracklist detail endpoint to show progressive updates.

**Primary recommendation:** Implement the six Go modules (parser, tracklist, tracks, artwork, storage, screenshot-client) and the two Nuxt modules (render route, screenshot Nitro handler + plugin) as separate, testable units before wiring them together.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `golang.org/x/text` | v0.34.0 (already in go.mod) | UTF-16 LE → UTF-8 transcoding | Official Go sub-repo; `BOMOverride` handles BOM detection automatically |
| `encoding/csv` (stdlib) | Go 1.26 | Tab-delimited parsing after UTF-8 transcode | Zero dependencies; `Comma: '\t'` turns it into a TSV reader |
| `github.com/go-chi/chi/v5` | v5.2.5 (already in go.mod) | HTTP routing for new TRKL endpoints | Already used in Phase 0 |
| `github.com/jackc/pgx/v5` | v5.8.0 (already in go.mod) | PostgreSQL queries | Already used in Phase 0 |
| `github.com/minio/minio-go/v7` | v7.0.99 (already in go.mod) | MinIO object storage: PutObject, PresignedGetObject | Already used in Phase 0 |
| `github.com/google/uuid` | v1.6.0 (already in go.mod) | UUID generation for PKs | Already used in Phase 0 |
| `playwright-core` | npm, matches `@nx/playwright` v1.x | Headless Chromium in Nitro | Official MS library; no separate browser service needed |
| `nuxt` | ^4.0.0 (already in package.json) | SSR render route + Nitro screenshot handler | Already used in Phase 0 |
| `h3` | ^1.8.2 (already in package.json) | Nitro event handlers | Already used in Phase 0 |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/sync/errgroup` | v0.19.0 (already in go.mod via sync) | Bounded goroutine pool for cover art fetch | Concurrent Spotify/Discogs/MB calls per track |
| `net/http` (stdlib) | Go 1.26 | Spotify / Discogs / MusicBrainz HTTP calls | No extra deps; simple `http.Client` with timeout |
| `encoding/json` (stdlib) | Go 1.26 | Unmarshal API responses | Already the pattern in Phase 0 repository |
| `github.com/stretchr/testify` | v1.11.1 (already in go.mod) | Assertions in Go unit tests | Already used in Phase 0 |
| `github.com/testcontainers/testcontainers-go` | v0.41.0 (already in go.mod) | Integration tests against real PostgreSQL | Same TestMain pattern as Phase 0 |
| `@vueuse/core` | latest (add to package.json) | `useDropZone`, `useFileDialog`, `useEventSource` | Best-practice drag-and-drop in Vue 3; composable-based |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `playwright-core` in Nitro | `chromedp` in Go | `chromedp` would need its own browser process; can't leverage the Vue render route |
| `playwright-core` in Nitro | Separate Puppeteer service | Extra Docker service; harder to coordinate with Nuxt rendering |
| `encoding/csv` for TSV | `bufio.Scanner` with manual split | csv.Reader handles quoted edge-cases; Rekordbox has none, but csv is cleaner |
| `golang.org/x/sync/errgroup` | Raw goroutines + WaitGroup | errgroup propagates first error, limits worker count with semaphore |
| Frontend polling for artwork status | WebSocket or SSE | Polling is simpler; cover art fetches take 2–5 s total; SSE adds complexity |

### Installation

```bash
# Go — no new deps needed; golang.org/x/text already in go.mod
# Verify: grep "golang.org/x/text" api/go.mod  → v0.34.0 ✓

# Nuxt — add VueUse and playwright-core
cd apps/dj
pnpm add @vueuse/core
pnpm add -D playwright-core
# Install Chromium binary
npx playwright install chromium
```

---

## Architecture Patterns

### Recommended Project Structure

```
api/
├── internal/
│   ├── platform/           # Phase 0 packages (unchanged)
│   │   ├── migrations/
│   │   │   ├── 001_initial_schema.sql
│   │   │   └── 002_tracklists.sql     # NEW: tracklists + tracks tables
│   ├── tracklist/          # NEW domain package
│   │   ├── model.go        # Tracklist, Track structs; sentinel errors
│   │   ├── parser.go       # Rekordbox UTF-16 LE TSV parser
│   │   ├── parser_test.go
│   │   ├── repository.go   # CRUD against PostgreSQL
│   │   ├── repository_test.go  # testcontainers-go
│   │   ├── service.go      # Business logic: parse+save, trigger artwork
│   │   └── handler.go      # chi route handlers
│   └── artwork/            # NEW domain package
│       ├── model.go        # ArtworkStatus enum, ArtworkResult
│       ├── fetcher.go      # Spotify → Discogs → MB fallback chain
│       ├── fetcher_test.go # table-driven tests with mock HTTP
│       ├── cache.go        # MinIO read/write for artwork objects
│       └── worker.go       # goroutine pool, updates tracks.artwork_status
apps/dj/
├── server/
│   ├── plugins/
│   │   └── playwright.ts   # NEW: Nitro plugin — browser singleton init
│   └── api/
│       ├── render/
│       │   └── tracklist/
│       │       └── [id].get.ts    # NEW: returns styled HTML for screenshot
│       └── screenshot/
│           └── tracklist/
│               └── [id].get.ts    # NEW: runs Playwright, returns PNG buffer
├── app/
│   ├── pages/
│   │   ├── tracklist.vue          # NEW: upload + preview + past list
│   │   └── render/tracklist/[id].vue  # NEW: render-only card (no nav)
│   └── components/
│       └── TrackcardPreview.vue   # NEW: styled card component
```

### Pattern 1: Rekordbox UTF-16 LE TSV Parser

**What:** Read a UTF-16 LE BOM file from a `multipart.File`, transcode to UTF-8 using `BOMOverride`, then parse tab-delimited rows.
**When to use:** `POST /api/v1/tracklists/upload` handler; also called for Serato/Traktor format detection.

```go
// Source: pkg.go.dev/golang.org/x/text/encoding/unicode
import (
    "encoding/csv"
    "golang.org/x/text/encoding/unicode"
    "golang.org/x/text/transform"
    "io"
)

func ParseRekordboxTSV(r io.Reader) ([]Track, []ParseWarning, error) {
    // UTF-16 LE fallback; BOMOverride switches to UTF-8 if BOM is absent
    fallback := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
    decoder := unicode.BOMOverride(fallback)
    utf8Reader := transform.NewReader(r, decoder)

    cr := csv.NewReader(utf8Reader)
    cr.Comma = '\t'
    cr.LazyQuotes = true   // Rekordbox does not quote fields but defence-in-depth
    cr.FieldsPerRecord = -1 // tolerate variable column counts

    headers, err := cr.Read() // first row
    if err != nil { return nil, nil, err }
    colIdx := buildColumnIndex(headers) // map col name → index

    var tracks []Track
    var warnings []ParseWarning
    pos := 0
    for {
        row, err := cr.Read()
        if err == io.EOF { break }
        if err != nil {
            warnings = append(warnings, ParseWarning{Row: pos, Err: err})
            continue
        }
        t, w := mapRow(row, colIdx, pos)
        tracks = append(tracks, t)
        warnings = append(warnings, w...)
        pos++
    }
    if len(tracks) == 0 {
        return nil, warnings, ErrZeroTracks
    }
    return tracks, warnings, nil
}
```

### Pattern 2: Nitro Playwright Browser Singleton

**What:** A Nitro plugin that launches one Chromium browser at server startup, stores it on `nitroApp`, and provides a helper used by the screenshot handler.
**When to use:** Always — one browser process per Nuxt container.

```typescript
// Source: Playwright Library docs + Nuxt server/plugins pattern
// apps/dj/server/plugins/playwright.ts
import { chromium, type Browser } from 'playwright-core'

export default defineNitroPlugin(async (nitroApp) => {
  const browser: Browser = await chromium.launch({
    headless: true,
    args: [
      '--no-sandbox',
      '--disable-setuid-sandbox',
      '--disable-dev-shm-usage',  // critical for Docker
      '--disable-gpu',
    ],
  })

  // Attach to nitroApp so handlers can access it
  ;(nitroApp as any)._playwright = browser

  // Graceful shutdown
  nitroApp.hooks.hookOnce('close', async () => {
    await browser.close()
  })
})
```

### Pattern 3: Nitro Screenshot Handler

**What:** Nitro server route that opens a page in the singleton browser, navigates to the render route, waits for network idle, and returns a PNG buffer.
**When to use:** `GET /api/screenshot/tracklist/[id]?format=story|square`

```typescript
// Source: Playwright Library docs (playwright.dev/docs/library)
// apps/dj/server/api/screenshot/tracklist/[id].get.ts
export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  const format = getQuery(event).format ?? 'story'
  const { width, height } = format === 'square'
    ? { width: 1080, height: 1080 }
    : { width: 1080, height: 1920 }

  const nitroApp = useNitroApp() as any
  const browser = nitroApp._playwright

  const context = await browser.newContext({
    viewport: { width, height },
    deviceScaleFactor: 2,      // retina — actual PNG is 2160×3840 or 2160×2160
  })
  const page = await context.newPage()

  // Navigate to the SSR render route on THIS Nuxt process
  // In Docker: frontend is on localhost:3000 from its own perspective
  const baseUrl = process.env.NUXT_APP_BASE_URL ?? 'http://localhost:3000'
  await page.goto(`${baseUrl}/render/tracklist/${id}`, {
    waitUntil: 'networkidle',
    timeout: 15_000,
  })

  const buffer = await page.screenshot({
    type: 'png',
    fullPage: false,  // viewport-sized only
  })

  await context.close()

  setResponseHeader(event, 'Content-Type', 'image/png')
  return buffer
})
```

### Pattern 4: Go Calls Nuxt Screenshot Endpoint and Stores in MinIO

**What:** After tracklist is confirmed and saved, Go triggers screenshot generation and stores the result in MinIO, returning the presigned URL.
**When to use:** `POST /api/v1/tracklists/{id}/generate-image`

```go
// Source: minio-go/v7 official examples + net/http stdlib
func (s *Service) GenerateImage(ctx context.Context, id uuid.UUID, format string) (string, error) {
    nuxtBase := s.config.NuxtInternalURL // e.g. http://frontend:3000
    url := fmt.Sprintf("%s/api/screenshot/tracklist/%s?format=%s", nuxtBase, id, format)

    resp, err := s.httpClient.Get(url)
    if err != nil { return "", fmt.Errorf("screenshot request: %w", err) }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("screenshot returned %d", resp.StatusCode)
    }

    objectKey := fmt.Sprintf("tracklist-images/%s/%s.png", id, format)
    _, err = s.storage.PutObject(ctx, "klubhub", objectKey, resp.Body,
        resp.ContentLength, minio.PutObjectOptions{ContentType: "image/png"})
    if err != nil { return "", fmt.Errorf("minio store: %w", err) }

    presigned, err := s.storage.PresignedGetObject(ctx, "klubhub", objectKey,
        7*24*time.Hour, nil)
    if err != nil { return "", fmt.Errorf("presign: %w", err) }

    return presigned.String(), nil
}
```

### Pattern 5: Cover Art Fetch Chain (Goroutine Worker Pool)

**What:** After tracklist save, fire a goroutine worker for each track. Workers try Spotify → Discogs → MusicBrainz/CAA in order, caching hits to MinIO, updating `artwork_status` in DB.
**When to use:** Called from `tracklist.Service.SaveTracklist` with `go s.fetchAllArtwork(ctx, tracklist)`.

```go
// Source: Go by Example Worker Pools + net/http stdlib patterns
func (s *ArtworkService) FetchAll(ctx context.Context, tracks []Track) {
    const workers = 4
    jobs := make(chan Track, len(tracks))
    for _, t := range tracks { jobs <- t }
    close(jobs)

    var wg sync.WaitGroup
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for t := range jobs {
                result := s.fetchOne(ctx, t)
                s.repo.UpdateArtworkStatus(ctx, t.ID, result)
            }
        }()
    }
    wg.Wait()
}

func (s *ArtworkService) fetchOne(ctx context.Context, t Track) ArtworkResult {
    // 1. Check MinIO cache by content hash key
    if cached := s.cache.Get(ctx, t.cacheKey()); cached != "" {
        return ArtworkResult{URL: cached, Source: "cache"}
    }
    // 2. Try Spotify
    if url, err := s.spotify.Search(ctx, t.Title, t.Artist); err == nil {
        s.cache.Put(ctx, t.cacheKey(), url)
        return ArtworkResult{URL: url, Source: "spotify"}
    }
    // 3. Try Discogs
    if url, err := s.discogs.Search(ctx, t.Title, t.Artist); err == nil {
        s.cache.Put(ctx, t.cacheKey(), url)
        return ArtworkResult{URL: url, Source: "discogs"}
    }
    // 4. Try MusicBrainz → Cover Art Archive
    if url, err := s.musicbrainz.Search(ctx, t.Title, t.Artist); err == nil {
        s.cache.Put(ctx, t.cacheKey(), url)
        return ArtworkResult{URL: url, Source: "musicbrainz"}
    }
    // 5. Placeholder
    return ArtworkResult{URL: s.placeholderURL, Source: "placeholder"}
}
```

### Anti-Patterns to Avoid

- **Spawning a new browser per screenshot request:** Each `chromium.launch()` takes 1–3 seconds and uses ~100 MB RAM. Use the singleton plugin pattern instead.
- **Storing artwork URLs pointing to external CDNs:** External URLs expire or move. Always cache artwork bytes to MinIO and serve presigned URLs.
- **Parsing TSV with `strings.Split(line, "\t")`:** Loses BOM handling, doesn't handle edge cases. Use `csv.Reader` with `Comma: '\t'` after UTF-16 transcode.
- **Running `npx playwright install` in the Dockerfile CMD:** This installs browsers at container start and adds 30-60 seconds cold-start. Install at image build time.
- **Using Alpine Linux for the Nuxt Docker image:** Playwright's Chromium requires glibc, which Alpine lacks. Use `node:22-bookworm-slim` (Debian-based).
- **Blocking the HTTP handler goroutine while fetching all cover art:** Fire the artwork goroutine pool in a background goroutine; return the tracklist immediately to the client.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| UTF-16 BOM detection | Custom byte sniffing | `unicode.BOMOverride` from `golang.org/x/text` | BOM detection is finicky; the transformer handles multi-encoding detection correctly |
| Browser lifecycle management | Custom pool/process manager | Nitro plugin singleton pattern | Browser crashes, restarts, port management are solved by Playwright's own process supervision |
| Image resizing for 50% preview | Server-side resize | CSS `transform: scale(0.5)` on the Vue component | Zero compute; the export is the authoritative size |
| Presigned URL signing | Custom HMAC URL | `minio.PresignedGetObject` | Signing algorithm differences between MinIO versions; library always correct |
| Spotify token refresh | Manual expiry tracking | Simple in-memory token cache with `time.Now().Add(3550*time.Second)` (reuse until 50s before expiry) | Spotify Client Credentials tokens last 3600s; simple expiry check avoids token churn |
| MusicBrainz search + CAA resolution | One-shot URL guess | Two-step: search `/ws/2/recording`, extract release MBID, then `coverartarchive.org/release/{mbid}/front` | CAA redirects are the correct API contract |

**Key insight:** The browser and the UTF-16 transcoder are the two highest-risk components. Both are solved by battle-tested libraries; custom implementations have failed in production in documented ways.

---

## Common Pitfalls

### Pitfall 1: Chromium Executable Missing in Production Docker Container

**What goes wrong:** `playwright-core` in Node.js does not bundle a browser. If the Dockerfile copies only the `.output` directory (as in the current Phase 0 `apps/dj/Dockerfile`), the Chromium binary installed during `npm install` is not present in the production image.

**Why it happens:** `npx playwright install chromium` installs to `~/.cache/ms-playwright` on the build host. The current multi-stage Dockerfile (`FROM node:22-alpine AS builder → COPY --from=builder /app/apps/dj/.output ./.output`) discards this cache.

**How to avoid:** Switch the production base image to `node:22-bookworm-slim`, install system font + Chromium deps in the builder stage, and add `RUN npx playwright install chromium` before `pnpm nx build dj`. Then copy `~/.cache/ms-playwright` into the final image alongside `.output`.

**Warning signs:** Container start log shows `Executable doesn't exist at /root/.cache/ms-playwright/...` on the first screenshot request.

**Corrected Dockerfile pattern:**
```dockerfile
FROM node:22-bookworm-slim AS builder
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends \
    fonts-noto fonts-noto-cjk \
    libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 \
    libdrm2 libxkbcommon0 libxcomposite1 libxdamage1 \
    libxrandr2 libgbm1 libasound2 libpangocairo-1.0-0 \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g pnpm
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/dj/package.json apps/dj/
RUN pnpm install --frozen-lockfile
RUN npx playwright install chromium   # install browser at build time
COPY . .
RUN pnpm nx build dj

FROM node:22-bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends \
    fonts-noto fonts-noto-cjk \
    libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 \
    libdrm2 libxkbcommon0 libxcomposite1 libxdamage1 \
    libxrandr2 libgbm1 libasound2 libpangocairo-1.0-0 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/apps/dj/.output ./.output
COPY --from=builder /root/.cache/ms-playwright /root/.cache/ms-playwright
ENV PORT=3000
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
```

### Pitfall 2: --disable-dev-shm-usage Is Mandatory in Docker

**What goes wrong:** Chromium's renderer process crashes silently inside Docker with the default `/dev/shm` size (64 MB). The Nitro handler times out with no useful error.

**Why it happens:** Docker defaults `/dev/shm` to 64 MB. Chromium's renderer processes use shared memory for page rendering. With a 1080×1920 canvas, this overflows easily.

**How to avoid:** Always pass `--disable-dev-shm-usage` in `chromium.launch({ args: [...] })`. This tells Chromium to use `/tmp` instead of `/dev/shm`. Also set `--no-sandbox` and `--disable-setuid-sandbox` when running as root (default in Docker).

### Pitfall 3: UTF-16 LE BOM Edge Cases

**What goes wrong:** Some Rekordbox versions emit files with the BOM (`\xFF\xFE`) and some emit without. Some files have Windows line endings (`\r\n`). Naive byte-slice parsing silently corrupts data.

**Why it happens:** The BOM is optional per the Unicode spec. `csv.Reader` does not perform encoding detection.

**How to avoid:** Always wrap the reader with `unicode.BOMOverride(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder())`. The `transform.NewReader` wraps the original `io.Reader` transparently. Add `cr.LazyQuotes = true` and trimming `\r` from field values as a defensive measure.

### Pitfall 4: `page.goto` Hitting Localhost from Inside Docker

**What goes wrong:** The Nitro screenshot handler calls `http://localhost:3000/render/tracklist/{id}` which works in dev but fails in Docker because the container doesn't have a loopback server on that port during `goto`.

**Why it happens:** Inside Docker, the Nuxt server is the process running the Nitro handler. Calling `localhost:3000` from within Playwright works when the Nuxt server is actually listening on 3000 — which it is, on itself. However, `process.env.NUXT_APP_BASE_URL` may resolve to `0.0.0.0` or an external hostname.

**How to avoid:** Always use `http://localhost:${process.env.PORT ?? 3000}` for the internal self-call. This is the local loop inside the same container. Do NOT use the external hostname or Docker service name (that would go out via the Docker network unnecessarily).

### Pitfall 5: MusicBrainz Rate Limiting

**What goes wrong:** Sending one HTTP request per track without a User-Agent header gets IP-blocked by MusicBrainz (1 req/s limit; blocks at 503 for anonymous agents).

**Why it happens:** MusicBrainz requires a `User-Agent` string identifying the application and contact info.

**How to avoid:** Set `User-Agent: KlubHub-DJ/1.0 (contact@example.com)` on every request. Add a simple rate limiter (1 req/s via `time.Ticker`) in the MusicBrainz client. The Cover Art Archive has no rate limit.

### Pitfall 6: Spotify Token Expiry Mid-Batch

**What goes wrong:** A cover art fetch batch starts, the Spotify access token expires mid-batch, and all remaining tracks silently get placeholder art.

**Why it happens:** Spotify Client Credentials tokens expire in 3600 seconds. A 100-track set takes ~30-60 seconds to process, but a token obtained 3500 seconds ago will expire during processing.

**How to avoid:** Store the token and its `obtainedAt` timestamp. Before each Spotify call, check if `time.Since(obtainedAt) > 3550*time.Second` and refresh proactively.

### Pitfall 7: `deviceScaleFactor: 2` Doubles PNG File Size

**What goes wrong:** A 1080×1920 card at `deviceScaleFactor: 2` produces a 2160×3840 PNG that is 8–15 MB. This is correct for social media but MinIO storage and presigned URL delivery latency may surprise users.

**Why it happens:** `deviceScaleFactor: 2` physically doubles pixel density. This is desirable (retina quality) but needs to be documented so consumers know the final image dimensions.

**How to avoid:** Accept `deviceScaleFactor: 2` as the correct design choice. Note in the API response that `image_width: 2160`, `image_height: 3840`. For the 50% preview in the browser, use the generated image with `width: 50%` CSS — no need to generate a separate low-res image.

---

## Code Examples

Verified patterns from official sources:

### UTF-16 LE BOM Auto-Detection (Go)

```go
// Source: pkg.go.dev/golang.org/x/text/encoding/unicode (v0.34.0 — in go.mod)
import (
    "golang.org/x/text/encoding/unicode"
    "golang.org/x/text/transform"
    "io"
)

// NewUTF16LEReader wraps any reader that may emit UTF-16 LE with optional BOM.
// BOMOverride: if \xFF\xFE is present at the start, the decoder uses UTF-16 LE.
// If \xFE\xFF is present (UTF-16 BE), the decoder switches to BE.
// If no BOM, falls back to the provided fallback (UTF-16 LE / IgnoreBOM).
func NewUTF16LEReader(r io.Reader) io.Reader {
    fallback := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
    decoder := unicode.BOMOverride(fallback)
    return transform.NewReader(r, decoder)
}
```

### Spotify Client Credentials Token (Go)

```go
// Source: developer.spotify.com/documentation/web-api/tutorials/client-credentials-flow
func (c *SpotifyClient) fetchToken(ctx context.Context) error {
    form := url.Values{"grant_type": {"client_credentials"}}
    req, _ := http.NewRequestWithContext(ctx, "POST",
        "https://accounts.spotify.com/api/token",
        strings.NewReader(form.Encode()))
    creds := base64.StdEncoding.EncodeToString(
        []byte(c.clientID + ":" + c.clientSecret))
    req.Header.Set("Authorization", "Basic "+creds)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := c.http.Do(req)
    // ... decode resp.Body into SpotifyTokenResponse
}
```

### Spotify Track Search + Cover Art URL (Go)

```go
// Endpoint: GET https://api.spotify.com/v1/search?q=artist:X+track:Y&type=track&limit=1
// Response: tracks.items[0].album.images[0].url (widest first)
func (c *SpotifyClient) Search(ctx context.Context, title, artist string) (string, error) {
    q := fmt.Sprintf("artist:%s track:%s", url.QueryEscape(artist), url.QueryEscape(title))
    endpoint := fmt.Sprintf("https://api.spotify.com/v1/search?q=%s&type=track&limit=1", q)
    // ... GET with Bearer token, decode response, return images[0].url
}
```

### MusicBrainz Recording Search + Cover Art Archive (Go)

```go
// Step 1: GET https://musicbrainz.org/ws/2/recording?query=artist:X+AND+recording:Y&fmt=json&limit=1
// Response: recordings[0].releases[0].id  → release MBID
// Step 2: GET https://coverartarchive.org/release/{mbid}/front
// CAA returns 307 redirect; follow redirect to get the JPEG URL
func (c *MBClient) Search(ctx context.Context, title, artist string) (string, error) {
    q := fmt.Sprintf("artist:%s AND recording:%s",
        url.QueryEscape(artist), url.QueryEscape(title))
    endpoint := fmt.Sprintf(
        "https://musicbrainz.org/ws/2/recording?query=%s&fmt=json&limit=1&inc=releases", q)
    req, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
    req.Header.Set("User-Agent", "KlubHub-DJ/1.0 (admin@klubhub.io)")
    // ... extract releases[0].id
    // then: coverartarchive.org/release/{mbid}/front — follow redirect
}
```

### MinIO PutObject + PresignedGetObject (Go)

```go
// Source: github.com/minio/minio-go/v7 — already in go.mod at v7.0.99
_, err = mc.PutObject(ctx, bucket, objectKey, reader, size,
    minio.PutObjectOptions{ContentType: "image/png"})

presignedURL, err := mc.PresignedGetObject(ctx, bucket, objectKey,
    7*24*time.Hour,   // max: 7 days
    nil,              // no override query params
)
// presignedURL is *url.URL; use .String() for serialization
```

### Playwright browser.newContext for 1080×1920 Retina (TypeScript)

```typescript
// Source: playwright.dev/docs/library + playwright.dev/docs/emulation
const context = await browser.newContext({
  viewport: { width: 1080, height: 1920 },
  deviceScaleFactor: 2,   // produces 2160x3840 physical pixels
})
const page = await context.newPage()
await page.goto(renderUrl, { waitUntil: 'networkidle', timeout: 15_000 })
const buffer = await page.screenshot({ type: 'png', fullPage: false })
await context.close()   // always close context to free GPU/RAM
```

### Vue 3 Inline-Edit Table Row (TypeScript)

```typescript
// Composable pattern for per-row edit mode
const editingId = ref<string | null>(null)
const editBuffer = ref<Partial<Track>>({})

function startEdit(track: Track) {
  editingId.value = track.id
  editBuffer.value = { ...track }
}
async function saveEdit(trackId: string) {
  await $fetch(`/api/v1/tracklists/${props.tracklistId}/tracks/${trackId}`,
    { method: 'PUT', body: editBuffer.value })
  editingId.value = null
  await refresh()   // useAsyncData refresh
}
```

---

## Database Schema Design

### Migration 002: tracklists + tracks

```sql
-- +goose Up

-- Tracklists: one per DJ set / upload
CREATE TABLE tracklists (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title               TEXT NOT NULL DEFAULT '',
    set_date            DATE,
    source_format       TEXT NOT NULL DEFAULT 'rekordbox'
                            CHECK (source_format IN ('rekordbox','serato','traktor')),
    source_file_path    TEXT NOT NULL DEFAULT '', -- MinIO key for raw upload
    preset              TEXT NOT NULL DEFAULT 'default',
    background_mode     TEXT NOT NULL DEFAULT 'solid'
                            CHECK (background_mode IN ('solid','upload','mosaic')),
    background_color    TEXT NOT NULL DEFAULT '#000000',
    background_image    TEXT NOT NULL DEFAULT '', -- MinIO key if mode=upload
    visible_fields      JSONB NOT NULL DEFAULT '{}',
    max_tracks          INT NOT NULL DEFAULT 30,
    story_image_path    TEXT NOT NULL DEFAULT '', -- MinIO key for 1080x1920 PNG
    square_image_path   TEXT NOT NULL DEFAULT '', -- MinIO key for 1080x1080 PNG
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);
CREATE INDEX ON tracklists(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX ON tracklists(created_at DESC) WHERE deleted_at IS NULL;

-- Tracks: child rows of a tracklist
CREATE TABLE tracks (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tracklist_id        UUID NOT NULL REFERENCES tracklists(id) ON DELETE CASCADE,
    position            INT NOT NULL,
    title               TEXT NOT NULL DEFAULT '',
    artist              TEXT NOT NULL DEFAULT '',
    album               TEXT NOT NULL DEFAULT '',
    genre               TEXT NOT NULL DEFAULT '',
    bpm                 NUMERIC(6,2),
    rating              INT NOT NULL DEFAULT 0,
    duration_seconds    INT,
    key_notation        TEXT NOT NULL DEFAULT '',
    date_added          DATE,
    artwork_path        TEXT NOT NULL DEFAULT '', -- MinIO key for cached cover art
    artwork_status      TEXT NOT NULL DEFAULT 'pending'
                            CHECK (artwork_status IN ('pending','fetched','placeholder','manual')),
    artwork_source      TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    UNIQUE (tracklist_id, position)
);
CREATE INDEX ON tracks(tracklist_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS tracks;
DROP TABLE IF EXISTS tracklists;
```

**Schema design rationale (discretion areas):**
- `artwork_status` column lets the frontend poll for per-track progress without a WebSocket
- `source_file_path` stores the raw MinIO key; MinIO object is never modified (TRKL-08)
- `story_image_path` and `square_image_path` are blank until generation is triggered
- Soft-delete on tracklists + cascade ON DELETE to tracks (TRKL-27 requirement)

---

## Go API Routes

```
POST   /api/v1/tracklists/upload        — upload + parse; returns parsed []Track (not yet saved)
POST   /api/v1/tracklists               — save confirmed tracklist; triggers artwork fetch
GET    /api/v1/tracklists               — list (paginated, ?page=1&per_page=20)
GET    /api/v1/tracklists/{id}          — get tracklist + tracks (includes artwork_status per track)
PUT    /api/v1/tracklists/{id}          — update tracklist metadata + field config (optimistic concurrency)
DELETE /api/v1/tracklists/{id}          — soft-delete tracklist + cascade tracks
PUT    /api/v1/tracklists/{id}/tracks/{track_id}          — inline-edit one track
PUT    /api/v1/tracklists/{id}/tracks/{track_id}/artwork  — upload custom artwork (multipart)
POST   /api/v1/tracklists/{id}/generate-image             — trigger screenshot; returns {story_url, square_url}
```

---

## MinIO Object Paths

| Object Type | Path Pattern |
|-------------|-------------|
| Raw upload | `raw-uploads/{tracklist_id}/{filename}` |
| Cover art cached | `cover-art/{sha256_of_title_artist}.jpg` |
| Custom per-track artwork | `track-artwork/{track_id}.jpg` |
| Generated story PNG | `tracklist-images/{tracklist_id}/story.png` |
| Generated square PNG | `tracklist-images/{tracklist_id}/square.png` |
| Placeholder art | `cover-art/placeholder.png` (one global object) |

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Separate Puppeteer service | Playwright inside Nitro plugin | ~2023 | Eliminates extra Docker service |
| Alpine-based Node Docker images | Debian-slim (`bookworm-slim`) for Playwright | 2022+ | Alpine/musl doesn't support Chromium's glibc deps |
| Hand-rolled UTF-16 byte parser | `golang.org/x/text BOMOverride` | Stable | Handles all BOM edge cases including missing BOM |
| Polling for image ready state | Still polling (SSE is overkill here) | N/A | Cover art takes 2-5s; polling every 1s is acceptable |
| Global presigned URL signing | `minio-go/v7 PresignedGetObject` with 7-day TTL | Stable | Direct API; handles signing algorithm correctly |

**Deprecated/outdated:**
- `github.com/lib/pq` for PostgreSQL: replaced by pgx/v5 (already using pgx/v5 in Phase 0)
- `@playwright/test` for screenshot: the test runner is overkill; `playwright-core` library mode is correct for this use case

---

## Open Questions

1. **Serato and Traktor parser scope in Phase 1**
   - What we know: TRKL-02 says "auto-detect Rekordbox, Serato, Traktor" but CONTEXT.md only documents the Rekordbox format in detail
   - What's unclear: Whether Serato and Traktor parsers need to be production-ready in Phase 1 or just stubbed
   - Recommendation: Implement Rekordbox fully; add format-detection stubs for Serato/Traktor that return a clear "format not yet supported" error — satisfies TRKL-02 partially and unblocks Phase 1 end-to-end

2. **Cover art cache key strategy when Artist is empty**
   - What we know: Rekordbox exports often have empty Artist field (CONTEXT.md); cache key would be hash of title only
   - What's unclear: Risk of false-positive cache hits when different tracks share a title
   - Recommendation: Cache key = `sha256(lowercase(title) + "|" + lowercase(artist))`; empty artist is a valid component; miss is acceptable for popular title collisions

3. **`NUXT_APP_BASE_URL` for self-referencing Playwright call in Docker**
   - What we know: The Nitro handler must call `http://localhost:{PORT}/render/tracklist/{id}` to screenshot itself
   - What's unclear: Whether the Nuxt server is guaranteed to be listening before the plugin runs and before any screenshot requests arrive
   - Recommendation: In the Nitro plugin `playwright.ts`, do not call `goto` at startup. The browser is launched lazily per-request or eagerly at plugin init. The Nuxt process is fully started before Nitro plugins run, so `localhost:3000` is safe.

4. **Discogs image URL policy**
   - What we know: `go-discogs` library provides `BasicInformation.CoverImage` field
   - What's unclear: Whether Discogs CDN URLs are stable for caching, or if they expire like S3 presigned URLs
   - Recommendation: Always download and re-upload Discogs image bytes to MinIO rather than storing the Discogs URL directly; this ensures the artwork remains accessible even if Discogs CDN URLs change

---

## Validation Architecture

> `workflow.nyquist_validation` is `true` in `.planning/config.json` — this section is included.

### Test Framework

| Property | Value |
|----------|-------|
| Framework (Go) | `testing` stdlib + testify v1.11.1 + testcontainers-go v0.41.0 |
| Framework (Nuxt) | Vitest v4.0.8 (unit), `@playwright/test` v1.36.0 (e2e) |
| Go config | `go test ./...` from `api/` |
| Nuxt config | `pnpm nx test dj` (vitest) |
| Quick run (Go) | `cd api && go test ./internal/tracklist/... -run TestParse -count=1` |
| Full suite (Go) | `cd api && go test ./... -timeout 120s` |
| Quick run (Nuxt) | `pnpm nx test dj --testNamePattern="parser"` |
| Full suite (Nuxt) | `pnpm nx test dj` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TRKL-02 | Rekordbox UTF-16 LE TSV detected and parsed | Unit | `go test ./internal/tracklist/... -run TestParseRekordbox` | Wave 0 |
| TRKL-03 | All fields extracted (title, artist, BPM, key, duration) | Unit | `go test ./internal/tracklist/... -run TestTrackFields` | Wave 0 |
| TRKL-04 | Invalid file format returns structured error | Unit | `go test ./internal/tracklist/... -run TestInvalidFormat` | Wave 0 |
| TRKL-05 | Files > 50 MB rejected | Unit | `go test ./internal/tracklist/... -run TestMaxUploadSize` | Wave 0 |
| TRKL-06 | Partial parse returns warnings, keeps valid tracks | Unit | `go test ./internal/tracklist/... -run TestPartialParse` | Wave 0 |
| TRKL-07 | Zero tracks returns ErrZeroTracks | Unit | `go test ./internal/tracklist/... -run TestZeroTracks` | Wave 0 |
| TRKL-08 | Inline-edit track field persists to DB | Integration | `go test ./internal/tracklist/... -run TestUpdateTrack` | Wave 0 |
| TRKL-09 | Tracklist + tracks saved to PostgreSQL; raw file in MinIO | Integration | `go test ./internal/tracklist/... -run TestSaveTracklist` | Wave 0 |
| TRKL-10 | Artwork fetch chain: Spotify → Discogs → MB | Unit (mock HTTP) | `go test ./internal/artwork/... -run TestFetchChain` | Wave 0 |
| TRKL-11 | Artwork cached to MinIO, not re-fetched | Unit | `go test ./internal/artwork/... -run TestArtworkCache` | Wave 0 |
| TRKL-17 | Screenshot produced at 1080×1920 and 1080×1080 | E2E (Playwright) | `pnpm nx e2e dj-e2e --grep="screenshot dimensions"` | Wave 0 |
| TRKL-26 | CJK/Arabic characters render without boxes | E2E (Playwright visual) | `pnpm nx e2e dj-e2e --grep="unicode rendering"` | Wave 0 |
| TRKL-27 | Delete soft-deletes tracklist and cascade tracks | Integration | `go test ./internal/tracklist/... -run TestDeleteCascade` | Wave 0 |

### Sampling Rate

- **Per task commit:** `cd api && go test ./internal/tracklist/... -timeout 30s`
- **Per wave merge:** `cd api && go test ./... -timeout 120s && pnpm nx test dj`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `api/internal/tracklist/parser_test.go` — covers TRKL-02, TRKL-03, TRKL-04, TRKL-05, TRKL-06, TRKL-07
- [ ] `api/internal/tracklist/repository_test.go` — covers TRKL-08, TRKL-09, TRKL-27 (testcontainers-go)
- [ ] `api/internal/artwork/fetcher_test.go` — covers TRKL-10, TRKL-11 (mock HTTP server)
- [ ] `api/internal/platform/migrations/002_tracklists.sql` — schema for all integration tests
- [ ] `apps/dj-e2e/src/screenshot.spec.ts` — covers TRKL-17, TRKL-26 (needs running stack)
- [ ] Testdata file: `api/testdata/rekordbox_sample.txt` — UTF-16 LE TSV with BOM, 5 tracks including CJK titles

---

## Sources

### Primary (HIGH confidence)

- `pkg.go.dev/golang.org/x/text/encoding/unicode` — UTF-16 BOM handling, `BOMOverride` API
- `playwright.dev/docs/library` — browser launch, `newContext`, `page.screenshot` TypeScript patterns
- `playwright.dev/docs/docker` — recommended base image (Debian/Ubuntu, not Alpine), Chromium deps
- `nuxt.com/docs/4.x/guide/directory-structure/server` — Nitro plugins, server handlers, directory structure
- `developer.spotify.com/documentation/web-api/tutorials/client-credentials-flow` — token endpoint format, response shape
- `musicbrainz.org/doc/Cover_Art_Archive/API` + `musicbrainz.org/doc/MusicBrainz_API/Search` — CAA redirect pattern, recording search
- `pkg.go.dev/github.com/minio/minio-go/v7` — `PutObject`, `PresignedGetObject` signatures

### Secondary (MEDIUM confidence)

- `github.com/nuxt/nuxt/discussions/26590` — Nitro singleton pattern confirmed by Nuxt maintainer
- `github.com/irlndts/go-discogs` — `SearchRequest.ReleaseTitle`, `SearchRequest.Artist`, `BasicInformation.CoverImage`
- Docker font research: Playwright Issue #10036 + Automattic/wp-calypso Issue #63771 — `fonts-noto`, `fonts-noto-cjk` apt packages on Debian

### Tertiary (LOW confidence — flag for validation)

- Discogs CDN URL stability: assumed URLs are long-lived based on community reports; recommend downloading bytes regardless
- Serato/Traktor format specifics: not researched in depth; stubbing recommended for Phase 1

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all dependencies already in go.mod or package.json; APIs verified via official docs
- Architecture: HIGH — Nitro singleton pattern confirmed; DB schema follows established Phase 0 conventions
- Screenshot pipeline: HIGH — Playwright library API verified; Docker deps verified via official Playwright Docker docs
- Cover art APIs: MEDIUM — Spotify/MusicBrainz verified via official docs; Discogs via community library
- Pitfalls: HIGH — all based on real documented bugs/issues from official GitHub trackers

**Research date:** 2026-03-14
**Valid until:** 2026-04-14 (30 days — stable stack; Nuxt 4 minor releases unlikely to break patterns)
