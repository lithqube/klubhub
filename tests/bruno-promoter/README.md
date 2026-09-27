# KlubHub Promoter — Bruno HTTP API tests

[Bruno](https://www.usebruno.com/) collection that exercises the Promoter Go
API (`api/cmd/promoter`) over HTTP: auth, org, venues/events, guest lists,
attendee import, the offline door and the post-event report. It checks status codes, key JSON fields
and security behaviour (401/403/CSRF/MFA, name-only door data, CSV hardening).

## Run

```bash
pnpm nx run api:test-api          # or: bash scripts/test-promoter-api.sh
```

`scripts/test-promoter-api.sh`:

1. starts a disposable `postgres:16-alpine` container (unique name, random
   127.0.0.1 port, removed on exit by a trap);
2. builds `api/cmd/promoter` with `go build` into a temp dir;
3. runs `promoter migrate` (the goose provider the binary and `pgtest` use)
   and sets the `klubhub_app` / `klubhub_relay` passwords the same way;
4. runs `promoter bootstrap` and reads the one-time setup token it prints;
5. starts `promoter serve` (local identity, no NATS) on a free port, waits for
   `/api/v1/health`;
6. runs `bru run -r --env local --disable-cookies` here, passing `baseUrl`,
   `origin`, `ownerEmail`, `ownerPassword` and `setupToken` as `--env-var`;
7. stops the API and removes the container and temp dir.

DB passwords, the KEK, the owner password and the setup token are generated
per run and never written into the repo. Extra arguments go to `bru run`
(e.g. `--bail`). `KEEP_LOGS=1` keeps the temp dir (API log and
`results.json`; the latter contains that run's throwaway credentials).

Requirements: Docker, Go, OpenSSL, Python 3 (free-port lookup) and the
`bru` CLI (`npm i -g @usebruno/cli`, v2.x).

The collection is **stateful and ordered** (folders `01-` … `12-`, `seq` per
request, ids chained with `bru.setVar`). It needs a fresh instance: the setup
link works once and TOTP is enabled at the end. Run the whole collection, not
single requests.

## How auth is handled

- Session cookies are `__Host-kh_session` with `Secure`, which no cookie jar
  stores over plain `http://127.0.0.1`. Login and door-login tests capture the
  value from `Set-Cookie` into `staffCookie` / `doorCookie` and later requests
  send an explicit `Cookie` header (hence `--disable-cookies`).
- CSRF: `collection.bru` sends `Origin: {{origin}}` and `X-KlubHub-CSRF: 1`
  on every request; `origin` must equal `PROMOTER_PUBLIC_ORIGIN`.
- MFA: `09-mfa` enrolls TOTP, computes RFC 6238 codes in the script sandbox
  (node `crypto`, HMAC-SHA1, 30 s, 6 digits) and confirms, which upgrades the
  current session to `amr [pwd otp]`. MFA-gated actions are tested both
  before (403 `mfa_required`) and after.
- Runtime secrets (`deviceToken`, PINs, TOTP secret, cookies) live only in
  runtime variables and are never asserted by value or logged.

## Coverage map (107 requests)

| Folder | Endpoints | Checks |
|---|---|---|
| `01-health/` | `GET /api/v1/health`, unknown route | 200 `status/database: ok`, security headers (`nosniff`, `DENY`, `no-store`); JSON 404 |
| `02-auth/` | `GET /org`, `GET /auth/me`, `POST /auth/setup`, `POST /auth/login` | 401 without / with forged cookie; setup from foreign `Origin` → 403 csrf; weak password → 400 `weak_password`; setup → 204; replay → 400 `invalid_link`; wrong password → 401 without cookie; login → 200 + `HttpOnly; Secure; SameSite=Lax; Path=/` cookie; `me` → owner, `mfa: false` |
| `03-org/` | `GET /org`, `POST /members/invites` | org slug/timezone/currency, id = session tenant; invite without MFA → 403 `mfa_required` |
| `04-events/` | `POST/GET /venues`, `GET /venues/{id}`, `POST /venues/{id}/reveal`, `POST/GET /events`, `GET /events/{id}` | sealed address masked in create/get/list; reveal without MFA → 403; invalid event → 422 `field: ends_at`; event create with venue → stages from rooms; listed under `?view=drafts`; unknown id → 404; no session → 401 |
| `05-guests/` | `POST/GET /events/{id}/lists`, `POST/GET …/lists/{id}/allocations`, `POST/GET …/guests`, `POST …/guests/bulk-status`, `GET …/guests/export.csv` | perk normalisation; unknown list type → 422; allocation quota; add going guests; duplicate email (case-insensitive) skipped; email on name-only list → 422 `guests[0].email`; over quota → 409 `quota_exceeded` with numbers; bulk status by email with `unmatched`; invalid status → 422; status filter + tab counts; CSV: `text/csv` attachment, `no-store`, **raw UTF-8 BOM**, header row, **`=`/`@` cells prefixed with `'`**; export / add without session → 401 |
| `06-attendees/` | `POST /events/{id}/attendees/import` (multipart + JSON), `GET …/guests` | RA preset dry run (default): mapping, counts, masked preview, no secrets, nothing written; `dry_run=false` import; re-import is idempotent (`positions_unchanged: 3`); missing file → 422 `file`; unknown preset → 422; incomplete generic mapping → 422 `mapping_incomplete`; `text/plain` → 415; tickets in guest table without `secret`; no session → 401. Fixture: `fixtures/ra-tickets.csv` (fake names, `example.org`) |
| `07-door-setup/` | `POST/GET /door/devices`, `POST/GET /door/events/{id}/pin` | device token shown once, not listed; empty label → 400; staff and manager PIN (6 digits); window > 36 h → 400; PIN status has windows but no PIN/hash; **staff calling `/door/bundle` and `/door/checkins` → 403 `door_session_required`**; no session → 401 |
| `08-door/` | `POST /door/login`, `GET /auth/me`, `GET /door/bundle`, `POST /door/checkins`, `POST /door/adds` | wrong PIN / unknown device / manager PIN → 401; login → 204 + cookie; principal is `door`, scoped to the event; **bundle has no email/phone key or email-shaped value anywhere**, has lists, guests, ticket secrets, PBKDF2 manager verifier; sync applies guest/ticket check-ins and a walk-up; **resend → `duplicate`**, nothing double-counted; **undo → applied, `undone: true` in delta**; unknown undo target, `count: 0` (`invalid_op…`) and foreign subject (`unknown_subject`) rejected per op; **adds with wrong manager PIN → `manager_pin_invalid`**; correct PIN → applied with client id, resend → duplicate, guest shows as `going` / `source: door`; door session → 403 `no_role_grant` on `/org`, guest table and CSV export; unknown body field → 400; no session → 401; **staff guest table after sync: `heads_in` 2 and `first_in_at` = device time for the checked-in guest, 0 / null for everyone else (rejected op), undone ticket `checked_in: false`, `counts.checked_in` 1**; `status=checked_in` → only that guest, counts unfiltered, no tickets; CSV export with `status=checked_in` → 422 |
| `09-mfa/` | `POST /auth/totp/enroll`, `POST /auth/totp/confirm`, `GET /auth/me`, `POST /venues/{id}/reveal`, `POST /auth/login` | otpauth URI params; wrong code → 401; computed code → 204; `me.mfa: true`; reveal → sealed address, `no-store`; re-enroll → 400; password-only login → 401 `totp_required`, no cookie |
| `10-report/` | `GET /events/{id}/report`, `GET …/report/list-back.csv` | report shape (event, totals keys, by list / submitter / ticket type, curve buckets); arrived 1, heads admitted 2, scanned 0 (undone), walk-ups 1; **no guest or ticket-holder name or email anywhere** (every name/email from the guest table, plus an email pattern); list-back: `text/csv` attachment, `no-store`, header `name,plus_n,status,arrived,heads_admitted,first_in_local` (**no email/phone column**), the allocation's guest with its door outcome, no `@`; without `allocation_id` → 422 `field: allocation_id`; random uuid → 404 |
| `11-door-lockout/` | `POST /events`, `POST/GET /door/events/{id}/pin`, `POST /door/login` | on a **separate event** (the main event's door tests keep working): wrong staff PIN tries 1–4 → 401, **5th → 429 `pin_locked` with `Retry-After` (~15 min) and `retry_after`**, no cookie; the right PIN while locked → 429; PIN status shows `staff.locked_until`; a new staff PIN → 201 clears it (`locked_until: null`) and login works → 204 |
| `12-sessions/` | `DELETE /door/devices/{id}`, `GET /door/bundle`, `POST /door/login`, `POST /auth/logout`, `GET /org` | revoke → 204; revoked device's session → 401 and its login → 401; logout → 204 + expired cookie; old cookie → 401 |

## Not covered

- **Full login with TOTP** (email + password + code): the confirm step marks
  the current 30-second step as used, so a login in the same step is refused
  as a replay. Waiting up to 30 s was not worth the runtime; `login` with a
  code is exercised by the Go integration tests.
- **`audience.export` / `member.manage` / `security.manage` step-up
  (15-minute re-authentication)** and finance routes: no routes for most of
  them yet, and step-up expiry needs a clock the HTTP test cannot move.
- **Invites and a second member with a lesser role** (e.g. `booker`, `door`
  user): invite needs MFA and the token is only shown to the inviter; kept out
  to keep the flow short. Role denials are covered with the door principal.
- **Cross-device conflicts** (`conflict: true`) need two door devices with
  separate sessions; covered by `door_integration_test.go`.
- **Manager PIN lockout** (5 attempts / 15 min) is not driven to the limit,
  so later tests keep working (the staff PIN lockout is, on its own event).
- Venue update/archive, event status/stages/lineup/exports, standing lists,
  allocation update/revoke, guest update/delete, list delete, `guests/overview`,
  org profile update.
