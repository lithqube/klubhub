# Phase 2: Social Media Scheduler — Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can schedule generated tracklist images to Instagram Feed and Stories. The system publishes reliably — with retry on failure, a manual download fallback, and queue/calendar visibility of all posts. OAuth token management is included. Analytics and Automations are explicitly out of scope for this phase.

</domain>

<decisions>
## Implementation Decisions

### Queue vs Calendar Views
- **Default view: Queue** — card grid of posts (matches the Social Scheduler mockup)
- **Calendar tab also built in Phase 2**: monthly grid with dot indicators per day (colored by status: cyan=scheduled, gray=published, magenta=failed)
- Clicking a calendar day filters the queue to show only posts for that date
- **Tabs in scope: QUEUE + CALENDAR only** — Analytics and Automations tabs rendered as "Coming Soon" stubs (no backend, just placeholder UI)
- **Empty queue empty state**: dashed "NEXT SLOT" card (matching mockup) + a connection banner above if no Instagram account is connected
- Post cards in queue: image thumbnail, status badge (READY/SCHEDULED/PUBLISHED/FAILED), datetime, caption excerpt, platform tag (INSTAGRAM / INSTAGRAM REELS)

### "Schedule Post" Integration with Tracklist Page
- **Flow: Navigate to Social page** — clicking "Schedule to Instagram" on the tracklist page navigates to `/social?imageId={id}`
- The Social page reads the `imageId` query param, fetches the image from MinIO, auto-expands the new-post form, and pre-fills the image
- Navigation (not a sheet/modal) so the user sees their queue context while composing
- **Form visibility**: collapsed by default ("+ ADD TO QUEUE" button); clicking it expands a panel inline above the queue grid. `⚡ QUICK EXPORT FROM TRACKLIST` button next to it also triggers this flow from within Social

### Post Compose Form
- **Caption**: auto-generates a suggested caption from tracklist metadata when pre-filling from a tracklist (template: `[DJ Name] @ [set name] — [track count] tracks • avg [BPM] BPM #techno #djset`). User can freely edit.
- **Character counter**: adaptive by post type
  - Feed: live counter `482 / 2200`, turns red at 90%+
  - Story: counter dimmed + note "Captions are not shown on Stories"
- **Timezone**: datetime-local picker + searchable timezone dropdown (IANA names, e.g. `Europe/Berlin`). Stored as UTC internally. Display shows local time + UTC equivalent.
- **Post type selector**: Feed vs Story (affects image aspect ratio expectations + caption counter behavior)

### Editing Scheduled Posts
- **Inline card expansion**: clicking a scheduled post card expands it in-place (card grows vertically). Editable fields appear within the card. Save collapses back. No navigation away from the queue.
- Edit only available on posts in `scheduled` status (not in-flight or published)

### Failed Post Handling
- **Magenta banner at top of card**: `⚠ ATTENTION REQUIRED: SYNC ERROR` (matches mockup)
- Status badge: FAILED (magenta filled pill)
- **Two actions on failed card**: `RETRY SYNC` (manually trigger one more attempt) + `DOWNLOAD IMAGE` (download the image file to post manually)
- This state appears after 3 automatic retries with exponential backoff have been exhausted
- Error reason shown below the banner (e.g. "Token expired — reconnect Instagram")

### Instagram OAuth + Token State
- **No Instagram connected**: connection banner visible above the queue with `CONNECT INSTAGRAM` CTA. Queue shows dashed NEXT SLOT empty card below.
- **Connected**: small green status dot + account handle in top-right of the Social module header (matching the `STATUS: SYNCED` sidebar pattern)
- **Token expiry warning**: proactive orange banner `⚠ INSTAGRAM TOKEN EXPIRING IN 7 DAYS — RE-AUTHORIZE` before expiry; does not freeze the queue until actually expired
- **Token expired mid-session**: queue freezes for new scheduling attempts, banner changes to `● INSTAGRAM DISCONNECTED — RE-AUTHORIZE` (magenta). Existing scheduled posts remain but won't publish.

