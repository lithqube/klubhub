# KLUBHUB DJ

> **Historical requirements record:** MinIO references below are superseded
> for v1.0.0 deployment by Garage (S3-compatible). They remain here to preserve
> the original requirements traceability, not as current operating guidance.

*Requirements Addendum to BRD v2.0*

---

**Business Requirements Addendum**
Version 2.0-A — March 2026

This document captures all clarified gaps, unstated assumptions, and new requirements identified during BRD analysis. Every item here is accepted and supplements the original BRD v2.0. In case of conflict, this addendum takes precedence.

---

## A1. Network & Security Boundaries

### A1.1 Default Network Binding

All Docker services SHALL bind to `127.0.0.1` by default. A `BIND_ADDRESS` environment variable in `.env` allows the user to override this (e.g., `0.0.0.0` for LAN access). The `.env.example` file SHALL include `BIND_ADDRESS=127.0.0.1` with a comment explaining the security implications of changing it.

### A1.2 MinIO Console Access

The MinIO web console (port 9001) SHALL be disabled in the default `docker-compose.yml`. A `docker-compose.dev.yml` override SHALL enable it for development/debugging. The main compose file only exposes the S3 API (port 9000) to the Go backend via Docker networking.

### A1.3 Token Encryption at Rest

Social media access tokens stored in `social_accounts.access_token` SHALL be encrypted using AES-256-GCM. The encryption key is derived from a `TOKEN_ENCRYPTION_KEY` environment variable. If this key is lost or changed, all social account tokens become invalid and the user must re-authorize. The `.env.example` SHALL include a generated placeholder and a comment explaining this.

---

## A2. File Upload & Parsing

### A2.1 Maximum Upload Size

| Setting | Default | Configurable |
|---|---|---|
| `MAX_UPLOAD_SIZE_MB` | 50 | Yes, via `.env` |

The system SHALL reject uploads exceeding this limit with a clear error message: "File exceeds the maximum allowed size of {limit} MB."

### A2.2 Duplicate Upload Handling

Uploading the same file multiple times SHALL always create a new, independent tracklist entry. No deduplication is performed. Users may want different image configurations from the same set data. Users can manually delete unwanted duplicates.

### A2.3 Partial Parse Results

If a file contains tracks with malformed or missing metadata fields, the system SHALL:

1. Parse all tracks that can be parsed.
2. Include partially parsed tracks with available fields populated and missing fields set to null.
3. Return per-track warnings (e.g., "Track 14: BPM field missing or malformed").
4. Never silently discard tracks.
5. Allow the user to review warnings and proceed or cancel.

### A2.4 Post-Parse Track Editing

**New FR-003a:** The system SHALL allow inline editing of all track metadata fields (artist, title, BPM, key, label, duration) after parsing. Edits persist to the `tracks` table in PostgreSQL. The original source file in MinIO is never modified.

### A2.5 Empty File Handling

If a valid file is uploaded but contains zero parseable tracks, the system SHALL return a specific error: "File parsed successfully but contains no tracks. Please verify this is a valid DJ history export with at least one track."

### A2.6 Large Tracklist Display

Each image template SHALL define a `max_tracks_displayed` value (default: 30). If a tracklist exceeds this limit:

1. The UI SHALL display a track range selector (e.g., "Showing tracks 1–30 of 200").
2. The generated image includes a footer note: "... and {N} more tracks" or the user-selected range.
3. The user may generate multiple images covering different track ranges.

### A2.7 Non-Latin Character Support

**New NFR-004:** Image templates SHALL support full Unicode rendering, including CJK (Chinese, Japanese, Korean), Cyrillic, Arabic, and accented Latin characters. The system SHALL bundle fonts with broad Unicode coverage (Noto Sans family recommended). If a glyph cannot be rendered, a substitution marker (e.g., `?`) SHALL be displayed rather than crashing or producing blank text.

---

## A3. Tracklist Deletion & Cascading

**New FR-005a:** The system SHALL allow deleting tracklists with the following cascade behavior:

