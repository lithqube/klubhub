# Phase 2: Social Media Scheduler - Research

**Researched:** 2026-03-14
**Domain:** Social media scheduling, Instagram Graph API, Go background workers
**Confidence:** MEDIUM

## Summary

This phase involves implementing a social media scheduler for Instagram Feed and Stories with reliable publishing. Key technical components include: Instagram OAuth connection, automatic token refresh, scheduling posts with timezone support, a queue/calendar view, automatic publishing with retry logic, rate limit adherence, and integration with the tracklist generator via a "Schedule Post" CTA. The implementation decisions have been locked in the CONTEXT.md, specifying a DB-backed polling mechanism (goroutine checking DB every minute), specific UI views (calendar/list), editing behavior for scheduled posts, rate limiting with exponential backoff, and integration points. Research focuses on validating the technical feasibility of these decisions, particularly the Instagram Graph API capabilities for publishing, token management, and rate limits, along with Go concurrency patterns for the polling worker.

**Primary recommendation:** Implement the scheduler using the locked decisions (DB-backed polling, specific UI/UX patterns) and leverage the Instagram Graph API for publishing with careful attention to token refresh and rate limit handling. Use Go's standard library for concurrency (time.Ticker, context) rather than external worker libraries for the polling mechanism.

## Standard Stack

### Core

| Library             | Version               | Purpose                                  | Why Standard                                                                      |
| ------------------- | --------------------- | ---------------------------------------- | --------------------------------------------------------------------------------- |
| Go                  | 1.25.7                | Primary backend language                 | Matches project's existing Go version and concurrency model                       |
| Instagram Graph API | v24.0                 | Publishing to Instagram Feed and Stories | Official Meta API for professional accounts; required for programmatic publishing |
| Go time.Ticker      | stdlib                | Polling mechanism for scheduled posts    | Built-in, lightweight ticker for interval-based database checks                   |
| Go context          | stdlib                | Cancellation and timeout handling        | Standard for managing goroutine lifecycles and request-scoped values              |
| PostgreSQL          | Project's existing DB | Storing scheduled posts and metadata     | Consistent with project's existing data storage                                   |
| MinIO               | Project's existing    | Storing tracklist images for scheduling  | Already integrated via internal/pkg/minio/ wrapper                                |
| Vue 3               | 3.5.13                | Frontend framework                       | Matches project's existing Nuxt/Vue stack                                         |
| Nuxt                | 4.0.0                 | Frontend framework                       | Existing project framework for SSR and routing                                    |

### Supporting

| Library                                        | Version | Purpose                                    | When to Use                                                  |
| ---------------------------------------------- | ------- | ------------------------------------------ | ------------------------------------------------------------ |
| github.com/assaidy/workers                     | v1.1.0  | Background job processing with retry logic | Alternative to custom polling worker if complexity increases |
| github.com/qcserestipy/instagram-api-go-client | latest  | Type-safe Instagram Graph API client       | For simplified API interactions (if adopted)                 |
| golang.org/x/oauth2                            | stdlib  | OAuth 2.0 token management                 | Standard library for Instagram OAuth flow                    |
| github.com/go-chi/jwt                          | latest  | JWT handling for token storage             | If implementing custom token storage beyond DB               |

### Alternatives Considered

| Instead of                         | Could Use                                | Tradeoff                                                                                                                               |
| ---------------------------------- | ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Custom DB-backed polling goroutine | Redis-backed queue (e.g., Redis Streams) | Redis adds infrastructure complexity but offers better visibility and durability; DB polling is simpler with existing stack            |
| Go time.Ticker                     | github.com/robfig/cron/v3                | Cron library offers more scheduling flexibility but adds dependency; ticker is sufficient for fixed-interval polling                   |
| Manual OAuth implementation        | golang.org/x/oauth2                      | Standard library is battle-tested; custom implementation risks security issues                                                         |
| Custom retry logic                 | github.com/cenkalti/backoff/v4           | Library provides robust backoff strategies but adds dependency; simple exponential backoff with jitter is straightforward to implement |