### Claude's Discretion
- Exact Pinia store structure for social posts (`useSocialStore`)
- Database schema for `scheduled_posts` table (UUIDs, status enum, retry_count, scheduled_at_utc, timezone_name)
- Go polling worker implementation (time.Ticker interval — 1 min recommended)
- Exponential backoff formula (start: 5 min, max: 4 hr)
- MinIO path convention for social post images
- Instagram Graph API endpoint selection (Feed: `/me/media` + `/me/media_publish`, Stories: same with `media_type=STORIES`)
- Exact timezone dropdown component (searchable select from IANA list, filter on type)

</decisions>

<specifics>
## Specific Ideas

### Mockup-derived patterns (from user-provided screenshots)
- **Social page header**: `SOCIAL SCHEDULER` in cyan italic + tabs: `QUEUE` (active, underlined) | `ANALYTICS` | `AUTOMATIONS`
- **Post cards**: 3-column grid layout, image thumbnail at top, status badge, datetime (`OCT 24, 2023 — 23:45`), caption excerpt, platform tag at bottom
- **Failed card top**: full-width magenta bar with `⚠ ATTENTION REQUIRED: SYNC ERROR` label
- **RETRY SYNC**: appears in the failed card footer in magenta text
- **NEXT SLOT card**: dashed border empty card with future datetime, `SCHEDULE NEW ACTIVITY` text at very bottom of the page
- **Buttons**: `⚡ QUICK EXPORT FROM TRACKLIST` (ghost-border) + `+ ADD TO QUEUE` (gradient-cta) — both in top right of the queue view

### Caption template
When pre-filling from a tracklist:
```
[DJ Name] @ [Tracklist Title] — [N] tracks • avg [BPM] BPM
#techno #djset #[genre]
```
User edits freely after auto-fill.

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `app/components/ui/badge/Badge.vue` — 13 variants including SCHEDULED (cyan filled), PUBLISHED (gray), FAILED (magenta), PENDING (text-only). Directly usable for post status.
- `app/components/ui/card/Card.vue` — glass-panel dark surface. Post cards extend this.
- `app/components/ui/progress/Progress.vue` — 4px cyan bar. Useful if showing "publishing..." progress within a card.
- `app/components/ui/switch/Switch.vue` — sharp toggle. Usable for Feed/Story toggle or notification toggles.
- `app/components/ui/input/Input.vue` — accent-bar-focus. Caption textarea uses same style.
- `app/components/ui/select/Select.vue` — shadcn Select for timezone dropdown (may need to extend to searchable combobox).
- `app/stores/settings.ts` — `djName` ref. Caption auto-generator reads this.
- `app/stores/tracklist.ts` — `tracklist` ref with track count, BPM data. Caption auto-generator reads this when navigating from tracklist page.

### Established Patterns
- Pinia stores: composition API style with `defineStore('name', () => { ... })`. New `useSocialStore` should follow this.
- API calls: `$fetch('/api/v1/...')` — no Axios, no wrapper. Direct Nuxt $fetch.
- Pages: single-file `pages/social.vue`. Feature components in `components/social/`.
- Status colors: cyan=active/ready, magenta=error/failed, gray=inactive/published — already in design tokens.

### Integration Points
- `pages/tracklist.vue` → adds "Schedule to Instagram" button in the export panel → navigates to `/social?imageId={minioId}`
- `pages/social.vue` → new page, added to sidebar nav (Social entry already in mockup sidebar)
- `TheNav.vue` (Plan 06) → will include Social nav item with icon
- Go API: new routes `/api/v1/social/accounts`, `/api/v1/social/posts`, `/api/v1/social/posts/{id}/retry`
- Background worker: Go goroutine polling `scheduled_posts` table every 60s

</code_context>

<deferred>
## Deferred Ideas

- **Analytics tab** — post performance metrics (reach, impressions, engagement) — future phase after Phase 2 ships
- **Automations tab** — auto-scheduling rules (e.g. "post every tracklist image at 8pm on the day of the gig") — future phase
- **Multi-platform support** (Twitter/X, TikTok, Facebook) — future phase; architecture should not block it but Phase 2 is Instagram-only
- **Best time to post suggestions** — ML/analytics-based optimal timing — future phase

</deferred>

---

*Phase: 02-social-media-scheduler*
*Context gathered: 2026-03-20*
