# Plan: KlubHub DJ — Spotify integration (Phase 4.8 stays free; personal data + playlist sync is a new licensed feature)

**Source**: user request + Spotify Web API research (2026-09-27)
**Selected scope**: personal listening data (saved tracks, playlists, top tracks/artists, recently played) and playlist creation/sync from a tracklist — both licensed/SaaS-gated. The existing Phase 4.8 (public catalog import + audio features, Client Credentials only) stays free/OSS and unchanged.
**Complexity**: Large for the new licensed feature; Phase 4.8 itself is unaffected in scope but has a newly-discovered blocker (see §1.4)
**Status**: Not started — this document records the research and design; no code has been written yet

---

## Summary

`.planning/ROADMAP.md` already documents Phase 4.8 (Spotify Integration, "DOCUMENTED FOR LATER," not started): public artist/album/track catalog data and audio features for tracklist images, using Client Credentials (app-level auth, no DJ login, no per-user OAuth). It is not gated as licensed anywhere today.

This plan adds a **second, separate, licensed feature** on top of that — importing the DJ's *own* Spotify listening data (needs the DJ's login) and syncing a KlubHub tracklist to a real Spotify playlist (needs write access). Phase 4.8 is left as-is, free, and Client-Credentials-only; nothing here changes it, except for one finding below that affects Phase 4.8's own feasibility and needs surfacing regardless of today's paid-tier scope.

**Two things came out of this research that change the picture materially, not just fill in detail:**

1. **`/audio-features` and `/audio-analysis` are both marked deprecated** on Spotify's own reference pages. Phase 4.8's key deliverable — "tempo, energy, danceability, valence, key, loudness for image generation" — depends entirely on these two endpoints. This is a risk to a plan that already exists in this repo, independent of the new licensed feature; it should be checked (and Phase 4.8 revised or descoped) before anyone starts that phase, not discovered mid-implementation.
2. **Spotify apps in Development Mode are capped at 5 authenticated users, each manually allowlisted by the developer**, and Extended Quota Mode (unlimited users) requires a registered organisation with **250,000+ monthly active users** to even apply (individuals have been ineligible since May 2025). This is a non-issue for self-hosted deployments (one DJ, one app registration, well under the cap) but is a hard blocker for a hosted KlubHub DJ Cloud SaaS edition serving many DJs' Spotify connections through one shared app — that edition cannot offer this feature to more than 5 total connected accounts until it has an enormous user base. This needs to be a known, documented limitation of the SaaS edition, not something discovered at launch.

---

## 1. Research: the Spotify Web API (verified 2026-09-27)