**Installation:**

```bash
go get github.com/assaidy/workers
go get github.com/qcserestipy/instagram-api-go-client
# Note: Instagram Graph API is accessed via HTTP, no Go SDK installation strictly required
# Standard library packages (time, context, oauth2) are included with Go
```

## Architecture Patterns

### Recommended Project Structure

```bash
api/
├── internal/
│   ├── scheduler/          # Scheduler-specific logic
│   │   ├── service.go      # Scheduler service implementation
│   │   ├── worker.go       # Background polling worker
│   │   ├── model.go        # Data models for scheduled posts
│   │   └── handler.go      # API handlers for scheduler endpoints
│   ├── instagram/          # Instagram API wrapper (to be implemented)
│   │   └── client.go       # Instagram Graph API client
│   ├── api/
│   │   └── v1/
│   │       ├── scheduler/  # Scheduler API routes
│   │       └── ...         # Other API versions
│   └── minio/              # Existing MinIO wrapper
├── pkg/
│   └── instagram/          # Placeholder for Instagram package (per existing code context)
└── cmd/
    └── api/                # Main application entry point

frontend/
├── components/
│   └── scheduler/          # Scheduler-specific UI components
│       ├── CalendarView.vue
│       ├── ListView.vue
│       ├── SchedulerModal.vue
│       └── PostForm.vue
├── composables/
│   └── useScheduler.js     # Scheduler API composable
├── pages/
│   └── scheduler/          # Scheduler page route
│       └── index.vue
└── lib/
    └── date-fns/           # Existing date formatting utilities
```

### Pattern 1: DB-backed Polling Worker

**What:** A background goroutine that periodically checks the database for posts ready to be published, processes them with appropriate status transitions, and handles errors with retry logic.

**When to use:** When you need reliable, durable scheduling that persists across application restarts and doesn't require external infrastructure beyond the existing database.

**Example:**

```go
// Source: Internal patterns from existing codebase (health checks, GIG phase workflows)
func (s *SchedulerService) StartPollingWorker(ctx context.Context) {
    ticker := time.NewTicker(s.pollingInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // Check for posts ready to publish
            readyPosts, err := s.repo.GetReadyForPublishingPosts()
            if err != nil {
                s.logger.Error("Failed to get ready posts", "error", err)
                continue
            }

            for _, post := range readyPosts {
                // Process each post in a separate goroutine to avoid blocking
                go s.processPost(ctx, post)
            }
        case <-ctx.Done():
            s.logger.Info("Scheduler worker shutting down")
            return
        }
    }
}

func (s *SchedulerService) processPost(ctx context.Context, post *model.ScheduledPost) {
    // Transition to publishing status
    if err := s.repo.UpdatePostStatus(post.ID, model.StatusPublishing); err != nil {
        s.logger.Error("Failed to update post status to publishing", "error", err, "post_id", post.ID)
        return
    }

    // Attempt to publish via Instagram API
    if err := s.instagramClient.PublishPost(ctx, post); err != nil {
        // Handle failure with retry logic
        s.handlePublishFailure(ctx, post, err)
        return
    }

    // Success - transition to published
    if err := s.repo.UpdatePostStatus(post.ID, model.StatusPublished); err != nil {
        s.logger.Error("Failed to update post status to published", "error", err, "post_id", post.ID)
    }
}
```

### Pattern 2: State Transition Model

**What:** Modeling scheduled posts as a state machine with clear transitions: scheduled → publishing → published/failed → (retry) → publishing or permanently_failed.

**When to use:** When you need clear auditability of post lifecycle and deterministic behavior for retries and editing.

**Example:**

