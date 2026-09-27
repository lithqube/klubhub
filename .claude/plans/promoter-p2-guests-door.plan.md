# Plan: KlubHub Promoter P2 — guest lists, attendee import, offline door

**Source plan**: `.claude/plans/promoter-app.plan.md` §5 (P2), adjusted by §12 (D2: SaaS-only public pages) and §13.4 (data classes)
**Branch**: `feat/promoter-p2`
**Complexity**: Large — delivered in slices, one commit (or more) each

## What D2 changes for P2

Self-hosted Promoter has no public surface, so the P2 items that need one are SaaS-only and are **not built here**:

| Master-plan item | OSS P2 (this plan) | SaaS / later |
|---|---|---|
| RSVP / free-ticket checkout, confirmation email, QR ticket page, 1 d / 1 h reminders | External ticket link (P1) + **attendee import** into the same pretix-shaped `orders` / `order_positions` | Native checkout via `publicedge` |
| Secret submission links for allocations | Staff enter or paste names per allocation; CSV import | `/l/:token` links; file exchange with the DJ app in P4 |
| Waitlist, registration questions | Guest status `waitlist`; imported answers kept as internal notes | Native flows |

Everything else in master-plan P2 is in scope.

## Scope

| Slice | Delivers |
|---|---|
| P2.1 Guest lists & guest table | Lists per event (types artist / promoter / comp / industry / VIP / reduced / crew), `entry_terms` (free / reduced price, **cutoff time**, perk tags), allocations per submitter (quota, +N per guest, deadline, needs approval, revoke), **standing lists** copied into each new event. Guests with status Going / Pending / Waitlist / Invited / Declined (+ derived Checked-in). Guest table: status tabs with counts, search, list filter, bulk status by pasting emails, "add directly as Going", CSV export |
| P2.2 Attendee import | Pretix-shaped `ticket_types`, `orders`, `order_positions` (with ticket secret) for attendees bought elsewhere. CSV import with column mapping presets for RA, DICE, Shotgun, pretix, Luma and generic; dry-run preview; dedupe by email blind index; re-import updates instead of duplicating |
| P2.3 Offline door | `/door` for the event-scoped `door` session: **downloads the whole list** (guests + ticket positions + entry terms) into IndexedDB, encrypted at rest on the device; search from the first letter, accent- and typo-tolerant; QR scan (Express = scan checks in, Standard = scan opens the card); partial arrivals of +N; one-tap undo toast; **PAST CUTOFF** state; in/out occupancy counter with walk-up button; on-the-spot adds gated by a manager PIN; local check-in queue with sync badge; check-ins **idempotent by client nonce**; conflicts (same guest checked in on two devices) flagged on sync; forced dark theme, large touch targets |
| P2.4 Post-event report | Attendance by list and submitter, no-show rate, +1s used, check-in curve (15-min buckets), walk-ups, "list back" CSV per artist allocation |
| P2.5 Privacy & retention | Name-only by default, contact fields opt-in per list; **retention job** purges guest personal data 30 days after the event (org setting), keeping anonymised counts so reports survive; no ID images anywhere |
| P2.6 Sealed tier + ban list | P0 task 12: browser X25519 member keys wrapped with an Argon2id passphrase key, org sealed key wrapped to members, **mandatory recovery kit**, re-wrap on member removal, door-device provisioning. Ban list entries (name, optional email, reason, expiry) sealed; the door matches locally and shows a quiet warning |

