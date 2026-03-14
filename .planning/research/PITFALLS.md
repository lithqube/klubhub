# Pitfalls Research

**Domain:** Self-hosted DJ career toolkit — Go modular monolith + Nuxt 4 + PostgreSQL + MinIO + Docker Compose
**Researched:** 2026-03-14
**Confidence:** MEDIUM-HIGH (based on established patterns; external web research unavailable in this session — findings drawn from direct knowledge of all four domains as of August 2025)

---

## Critical Pitfalls

### Pitfall 1: Instagram Graph API App Review Blocks Phase 1b Launch

**What goes wrong:**
The Instagram Graph API requires Facebook App Review before you can publish content to real user accounts. Until the app passes review, posting only works with Developer-role test accounts. The review process for `instagram_content_publish` and `instagram_basic` permissions requires a demo video, privacy policy URL, and a functional walkthrough of the OAuth flow. Review can take 1–4 weeks and may be rejected, requiring iteration. Many projects discover this after building the entire scheduler and OAuth flow, only to find they cannot test against a real Instagram account without approval.

**Why it happens:**
Developers focus on the technical implementation (OAuth flow, token refresh, image upload endpoint) and assume Meta approval is a formality. The API surface area works perfectly in sandbox mode against test accounts, which creates false confidence.

**How to avoid:**
Submit the Facebook App for review at the start of Phase 1b, not the end. Create a minimal demo (even a screen recording of a Postman flow) early. Register a Facebook Business account and an Instagram Professional (Creator or Business) account immediately — personal accounts cannot use the Graph API at all. Plan for at least 3 weeks of review time in the Phase 1b schedule.

**Warning signs:**
- `publish_content` permission returns "permission not approved" for non-test accounts
- OAuth flow completes but `GET /me/accounts` returns empty array (personal account, not Business/Creator)
- Instagram account is not linked to a Facebook Page (required for API access)

**Phase to address:** Phase 1b (Social Media Scheduler) — begin app review submission during Phase 1a build, not after.

---

### Pitfall 2: Instagram Long-Lived Token Silently Expires

**What goes wrong:**
Instagram long-lived User Access Tokens expire after 60 days. If the token is not refreshed before expiry, all scheduled posts silently fail with `OAuthException` errors. The token refresh window opens at 50 days; refresh must be triggered before day 60. A self-hosted app running on a user's machine may go unused for weeks, and if the token expires during a period of inactivity, the user returns to a broken social connection with no notification.

**Why it happens:**
The OAuth token lifecycle is documented but easy to misread. Short-lived tokens (1–2 hours) must first be exchanged for long-lived tokens (60 days). The long-lived token refresh is a separate HTTP call that must be made proactively. Projects commonly store the initial token and never implement the refresh loop, or implement it but only trigger it during active scheduling activity — missing the case where the app is idle for >50 days.

**How to avoid:**
Implement token refresh as a scheduled job that runs on every scheduler tick (not just when publishing). On API startup, immediately check all stored tokens — if a token's `token_expiry` is within 15 days of now, attempt refresh and log the result. If token is already expired, mark the `social_accounts` record as `token_status: expired` and surface a notification in the UI. Store `token_expiry` and `last_refresh_attempt` on the `social_accounts` row.

**Warning signs:**
- Scheduled posts failing with `OAuthException error code 190`
- `token_expiry` column not populated or populated with wrong value
- No scheduler logic checking `token_expiry < NOW() + interval '15 days'`

**Phase to address:** Phase 1b — implement proactive token refresh as part of the scheduler, not as an afterthought in Phase 1b polish.

---

### Pitfall 3: Go Image Generation — Font Rendering Failures for Non-ASCII Track Names

**What goes wrong:**
The `gg` (fogleman/gg) library renders text using loaded font faces. If the font does not contain a glyph for a character (CJK, Arabic, Cyrillic, accented Latin), the character either renders as an empty box, a tofu square, or causes `gg` to silently skip the glyph — producing garbled or truncated text in the generated image. This is a critical failure for DJs playing non-English music, which is a large portion of the target audience (Afrobeats, Japanese city pop, Arabic music, etc.).

**Why it happens:**
Most Go `gg` examples use standard Latin-only fonts (Roboto, Open Sans Latin subset). The full Unicode-supporting versions (Noto Sans CJK, Noto Sans Arabic) are large and not included by default. Developers test with Latin track names and never encounter the issue during development.