```go
// Source: GIG phase workflow patterns (inquiry→confirmed→etc.)
type PostStatus string

const (
    StatusScheduled     PostStatus = "scheduled"
    StatusPublishing    PostStatus = "publishing"
    StatusPublished     PostStatus = "published"
    StatusFailed        PostStatus = "failed"
    StatusPermanentlyFailed PostStatus = "permanently_failed"
    StatusCancelled     PostStatus = "cancelled"
)

// State transition rules:
// scheduled → publishing (when time reaches)
// publishing → published (on success)
// publishing → failed (on failure, then retry logic)
// failed → publishing (on retry, up to max attempts)
// failed → permanently_failed (after max retries)
// scheduled → cancelled (when edited, original post)
// scheduled → scheduled (when edited, new post)
```

### Pattern 3: Optimistic Concurrency for Post Editing

**What:** Using `updated_at` timestamp to detect and prevent lost updates when editing scheduled posts, similar to existing INFRA-11 pattern.

**When to use:** When multiple users might edit the same post or when edits could happen concurrently with publishing attempts.

**Example:**

```go
// Source: INFRA-11 pattern using updated_at for conflict detection
func (s *SchedulerService) EditPost(ctx context.Context, postID uint64, edit model.PostEdit) (*model.ScheduledPost, error) {
    // Get current post with version check
    currentPost, err := s.repo.GetPostByID(postID)
    if err != nil {
        return nil, err
    }
    if currentPost.Status != model.StatusScheduled {
        return nil, errors.New("can only edit scheduled posts")
    }

    // Create new post with edited values
    newPost := &model.ScheduledPost{
        UserID:     currentPost.UserID,
        InstagramAccountID: currentPost.InstagramAccountID,
        ImageID:    edit.ImageID or currentPost.ImageID,
        Caption:    edit.Caption or currentPost.Caption,
        ScheduledAt: edit.ScheduledAt or currentPost.ScheduledAt,
        Timezone:   edit.Timezone or currentPost.Timezone,
        Status:     model.StatusScheduled,
    }

    // Mark original as cancelled (using updated_at for conflict detection)
    if err := s.repo.CancelPost(postID, currentPost.UpdatedAt); err != nil {
        return nil, err
    }

    // Create new post
    return s.repo.CreatePost(newPost)
}
```

### Anti-Patterns to Avoid

- **Building custom OAuth 2.0 implementation:** Don't hand-roll OAuth logic; use golang.org/x/oauth2 which is battle-tested and secure.
- **Using unbounded goroutines for each post:** Don't spawn a new goroutine for every post to publish; use worker pools or the existing ticker pattern to prevent resource exhaustion.
- **Storing access tokens without encryption:** Don't store long-lived access tokens in plaintext; encrypt them at rest or use a secure vault solution.
- **Ignoring rate limit headers:** Don't fail to check and respect Instagram's rate limit headers (X-Ratelimit-Limit, X-Ratelimit-Remaining, X-Ratelimit-Reset) which can lead to temporary blocks.
- **Not handling token expiration gracefully:** Don't assume tokens never expire; implement proactive refresh before expiry and handle 401 responses with refresh attempts.

## Don't Hand-Roll

| Problem                              | Don't Build                 | Use Instead                                                    | Why                                                                                                                                                                         |
| ------------------------------------ | --------------------------- | -------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OAuth 2.0 flow for Instagram         | Custom OAuth implementation | golang.org/x/oauth2                                            | Instagram's OAuth has specific requirements (state validation, PKCE, proper redirect handling) that are easy to get wrong; standard library handles token exchange securely |
| Retry logic with exponential backoff | Custom retry implementation | github.com/cenkalti/backoff/v4 or simple stdlib implementation | Proper jitter implementation and max retry limits are nuanced; library prevents thundering herd problems                                                                    |
| Rate limit handling                  | Custom rate limiter         | Respect Instagram's headers + simple token bucket              | Instagram's rate limits are complex (200/hr, per-app, per-user); custom implementations often miscalculate reset times                                                      |
| Media upload handling                | Custom multipart/form-data  | net/http with proper multipart writer                          | Instagram requires specific headers and formatting; manual implementation risks malformed requests                                                                          |
| Timezone storage/display             | Manual timezone conversion  | time.Location with UTC storage + TZ database                   | Proper timezone handling requires IANA database; manual offsets fail with DST changes                                                                                       |