| Entity | On Tracklist Deletion |
|---|---|
| `tracks` (children) | Soft-deleted with parent |
| `generated_images` (children) | Soft-deleted; MinIO files marked for cleanup |
| `gigs.tracklist_id` (reference) | Set to NULL; gig remains intact |
| `scheduled_posts` referencing images | Unaffected (image file remains until MinIO cleanup) |

A confirmation dialog SHALL warn the user: "This will delete the tracklist, {N} tracks, and {M} generated images. Linked gigs will be unlinked. Continue?"

---

## A4. Cover Art

### A4.1 Placeholder Specification

The default placeholder image SHALL be a dark-toned tile (matching the dark minimal template palette) with a centered music note icon. Dimensions: 500x500 pixels, PNG format. The placeholder SHALL be visually distinct from real cover art so users can identify unfound artwork at a glance.

Users MAY upload a custom placeholder image in `user_settings`. Custom placeholders are stored in MinIO and used globally for all tracks without matched art.

### A4.2 Manual Cover Art Override

**New FR-007a:** The system SHALL allow per-track manual cover art upload. A user-uploaded image overrides the API-fetched result for that track. The override is:

- Stored in MinIO with a reference in `tracks.cover_art_path`.
- Flagged as `user_provided` (metadata field or naming convention) to distinguish from API results.
- Persistent — re-fetching cover art from APIs does not overwrite a manual override unless the user explicitly resets it.

### A4.3 Cover Art Review Before Generation

The tracklist configuration UI (step 3 of the workflow) SHALL display all fetched cover art thumbnails next to their tracks. For each track, the user can:

1. Accept the fetched art (default).
2. Reject it (falls to next source in chain or placeholder).
3. Upload a replacement (per A4.2).

### A4.4 All Cover Art APIs Unreachable

If all three cover art APIs (Spotify, Discogs, MusicBrainz) are unreachable during a fetch attempt:

1. All tracks receive placeholder art.
2. The UI displays a warning: "Cover art services are unreachable. Check your internet connection and API keys. You can retry cover art fetching later."
3. The user is NOT blocked from generating images.
4. A "Retry Cover Art" button SHALL be available on the tracklist detail view to re-attempt fetching without re-uploading the source file.

---

## A5. Image Generation

### A5.1 Preview vs. Export Resolution

| Mode | Resolution | Purpose |
|---|---|---|
| Live Preview | 50% of output dimensions | Fast rendering for real-time configuration |
| Export | Full resolution (1080x1920 or 1080x1080) | Final output for download/sharing |

The UI SHALL indicate this distinction: "Preview is approximate. Exported image is full quality."

### A5.2 Concurrent Generation

Multiple image generation requests MAY run concurrently. Each request is independent and stateless. The UI SHALL provide an "Export Both" convenience button that triggers both Story (1080x1920) and Square (1080x1080) generation in a single user action. Both results appear in `generated_images` as separate entries.

---

## A6. Social Media Scheduler

### A6.1 OAuth Token Refresh

**New FR-020a:** The system SHALL automatically refresh Instagram OAuth tokens before expiry. A background job SHALL check token expiry daily and refresh tokens with more than 7 days remaining (to handle API delays). If refresh fails:

1. Mark the `social_accounts` entry status as `disconnected`.
2. Surface a warning on the dashboard: "Instagram account {name} needs re-authorization."
3. All scheduled posts for that account remain in `scheduled` status but are skipped by the scheduler until the account is reconnected.

### A6.2 Post Editing After Scheduling

Scheduled posts MAY be fully edited (caption, image, hashtags, scheduled time, timezone) while in `scheduled` status. Once the scheduler transitions a post to `publishing` status, edits are blocked. The UI SHALL disable edit controls and display: "This post is currently being published."

### A6.3 Default Timezone

The default timezone for all scheduling is read from the `TZ` environment variable in `.env`. Each scheduled post MAY override this with a per-post timezone selection. All times in the UI SHALL display in the user's configured timezone with UTC offset shown (e.g., "Mar 15, 2026 at 7:00 PM (UTC+1)").

### A6.4 Rate Limit Handling

The scheduler SHALL respect Instagram Graph API rate limits by:

1. Spacing publish attempts with a minimum interval of 30 seconds between posts.
2. If a 429 (rate limited) response is received, backing off for the duration specified in the response headers (or 15 minutes default).
3. Surfacing rate-limit status in the queue view: "Publishing paused — rate limited until {time}."

### A6.5 Image Compliance Validation

Before scheduling a post with a custom-uploaded image (not generated by the tracklist module), the system SHALL validate:

| Check | Requirement |
|---|---|
| Format | JPEG or PNG |
| File size | Under 8 MB |
| Dimensions | Minimum 320px on shortest side; maximum 1440px on longest side for feed posts |
| Aspect ratio | Between 4:5 and 1.91:1 for feed; 9:16 for stories |

Non-compliant uploads are rejected with a specific error identifying which check failed.

### A6.6 Account Disconnection

When a user disconnects a social account:

1. A confirmation dialog SHALL warn: "Disconnecting will pause {N} scheduled posts for this account. Continue?"
2. On confirmation, all posts in `scheduled` status for that account transition to `draft` status.
3. The scheduler stops processing those posts.
4. `draft` posts are preserved and can be reassigned to a new account or deleted.

### A6.7 Permanently Failed Posts

**New FR-027a:** After exhausting the maximum retry count (3 attempts with exponential backoff), the system SHALL:

1. Set post status to `permanently_failed`.
2. Surface the failure on the dashboard widget and in the queue view with the error reason from the API.
3. Provide a "Retry Now" button for manual one-time retry.
4. Provide a "Download Image" button so the user can manually post to Instagram.
5. Permanently failed posts remain in the queue view with a distinct visual indicator until the user deletes or retries them.

---

## A7. Tracklist-to-Social Handoff (Cross-Module)

### A7.1 Post-Export CTA

After successful image export in the Tracklist Image Generator, the UI SHALL display a "Schedule Post" call-to-action button. Clicking it navigates to the social post creation form with:

- The generated image pre-attached.
- The tracklist name as a suggested caption prefix.

If no social account is connected, the CTA links to the social account setup flow instead.

### A7.2 Module-Aware UI

UI elements that reference unshipped or inactive modules SHALL be hidden. Specifically:

- CTAs linking to modules not yet deployed are not rendered.
- Dashboard widgets for modules with no data are hidden.
- Module detection uses a lightweight check: the API responds to the module's health sub-endpoint (e.g., `GET /api/v1/social/health` returns 200 or 404).

When a new module ships (user updates their deployment), its UI elements appear automatically on next page load.

---

## A8. EPK / Press Kit Builder

### A8.1 Rich Text Formatting Scope

The rich text editor for artist bios SHALL support:

| Format | PDF Rendering |
|---|---|
| Bold | Bold font weight |
| Italic | Italic font style |
| Bulleted lists | Indented bullet points |
| Numbered lists | Indented numbered items |
| Hyperlinks | Blue underlined text; URL in parentheses after link text |
| H2, H3 headings | Larger/bolder font sizes |

Unsupported: embedded images, tables, code blocks, custom fonts within rich text. These are stripped on save with a warning.

### A8.2 Press Photo Limits

| Constraint | Value |
|---|---|
| Maximum photos per EPK | 20 |
| Maximum file size per photo | 10 MB |
| Accepted formats | JPEG, PNG |
| PDF optimization | Resized to max 2000px longest edge |

Original uploads are preserved at full resolution in MinIO. The PDF uses optimized versions.

### A8.3 EPK Export Versioning

Each PDF export creates a new `epk_exports` row. Previous exports are preserved and listed in an export history view with timestamps. Users may download or delete any previous export. Deleting an export removes the PDF from MinIO and the DB record.

### A8.4 Manual Gig History in EPK (Pre-Gig Tracker)

`epk_profiles.gig_highlights` stores a JSONB array of objects:

```json
[
  {
    "date": "2025-12-31",
    "venue": "Berghain",
    "city": "Berlin",
    "event_name": "NYE 2025"
  }
]
```

These are manually entered in Phase 1c. When the Gig Tracker ships in Phase 1d, a UI button "Import from Gigs" SHALL populate this array from selected `gigs` entries. Manual entries and imported entries coexist in the same array.