**How to avoid:**
Bundle the Noto Sans family (NotoSans-Regular, NotoSansCJK, NotoSansArabic, NotoSansCyrillic) as embedded files using Go `embed` directives. Load all fonts at API startup into an in-memory `truetype.Font` slice. For each text render, iterate the font slice and use the first font containing the required glyph. Implement a fallback chain: primary display font → Noto Sans Latin → Noto Sans CJK → Noto Sans Arabic → replacement character (`?`). Test the image generator with a tracklist containing Japanese, Arabic, and Russian track names before any phase ships.

**Warning signs:**
- Generated images show boxes or missing characters for non-Latin text
- Test coverage only uses ASCII track names
- Font loading done lazily (per-request) rather than at startup — this causes 100ms+ overhead per image

**Phase to address:** Phase 1a — must be solved before first release. Add a Unicode test fixture (tracklist with CJK/Arabic/Cyrillic tracks) to the image generator test suite.

---

### Pitfall 4: Cover Art Fetch — Race Condition Between Concurrent Fetchers and MinIO Cache

**What goes wrong:**
Cover art fetching is concurrent (the NFR requires minimum 5 concurrent fetches for a 30-track list). If two tracks share the same artist+title (identical track played twice in a set, or tracks matching the same cache key), two concurrent goroutines will both trigger a full Spotify→Discogs→MusicBrainz lookup and attempt to write the same MinIO object key simultaneously. This is harmless for MinIO (last write wins), but creates redundant external API calls — doubling rate limit consumption. Under heavy concurrent use, this becomes a thundering herd on the Spotify API.

**Why it happens:**
The cache-check → fetch → cache-write sequence is not atomic. A goroutine checks MinIO for the key, finds it absent, begins the external fetch, while another goroutine doing the same thing for the same key has not yet written its result.