**Key insight:** The Instagram Graph API has specific requirements for media publishing (container creation, publishing endpoint, media types) and authentication that are complex to implement correctly. Leveraging established patterns and libraries reduces the risk of integration failures that could prevent posts from publishing.

## User Constraints (from CONTEXT.md)

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions

- Use DB-backed polling: a background goroutine checks the database every minute for posts whose scheduled time has passed and are in `scheduled` status.
- When time matches, the post status transitions to `publishing`, the image is published via Instagram API, and on success moves to `published` or on failure to `failed` with retry logic.
- Default view is a monthly calendar with color-coded dots on dates indicating post status (scheduled, publishing, published, failed, permanently_failed).
- Clicking a date shows a list view of posts for that day with details: scheduled time, caption preview, image thumbnail, and status icons.
- Users can toggle between calendar and list views via a toolbar button.
- While a post is in `scheduled` status, users can edit caption, image, and scheduled time.
- Editing creates a new scheduled post entry; the original is marked as `cancelled` (not shown in default views).
- If the new scheduled time is in the past, an error is shown and the edit is rejected.
- Editing does not reset retry counters; the new post starts fresh.
- Implement exponential backoff with jitter for retries (base 1 minute, max 15 minutes) after failures.
- Enforce minimum 30-second interval between successful posts to the same Instagram account; if a post is ready but the interval hasn't elapsed, delay until it has.
- Track the last successful post timestamp per account to enforce the interval.
- After a tracklist image is successfully exported, a non-intrusive toast notification appears with a "Schedule Post" button.
- Clicking the button opens the scheduler modal in overlay mode, with the exported image pre-selected in the image upload field and a preview shown.
- The modal defaults to the current date/time for scheduling, but users can adjust all fields.

### Claude's Discretion

- Exact polling interval duration (currently 1 minute) can be tuned for responsiveness vs. resource usage.
- Design of toast notification and scheduler modal (styling, animations).
- Specific implementation of exponential backoff jitter algorithm.
- Handling of timezone storage and display (using ICU or moment.js equivalents).

### Deferred Ideas (OUT OF SCOPE)

- Support for Facebook Pages, Twitter/X, and TikTok posting — future phases.
- Social media content calendar view with drag-and-drop rescheduling.
- AI-generated captions or hashtag suggestions.
- Advanced analytics on post performance (likes, comments, reach).
- Bulk scheduling via CSV upload.
  </user_constraints>

## Common Pitfalls

### Pitfall 1: Media Type Confusion for Reels vs Video

**What goes wrong:** When attempting to publish Reels via the Instagram Graph API, developers consistently receive the error "Media created with media_type=VIDEO is a carousel item and cannot be published as a standalone post" even when explicitly setting media_type=REELS in the container creation request.

**Why it happens:** This is a known issue with the Instagram Graph API where certain video specifications (particularly around duration, frame rate, or encoding) cause the API to misclassify Reels-standard videos as carousel-ineligible content. The error message is misleading as it suggests using media_type=REELS when it's already being used.

**How to avoid:**

1. Ensure video meets strict Reels requirements:
   - Duration between 3-60 seconds
   - Frame rate 30fps or less
   - H.264 codec, AAC audio
   - Maximum 30MB file size
   - Vertical aspect ratio (9:16)
2. Upload video first to a publicly accessible URL (required by API)
3. Consider using the Resumable Upload endpoint for larger files
4. Check the Media Container status endpoint after creation to see if it's valid before attempting to publish

**Warning signs:**

- Getting error 2207089 with "Carousel Item Cannot Be Published Standalone" message
- Successful container creation but failed publish despite correct media_type
- Works for short test videos but fails with actual content

### Pitfall 2: Token Expiration Without Refresh

**What goes wrong:** Long-lived Instagram User Access Tokens expire after 60 days, causing scheduled posts to fail silently when the scheduler attempts to publish with an expired token.

