# Phase 2: Social Media Scheduler - Research

**Researched:** 2026-03-21
**Domain:** Instagram Graph API, OAuth 2.0, Go background workers, Vue/Pinia scheduling UI
**Confidence:** HIGH (core stack verified against official Meta docs and existing codebase)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Default view: Queue** — card grid of posts (matches the Social Scheduler mockup)
- **Calendar tab also built in Phase 2**: monthly grid with dot indicators per day (colored by status: cyan=scheduled, gray=published, magenta=failed)
- Clicking a calendar day filters the queue to show only posts for that date
- **Tabs in scope: QUEUE + CALENDAR only** — Analytics and Automations tabs rendered as "Coming Soon" stubs (no backend, just placeholder UI)
- **Empty queue empty state**: dashed "NEXT SLOT" card + a connection banner above if no Instagram account is connected
- Post cards in queue: image thumbnail, status badge (READY/SCHEDULED/PUBLISHED/FAILED), datetime, caption excerpt, platform tag (INSTAGRAM / INSTAGRAM REELS)
- **Flow: Navigate to Social page** — clicking "Schedule to Instagram" on the tracklist page navigates to `/social?imageId={id}`
- The Social page reads the `imageId` query param, fetches the image from MinIO, auto-expands the new-post form, and pre-fills the image
- **Form visibility**: collapsed by default ("+ ADD TO QUEUE" button); clicking it expands a panel inline above the queue grid. `⚡ QUICK EXPORT FROM TRACKLIST` button also triggers this flow from within Social
- **Caption**: auto-generates a suggested caption from tracklist metadata (template: `[DJ Name] @ [set name] — [track count] tracks • avg [BPM] BPM #techno #djset`). User can freely edit.
- **Character counter**: adaptive by post type (Feed: `482 / 2200`, turns red at 90%+; Story: dimmed + note "Captions are not shown on Stories")
- **Timezone**: datetime-local picker + searchable timezone dropdown (IANA names). Stored as UTC internally.
- **Post type selector**: Feed vs Story
- **Inline card expansion**: clicking a scheduled post card expands it in-place. Edit only available on `scheduled` status.
- **Magenta banner at top of card**: `⚠ ATTENTION REQUIRED: SYNC ERROR` for failed posts
- **Two actions on failed card**: `RETRY SYNC` + `DOWNLOAD IMAGE`
- Failed state appears after 3 automatic retries with exponential backoff exhausted
- **No Instagram connected**: connection banner visible with `CONNECT INSTAGRAM` CTA
- **Connected**: small green status dot + account handle in top-right of Social module header
- **Token expiry warning**: proactive orange banner `⚠ INSTAGRAM TOKEN EXPIRING IN 7 DAYS — RE-AUTHORIZE`
- **Token expired mid-session**: queue freezes for new scheduling, banner changes to magenta

### Claude's Discretion
- Exact Pinia store structure for social posts (`useSocialStore`)
- Database schema for `scheduled_posts` table (UUIDs, status enum, retry_count, scheduled_at_utc, timezone_name)
- Go polling worker implementation (time.Ticker interval — 1 min recommended)
- Exponential backoff formula (start: 5 min, max: 4 hr)
- MinIO path convention for social post images
- Instagram Graph API endpoint selection (Feed: `/me/media` + `/me/media_publish`, Stories: same with `media_type=STORIES`)
- Exact timezone dropdown component (searchable select from IANA list, filter on type)