**How to avoid:**
Implement an in-process `singleflight` group (Go's `golang.org/x/sync/singleflight`) keyed on the normalized `artist:title` pair. With singleflight, duplicate in-flight requests for the same key are deduplicated — only one goroutine performs the actual fetch; all others wait and receive the same result. This eliminates the redundant API calls without requiring a distributed lock. The MinIO cache write still happens once, and all callers receive the same cover art path.

**Warning signs:**
- Spotify API returns 429 (Too Many Requests) during tracklist parsing of large sets
- Log shows multiple simultaneous fetches for the same `artist:title` pair
- Rate limit errors spike when parsing sets containing repeated tracks

**Phase to address:** Phase 1a — implement singleflight in the cover art fetcher from the start.

---

### Pitfall 5: DB-Backed Scheduler — Missed Ticks When Scheduler Is Down

**What goes wrong:**
The chosen architecture uses in-process goroutine scheduling with DB-backed job state (no external queue). If the API container is restarted or crashes at 14:59 when a post is scheduled for 15:00, the scheduler tick for that minute is missed. On restart at 15:02, the scheduler sees a post with `scheduled_at < NOW()` and `status = scheduled`. It must decide: publish it now (2 minutes late), skip it (silent failure), or mark it as missed. If the logic is "publish it now," a post scheduled for midnight while the user is asleep may silently go out hours late. If the logic is "skip it if older than N minutes," posts get lost during restarts.

**Why it happens:**
DB-backed schedulers require explicit "catch-up" logic for missed windows. Many implementations add the scheduler tick logic but forget the startup catch-up scan and the "how stale is too stale" decision.

**How to avoid:**
On API startup, run a catch-up scan: find all posts with `status = scheduled` and `scheduled_at < NOW()`. Posts within the last 10 minutes: publish immediately (minor latency). Posts older than 10 minutes: transition to `status = missed` and surface a notification in the UI. This 10-minute window accommodates normal restart/update cycles without silently publishing very stale posts. Document this behavior in the user-facing docs.

**Warning signs:**
- No catch-up logic in the API startup sequence
- Scheduler logic only runs on the goroutine tick, not on startup
- Posts stuck in `scheduled` status after an API restart

**Phase to address:** Phase 1b — scheduler catch-up must be part of the initial scheduler implementation, not a later fix.

---

### Pitfall 6: MinIO — Orphaned Objects Accumulate Without Cleanup

**What goes wrong:**
Every failed image generation attempt, every re-generation, and every draft that is never published stores objects in MinIO. Over months of use, MinIO accumulates orphaned objects — blobs with no corresponding database record — consuming significant disk space. With no cleanup mechanism, a power user who generates 50 image variants of each tracklist will exhaust disk space on their host machine.

**Why it happens:**
The write-MinIO-first pattern (NFR-106) means partial failures leave blobs. Re-generation always creates a new object rather than overwriting (due to unique keys). Developers focus on the happy path and defer cleanup logic.

**How to avoid:**
Implement the orphan reconciliation endpoint (`GET /api/v1/system/reconcile`) at the same time as the first MinIO write, not as a later addition. Run reconciliation at API startup (as specified in NFR-106). Add a configurable `STORAGE_RETENTION_DAYS` env var (default: 90) that marks generated images older than N days for cleanup. Include the `scripts/backup.sh` and `scripts/restore.sh` scripts in Phase 1a alongside the MinIO integration — users may need to back up before cleanup runs.

**Warning signs:**
- `generated_images` table has fewer rows than MinIO objects in the same bucket prefix
- No reconciliation logic in the startup sequence
- MinIO console shows bucket size growing unboundedly after many image generation cycles

**Phase to address:** Phase 1a (with MinIO integration) — reconciliation must ship with the first MinIO write, not later.

---

### Pitfall 7: Rekordbox / Serato / Traktor Format Changes Break Parsers Silently

**What goes wrong:**
DJ software vendors update their export formats with new software versions. Pioneer updated Rekordbox's history export format (column ordering, XML structure, encoding) multiple times between major versions. If a user upgrades their DJ software, the parser may silently produce wrong results — tracks parsed with wrong artist/title mapping, BPM appearing in the label field, etc. The parser succeeds (no error) but the data is corrupted.

**Why it happens:**
Parsers are often written against a specific file sample from one software version and never tested against other versions. Format documentation from vendors is sparse or absent. The parser uses column index rather than column name (for CSV-style formats), so column reordering silently breaks mappings.

**How to avoid:**
For CSV/TSV formats (Rekordbox history export): parse by column header name, not column index. Always validate that expected headers are present and log a warning if unexpected columns appear. For XML formats (Rekordbox XML library): use XPath/element name lookup, not position-based traversal. Maintain a test fixture directory with at least one sample file per supported software version (e.g., `testdata/rekordbox_6.7.txt`, `testdata/rekordbox_7.0.txt`). When a community reports a broken format, add a failing test first, then fix the parser.

**Warning signs:**
- Parser tests use only a single sample file per format
- CSV parsing uses `record[3]` instead of `record[headerIndex["BPM"]]`
- No test fixtures for multiple software versions

**Phase to address:** Phase 1a — build format-resilient parsers from the start; add multi-version test fixtures.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Hardcode AES key in code instead of env var | Simplifies early dev | Social tokens immediately exposed if repo is shared; impossible to rotate key | Never — always load from env var |
| Skip `singleflight` on cover art fetcher | Simpler initial code | Thundering herd on Spotify API, 429 errors during large tracklist parse | Never — implement from Phase 1a |
| Use `int` row IDs instead of UUIDs | Simpler queries | Breaks v3 multi-user extraction (predictable IDs are a security risk); FK conflicts on merge | Never — use UUID from day one |
| Store Instagram tokens unencrypted | Faster to implement | Token exposure in `pg_dump` or log output | Never — AES-256-GCM from Phase 1b day one |
| Run migrations manually instead of auto-migrate on startup | Gives control over migration timing | Self-hosters won't know to run migrations after update; broken state on restart | Never — auto-migrate on API startup |
| Skip soft delete, use hard delete | Simpler queries | Referential integrity broken across modules when linked entities (gig→tracklist) are deleted; no recovery | Never — soft delete from day one |
| Embed all fonts as `[]byte` in code | Simple to distribute | Binary size bloat >50MB; long compile times | Never — use Go `embed` with font files in `/assets/fonts/` |
| Poll external APIs without caching | No cache logic needed | Rate limits hit immediately; Spotify free tier is ~100 req/s, exhausted by a 100-track parse | Never — MinIO cache must ship with cover art fetcher |
| Skip `updated_at` optimistic concurrency | Simpler PUT handlers | Race condition when user has two browser tabs open; silent data loss on concurrent edit | Acceptable until Phase 1d (multiple state-heavy modules exist) |

---

## Integration Gotchas

Common mistakes when connecting to external services.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Spotify Web API | Using Client Credentials flow for cover art search — this only returns public catalog data, which is fine; but developers add unnecessary user-auth complexity | Use Client Credentials (`/token` with `grant_type=client_credentials`) — no user OAuth needed for search/cover art |
| Spotify Web API | Not handling 429 with `Retry-After` header — retrying immediately after rate limit hit | Read the `Retry-After` header (in seconds); sleep exactly that duration before retry |
| Discogs API | Not including a `User-Agent` header — Discogs blocks requests with no user agent | Always set `User-Agent: KlubHubDJ/1.0 (+https://github.com/your-repo)` per Discogs API requirements |
| Discogs API | Not using authenticated requests — unauthenticated Discogs requests are rate-limited to 25/minute | Use OAuth 1.0a or API key header for 60 req/min authenticated limit |
| MusicBrainz API | Hitting the API without rate limiting — MusicBrainz requires max 1 req/sec per IP | Implement a MusicBrainz-specific rate limiter (`time.Ticker` at 800ms interval) separate from Spotify limiter |
| MusicBrainz API | Searching by raw title with special characters unescaped — Lucene syntax in search query causes 400 errors | URL-encode and escape Lucene special characters (`+`, `-`, `"`, `(`, `)`) in search terms |
| Instagram Graph API | Publishing image by posting raw binary — Graph API requires the image to be at a public URL | Upload image to MinIO, generate a time-limited presigned URL, pass that URL to the `media` creation endpoint |
| Instagram Graph API | Creating media container and immediately publishing — there's a processing delay | After `POST /media` (container creation), poll `GET /media/{id}?fields=status_code` until `status_code = FINISHED` before calling `POST /media/publish` |
| Instagram Graph API | Not handling `EXPIRED` token before scheduled publish runs | Check token validity before every publish attempt; surface UI warning if token will expire within 7 days |
| MinIO S3 client | Using path-style URLs (`http://minio:9000/bucket/key`) in Go SDK without explicit path-style config | Set `s3.New(sess, &aws.Config{S3ForcePathStyle: aws.Bool(true)})` or equivalent in the AWS SDK v2 / MinIO Go SDK |
| MinIO presigned URLs | Generating presigned URLs with internal Docker hostname (`minio:9000`) and serving them to the browser | Presigned URLs for browser use must use the external/public hostname — use a separate `MINIO_PUBLIC_ENDPOINT` env var for browser-facing URLs |

---

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Sequential cover art fetching (no concurrency) | 30-track parse takes 150+ seconds (5s/track × 30 tracks) | Use goroutine pool (min 5 concurrent fetchers per NFR-203) with `errgroup` | Immediately — even a 10-track set takes 50+ seconds |
| Loading full tracklist with all tracks into memory for image generation | OOM on 500+ track sets | Stream tracks from DB in batches; render only the `max_tracks_displayed` slice needed for the image | Sets >500 tracks on machines with 8GB RAM |
| No DB index on `scheduled_posts(status, scheduled_at)` | Scheduler tick query does a full table scan | Add composite index on `(status, scheduled_at)` at migration time | After ~1000 scheduled posts |
| Generating full-resolution images for live preview | Preview takes 10–15 seconds, breaking the interactive experience | Render preview at 50% resolution (per NFR-205); use a separate `/preview` endpoint | Immediately — even 3-second previews feel broken |
| Storing all cover art fetches in the MinIO path without normalizing the cache key | Same track with different capitalizations (`"The Chemical Brothers"` vs `"the chemical brothers"`) results in duplicate fetches | Normalize cache keys: lowercase, strip leading `the`, strip punctuation before constructing MinIO key | After ~100 fetches of the same catalog |
| Dashboard aggregation query without query-level timeout | Dashboard hangs if any module table has a bad query plan | Set `statement_timeout = 500ms` on the dashboard connection context; return partial data if timeout hit | After the Finance module ships with many entries |
| PDF generation blocking the HTTP handler goroutine | API becomes unresponsive during EPK export (up to 15 seconds per NFR-204) | Run PDF generation in a goroutine; return a `202 Accepted` with a job ID; poll for completion | From first user with full EPK (20 photos) |

---

## Security Mistakes

Domain-specific security issues beyond general web security.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Serving MinIO presigned URLs with no expiry time | Generated images publicly accessible forever via guessable (but not secret) URLs | Set presigned URL expiry to 1 hour for temporary shares; for download endpoints, generate presigned URL on each request |
| Binding all Docker services to `0.0.0.0` by default | Anyone on the same network (LAN, coffee shop WiFi) can access the dashboard and MinIO API | Default to `127.0.0.1` binding per A1.1; document `BIND_ADDRESS` override with explicit security warning |
| Using the same AES-256-GCM key for both token encryption and other purposes | Key rotation for a security incident requires migrating all encrypted fields simultaneously | Use a dedicated `TOKEN_ENCRYPTION_KEY` for social account tokens only; other encrypted fields use separate keys |
| Logging `access_token` values in structured log output | Tokens in plaintext in log files; exfiltrated via log shipping | Redact all fields named `access_token`, `token`, `secret`, `key` in the zerolog middleware |
| No file type validation beyond MIME type header | Malicious file with `.txt` extension and embedded payload uploaded as DJ history | Validate file content (check actual byte signatures), not just `Content-Type` header; limit parser input to confirmed text/XML/NML content |
| MinIO bucket publicly readable | All generated images (tracklist art, EPK photos) accessible without any authentication | Create MinIO buckets with private ACL; all access goes through the Go API which generates presigned URLs on demand |
| Storing `TOKEN_ENCRYPTION_KEY` in `docker-compose.yml` rather than `.env` | Key committed to version control in public repos | `TOKEN_ENCRYPTION_KEY` must only be in `.env` (gitignored); `.env.example` has a placeholder, not a real key |

---

## UX Pitfalls

Common user experience mistakes in this domain.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Showing a spinner during cover art fetch with no progress indication | User cannot tell if 100-track parse will take 10 seconds or 10 minutes; may close the tab | Show per-track progress: "Fetching artwork: 14 / 30 tracks complete" with a progress bar |
| Blocking image export until all cover art is fetched | User cannot export if one track's cover art is slow or unavailable | Allow export with placeholder art; show per-track warning indicator on tracks without resolved art |
| Requiring users to understand Instagram's Business account + Facebook Page requirement before Phase 1b works | Users try to connect personal Instagram accounts, the OAuth succeeds but posting fails silently | Show a pre-connection checklist: "Before connecting, ensure your Instagram is a Professional account linked to a Facebook Page" with a screenshot guide |
| Silently using the wrong DJ software parser when format auto-detection is uncertain | Parsed tracklist has wrong data; user doesn't know until they review all tracks | When format confidence is below a threshold, ask the user: "We detected this may be a Serato file. Confirm format?" |
| No "delete generated images" workflow | MinIO fills with old generated images; users have no way to reclaim space | Add a "Generated Images" management page with bulk delete; show per-tracklist storage usage |
| Image export offering only one resolution | Story (1080×1920) export downloaded by a user who wanted Square (1080×1080), requiring re-generation | Show both export buttons simultaneously; remember last-used format preference in `user_settings` |
| Showing raw API error messages on social post failure | Instagram API errors like `OAuthException: Invalid OAuth access token` are meaningless to DJs | Map known error codes to human-readable messages: `OAuthException 190` → "Your Instagram connection has expired. Please reconnect your account." |

---

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **Cover art fetcher:** Cache appears to work in dev but MinIO bucket ACL is set to public-read, exposing all artwork — verify bucket is private and access uses presigned URLs
- [ ] **Instagram OAuth flow:** OAuth dance completes and token is stored, but token is stored unencrypted — verify AES-256-GCM encryption is applied before the DB write
- [ ] **Image generator:** Latin text renders correctly in preview — verify CJK/Arabic/Cyrillic track names also render correctly by running the Unicode test fixture
- [ ] **Scheduler:** Posts publish correctly from fresh state — verify catch-up logic handles posts that were scheduled while the API was down
- [ ] **Docker Compose:** `docker compose up` works on dev machine — verify it works with a clean Docker state (no pre-existing named volumes) on a fresh machine running Linux
- [ ] **MinIO presigned URLs:** Images load in browser during local dev — verify they load in browser when MinIO is accessed from a different host (the internal `minio:9000` hostname won't resolve in the browser)
- [ ] **DB migrations:** Migrations run on startup in dev — verify that migrations are idempotent (can be run twice without error) and that a failed migration halts API startup with a non-zero exit code
- [ ] **Soft delete:** Records are soft-deleted (hidden from queries) — verify that all list queries include `WHERE deleted_at IS NULL` and that foreign key queries also filter soft-deleted parents
- [ ] **Instagram posting:** Single image posts work — verify that Story posts (1080×1920) are correctly identified as `REELS`-type or `STORIES`-type in the Graph API container creation call, not defaulting to feed post
- [ ] **gofpdf EPK export:** PDF generates with logo and bio — verify that press photos embedded in the PDF are correctly scaled and do not cause OOM on a 20-photo EPK with 10MB photos each

---

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Instagram token expired, all scheduled posts failed | LOW | Re-authorize Instagram in the social accounts UI; re-schedule permanently_failed posts via the retry endpoint |
| Instagram App Review rejected | MEDIUM | Review rejection feedback; update privacy policy, re-record demo video; re-submit (typically 1–2 week cycle) |
| MinIO orphan objects consuming disk | LOW | Run `GET /api/v1/system/reconcile` with `?action=delete`; run backup before cleanup; restart API to trigger auto-reconciliation |
| Parser broken by DJ software update | MEDIUM | Add new test fixture file; update parser to handle both old and new format (version detection by header); release patch |
| Font rendering produces blank text for a script | LOW | Identify missing font file; add Noto Sans variant for that script; rebuild API container |
| Generated images lost (MinIO volume deleted) | HIGH | Restore from `scripts/restore.sh` with latest backup archive; re-generate any images not in backup (tracklist metadata preserved in PostgreSQL) |
| AES encryption key lost (social tokens invalid) | MEDIUM | Re-authorize all social accounts (tokens must be re-fetched); generate a new `TOKEN_ENCRYPTION_KEY` and rotate — document this as a known recovery scenario |
| DB migration fails on update | MEDIUM | Check migration error log; fix migration file; the API fails to start (by design per NFR-703), so no data corruption; rollback to previous image if migration is non-reversible |

---

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Instagram App Review delay | Phase 1b start (submit review immediately) | App has `instagram_content_publish` permission approved before scheduler is coded |
| Instagram token expiry | Phase 1b | Unit test: token with `expiry = NOW() - 1 day` triggers `token_status = expired`; scheduler test: expired token causes skip + notification, not silent failure |
| Font Unicode rendering failure | Phase 1a | Integration test: generate image with fixture containing CJK + Arabic + Cyrillic track names; assert no blank characters |
| Cover art cache race (singleflight) | Phase 1a | Test: 10 concurrent requests for same artist+title produce exactly 1 external API call |
| Scheduler missed-tick catch-up | Phase 1b | Integration test: insert `scheduled` post with `scheduled_at = 5 minutes ago`, restart API, assert post is published within first tick |
| MinIO orphan objects | Phase 1a (with MinIO) | Test: simulate failed image gen (write MinIO, then mock DB failure); run reconciliation; assert orphan object is deleted |
| DJ format parser silent regression | Phase 1a | Per-format test fixtures for at least 2 software versions; CI gate fails if any fixture produces mismatched track count or wrong artist/title mapping |
| MinIO presigned URL internal hostname | Phase 1a | Test: generate presigned URL; assert hostname matches `MINIO_PUBLIC_ENDPOINT`, not `minio:9000` |
| Missing `WHERE deleted_at IS NULL` | All phases | Code review gate: every repository query returning a list must include deleted_at filter; add linting rule if possible |
| Instagram media container processing delay | Phase 1b | Integration test against test account: measure time between container creation and `status_code = FINISHED`; assert publisher polls before publish call |

---

## Sources

- Instagram Graph API documentation (Meta for Developers) — token lifecycle, app review requirements, media publishing flow, container status polling
- Spotify Web API documentation — Client Credentials flow, rate limit headers, search endpoint behavior
- Discogs API documentation — User-Agent requirements, authentication, rate limit tiers
- MusicBrainz API documentation — rate limiting policy (1 req/sec), Lucene query syntax
- Go standard library: `golang.org/x/sync/singleflight` — deduplication pattern for concurrent fetches
- MinIO Go SDK documentation — path-style URL configuration, presigned URL generation, bucket ACL
- fogleman/gg library — font loading, text rendering, glyph fallback behavior
- Docker Compose v2 documentation — `depends_on` with `condition: service_healthy`, named volumes, network binding
- Known Instagram Graph API gotchas: container processing state machine (`FINISHED`/`IN_PROGRESS`/`ERROR`), Story vs Feed container type distinction
- Go `embed` package — embedding fonts and static assets in binary

*All sources accessed from training knowledge (cutoff August 2025). External web verification unavailable in this research session — findings assessed as MEDIUM-HIGH confidence based on stable, well-documented platform behaviors.*

---
*Pitfalls research for: KlubHub DJ — self-hosted DJ career toolkit*
*Researched: 2026-03-14*