---

## A9. Gig Tracker

### A9.1 Status Workflow — Defined States

**Gig Status** enum (ordered):

| State | Description | Terminal? |
|---|---|---|
| `inquiry` | Initial interest or inquiry sent/received | No |
| `confirmed` | Gig is confirmed by both parties | No |
| `advanced` | Deposit or advance payment received | No |
| `played` | Gig has been performed | No |
| `cancelled` | Gig cancelled (by either party) | Yes |

Forward skipping is allowed (e.g., `inquiry` → `played` for retroactive logging). Backward transitions are allowed (e.g., `confirmed` → `cancelled`). The `cancelled` state is terminal — no transitions out.

**Payment Status** enum (independent of gig status):

| State | Description |
|---|---|
| `unpaid` | No payment received |
| `deposit_paid` | Partial/advance payment received |
| `paid` | Full payment received |
| `overdue` | Payment past agreed due date |
| `waived` | Fee waived (charity gig, trade, etc.) |

Gig status and payment status are updated independently. The Finance module auto-creates an income entry when `payment_status` transitions to `paid`.

### A9.2 Currency Handling

- Currency stored as ISO 4217 code (e.g., `USD`, `EUR`, `GBP`, `JPY`).
- No automatic currency conversion anywhere in the system.
- The UI provides a searchable dropdown of common currencies pre-populated from the ISO 4217 list.
- Amounts are stored as decimal with 2 decimal places.
- Display format follows the currency's convention (e.g., `$1,000.00`, `1.000,00 EUR`).

### A9.3 Promoter Data — Inline with Copy Convenience

Promoter contact fields (name, email, phone) remain inline on the `gigs` table in v1. No separate contacts entity. A "Copy from Previous Gig" dropdown SHALL allow the user to select a previous gig and auto-fill promoter fields. A full CRM/Contacts module is deferred to v2.0 per the roadmap.

---

## A10. Finance Tracker

### A10.1 Auto-Created Income Entry Lifecycle

When a gig's `payment_status` transitions to `paid`:

1. An income entry is auto-created with: amount = gig fee, currency = gig currency, category = `gig_fee`, date = transition date, gig_id = linked gig, description = "Gig fee: {event_name} at {venue_name}".
2. The income entry is flagged as `auto_generated: true`.

**If gig fee changes after auto-creation:**

- The system SHALL display a notification: "The fee for gig '{event_name}' has changed from {old} to {new}. Update the linked income entry?"
- User confirms or dismisses. No silent modification of financial records.

**If gig reverts from `paid` to another payment status:**

- The system SHALL display a notification: "Gig '{event_name}' is no longer marked as paid. Delete or void the linked income entry?"
- User chooses: delete the entry, void it (keep for audit trail with $0 amount), or keep as-is.

### A10.2 Tax Reporting — Intentionally Excluded

KlubHub DJ v1 is a tracking tool, not an accounting or tax tool. The system does NOT calculate taxes, tax rates, VAT, or withholding. Category breakdowns and date-filtered exports provide data users can give to their accountant or import into accounting software. This limitation SHALL be documented in the Finance module UI and in project documentation.

### A10.3 Invoice Numbering

| Setting | Value |
|---|---|
| Format | `{PREFIX}-{YYYY}-{NNN}` |
| Default prefix | `INV` |
| Sequence | Auto-incrementing per calendar year, starting at 001 |
| Custom prefix | Configurable in `user_settings` |
| On deletion | Sequence number is never reused (standard accounting practice) |

Example: `INV-2026-001`, `INV-2026-002`. If `INV-2026-002` is deleted, the next invoice is `INV-2026-003`.

### A10.4 Multi-Currency Summaries

All summary views (monthly, yearly, P&L) SHALL group totals by currency. Example:

```
March 2026 Income:
  EUR: 5,000.00
  USD: 3,200.00
  GBP: 1,500.00

March 2026 Expenses:
  EUR: 2,100.00
  USD: 800.00
```

No cross-currency aggregation. No automatic conversion. Manual exchange rate entries (FR-056) allow the user to record a currency conversion as a pair of transactions (expense in source currency, income in target currency).