### Deferred Ideas (OUT OF SCOPE)
- Analytics tab — post performance metrics (reach, impressions, engagement)
- Automations tab — auto-scheduling rules
- Multi-platform support (Twitter/X, TikTok, Facebook) — architecture should not block it but Phase 2 is Instagram-only
- Best time to post suggestions
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| SOCL-01 | User can connect an Instagram account via OAuth (Facebook Business account flow) | Instagram Platform API OAuth flow documented; requires `instagram_business_basic` + `instagram_business_content_publish` scopes |
| SOCL-02 | System automatically refreshes Instagram OAuth tokens before expiry; marks account as `disconnected` on refresh failure | Token refresh via `GET /refresh_access_token`; 60-day expiry; polling worker checks `token_expiry` column 7 days ahead |
| SOCL-03 | User can schedule a post for a specific date and time with timezone selection | Stored as UTC; IANA timezone dropdown via `Intl.supportedValuesOf('timeZone')`; datetime-local picker |
| SOCL-04 | System publishes scheduled posts automatically at the specified time | Go time.Ticker polling worker; `scheduled_at_utc <= NOW()` query with status=scheduled |
| SOCL-05 | User can post to Instagram Feed (single image + caption with hashtags) | `POST /<IG_ID>/media` with `media_type=IMAGE` then `POST /<IG_ID>/media_publish` |
| SOCL-06 | User can post to Instagram Stories (single image) | `POST /<IG_ID>/media` with `media_type=STORIES` then `POST /<IG_ID>/media_publish` |
| SOCL-07 | User can attach any generated tracklist image or a custom uploaded image | MinIO path stored on post; presigned GET URL generated at publish time |
| SOCL-08 | Custom-uploaded images validated for format (JPEG/PNG), size (<8 MB), dimensions, and aspect ratio | Instagram requires JPEG; PNG must be converted; 8 MB max; aspect ratio 4:5 to 1.91:1 for feed |
| SOCL-09 | User sees a queue/calendar view of all scheduled, publishing, published, failed, and permanently failed posts | Queue + Calendar tabs; status-colored UI; poll `/api/v1/social/posts` |
| SOCL-10 | Post status tracks: draft, scheduled, publishing, published, failed, permanently_failed | PostgreSQL enum column with CHECK constraint |
| SOCL-11 | Failed posts retry up to 3 times with exponential backoff; after exhaustion, mark `permanently_failed` | retry_count column; backoff: 5 min, 20 min, 80 min; worker checks `next_retry_at` |
| SOCL-12 | Permanently failed posts surface with error reason; user can retry once manually or download image | `RETRY SYNC` + `DOWNLOAD IMAGE` actions on failed card |
| SOCL-13 | Scheduler respects Instagram rate limits: minimum 30-second interval between posts; backs off on 429 | Worker enforces 30s minimum; reads Retry-After header on 429; defaults to 15-min backoff |
| SOCL-14 | Scheduled posts can be fully edited while in `scheduled` status; edits blocked once `publishing` begins | Edit blocked on publishing, published, failed, permanently_failed |
| SOCL-15 | Disconnecting account moves all its `scheduled` posts to `draft` status; user warned | Cascade status update on disconnect |
| SOCL-16 | Before retrying, scheduler checks whether post was already published (duplicate prevention) | `GET /<container_id>?fields=status_code` before retry |
| SOCL-17 | After image export, UI shows "Schedule Post" CTA that pre-fills the new post form | Navigate to `/social?imageId={minioId}`; Social page reads param and pre-fills compose panel |
</phase_requirements>

---

## Summary

Phase 2 builds a complete Instagram social scheduler on top of the existing Go/Nuxt/PostgreSQL/MinIO stack. The three main technical layers are: (1) Instagram OAuth integration with token lifecycle management, (2) a Go background worker that polls the `scheduled_posts` table every 60 seconds and publishes via the Instagram Graph API, and (3) a Nuxt social page with Queue and Calendar views following the established Cyberpunk HUD design system.

The Instagram Graph API uses a two-step publish flow: create a media container (`POST /<IG_ID>/media`) then publish it (`POST /<IG_ID>/media_publish`). Images must be served from a publicly accessible URL — MinIO presigned GET URLs using `MINIO_PUBLIC_ENDPOINT` with a 30-minute expiry satisfy this requirement. Critically, the API only accepts JPEG; PNG images from the tracklist generator must be converted before submission. Do NOT create the media container at scheduling time — Instagram containers expire after 24 hours, so the container must be created at publish time (in the worker).

The `social_accounts` table stub already exists in migration 001. This phase adds migration 003 for `scheduled_posts`, the `social` Go package following the handler/service/repository/worker structure of the existing `tracklist` package, and `pages/social.vue` with feature components under `components/social/`.

**Primary recommendation:** Use the existing `platform/crypto` AES-256-GCM package for token encryption (already implemented in `api/internal/platform/crypto/aes.go`), the existing time.Ticker + goroutine pattern for the publishing worker, and MinIO presigned GET URLs with a 30-minute expiry window for Instagram's `image_url` parameter.

---

## Standard Stack

