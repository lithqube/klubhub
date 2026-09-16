# Container images

Production images for KlubHub DJ are built and published to **GitHub
Container Registry (GHCR)** by `.github/workflows/containers.yml`.

## Images

KlubHub DJ v1.0.0 ships a **single container image** that carries both
the distroless Go API binary and the Nuxt frontend `.output/` build.
The API process serves `/api/v1/*` and the embedded UI on the same
listener, so operators only pull one package.

| Service | Image | Built from |
| --- | --- | --- |
| KlubHub DJ (API + frontend) | `ghcr.io/lithqube/klubhub-dj` | `apps/dj/Dockerfile` |

> **Repository / image naming.** The repository is `lithqube/klubhub`
> on GitHub. `${{ github.repository }}` resolves to
> `lithqube/klubhub-dj` because the live repository was renamed in place
> during the v1.0.0 prep work; the single-image package follows the
> renamed name (`klubhub-dj`, not `klubhub`). Older multi-image
> proposals used `klubhub-api` / `klubhub-frontend` — those are
> obsolete and were never published.

## Architecture: linux/arm64 only

v1.0.0 publishes **linux/arm64 only**. The workflow's
`platforms: linux/arm64` line runs natively on `ubuntu-24.04-arm`
without QEMU emulation. CodeRabbit's suggestion to add a
`linux/amd64,linux/arm64` matrix is intentionally NOT applied for
v1.0.0:

- ARM SBCs (Raspberry Pi 4/5, Apple Silicon dev boxes) and
  Graviton-style cloud instances are the dominant KlubHub host.
- Skipping QEMU keeps CI runs fast, the manifest small, and the
  verification step (`docker buildx imagetools inspect`) trivial.
- linux/amd64 support will be evaluated as a post-v1.0.0 point release
  and is tracked separately.

amd64 operators must run an emulated arm64 container or wait for the
post-release matrix expansion. The release notes call this out as
"What does NOT ship in v1.0.0".

## Tags

- `<short-sha>` — every build, for exact traceability/rollback.
- `v<semver>` — only on pushes of a `v*` tag (e.g. `v1.0.0`), for
  versioned releases. The tag pattern emits the full `v`-prefixed
  string so `IMAGE_TAG=v1.0.0` in `docker-compose.prod.yml` resolves
  to a real published tag. There is no `latest` tag — operators must
  pin an explicit version or short SHA.
- `dev` — produced only by manual `workflow_dispatch` runs that
  override the tag. Never use in production.

## Triggers

- **Tag push** `v*` — descendant of `origin/main` required; builds,
  pushes `ghcr.io/lithqube/klubhub-dj:<vX.Y.Z>` and
  `…-<short-sha>`, and runs the platform-verification step.
- **`workflow_dispatch`** — runs the same pipeline with an optional
  tag override (handy for pre-release `v0.1.0-rc1` style tags). The
  ancestry check is skipped on dispatch.

## One-time publishing setup (repository owner)

The workflow does not change package visibility. GHCR packages
published via the built-in `GITHUB_TOKEN` are created **private** the
first time, regardless of the repository's own visibility, and must
be flipped to public by hand:

1. Push/merge this workflow and let it run once so the package exists.
2. In the repository, open **Packages** (or
   `https://github.com/orgs/lithqube/packages` depending on account
   type) and open `klubhub-dj`.
3. **Package settings → Danger Zone → Change visibility → Public.**
4. Optionally, under the same package's settings, **Manage Actions
   access** to link it back to `lithqube/klubhub` so the repository
   page shows it under "Packages".

No PAT or extra secret is needed for the workflow itself —
`GITHUB_TOKEN` has enough scope to push. A token with
`write:packages` + package-admin rights is only needed if you want to
script the visibility change instead of using the UI.

## Pulling an image

```sh
docker pull ghcr.io/lithqube/klubhub-dj:v1.0.0
# or pin to an exact build:
docker pull ghcr.io/lithqube/klubhub-dj:sha-abcdef1
```

`docker-compose.prod.yml` defaults to `IMAGE_TAG=v1.0.0` and
`IMAGE_REPOSITORY=ghcr.io/lithqube/klubhub-dj`. Public images pull
without authentication once step 3 above is done.

## Verifying the published manifest

```sh
docker buildx imagetools inspect ghcr.io/lithqube/klubhub-dj:v1.0.0 --raw | jq '.manifests[].platform'
# expected: {"architecture":"arm64","os":"linux"}
```

Anything other than a single `linux/arm64` manifest means a build
escaped the workflow's platform restriction — treat as a release bug.

## Official references

- [Working with the Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Configuring a package's access control and visibility](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility)
