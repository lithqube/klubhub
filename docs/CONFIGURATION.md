# KlubHub DJ — Configuration

Follow [Setup and Self-Hosting](./SELF-HOSTING.md) for provisioning. `scripts/setup.sh dev|prod` creates private file secrets and Garage configuration under `.local/dev` or `.local/prod`. The default mode is development. Rerunning setup preserves existing credentials; it does not rotate them.

This reference describes the Compose runtime contract. Setup does not load a root `.env`; supply overrides as exported environment variables. Keep them consistent in later Compose commands and use `--env-file /dev/null` to avoid importing unrelated root `.env` values. Never commit generated configuration, file secrets, or `.env` overrides, and never paste their contents into an issue.

## Modes and addresses

| Setting | Development | Production |
|---|---|---|
| Compose file | `docker-compose.yml` | `docker-compose.prod.yml` alone |
| App source | Local API build; host Nuxt | Published combined app image |
| Private state | `.local/dev` | `.local/prod` |
| Frontend URL | `http://localhost:4200` | `http://127.0.0.1:8080` |
| `SERVE_FRONTEND` | Disabled | `true` |
| `NUXT_INTERNAL_URL` | `http://host.docker.internal:4200` | `http://127.0.0.1:3000` inside the app container |
| `NUXT_PUBLIC_API_BASE` | Set to `http://127.0.0.1:8080` for host Nuxt; unset for mocks | Managed by the combined runtime; no host Nuxt command needed |

The API listens on `0.0.0.0:8080` **inside** its container. Host loopback publication provides the default access restriction. Do not substitute a container-loopback listener for a host bind restriction.

Production runs Go, Nuxt/Node, and Playwright in one supervised app container. Port 3000 is internal and is not published. Production PostgreSQL also has no host port. The optional development override exposes Garage administration, not another app service.

## Host publication and browser URLs

| Variable | Default / meaning | Notes |
|---|---|---|
| `BIND_ADDRESS` | `127.0.0.1` host app/API bind | Independent of the container listener. Wider exposure requires a deliberate network-access policy. |
| `API_PORT` | `8080` host port | Update host Nuxt's API URL and external clients if changed. |
| `S3_BIND` | `127.0.0.1` host S3 bind | Independent of `BIND_ADDRESS`; changing one does not expose the other. |
| `S3_PORT` | `39000` host S3 port | Maps to Garage port `3900` inside Docker. |
| `S3_ENDPOINT` | `storage:3900` inside Docker | API-to-Garage address, not a browser URL. |
| `S3_PUBLIC_ENDPOINT` | `http://127.0.0.1:39000` | Presigned browser URL base; explicitly set a reachable URL for any remote browser. |
| `S3_REGION` | `europe-west-1` | Must match the Garage configuration. |
| `S3_BUCKET` | `klubhub` | Bucket provisioned by setup. |
| `CORS_ORIGIN` | Frontend origin for the selected mode | Match scheme, host, and port when changing frontend access. CORS is not authentication. |
| `IMAGE_TAG` | `v1.0.0` | Production only. Select an accessible published release containing the desired fixes. |
| `IMAGE_REPOSITORY` | `ghcr.io/lithqube/klubhub-dj-api` | Production app image repository. |

For a remote browser, its `127.0.0.1` is not the server. Provide an explicitly reachable S3 endpoint and matching publication/proxy route. HTTPS frontend pages need HTTPS object URLs to avoid mixed content. Do not expose Garage admin/RPC as a way to fix object downloads.

Development and production have separate Compose project names and volumes, but default host ports overlap. Use one mode at a time or assign distinct ports and matching URLs. Existing `klubhub-dj` volumes require [explicit reuse or migration](./SELF-HOSTING.md#upgrades-and-existing-installations).

## Required private values

Setup manages these values as private files and wires the corresponding file-backed settings into containers. Do not print them or duplicate them into shell commands.

| Value | Purpose |
|---|---|
| `POSTGRES_PASSWORD` | Database password shared by Postgres and the API. Changing its file does not change a password in an existing database. |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | Garage application key pair provisioned with bucket access. |
| `GARAGE_RPC_SECRET` | Garage node communication secret. |
| `GARAGE_ADMIN_TOKEN` | Garage administration token. |
| `TOKEN_ENCRYPTION_KEY` | AES-256-GCM token encryption key. Replacing it makes stored OAuth tokens unreadable unless migrated; preserve it with backups. |
| `ICAL_SECRET` | Shared secret for protected calendar/booking endpoints, not general app authentication. Rotation invalidates existing links. |

Compose secrets are mounted files, not an encrypted secret-management service. Protect host permissions and backups. Do not edit `/run/secrets` inside a container or evaluate bootstrap output. See [Operations](./OPERATIONS.md) for rotation precautions.

## Optional API settings

| Variable | Default | Purpose |
|---|---|---|
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `SHUTDOWN_TIMEOUT_SEC` | `30` | API graceful-shutdown bound; coordinate with Compose's stop grace period. |
| `HTTP_READ_HEADER_TIMEOUT_SEC` | `5` | Header-read timeout. |
| `HTTP_READ_TIMEOUT_SEC` | `30` | Request-read timeout. |
| `HTTP_WRITE_TIMEOUT_SEC` | `60` | Response-write timeout. |
| `HTTP_IDLE_TIMEOUT_SEC` | `120` | Keep-alive idle timeout. |
| `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` | Empty | Optional cover-art integration. |
| `DISCOGS_API_KEY` | Empty | Optional cover-art integration. |
| `INSTAGRAM_CLIENT_ID` / `INSTAGRAM_CLIENT_SECRET` | Empty | Optional Instagram OAuth integration. |

An API setting must be passed to the container to take effect; setting a host environment variable alone does not automatically inject it into Compose services. Review the service environment mapping when adding an override.

## Legacy configuration and release access

`MINIO_*` aliases are no longer supported; Garage uses `S3_*` settings. Do not copy old root-level secrets or Garage configuration into a fresh mode without checking that they match the data being reused.

The default production tag is not evidence that local runtime changes are released. Those fixes require a new image publication. Anonymous requests for the existing v1.0.0 package returned 403; see [container images](./container-images.md) for GHCR access and tag verification.