### A10.5 Foreign Currency Gig Payment

When a gig fee is in currency A but the DJ receives payment in currency B:

1. The auto-created income entry uses the gig's original currency and amount.
2. The user MAY manually edit the income entry to reflect the actual received currency/amount.
3. Alternatively, the user MAY void the auto-created entry and create a manual entry with the converted amount.
4. The system does not enforce any specific workflow — it provides the tools and lets the user choose.

---

## A11. Release Planner

### A11.1 Artists Field

`releases.artists` is a TEXT field containing a comma-separated list of artist/collaborator names (e.g., "DJ Shadow, Cut Chemist"). No separate artist entity in v1. This matches industry standard credit formatting.

### A11.2 Promo Checklist Templates

The system SHALL ship with a default promo checklist template:

1. Send promo copies to DJs
2. Submit to playlist curators
3. Schedule social media posts
4. Update artist profiles (Spotify, Bandcamp, etc.)
5. Send to press/blogs for review
6. Update website/EPK with new release

Users MAY customize the checklist per release (add, remove, reorder items). A "Save as Default Template" button SHALL allow the user to persist their customized checklist as the new default for future releases.

---

## A12. Tour Manager

### A12.1 Tour Stops Require Linked Gigs

Tour stops (`tour_stops`) MUST reference an existing gig (`gig_id` FK is required, not nullable). The UI SHALL provide a "Create Gig + Add to Tour" shortcut that:

1. Opens the gig creation form.
2. On save, automatically creates the tour stop linked to the new gig.

This ensures all financial data flows through the gig → income pipeline consistently.

### A12.2 Tour Budget Multi-Currency

Tour budget aggregation (FR-073) follows the same multi-currency grouping as Finance summaries. Display format:

```
Tour: European Summer 2026
  Total Fees:     EUR 8,000.00 | GBP 2,500.00
  Total Expenses: EUR 3,200.00 | GBP 400.00
  Net:            EUR 4,800.00 | GBP 2,100.00
```

No cross-currency totals.

---

## A13. Data Integrity & Concurrency

### A13.1 Optimistic Concurrency Control

**New FR-046a:** All entity update operations (PUT endpoints) SHALL implement optimistic concurrency control:

1. The client sends the entity's `updated_at` timestamp with the update request.
2. The backend compares this against the current `updated_at` in the database.
3. If they match, the update proceeds and `updated_at` is refreshed.
4. If they do not match (another write occurred), the backend returns HTTP 409 Conflict with the message: "This record was modified elsewhere. Please refresh and try again."

This prevents silent data loss from concurrent edits in multiple browser tabs.

### A13.2 Soft Deletion Behavior

All entities use soft deletion (`deleted_at` timestamp, NULL when active). Cascading rules:

| Parent Deleted | Child Behavior | Reference Behavior |
|---|---|---|
| Tracklist | Tracks + Generated Images soft-deleted | `gigs.tracklist_id` set to NULL |
| Gig | Linked income entry flagged for user review | `tour_stops.gig_id` — tour stop soft-deleted |
| Tour | Tour stops soft-deleted | Linked gigs remain active |
| EPK Profile | EPK exports soft-deleted | N/A |
| Social Account | Scheduled posts move to `draft` status | N/A |

Soft-deleted entities are excluded from all default list queries. An "Archive" or "Trash" view SHALL allow reviewing and permanently deleting or restoring soft-deleted entities.

### A13.3 MinIO / PostgreSQL Consistency

Write order: MinIO first, then PostgreSQL.

**If PostgreSQL insert fails after MinIO write:**

1. Attempt to delete the orphaned MinIO object (best-effort).
2. Return an error to the user: "Image was generated but could not be saved. Please try again."

**Orphan reconciliation:** A background job (triggered manually via `GET /api/v1/system/reconcile` or on API startup) SHALL:

1. List all MinIO objects in the generated-images bucket.
2. Compare against `generated_images.output_path` in PostgreSQL.
3. Log orphaned objects (present in MinIO but not in DB).
4. Optionally delete orphans older than 24 hours (configurable).

---

## A14. Operational Concerns

