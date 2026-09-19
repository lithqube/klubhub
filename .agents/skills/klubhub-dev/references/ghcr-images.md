# GHCR image naming — KlubHub

Each KlubHub product owns its own API image. The naming convention was decided 2026-09-18.

## Per-product images

| Product | Image | Status |
|---|---|---|
| KlubHub DJ | `ghcr.io/lithqube/klubhub-dj-api` | First product, ships v1.0.0 |
| KlubHub Promoter | `ghcr.io/lithqube/klubhub-promoter-api` | Future |
| KlubHub Label | `ghcr.io/lithqube/klubhub-label-api` | Future |

Each product also gets:

- Its own Compose project name (`klubhub-dj-dev`, `klubhub-dj-prod`, future: `klubhub-promoter-dev`, `klubhub-label-prod`)
- Its own `.local/<product>/` private state directory
- Its own `.github/workflows/containers.<product>.yml` workflow file

## Legacy retirement

The combined `ghcr.io/lithqube/klubhub-dj` package was a transitional name for the Go+Node combined container. It has been retired. Do not push to it.

Existing tags (`v0.1.0-rc1`, `v1.0.0` under the old name) reflect pre-rename code and should not be used as deployment references. New deployments must use `ghcr.io/lithqube/klubhub-dj-api:v1.0.0` (and onwards).

## Pushing a new image

The workflow lives at `.github/workflows/containers.dj.yml` (renamed from `containers.yml` to reflect per-product ownership). It triggers on:

- Push to `main` of any file under `apps/dj/**` or the Dockerfile
- Manual `workflow_dispatch` with a tag input

The workflow:

1. Builds the image using the `apps/dj/Dockerfile` (multi-stage: Go binary + Nuxt build)
2. Tags it as `ghcr.io/lithqube/klubhub-dj-api:${GITHUB_SHA}` and `:${TAG-input}` if dispatched
3. Pushes to GHCR with the `packages: write` permission
4. Updates the `latest` tag only on tagged releases

## Authenticating with GHCR locally

For local debugging:

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u $GITHUB_USER --password-stdin
docker pull ghcr.io/lithqube/klubhub-dj-api:v1.0.0
```

## Image contents

The `apps/dj/Dockerfile` builds a multi-stage image:

1. `node:*` stage — builds the Nuxt app (`apps/dj/app`) into `dist/`
2. `golang:*` stage — builds the Go API (`apps/dj/api`) into a static binary
3. Final `debian:stable-slim` stage — copies the Nuxt dist, the Go binary, and a process supervisor (s6-overlay or dumb-init)

The final image runs the Go binary as PID 1 and supervises the Nuxt dev/prod server (whichever `SERVE_FRONTEND` flag dictates).

## Container size expectations

| Stage | Expected size |
|---|---|
| `node` builder | ~1.2 GB |
| `golang` builder | ~800 MB |
| Final image | ~180 MB (slim + binary + Nuxt dist + Node runtime) |

If a PR makes the final image jump above 250 MB, something leaked (probably a `apt install` that didn't clean up, or a debug build flag left on). Investigate before merging.