### Core
| Library / Feature | Version | Purpose | Why Standard |
|-------------------|---------|---------|--------------|
| Instagram Graph API | v22.0 (2025) | Publishing feed posts and stories | Only supported path for Business/Creator accounts after Dec 2024 deprecation of Basic Display API |
| `github.com/go-chi/chi/v5` | v5.2.5 (existing) | HTTP routing for social endpoints | Already in go.mod; established project router |
| `github.com/jackc/pgx/v5` | v5.8.0 (existing) | PostgreSQL queries for social_accounts + scheduled_posts | Already in go.mod; all other packages use it |
| `github.com/minio/minio-go/v7` | v7.0.99 (existing) | Object storage for post images; presigned GET URLs | Already in go.mod; `PresignedGetObject` interface already defined |
| `api/internal/platform/crypto` | existing (internal) | AES-256-GCM encryption for OAuth access tokens | Already implemented; satisfies INFRA-09 |
| `time.Ticker` (Go stdlib) | Go 1.26 | Background polling worker at 60s intervals | Standard lib; no new dependency; matches artwork worker pattern |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/sethvargo/go-retry` | v0.3.0 (existing in go.mod) | Retry with backoff for Instagram API calls | Use for the Instagram HTTP client retry wrapper |
| `github.com/gabriel-vasile/mimetype` | existing in go.mod | MIME type detection for uploaded images | Use when validating custom-uploaded images (SOCL-08) |
| radix-vue Tabs (shadcn-vue) | existing | Queue/Calendar tab switching on social page | `ui/tabs/` components already present |
| Pinia composition store | existing | `useSocialStore` state management | All stores use setup-function style |
| `$fetch` Nuxt built-in | existing | Social API calls from frontend | Project standard; no Axios |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| time.Ticker + goroutine worker | `riverqueue/river` (Postgres-backed job queue) | River is more robust for high volume; overkill for a single-user app with <100 posts/day; time.Ticker keeps zero new dependencies |
| MinIO presigned GET URL as image_url | Separate public CDN | Presigned URLs work as long as expiry > Instagram processing time (~5 min); 30-min window is safe and uses existing infrastructure |
| Custom searchable timezone combobox | `vue-timezone-select` npm package | Building from radix-vue ComboBox primitive avoids a new dep and matches design system exactly |
| `golang.org/x/oauth2` | Manual HTTP OAuth flow | For this app the OAuth flow is simple (3 HTTP calls); manual implementation using `net/http` is more transparent and avoids the x/oauth2 token storage abstraction complexity |

**Installation:** No new Go dependencies required. No new npm packages required for frontend.

---

## Architecture Patterns

### Recommended Project Structure
```
api/internal/social/
├── handler.go          # chi routes: /accounts, /posts, /posts/{id}/retry
├── model.go            # SocialAccount, ScheduledPost, PostStatus enum
├── repository.go       # DB queries: CRUD on social_accounts + scheduled_posts
├── repository_test.go  # testcontainers-go integration tests (mirrors tracklist pattern)
├── service.go          # business logic: schedule, edit, disconnect, retry
├── worker.go           # time.Ticker goroutine; publish loop; token refresh check
└── instagram.go        # Instagram Graph API HTTP client: createContainer, publish, checkStatus

api/internal/platform/migrations/
└── 003_social.sql      # scheduled_posts table + social_accounts additions

apps/dj/app/
├── pages/social.vue                  # thin layout page; reads ?imageId query param
├── stores/social.ts                  # useSocialStore: posts[], account, composePanelOpen
├── components/social/
│   ├── SocialPageHeader.vue          # "SOCIAL SCHEDULER" + account status dot + account handle
│   ├── SocialTabs.vue                # QUEUE / CALENDAR tabs + stub triggers for ANALYTICS / AUTOMATIONS
│   ├── SocialConnectionBanner.vue    # "CONNECT INSTAGRAM" CTA when disconnected
│   ├── SocialTokenWarningBanner.vue  # Orange 7-day warning; magenta disconnected banner
│   ├── SocialPostCompose.vue         # Collapsible compose panel (caption, type, timezone, datetime)
│   ├── SocialQueueGrid.vue           # 3-column grid of SocialPostCard
│   ├── SocialPostCard.vue            # Post card with inline expand-to-edit on click
│   ├── SocialPostCardFailed.vue      # Failed card with RETRY SYNC + DOWNLOAD IMAGE
│   ├── SocialCalendarView.vue        # Monthly grid, dot indicators, day-click filters queue
│   └── SocialNextSlotCard.vue        # Dashed empty card for empty queue
└── composables/
    └── useSocialPostForm.ts          # Caption auto-generation; character counter logic; timezone helpers
```

### Pattern 1: Go Publishing Worker (time.Ticker)
**What:** A goroutine started from `main.go` that polls `scheduled_posts WHERE status='scheduled' AND scheduled_at_utc <= NOW()` every 60 seconds.
**When to use:** Single-user app; polling is simpler than a full job queue; matches the existing artwork worker pattern.
**Example:**
```go
// Source: Go stdlib time package
func StartPublishWorker(ctx context.Context, svc *Service, log zerolog.Logger) {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            if err := svc.PublishDuePosts(ctx); err != nil {
                log.Error().Err(err).Msg("publish worker error")
            }
            if err := svc.RefreshExpiringTokens(ctx); err != nil {
                log.Error().Err(err).Msg("token refresh worker error")
            }
        case <-ctx.Done():
            return
        }
    }
}
```

### Pattern 2: Instagram Two-Step Publish (CRITICAL)
**What:** Create a media container then publish it. Two separate HTTP calls.
**When to use:** Every feed post and story. Container must be created at publish time, NOT at scheduling time (containers expire after 24 hours).
**Example:**
```go
// Source: https://developers.facebook.com/docs/instagram-platform/content-publishing/

// Step 1: Create container
// POST https://graph.instagram.com/v22.0/{ig_user_id}/media
// Params: image_url (public URL), media_type (IMAGE|STORIES), caption, access_token

