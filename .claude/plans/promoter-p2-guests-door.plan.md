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

## Status (2026-09-26)

| Slice | State | Commits |
|---|---|---|
| P2.1 Guest lists & guest table | Done | `3b8b018` (API, migration 00008), `b0aa9e2` (guest table, /guests, mocks, e2e) |
| P2.2 Attendee import | Not started | — |
| P2.3 Offline door | Not started | — |
| P2.4 Post-event report | Not started | — |
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
