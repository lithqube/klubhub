# KlubHub DJ — Bruno HTTP test collection

[Bruno](https://www.usebruno.com/) collections for HTTP-level smoke
testing against a running KlubHub DJ API.

## Install Bruno

```bash
# macOS
brew install bruno

# Linux
# See https://www.usebruno.com/downloads
```

## Run

Open Bruno, point it at this directory (`tests/bruno/`), and click "Run
Collection". Or use the CLI:

```bash
cd tests/bruno
bru run --env local
```

The CLI produces `results.json` (JUnit-style) which CI can consume.

## Disposable stack (no live services needed)

`scripts/test-dj-api.sh` stands up everything the collection needs and
tears it down on exit — a disposable Postgres container, a tiny in-memory
S3 stub (`scripts/test-dj-api-s3-stub.py`), and the real `./cmd/api`
binary built from source. It seeds one gig, one tracklist, one venue and
one contact, then runs the whole collection against them:

```bash
scripts/test-dj-api.sh          # or: pnpm nx run api:test-dj-api
```

Set `KEEP_LOGS=1` to keep the temp dir (API log, S3 stub log, Bruno
`results.json`) for debugging.

### CLI quirk: multipart bodies need camelCase `body:` in the request block

The `bru` CLI (2.6.1) copies the request block's `body:` value verbatim
into the internal body mode, but its request preparer only recognises
the camelCase mode names. `body: multipart-form` therefore silently
drops the body (the request goes out empty and urlencoded). The
`tracklists/upload.bru` request works around this by declaring
`body: multipartForm` in the `post {}` block while keeping the
hyphenated `body:multipart-form {}` content block (the content block
name is what the .bru grammar matches on). The Bruno GUI writes the
hyphenated form, so this file intentionally differs; do not "fix" it
back to `multipart-form` or the upload test will send an empty body.

## Environment variables

| Var | Default | Purpose |
|---|---|---|
| `baseUrl` | `http://127.0.0.1:8080` | Go API base URL |
| `calendarSecret` | `test-secret` | Bearer secret for `calendar.ics` (must match the API's `ICAL_SECRET`) |
| `gigId` | all-zeros UUID | Gig used by the link/unlink tracklist tests |
| `tracklistId` | all-zeros UUID | Tracklist used by the link/unlink tracklist tests |
| `venueId` | all-zeros UUID | Venue linked to the created gig |
| `contactId` | all-zeros UUID | Contact linked to the created gig |

`scripts/test-dj-api.sh` overrides all of these with freshly seeded IDs
via `--env-var`. Override at runtime via Bruno's `--env <name>` flag with
an `.env.bruno` file alongside the collection.

## Coverage map

| Folder | Endpoints tested |
|---|---|
| `health/` | `GET /api/v1/health` (200 + database/storage integration check) |
| `settings/` | `GET` and `PUT /api/v1/settings`, invalid JSON (400), >1 MiB body (413) |
| `venues/` | `GET`/`POST /api/v1/venues`, `GET`/`PUT`/`DELETE /api/v1/venues/{id}`, `GET /api/v1/venues/autocomplete`, invalid JSON (400), 404 after delete |
| `contacts/` | `GET`/`POST /api/v1/contacts`, `GET`/`PUT`/`DELETE /api/v1/contacts/{id}`, `GET /api/v1/contacts/autocomplete`, missing name (400), invalid JSON (400), 404 after delete |
| `gigs/` | `GET`/`POST /api/v1/gigs`, `GET`/`PUT`/`DELETE /api/v1/gigs/{id}`, `GET /api/v1/gigs/{id}/detail`, `POST /api/v1/gigs/{id}/link-venue`, `POST /api/v1/gigs/{id}/link-contact`, `POST`/`DELETE /api/v1/gigs/{id}/tracklists/{tracklistId}`, `GET /api/v1/gigs/calendar.ics?secret=...` (200), `GET /api/v1/gigs/calendar.ics` (403, no bearer — Plan B.5 regression), stale `updated_at` (409), status change on cancelled gig (403), invalid JSON (400), 404 after delete |
| `tracklists/` | `GET /api/v1/tracklists`, `POST /api/v1/tracklists/upload` (multipart Rekordbox TSV), `GET`/`DELETE /api/v1/tracklists/{id}`, 404 for unknown id, 404 after delete |
| `epk/` | `GET` and `PUT /api/v1/epk/content`, `GET /api/v1/epk/exports` |
| `social/` | `GET /api/v1/social/posts`, `GET /api/v1/social/accounts` |

## Note on auth

The v1.0.0 release is single-user self-hosted with **no management
auth** (Plan B audit finding). Bruno tests therefore exercise
unauthenticated endpoints. If/when an auth layer lands, the
collection should be updated to inject the appropriate token via
Bruno's auth helpers.

## CI integration

A future CI job can run:

```yaml
- name: Run Bruno smoke tests
  run: |
    docker compose -f docker-compose.prod.yml up -d
    sleep 30  # wait for API to become healthy
    bru run --env local tests/bruno/
```

This validates the entire production compose end-to-end beyond what
the Go-side integration test (`internal/gig/integration_test.go`)
covers.
