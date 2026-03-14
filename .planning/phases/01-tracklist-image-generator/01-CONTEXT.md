# Phase 1: Tracklist Image Generator — Context

**Gathered:** 2026-03-14
**Status:** Ready for planning
**Source:** /gsd:discuss-phase interactive session

<domain>
## Phase Boundary

Parse Rekordbox history exports (UTF-16 LE TSV), store tracklists in PostgreSQL, and generate shareable card images using a Nuxt SSR screenshot pipeline. The user uploads a file, reviews/edits the parsed tracks, confirms, and the image is generated on demand. No public share URLs in Phase 1 — local access only.

</domain>

<decisions>
## Implementation Decisions

### Image Generation Pipeline
- **Approach: Nuxt SSR screenshot via Playwright inside Nitro**
- Nuxt exposes an internal render route (e.g. `/api/render/tracklist/[id]`) that returns the tracklist card as HTML
- A Nuxt Nitro server handler (`/api/screenshot/tracklist/[id]`) runs Playwright internally (inside the Node/Nuxt process) to screenshot that page and return the PNG binary
- Go backend calls the Nuxt screenshot endpoint (`http://frontend/api/screenshot/tracklist/[id]`) to retrieve the PNG
- Go stores the PNG in MinIO and returns the object URL to the client
- Playwright runs as a Nitro plugin — no separate service

### Upload Flow (Two-Step)
1. User uploads Rekordbox `.txt` export file (UTF-16 LE TSV format)
2. Go parses the file: validates UTF-16 LE encoding, splits TSV columns (#, Artwork, Track Title, Artist, Album, Genre, BPM, Rating, Time, Key, Date Added)
3. Frontend shows a review table with **inline editing** — user can fix track titles, artists, BPM, key before saving
4. User clicks "Save Tracklist" → Go persists to DB → triggers screenshot generation → preview displayed

### Customization
- Global settings only: `dj_name`, `logo_path`, `default_colors` pulled from `user_settings` table
- No per-set template overrides in Phase 1
- Card layout uses a single default template

### Frontend Page Structure
- Single page: `pages/tracklist.vue`
- Left panel: file upload dropzone + parsed track list (editable table)
- Right panel: generated card image preview + download button
- Past tracklists listed below (paginated)

### Share/Privacy
- No public shareable URLs in Phase 1 — local/private only
- Tracklists accessible on your own instance only

### Key Format
- Rekordbox exports mixed Camelot (`8B`, `11A`) and standard notation (`C, Am`) — store as-is, display as-is
- No key normalization in Phase 1

### Claude's Discretion
- Database schema for tracklists and tracks tables (UUID PK, soft-delete pattern from Phase 0)
- Rekordbox TSV parser implementation details (encoding detection, error handling)
- Playwright configuration for screenshot sizing and quality
- Pagination approach for tracklist list view
- Error states for failed parses (malformed file, wrong encoding)
- MinIO path convention for stored PNGs

</decisions>

<specifics>
## Specific Ideas

### Real Rekordbox Export Format (from user-provided example)
```
#\tArtwork\tTrack Title\tArtist\tAlbum\tGenre\tBPM\tRating\tTime\tKey\tDate Added
1\t\tAFRICA (TOTO)\t\t\tMix\t103\t0\t4:48\t8B\t2025/10/23
```
- Encoding: **UTF-16 LE with BOM** (`\xFF\xFE`)
- Separator: **tab** (`\t`)
- First row: column headers
- Artwork column: always empty in exports
- Artist column: often empty (only track title populated)
- Key column: Camelot notation (`8B`, `11A`, etc.)
- Date format: `YYYY/MM/DD`
- No quotes around fields

### Go Rekordbox Parser
- Detect BOM, transcode UTF-16 LE → UTF-8 using `golang.org/x/text/encoding/unicode`
- Skip `#` (number) and `Artwork` columns
- Handle rows where Artist is empty string
- Return `[]Track` struct: `{Position, Title, Artist, Album, Genre, BPM, Rating, DurationSeconds, Key, DateAdded}`

### Vue Tracklist Card Component
- `components/TrackcardPreview.vue` — the component rendered for screenshot
- Card shows: DJ name (from settings), set date, tracklist title, track rows
- Styled per `default_colors` from user_settings

</specifics>

<deferred>
## Deferred Ideas

- Public shareable URL (`/share/[id]`) — Phase 1 explicitly out of scope
- Per-set template overrides — deferred to a future phase
- Social media sharing integrations — Phase 2
- Key normalization / display toggle — future enhancement
- Advanced layout algorithm for large tracklists (50+ tracks) — future

</deferred>

---

*Phase: 01-tracklist-image-generator*
*Context gathered: 2026-03-14 via /gsd:discuss-phase*
