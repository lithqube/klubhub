# Container images

Production images for the API and frontend are built and published to **GitHub Container Registry (GHCR)** by `.github/workflows/containers.yml`.

## Images

| Service | Image | Built from |
| --- | --- | --- |
| API (Go) | `ghcr.io/lithqube/klubhub-api` | `api/Dockerfile` |
| Frontend (Nuxt) | `ghcr.io/lithqube/klubhub-frontend` | `apps/dj/Dockerfile` |

## Tags

- `latest` — the most recent build from `main`.
- `<short-sha>` — every build, for exact traceability/rollback.
- `v<semver>` — only on pushes of a `v*` tag (e.g. `v1.2.0`), for versioned releases.

## Triggers

Pushes to `main` or a `v*` tag that touch `api/**`, `apps/dj/**`, the workspace lockfiles, or the workflow itself. Also runnable manually via **Actions → Publish containers → Run workflow**.

## One-time publishing setup (repository owner)

Adding the workflow does not change package visibility. GHCR packages published via the built-in `GITHUB_TOKEN` are created **private** the first time, regardless of the repository's own visibility, and must be flipped to public by hand:

1. Push/merge this workflow and let it run once so each package exists.
2. In the repository, open **Packages** (or `https://github.com/orgs/lithqube/packages` / `https://github.com/users/<owner>/packages` depending on account type) and open `klubhub-api`.
3. **Package settings → Danger Zone → Change visibility → Public.** Repeat for `klubhub-frontend`.
4. Optionally, under the same package's settings, **Manage Actions access** to link it back to `lithqube/klubhub` so the repository page shows it under "Packages".

No PAT or extra secret is needed for the workflow itself — `GITHUB_TOKEN` has enough scope to push. A token with `write:packages` + package-admin rights is only needed if you want to script the visibility change instead of using the UI.

## Pulling an image

```sh
docker pull ghcr.io/lithqube/klubhub-api:latest
docker pull ghcr.io/lithqube/klubhub-frontend:latest
```

Public images pull without authentication once step 3 above is done.

## Official references

- [Working with the Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Configuring a package's access control and visibility](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility)