// Step 2: Publish container
// POST https://graph.instagram.com/v22.0/{ig_user_id}/media_publish
// Params: creation_id (container ID from step 1), access_token
```

### Pattern 3: Token Refresh (Proactive)
**What:** During each worker tick, check `token_expiry - 7 days <= NOW()` and call the refresh endpoint for connected accounts.
**When to use:** On every 60-second worker tick alongside post publishing.
**Refresh endpoint:**
```
GET https://graph.instagram.com/refresh_access_token
  ?grant_type=ig_refresh_token
  &access_token={current_long_lived_token}
```
Token must be valid (not expired) and at least 24 hours old. Returns a new 60-day token. If refresh fails, mark account `status='disconnected'`.

### Pattern 4: Exponential Backoff on Publish Failure
**What:** After each failed publish, set `next_retry_at = NOW() + backoff`. Worker skips posts where `next_retry_at > NOW()`.
**Backoff schedule (Claude's Discretion — recommended values):**
| retry_count | Delay before next attempt |
|-------------|--------------------------|
| 0 (first try) | immediate |
| 1 (1st retry) | 5 minutes |
| 2 (2nd retry) | 20 minutes |
| 3 (3rd retry) | 80 minutes |
| exhausted | mark `permanently_failed`; set `error_reason` |

### Pattern 5: Duplicate Prevention via Container Status Check (SOCL-16)
**What:** Before retrying a failed post, call `GET /<container_id>?fields=status_code` to see if Instagram already published it.
**status_code values:** `IN_PROGRESS`, `FINISHED`, `PUBLISHED`, `ERROR`, `EXPIRED`
If `status_code=PUBLISHED`, mark post `published` locally without re-publishing.

### Pattern 6: useSocialStore (Pinia Composition API)
**What:** Composition-API Pinia store following project conventions (`apps/dj/app/stores/settings.ts`).
**Example:**
```typescript
// Source: project convention from apps/dj/app/stores/settings.ts
export const useSocialStore = defineStore('social', () => {
  const posts = ref<ScheduledPost[]>([])
  const account = ref<SocialAccount | null>(null)
  const loading = ref(false)
  const composePanelOpen = ref(false)
  const prefilledImageId = ref<string | null>(null)

  async function loadPosts(): Promise<void> {
    // $fetch('/api/v1/social/posts')
  }
  async function createPost(req: CreatePostRequest): Promise<void> { ... }
  async function retryPost(id: string): Promise<void> { ... }
  async function downloadImage(id: string): Promise<void> { ... }

  return { posts, account, loading, composePanelOpen, prefilledImageId, loadPosts, createPost, retryPost, downloadImage }
})
```

### Anti-Patterns to Avoid
- **Creating the Instagram media container at scheduling time**: Containers expire after 24 hours. Always create at publish time (inside the worker).
- **Polling `/api/v1/social/posts` from the browser on an interval**: Use manual/user-triggered refresh. Background publish status is rare; browser auto-polling adds complexity without meaningful UX benefit.
- **Storing decrypted access tokens in memory across requests**: Decrypt from DB on demand per operation, discard immediately.
- **Using the same `scheduled_at_utc` column to track retries**: Keep `scheduled_at_utc` immutable (user intent) and use a separate `next_retry_at` column (worker bookkeeping).
- **Sending PNG directly to Instagram**: Convert PNG to JPEG before storing the social-specific copy; Instagram rejects non-JPEG with a non-obvious error.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| AES-256-GCM token encryption | Custom crypto | `api/internal/platform/crypto` (existing) | Already tested, handles nonce generation and base64 encoding |
| HTTP retry with backoff | Custom retry loop | `github.com/sethvargo/go-retry` v0.3.0 (already in go.mod) | Already in go.mod; handles jitter, max attempts, context cancellation |
| OAuth state/CSRF protection | Custom state tracking | `github.com/google/uuid` (existing) + DB record | State = `uuid.NewString()`; store in short-lived DB row; verify on callback |
| IANA timezone list | Hardcoded slice | `Intl.supportedValuesOf('timeZone')` (Web API) | Browser-native; zero npm dependency; 400+ IANA zones; filter on input for combobox |
| Image MIME type detection | `strings.HasSuffix` | `github.com/gabriel-vasile/mimetype` (existing in go.mod) | Already in go.mod; used by tracklist upload; detects real content-type regardless of filename |

**Key insight:** Every Go package needed for social scheduling is already in `go.mod`. No new `go get` commands required.

---

## Common Pitfalls

### Pitfall 1: Instagram Requires Publicly Accessible image_url
**What goes wrong:** Posting with a private MinIO URL (e.g. `http://minio:9000/...`) or a localhost URL fails with `(#100) image_url is not a valid URL` or `(#2207026) image could not be retrieved`.
**Why it happens:** Instagram's servers fetch the image during container creation. Internal Docker network addresses are unreachable by Meta's servers.
**How to avoid:** Generate a presigned GET URL using `MINIO_PUBLIC_ENDPOINT` (the externally accessible MinIO address) with a 30-minute expiry before calling the Instagram API. The `PresignedGetObject` method on the storage interface is already defined in `api/internal/tracklist/service.go`.
**Warning signs:** Container creation returns HTTP 400 with error code `#2207026`.

