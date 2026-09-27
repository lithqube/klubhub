# Plan: KlubHub DJ — SoundCloud integration (licensed feature)

**Source**: user request + SoundCloud API research (2026-09-27)
**Selected scope**: EPK import, streaming stats, playlist-as-tracklist import, and scheduled publishing — all four, sliced by risk
**Complexity**: Large (a full external OAuth integration on the write side; small on the read side)
**Status**: Not started — this document records the research and design; no code has been written yet

---

## Summary

Add a SoundCloud integration to KlubHub DJ, gated as a licensed/SaaS edition feature (off by default, same mechanism as the existing Resident Advisor import — see `docs/EDITIONS.md`). Unlike RA import, SoundCloud requires per-user OAuth (the DJ's own login), not an anonymous public API, so the closer architectural precedent is the existing Instagram social scheduler in `api/internal/social`.

Four sub-features were requested, in order of implementation risk:

1. **EPK import** (read-only) — profile, bio, tracks into the press kit.
2. **Streaming stats** (read-only) — play counts, likes, follower growth as a dashboard widget.
3. **Playlist-as-tracklist import** (read-only) — see §3, this is not "extract cue points from a mix," which SoundCloud's API cannot do.
4. **Scheduled publishing** (write) — schedule a track/mix to publish to SoundCloud, mirroring the Instagram scheduler.

(1)-(3) share one OAuth/token foundation and are comparable in risk to RA import once that foundation exists. (4) is materially larger: it needs a chunked upload pipeline, a retry worker, and a SoundCloud API Terms of Use / attribution review before it ships.

---

## 1. Research: the SoundCloud API (verified 2026-09-27)

Sources used: the API Guide (`https://developers.soundcloud.com/docs/api/guide`), the LLM-context overview, and the OpenAPI spec (`https://developers.soundcloud.com/docs/api/explorer/api.json`). Facts below are quoted or paraphrased from those pages, not invented — per the user's explicit instruction, no endpoint or header here should be treated as authoritative without re-checking the spec at implementation time, since API surfaces drift.

### 1.1 Auth

- **Flow: OAuth 2.1, Authorization Code + PKCE.** Client Credentials exists but is for public-resource-only access and doesn't fit any of the four sub-features — all four need data or actions behind the DJ's own SoundCloud login.
- **Authorize URL**: `https://secure.soundcloud.com/authorize?client_id=...&redirect_uri=...&response_type=code&code_challenge=...&code_challenge_method=S256&state=...`
- **Token URL** (exchange and refresh): `POST https://secure.soundcloud.com/oauth/token`
  - Exchange: `grant_type=authorization_code`, `client_id`, `client_secret`, `code`, `code_verifier`, `redirect_uri` (must match the authorize call exactly).
  - Refresh: `grant_type=refresh_token`, `client_id`, `client_secret`, `refresh_token`.
- **API request header**: `Authorization: OAuth <access_token>` — **not** `Bearer`. Easy to get wrong by copying the Instagram client's header logic verbatim.
- **Token lifetime**: ~1 hour. **Refresh tokens are single-use** — every refresh call both consumes the old refresh token and issues a new one, which must be persisted immediately or the next refresh fails permanently for that connection.
- **App registration requires a SoundCloud Artist Pro subscription** on whatever account owns the `client_id`/`client_secret`. For a self-hosted product this means **every self-hoster who wants this feature needs their own Artist Pro subscription and their own registered app** — the same shape as the existing `SPOTIFY_CLIENT_ID`/`DISCOGS_API_KEY`/`INSTAGRAM_CLIENT_ID` "bring your own keys" pattern in `docs/CONFIGURATION.md`, not something KlubHub can register once centrally.
- No redirect-URI allowlist or app-review process is documented (unlike Meta's app review for Instagram), which simplifies onboarding relative to the existing Instagram integration.

### 1.2 Rate limits

- **Client Credentials token exchange**: 50 tokens per 12h per app, 30 per 1h per IP. Irrelevant here since this integration uses the authorization-code/refresh-token grants, not repeated client-credentials exchanges — but a bug that falls back to re-running the full auth flow instead of refreshing would hit this.
- **Play-stream requests**: 15,000 per 24h per `client_id`. Matters only if a future slice proxies or previews audio playback; none of the four sub-features currently need this.
- **General API calls**: no documented global aggregate limit, but `429 Too Many Requests` is a defined response and must be handled with exponential backoff, per the user's own rule — the existing `artworkClient` pattern (bounded HTTP client, timeouts, no redirects, size cap; see the v1.0.0 CHANGELOG entry for MusicBrainz) is the model to extend, not a new one to invent.
- `503`/`504` also warrant retry-with-backoff per the guide.

### 1.3 Pagination and errors

- Pagination: `?linked_partitioning=true&limit=N` (default 50, max 200); response includes `next_href`; follow it until absent.
- Error shape (verified from the guide):
  ```json
  {"code": 404, "message": "404 - Not Found", "link": "...", "status": "404 - Not Found", "errors": [{"error_message": "404 - Not Found"}], "error": null}
  ```

### 1.4 Endpoints relevant to the four sub-features

| Sub-feature | Endpoints |
|---|---|
| EPK import | `GET /me` (profile, bio, avatar), `GET /me/tracks` (paginated) |
| Streaming stats | `GET /me/tracks` fields `playback_count`, `favoritings_count`; `GET /me/followers` for follower count |
| Playlist-as-tracklist | `GET /me/playlists`, `GET /playlists/{playlist_urn}` (ordered list of tracks) |
| Scheduled publishing | `POST /tracks` (multipart: `track[title]`, `track[artist]`, `track[asset_data]`; up to 4GB, up to 24h audio, formats AIFF/WAVE/FLAC/OGG/MP2/MP3/AAC/AMR/WMA), `PUT /tracks/{track_urn}` to update, `DELETE /tracks/{track_urn}` |

Track metadata fields confirmed in the schema: `streamable`, `downloadable`, `playback_count`, `favoritings_count` (not `likes_count` — the schema uses "favoriting" terminology throughout), `genre`, `bpm`, `duration` (milliseconds), `artwork_url`, `permalink_url`, `license`, `sharing` (`"public"`/`"private"`).

No webhook or push-notification support is documented anywhere — a publish worker must poll for status, matching the existing Instagram worker's polling shape, not build a callback receiver.

### 1.5 An unresolved ambiguity: "tracklist source"

SoundCloud has no concept of "the tracklist inside this DJ mix" — a track is one audio file with flat metadata, not a sequence of sub-tracks with timestamps the way a 1001Tracklists plain-text export is. The nearest structural match is `GET /playlists/{playlist_urn}`, which returns an **ordered list of separate tracks**. So "tracklist source for the image generator" as scoped here means **importing a SoundCloud playlist as a tracklist** (each playlist item → one row in the existing tracklist model), not extracting cue points from one uploaded mix — SoundCloud's API cannot do the latter. **This needs the user to confirm before §5.3 is built**; if the actual want is "read the tracklist a DJ pasted into their own mix's description," that requires parsing free-text (SoundCloud's `description` field on a track), which is a different and much less reliable feature.

---

## 2. Codebase facts that shape the plan

- **DJ has no user-account model at all** (confirmed via `.claude/plans/promoter-app.plan.md` §11 D4: "DJ-app auth → Promoter only. DJ stays single-user."). This is not a blocker: the existing Instagram integration already stores per-integration OAuth tokens against the single implicit DJ user, not a multi-user auth system. SoundCloud tokens follow the identical shape: one connection record, not a per-user table.
- **The OAuth CSRF/PKCE state pattern already exists** in `api/internal/social/state_store.go`: an in-memory `StateStore` (`state` token returned in the URL, `bind` token set as an HttpOnly cookie, single-use `Consume`, TTL-expired). Its own doc comment says it is "suitable for a single-user self-hosted deployment where the API process is the only authority" — exactly this integration's situation. Reuse this store rather than writing a second one; it may need a small extension to also hold the PKCE `code_verifier` across the redirect, which Instagram's flow (implicit/non-PKCE on SoundCloud's predecessor pattern) didn't need.
- **The retry/worker pattern already exists** in `api/internal/social/worker.go`: `ListDuePosts`, `SetNextRetry`, `UpdateAccountToken` (encrypted token + expiry), all through a narrow repository interface. The publish-slice worker (§5.4) should be shaped the same way, not reinvented.
- **Token encryption already exists**: `api/internal/platform/crypto` is used by the social worker to store the encrypted Instagram token. Reuse it for the SoundCloud access/refresh token pair.
- **Bounded external HTTP clients already exist**: the `artworkClient` pattern (timeout, no redirects, size cap) used for Spotify/Discogs/MusicBrainz cover-art lookups (see `api/internal/artwork`) is the model for a `soundcloudClient`, not `net/http.Get` directly.
- **Feature-flag mechanism is fully built** (`docs/EDITIONS.md`): one bool field on `config.Features` (Go, `api/internal/platform/config/config.go`), one entry in the `Enabled()` map, one entry in `apps/dj/app/utils/features.ts`'s `FEATURES` registry (App/API env var names, edition, description), one `<key>: false` in `nuxt.config.ts`'s `runtimeConfig.public.features`, gated with `useFeatures().isEnabled('<key>')` on the frontend. RA import (`raImport` / `FEATURE_RA_IMPORT` / `NUXT_PUBLIC_FEATURES_RA_IMPORT`) is the exact template — copy its shape, don't design a new one.
- **Migrations**: the highest number as of this plan is `023` (Phase 5 finance-entries work, in progress on a separate branch as of this writing — check the actual latest migration number in `api/internal/platform/migrations/` before picking the next one; do not assume it is 024).
- **"Bring your own keys" env var naming convention**: `SPOTIFY_CLIENT_ID`/`SPOTIFY_CLIENT_SECRET`, `DISCOGS_API_KEY`, `INSTAGRAM_CLIENT_ID`/`INSTAGRAM_CLIENT_SECRET` (see `docs/CONFIGURATION.md`). Follow with `SOUNDCLOUD_CLIENT_ID`/`SOUNDCLOUD_CLIENT_SECRET`, both optional/empty by default, both `*_FILE`-secret-capable per this repo's existing secrets convention.

---

## 3. Proposed architecture

```
apps/dj (Nuxt) ── EPK page (import panel, mirrors EpkRaImportPanel)
              ── Dashboard (stats widget)
              ── Tracklist page (playlist import panel)
              ── Social page? or a new "SoundCloud" settings panel (connect/disconnect, publish schedule)
                    │  /api/v1/**
                    ▼
api/cmd/api (DJ binary)
   └─ internal/soundcloud/
         ├─ client.go       (bounded HTTP client: authorize URL builder, token exchange/refresh, GET/POST wrappers, 429/503/504 backoff)
         ├─ oauth.go         (connect/callback/disconnect handlers; reuses social.StateStore, extended for PKCE code_verifier)
         ├─ model.go         (Connection {access_token_enc, refresh_token_enc, expires_at, scdj_user_id, scopes, connected_at}; imported track/playlist DTOs)
         ├─ repository.go    (connection CRUD; token update on refresh)
         ├─ service.go       (import EPK profile+tracks, compute stats, import playlist as tracklist, enqueue a publish job)
         ├─ handler.go       (routes below)
         └─ worker.go        (publish queue: upload, poll status, retry with backoff — mirrors social.Worker)
```

### Feature flags (two, not one)

- **`FEATURE_SOUNDCLOUD_IMPORT`** — gates EPK import, stats widget, playlist-as-tracklist import. All read-only. Same trust level as RA import.
- **`FEATURE_SOUNDCLOUD_PUBLISH`** — gates the connect flow's write scope and the publish worker/UI. A licensee can enable import without also granting KlubHub the ability to publish to their SoundCloud account — a materially different trust and ToS-exposure boundary, so it should not be one flag. `FEATURE_SOUNDCLOUD_PUBLISH` implies `FEATURE_SOUNDCLOUD_IMPORT` is also on (publishing needs the same OAuth connection); enforce that in config validation rather than the UI alone.

### API routes (all 404 when their flag is off, mirroring RA import exactly)

| Route | Flag |
|---|---|
| `POST /api/v1/soundcloud/connect` (starts OAuth, returns the authorize URL) | `IMPORT` |
| `GET /api/v1/soundcloud/callback` | `IMPORT` |
| `POST /api/v1/soundcloud/disconnect` | `IMPORT` |
| `GET /api/v1/soundcloud/status` (connected? scopes? expiry?) | `IMPORT` |
| `POST /api/v1/epk/import-soundcloud` | `IMPORT` |
| `GET /api/v1/soundcloud/stats` | `IMPORT` |
| `GET /api/v1/soundcloud/playlists`, `POST /api/v1/tracklist/import-soundcloud-playlist` | `IMPORT` |
| `POST /api/v1/social/soundcloud/schedule`, matching routes for edit/cancel | `PUBLISH` |

---

## 4. Patterns to mirror

| Concern | Source | Pattern |
|---|---|---|
| OAuth CSRF/PKCE state | `api/internal/social/state_store.go` | In-memory single-use state+bind pair, TTL-expired, cookie-bound |
| Token refresh + encrypted storage | `api/internal/social/worker.go`, `UpdateAccountToken` | Encrypt via `platform/crypto`; refresh before expiry, never on every request |
| Publish/retry worker | `api/internal/social/worker.go` | `ListDuePosts`/`SetNextRetry` shape; poll, don't wait for a webhook (SoundCloud has none) |
| Bounded external client | `api/internal/artwork` (`artworkClient`) | Timeout, no redirects, response-size cap |
| Read-only import panel (frontend) | `apps/dj/app/components/epk/EpkRaImportPanel.vue` (or Spotify equivalent once built) | Same panel shape for the SoundCloud EPK import panel |
| Feature flag wiring | `FEATURE_RA_IMPORT` end to end | Copy exactly: `config.Features` field → `Enabled()` map → `apps/dj/app/utils/features.ts` entry → `nuxt.config.ts` → `useFeatures()` gate |
| Sanitized transport errors | `api/internal/social/sanitize.go` | Never leak SoundCloud's raw error body or tokens into API responses or logs |

---

## 5. Feature scope by slice (recommended build order)

### 5.1 OAuth foundation + EPK import — Size M
- `internal/soundcloud/client.go`, `oauth.go`, `model.go`, `repository.go` (new migration for the `soundcloud_connection` singleton, using the next available migration number).
- Connect/callback/disconnect/status routes, gated by `FEATURE_SOUNDCLOUD_IMPORT`.
- `GET /me` → EPK bio/avatar fields; `GET /me/tracks` (paginated via `linked_partitioning`) → track list for the EPK.
- Frontend: a SoundCloud import panel on `/epk`, mirroring the RA import panel's states (loading/empty/error/selected/result).
- Validation: unit tests for the PKCE flow and token refresh (mock the token endpoint), a contract test for the import handler, `docs/EDITIONS.md` + `docs/CONFIGURATION.md` updated with `SOUNDCLOUD_CLIENT_ID`/`SOUNDCLOUD_CLIENT_SECRET` and the new feature row.

### 5.2 Streaming stats — Size S
- Reuses 5.1's connection. Aggregate `playback_count`/`favoritings_count` across `GET /me/tracks`, follower count from `GET /me/followers`.
- Frontend: a dashboard widget (mirrors the existing `TheStatusBar` quick-preview pattern: label over value, real data only, no placeholder metrics).
- Cache the aggregate rather than refetching on every dashboard load — SoundCloud's per-request cost and the lack of a webhook mean this should poll on an interval (e.g. once per hour), not on every page view.

### 5.3 Playlist-as-tracklist import — Size M
- **Blocked on resolving §1.5** with the user first.
- If confirmed as playlist import: `GET /me/playlists` → picker UI → `GET /playlists/{urn}` → map each track to the existing tracklist row shape (title, artist from SoundCloud's flat `title` string — SoundCloud tracks don't reliably separate "artist" and "title" the way a 1001Tracklists export does, so this needs a parsing heuristic or a manual review step before saving, matching the existing tracklist review-table UX).

### 5.4 Scheduled publishing — Size L
- **Do not start until the SoundCloud API Terms of Use and attribution/branding requirements have been read by a human** (rule #10 of the original brief) — this is the slice most exposed to ToS terms neither this document nor the fetched pages covered.
- Upload pipeline: audio file already lives in Garage (uploaded via the existing tracklist/EPK upload path); the worker streams it to `POST /tracks` as multipart, up to the 4GB/24h-audio limit.
- Worker mirrors `social.Worker`: due-job polling, exponential backoff on `429`/`503`/`504`, encrypted token refresh before each run, sanitized error surfacing.
- Frontend: scheduling UI mirrors the existing Instagram scheduler's calendar view and retry/failure states, added as a SoundCloud tab or unified into the existing Social page if the UX reads better that way — a UX decision to make at implementation time, not here.

---

## 6. Open questions (need a decision before implementation starts)

- **§1.5**: does "tracklist source" mean playlist-as-tracklist (buildable) or cue-point extraction from one mix (not possible via this API)?
- Confirmed: **two feature flags** (`IMPORT`, `PUBLISH`), not one — flag if this reads wrong once the UX is mocked up.
- Who reads the SoundCloud API Terms of Use for §5.4 before that slice starts, and when?
- Does the "artist" field on `POST /tracks` need to be populated from the DJ's settings/profile, or left to SoundCloud's own account-level default?

## 7. Risks

| Risk | Mitigation |
|---|---|
| Refresh-token single-use semantics: a crash between "receive new refresh token" and "persist it" permanently breaks that connection | Persist the new refresh token in the same DB transaction as any other state change from the refresh call; never log it |
| Self-hosters without an Artist Pro subscription can't register an app at all | Document the prerequisite prominently in `docs/EDITIONS.md` and `docs/CONFIGURATION.md`, same as Instagram's Meta-app prerequisite |
| No webhooks — a publish job's true status can only be learned by polling `GET /tracks/{urn}` | Worker polls on a backoff schedule; UI shows "publishing" rather than assuming success from the initial `POST` response |
| `favoritings_count` naming (not `likes_count`) causes a silent mismatch if code assumes Instagram-style naming | Use the field names exactly as documented in §1.4, verified against the live OpenAPI spec at implementation time, not this document alone |
| ToS/attribution requirements unmet at ship time for the publish slice | Explicit gate in §5.4: do not build until a human has read the terms |