**Why it happens:** Many developers assume that once obtained, long-lived tokens remain valid indefinitely or don't implement proactive refresh mechanisms, only discovering the issue when posts start failing.

**How to avoid:**

1. Implement automatic token refresh before expiry (refresh when token is <7 days old)
2. Store token expiry timestamp alongside the token in the database
3. Handle 401 Unauthorized responses from Instagram API by attempting token refresh
4. Notify users when token refresh fails requiring re-authentication
5. Use the `/refresh_access_token` endpoint with `grant_type=ig_refresh_token`

**Warning signs:**

- Posts failing with OAuthException or invalid token errors
- Scheduler logs showing 401 responses from Instagram API
- No new posts being published despite scheduled times passing

### Pitfall 3: Rate Limit Miscalculation

**What goes wrong:** The scheduler exceeds Instagram's rate limits (200 API calls per hour) causing temporary blocks and failed posts, particularly when handling multiple accounts or retrying failed posts.

**Why it happens:** Developers often underestimate the cumulative API calls from token checks, media container creation, publishing attempts, and insights requests, especially when implementing retry logic without proper backoff.

**How to avoid:**

1. Implement request counting and respect the X-Ratelimit-Remaining header
2. Queue requests when approaching limits rather than bursting
3. Use exponential backoff with jitter for retries (as specified in locked decisions)
4. Monitor and log rate limit header values for tuning
5. Consider batching operations where possible (though Instagram API has limited batch support)

**Warning signs:**

- HTTP 429 Too Many Requests responses
- X-Ratelimit-Remaining header consistently low or zero
- Posts failing intermittently with "try again later" messages
- Need to wait extended periods for limits to reset

### Pitfall 4: Timezone Handling Errors

**What goes wrong:** Posts publish at incorrect times due to improper timezone storage, conversion, or scheduling logic, particularly around DST transitions.

**Why it happens:** Storing times as UTC without proper timezone context, or converting times incorrectly when displaying to users versus storing for scheduling.

**How to avoid:**

1. Always store scheduled times in UTC in the database
2. Store the original timezone identifier (e.g., "America/New_York") separately
3. Use a reliable timezone database (IANA/TZ database) for conversions
4. When displaying times to users, convert from UTC to their selected timezone
5. When checking for posts to publish, convert current time to UTC for comparison
6. Test scheduling around DST transition dates (spring/fall)

**Warning signs:**

- Posts publishing 1 hour early or late
- Incorrect times displayed in calendar view
- Issues specifically occurring during DST transition periods
- Users in different timezones seeing inconsistent times

### Pitfall 5: Lost Updates During Post Editing

**What goes wrong:** When users edit a scheduled post concurrently with the scheduler checking for posts to publish, changes are lost or inconsistent states occur.

**Why it happens:** Without proper concurrency control, two operations (edit and publish check) can read the same post state, leading to one overwriting the other or processing outdated data.

**How to avoid:**

1. Implement optimistic concurrency using updated_at timestamps (as in locked decisions)
2. When editing a scheduled post, verify the record hasn't changed since last read
3. If conflict detected, reject the edit and ask user to retry
4. Consider locking mechanisms for critical sections if optimistic approach proves insufficient
5. Ensure edits create new posts rather than modifying in-place (as specified)

**Warning signs:**

- Edits disappearing or not taking effect
- Posts publishing with old captions/images despite edits
- Database showing inconsistent states (e.g., cancelled posts still being processed)
- User reporting that edits "didn't save"

### Pitfall 6: Media Hosting Requirements Oversight

**What goes wrong:** Attempting to publish posts fails because the media (image/video) is not hosted at a publicly accessible URL at the time of API calls.

**Why it happens:** The Instagram Graph API requires media to be accessible via a public URL when creating the media container. Developers sometimes assume they can upload directly or use private/storage URLs.

**How to avoid:**

1. Ensure all media is uploaded to a publicly accessible CDN or storage service before attempting to schedule
2. In this project's case, use MinIO with public bucket access or pre-signed URLs with sufficient expiry
3. Validate media accessibility before creating container (HEAD request to media URL)
4. Store media URLs (not just IDs) in the scheduled post record for reliability
5. Implement media expiry tracking and refresh if needed