### Pitfall 2: Instagram Only Accepts JPEG
**What goes wrong:** Posting a PNG (default Playwright screenshot output) fails silently or with an unhelpful media format error.
**Why it happens:** The Instagram Graph API rejects non-JPEG images.
**How to avoid:** When a social post image is attached (either from tracklist export or custom upload), convert PNG to JPEG using Go's `image/jpeg` standard library and store the JPEG version under `social/posts/{post_id}/image.jpg` in MinIO. Do not modify the original tracklist export file.
**Warning signs:** Container creation returns error code `#2207026` with no other indication.

### Pitfall 3: Instagram Container Expires After 24 Hours
**What goes wrong:** If the media container is created at scheduling time and the post is scheduled more than 24 hours out, publish fails with `EXPIRED` status_code.
**Why it happens:** Instagram containers have a 24-hour TTL.
**How to avoid:** Never create the container at scheduling time. Create it in the worker, immediately before calling `media_publish`. Store only the MinIO path and caption in `scheduled_posts`.

### Pitfall 4: Token Refresh Window
**What goes wrong:** Token refresh fails because it was attempted on an already-expired token. Expired tokens cannot be refreshed via the API.
**Why it happens:** If the worker only checks token expiry when publishing, an inactive account's token may expire unnoticed between refreshes.
**How to avoid:** On each 60-second worker tick, also check `token_expiry - 7 days <= NOW()` for all connected accounts and refresh proactively. If refresh fails, mark account `status='disconnected'` immediately. Surface orange warning banner at 7 days, magenta at expiry.

### Pitfall 5: Duplicate Post on Retry (SOCL-16)
**What goes wrong:** A publish call timed out after Instagram had already processed it, causing a retry to create a duplicate post.
**Why it happens:** Network timeouts don't mean the operation failed; Instagram may have published successfully despite the Go HTTP client receiving no response.
**How to avoid:** Before retrying any failed post that has a `ig_container_id`, call `GET /<container_id>?fields=status_code`. If `status_code=PUBLISHED`, mark the post `published` locally without re-calling `media_publish`.

### Pitfall 6: Timezone Arithmetic Errors
**What goes wrong:** Posts publish at the wrong time (often off by 1 hour during DST transitions).
**Why it happens:** Performing arithmetic on local timestamps instead of UTC, or not storing the IANA timezone name alongside the UTC time.
**How to avoid:** Store `scheduled_at_utc TIMESTAMPTZ` and `timezone_name TEXT` (e.g., `Europe/Berlin`) as separate columns. Convert to UTC at form submission on the frontend using `luxon` or Go's `time.LoadLocation`. Display in local time using `Intl.DateTimeFormat`. The DB comparison in the worker uses only UTC: `scheduled_at_utc <= NOW()`.

### Pitfall 7: OAuth State Parameter Not Validated
**What goes wrong:** CSRF attack allows an attacker to associate their Instagram account with the victim's session.
**Why it happens:** State parameter skipped or not verified on callback.
**How to avoid:** Generate `state = uuid.NewString()`, store in a short-lived DB row (e.g. `oauth_states` table with `expires_at = NOW() + 10 minutes`), verify on callback. Reject mismatched or expired state.

---

## Code Examples

Verified patterns from official sources:

### OAuth Authorization URL Construction
```go
// Source: https://developers.facebook.com/docs/instagram-platform/reference/oauth-authorize/
// Required scopes for feed + stories posting:
authURL := fmt.Sprintf(
    "https://api.instagram.com/oauth/authorize?client_id=%s&redirect_uri=%s&scope=%s&state=%s&response_type=code",
    cfg.InstagramClientID,
    url.QueryEscape(redirectURI),
    "instagram_business_basic,instagram_business_content_publish",
    state,
)
```

### Token Exchange (code → short-lived → long-lived)
```go
// Source: https://gist.github.com/PrenSJ2/0213e60e834e66b7e09f7f93999163fc
// Step 1: Exchange auth code for short-lived token
// POST https://api.instagram.com/oauth/access_token
// Body: client_id, client_secret, grant_type=authorization_code, redirect_uri, code

// Step 2: Exchange short-lived for long-lived (60 days)
// GET https://graph.instagram.com/access_token
//   ?grant_type=ig_exchange_token
//   &client_secret={secret}
//   &access_token={short_lived_token}

// Step 3: Get IG user ID (needed for all publishing calls)
// GET https://graph.instagram.com/me?fields=id,username&access_token={token}
// Response: {"id": "123456789", "username": "dj_handle"}
```

