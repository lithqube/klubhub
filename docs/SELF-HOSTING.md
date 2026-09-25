# KlubHub DJ — Setup and Self-Hosting

This is the canonical setup guide for frontend mocks, a local development backend, and an image-only production deployment. Run commands from the repository root. Keep the scripts, Compose files, and image version from a compatible release together; downloading only a Compose file is not enough.

> **Release status:** local runtime changes do not update an existing GHCR tag. The production default is `IMAGE_TAG=v1.0.0`, but these fixes require a newly published image. Anonymous requests for the existing v1.0.0 package returned 403. Confirm package access and select a release that contains the fixes before deploying. See [container images](./container-images.md).

## System requirements

- Use Docker Engine 24+ with Compose v2 and a running daemon. On Apple Silicon, Docker Desktop or OrbStack provides the Linux VM.
- Production app images target `linux/arm64`. Do not assume native amd64 support.
- Local frontend development needs Node and pnpm matching the repository toolchain; run `pnpm install` first.
- Setup uses Bash, Python 3, and Docker Compose. Reserve space for PostgreSQL, Garage objects, browser dependencies, and backups.

The app is single-user and network-private, **not an authenticated public SaaS**. TLS encrypts traffic; CORS controls browser cross-origin access. Neither authenticates a user. Keep the stack private or place it behind an authentication gateway before making it accessible from untrusted networks.

## Frontend-only mock mode

Leave `NUXT_PUBLIC_API_BASE` unset in both your shell and any local environment overrides. No Docker services are needed.

```bash
pnpm install
pnpm nx serve @dev/dj
```

Open http://localhost:4200. Nitro serves the mock handlers under `apps/dj/server/api/v1/`.

## Development with a real backend

Setup creates private file secrets and a Garage configuration under `.local/dev`, bootstraps the Garage layout, bucket, and application key, and starts the default `docker-compose.yml` stack. It preserves existing credentials on reruns; it is not a reset or rotation command.

```bash
bash scripts/setup.sh dev
```

Omitting the mode also selects `dev`. The Docker stack contains `db`, `storage`, and a locally built API only. Run Nuxt on the host in a second terminal:

```bash
NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080 pnpm nx serve @dev/dj --host 0.0.0.0 --port 4200
```

Open http://localhost:4200. The API health endpoint is http://127.0.0.1:8080/api/v1/health. The API container listens on `0.0.0.0:8080`, while Docker publishes it on host loopback by default. These are different bind addresses: binding the API to loopback *inside* the container prevents Docker forwarding from reaching it.

The API's rendering callback uses `NUXT_INTERNAL_URL=http://host.docker.internal:4200`. Nuxt must listen on the host interface reachable from Docker, which is why the command uses `--host 0.0.0.0`. Restrict that development server with your firewall and use a trusted network. If rendering fails, check host-name resolution from Docker, host firewall rules, and whether Nuxt is still running.

### Optional Garage administration

The development override publishes Garage's admin endpoint on host loopback. It is optional, not a frontend container or a production override. After setup, apply it with:

```bash
docker compose --env-file /dev/null -f docker-compose.yml -f docker-compose.dev.yml up -d
```

Garage admin uses container port `3903`; S3 uses container port `3900`. Do not confuse either with the RPC port `3901`, and do not expose admin or RPC to untrusted networks. The setup script handles provisioning without asking you to print keys or evaluate shell output.

### Optional self-hosted email (Plunk)

