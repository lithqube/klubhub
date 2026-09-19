# Compose runtime contract — KlubHub

Three compose files, three purposes, zero ambiguity. This file documents the contracts.

## File purposes

| File | Purpose | When to use |
|---|---|---|
| `docker-compose.yml` | Dev defaults — local build, API-only, exposes port 8080 to host | Day-to-day development |
| `docker-compose.prod.yml` | Standalone prod — published image, full stack with frontend served by API container | Production deployment |
| `docker-compose.dev.yml` | Overlay — adds the Garage admin port mapping | Only when debugging S3 storage directly |

`docker-compose.yml` is the base for dev. `docker-compose.prod.yml` is a complete standalone (not an overlay of dev). `docker-compose.dev.yml` is an overlay that adds one port mapping on top of dev.

## Service name: `app`

The service was renamed from `api` to `app` for consistency with future products (Promoter, Label all become `app` services in their own compose files). Anywhere you see `services.api` in older docs / branches, it's stale.

## Dev (`docker-compose.yml`)

```
services:
  app:
    build:
      context: .
      dockerfile: apps/dj/Dockerfile
    image: ghcr.io/lithqube/klubhub-dj-api:dev
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      BIND_ADDRESS: "0.0.0.0"
      SERVE_FRONTEND: "false"   # API does NOT serve the frontend in dev
      NUXT_PUBLIC_API_BASE: "http://host.docker.internal:4200"
    depends_on:
      db:
        condition: service_healthy
      storage:
        condition: service_healthy
```

**`SERVE_FRONTEND=false` makes `/` and `/tracklist` return 404** — this is correct behavior, not a bug. The frontend runs on the host via `pnpm nx run @dev/dj:serve` (Nuxt on `:4200`), which talks to the API at `http://127.0.0.1:8080`.

Screenshot/callback URLs from the dev backend (e.g. Plunk email rendering, Open Graph card generation) use `http://host.docker.internal:4200`, NOT `http://localhost:4200` (which would resolve inside the container's loopback).

The API binds `0.0.0.0:8080` inside the container so the host port mapping can reach it. Setting `BIND_ADDRESS=127.0.0.1` is a common bug — it makes the port unreachable from the host.

## Prod (`docker-compose.prod.yml`)

Standalone. Pulls the published image, no `build:` block, no dev dependencies.

```
services:
  app:
    image: ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG:-v1.0.0}
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      BIND_ADDRESS: "0.0.0.0"
      SERVE_FRONTEND: "true"   # API serves the frontend at / in prod
      S3_BIND: "127.0.0.1"     # storage only reachable from the API container's network
    secrets:
      - plunk_secret_key
```

**`SERVE_FRONTEND=true` makes `/` return the Nuxt-built static site.** The internal Nuxt dev server runs at `:3000` inside the container; the API container proxies it. Do not publish port 3000 to the host.

## depends_on ordering

Storage (Garage) takes longer to become healthy than the database. `scripts/stack_setup.py` handles this with explicit `up --wait` ordering:

```python
subprocess.run(["docker", "compose", "up", "-d", "--wait", "storage"])  # Garage first
# bootstrap creates the S3 bucket
subprocess.run(["docker", "compose", "up", "-d", "--wait"])              # then everything
```

If you start the full stack without that order, the bootstrap step will fail because the storage container isn't accepting connections yet.

## Volume naming

Dev and prod use distinct volume names:

- Dev: `klubhub-dj-dev_postgres`, `klubhub-dj-dev_garage-data`, `klubhub-dj-dev_garage-meta`
- Prod: `klubhub-dj-prod_postgres`, `klubhub-dj-prod_garage-data`, `klubhub-dj-prod_garage-meta`

These come from the compose project name. Never set `container_name:` in compose — let compose generate unique names. Setting it causes the second stack you start (dev + prod on the same host) to fail with "container name already in use".

## Plunk secret mount (prod)

```
secrets:
  plunk_secret_key:
    file: /etc/klubhub/secrets/plunk_secret_key  # host path

services:
  app:
    secrets:
      - plunk_secret_key
    environment:
      PLUNK_SECRET_KEY_FILE: /run/secrets/plunk_secret_key  # container path
```

Operator must pre-place the file on the host. The Go API reads it via `os.Getenv("PLUNK_SECRET_KEY_FILE")`. See `references/plunk-secrets.md`.

## Common bugs (real ones hit this session)

| Symptom | Root cause | Fix |
|---|---|---|
| API unreachable from host | `BIND_ADDRESS=127.0.0.1` inside container | Set to `0.0.0.0` |
| `/` returns 404 in dev | Forgot to run Nuxt on host | `pnpm nx run @dev/dj:serve` (port 4200) |
| `/` returns 404 in prod | `SERVE_FRONTEND=false` in prod compose | Set to `true` |
| Screenshot callback fails in dev | Used `http://localhost:4200` from inside container | Use `http://host.docker.internal:4200` |
| Bootstrap fails | Storage not healthy | Run `up --wait storage` first |
| Second stack fails to start | `container_name` collision | Remove `container_name:` from compose file |
| Storage debug port unreachable | Wrong port in overlay | Use `127.0.0.1:39002:3903` (Garage admin runs on 3903) |
