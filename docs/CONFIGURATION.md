# KlubHub DJ — Configuration

Every environment variable this codebase reads, grouped by service.

The values shown are **placeholders**. Never commit real secrets; the
gitignore blocks `.env`, `garage.toml`, and `backups/*.tar.gz`.

## Required (every deployment breaks without these)

| Variable | Service | Default | Notes |
|---|---|---|---|
| `POSTGRES_PASSWORD` | db, api | — | Generate with `openssl rand -hex 32`. The same value reaches the API via `${POSTGRES_PASSWORD}`; do not put the literal in `docker-compose.yml`. |
| `S3_ACCESS_KEY` | api, storage (Garage) | — | Garage key id. Bootstrap with `./scripts/garage-bootstrap.sh`. |
| `S3_SECRET_KEY` | api, storage (Garage) | — | Garage secret. Bootstrap with `./scripts/garage-bootstrap.sh`. |
| `GARAGE_RPC_SECRET` | storage | — | `openssl rand -hex 32`. |
| `GARAGE_ADMIN_TOKEN` | storage | — | `openssl rand -base64 32`. |
| `TOKEN_ENCRYPTION_KEY` | api | — | `openssl rand -hex 32`. **Rotating this invalidates all stored OAuth tokens** (Instagram). Re-authorize accounts after rotation. |
| `ICAL_SECRET` | api | — | High-entropy shared secret (≥32 bytes). Gates the `calendar.ics` and per-gig booking PDF endpoints. |
| `NUXT_PUBLIC_API_BASE` | frontend | — | The Go API URL. In a single-host self-hosted setup: `http://api:8080`. |

## Optional with sane defaults

| Variable | Service | Default | Notes |
|---|---|---|---|
| `BIND_ADDRESS` | api, db, frontend | `127.0.0.1` | Set to `0.0.0.0` only behind a reverse proxy. |
| `CORS_ORIGIN` | api | `http://127.0.0.1:3000` | The frontend origin. |
| `LOG_LEVEL` | api | `info` | `debug` / `info` / `warn` / `error`. |
| `S3_REGION` | api | `europe-west-1` | Garage requires a region. |
| `S3_PUBLIC_ENDPOINT` | api | (internal) | Public S3 URL used for presigned URLs the browser fetches. Must be reachable from the user's browser. |
| `S3_BUCKET` | api | `klubhub` | The single bucket this app uses. |
| `S3_ENDPOINT` | api | `storage:39000` | Internal Docker endpoint. |
| `SHUTDOWN_TIMEOUT_SEC` | api | `30` | Bounded time `srv.Shutdown` waits for in-flight requests before force-closing. Must be ≤ `stop_grace_period` in compose. |
| `HTTP_READ_HEADER_TIMEOUT_SEC` | api | `5` | Bound before any request body is processed. |
| `HTTP_READ_TIMEOUT_SEC` | api | `30` | Total request read time. |
| `HTTP_WRITE_TIMEOUT_SEC` | api | `60` | Response write time. |
| `HTTP_IDLE_TIMEOUT_SEC` | api | `120` | Keep-alive idle close. |

## Module-fed (set only if you wire the integration)

| Variable | Module | Default | Notes |
|---|---|---|---|
| `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` | tracklist (cover art) | empty | Optional. Without these, cover art falls back to Discogs / MusicBrainz. |
| `DISCOGS_API_KEY` | tracklist (cover art) | empty | Optional. |
| `INSTAGRAM_CLIENT_ID` / `INSTAGRAM_CLIENT_SECRET` | social (Instagram OAuth) | empty | Optional. Disables the social scheduler entirely when absent. |

## Removed MinIO aliases

`MINIO_*` variables are not read in v1.0.0. Garage is the supported object
store; rename legacy variables to their matching `S3_*` names before upgrade.

## Secrets storage

- **Self-hosted single-host**: bind-mount files under `./secrets/`
  into the api and storage containers. The plan's `docker-compose.prod.yml`
  declares them via the `secrets:` block.
- **Self-hosted via reverse proxy**: usually the easiest is to keep
  the bind mounts.
- **CI / ephemeral smoke tests**: never reuse production secrets.
  `make-vault-blank.sh` is not in this repo; use your own tooling.

## See also

- `SELF-HOSTING.md` — end-to-end local install with the production compose file.
- `OPERATIONS.md` — daily operations (health, logs, restore, rotate).
- `v1-release-plan.md` — what the v1.0.0 release covers and what it doesn't.