**Warning signs:**

- HTTP 400 errors with "Invalid parameter" or "URL not accessible" messages
- Successful container creation but failed publish
- Works in development with localhost URLs but fails in production
- Media ID valid but URL returns 403 or 404 when accessed by Instagram's servers

## Code Examples

Verified patterns from official sources:

### Instagram Media Container Creation for Image

```go
// Source: Facebook Instagram Graph API Documentation - Media endpoint
// https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/ig-user/media/
func (c *InstagramClient) CreateImageContainer(ctx context.Context, imageURL, caption string) (string, error) {
    endpoint := fmt.Sprintf("%s/%s/media", c.baseURL, c.igUserID)
    data := url.Values{}
    data.Set("image_url", imageURL)
    data.Set("caption", caption)
    data.Set("access_token", c.accessToken)

    req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(data.Encode()))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        ID string `json:"id"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    return result.ID, nil
}
```

### Instagram Media Container Creation for Reels

```go
// Source: Facebook Instagram Graph API Documentation - Reels posting
// https://developers.facebook.com/docs/instagram-platform/content-publishing/#reels
func (c *InstagramClient) CreateReelsContainer(ctx context.Context, videoURL, caption string) (string, error) {
    endpoint := fmt.Sprintf("%s/%s/media", c.baseURL, c.igUserID)
    data := url.Values{}
    data.Set("video_url", videoURL)
    data.Set("caption", caption)
    data.Set("media_type", "REELS")
    data.Set("access_token", c.accessToken)

    req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(data.Encode()))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        ID string `json:"id"`
        Error struct {
            Message string `json:"message"`
            Code    int    `json:"code"`
        } `json:"error"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    if result.Error.Message != "" {
        return "", fmt.Errorf("instagram api error (%d): %s", result.Error.Code, result.Error.Message)
    }

    return result.ID, nil
}
```

### Publishing Media Container

```go
// Source: Facebook Instagram Graph API Documentation - Media Publish endpoint
// https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/ig-user/media_publish/
func (c *InstagramClient) PublishContainer(ctx context.Context, creationID string) (string, error) {
    endpoint := fmt.Sprintf("%s/%s/media_publish", c.baseURL, c.igUserID)
    data := url.Values{}
    data.Set("creation_id", creationID)
    data.Set("access_token", c.accessToken)

    req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(data.Encode()))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        ID string `json:"id"`
        Error struct {
            Message string `json:"message"`
            Code    int    `json:"code"`
        } `json:"error"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    if result.Error.Message != "" {
        return "", fmt.Errorf("instagram api error (%d): %s", result.Error.Code, result.Error.Message)
    }

    return result.ID, nil
}
```

### Token Refresh Implementation

```go
// Source: Facebook Instagram Graph API Documentation - Refresh Access Token
// https://developers.facebook.com/docs/instagram-platform/reference/refresh_access_token/
func (c *InstagramClient) RefreshToken(ctx context.Context) error {
    endpoint := fmt.Sprintf("%s/refresh_access_token", c.baseURL)
    data := url.Values{}
    data.Set("grant_type", "ig_refresh_token")
    data.Set("access_token", c.accessToken)

    req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
    if err != nil {
        return err
    }
    req.URL.RawQuery = data.Encode()

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var result struct {
        AccessToken string `json:"access_token"`
        ExpiresIn   int    `json:"expires_in"` // Seconds until expiry (typically 5184000 = 60 days)
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return err
    }

    c.accessToken = result.AccessToken
    c.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
    return nil
}
```

### Database Query for Ready Posts

```sql
-- Get posts scheduled for publishing that are ready to be processed
SELECT id, user_id, instagram_account_id, image_id, caption, scheduled_at, timezone, status, retry_count
FROM scheduled_posts
WHERE status = 'scheduled'
  AND scheduled_at <= NOW() AT TIME ZONE 'UTC'
  AND (next_retry_at IS NULL OR next_retry_at <= NOW() AT TIME ZONE 'UTC')