### A14.1 Data Retention & Storage Management

No automatic data deletion in v1. The system SHALL provide:

1. **Storage Usage View** in Settings: total MinIO usage by category (source files, cover art, generated images, EPK assets, documents).
2. **Bulk Delete Tools**: "Delete all generated images older than {date}" with confirmation.
3. **Per-Entity Delete**: individual delete available on all entities via UI and API.
4. Documentation of recommended cleanup practices.

### A14.2 Backup & Restore

The project SHALL include a documented backup procedure and helper script:

**`scripts/backup.sh`:**
- Runs `pg_dump` for the PostgreSQL database.
- Exports the MinIO bucket(s) using `mc mirror` or equivalent.
- Packages both into a timestamped archive (e.g., `klubhub-backup-2026-03-15.tar.gz`).

**`scripts/restore.sh`:**
- Accepts a backup archive.
- Restores PostgreSQL from dump.
- Restores MinIO objects from export.

These are manual scripts, not automated scheduled backups. Documentation SHALL include recommended backup frequency (weekly for active users).

### A14.3 Update & Migration Path

Database migrations use a Go-based migration tool (golang-migrate or goose). Migrations:

1. Are embedded in the Go binary.
2. Run automatically on API startup (migrate up to latest).
3. Are versioned and sequential (e.g., `001_initial.up.sql`, `002_add_cancelled_status.up.sql`).
4. Support rollback (`down` migrations) for each version.

User update procedure:

```bash
git pull
docker-compose up -d --build
```

The API container runs migrations before accepting traffic. An `UPGRADING.md` file documents breaking changes and manual steps (if any) per version.

### A14.4 API Key Validation on Startup

The API service SHALL start regardless of API key configuration. Missing or invalid keys degrade gracefully:

| Missing Key | Effect |
|---|---|
| Spotify API | Cover art from Spotify unavailable; Discogs/MusicBrainz still used |
| Discogs API | Cover art from Discogs unavailable; Spotify/MusicBrainz still used |
| MusicBrainz API | No key required (public API); rate-limited by default |
| Instagram/Facebook API | Social Media module disabled; scheduling endpoints return 503 |
| MinIO credentials | API fails to start (hard dependency) — clear error log |
| PostgreSQL credentials | API fails to start (hard dependency) — clear error log |

`GET /api/v1/health` SHALL return per-integration status:

```json
{
  "status": "healthy",
  "database": "connected",
  "storage": "connected",
  "integrations": {
    "spotify": "configured",
    "discogs": "configured",
    "musicbrainz": "available",
    "instagram": "not_configured"
  }
}
```

### A14.5 Instagram Duplicate Post Prevention

When retrying a failed Instagram publish, the scheduler SHALL first check whether the post was actually published (the original request may have succeeded but the response was lost). Before re-attempting publish:

1. Query the Instagram Graph API for recent media on the connected account.
2. If a matching media item exists (by creation time proximity and image hash), mark the post as `published` and skip re-publish.
3. If no match, proceed with retry.

---

## A15. Legal & Compliance

### A15.1 GDPR — Self-Hosted Disclaimer

KlubHub DJ is a self-hosted, single-user tool. Personal data (promoter contacts, etc.) never leaves the user's infrastructure except:

- Instagram Graph API: post content and captions (no PII by default).
- Cover art APIs: artist name + track title sent as search queries (not PII).

GDPR compliance for stored contact data is the user's responsibility. Documentation SHALL include a note: "As the data controller for information stored in your KlubHub DJ instance, you are responsible for handling personal data (e.g., promoter contacts) in compliance with applicable privacy laws."

### A15.2 Cover Art Usage — User Responsibility

Album artwork fetched from Spotify, Discogs, and MusicBrainz is used under those platforms' respective API terms of service. Re-embedding fetched artwork in user-generated images for social media posting may have copyright implications depending on jurisdiction and use case.

The system SHALL display a one-time dismissible notice: "Cover art is fetched from third-party services. You are responsible for ensuring your use of album artwork complies with applicable copyright laws and platform terms of service."

This notice SHALL also appear in project documentation.

---

## A16. Consolidated New & Modified Functional Requirements