### Proactive Token Refresh
```go
// Source: https://developers.facebook.com/docs/instagram-platform/reference/refresh_access_token/
// GET https://graph.instagram.com/refresh_access_token
//   ?grant_type=ig_refresh_token
//   &access_token={current_non_expired_long_lived_token}
// Response: {"access_token": "...", "token_type": "bearer", "expires_in": 5183944}
// Note: token must be valid (not expired) and at least 24h old to refresh
```

### Publish Feed Post (Go struct pattern)
```go
// Source: https://developers.facebook.com/docs/instagram-platform/content-publishing/

// Step 1: Create container
// POST https://graph.instagram.com/v22.0/{ig_user_id}/media
// Required: image_url (must be publicly accessible JPEG), access_token
// Optional: caption (omit for stories), media_type (default IMAGE; STORIES for stories)

// Step 2: Publish
// POST https://graph.instagram.com/v22.0/{ig_user_id}/media_publish
// Required: creation_id (from step 1), access_token
// Returns: {"id": "<IG_MEDIA_ID>"}
```

### Container Status Check (Duplicate Prevention)
```go
// Source: https://developers.facebook.com/docs/instagram-platform/content-publishing/
// GET https://graph.instagram.com/v22.0/{container_id}
//   ?fields=status_code
//   &access_token={token}
// Response: {"status_code": "PUBLISHED", "id": "..."}
// Possible status_code values: IN_PROGRESS, FINISHED, PUBLISHED, ERROR, EXPIRED
```

### Database Schema for scheduled_posts (Migration 003)
```sql
-- +goose Up

-- Extend social_accounts (stub from migration 001) with columns needed for publishing
ALTER TABLE social_accounts
  ADD COLUMN IF NOT EXISTS ig_user_id     TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS token_expiry   TIMESTAMPTZ;

CREATE TABLE scheduled_posts (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  social_account_id UUID NOT NULL REFERENCES social_accounts(id),
  image_minio_path  TEXT NOT NULL,
  caption           TEXT NOT NULL DEFAULT '',
  post_type         TEXT NOT NULL CHECK (post_type IN ('feed', 'story')),
  status            TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','scheduled','publishing','published','failed','permanently_failed')),
  scheduled_at_utc  TIMESTAMPTZ,
  timezone_name     TEXT NOT NULL DEFAULT 'UTC',
  published_at_utc  TIMESTAMPTZ,
  ig_media_id       TEXT,
  ig_container_id   TEXT,
  retry_count       INT NOT NULL DEFAULT 0,
  next_retry_at     TIMESTAMPTZ,
  error_reason      TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at        TIMESTAMPTZ
);

CREATE INDEX ON scheduled_posts(status, scheduled_at_utc) WHERE deleted_at IS NULL;
CREATE INDEX ON scheduled_posts(social_account_id)        WHERE deleted_at IS NULL;
CREATE INDEX ON scheduled_posts(status, next_retry_at)    WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS scheduled_posts;
ALTER TABLE social_accounts DROP COLUMN IF EXISTS ig_user_id;
ALTER TABLE social_accounts DROP COLUMN IF EXISTS token_expiry;
```

### Caption Auto-Generation (TypeScript)
```typescript
// Source: CONTEXT.md caption template decision
// Reads djName from useSettingsStore, tracklist metadata from useTracklistStore
function generateCaption(djName: string, title: string, trackCount: number, avgBpm: number): string {
  return `${djName} @ ${title} — ${trackCount} tracks • avg ${avgBpm} BPM\n#techno #djset`
}
```

### Timezone Picker (Browser-Native, No npm Package)
```typescript
// Source: MDN Web API — Intl.supportedValuesOf
// Available in Chrome 99+, Firefox 103+, Safari 15.4+
const allTimezones: string[] = Intl.supportedValuesOf('timeZone')
// Example output: ["Africa/Abidjan", ..., "Europe/Berlin", ..., "UTC"]

// Filter on user input for searchable combobox
const filtered = computed(() =>
  allTimezones.filter(tz => tz.toLowerCase().includes(search.value.toLowerCase()))
)
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Instagram Basic Display API (personal accounts) | Instagram Graph API (Business/Creator accounts only) | December 4, 2024 (EOL) | User must have a Business or Creator Instagram account; personal accounts cannot connect |
| `graph.facebook.com` for Instagram publishing | `graph.instagram.com` for token management and publishing | 2024 Instagram Direct Login launch | Simpler flow; doesn't require Facebook account linkage |
| Creating media container at scheduling time | Creating container only at publish time (in worker) | API constraint — always been this way | Containers expire after 24 hours; must be created within 24h of intended publish time |
| Posting image URL from internal network | Publicly accessible presigned URL via `MINIO_PUBLIC_ENDPOINT` | Fundamental API requirement | Instagram servers fetch the image directly; internal URLs will never work |

