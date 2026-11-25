# Container images

The production deployment uses one app package, `ghcr.io/lithqube/klubhub-dj-api`, plus PostgreSQL and Garage. Follow [Setup and Self-Hosting](./SELF-HOSTING.md) for the canonical provisioning flow.

## Runtime and release status

The combined app image built from `apps/dj/Dockerfile` runs **Go + Nuxt/Node + Playwright under supervision**. It is not a distroless Go-only image serving a static Nuxt directory. Go owns the public listener on port 8080, serves API routes, and proxies frontend traffic to Nuxt on internal port 3000. That internal port is not published. `SERVE_FRONTEND=true` enables this production behavior; development uses an API-only image and host Nuxt instead.

**Local changes are not a released image.** Fixes in this checkout require a new version publication before production operators can pull them. `docker-compose.prod.yml` defaults to `IMAGE_TAG=v1.2.0`. That tag exists only after the release image is published. `v1.2.0` was tagged before release tags built automatically, so it needs a backfill: run the **Publish KlubHub DJ container image** workflow manually on ref `v1.2.0` with `tag_override=v1.2.0` and verify the manifest below. Later `v*` tags publish their image when pushed. Until then, export `IMAGE_TAG` to the newest `sha-<commit>` tag. The older `v1.0.1` default was never published (only `1.0.1`, no `v`, was) and predates the invoicing work; do not use it. Do not report a local build as a successful registry deployment.

## Production image references

| Service | Default image |
|---|---|
| `db` | `postgres:16-alpine` |
| `storage` | `dxflrs/garage:v2.2.0` |
| `app` (combined) | `ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG:-v1.2.0}` |

The production Compose file is standalone and image-only. Do not merge it with the default development Compose file, which builds the API from local source. Override `IMAGE_REPOSITORY` only when intentionally using a different app registry package. There are no separate `/api` and `/frontend` production packages in this deployment contract.

## Architecture

The publishing workflow builds **`linux/amd64` and `linux/arm64`**, each natively (`ubuntu-24.04` and `ubuntu-24.04-arm`), scans each with Trivy, pushes each by digest, then merges them under one tag and signs it. The merge step fails unless the tag lists exactly `amd64,arm64`.

Tags published before this change (`v1.2.0` and earlier) are `linux/arm64` only. `docker-compose.prod.yml` pins `platform: ${APP_PLATFORM:-linux/arm64}`; on an x86 host set `APP_PLATFORM=linux/amd64` together with an `IMAGE_TAG` that lists both platforms. A matching local build does not prove that the registry manifest exists.

## Tags and publishing

`.github/workflows/containers-ci.yml` builds from the repository root and uses `apps/dj/Dockerfile`. It publishes to `ghcr.io/lithqube/klubhub-dj-api`; verify that path agrees with the deployment's `IMAGE_REPOSITORY` when renaming or forking the repository.

Three tag paths are supported:

- **Automatic runs from `main`:** the workflow publishes `sha-<short SHA>` when DJ files change. This is the rolling CI path; it does not create stable release tags.
- **Release tags:** pushing a git tag `vX.Y.Z` (or `vX.Y.Z-rc.N`) publishes `:vX.Y.Z` plus that commit's `sha-<short SHA>`, whatever files changed. `promoter-ci.yml` does the same for `klubhub-promoter-api`, so one release tag publishes both products. `./klubhub update --release` checks out the newest `v*` tag and pulls the image published for it.
- **Manual `workflow_dispatch` runs:** supply `tag_override` (it must look like `v1.2.0`) to publish or backfill a release tag, for example one pushed before release tags built automatically. Inspect the resolved tag in the run before relying on it.

The workflow disables `latest`. Pin a known release or exact published build, not an assumed moving tag.

Release tags are not created automatically by `v*` push events in this workflow; they remain a manual operator action via `workflow_dispatch` or a separate release workflow.

Publish a new version containing the runtime fixes rather than silently implying that the old v1.0.0 artifact changed. Verify the build, tag, digest, and package access before directing operators to upgrade.

## GHCR visibility and access

The workflow uses `GITHUB_TOKEN` with package-write permission. Successful publication does not by itself establish anonymous pull access: package visibility and access permissions are separate from repository visibility.

For anonymous distribution, the owner must open the `klubhub-dj-api` package settings and make the package public. For a private package, operators need a Docker registry login with package-read permission. Use a credential manager or Docker's supported secure login flow; do not place a token in a shell command or a support report.

A 403 indicates an access failure, not proof that an image contains the fix or even that its manifest was inspected. Do not claim a successful production pull until the intended operator can fetch the selected tag.

## Verify a published image

Set `IMAGE_TAG` to the accessible version you intend to deploy. These commands require it to be explicitly selected and do not fall back to an old tag:

```bash
docker pull "ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG:?Set IMAGE_TAG to the published release}"
docker buildx imagetools inspect "ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG:?Set IMAGE_TAG to the published release}"
```

Inspect the runnable manifests for `linux/amd64` and `linux/arm64` (older tags: `linux/arm64` only). Build attestations can appear as additional non-runnable manifests; do not mistake them for extra supported CPU architectures. Record the digest and test the deployed UI, API, storage access, and image generation. Manifest inspection alone does not validate application startup.

## Official references

- [Working with the Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Configuring package access and visibility](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility)