Transactional email (invoices issued/paid/cancelled, agreement sent/signed) is delivered through [Plunk](https://github.com/useplunk/plunk), an open-source email platform built on AWS SES. The compose stack supports **both** options without modifying the base services:

| Mode | Command | Email delivery |
|---|---|---|
| Hosted Plunk | (no overlay) | Outbound to `https://app.useplunk.com`; requires `PLUNK_API_KEY` env var |
| Self-hosted Plunk | overlay below | Outbound to your cluster's SMTP relay (default: AWS SES); no third-party account required |

For the **stand-alone product** specifically, self-host Plunk so email data never leaves the host's infrastructure. Bootstrap:

```bash
# Generate secrets/email.env + secrets/plunk_api_key (both gitignored) and
# start the plunk service alongside the rest of the stack under the
# `email` profile.
bash scripts/plunk-bootstrap.sh
docker compose \
  --env-file /dev/null \
  -f docker-compose.yml \
  -f docker-compose.email.yml \
  --profile email \
  up -d
```

After the first start, open http://localhost:3030 in a browser, create the admin account, copy the project ID into `secrets/email.env`, and paste the project API key into `secrets/plunk_api_key` (single value, no other content — it is mounted verbatim as the `plunk_api_key` Docker secret). Restart the API so the new env vars and secret take effect:

```bash
docker compose --env-file /dev/null \
  -f docker-compose.yml -f docker-compose.email.yml --profile email \
  restart app
```

The same overlay applies to `docker-compose.prod.yml`; substitute it for `docker-compose.yml` in the commands above. `secrets/email.env` and `secrets/plunk_api_key` are **not** committed and are the only places these secrets live outside of the standard `secrets/dev/` tree. `secrets/plunk_api_key` is mounted on the `app` service as the `plunk_api_key` Docker secret at `/run/secrets/plunk_api_key` — a Compose secret always mounts as a single file at `/run/secrets/<name>`, never a directory, so this is a dedicated file rather than a value inside `email.env`. Override its location with `PLUNK_API_KEY_SECRET_FILE` if you manage secrets elsewhere; `PLUNK_API_KEY_FILE` on the `app` service is kept in sync with the same default.

**Wiring at runtime:**

- `PLUNK_BASE_URL` → either `https://app.useplunk.com` (hosted) or `http://plunk:3000` (self-hosted, default in the overlay)
- `PLUNK_PROJECT_ID` → Plunk project UUID
- `PLUNK_API_KEY_FILE` → path to a file containing the project API token (Bearer); defaults to `/run/secrets/plunk_api_key`, backed by `secrets/plunk_api_key` on the host
- `PLUNK_FROM_EMAIL` / `PLUNK_FROM_NAME` → default sender for transactional messages

If any of these are missing at boot, the API still mounts `/api/v1/finance/emails/*` but queues messages without sending them — the outbox row is preserved for replay once Plunk is reachable.

## Production

Use `docker-compose.prod.yml` **alone**, not merged with the development files. It contains only image references, with no local application build:

| Service | Image | Role |
|---|---|---|
| `db` | `postgres:16-alpine` | PostgreSQL; no published host port. |
| `storage` | `dxflrs/garage:v2.2.0` | Garage S3; loopback host port 39000 by default. |
| `app` | `ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG:-v1.0.0}` | One supervised Go + Nuxt/Node + Playwright app container. |

Choose an accessible, newly published tag containing the runtime fixes and export it before setup. The default v1.0.0 is not proof that the fixes have been released.

```bash
# Set IMAGE_TAG to the published version you intend to run, then:
bash scripts/setup.sh prod
```

Setup uses `.local/prod` for private secrets and Garage configuration, provisions storage, and starts the stack. `SERVE_FRONTEND=true` belongs only to production. Go handles API requests and proxies UI/rendering requests to the Node runtime. Nuxt listens internally on port 3000, which is not published. Open http://127.0.0.1:8080 for the UI and use the same port for the API.

Check the deployment rather than assuming startup means readiness:

```bash
docker compose --env-file /dev/null -f docker-compose.prod.yml ps
curl --fail http://127.0.0.1:8080/api/v1/health
curl --fail --output /dev/null http://127.0.0.1:8080/
```

Also test a real upload, a browser-visible stored image, and a generated tracklist image. An API health response alone does not prove that the UI or Playwright rendering works.

## Configuration and remote browsers

The gitignored `.local/dev` and `.local/prod` directories are separate private state. Keep them backed up securely with their matching data. Do not delete them to troubleshoot an existing database: regenerated credentials will not match persisted data. Do not commit or paste their contents into issues.

See [Configuration](./CONFIGURATION.md) for overrides. Setup does not load a root `.env` file; pass overrides as exported environment variables. Preserve those same values in subsequent commands. The Compose examples use `--env-file /dev/null` to avoid silently importing an unrelated root `.env`.

- `BIND_ADDRESS` controls the host-published app/API interface and defaults to loopback. It does not change the API's all-interface container listener.
- `S3_BIND` independently controls the host-published S3 interface and defaults to loopback. Changing the app bind does not expose S3.
- `S3_PUBLIC_ENDPOINT` is the URL embedded in presigned links. Its default `http://127.0.0.1:39000` works only for a browser on the Docker host. A remote browser needs an explicitly configured, reachable S3 URL; container DNS names and the server's loopback address will not work.
- For HTTPS pages, use an HTTPS S3 endpoint too, to avoid mixed-content failures. Preserve the signed host/path when proxying S3.
- Set `CORS_ORIGIN` for the actual frontend origin when changing the scheme, hostname, or port. CORS is not authentication.

A host reverse proxy forwards production UI and API traffic to **one upstream**, `127.0.0.1:8080`. Add authentication at that boundary and keep the backend loopback-bound. A proxy running in another container needs a deliberately configured Docker-network upstream; its own loopback is not the app container. S3 needs its own reachable route for browser downloads. Do not forward the internal Nuxt port or Garage administration ports.

## Upgrades and existing installations

Development and production use different Compose project names and project-scoped volumes, without fixed container names. This isolates data and credentials, but does not avoid host-port collisions. Stop the other mode first or configure different published ports and matching callback/browser URLs.

**The previous `klubhub-dj` project does not automatically migrate.** Starting the new production project can create empty volumes while leaving old data untouched. Do not treat that as a successful upgrade.

1. Record the existing project name, volume names, image tag, configuration paths, and credentials. Inspect container mounts and volume metadata without printing secret contents.
2. Back up PostgreSQL, Garage, and the matching private configuration. Verify the backup with an isolated restore drill.
3. Stop the old writers. Choose explicit reuse of the old volumes through a reviewed Compose override, or migrate/restore into the new project. Preserve the matching database and Garage credentials and token encryption key; do not generate replacements for old data.
4. Check the resolved volume mounts and project name before starting the replacement. Never run two database containers against the same PostgreSQL volume.
5. Select a published compatible image, then start the chosen deployment. Verify health, existing records, stored objects, and rendering before removing any old resources.

For later upgrades within the same project, back up first, select the new `IMAGE_TAG`, and rerun production setup with the same configuration overrides. Database migrations may make an image-only rollback unsafe; retain a compatible data backup and previous configuration.

`docker compose down` normally preserves named volumes. **`down -v` deletes them. Use it only for disposable data, never as an upgrade or routine troubleshooting step.** Do not remove the old project or its volumes until migration is verified.

## Operations and security

See [Operations](./OPERATIONS.md) for health, logs, backup prerequisites, and rotation. Report security issues through [SECURITY.md](../SECURITY.md).
