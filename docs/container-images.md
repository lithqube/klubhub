# Container images

The production deployment uses one app package, `ghcr.io/lithqube/klubhub-dj-api`, plus PostgreSQL and Garage. Follow [Setup and Self-Hosting](./SELF-HOSTING.md) for the canonical provisioning flow.

## Runtime and release status

The combined app image built from `apps/dj/Dockerfile` runs **Go + Nuxt/Node + Playwright under supervision**. It is not a distroless Go-only image serving a static Nuxt directory. Go owns the public listener on port 8080, serves API routes, and proxies frontend traffic to Nuxt on internal port 3000. That internal port is not published. `SERVE_FRONTEND=true` enables this production behavior; development uses an API-only image and host Nuxt instead.

**Local changes are not a released image.** Fixes in this checkout require a new version publication before production operators can pull them. `docker-compose.prod.yml` defaults to `IMAGE_TAG=v1.0.0`, but this does not assert that the existing tag contains the runtime fixes. Anonymous requests for the existing v1.0.0 package returned 403. Resolve GHCR access and verify the selected release before using it; do not report a local build as a successful registry deployment.

## Production image references

| Service | Default image |
|---|---|
| `db` | `postgres:16-alpine` |
| `storage` | `dxflrs/garage:v2.2.0` |
| `app` (combined) | `ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG:-v1.0.0}` |

The production Compose file is standalone and image-only. Do not merge it with the default development Compose file, which builds the API from local source. Override `IMAGE_REPOSITORY` only when intentionally using a different app registry package. There are no separate `/api` and `/frontend` production packages in this deployment contract.

## Architecture

The publishing workflow targets **`linux/arm64` only**, using the native `ubuntu-24.04-arm` runner. Native amd64 support is not part of this release configuration. A matching local build does not prove that the registry manifest exists or that an amd64 host can run it without emulation.

## Tags and publishing

`.github/workflows/containers-ci.yml` builds from the repository root and uses `apps/dj/Dockerfile`. It publishes to `ghcr.io/lithqube/klubhub-dj-api`; verify that path agrees with the deployment's `IMAGE_REPOSITORY` when renaming or forking the repository.

Two tag paths are supported:

- **Automatic runs from `main`:** when `tag_override` is empty, the workflow publishes `sha-<short SHA>`. This is the rolling CI path; it does not create stable release tags.
- **Manual `workflow_dispatch` runs:** supply `tag_override` for release-style tags such as `v1.0.1`. Inspect the resolved tag in the run before relying on it.

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

Inspect the runnable manifest for `linux/arm64`. Build attestations can appear as additional non-runnable manifests; do not mistake them for extra supported CPU architectures. Record the digest and test the deployed UI, API, storage access, and image generation. Manifest inspection alone does not validate application startup.

## Official references

- [Working with the Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Configuring package access and visibility](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility)