**Deprecated/outdated:**
- `instagram.com/oauth/authorize` with `instagram_basic` scope alone: Needs `instagram_business_content_publish` for posting
- Instagram Basic Display API: EOL Dec 4, 2024; all integrations must use Graph API
- Assuming PNG works for image uploads: Instagram only accepts JPEG

---

## Open Questions

1. **JPEG Conversion Pipeline**
   - What we know: Instagram only accepts JPEG. The tracklist generator exports PNG (via Playwright screenshot).
   - What's unclear: Whether conversion should happen at export time or at social-upload time.
   - Recommendation: Convert PNG to JPEG in the social service layer (`image/jpeg` stdlib) when the image is attached to a post. Store the JPEG under `social/posts/{post_id}/image.jpg` in MinIO. Do not modify the original tracklist export.

2. **Instagram App Review for Stories Publishing**
   - What we know: Stories publishing requires `instagram_business_content_publish` scope.
   - What's unclear: Whether a self-hosted personal-use app with no public distribution requires the same App Review process as a publicly distributed app. Meta's documentation is written for publicly distributed apps.
   - Recommendation: Test stories publishing in Meta's development mode first (sandbox with test accounts). If App Review is required, gate the Stories post type behind a UI flag and show a "pending review" message. Feed posts work without App Review for development mode.

3. **OAuth Redirect URI for Self-Hosted Production**
   - What we know: The redirect URI must be whitelisted in the Meta App Dashboard. Localhost URIs are supported for development.
   - What's unclear: For a self-hosted production instance at a custom domain, the user must manually add their domain to the Meta App Dashboard.
   - Recommendation: Support `INSTAGRAM_REDIRECT_URI` env var (already scaffolded in `config.go` as `InstagramClientID`/`InstagramClientSecret`). Document in `docs/SELF-HOSTING.md` that the production redirect URI must be registered in the Meta App Dashboard.

---

## Validation Architecture

> `workflow.nyquist_validation` is `true` in `.planning/config.json` — section is required.

### Test Framework
| Property | Value |
|----------|-------|
| Framework (frontend) | Vitest (existing, `apps/dj/vitest.config.ts`) |
| Framework (backend) | `go test` + testcontainers-go (existing) |
| Config file (frontend) | `apps/dj/vitest.config.ts` |
| Quick run (frontend) | `pnpm nx test dj -- --testPathPattern social` |
| Full suite (frontend) | `pnpm nx test dj` |
| Quick run (backend) | `pnpm nx test api -- -run TestSocial -count=1 ./internal/social/...` |
| Full suite (backend) | `pnpm nx test api` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SOCL-01 | OAuth URL constructed with correct scopes and CSRF state | unit | `go test ./internal/social/... -run TestOAuthURL` | ❌ Wave 0 |
| SOCL-01 | OAuth callback exchanges code for long-lived token and stores encrypted | unit (mocked HTTP) | `go test ./internal/social/... -run TestOAuthCallback` | ❌ Wave 0 |
| SOCL-02 | Token refresh called when expiry within 7 days | unit | `go test ./internal/social/... -run TestTokenRefreshProactive` | ❌ Wave 0 |
| SOCL-02 | Account marked disconnected on refresh failure | unit | `go test ./internal/social/... -run TestTokenRefreshFailure` | ❌ Wave 0 |
| SOCL-03 | Post scheduled with UTC conversion from IANA timezone | unit | `go test ./internal/social/... -run TestScheduleUTCConversion` | ❌ Wave 0 |
| SOCL-04 | Worker picks up due posts (status=scheduled, scheduled_at_utc<=NOW) | integration (testcontainers) | `go test ./internal/social/... -run TestWorkerPublishDue` | ❌ Wave 0 |
| SOCL-05 | Instagram client creates IMAGE container then publishes | unit (mocked HTTP) | `go test ./internal/social/... -run TestPublishFeedPost` | ❌ Wave 0 |
| SOCL-06 | Instagram client creates STORIES container then publishes | unit (mocked HTTP) | `go test ./internal/social/... -run TestPublishStory` | ❌ Wave 0 |
| SOCL-07 | Presigned GET URL generated from MINIO_PUBLIC_ENDPOINT before publish | unit | `go test ./internal/social/... -run TestPresignedURL` | ❌ Wave 0 |
| SOCL-08 | Image >8MB rejected at upload; non-JPEG MIME rejected | unit | `go test ./internal/social/... -run TestImageValidation` | ❌ Wave 0 |
| SOCL-09 | useSocialStore loads posts and groups by status | unit (Vitest) | `pnpm nx test dj -- --testPathPattern social.test` | ❌ Wave 0 |
| SOCL-10 | All 6 valid status values accepted; invalid value rejected by DB CHECK | unit | `go test ./internal/social/... -run TestPostStatusEnum` | ❌ Wave 0 |
| SOCL-11 | Backoff delays computed correctly (5m, 20m, 80m) | unit | `go test ./internal/social/... -run TestBackoffSchedule` | ❌ Wave 0 |
| SOCL-11 | After 3 failures post marked permanently_failed | unit | `go test ./internal/social/... -run TestRetryExhaustion` | ❌ Wave 0 |
| SOCL-12 | Manual retry resets retry_count and schedules immediately | unit | `go test ./internal/social/... -run TestManualRetry` | ❌ Wave 0 |
| SOCL-13 | 429 response triggers Retry-After header respecting backoff | unit (mocked HTTP) | `go test ./internal/social/... -run TestRateLimit429` | ❌ Wave 0 |
| SOCL-14 | Edit blocked on publishing/published/failed/permanently_failed | unit | `go test ./internal/social/... -run TestEditBlocking` | ❌ Wave 0 |
| SOCL-15 | Disconnect moves all account's scheduled posts to draft | unit | `go test ./internal/social/... -run TestDisconnectCascade` | ❌ Wave 0 |
| SOCL-16 | Retry aborted if container status_code=PUBLISHED; post marked published | unit (mocked HTTP) | `go test ./internal/social/... -run TestDuplicatePrevention` | ❌ Wave 0 |
| SOCL-17 | Social page reads ?imageId and opens compose panel pre-filled | unit (Vitest, mount social.vue) | `pnpm nx test dj -- --testPathPattern socialPage.test` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** Run the test package for the task's files (e.g. `go test ./internal/social/...` for backend tasks; `pnpm nx test dj -- --testPathPattern social` for frontend tasks)
- **Per wave merge:** `pnpm nx run-many -t test -p dj api`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `api/internal/platform/migrations/003_social.sql` — required before any social service tests can run
- [ ] `api/internal/social/handler_test.go` — HTTP handler tests for all routes
- [ ] `api/internal/social/service_test.go` — business logic unit tests (scheduling, retry, disconnect cascade)
- [ ] `api/internal/social/worker_test.go` — worker tick, backoff calculation, token refresh trigger
- [ ] `api/internal/social/instagram_test.go` — mocked HTTP Instagram API client (container create, publish, status check)
- [ ] `apps/dj/app/stores/__tests__/social.test.ts` — useSocialStore unit tests (load, create, retry, prefill from imageId)
- [ ] `apps/dj/app/components/social/__tests__/SocialPostCompose.test.ts` — caption auto-gen, char counter, timezone combobox