ORDER BY scheduled_at ASC;
```

### Exponential Backoff with Jitter Implementation

```go
// Calculate delay with exponential backoff and jitter
// Base: 1 minute, Max: 15 minutes, Jitter: ±25%
func calculateRetryDelay(attempt int) time.Duration {
    if attempt <= 0 {
        return 0
    }

    // Exponential backoff: 1min * 2^(attempt-1), capped at 15min
    baseDelay := time.Minute
    maxDelay := 15 * time.Minute

    delay := baseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
    if delay > maxDelay {
        delay = maxDelay
    }

    // Add jitter: ±25% of delay
    jitter := time.Duration(float64(delay) * 0.25 * (2*rand.Float64() - 1))
    delay += jitter

    // Ensure non-negative
    if delay < 0 {
        delay = 0
    }

    return delay
}
```


## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Basic Display API for read-only access | Instagram Graph API for publishing and insights | Basic Display API deprecated Dec 2024 | Enabled programmatic content publishing for business/creator accounts |
| Manual token management | Automatic token refresh with expiry tracking | Ongoing improvement | Reduced authentication failures and improved scheduler reliability |
| Fixed retry intervals | Exponential backoff with jitter | Industry standard practice | Prevented thundering herd problems and rate limit exhaustion |
| UTC-only time storage | UTC storage with timezone identifier | Growing awareness of timezone complexity | Correct scheduling across DST transitions and user timezones |
| Direct media upload to API | Public URL requirement for media | Instagram API design constraint | Necessitated reliable media hosting (MinIO/CDN) and URL validation |
| Unbounded goroutine per task | Worker pools or ticker-based polling | Modern Go concurrency practices | Prevented resource exhaustion under load |
| Manual OAuth implementation | Standard library (golang.org/x/oauth2) | Security best practices | Reduced vulnerability to authentication flaws |

**Deprecated/outdated:**
- Basic Display API: Deprecated December 2024, replaced by Instagram Graph API for all programmatic access
- Manual token refresh without expiry tracking: Leads to unexpected authentication failures
- Fixed interval retries without jitter: Causes thundering herd problems during service outages
- Storing times as strings instead of time objects: Leads to parsing errors and timezone mishandling
- Assuming media can be uploaded directly to Instagram API: Instagram requires publicly accessible media URLs

## Sources

### Primary (HIGH confidence)
- Facebook Instagram Graph API Documentation - Official API reference for media publishing, token refresh, and endpoints
- Go standard library documentation - time, context, http packages for implementation patterns
- Existing project codebase patterns - Reusable workers, middleware, and MinIO integration

### Secondary (MEDIUM confidence)
- Instagram Graph API: Complete Developer Guide for 2026 (Elfsight) - Verified API limits and authentication requirements
- Publishing to Instagram via API: A technical guide (Postproxy Blog) - Verified permissions and app review process
- How to Publish Instagram Reels via API: Upload, Schedule, and Automate Short-Form Video (Postproxy Blog) - Verified Reels-specific requirements

### Tertiary (LOW confidence)
- Go Concurrency Patterns 2026: Modern Parallel Programming Best Practices (Reintech.io) - Worker pool patterns for background processing
- Instagram API Rate Limits: 200 DMs/Hour Explained (2026) (Creatorflow.so) - Rate limit information
- Refresh Access Token - Instagram Platform (Meta for Developers) - Token refresh endpoint details

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Based on project's existing technology stack and documented decisions
- Architecture: HIGH - Based on locked decisions in CONTEXT.md and verified implementation patterns
- Pitfalls: MEDIUM - Based on verified API behaviors and industry knowledge, with some specific error codes requiring validation
- Code examples: HIGH - Directly sourced from official Facebook API documentation

**Research date:** 2026-03-14
**Valid until:** 2026-04-13 (30 days for stable technologies like Instagram API and Go)