### A16.1 New Functional Requirements

| ID | Module | Requirement |
|---|---|---|
| **FR-003a** | Tracklist | System shall allow inline editing of all parsed track metadata fields. Edits persist to PostgreSQL; original source file is unchanged. |
| **FR-004a** | Tracklist | System shall return partial parse results with per-track warnings for malformed or missing metadata. Tracks are never silently discarded. |
| **FR-005a** | Tracklist | System shall allow deleting tracklists with cascading soft-delete to child tracks and generated images, with confirmation dialog. Gig references are nullified. |
| **FR-007a** | Tracklist | System shall allow per-track manual cover art upload, overriding API-fetched results. Overrides persist and are not replaced by API re-fetch unless explicitly reset. |
| **FR-009a** | Tracklist | System shall enforce a max-tracks-displayed limit per template (default 30) with a user-selectable track range for display. |
| **FR-017** | Tracklist | System shall enforce a configurable maximum upload file size (default 50 MB) and reject oversized files with a clear error. |
| **FR-020a** | Social | System shall automatically refresh Instagram OAuth tokens before expiry and mark accounts as disconnected on refresh failure, prompting re-authorization. |
| **FR-027a** | Social | After max retry exhaustion (3 attempts), system shall mark posts as permanently_failed, surface the error, and provide manual retry and image download options. |
| **FR-040a** | Gig | System shall support a `cancelled` terminal gig status with no outbound transitions. |
| **FR-046a** | All | System shall implement optimistic concurrency control using `updated_at` on all entity update operations, returning HTTP 409 on conflict. |
| **FR-052a** | Finance | System shall prompt the user (not silently modify) when gig fee changes or payment status reversals affect auto-created income entries. |

### A16.2 New Non-Functional Requirements

| ID | Area | Requirement |
|---|---|---|
| **NFR-001** | Security | All services shall bind to 127.0.0.1 by default; configurable via `BIND_ADDRESS` env var. |
| **NFR-002** | Operations | Database migrations shall run automatically on API startup using a versioned migration tool. |
| **NFR-003** | Operations | `GET /api/v1/health` shall report per-integration status (database, storage, each external API). |
| **NFR-004** | Accessibility | Image templates shall support full Unicode rendering including CJK, Cyrillic, and Arabic via bundled Noto Sans fonts. |
| **NFR-005** | Integrity | Write order for generated assets: MinIO first, PostgreSQL second. Orphan reconciliation available via API endpoint and on startup. |
| **NFR-006** | Operations | Project shall include `scripts/backup.sh` and `scripts/restore.sh` for manual backup/restore of PostgreSQL and MinIO data. |

---

## A17. Data Model Amendments

The following changes supplement the data model defined in BRD Section 14:

### A17.1 Modified Tables

| Table | Change |
|---|---|
| `tracks` | Add `cover_art_source` enum: `spotify`, `discogs`, `musicbrainz`, `placeholder`, `user_upload`. Allows distinguishing API art from manual overrides. |
| `gigs` | `gig_status` enum updated to: `inquiry`, `confirmed`, `advanced`, `played`, `cancelled`. |
| `gigs` | `payment_status` enum defined as: `unpaid`, `deposit_paid`, `paid`, `overdue`, `waived`. |
| `income_entries` | Add `auto_generated` boolean (default false). Flags entries created by the gig→finance auto-link. |
| `scheduled_posts` | Add `permanently_failed` to status enum. Full enum: `draft`, `scheduled`, `publishing`, `published`, `failed`, `permanently_failed`. |
| `social_accounts` | Add `status` enum: `connected`, `disconnected`. Tracks token validity. |
| All tables | Add `deleted_at` timestamp (nullable) for soft deletion, if not already implied by BRD. |

### A17.2 New Tables

| Table | Fields | Purpose |
|---|---|---|
| `promo_checklist_templates` | id, name, items (JSONB array of task names), is_default (boolean) | Stores user-customizable default promo checklist templates for the Release Planner. |

---

*End of Addendum. This document is complete and ready for use alongside BRD v2.0 as the full requirements baseline for KlubHub DJ.*
