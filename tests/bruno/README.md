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

## Environment variables

| Var | Default | Purpose |
|---|---|---|
| `baseUrl` | `http://127.0.0.1:8080` | Go API base URL |

Override at runtime via Bruno's `--env <name>` flag with an
`.env.bruno` file alongside the collection.

## Coverage map

| Folder | Endpoints tested |
|---|---|
| `health/` | `GET /api/v1/health` (200 + database/storage integration check) |
| `settings/` | `GET` and `PUT /api/v1/settings` |
| `tracklists/` | `GET /api/v1/tracklists`, `POST /api/v1/tracklists` |
| `gigs/` | `GET /api/v1/gigs`, `GET /api/v1/gigs/calendar.ics?secret=...` (200), `GET /api/v1/gigs/calendar.ics` (401, no bearer — Plan B.5 regression) |
| `epk/` | `GET` and `PUT /api/v1/epk/content` |
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