Order: P2.1 → P2.2 → P2.3 → P2.4 → P2.5 → P2.6. P2.3 needs P2.1/P2.2 data; P2.6 lands last because it is the largest crypto change and nothing else depends on it (the door's ban-list hook is a no-op until then).

## Data model (migrations 00008–00011)

All tables: `tenant_id`, ENABLE + FORCE RLS, fail-closed policy, table and column `data_class` comments (existing guards enforce).

**00008 guest lists** (P2.1)
- `guest_lists` (internal): event_id, name, type, entry_terms (jsonb: price_mode free/reduced, reduced_price_text, cutoff_at timestamptz, perks text[]), collect_contact bool, standing_template_id, position.
- `standing_lists` (internal): org-level templates with the same shape; copied on event create.
- `guest_allocations` (internal + personal): list_id, submitter label (public: artist/stage name, reviewed exception as in lineup), `submitter_contact_enc`, quota, plus_n_max, deadline, requires_approval, revoked_at.
- `guests` (personal): list_id, allocation_id, `name_enc`, `email_enc`, `phone_enc`, `note_enc`, `email_bidx`, `name_bidx` (normalised tokens, for exact lookup and dedupe), plus_n, status, source (manual / paste / import / door), created_by.

**00009 attendees** (P2.2)
- `ticket_types` (internal): event_id, name, capacity, external_ref.
- `orders` (personal): event_id, source platform, external_ref, `buyer_name_enc`, `buyer_email_enc`, `buyer_email_bidx`, imported_at.
- `order_positions` (personal): order_id, ticket_type_id, `attendee_name_enc`, `attendee_email_bidx`, `secret_bidx` (QR lookup; the secret itself is sealed), status.

**00010 check-ins** (P2.3)
- `checkins` (personal): event_id, subject (guest or position), count (partial +N), direction in/out, client_nonce **UNIQUE per tenant**, device_id, at (device clock) and received_at (server), undone_at, conflict_of.
- `door_counters` (internal): walk-ups and manual in/out adjustments per device, nonce-idempotent.
- `door_pins` gains a `manager` flag (on-the-spot adds need a manager PIN).

**00011 sealed tier** (P2.6)
- `member_keys` (internal): user_id, public key, `wrapped_private` (ciphertext from the browser), kdf params.
- `org_sealed_keys` (internal): version, per-member and per-device wrapped copies, recovery-kit fingerprint.
- `ban_entries` (sealed): ciphertext blob + expiry (plaintext, for purge) + key version. The server cannot read or match them.

## API (all through `authz.Engine.Handle`; actions already exist in `authz.rego`)

| Route | Action |
|---|---|
| `GET/POST /api/v1/events/{eventID}/lists`, `PUT/DELETE .../lists/{listID}` | guestlist.read / guestlist.write |
| `GET/POST /api/v1/standing-lists`, `PUT/DELETE .../{id}` | guestlist.read / guestlist.write |
| `GET/POST .../lists/{listID}/allocations`, `PUT/DELETE .../allocations/{id}` | guestlist.read / guestlist.write |
| `GET /api/v1/events/{eventID}/guests` (status, list, q filters; counts) | guestlist.read |
| `POST .../guests`, `PUT/DELETE .../guests/{guestID}`, `POST .../guests/bulk-status` | guestlist.write |
| `GET .../guests/export.csv` | guestlist.read |
| `POST /api/v1/events/{eventID}/attendees/import?dry_run=` | guestlist.write |
| `GET /api/v1/door/bundle` (session event only) | door.read |
| `POST /api/v1/door/checkins` (batch, nonce-idempotent) | door.checkin |
| `POST /api/v1/door/adds` (needs manager-PIN session) | door.checkin |
| `GET /api/v1/events/{eventID}/report`, `.../report/list-back.csv` | guestlist.read |
| `GET/PUT /api/v1/org/retention` | org.update |
| `GET/PUT /api/v1/keys/*`, `GET/PUT /api/v1/ban-list` | security.manage / guestlist.write |

Door routes read the event from the `door` principal, never from the URL, so a door session cannot reach another event.

Events emitted (IDs and counts only, never personal data): `guestlist.updated`, `guest.status_changed`, `attendees.imported`, `door.checkins_synced`, `retention.purged`.

## Frontend

- Event page gains a **Guests** tab (`/events/[id]/guests`): list manager (lists, entry terms, allocations with quota bars) and the guest table.
- `/guests`: cross-event overview (upcoming events with list fill and pending approvals) + standing lists.
- `/door`: PIN login → bundle download → offline search / scan / check-in. A separate route tree with its own layout (no app shell), forced dark theme, `display: standalone` manifest. The service worker caches the door shell only.
- Settings: retention period, ban list (P2.6), member key setup and recovery kit (P2.6).
- Stores: `useGuestStore`, `useDoorStore` (IndexedDB via a small wrapper, no new dependency unless `idb` is justified); mock handlers under `apps/promoter/server/api/v1/` for every new route (repo convention).

## Door device security

- The bundle holds decrypted personal data, so the device cache is **encrypted at rest** with a non-extractable WebCrypto AES-GCM key created per door session and kept in IndexedDB. Session expiry, logout or event end wipes both.
- The bundle is scoped to one event, only served to a registered device with a valid PIN session, and audited (`door.bundle_downloaded` with a count).
- The ban list reaches the device only as sealed ciphertext plus the device-wrapped org key (P2.6).

## Validation

Per slice: `go test -race` (domain table tests, testcontainers Postgres integration, RLS + data-class guards, route coverage), Rego tests for any new deny rules, Vitest (search normalisation, CSV mapping, queue/sync reducer, crypto round-trips), vue-tsc, lint, Playwright e2e (guest table; door offline flow with the network cut in the browser context), and a built-in browser check at desktop and 375 px.

## Risks

| Risk | Mitigation |
|---|---|
| Personal data at rest on door phones | Encrypted cache with a non-extractable key; wipe on expiry/logout/event end; name-only lists by default |
| Double entry across offline devices | Client nonces; server keeps both rows and flags `conflict_of`; door shows the flag after sync |
| Search over encrypted names | Search happens on the device after download (lists are small); the server only does exact blind-index lookups |
| CSV formats drift (RA, DICE, Shotgun…) | Mapping presets with a generic fallback and a dry-run preview; fixtures per platform in tests |
| Retention deleting data needed for reports | Report counts are materialised into `internal` aggregates before purge |
| Sealed-tier key loss locks an org out of its ban list | Mandatory recovery kit at setup; ban list is the only sealed data in P2 |
| Scope creep into SaaS flows | D2 table above; `publicedge` stays unmounted |

## Decisions taken as defaults (revisit if needed)

- **P2-D1** RSVP checkout, reminders and submission links are SaaS-only; OSS imports attendees.
- **P2-D2** Door search runs on the device; no server-side fuzzy search over personal data.
- **P2-D3** Retention default 30 days after the event end, configurable per org (min 1, max 365).
- **P2-D4** The sealed tier ships in P2.6 with the ban list, as ADR 0001 planned.

## Status (2026-09-27)

| Slice | State | Commits |
|---|---|---|
| P2.1 Guest lists & guest table | Done | `3b8b018` (API, migration 00008), `b0aa9e2` (guest table, /guests, mocks, e2e) |
| P2.2 Attendee import | Done | `67acbac` (API, migration 00009, presets), next commit (import panel, ticket badges, mocks, e2e) |
| P2.3 Offline door | Done | `2c97f57` (API, migration 00010, manager PIN), next commit (`/door`, event Door tab, encrypted cache, service worker, jsQR, mocks, e2e) |
| P2.4 Post-event report | Done | `5ed04eb` (report API, list-back CSV), `e851b9e` (REPORT tab, curve, mocks, e2e) |
| P2.5 Privacy & retention | Not started | — |
| P2.6 Sealed tier + ban list | Not started | — |

### P2.1 decisions (implementation)

- **Quota counts heads** (guest + N) of going, pending and invited guests; waitlisted and declined guests hold none. Lowering a quota below the heads already on it is refused.
- **Standing lists carry a local cutoff** (`entry_terms.cutoff_local`, "HH:MM"); on copy it becomes `cutoff_at` = the first occurrence of that time at or after 12 h before the event start, in the event's timezone. Event lists only store `cutoff_at`. Copies are independent of later template edits.
- **Standing lists are copied through `event.Service.OnCreate`** (a hook run inside the event-creation transaction), so `event` does not import `guest`.
- **Duplicates are skipped, not rejected:** the same email anywhere in the event, or the same normalised name on the same list (name-only guests), is reported back by input index.
- **Explicit status wins over approval:** "add directly as Going" sends `status: going`; without it, allocations that need approval add guests as pending.
- **Revoke is `DELETE …/allocations/{id}`** and keeps the allocation's guests; deleting a list with guests needs `?force=true`.
- **Turning `collect_contact` off erases** stored emails and phones on that list (notes stay).
- **Extra route:** `GET /api/v1/guests/overview` (guestlist.read) feeds the cross-event page, instead of one request per event.
- **CSV export** is audited (`guestlist.export`), starts with a UTF-8 BOM, and prefixes cells starting with `= + - @`, tab or CR with an apostrophe.
- **Blind indexes use the current DEK version.** After a key rotation, lookups by old index values miss until guests are re-indexed; P2.5 or the rotation job must re-index (noted, not built).

### P2.3 decisions (implementation)

- **Conflicts:** an `in` is flagged only when another device already admitted the subject and in − out across all devices would exceed the allowance; re-entry after an `out` is not a conflict, and one device over-admitting is "admit anyway". `conflict_of` points at the earliest live `in` of another device.
- **Cursor:** each bundle/sync takes a per-event advisory lock and stamps rows with the database clock read after it, so no later commit is skipped. Deltas include rows whose `received_at` **or** `undone_at` is after the cursor, so undos reach every device.
- **Idempotency:** nonces are unique per tenant across check-ins, counters and undos; a repeat is `duplicate` with the original conflict flag; rejected ops are not stored and the door drops them from its queue after showing them. Undoing an undone row is a no-op `applied`.
- **Rejection codes:** `invalid_op: …` / `invalid_add: …` (match by prefix), `unknown_subject`, `unknown_target`, `unknown_list`, `id_conflict`, `manager_pin_invalid` (also missing, expired or locked), `event_full`.
- **Outbox events carry identifiers only** (`events.New` takes UUID refs); counts go to the audit entry in the same transaction. Door adds also emit `guestlist.updated` and audit `door.guests_added`.
- **The server accepts any subject status**; the door blocks cancelled/refunded tickets and asks for a second tap for not-going guests, past cutoff and used-up allowance. Express mode checks in straight from a scan only when there is nothing to warn about.
- **Manager PIN:** each distinct PIN in an add batch is verified once (one typo costs one attempt). Staff and manager hashes are sealed under different column labels. The offline PBKDF2 verifier of a 6-digit PIN is brute-forceable from an unlocked door device — accepted, since the device already holds the list and the server re-checks every add. With Zitadel identity there is no manager PIN source yet, so adds are rejected.
- **Session interplay:** door login sets the same session cookie as staff login, so it signs the admin out on that browser (the Door tab warns; use a separate phone or profile). A reload reopens the door without the PIN until the session expires (needed offline); queued ops survive a server-side session end and sync after a new PIN login for the same event.
- **QR:** `BarcodeDetector` where available, otherwise **jsQR** (`jsqr` ^1.4.0, lazy chunk ≈ 130 KB / 46 KB gzip, prefetched on browsers without `BarcodeDetector` so it works offline); typed code always available. Camera needs HTTPS or localhost (so does WebCrypto); `camera=(self)` only on `/door`.
- **`/door` always loads as a full document** (camera permission and service worker scope apply that way); its theme script forces dark before paint. The service worker is told about already-loaded assets after registering so they are cached too.
- **Default PIN window:** event end + 6 h, capped at 36 h from now, at least 1 h ahead. The device cache is wiped on logout, session expiry, or 6 h after the event ends.

### P2.4 decisions (implementation)

- **`live`** is `now < ends_at`; the UI shows LIVE only once the event has started and an empty state before any activity.
- **Arrived and scanned count any status** with a live `in` (a pending guest admitted anyway, a scanned refunded ticket), while the no-show rate uses going guests only — so arrived can exceed going and scanned can exceed valid; the UI shows counts, never ratios above 100 %.
- **Heads admitted** sums every non-undone `in` (re-entry counts again); +1s used stays capped at `plus_n`.
- **Curve:** manual in/out add to `in`/`out`, occupancy includes walk-ups; each row is floored to the local :00/:15/:30/:45 (a DST fall-back night keeps two distinct repeated-hour buckets, spring-forward has no gap, :30/:45-offset zones align locally); `bucket_start` is UTC and uses the device clock; rows more than 24 h outside the event are clamped into the first/last bucket (totals unaffected); `peak_at` is the start of the first bucket reaching the peak, null when the peak is 0.
- **List-back** includes every status and revoked allocations; `arrived` is yes/no; 422 for a missing/malformed `allocation_id`, 404 for another event's; filename `<slug>-list-back-<label>.csv`; audited with counts only.
- **`by_submitter`** orders artist lists first, then list position, then creation; revoked allocations stay (muted in the UI).
- **Chart:** two panels on one time axis (occupancy line; arrivals up / exits down per bucket), keyboard readout, screen-reader table and SHOW AS TABLE; walk-ups are merged into arrivals in the chart (a third hue was indistinguishable in light mode) and broken out in the readout and table.

### UX review of P2.1–P2.3 (commit `99ea899`)

All P0/P1/P2 findings fixed except showing check-in state in the guest table (needs P2.4 aggregates; follow-up). API change: locked PIN → **429 `pin_locked`** with `Retry-After` and `retry_after` (429 rather than 423: the lock rate-limits guessing and lifts itself; the fifth wrong try already gets it), expired PIN → 401 `pin_expired`; unknown/revoked devices still only see `invalid_credentials`. PIN status gains `locked_until`. Queued ops survive session expiry (door goes to re-login instead of wiping).

### API tests

`tests/bruno-promoter` (commit `a04766e`), run with `pnpm nx run api:test-api` against a disposable Postgres + promoter API with per-run secrets. Not covered there (covered by Go integration tests): two-device conflicts, PIN lockout exhaustion, login with a TOTP code right after enrolment (replay window), step-up after 15 min.

## P2.3 contract (offline door)

Backend (`api/internal/promoter/door`, new package) and frontend build against this. JSON is snake_case; times are RFC 3339 UTC.

### Migration 00010_door.sql

- `checkins` (data_class **personal** — links a person to a time and place): `id` uuid PK, `tenant_id`, `event_id` → events, `guest_id` → guests (nullable, ON DELETE CASCADE), `position_id` → order_positions (nullable, ON DELETE CASCADE), CHECK exactly one of guest/position, `count` int 1..50, `direction` in/out, `client_nonce` text (8..64), UNIQUE (`tenant_id`, `client_nonce`), `device_id` → door_devices, `at` (device clock), `received_at` default now(), `undone_at`, `undo_nonce` text, `conflict_of` uuid → checkins. Index (`tenant_id`, `event_id`, `received_at`).
- `door_counters` (internal): `id`, `tenant_id`, `event_id`, `device_id`, `kind` walkup / in / out, `delta` int 1..50, `client_nonce` UNIQUE per tenant, `at`, `received_at`, `undone_at`.
- `door_pins`: add `manager` boolean NOT NULL DEFAULT false and `check_enc` BYTEA (sealed offline verifier, manager rows only); primary key becomes (`tenant_id`, `event_id`, `manager`). Door login only accepts the staff row (`manager = false`).
- All tables: ENABLE + FORCE RLS, fail-closed tenant policy, table and column `data_class` comments.

### Routes

The event always comes from the door principal (`EventScope`), never the URL: `authz.Route` gains `EventFromScope bool`, and the guard then sets `Resource.EventID = principal.EventScope`. Handlers answer 403 `door_session_required` when the principal has no event scope (staff cannot call door routes).

| Route | Action | Notes |
|---|---|---|
| `GET /api/v1/door/bundle` | door.read | Audited `door.bundle_downloaded` (counts only) |
| `POST /api/v1/door/checkins` | door.checkin | Batch of ≤ 500 ops, nonce-idempotent |
| `POST /api/v1/door/adds` | door.checkin | ≤ 50 on-the-spot adds, each carries the manager PIN |
| `GET /api/v1/door/devices` | door.device.manage | `[{id,label,created_at,last_seen_at,revoked_at}]` (new, staff) |
| `POST /api/v1/door/events/{eventID}/pin` | door.device.manage | Existing; body gains optional `"manager": true` |
| `GET /api/v1/door/events/{eventID}/pin` | door.device.manage | `{staff:{valid_until}\|null, manager:{valid_until}\|null}` (never the PIN) |

**Bundle** (`GET /api/v1/door/bundle`):

```json
{
  "generated_at": "…", "device_id": "uuid", "session_expires_at": "…",
  "event": {"id","title","starts_at","ends_at","doors_at","timezone","capacity"},
  "lists": [{"id","name","type","entry_terms":{"price_mode","reduced_price_text","cutoff_at","perks"}}],
  "guests": [{"id","list_id","name","plus_n","status","note"}],
  "tickets": [{"id","name","ticket_type","order_ref","source","status","secret"}],
  "checkins": [{"nonce","subject":{"kind":"guest|ticket","id"},"count","direction","at","device_id","undone","conflict"}],
  "counters": {"walkups","manual_in","manual_out"},
  "cursor": "…",
  "manager_pin": {"salt":"b64","iterations":210000,"hash":"b64"} | null
}
```

Guests exclude `declined`; no email or phone ever reaches the door (name-only). Tickets carry the decrypted secret (the QR payload) so scans match offline; cancelled/refunded tickets are included with their status so the door can say why. `manager_pin` is PBKDF2-SHA256(pin, salt, iterations) → 32 bytes, computed when the manager PIN is generated, stored sealed in `check_enc`; the device verifies offline, the server re-verifies every add against the Argon2id hash.

**Check-in sync** (`POST /api/v1/door/checkins`):

```json
{"since": "cursor|null", "ops": [
  {"nonce","type":"checkin","subject":{"kind":"guest|ticket","id"},"count":1,"direction":"in|out","at"},
  {"nonce","type":"undo","target":"nonce-of-checkin-or-counter","at"},
  {"nonce","type":"counter","kind":"walkup|in|out","delta":1,"at"}
]}
```

Response: `{"results":[{"nonce","status":"applied|duplicate|rejected","conflict":bool,"error":"…"}], "checkins":[…same shape as bundle, all devices, received after since…], "counters":{…}, "cursor":"…"}`. Ops apply in order in one transaction; a repeated nonce returns `duplicate` with the original outcome. A check-in `in` whose subject would exceed its allowance (guest: 1 + plus_n heads, ticket: 1) counting non-undone rows from **other** devices is stored anyway with `conflict_of` set to the earliest such row, and reported `conflict: true`. Subjects must belong to the session's event (else `rejected`). Undo sets `undone_at`; undoing an unknown nonce is `rejected`. Emits `door.checkins_synced` (counts only) through the outbox.

**Adds** (`POST /api/v1/door/adds`): `{"adds":[{"id":"client uuid","nonce","list_id","name","plus_n","manager_pin","at"}]}` → guest with `source: door`, `status: going`, `id` = client uuid (so the device can queue a check-in for it before sync; an existing id is `duplicate`). Wrong manager PIN → `rejected` with `manager_pin_invalid`; failed attempts share the manager row's lockout (5 / 15 min). Response `{"results":[{"nonce","id","status","error"}]}`.

### Frontend

- `/door` (`public: true`, `layout: false`, bare): device not prepared → explains to prepare it from the event's Door tab; prepared → event title + PIN pad → `POST /api/v1/door/login` → bundle download → door UI. Forced dark theme, targets ≥ 56 px.
- Device preparation lives on a new event tab **`/events/[id]/door`**: register this browser as a door device (token kept in `localStorage['klubhub-door-device']` = `{id,label,token,event:{id,title,starts_at}}`), generate staff and manager PINs (shown once), list/revoke devices.
- `useDoorStore` + `utils/doorDb.ts`: IndexedDB (tiny wrapper, no dependency) holding the bundle and the op queue **AES-GCM encrypted** with a non-extractable key created per door session; wipe on logout, session expiry or event end.
- Search: accent-folded, prefix per token, Damerau-Levenshtein ≤ 1 for tokens ≥ 4 chars (pure util + tests). Ticket order refs and secrets match exactly.
- QR: `BarcodeDetector` where available, typed-code fallback; Express mode checks in on scan, Standard opens the card.
- Card: list, entry terms, perks, **PAST CUTOFF** (admit anyway needs a second tap), +N stepper for partial arrivals, remaining heads; conflict flag after sync.
- Undo toast (≈ 6 s) after every check-in; occupancy (in − out + walk-ups vs capacity) with walk-up and out buttons; sync badge (queued count, last sync, offline); sync every 15 s and on `online`.
- On-the-spot add: name, +N, list, manager PIN verified offline (PBKDF2 via WebCrypto), then queued add + check-in.
- Door shell service worker (`public/door-sw.js`, production only): caches `/door` and `/_nuxt/*`, never `/api`. `public/door.webmanifest` with `display: standalone`, `start_url: /door`.
- Mock handlers for every route above.

## P2.4 contract (post-event report)

Read-only aggregates over guests, allocations, tickets, `checkins` and `door_counters`. No names in the report JSON except allocation submitter labels (public class, as in P2.1). Undone rows never count.

| Route | Action | Notes |
|---|---|---|
| `GET /api/v1/events/{eventID}/report` | guestlist.read | Works during the event too (`live: true` while now < ends_at) |
| `GET /api/v1/events/{eventID}/report/list-back.csv?allocation_id=` | guestlist.read | One allocation's guests; audited `guestlist.export`; same CSV hardening as the guest export (BOM, formula-prefix escaping) |

Definitions: a guest **arrived** when its non-undone `in` heads ≥ 1; **heads admitted** = Σ non-undone `in` counts; **+1s used** = min(plus_n, heads admitted − 1) for arrived guests; **no-show rate** = going guests not arrived / going guests (null when 0 going); a ticket is **scanned** when it has a non-undone `in`; **occupancy** = running Σ(in − out) over check-ins + walk-ups + manual in − manual out.

```json
{
  "event": {"id","title","starts_at","ends_at","timezone","capacity"},
  "generated_at": "…", "live": false,
  "totals": {"guests_going","guests_arrived","no_show_rate","heads_expected","heads_admitted",
             "plus_ones_allowed","plus_ones_used","tickets_valid","tickets_scanned",
             "walkups","peak_occupancy","peak_at","conflicts"},
  "by_list": [{"list_id","name","type","going","arrived","no_show_rate","heads_expected","heads_admitted","plus_ones_allowed","plus_ones_used"}],
  "by_submitter": [{"allocation_id","list_id","list_name","list_type","submitter","quota","going","arrived","no_show_rate","heads_admitted","revoked"}],
  "tickets_by_type": [{"ticket_type_id","name","valid","scanned"}],
  "curve": [{"bucket_start","in","out","walkups","occupancy"}]
}
```

`heads_expected` = Σ (1 + plus_n) of going guests + valid tickets. `curve` has 15-minute buckets aligned to :00/:15/:30/:45 in the event's timezone, from the first to the last activity (empty array when none); `occupancy` is the value at the end of the bucket. `peak_occupancy`/`peak_at` come from the same series.

List-back CSV columns: `name, plus_n, status, arrived, heads_admitted, first_in_local` (event timezone, `YYYY-MM-DD HH:MM`). No email or phone.

Frontend: new event tab **REPORT** (`/events/[id]/report`): KPI tiles (arrived / going, no-show rate, heads admitted vs expected vs capacity, +1s used, walk-ups, peak occupancy and time), check-in curve as an inline SVG chart (no chart dependency; in, out and occupancy, keyboard- and screen-reader-accessible table fallback), by-list and by-submitter tables with a LIST BACK CSV button per allocation (artist lists first), tickets by type, empty state when nothing happened yet, LIVE badge while the event runs. Mock handlers derive the report from the mock guests, tickets and check-ins.