---

## Sources

### Primary (HIGH confidence)
- [Meta — Instagram Content Publishing](https://developers.facebook.com/docs/instagram-platform/content-publishing/) — two-step publish flow, endpoint parameters, rate limits, image requirements (JPEG only, 8 MB max, publicly accessible URL)
- [Meta — OAuth Authorize](https://developers.facebook.com/docs/instagram-platform/reference/oauth-authorize/) — authorization URL, required scopes, callback handling
- [Meta — Refresh Access Token](https://developers.facebook.com/docs/instagram-platform/reference/refresh_access_token/) — 60-day token validity, 24-hour minimum age for refresh, refresh endpoint
- [Instagram Direct Login implementation guide (July 2024)](https://gist.github.com/PrenSJ2/0213e60e834e66b7e09f7f93999163fc) — full OAuth flow, short-lived to long-lived exchange, IG user ID retrieval
- `api/internal/platform/crypto/aes.go` — existing AES-256-GCM implementation confirmed
- `api/internal/platform/migrations/001_initial_schema.sql` — existing `social_accounts` table stub confirmed
- `api/go.mod` — confirmed all needed packages already present (no new `go get` required)
- `apps/dj/vitest.config.ts` — confirmed Vitest test infrastructure

### Secondary (MEDIUM confidence)
- Multiple sources corroborating: Instagram Basic Display API EOL December 4, 2024; Business/Creator accounts only
- `Intl.supportedValuesOf('timeZone')` — MDN Web API; confirmed browser support (Chrome 99+, Firefox 103+, Safari 15.4+)
- Go `time.Ticker` + goroutine for polling workers — standard Go pattern confirmed against stdlib docs

### Tertiary (LOW confidence — needs validation in development)
- Instagram App Review requirement for Stories publishing on self-hosted non-public apps — unclear from official docs whether development-mode access bypasses App Review for single-user apps

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all backend deps in existing go.mod; all UI primitives installed; no new packages needed
- Architecture: HIGH — exactly mirrors the tracklist package pattern (handler/service/repository + worker goroutine)
- Instagram API endpoints and parameters: HIGH — verified against official Meta documentation
- Token lifecycle (60-day, 7-day proactive refresh): HIGH — verified from official refresh endpoint docs
- Image constraints (JPEG only, 8 MB, publicly accessible URL): HIGH — verified from official IG media endpoint docs
- Stories App Review requirement for non-public apps: LOW — unclear; needs testing in development mode

**Research date:** 2026-03-21
**Valid until:** 2026-06-21 (Instagram API stable; token behavior unlikely to change; 90-day window)