Sources: the Web API PKCE tutorial, the token-refresh tutorial, the scopes reference, the quota-modes concept page, individual endpoint reference pages (checked directly for deprecation banners, since a general overview page did *not* surface the deprecations that the endpoints' own pages show), and the Spotify Developer Terms. Per the user's explicit instruction, treat none of this as permanent — re-verify against the live OpenAPI spec (`https://developer.spotify.com/reference/web-api/open-api-schema.yaml`) at implementation time.

### 1.1 Auth: Authorization Code with PKCE

- **Authorize**: `https://accounts.spotify.com/authorize` — `response_type=code`, `client_id`, `redirect_uri` (must match registration exactly), `code_challenge_method=S256`, `code_challenge`, optional `scope` (space-separated) and `state`.
- **Code verifier**: 43-128 char high-entropy random string (letters/digits/`_`/`.`/`-`/`~`). **Code challenge**: SHA-256 of the verifier, base64url-encoded (no padding, `+`→`-`, `/`→`_`).
- **Token exchange**: `POST https://accounts.spotify.com/api/token`, `Content-Type: application/x-www-form-urlencoded`, body `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `code_verifier`. **No `client_secret` is required or sent for PKCE** — this is a real difference from both the SoundCloud integration (`.claude/plans/soundcloud-integration.plan.md`, which does need a secret even with PKCE) and the existing Instagram flow. A pure-PKCE client never needs to hold a secret at all.
- **Refresh**: same token URL, `grant_type=refresh_token`, `refresh_token`, `client_id` (no secret, for PKCE apps). Access tokens last exactly 1 hour (`expires_in: 3600`). **Whether the refresh token itself rotates on every refresh is not confirmed by the docs fetched** — treat this defensively: always persist whatever `refresh_token` comes back if one is present, keep the existing one if the response omits it. A failed/expired/revoked refresh token returns `invalid_grant` with no further distinction — on that error, the DJ must go through the authorization flow again; there is no automatic recovery.
- **Redirect URIs must be HTTPS**, except `http://127.0.0.1` (not `localhost`) for local development, and no wildcards — this is already in the user's brief and matches Spotify's own docs.

### 1.2 Quota modes — the SaaS-scale blocker

- **Development Mode**: up to 5 authenticated users, each requiring the app owner to manually allowlist them by email in the Spotify dashboard, and the app owner must hold a Spotify Premium account. This is exactly the shape of a **self-hosted** deployment (the DJ registers their own app, allowlists their own account, done) — fine.
- **Extended Quota Mode**: unlimited users, higher rate limits, no allowlist. Requires: a legally registered business/organisation (not an individual, as of 2025-05-15), an active launched service, and **at least 250,000 monthly active users**, plus demonstrated commercial viability and Terms compliance.
- **Consequence**: this feature can ship for self-hosted and licensed-self-hosted operators without issue. It **cannot** meaningfully ship on a hosted KlubHub DJ Cloud SaaS edition serving many DJs under one shared Spotify app registration until that edition has an enormous user base — a scale KlubHub is nowhere near today. Document this as a known SaaS-edition limitation up front rather than promising it.

### 1.3 Rate limits and errors

- `429 Too Many Requests` — respect the `Retry-After` header exactly; exponential backoff without a tight retry loop (per the user's own rule). No specific numeric quota is published for authenticated per-app rate limits (Spotify's docs describe an internal rolling-window algorithm, not a fixed number) — the client must be defensive (backoff on any 429) rather than pre-computing a budget.
- Standard HTTP error codes apply; read the response body's error object for a user-facing message rather than a generic failure string, per the user's rule.

### 1.4 Deprecated endpoints (confirmed directly on their own reference pages — a general API-overview page did *not* show these, so check each endpoint's own page, not just an index)

| Deprecated | Use instead |
|---|---|
| `GET /audio-features/{id}` and the several-tracks variant | **No replacement documented.** This affects Phase 4.8 directly (§ Summary). |
| `GET /audio-analysis/{id}` | **No replacement documented.** Same effect on Phase 4.8. |
| `GET /playlists/{playlist_id}/tracks` | `GET /playlists/{playlist_id}/items` |
| Type-specific library endpoints (implied by the user's own rule: "use `/me/library` over the type-specific library endpoints") | `/me/library` — **not independently verified in this pass; re-check against the live spec, since the fetched pages didn't confirm this endpoint's exact shape.** |

### 1.5 Scopes (exact strings, verified)

| Need | Scope |
|---|---|
| Saved/library tracks | `user-library-read` |
| Read private playlists | `playlist-read-private` |
| Read collaborative playlists | `playlist-read-collaborative` |
| Create/modify public playlists | `playlist-modify-public` |
| Create/modify private playlists | `playlist-modify-private` |
| Top tracks/artists | `user-top-read` |
| Recently played | `user-read-recently-played` |

Request only the scopes each enabled sub-feature actually needs, per the user's rule — a DJ who only wants personal-data import should never see a playlist-write consent screen.

### 1.6 Developer Terms constraints that shape the design, not just compliance boilerplate

- **No indefinite storage of Spotify content.** "Reasonable efforts" to keep displayed data current and to delete older data. This means personal-data import (saved tracks, top tracks, recently played) cannot be treated like RA import's bio text — a one-time snapshot saved permanently into the EPK. It needs either a refresh-on-view model or a TTL with a background refresh/expiry job.
- **Delete a disconnected user's data within 5 days.** This repo already has exactly this shape of job — Promoter's guest-data retention purge (`.claude/plans/promoter-p2-guests-door.plan.md` §P2.5) — mirror that pattern (a scheduled purge keyed off `disconnected_at`) rather than inventing a new one.
- **No ML training on Spotify content** — not applicable to anything planned here, but worth a one-line acknowledgment in the eventual PR description since it's an explicit term.
- Attribution: Spotify's own marks may be used "solely to promote your use... of the Spotify Platform" — no mandatory logo requirement was found, but re-check the Branding Guidelines (linked from the Terms) before shipping any UI that shows Spotify data, since brand-mark misuse is the kind of thing that gets an app's API access revoked with little warning.

---

## 2. Codebase facts that shape the plan

- **DJ has no user-account model** (same fact as the SoundCloud plan). A personal Spotify connection is one singleton record scoped to the implicit single DJ user, not a per-user table — same shape as the existing Instagram integration's token storage.
- **The OAuth CSRF-state pattern already exists**: `api/internal/social/state_store.go` (single-use, TTL-expired, cookie-bound `state`/`bind` pair). Reuse it; it will need the same small PKCE extension noted in the SoundCloud plan (holding `code_verifier` across the redirect) — if that extension gets built for SoundCloud first, reuse it here rather than building it twice.
- **Feature-flag mechanism is fully built** (`docs/EDITIONS.md`, `apps/dj/app/utils/features.ts`, `config.Features` in Go) — RA import is the template. Copy its shape.
- **Retention/purge precedent already exists** in the Promoter codebase (P2.5) for exactly the "must delete within N days" ToS shape this feature also needs — read that implementation before writing a new purge job from scratch.
- **Bounded external HTTP client precedent**: `api/internal/artwork`'s `artworkClient` (timeout, no redirects, size cap) — extend this pattern for the Spotify client, consistent with how Phase 4.8's own (separate, Client-Credentials) Spotify client should also be built.
- **No existing Spotify code of any kind** in this repo yet (checked: no `*Spotify*` files under `apps/dj` or `api`) — Phase 4.8 is genuinely unstarted, not partially scaffolded. `.planning/REQUIREMENTS.md` does not currently define `SPOT-01` through `SPOT-04` (only `.planning/ROADMAP.md` references those IDs) — that gap should be closed when Phase 4.8 is actually planned, separately from this document.
- **"Bring your own keys" convention**: follow `SPOTIFY_CLIENT_ID` (already reserved by Phase 4.8's own plan for the Client Credentials flow) — the new licensed feature's PKCE flow needs no client secret at all (§1.1), so no new secret-shaped env var is needed for it, only the existing `SPOTIFY_CLIENT_ID`, plus whatever redirect URI configuration the deployment needs (mirrors `INSTAGRAM_CLIENT_ID`'s existing redirect-URI handling in `docs/CONFIGURATION.md`).
- **Migrations**: check the actual latest number in `api/internal/platform/migrations/` at implementation time — do not assume a fixed number; Phase 5 (finance entries) and any SoundCloud work may land first.

---

## 3. Proposed architecture

```
apps/dj (Nuxt) ── EPK page (personal-data import panel; separate from Phase 4.8's public-catalog panel)
              ── Dashboard (top tracks / recently played widget)
              ── Tracklist page (create-or-sync-to-Spotify-playlist action)
                    │  /api/v1/**
                    ▼
api/cmd/api (DJ binary)
   └─ internal/spotify/            (NEW — the licensed, per-user-OAuth feature; separate from Phase 4.8's future Client-Credentials package)
         ├─ client.go              (bounded HTTP client; PKCE auth URL builder, token exchange/refresh, 429+Retry-After backoff)
         ├─ oauth.go               (connect/callback/disconnect; reuses social.StateStore + its PKCE extension)
         ├─ model.go               (Connection {access_token_enc, refresh_token_enc, expires_at, scopes, connected_at, disconnected_at})
         ├─ repository.go
         ├─ service.go             (personal-data fetch with TTL/refresh-on-view; playlist create/sync)
         ├─ handler.go
         └─ retention.go           (purge job: delete cached personal data + tokens within 5 days of disconnect — mirrors Promoter P2.5)
```

Phase 4.8's own package (`internal/spotify_catalog` or similar — naming TBD when that phase is actually planned) stays separate: different auth model (Client Credentials, no user consent), different data class (public catalog, no retention/deletion obligation), different edition (free). Do not merge the two into one package just because they share a provider name.

### Feature flags

- **`FEATURE_SPOTIFY_PERSONAL`** — gates the OAuth connect flow, EPK personal-data import, and the dashboard widget. Read-only.
- **`FEATURE_SPOTIFY_PLAYLIST_SYNC`** — gates playlist creation/sync (write scope). Implies `FEATURE_SPOTIFY_PERSONAL` is also on, same rule as the SoundCloud plan's `PUBLISH` flag depending on `IMPORT`.
- Phase 4.8's catalog+audio-features feature, if it ships, stays **unflagged** (free/OSS) per this session's decision — do not fold it into either flag above.

### API routes (all 404 when their flag is off)

| Route | Flag |
|---|---|
| `POST /api/v1/spotify/connect`, `GET /api/v1/spotify/callback`, `POST /api/v1/spotify/disconnect`, `GET /api/v1/spotify/status` | `PERSONAL` |
| `GET /api/v1/spotify/me/top`, `GET /api/v1/spotify/me/recently-played`, `GET /api/v1/spotify/me/library`, `POST /api/v1/epk/import-spotify-personal` | `PERSONAL` |
| `POST /api/v1/tracklist/{id}/sync-spotify-playlist` | `PLAYLIST_SYNC` |

---

## 4. Patterns to mirror

| Concern | Source | Pattern |
|---|---|---|
| OAuth CSRF/PKCE state | `api/internal/social/state_store.go` | Same store as the SoundCloud plan proposes reusing/extending — build the PKCE extension once, use it for both providers |
| Token refresh + encrypted storage | `api/internal/social/worker.go` | Encrypt via `platform/crypto`; refresh proactively before the 1h expiry |
| Data retention / deletion-on-disconnect | Promoter P2.5 retention/purge job | A scheduled purge keyed off `disconnected_at`, not a manual cleanup step |
| Bounded external client | `api/internal/artwork` (`artworkClient`) | Timeout, no redirects, size cap; add `Retry-After`-aware backoff on top for Spotify's 429s specifically |
| Feature flag wiring | `FEATURE_RA_IMPORT` end to end | Same five-step wiring (Go config → `Enabled()` → TS registry → `nuxt.config.ts` → `useFeatures()` gate) |
| Sanitized transport errors | `api/internal/social/sanitize.go` | Never leak tokens or raw Spotify error bodies into logs/responses |

---

## 5. Feature scope by slice (recommended build order)

### 5.1 OAuth foundation + personal-data import — Size M
- `internal/spotify/client.go`, `oauth.go`, `model.go`, `repository.go`, a new migration for the `spotify_connection` singleton.
- Connect/callback/disconnect/status, gated by `FEATURE_SPOTIFY_PERSONAL`.
- `GET /me/top/tracks`, `GET /me/top/artists`, `GET /me/player/recently-played`, `GET /me/library` (verify this last one's exact shape against the live spec — see §1.4) with the scopes from §1.5 requested individually per capability, not all at once.
- Refresh-on-view or short-TTL caching for imported data, not permanent storage (§1.6) — implement the retention/expiry behavior in this slice, not as an afterthought.
- Validation: unit tests for PKCE generation and the token refresh path (mock the token endpoint; assert defensive refresh-token persistence per §1.1), a contract test for each import handler, `docs/EDITIONS.md` + `docs/CONFIGURATION.md` updated.

### 5.2 Dashboard widget — Size S
- Reuses 5.1. Top tracks/artists or recently-played surfaced on the dashboard, matching `TheStatusBar`'s real-data-only convention.

### 5.3 Playlist creation/sync from a tracklist — Size M-L
- Gated by `FEATURE_SPOTIFY_PLAYLIST_SYNC`. `playlist-modify-public`/`playlist-modify-private` scopes requested only when this flag is on.
- Create endpoint and add-items endpoint need re-verification against the live spec at implementation time (§1.4 — the exact non-deprecated "add items" path wasn't confirmed in this pass, only that a non-deprecated version exists distinct from the bracketed `[DEPRECATED]` one in Spotify's reference index).
- Matching KlubHub tracklist rows to Spotify tracks needs a search/match step (title+artist → Spotify track search) with a manual review UI for ambiguous matches — this is new logic, not something to reuse from RA/Instagram.

### 5.4 (Separate, not this plan) Phase 4.8 itself
- Out of scope here by the user's own decision (stays free/OSS, unchanged), but **flag its `/audio-features`/`/audio-analysis` dependency (§1.4) to whoever plans that phase** before work starts on it — this document is the first place that dependency's deprecated status was checked.

---

## 6. Open questions

- Exact shape of `/me/library` (the user's own rule says prefer it over type-specific library endpoints, but this pass didn't confirm its response shape) — verify against the live spec before 5.1 starts.
- Exact non-deprecated "add items to playlist" endpoint path/verb — confirmed to exist, not confirmed in detail; verify before 5.3 starts.
- Whether Spotify refresh tokens rotate on every use (§1.1) — undocumented in the pages fetched; implement defensively regardless, but worth a direct test against a real sandbox app before relying on either behavior.
- How prominently to disclose the SaaS-edition 5-user cap (§1.2) — a docs decision, not a technical one, but it should exist somewhere before this feature is marketed on klubhub.io the way Promoter's "self-hosting preview" status is disclosed today.

## 7. Risks

| Risk | Mitigation |
|---|---|
| Phase 4.8 built on deprecated `/audio-features`/`/audio-analysis` with no confirmed replacement | Re-verify against the live OpenAPI spec before Phase 4.8 starts; may need to descope the audio-features image-suggestion deliverable entirely if truly unavailable to new apps |
| SaaS edition cannot scale personal-data/playlist-sync past 5 connected accounts without Extended Quota Mode (250k MAU, org-only) | Document as a known SaaS-edition limitation; self-hosted and licensed-self-hosted are unaffected |
| Storing personal data (top tracks, recently played) indefinitely violates the Developer Terms | Refresh-on-view or TTL cache, not permanent storage; retention/purge job on disconnect within 5 days, mirroring Promoter's existing pattern |
| Token refresh defensiveness: assuming rotation behavior that turns out wrong | Always persist a returned `refresh_token`; never assume the old one still works if a new one arrives; test against a real app before shipping |
| Brand-mark misuse in the UI (no mandatory logo found, but Branding Guidelines exist and weren't fully reviewed here) | Read the Branding Guidelines before shipping any UI surfacing Spotify data |
